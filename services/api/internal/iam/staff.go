package iam

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/iam/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/audit"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// Staff management (ADR-0019): add a person to the school, give and take roles, switch access off.
// Every change writes an audit entry in the same transaction. Permissions are checked by the handler;
// the rules about owners are checked here, where the data is.

// StaffMember is a person with access to one school.
type StaffMember struct {
	ID        string // the membership id
	UserID    string
	Mobile    string
	FullName  string
	Status    string // active or deactivated
	Roles     []string
	CreatedAt time.Time
}

var errStaffNotFound = apperr.NotFound("staff.not_found", "Staff member not found.")

// CreateStaff adds a person by mobile number with their starting roles. If the number already belongs
// to a person (here or in another school) it is the same person; the response never says which.
func (s *Service) CreateStaff(ctx context.Context, p Principal, in StaffInput) (StaffMember, error) {
	in, err := validateStaff(in)
	if err != nil {
		return StaffMember{}, err
	}
	var out StaffMember
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		actorRoles, err := s.rolesForOwnerRule(ctx, q, p, tenant, slices.Contains(in.Roles, "owner"))
		if err != nil {
			return err
		}
		for _, r := range in.Roles {
			if err := mayChangeRole(actorRoles, r); err != nil {
				return err
			}
		}
		userID, err := q.EnsureUser(ctx, gen.EnsureUserParams{Mobile: in.Mobile, FullName: in.FullName})
		if err != nil {
			return fmt.Errorf("ensure user: %w", err)
		}
		membershipID, err := q.InsertStaffMembership(ctx, gen.InsertStaffMembershipParams{
			TenantID: tenant, UserID: userID, DisplayName: pgtype.Text{String: in.FullName, Valid: true}, CreatedBy: actor,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("staff.already_member", "This person is already on your staff. Open them to change their roles or reactivate them.")
		}
		if err != nil {
			return fmt.Errorf("insert membership: %w", err)
		}
		for _, r := range in.Roles {
			if _, err := q.AssignRole(ctx, gen.AssignRoleParams{TenantID: tenant, MembershipID: membershipID, Role: r, CreatedBy: actor}); err != nil {
				return fmt.Errorf("assign role %s: %w", r, err)
			}
		}
		if err := s.auditStaff(ctx, tx, p, "staff.created", db.UUIDString(membershipID), map[string]any{"roles": in.Roles}); err != nil {
			return err
		}
		out, err = loadOne(ctx, q, tenant, membershipID)
		return err
	})
	return out, err
}

// ListStaff returns the school's staff, optionally only active or only deactivated.
func (s *Service) ListStaff(ctx context.Context, p Principal, status string) ([]StaffMember, error) {
	if status != "" && status != "active" && status != "deactivated" {
		return nil, apperr.Validation("staff.status_invalid", "status must be active or deactivated.")
	}
	var out []StaffMember
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		params := gen.ListStaffParams{TenantID: tenant}
		if status != "" {
			params.Status = pgtype.Text{String: status, Valid: true}
		}
		rows, err := q.ListStaff(ctx, params)
		if err != nil {
			return fmt.Errorf("list staff: %w", err)
		}
		out, err = withRoles(ctx, q, tenant, rows)
		return err
	})
	return out, err
}

// GetStaff returns one staff member.
func (s *Service) GetStaff(ctx context.Context, p Principal, staffID string) (StaffMember, error) {
	var out StaffMember
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		id, err := db.ParseUUID(staffID)
		if err != nil {
			return errStaffNotFound
		}
		out, err = loadOne(ctx, gen.New(tx), tenant, id)
		return err
	})
	return out, err
}

// AssignRole gives a staff member a role. Holding it already is a 409; a deactivated person cannot be given roles.
func (s *Service) AssignRole(ctx context.Context, p Principal, staffID, role string) (StaffMember, error) {
	if !knownRole(role) {
		return StaffMember{}, apperr.Validation("staff.invalid", "Unknown role.", apperr.Field{Field: "role", Message: "Unknown role: " + role + "."})
	}
	var out StaffMember
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		id, err := db.ParseUUID(staffID)
		if err != nil {
			return errStaffNotFound
		}
		actorRoles, err := s.rolesForOwnerRule(ctx, q, p, tenant, role == "owner")
		if err != nil {
			return err
		}
		if err := mayChangeRole(actorRoles, role); err != nil {
			return err
		}
		member, err := loadOne(ctx, q, tenant, id)
		if err != nil {
			return err
		}
		if member.Status != "active" {
			return apperr.Conflict("staff.deactivated", "This person is deactivated. Reactivate them before changing their roles.")
		}
		if _, err := q.AssignRole(ctx, gen.AssignRoleParams{TenantID: tenant, MembershipID: id, Role: role, CreatedBy: actor}); err != nil {
			if db.IsUniqueViolation(err) {
				return apperr.Conflict("staff.role_already_assigned", "This person already has that role.")
			}
			return fmt.Errorf("assign role: %w", err)
		}
		if err := s.auditStaff(ctx, tx, p, "role.assigned", staffID, map[string]any{"role": role}); err != nil {
			return err
		}
		out, err = loadOne(ctx, q, tenant, id)
		return err
	})
	return out, err
}

// RevokeRole takes a role away. The last active owner keeps the owner role.
func (s *Service) RevokeRole(ctx context.Context, p Principal, staffID, role string) (StaffMember, error) {
	if !knownRole(role) {
		return StaffMember{}, apperr.Validation("staff.invalid", "Unknown role.", apperr.Field{Field: "role", Message: "Unknown role: " + role + "."})
	}
	var out StaffMember
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		id, err := db.ParseUUID(staffID)
		if err != nil {
			return errStaffNotFound
		}
		actorRoles, err := s.rolesForOwnerRule(ctx, q, p, tenant, role == "owner")
		if err != nil {
			return err
		}
		if err := mayChangeRole(actorRoles, role); err != nil {
			return err
		}
		member, err := loadOne(ctx, q, tenant, id)
		if err != nil {
			return err
		}
		if role == "owner" {
			owners, err := q.CountActiveOwners(ctx, tenant)
			if err != nil {
				return fmt.Errorf("count owners: %w", err)
			}
			if err := lastOwnerGuard(int(owners), member.Status == "active" && slices.Contains(member.Roles, "owner")); err != nil {
				return err
			}
		}
		if _, err := q.RevokeRole(ctx, gen.RevokeRoleParams{TenantID: tenant, MembershipID: id, Role: role, RevokedBy: actor}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("staff.role_not_assigned", "This person does not have that role.")
			}
			return fmt.Errorf("revoke role: %w", err)
		}
		if err := s.auditStaff(ctx, tx, p, "role.revoked", staffID, map[string]any{"role": role}); err != nil {
			return err
		}
		out, err = loadOne(ctx, q, tenant, id)
		if err != nil {
			return err
		}
		// REQ-IAM-006: revoking a person's last role ends their access at once, history kept.
		if len(out.Roles) == 0 && out.Status == "active" {
			if err := notSelf(p.MembershipID, staffID); err != nil {
				return apperr.Conflict("staff.self_deactivate", "That is your last role, and taking it would switch you off. Ask another owner.")
			}
			if err := s.switchOff(ctx, tx, q, p, tenant, actor, id, map[string]any{"reason": "last role revoked"}); err != nil {
				return err
			}
			out, err = loadOne(ctx, q, tenant, id)
		}
		return err
	})
	return out, err
}

// DeactivateStaff switches a person off. Their tokens are revoked, and because every request checks
// the membership, access ends on their next request (US-IAM-005). History is kept.
func (s *Service) DeactivateStaff(ctx context.Context, p Principal, staffID string) (StaffMember, error) {
	if err := notSelf(p.MembershipID, staffID); err != nil {
		return StaffMember{}, err
	}
	var out StaffMember
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		id, err := db.ParseUUID(staffID)
		if err != nil {
			return errStaffNotFound
		}
		if err := q.LockOwnerChanges(ctx, db.UUIDString(tenant)); err != nil {
			return fmt.Errorf("lock owner changes: %w", err)
		}
		member, err := loadOne(ctx, q, tenant, id)
		if err != nil {
			return err
		}
		if member.Status != "active" {
			return apperr.Conflict("staff.already_deactivated", "This person is already deactivated.")
		}
		owners, err := q.CountActiveOwners(ctx, tenant)
		if err != nil {
			return fmt.Errorf("count owners: %w", err)
		}
		targetIsOwner := slices.Contains(member.Roles, "owner")
		if err := lastOwnerGuard(int(owners), targetIsOwner); err != nil {
			return err
		}
		if targetIsOwner {
			actorRoles, err := s.liveActorRoles(ctx, q, p, tenant)
			if err != nil {
				return err
			}
			if err := mayChangeRole(actorRoles, "owner"); err != nil {
				return err
			}
		}
		if err := s.switchOff(ctx, tx, q, p, tenant, actor, id, map[string]any{}); err != nil {
			return err
		}
		out, err = loadOne(ctx, q, tenant, id)
		return err
	})
	return out, err
}

// switchOff deactivates a membership and revokes its tokens, with one audit entry.
func (s *Service) switchOff(ctx context.Context, tx pgx.Tx, q *gen.Queries, p Principal, tenant, actor, id pgtype.UUID, summary map[string]any) error {
	if _, err := q.DeactivateMembership(ctx, gen.DeactivateMembershipParams{TenantID: tenant, ID: id, DeactivatedBy: actor}); err != nil {
		return fmt.Errorf("deactivate: %w", err)
	}
	revoked, err := q.RevokeMembershipTokens(ctx, gen.RevokeMembershipTokensParams{TenantID: tenant, MembershipID: id})
	if err != nil {
		return fmt.Errorf("revoke tokens: %w", err)
	}
	summary["tokens_revoked"] = revoked
	return s.auditStaff(ctx, tx, p, "staff.deactivated", db.UUIDString(id), summary)
}

// ReactivateStaff switches a deactivated person back on. They keep their roles and sign in again.
func (s *Service) ReactivateStaff(ctx context.Context, p Principal, staffID string) (StaffMember, error) {
	var out StaffMember
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		id, err := db.ParseUUID(staffID)
		if err != nil {
			return errStaffNotFound
		}
		if err := q.LockOwnerChanges(ctx, db.UUIDString(tenant)); err != nil {
			return fmt.Errorf("lock owner changes: %w", err)
		}
		member, err := loadOne(ctx, q, tenant, id)
		if err != nil {
			return err
		}
		if member.Status != "deactivated" {
			return apperr.Conflict("staff.already_active", "This person is already active.")
		}
		if slices.Contains(member.Roles, "owner") {
			actorRoles, err := s.liveActorRoles(ctx, q, p, tenant)
			if err != nil {
				return err
			}
			if err := mayChangeRole(actorRoles, "owner"); err != nil {
				return err
			}
		}
		if _, err := q.ReactivateMembership(ctx, gen.ReactivateMembershipParams{TenantID: tenant, ID: id}); err != nil {
			return fmt.Errorf("reactivate: %w", err)
		}
		if err := s.auditStaff(ctx, tx, p, "staff.reactivated", staffID, map[string]any{}); err != nil {
			return err
		}
		out, err = loadOne(ctx, q, tenant, id)
		return err
	})
	return out, err
}

// ---- helpers --------------------------------------------------------------------------

// rolesForOwnerRule returns the roles to judge the owner-only rule with. Normally the roles from
// sign-in are enough, but a change that touches the owner role takes the per-school owner lock first
// and then re-reads the caller's roles inside the transaction, so someone demoted or switched off a
// moment ago cannot still promote anyone.
func (s *Service) rolesForOwnerRule(ctx context.Context, q *gen.Queries, p Principal, tenant pgtype.UUID, touchesOwner bool) ([]string, error) {
	if !touchesOwner {
		return p.roles, nil
	}
	if err := q.LockOwnerChanges(ctx, db.UUIDString(tenant)); err != nil {
		return nil, fmt.Errorf("lock owner changes: %w", err)
	}
	return s.liveActorRoles(ctx, q, p, tenant)
}

// liveActorRoles reads the caller's current roles and refuses a caller who is no longer active.
func (s *Service) liveActorRoles(ctx context.Context, q *gen.Queries, p Principal, tenant pgtype.UUID) ([]string, error) {
	id, err := db.ParseUUID(p.MembershipID)
	if err != nil {
		return nil, err
	}
	me, err := loadOne(ctx, q, tenant, id)
	if err != nil || me.Status != "active" {
		return nil, apperr.Unauthenticated("auth.token_invalid", "The token is not valid.")
	}
	return me.Roles, nil
}

func ids(p Principal) (tenant, actor pgtype.UUID, err error) {
	if tenant, err = db.ParseUUID(p.TenantID); err != nil {
		return
	}
	actor, err = db.ParseUUID(p.UserID)
	return
}

func loadOne(ctx context.Context, q *gen.Queries, tenant, id pgtype.UUID) (StaffMember, error) {
	row, err := q.GetStaff(ctx, gen.GetStaffParams{TenantID: tenant, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return StaffMember{}, errStaffNotFound
	}
	if err != nil {
		return StaffMember{}, fmt.Errorf("get staff: %w", err)
	}
	list, err := withRoles(ctx, q, tenant, []gen.ListStaffRow{gen.ListStaffRow(row)})
	if err != nil {
		return StaffMember{}, err
	}
	return list[0], nil
}

func withRoles(ctx context.Context, q *gen.Queries, tenant pgtype.UUID, rows []gen.ListStaffRow) ([]StaffMember, error) {
	idList := make([]pgtype.UUID, 0, len(rows))
	for _, r := range rows {
		idList = append(idList, r.ID)
	}
	roleRows, err := q.LiveRolesForMemberships(ctx, gen.LiveRolesForMembershipsParams{TenantID: tenant, MembershipIds: idList})
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	byMember := map[string][]string{}
	for _, r := range roleRows {
		k := db.UUIDString(r.MembershipID)
		byMember[k] = append(byMember[k], r.Role)
	}
	out := make([]StaffMember, 0, len(rows))
	for _, r := range rows {
		id := db.UUIDString(r.ID)
		roles := byMember[id]
		if roles == nil {
			roles = []string{}
		}
		out = append(out, StaffMember{
			ID: id, UserID: db.UUIDString(r.UserID), Mobile: r.Mobile, FullName: r.FullName,
			Status: r.Status, Roles: roles, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out, nil
}

func (s *Service) auditStaff(ctx context.Context, tx pgx.Tx, p Principal, action, membershipID string, summary map[string]any) error {
	return audit.Record(ctx, tx, audit.Entry{
		TenantID: p.TenantID, Source: audit.SourceAPI, ActorUserID: p.UserID, TokenID: p.TokenID,
		Action: action, Entity: "membership", EntityID: membershipID, Summary: summary,
	})
}
