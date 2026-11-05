package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/httpapi"
	"example.com/local-capture/internal/mcp"
	"example.com/local-capture/internal/store/sqlite"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ids struct{}

func (ids) NewID() (string, error) { return httpapi.GenerateToken() }

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "grant" || os.Args[1] == "revoke" || os.Args[1] == "list-grants") {
		if err := grantCommand(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	if err := serve(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func grantPath() (string, error) {
	dir := os.Getenv("APP_DATA_DIR")
	if dir == "" {
		dir = "./data"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(abs, "mcp-grants.json"), nil
}

func grantCommand(command string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	client := flags.String("client", "", "configured MCP client identity")
	scopeText := flags.String("scopes", "", "comma-separated permission scopes")
	grantFile := flags.String("grant-file", "", "MCP grants file (default APP_DATA_DIR/mcp-grants.json)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *client == "" {
		return fmt.Errorf("--client is required")
	}
	path := *grantFile
	if path == "" {
		var err error
		path, err = grantPath()
		if err != nil {
			return err
		}
	}
	store, err := mcp.OpenGrantStore(path)
	if err != nil {
		return err
	}
	switch command {
	case "grant":
		if strings.TrimSpace(*scopeText) == "" {
			return fmt.Errorf("--scopes is required; available scopes: metadata:read, notes:read, attachments:read, notes:create, notes:edit, notes:archive, notes:delete, collections:read, collections:write")
		}
		items := strings.Split(*scopeText, ",")
		scopes := make([]mcp.Scope, 0, len(items))
		for _, item := range items {
			scopes = append(scopes, mcp.Scope(strings.TrimSpace(item)))
		}
		if err := store.Grant(*client, scopes); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Granted scopes to %s: %s\n", *client, strings.Join(strings.Split(*scopeText, ","), ", "))
	case "revoke":
		if err := store.Revoke(*client); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "Revoked MCP access for %s\n", *client)
	case "list-grants":
		fmt.Fprintf(os.Stdout, "%s: %s\n", *client, joinScopes(store.Scopes(*client)))
	}
	return nil
}
func joinScopes(scopes []mcp.Scope) string {
	parts := make([]string, len(scopes))
	for i, v := range scopes {
		parts[i] = string(v)
	}
	return strings.Join(parts, ", ")
}

func serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	client := flags.String("client-id", "", "identity key for the configured MCP client")
	dataDir := flags.String("data-dir", os.Getenv("APP_DATA_DIR"), "local application data directory (default ./data)")
	grantFile := flags.String("grant-file", "", "MCP grants file (default APP_DATA_DIR/mcp-grants.json)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *client == "" {
		return fmt.Errorf("--client-id is required; configure one ID per MCP client")
	}
	if *dataDir == "" {
		*dataDir = "./data"
	}
	abs, err := filepath.Abs(*dataDir)
	if err != nil {
		return err
	}
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(abs, "capture.db"))
	if err != nil {
		return fmt.Errorf("open local application database: %w", err)
	}
	defer store.Close()
	service := app.NewService(store, store, store, store, ids{}, clock{}, store)
	reminders := app.NewReminderService(store, store, ids{}, clock{})
	grantPath := *grantFile
	if grantPath == "" {
		grantPath, err = grantPathDefault()
		if err != nil {
			return err
		}
	}
	grants, err := mcp.OpenGrantStore(grantPath)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	adapter, err := mcp.NewServer(service, reminders, mcp.ServerConfig{ClientID: *client, Grants: grants, Auditor: mcp.SlogAuditor{Logger: logger}})
	if err != nil {
		return err
	}
	return adapter.Run(ctx, &sdk.StdioTransport{})
}

func grantPathDefault() (string, error) { return grantPath() }
