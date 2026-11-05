package mcp

import (
	"context"
	"log/slog"
)

// SlogAuditor writes a fixed set of metadata fields. It never accepts or logs
// tool arguments, resource contents, note IDs, or application error strings.
type SlogAuditor struct{ Logger *slog.Logger }

func (a SlogAuditor) Record(ctx context.Context, event AuditEvent) {
	if a.Logger == nil {
		return
	}
	a.Logger.InfoContext(ctx, "MCP access", "client_id", event.ClientID, "action", event.Action, "scope", string(event.Scope), "outcome", event.Outcome, "at", event.At.UTC().Format("2006-01-02T15:04:05.000000000Z"))
}
