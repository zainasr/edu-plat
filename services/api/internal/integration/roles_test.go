package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
)

// addStaff gives a tenant a second person with one built-in role and returns their session token.
// The role assignment is data, so it is inserted the way an admin screen will later do it.
func addStaff(t *testing.T, f *fixture, tenantID, slug, role string) string {
	t.Helper()
	ctx := context.Background()
	mobile := newMobile(t)
	var userID, membershipID string
	if err := f.admin.QueryRow(ctx, `INSERT INTO users (mobile, full_name) VALUES ($1, $2) RETURNING id`, mobile, "Staff "+role).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := f.admin.QueryRow(ctx, `INSERT INTO memberships (tenant_id, user_id) VALUES ($1, $2) RETURNING id`, tenantID, userID).Scan(&membershipID); err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if _, err := f.admin.Exec(ctx, `INSERT INTO role_assignments (tenant_id, membership_id, role) VALUES ($1, $2, $3)`, tenantID, membershipID, role); err != nil {
		t.Fatalf("assign role %s: %v", role, err)
	}
	return signIn(t, f, mobile, slug)
}

// REQ-IAM-005, REQ-IAM-002: the built-in roles hold exactly the permissions the spec gives them.
// Every endpoint is called with a body that would be refused (400/404) if the permission passed, so
// anything other than 403 means the role may call it. A role is "allowed" or "forbidden" per endpoint.
func TestREQ_IAM_005_BuiltInRolesHoldTheirPermissions(t *testing.T) {
	f := newFixture(t)
	slug, ownerMobile, tenantID := newTenant(t, f, "roles")
	tokens := map[string]string{
		"owner":        signIn(t, f, ownerMobile, slug),
		"accountant":   addStaff(t, f, tenantID, slug, "accountant"),
		"receptionist": addStaff(t, f, tenantID, slug, "receptionist"),
		"clerk":        addStaff(t, f, tenantID, slug, "attendance_clerk"),
	}
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)

	const id = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	line := `{"fee_type_id":"` + id + `","effective_from":"2026-04-01","amount":{"amount":1,"currency":"PKR"},"frequency":"monthly"}`
	endpoints := []struct {
		name, method, path, body string
		// who may call it
		owner, accountant, receptionist, clerk bool
	}{
		{"fee.settings: create fee type", http.MethodPost, "/api/v1/fee-types", `{"code":"lab_fee","name":"Lab","kind":"lab"}`, true, false, false, false},
		{"fee.view: list fee types", http.MethodGet, "/api/v1/fee-types", "", true, true, true, false},
		{"fee.plan.manage: set plan line", http.MethodPost, "/api/v1/fee-plans/" + id + "/lines", line, true, true, false, false},
		{"fee.view: get plan", http.MethodGet, "/api/v1/fee-plans/" + id, "", true, true, true, false},
		{"fee.generate: generate challans", http.MethodPost, "/api/v1/challans/generate", `{}`, true, true, false, false},
		{"fee.release: release challans", http.MethodPost, "/api/v1/challans/release", `{"challan_ids":[]}`, true, true, false, false},
		{"fee.generate: void a draft", http.MethodPost, "/api/v1/challans/" + id + "/void", `{"reason":"x"}`, true, true, false, false},
		{"fee.view: list challans", http.MethodGet, "/api/v1/challans", "", true, true, true, false},
		{"fee.view: get challan", http.MethodGet, "/api/v1/challans/" + id, "", true, true, true, false},
		{"ledger.view: list accounts", http.MethodGet, "/api/v1/accounts", "", true, true, false, false},
		{"student.view: list students", http.MethodGet, "/api/v1/students", "", true, false, true, true},
		{"student.create: enrol a student", http.MethodPost, "/api/v1/students", `{}`, true, false, false, false},
		{"org.manage: create class", http.MethodPost, "/api/v1/classes", `{}`, true, false, false, false},
		{"fee.collect: take a payment", http.MethodPost, "/api/v1/payments", `{}`, true, true, true, false},
		{"fee.view: list payments", http.MethodGet, "/api/v1/payments", "", true, true, true, false},
		{"fee.view: get payment", http.MethodGet, "/api/v1/payments/" + id, "", true, true, true, false},
		{"payment.void: void a payment", http.MethodPost, "/api/v1/payments/" + id + "/void", `{"reason":"x"}`, true, true, false, false},
		{"fee.refund: refund advance", http.MethodPost, "/api/v1/payments/" + id + "/refund", `{}`, true, true, false, false},
		{"fee.view: defaulters", http.MethodGet, "/api/v1/defaulters", "", true, true, true, false},
		{"fee.view: family dues", http.MethodGet, "/api/v1/families/" + id + "/dues", "", true, true, true, false},
		{"org.view: read the calendar", http.MethodGet, "/api/v1/calendar", "", true, false, true, true},
		{"calendar.manage: add a holiday", http.MethodPost, "/api/v1/calendar/days", `{}`, true, false, false, false},
		{"calendar.manage: set weekly off", http.MethodPut, "/api/v1/calendar/weekly-off", `{}`, true, false, false, false},
		{"calendar.manage: remove a day", http.MethodPost, "/api/v1/calendar/days/" + id + "/remove", "", true, false, false, false},
		{"attendance.view: settings", http.MethodGet, "/api/v1/attendance/settings", "", true, false, true, true},
		{"attendance.settings: change settings", http.MethodPut, "/api/v1/attendance/settings", `{}`, true, false, false, false},
		{"attendance.mark: open a register", http.MethodPost, "/api/v1/attendance/registers", `{}`, true, false, true, true},
		{"attendance.view: list registers", http.MethodGet, "/api/v1/attendance/registers", "", true, false, true, true},
		{"attendance.view: get a register", http.MethodGet, "/api/v1/attendance/registers/" + id, "", true, false, true, true},
		{"attendance.mark: mark records", http.MethodPut, "/api/v1/attendance/registers/" + id + "/records", `{}`, true, false, true, true},
		{"attendance.mark: submit", http.MethodPost, "/api/v1/attendance/registers/" + id + "/submit", "", true, false, true, true},
		{"attendance.edit_*: correct", http.MethodPost, "/api/v1/attendance/registers/" + id + "/corrections", `{}`, true, false, false, true},
		{"student.edit: issue a card", http.MethodPost, "/api/v1/students/" + id + "/cards", "", true, false, false, false},
		{"student.edit: list cards", http.MethodGet, "/api/v1/students/" + id + "/cards", "", true, false, false, false},
		{"student.edit: disable a card", http.MethodPost, "/api/v1/student-cards/" + id + "/disable", `{}`, true, false, false, false},
		{"attendance.scan: scan a card", http.MethodPost, "/api/v1/attendance/scan", `{}`, true, false, true, false},
		{"attendance.view: gate scans", http.MethodGet, "/api/v1/attendance/gate-scans", "", true, false, true, true},
		{"attendance.view: mismatches", http.MethodGet, "/api/v1/attendance/mismatches", "", true, false, true, true},
		{"attendance.view: missing registers", http.MethodGet, "/api/v1/attendance/missing-registers", "", true, false, true, true},
		{"attendance.view: summary", http.MethodGet, "/api/v1/attendance/summary", "", true, false, true, true},
		{"attendance.view: absent list", http.MethodGet, "/api/v1/attendance/absent", "", true, false, true, true},
		{"attendance.view: low attendance", http.MethodGet, "/api/v1/attendance/low", "", true, false, true, true},
		{"attendance.view: student attendance", http.MethodGet, "/api/v1/students/" + id + "/attendance", "", true, false, true, true},
		{"user.view: list roles", http.MethodGet, "/api/v1/roles", "", true, false, false, false},
		{"user.view: list staff", http.MethodGet, "/api/v1/staff", "", true, false, false, false},
		{"user.view: get staff", http.MethodGet, "/api/v1/staff/" + id, "", true, false, false, false},
		{"user.manage: add staff", http.MethodPost, "/api/v1/staff", `{}`, true, false, false, false},
		{"role.manage: give a role", http.MethodPost, "/api/v1/staff/" + id + "/roles", `{"role":"accountant"}`, true, false, false, false},
		{"role.manage: take a role", http.MethodPost, "/api/v1/staff/" + id + "/roles/accountant/revoke", "", true, false, false, false},
		{"user.manage: deactivate", http.MethodPost, "/api/v1/staff/" + id + "/deactivate", "", true, false, false, false},
		{"user.manage: reactivate", http.MethodPost, "/api/v1/staff/" + id + "/reactivate", "", true, false, false, false},
	}
	for _, e := range endpoints {
		for role, want := range map[string]bool{"owner": e.owner, "accountant": e.accountant, "receptionist": e.receptionist, "clerk": e.clerk} {
			t.Run(role+" "+e.name, func(t *testing.T) {
				rec := do(t, h, e.method, e.path, tokens[role], e.body)
				forbidden := rec.Code == http.StatusForbidden
				if rec.Code == http.StatusUnauthorized {
					t.Fatalf("the session was refused: %s", rec.Body)
				}
				if want && forbidden {
					t.Fatalf("%s must be allowed to call this, got 403: %s", role, rec.Body)
				}
				if !want && !forbidden {
					t.Fatalf("%s must NOT be allowed to call this, got %d: %s", role, rec.Code, rec.Body)
				}
			})
		}
	}
}
