package iam

import (
	"slices"
	"strings"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

func codeOf(t *testing.T, err error) string {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("want an apperr, got %T: %v", err, err)
	}
	return e.Code
}

func fieldsOf(err error) []string {
	e, ok := apperr.As(err)
	if !ok {
		return nil
	}
	var out []string
	for _, d := range e.Details {
		out = append(out, d.Field)
	}
	return out
}

// US-IAM-001, REQ-IAM-005: what may be sent to add a staff member.
func TestUS_IAM_001_ValidateStaff(t *testing.T) {
	good, err := validateStaff(StaffInput{Mobile: "0300 1234567", FullName: "  Ayesha Noor ", Roles: []string{"accountant", "receptionist"}})
	if err != nil || good.Mobile != "+923001234567" || good.FullName != "Ayesha Noor" {
		t.Fatalf("good input: %+v %v", good, err)
	}
	cases := []struct {
		name  string
		in    StaffInput
		field string
	}{
		{"bad mobile", StaffInput{Mobile: "12345", FullName: "A", Roles: []string{"accountant"}}, "mobile"},
		{"no name", StaffInput{Mobile: "03001234567", FullName: "  ", Roles: []string{"accountant"}}, "full_name"},
		{"name too long", StaffInput{Mobile: "03001234567", FullName: strings.Repeat("x", 81), Roles: []string{"accountant"}}, "full_name"},
		{"no roles", StaffInput{Mobile: "03001234567", FullName: "A"}, "roles"},
		{"unknown role", StaffInput{Mobile: "03001234567", FullName: "A", Roles: []string{"principal"}}, "roles"},
		{"duplicate role", StaffInput{Mobile: "03001234567", FullName: "A", Roles: []string{"accountant", "accountant"}}, "roles"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validateStaff(c.in)
			if err == nil || codeOf(t, err) != "staff.invalid" || !slices.Contains(fieldsOf(err), c.field) {
				t.Fatalf("want staff.invalid on %q, got %v", c.field, err)
			}
		})
	}
	if _, err := validateStaff(StaffInput{Mobile: "03001234567", FullName: strings.Repeat("x", 80), Roles: []string{"owner"}}); err != nil {
		t.Fatalf("an 80 character name is allowed: %v", err)
	}
}

// Owner decision 2026-11-15: only an owner may give or take the owner role.
func TestUS_IAM_001_OnlyAnOwnerChangesTheOwnerRole(t *testing.T) {
	if err := mayChangeRole([]string{"owner"}, "owner"); err != nil {
		t.Fatalf("an owner may: %v", err)
	}
	for _, actor := range [][]string{{"accountant"}, {"receptionist", "accountant"}, nil} {
		if err := mayChangeRole(actor, "owner"); err == nil || codeOf(t, err) != "staff.owner_only" {
			t.Fatalf("%v must not change the owner role: %v", actor, err)
		}
	}
	if err := mayChangeRole([]string{"accountant"}, "receptionist"); err != nil {
		t.Fatalf("any other role is the caller's permission to decide, not this rule: %v", err)
	}
}

// The last active owner can never be removed: a school must not lock itself out.
func TestUS_IAM_005_TheLastOwnerStays(t *testing.T) {
	cases := []struct {
		name          string
		activeOwners  int
		targetIsOwner bool
		refused       bool
	}{
		{"one owner, removing them", 1, true, true},
		{"zero owners left is refused too", 0, true, true},
		{"two owners, removing one", 2, true, false},
		{"removing someone who is not an owner", 1, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := lastOwnerGuard(c.activeOwners, c.targetIsOwner)
			if c.refused && (err == nil || codeOf(t, err) != "staff.last_owner") {
				t.Fatalf("must be refused with staff.last_owner, got %v", err)
			}
			if !c.refused && err != nil {
				t.Fatalf("must be allowed: %v", err)
			}
		})
	}
}

func TestUS_IAM_005_NoSelfDeactivation(t *testing.T) {
	if err := notSelf("m-1", "m-1"); err == nil || codeOf(t, err) != "staff.self_deactivate" {
		t.Fatalf("got %v", err)
	}
	if err := notSelf("m-1", "m-2"); err != nil {
		t.Fatalf("got %v", err)
	}
}

// REQ-IAM-005: the catalogue the API shows is the one the code enforces.
func TestREQ_IAM_005_RoleCatalogue(t *testing.T) {
	cat := RoleCatalogue()
	var names []string
	for _, r := range cat {
		names = append(names, r.Name)
		if !slices.IsSorted(r.Permissions) || len(r.Permissions) == 0 {
			t.Fatalf("%s: permissions must be sorted and not empty: %v", r.Name, r.Permissions)
		}
	}
	if !slices.Equal(names, []string{"accountant", "attendance_clerk", "owner", "receptionist"}) {
		t.Fatalf("roles: %v", names)
	}
	owner := cat[slices.IndexFunc(cat, func(r RoleInfo) bool { return r.Name == "owner" })]
	for _, p := range []string{PermUserView, PermUserManage, PermRoleManage} {
		if !slices.Contains(owner.Permissions, p) {
			t.Fatalf("the owner must hold %s", p)
		}
	}
	for _, r := range cat {
		if r.Name == "owner" {
			continue
		}
		for _, p := range []string{PermUserView, PermUserManage, PermRoleManage} {
			if slices.Contains(r.Permissions, p) {
				t.Fatalf("%s must not hold %s: staff management is for owners until an Admin role exists", r.Name, p)
			}
		}
	}
}
