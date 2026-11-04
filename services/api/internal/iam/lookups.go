package iam

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// These two reads are hand-written, not sqlc. sqlc cannot type a SQL function that returns
// TABLE(...), so the queries call the SECURITY DEFINER functions directly (ADR-0012).
// Errors are returned unwrapped so callers can test for pgx.ErrNoRows.

// tenantBySlug resolves a tenant from its slug through the controlled directory function.
func tenantBySlug(ctx context.Context, q rowQuerier, slug string) (tenantRef, error) {
	var id pgtype.UUID
	var t tenantRef
	err := q.QueryRow(ctx,
		`SELECT id, slug, name FROM tenant_id_by_slug($1)`, slug,
	).Scan(&id, &t.Slug, &t.Name)
	if err != nil {
		return tenantRef{}, err
	}
	t.ID = db.UUIDString(id)
	return t, nil
}

// resolveAPIToken finds a token by its hash through the controlled lookup function.
// It returns no fields a caller could use to reach another tenant's data beyond the tenant ID
// the token already belongs to.
func resolveAPIToken(ctx context.Context, q rowQuerier, hash []byte) (tokenRecord, error) {
	var tokenID, tenantID, membershipID, userID pgtype.UUID
	var rec tokenRecord
	var expires pgtype.Timestamptz
	var revoked pgtype.Timestamptz
	err := q.QueryRow(ctx,
		`SELECT token_id, tenant_id, membership_id, user_id, kind, expires_at, revoked_at
		   FROM resolve_api_token($1)`, hash,
	).Scan(&tokenID, &tenantID, &membershipID, &userID, &rec.Kind, &expires, &revoked)
	if err != nil {
		return tokenRecord{}, err
	}
	rec.TokenID = db.UUIDString(tokenID)
	rec.TenantID = db.UUIDString(tenantID)
	rec.MembershipID = db.UUIDString(membershipID)
	rec.UserID = db.UUIDString(userID)
	rec.ExpiresAt = expires.Time
	if revoked.Valid {
		t := revoked.Time
		rec.RevokedAt = &t
	}
	return rec, nil
}
