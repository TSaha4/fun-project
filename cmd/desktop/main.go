package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"example.com/local-capture/internal/desktop"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Vault wiring is intentionally absent until the security design is approved and
	// an encrypted application service exists. The companion therefore starts locked.
	if err := desktop.Run(ctx, desktop.Config{}); err != nil {
		slog.Error("desktop companion stopped", "error", err)
		os.Exit(1)
	}
}
