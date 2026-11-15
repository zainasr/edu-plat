package iam

import (
	"slices"
	"sort"
	"strings"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/phone"
)

// Staff rules: pure functions, no database (ADR-0019).

const maxStaffName = 80

// StaffInput is a request to add a staff member.
type StaffInput struct {
	Mobile   string
	FullName string
	Roles    []string
}

// RoleInfo describes a built-in role for the roles list.
type RoleInfo struct {
	Name        string
	Permissions []string
}

func knownRole(name string) bool {
	_, ok := rolePermissions[name]
	return ok
}

// RoleCatalogue lists the built-in roles with their permissions, both sorted so the answer is stable.
func RoleCatalogue() []RoleInfo {
	out := make([]RoleInfo, 0, len(rolePermissions))
	for name, perms := range rolePermissions {
		sorted := slices.Clone(perms)
		sort.Strings(sorted)
		out = append(out, RoleInfo{Name: name, Permissions: sorted})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// validateStaff normalises the mobile number and name and checks the roles.
func validateStaff(in StaffInput) (StaffInput, error) {
	var fields []apperr.Field
	add := func(field, msg string) { fields = append(fields, apperr.Field{Field: field, Message: msg}) }

	mobile, err := phone.Normalize(in.Mobile)
	if err != nil {
		add("mobile", "Enter a Pakistani mobile number, such as 03001234567.")
	}
	in.Mobile = mobile
	in.FullName = strings.TrimSpace(in.FullName)
	if in.FullName == "" || len(in.FullName) > maxStaffName {
		add("full_name", "Required, up to 80 characters.")
	}
	if len(in.Roles) == 0 {
		add("roles", "Give at least one role.")
	}
	seen := map[string]bool{}
	for _, r := range in.Roles {
		if !knownRole(r) {
			add("roles", "Unknown role: "+r+".")
		} else if seen[r] {
			add("roles", "Each role may be listed once.")
		}
		seen[r] = true
	}
	if len(fields) > 0 {
		return StaffInput{}, apperr.Validation("staff.invalid", "Some fields need attention.", fields...)
	}
	return in, nil
}

// mayChangeRole is the owner-only rule: only an owner gives or takes the owner role, so nobody with
// role.manage can make themselves an owner. Other roles are decided by the caller's permission.
func mayChangeRole(actorRoles []string, role string) error {
	if role == "owner" && !slices.Contains(actorRoles, "owner") {
		return apperr.Forbidden("staff.owner_only", "Only an owner can give or take the owner role.")
	}
	return nil
}

// lastOwnerGuard refuses to remove the only active owner (by revoking the role or deactivating them).
func lastOwnerGuard(activeOwners int, targetIsActiveOwner bool) error {
	if targetIsActiveOwner && activeOwners <= 1 {
		return apperr.Conflict("staff.last_owner", "A school must keep at least one active owner. Make someone else an owner first.")
	}
	return nil
}

func notSelf(actorMembershipID, targetMembershipID string) error {
	if actorMembershipID == targetMembershipID {
		return apperr.Conflict("staff.self_deactivate", "You cannot deactivate yourself.")
	}
	return nil
}
