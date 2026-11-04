package iam

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/iam/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/audit"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/phone"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9-]{3,40}$`)

// BootstrapInput creates the first tenant and its owner.
type BootstrapInput struct {
	TenantSlug  string
	TenantName  string
	OwnerMobile string
	OwnerName   string
}

// Bootstrap creates a tenant with one owner membership. It runs as the migration owner
// (ADR-0011 §1), outside the API, and is the only way to create a tenant until an admin
// console exists (DEC-014). It writes an audit entry with source "cli".
func Bootstrap(ctx context.Context, admin *pgxpool.Pool, in BootstrapInput) (string, error) {
	slug := strings.TrimSpace(in.TenantSlug)
	if !slugPattern.MatchString(slug) {
		return "", apperr.Validation("bootstrap.slug_invalid", "Slug must be 3 to 40 characters: a-z, 0-9, hyphen.")
	}
	name := strings.TrimSpace(in.TenantName)
	if name == "" {
		return "", apperr.Validation("bootstrap.name_required", "A tenant name is required.")
	}
	mobile, err := phone.Normalize(in.OwnerMobile)
	if err != nil {
		return "", apperr.Validation("bootstrap.mobile_invalid", "The owner mobile is not a Pakistani mobile number.")
	}
	ownerName := strings.TrimSpace(in.OwnerName)
	if ownerName == "" {
		return "", apperr.Validation("bootstrap.owner_name_required", "An owner name is required.")
	}

	var tenantID string
	err = pgx.BeginFunc(ctx, admin, func(tx pgx.Tx) error {
		q := gen.New(tx)

		tid, err := q.InsertTenant(ctx, gen.InsertTenantParams{Slug: slug, Name: name})
		if err != nil {
			return fmt.Errorf("insert tenant (is the slug taken?): %w", err)
		}
		tenantID = db.UUIDString(tid)

		// The person may already exist from another tenant: one identity, many memberships (REQ-IAM-001).
		user, err := q.FindUserByMobile(ctx, mobile)
		var userID = user.ID
		if errors.Is(err, pgx.ErrNoRows) {
			userID, err = q.InsertUser(ctx, gen.InsertUserParams{Mobile: mobile, FullName: ownerName})
		}
		if err != nil {
			return fmt.Errorf("find or create owner user: %w", err)
		}

		membership, err := q.InsertMembership(ctx, gen.InsertMembershipParams{TenantID: tid, UserID: userID})
		if err != nil {
			return fmt.Errorf("insert membership: %w", err)
		}
		if err := q.InsertRoleAssignment(ctx, gen.InsertRoleAssignmentParams{
			TenantID:     tid,
			MembershipID: membership,
			Role:         "owner",
		}); err != nil {
			return fmt.Errorf("assign owner role: %w", err)
		}

		return audit.Record(ctx, tx, audit.Entry{
			TenantID: tenantID,
			Source:   audit.SourceCLI,
			Action:   "tenant.bootstrapped",
			Entity:   "tenant",
			EntityID: tenantID,
			Summary:  map[string]any{"slug": slug, "owner_mobile": phone.Mask(mobile)},
		})
	})
	if err != nil {
		return "", err
	}
	return tenantID, nil
}
