package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/iam/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/audit"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// maxPATDays bounds personal access tokens (the create request allows up to 365).
const (
	defaultPATDays = 90
	maxPATDays     = 365
)

type tokenSpec struct {
	TenantID     string
	MembershipID string
	UserID       string
	Kind         string
	Name         string
	TTL          time.Duration
}

// IssuedToken is a new token. Plaintext exists only in this value; it is never stored.
type IssuedToken struct {
	ID        string
	Name      string
	Plaintext string
	ExpiresAt time.Time
}

// issueToken inserts a token row inside tx. The caller's transaction must already be
// scoped to the tenant.
func issueToken(ctx context.Context, tx pgx.Tx, now func() time.Time, spec tokenSpec) (IssuedToken, error) {
	plain, hash, err := newToken(spec.Kind)
	if err != nil {
		return IssuedToken{}, err
	}
	tenant, err := db.ParseUUID(spec.TenantID)
	if err != nil {
		return IssuedToken{}, err
	}
	membership, err := db.ParseUUID(spec.MembershipID)
	if err != nil {
		return IssuedToken{}, err
	}
	user, err := db.ParseUUID(spec.UserID)
	if err != nil {
		return IssuedToken{}, err
	}
	expires := now().Add(spec.TTL)
	row, err := gen.New(tx).InsertAPIToken(ctx, gen.InsertAPITokenParams{
		TenantID:     tenant,
		MembershipID: membership,
		UserID:       user,
		Kind:         spec.Kind,
		Name:         spec.Name,
		TokenHash:    hash,
		ExpiresAt:    pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		return IssuedToken{}, fmt.Errorf("insert %s token: %w", spec.Kind, err)
	}
	return IssuedToken{
		ID:        db.UUIDString(row.ID),
		Name:      spec.Name,
		Plaintext: plain,
		ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

// CreatePAT issues a personal access token for the caller (REQ-IAM-015). The caller must
// hold token.manage, and the token carries the caller's own permissions, never more.
func (s *Service) CreatePAT(ctx context.Context, p Principal, name string, days int) (IssuedToken, error) {
	if days == 0 {
		days = defaultPATDays
	}
	if days < 1 || days > maxPATDays {
		return IssuedToken{}, apperr.Validation("token.expiry_invalid", "Expiry must be between 1 and 365 days.",
			apperr.Field{Field: "expires_in_days", Message: "Between 1 and 365."})
	}
	var issued IssuedToken
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		t, err := issueToken(ctx, tx, s.now, tokenSpec{
			TenantID:     p.TenantID,
			MembershipID: p.MembershipID,
			UserID:       p.UserID,
			Kind:         KindPAT,
			Name:         name,
			TTL:          time.Duration(days) * 24 * time.Hour,
		})
		if err != nil {
			return err
		}
		issued = t
		return audit.Record(ctx, tx, audit.Entry{
			TenantID:    p.TenantID,
			Source:      audit.SourceAPI,
			ActorUserID: p.UserID,
			TokenID:     p.TokenID,
			Action:      "token.created",
			Entity:      "api_token",
			EntityID:    t.ID,
			Summary:     map[string]any{"name": name, "expires_at": t.ExpiresAt.UTC().Format(time.RFC3339)},
		})
	})
	if err != nil {
		return IssuedToken{}, err
	}
	return issued, nil
}

// Authenticate turns a bearer token into a principal with the caller's permissions in the
// token's tenant. It is the only place a token is resolved. Failures are 401, always with
// the same message for unknown and revoked tokens, so a probe learns nothing.
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	unauth := apperr.Unauthenticated("auth.token_invalid", "The token is not valid.")
	if kind := kindOf(token); kind != KindSession && kind != KindPAT {
		return Principal{}, unauth
	}

	rec, err := resolveAPIToken(ctx, s.pool, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, unauth
	}
	if err != nil {
		return Principal{}, fmt.Errorf("resolve token: %w", err)
	}
	if rec.Kind != kindOf(token) || rec.RevokedAt != nil {
		return Principal{}, unauth
	}
	if !s.now().Before(rec.ExpiresAt) {
		return Principal{}, apperr.Unauthenticated("auth.token_expired", "The token has expired. Sign in again.")
	}

	var p Principal
	err = db.WithTenant(ctx, s.pool, rec.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(rec.TenantID)
		if err != nil {
			return err
		}
		user, err := db.ParseUUID(rec.UserID)
		if err != nil {
			return err
		}
		// A deactivated membership ends access at once, even for a live token (US-IAM-005).
		if _, err := q.ActiveMembership(ctx, gen.ActiveMembershipParams{TenantID: tenant, UserID: user}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return unauth
			}
			return fmt.Errorf("check membership: %w", err)
		}
		membership, err := db.ParseUUID(rec.MembershipID)
		if err != nil {
			return err
		}
		tokenID, err := db.ParseUUID(rec.TokenID)
		if err != nil {
			return err
		}
		roles, err := q.RoleNamesForMembership(ctx, gen.RoleNamesForMembershipParams{
			TenantID:     tenant,
			MembershipID: membership,
		})
		if err != nil {
			return fmt.Errorf("load roles: %w", err)
		}
		if err := q.TouchAPIToken(ctx, gen.TouchAPITokenParams{TenantID: tenant, ID: tokenID}); err != nil {
			return fmt.Errorf("touch token: %w", err)
		}
		p = Principal{
			TenantID:     rec.TenantID,
			UserID:       rec.UserID,
			MembershipID: rec.MembershipID,
			TokenID:      rec.TokenID,
			TokenKind:    rec.Kind,
			permissions:  permissionsFor(roles),
		}
		return nil
	})
	if err != nil {
		return Principal{}, err
	}
	return p, nil
}

// tenantRef is a row from the tenant directory.
type tenantRef struct {
	ID   string
	Slug string
	Name string
}

// tokenRecord is what the controlled lookup returns for a token hash.
type tokenRecord struct {
	TokenID      string
	TenantID     string
	MembershipID string
	UserID       string
	Kind         string
	ExpiresAt    time.Time
	RevokedAt    *time.Time
}

// rowQuerier is satisfied by *pgxpool.Pool. The lookups below run outside any tenant
// transaction on purpose: they are the two controlled, SECURITY DEFINER reads (ADR-0011 §3).
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var _ rowQuerier = (*pgxpool.Pool)(nil)
