package integration

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// staffWorld is a school with its owner signed in.
type staffWorld struct {
	slug, mobile, tenantID string
	owner                  iam.Principal
	token                  string
}

func newStaffWorld(t *testing.T, f *fixture, label string) staffWorld {
	t.Helper()
	slug, mobile, tenantID := newTenant(t, f, label)
	token := signIn(t, f, mobile, slug)
	p, err := f.iam.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return staffWorld{slug: slug, mobile: mobile, tenantID: tenantID, owner: p, token: token}
}

func (w staffWorld) add(t *testing.T, f *fixture, name string, roles ...string) (iam.StaffMember, string) {
	t.Helper()
	mobile := newMobile(t)
	m, err := f.iam.CreateStaff(context.Background(), w.owner, iam.StaffInput{Mobile: mobile, FullName: name, Roles: roles})
	if err != nil {
		t.Fatalf("create staff %s: %v", name, err)
	}
	return m, mobile
}

// principal signs a staff member in through the real OTP flow.
func (w staffWorld) principal(t *testing.T, f *fixture, mobile string) (iam.Principal, string) {
	t.Helper()
	token := signIn(t, f, mobile, w.slug)
	p, err := f.iam.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("authenticate staff: %v", err)
	}
	return p, token
}

// US-IAM-001, REQ-IAM-002, REQ-IAM-006: add a person with several roles, change them, keep the history.
func TestUS_IAM_001_AddStaffAndChangeTheirRoles(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-add")

	m, mobile := w.add(t, f, "Ayesha Noor", "receptionist", "accountant")
	if m.Status != "active" || m.FullName != "Ayesha Noor" || !slices.Equal(m.Roles, []string{"accountant", "receptionist"}) {
		t.Fatalf("created: %+v", m)
	}
	// She signs in and holds the union: she can release challans (accountant) and see students (receptionist).
	p, _ := w.principal(t, f, mobile)
	if !p.Can(iam.PermFeeRelease) || !p.Can(iam.PermStudentView) || p.Can(iam.PermUserManage) {
		t.Fatalf("union of roles wrong: release=%v students=%v users=%v", p.Can(iam.PermFeeRelease), p.Can(iam.PermStudentView), p.Can(iam.PermUserManage))
	}

	// Revoke one: her permissions shrink on her next request, and the history is kept.
	after, err := f.iam.RevokeRole(ctx, w.owner, m.ID, "accountant")
	if err != nil || !slices.Equal(after.Roles, []string{"receptionist"}) {
		t.Fatalf("revoke: %v %+v", err, after)
	}
	p2, _ := w.principal(t, f, mobile)
	if p2.Can(iam.PermFeeRelease) || !p2.Can(iam.PermStudentView) {
		t.Fatal("a revoked role must stop granting permissions")
	}
	var live, revoked int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE revoked_at IS NULL), count(*) FILTER (WHERE revoked_at IS NOT NULL) FROM role_assignments WHERE membership_id = $1`, m.ID).Scan(&live, &revoked)
	if live != 1 || revoked != 1 {
		t.Fatalf("history must be kept: %d live, %d revoked", live, revoked)
	}

	// She can be given the role again (a revoked assignment does not block a new one).
	if again, err := f.iam.AssignRole(ctx, w.owner, m.ID, "accountant"); err != nil || len(again.Roles) != 2 {
		t.Fatalf("assign again: %v %+v", err, again)
	}
	// Refusals.
	if _, err := f.iam.AssignRole(ctx, w.owner, m.ID, "accountant"); errCode(t, err) != "staff.role_already_assigned" {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := f.iam.RevokeRole(ctx, w.owner, m.ID, "owner"); errCode(t, err) != "staff.role_not_assigned" {
		t.Fatalf("revoke a role not held: %v", err)
	}
	if _, err := f.iam.AssignRole(ctx, w.owner, m.ID, "principal"); errCode(t, err) != "staff.invalid" {
		t.Fatalf("unknown role: %v", err)
	}

	// Every change is audited, in order.
	rows, err := f.admin.Query(ctx, `SELECT action FROM audit_log WHERE tenant_id = $1 AND entity_id = $2 ORDER BY id`, w.tenantID, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		actions = append(actions, a)
	}
	rows.Close()
	if want := []string{"staff.created", "role.revoked", "role.assigned"}; !slices.Equal(actions, want) {
		t.Fatalf("audit: %v, want %v", actions, want)
	}
}

// REQ-IAM-001: a number that already belongs to a person (in another school) is the same person, and
// the response gives nothing away about the other school: not even the name it holds.
func TestREQ_IAM_001_ANumberFromAnotherSchoolRevealsNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newStaffWorld(t, f, "staff-a")
	b := newStaffWorld(t, f, "staff-b")
	inA, mobile := a.add(t, f, "Real Name In School A", "accountant")

	inB, err := f.iam.CreateStaff(ctx, b.owner, iam.StaffInput{Mobile: mobile, FullName: "Bee's Name For Them", Roles: []string{"receptionist"}})
	if err != nil {
		t.Fatal(err)
	}
	if inB.FullName != "Bee's Name For Them" || strings.Contains(inB.FullName, "School A") {
		t.Fatalf("school B must see only its own name: %q", inB.FullName)
	}
	// Same person, but nothing in the response lets school B tell: the API shows no global user id.
	var people, memberships int
	_ = f.admin.QueryRow(ctx, `SELECT count(*), (SELECT count(*) FROM memberships m WHERE m.user_id = u.id) FROM users u WHERE u.mobile = $1 GROUP BY u.id`, mobile).Scan(&people, &memberships)
	if people != 1 || memberships != 2 {
		t.Fatalf("one person with two memberships: %d people, %d memberships", people, memberships)
	}
	var stored string
	_ = f.admin.QueryRow(ctx, `SELECT full_name FROM users WHERE mobile = $1`, mobile).Scan(&stored)
	if stored != "Real Name In School A" {
		t.Fatalf("adding the number in school B must not change the person's stored name: %q", stored)
	}
	// School A's name for them is unchanged, and each school sees only its own staff.
	if got, _ := f.iam.GetStaff(ctx, a.owner, inA.ID); got.FullName != "Real Name In School A" {
		t.Fatalf("A's name changed: %q", got.FullName)
	}
	if _, err := f.iam.GetStaff(ctx, b.owner, inA.ID); errCode(t, err) != "staff.not_found" {
		t.Fatalf("B read A's staff member: %v", err)
	}
	// The person has the roles of each school only in that school.
	pa, _ := a.principal(t, f, mobile)
	pb, _ := b.principal(t, f, mobile)
	if !pa.Can(iam.PermFeeRelease) || pb.Can(iam.PermFeeRelease) || !pb.Can(iam.PermStudentView) || pa.Can(iam.PermStudentView) {
		t.Fatal("roles must not cross schools")
	}
	// Adding the same number again to the same school is a 409.
	if _, err := f.iam.CreateStaff(ctx, b.owner, iam.StaffInput{Mobile: mobile, FullName: "Again", Roles: []string{"accountant"}}); errCode(t, err) != "staff.already_member" {
		t.Fatalf("duplicate member: %v", err)
	}
}

// US-IAM-005: deactivation ends access on the very next request, revokes tokens, keeps history, and
// reactivation does not bring old tokens back.
func TestUS_IAM_005_DeactivationEndsAccessAtOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-off")
	m, mobile := w.add(t, f, "Leaving Person", "accountant")
	_, session := w.principal(t, f, mobile)
	pat, err := f.iam.CreatePAT(ctx, mustPrincipal(t, f, session), "terminal", 30)
	if err != nil {
		t.Fatal(err)
	}
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees), f.iam)
	if rec := do(t, h, http.MethodGet, "/api/v1/fee-types", session, ""); rec.Code != http.StatusOK {
		t.Fatalf("before: %d %s", rec.Code, rec.Body)
	}

	off, err := f.iam.DeactivateStaff(ctx, w.owner, m.ID)
	if err != nil || off.Status != "deactivated" || !slices.Equal(off.Roles, []string{"accountant"}) {
		t.Fatalf("deactivate: %v %+v", err, off)
	}
	for name, tok := range map[string]string{"session": session, "pat": pat.Plaintext} {
		if rec := do(t, h, http.MethodGet, "/api/v1/fee-types", tok, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s must fail on the very next request: %d %s", name, rec.Code, rec.Body)
		}
	}
	var open int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE membership_id = $1 AND revoked_at IS NULL`, m.ID).Scan(&open)
	if open != 0 {
		t.Fatalf("%d tokens are still live", open)
	}
	// They cannot sign in again while deactivated, and cannot be given roles.
	if _, err := f.iam.VerifyOTP(ctx, mobile, "000000", w.slug); err == nil {
		t.Fatal("a deactivated person must not sign in")
	}
	if _, err := f.iam.AssignRole(ctx, w.owner, m.ID, "receptionist"); errCode(t, err) != "staff.deactivated" {
		t.Fatalf("assign to a deactivated person: %v", err)
	}
	if _, err := f.iam.DeactivateStaff(ctx, w.owner, m.ID); errCode(t, err) != "staff.already_deactivated" {
		t.Fatalf("twice: %v", err)
	}
	if page, _ := f.iam.ListStaff(ctx, w.owner, "deactivated"); len(page) != 1 || page[0].ID != m.ID {
		t.Fatalf("deactivated filter: %+v", page)
	}

	// Reactivate: roles are as they were, and they sign in afresh; the old tokens stay dead.
	on, err := f.iam.ReactivateStaff(ctx, w.owner, m.ID)
	if err != nil || on.Status != "active" || !slices.Equal(on.Roles, []string{"accountant"}) {
		t.Fatalf("reactivate: %v %+v", err, on)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/fee-types", session, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the old session must stay revoked: %d", rec.Code)
	}
	_, fresh := w.principal(t, f, mobile)
	if rec := do(t, h, http.MethodGet, "/api/v1/fee-types", fresh, ""); rec.Code != http.StatusOK {
		t.Fatalf("a fresh sign-in works: %d %s", rec.Code, rec.Body)
	}
	if _, err := f.iam.ReactivateStaff(ctx, w.owner, m.ID); errCode(t, err) != "staff.already_active" {
		t.Fatalf("reactivate twice: %v", err)
	}
}

func mustPrincipal(t *testing.T, f *fixture, token string) iam.Principal {
	t.Helper()
	p, err := f.iam.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// You cannot switch yourself off; the last owner cannot be demoted or deactivated.
func TestUS_IAM_005_TheLastOwnerAndSelfProtection(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-last")
	ownerMember, err := f.iam.GetStaff(ctx, w.owner, w.owner.MembershipID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.iam.DeactivateStaff(ctx, w.owner, ownerMember.ID); errCode(t, err) != "staff.self_deactivate" {
		t.Fatalf("self: %v", err)
	}
	if _, err := f.iam.RevokeRole(ctx, w.owner, ownerMember.ID, "owner"); errCode(t, err) != "staff.last_owner" {
		t.Fatalf("the only owner demoting themselves: %v", err)
	}

	// With a second owner the first can step down (keeping an accountant role so they stay active: taking
	// a person's only role would switch them off), and then the second is the last one.
	if _, err := f.iam.AssignRole(ctx, w.owner, ownerMember.ID, "accountant"); err != nil {
		t.Fatal(err)
	}
	second, mobile := w.add(t, f, "Second Owner", "owner")
	p2, _ := w.principal(t, f, mobile)
	if _, err := f.iam.RevokeRole(ctx, w.owner, ownerMember.ID, "owner"); err != nil {
		t.Fatalf("stepping down with a second owner: %v", err)
	}
	if _, err := f.iam.RevokeRole(ctx, p2, second.ID, "owner"); errCode(t, err) != "staff.last_owner" {
		t.Fatalf("the new last owner: %v", err)
	}
	// The first person is no longer an owner, so they cannot deactivate the last one either way.
	stale := mustPrincipal(t, f, signIn(t, f, w.mobile, w.slug))
	if stale.Can(iam.PermUserManage) {
		t.Fatal("a former owner holds no staff permissions")
	}
	if _, err := f.iam.DeactivateStaff(ctx, p2, ownerMember.ID); err != nil {
		t.Fatalf("deactivating the former owner is fine: %v", err)
	}
}

// Two owners removing each other at the same moment: exactly one wins and an owner always remains.
func TestUS_IAM_005_OwnersCannotRemoveEachOtherAtOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for round := range 8 {
		w := newStaffWorld(t, f, "staff-race")
		first, err := f.iam.GetStaff(ctx, w.owner, w.owner.MembershipID)
		if err != nil {
			t.Fatal(err)
		}
		second, mobile := w.add(t, f, "Other Owner", "owner")
		p2, _ := w.principal(t, f, mobile)

		// Round shapes: both revoke each other's owner role, or one revokes while the other deactivates.
		type action func() error
		var a, b action
		if round%2 == 0 {
			a = func() error { _, err := f.iam.RevokeRole(ctx, w.owner, second.ID, "owner"); return err }
			b = func() error { _, err := f.iam.RevokeRole(ctx, p2, first.ID, "owner"); return err }
		} else {
			a = func() error { _, err := f.iam.RevokeRole(ctx, w.owner, second.ID, "owner"); return err }
			b = func() error { _, err := f.iam.DeactivateStaff(ctx, p2, first.ID); return err }
		}
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, fn := range []action{a, b} {
			wg.Add(1)
			go func() { defer wg.Done(); errs[i] = fn() }()
		}
		wg.Wait()
		wins := 0
		for _, err := range errs {
			if err == nil {
				wins++
			} else if c := errCode(t, err); c != "staff.last_owner" && c != "auth.token_invalid" {
				// the loser either hits the last-owner rule, or was switched off by the winner first
				t.Fatalf("round %d: unexpected error %s", round, c)
			}
		}
		var owners int
		_ = f.admin.QueryRow(ctx, `
			SELECT count(*) FROM role_assignments ra JOIN memberships m ON m.id = ra.membership_id
			WHERE ra.tenant_id = $1 AND ra.role = 'owner' AND ra.revoked_at IS NULL AND m.status = 'active'`, w.tenantID).Scan(&owners)
		if owners != 1 || wins != 1 {
			t.Fatalf("round %d: %d active owners, %d winners; want exactly 1 and 1", round, owners, wins)
		}
	}
}

// ADR-0002: school B cannot see or change school A's staff.
func TestRLS_StaffAreIsolatedBetweenSchools(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newStaffWorld(t, f, "staff-iso-a")
	b := newStaffWorld(t, f, "staff-iso-b")
	m, _ := a.add(t, f, "A's Accountant", "accountant")

	if _, err := f.iam.AssignRole(ctx, b.owner, m.ID, "receptionist"); errCode(t, err) != "staff.not_found" {
		t.Fatalf("assign: %v", err)
	}
	if _, err := f.iam.RevokeRole(ctx, b.owner, m.ID, "accountant"); errCode(t, err) != "staff.not_found" {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := f.iam.DeactivateStaff(ctx, b.owner, m.ID); errCode(t, err) != "staff.not_found" {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := f.iam.ReactivateStaff(ctx, b.owner, m.ID); errCode(t, err) != "staff.not_found" {
		t.Fatalf("reactivate: %v", err)
	}
	if list, _ := f.iam.ListStaff(ctx, b.owner, ""); len(list) != 1 {
		t.Fatalf("B sees %d staff, want only its own owner", len(list))
	}
	if got, _ := f.iam.GetStaff(ctx, a.owner, m.ID); got.Status != "active" || len(got.Roles) != 1 {
		t.Fatalf("A's staff member was touched: %+v", got)
	}
}

// The database refuses what the service would never do: changing a revoked assignment, changing the
// role of a live one, or granting the API role more than it needs.
func TestREQ_IAM_006_RoleAssignmentHistoryIsProtected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-hist")
	m, _ := w.add(t, f, "Hist", "accountant")
	if _, err := f.iam.RevokeRole(ctx, w.owner, m.ID, "accountant"); err != nil {
		t.Fatal(err)
	}
	for name, stmt := range map[string]string{
		"un-revoke":            `UPDATE role_assignments SET revoked_at = NULL WHERE membership_id = $1`,
		"change a revoked row": `UPDATE role_assignments SET revoked_by = NULL WHERE membership_id = $1`,
	} {
		if _, err := f.admin.Exec(ctx, stmt, m.ID); err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}
	live, _ := f.iam.AssignRole(ctx, w.owner, m.ID, "receptionist")
	if _, err := f.admin.Exec(ctx, `UPDATE role_assignments SET role = 'owner' WHERE membership_id = $1 AND revoked_at IS NULL`, live.ID); err == nil {
		t.Fatal("changing the role of a live assignment must be refused (revoke and assign instead)")
	}
}

// HTTP: error shapes, the idempotent create, and the role list.
func TestREQ_API_StaffOverHTTP(t *testing.T) {
	f := newFixture(t)
	w := newStaffWorld(t, f, "staff-http")
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees), f.iam, api.WithIdempotency(f.app))

	if rec := do(t, h, http.MethodGet, "/api/v1/roles", w.token, ""); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"accountant"`) || !strings.Contains(rec.Body.String(), `"fee.release"`) {
		t.Fatalf("roles: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/staff", w.token, `{"mobile":"12345","full_name":"","roles":[]}`); rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), `"staff.invalid"`) || !strings.Contains(rec.Body.String(), `"mobile"`) {
		t.Fatalf("bad create: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/staff", w.token, `{"mobile":"03001234567","full_name":"X","roles":["principal"]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown role is refused by the contract or the rule: %d %s", rec.Code, rec.Body)
	}

	mobile := newMobile(t)
	body := `{"mobile":"` + mobile + `","full_name":"Retry Person","roles":["receptionist"]}`
	first := keyed(t, h, "/api/v1/staff", w.token, "staff-create-key-001", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", first.Code, first.Body)
	}
	second := keyed(t, h, "/api/v1/staff", w.token, "staff-create-key-001", body)
	if second.Code != http.StatusCreated || second.Body.String() != first.Body.String() || second.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("retry must replay: %d %s", second.Code, second.Body)
	}
	if list, _ := f.iam.ListStaff(context.Background(), w.owner, ""); len(list) != 2 {
		t.Fatalf("one person was added once: %d staff", len(list))
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/staff?status=bogus", w.token, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status filter: %d", rec.Code)
	}
}

// Defence in depth: even if a token were somehow still live, a deactivated membership is refused on
// every request (the check in Authenticate, not only the token revocation).
func TestUS_IAM_005_ADeactivatedMembershipIsRefusedEvenWithALiveToken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-defence")
	m, mobile := w.add(t, f, "Still Has A Token", "accountant")
	_, token := w.principal(t, f, mobile)
	if _, err := f.admin.Exec(ctx, `UPDATE memberships SET status = 'deactivated' WHERE id = $1`, m.ID); err != nil {
		t.Fatal(err)
	}
	var live int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE membership_id = $1 AND revoked_at IS NULL`, m.ID).Scan(&live)
	if live == 0 {
		t.Fatal("this test needs a live token")
	}
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees), f.iam)
	if rec := do(t, h, http.MethodGet, "/api/v1/fee-types", token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a live token of a deactivated membership: %d %s", rec.Code, rec.Body)
	}
}

// The API role cannot write the shared users table; it can only call ensure_user, and PUBLIC cannot.
func TestREQ_IAM_001_OnlyTheNarrowFunctionAddsPeople(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-users")
	for name, stmt := range map[string]string{
		"insert a user": `INSERT INTO users (mobile, full_name) VALUES ('+923009990001', 'Sneaky')`,
		"rename a user": `UPDATE users SET full_name = 'Renamed'`,
		"delete a user": `DELETE FROM users`,
	} {
		err := db.WithTenant(ctx, f.app, w.tenantID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, stmt)
			return err
		})
		if err == nil {
			t.Fatalf("the API role must not %s", name)
		}
	}
	var public bool
	if err := f.admin.QueryRow(ctx, `SELECT has_function_privilege('public', 'ensure_user(text, text)', 'EXECUTE')`).Scan(&public); err != nil || public {
		t.Fatalf("PUBLIC must not execute ensure_user (%v, %v)", public, err)
	}
}

// A caller whose roles changed after they signed in cannot use the old roles: the owner rule is judged
// on live roles, taken after the owner lock.
func TestUS_IAM_001_AStalePrincipalCannotMakeOwners(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-stale")
	boss, bossMobile := w.add(t, f, "Second Owner", "owner", "accountant")
	bossP, _ := w.principal(t, f, bossMobile) // signed in while an owner
	target, _ := w.add(t, f, "Target", "receptionist")

	// The first owner demotes the second; the second's old principal is now stale.
	if _, err := f.iam.RevokeRole(ctx, w.owner, boss.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.iam.AssignRole(ctx, bossP, target.ID, "owner"); errCode(t, err) != "staff.owner_only" {
		t.Fatalf("a demoted owner's old session must not make an owner: %v", err)
	}
	if _, err := f.iam.CreateStaff(ctx, bossP, iam.StaffInput{Mobile: newMobile(t), FullName: "New Owner", Roles: []string{"owner"}}); errCode(t, err) != "staff.owner_only" {
		t.Fatalf("nor create one: %v", err)
	}
	// Switching an owner off or on is the owner's job too, judged on live roles.
	third, _ := w.add(t, f, "Third Owner", "owner")
	if _, err := f.iam.DeactivateStaff(ctx, bossP, third.ID); errCode(t, err) != "staff.owner_only" {
		t.Fatalf("a demoted owner must not deactivate an owner: %v", err)
	}
	if _, err := f.iam.DeactivateStaff(ctx, w.owner, third.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.iam.ReactivateStaff(ctx, bossP, third.ID); errCode(t, err) != "staff.owner_only" {
		t.Fatalf("nor reactivate one: %v", err)
	}
	// Switched off a moment ago, roles untouched: still an owner on paper, but refused outright.
	other, otherMobile := w.add(t, f, "Other Owner", "owner")
	otherP, _ := w.principal(t, f, otherMobile)
	if _, err := f.iam.DeactivateStaff(ctx, w.owner, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.iam.AssignRole(ctx, otherP, target.ID, "owner"); errCode(t, err) != "auth.token_invalid" {
		t.Fatalf("a deactivated caller must be refused: %v", err)
	}
	if got, _ := f.iam.GetStaff(ctx, w.owner, target.ID); slices.Contains(got.Roles, "owner") {
		t.Fatal("the target must not have become an owner")
	}
}

// REQ-IAM-006: taking a person's last role switches them off at once, history kept.
func TestREQ_IAM_006_RevokingTheLastRoleSwitchesThemOff(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-lastrole")
	m, mobile := w.add(t, f, "One Role", "accountant")
	_, token := w.principal(t, f, mobile)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees), f.iam)

	out, err := f.iam.RevokeRole(ctx, w.owner, m.ID, "accountant")
	if err != nil || out.Status != "deactivated" || len(out.Roles) != 0 {
		t.Fatalf("last role: %v %+v", err, out)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/fee-types", token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("their session must end at once: %d", rec.Code)
	}
	var revoked, total int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE revoked_at IS NOT NULL), count(*) FROM role_assignments WHERE membership_id = $1`, m.ID).Scan(&revoked, &total)
	if revoked != 1 || total != 1 {
		t.Fatalf("history kept: %d revoked of %d", revoked, total)
	}
	// With two roles, revoking one leaves them active.
	m2, _ := w.add(t, f, "Two Roles", "accountant", "receptionist")
	if out, err := f.iam.RevokeRole(ctx, w.owner, m2.ID, "accountant"); err != nil || out.Status != "active" {
		t.Fatalf("one of two: %v %+v", err, out)
	}
	// You cannot switch yourself off by revoking your own last role (an owner who is also the last owner
	// is refused as the last owner first, so use a second owner who then holds only that role).
	second, mobile2 := w.add(t, f, "Second", "owner")
	p2, _ := w.principal(t, f, mobile2)
	if _, err := f.iam.RevokeRole(ctx, p2, second.ID, "owner"); errCode(t, err) != "staff.self_deactivate" {
		t.Fatalf("revoking your own last role: %v", err)
	}
}

// Adding someone who is already on the staff (even deactivated) is a 409 and changes nothing.
func TestUS_IAM_005_AddingADeactivatedMemberAgainChangesNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newStaffWorld(t, f, "staff-readd")
	m, mobile := w.add(t, f, "Gone And Back", "accountant")
	if _, err := f.iam.DeactivateStaff(ctx, w.owner, m.ID); err != nil {
		t.Fatal(err)
	}
	_, err := f.iam.CreateStaff(ctx, w.owner, iam.StaffInput{Mobile: mobile, FullName: "Different Name", Roles: []string{"owner"}})
	if errCode(t, err) != "staff.already_member" {
		t.Fatalf("got %v", err)
	}
	got, _ := f.iam.GetStaff(ctx, w.owner, m.ID)
	if got.Status != "deactivated" || got.FullName != "Gone And Back" || !slices.Equal(got.Roles, []string{"accountant"}) {
		t.Fatalf("nothing may change: %+v", got)
	}
}
