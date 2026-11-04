// Package audit writes the append-only audit log (REQ-IAM-020, REQ-API-017).
// Record runs inside the same transaction as the change it describes, so an action
// and its audit entry commit or roll back together.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/platform/audit/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/reqctx"
)

// Source says where the action came from.
const (
	SourceAPI = "api"
	SourceCLI = "cli"
)

// Entry describes one audited action.
type Entry struct {
	TenantID    string
	Source      string
	ActorUserID string // empty for CLI actions
	TokenID     string // empty for CLI actions
	Action      string // e.g. "student.created"
	Entity      string // e.g. "student"
	EntityID    string
	Summary     map[string]any // never secrets, OTPs or full mobile numbers
}

// Record writes one entry inside tx. The request ID is taken from ctx.
func Record(ctx context.Context, tx pgx.Tx, e Entry) error {
	tenant, err := db.ParseUUID(e.TenantID)
	if err != nil {
		return fmt.Errorf("audit tenant: %w", err)
	}
	entityID, err := db.ParseUUID(e.EntityID)
	if err != nil {
		return fmt.Errorf("audit entity id: %w", err)
	}
	actor, err := optionalUUID(e.ActorUserID)
	if err != nil {
		return fmt.Errorf("audit actor: %w", err)
	}
	token, err := optionalUUID(e.TokenID)
	if err != nil {
		return fmt.Errorf("audit token: %w", err)
	}
	summary, err := json.Marshal(e.Summary)
	if err != nil {
		return fmt.Errorf("audit summary: %w", err)
	}

	params := gen.InsertAuditEntryParams{
		TenantID:    tenant,
		Source:      e.Source,
		ActorUserID: actor,
		TokenID:     token,
		RequestID:   reqctx.RequestID(ctx),
		Action:      e.Action,
		Entity:      e.Entity,
		EntityID:    entityID,
		Summary:     summary,
	}
	if err := gen.New(tx).InsertAuditEntry(ctx, params); err != nil {
		return fmt.Errorf("insert audit entry %s: %w", e.Action, err)
	}
	return nil
}

func optionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return db.ParseUUID(s)
}
