package integration

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/fees"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// feeWorld is a school with a class, a student in it, and the fee type ids by code.
type feeWorld struct {
	slug      string
	mobile    string
	p         iam.Principal
	tenantID  string
	classID   string
	studentID string
	types     map[string]string
}

func newFeeWorld(t *testing.T, f *fixture, label string) feeWorld {
	t.Helper()
	slug, mobile, tenantID := newTenant(t, f, label)
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)
	student, err := f.students.Create(context.Background(), p, enrolInput(t, st.SectionID))
	if err != nil {
		t.Fatalf("enrol student: %v", err)
	}
	list, err := f.fees.ListTypes(context.Background(), p)
	if err != nil {
		t.Fatalf("list fee types: %v", err)
	}
	types := map[string]string{}
	for _, ft := range list {
		types[ft.Code] = ft.ID
	}
	return feeWorld{slug: slug, mobile: mobile, p: p, tenantID: tenantID, classID: st.ClassID, studentID: student.ID, types: types}
}

// signInAs signs in as the world's owner and returns a session token for HTTP calls.
func signInAs(t *testing.T, f *fixture, w feeWorld) string {
	t.Helper()
	return signIn(t, f, w.mobile, w.slug)
}

func pkr(v int64) *money.Money { m := money.Money(v); return &m }

// setLine sets a plan line and fails the test on error.
func (w feeWorld) setLine(t *testing.T, f *fixture, code string, amount int64, freq, from string) fees.PlanLine {
	t.Helper()
	out, err := f.fees.SetPlanLine(context.Background(), w.p, w.classID, fees.PlanLineInput{
		FeeTypeID: w.types[code], EffectiveFrom: mustDate(t, from), Amount: pkr(amount), Frequency: freq,
	})
	if err != nil {
		t.Fatalf("set plan line %s: %v", code, err)
	}
	return out
}

func lineFor(lines []fees.Line, id string) (fees.Line, bool) {
	for _, l := range lines {
		if l.FeeTypeID == id {
			return l, true
		}
	}
	return fees.Line{}, false
}

// Defaults are suggestions, never locks: seeded once, an archived one stays archived, and racing
// first calls still produce exactly one set.
func TestREQ_FEE_SuggestedFeeTypesAreSeededOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "fee-seed")
	p := principalFor(t, f, slug, mobile)

	const callers = 6
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			_, err := f.fees.ListTypes(ctx, p)
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("list: %v", err)
		}
	}
	var n int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM fee_types WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil || n != 6 {
		t.Fatalf("want exactly 6 suggested types after %d racing calls, got %d (%v)", callers, n, err)
	}

	list, _ := f.fees.ListTypes(ctx, p)
	var transport string
	for _, ft := range list {
		if ft.Code == "transport" {
			transport = ft.ID
		}
	}
	if _, err := f.fees.ArchiveType(ctx, p, transport); err != nil {
		t.Fatal(err)
	}
	list, err := f.fees.ListTypes(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 6 {
		t.Fatalf("an archived suggestion must not be recreated or removed; got %d types", len(list))
	}
	for _, ft := range list {
		if ft.Code == "transport" && ft.Status != "archived" {
			t.Fatal("transport should stay archived")
		}
	}
}

func TestREQ_FEE_FeeTypeCreation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-type")

	lab, err := f.fees.CreateType(ctx, w.p, fees.FeeTypeInput{Code: "lab_fee", Name: "Lab Fee", Kind: "lab"})
	if err != nil || lab.Status != "active" || lab.IsDeposit {
		t.Fatalf("create: %v %+v", err, lab)
	}
	dep, err := f.fees.CreateType(ctx, w.p, fees.FeeTypeInput{Code: "hostel_deposit", Name: "Hostel Deposit", Kind: "security_deposit"})
	if err != nil || !dep.IsDeposit {
		t.Fatalf("a security_deposit kind must be a deposit (BR-FEE-004): %v %+v", err, dep)
	}

	for _, code := range []string{"lab_fee", "tuition"} { // a duplicate, and a clash with a suggested type
		_, err := f.fees.CreateType(ctx, w.p, fees.FeeTypeInput{Code: code, Name: "Again", Kind: "custom"})
		if got := errCode(t, err); got != "fees.type_exists" {
			t.Fatalf("code %s: %s, want fees.type_exists", code, got)
		}
	}
	if _, err := f.fees.CreateType(ctx, w.p, fees.FeeTypeInput{Code: "Bad Code", Name: "x", Kind: "custom"}); errCode(t, err) != "fees.type_invalid" {
		t.Fatalf("invalid input: %v", err)
	}
	if _, err := f.fees.ArchiveType(ctx, w.p, "5c9c6c26-e0e4-4938-af5c-f6fc35688270"); errCode(t, err) != "fees.type_not_found" {
		t.Fatalf("archive unknown: %v", err)
	}
}

// BR-FEE-004, backed by the database: only the security_deposit kind may be marked as a deposit.
func TestBR_FEE_004_DatabaseTiesTheDepositFlagToTheKind(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-dep")

	for _, c := range []struct {
		kind    string
		deposit bool
	}{
		{"tuition", true},           // a deposit flag on a non-deposit kind
		{"security_deposit", false}, // a deposit kind without the flag
	} {
		_, err := f.admin.Exec(ctx, `INSERT INTO fee_types (tenant_id, code, name, kind, is_deposit)
			VALUES ($1, 'raw_' || $2, 'raw', $2, $3)`, w.tenantID, c.kind, c.deposit)
		if pgCode(err) != "23514" {
			t.Fatalf("kind %s deposit=%v: want a check violation, got %v", c.kind, c.deposit, err)
		}
	}
	if !slices.Contains(keys(w.types), "security_deposit") {
		t.Fatal("the suggested set must include the security deposit type")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// REQ-FEE-001 / BR-FEE-001: a change is a new version from a month. Earlier months and the history
// keep what they had.
func TestBR_FEE_001_APlanChangeIsANewVersionAndHistoryIsExact(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-ver")

	w.setLine(t, f, "tuition", 500000, "monthly", "2026-04-17") // any day: stored as 2026-04-01
	w.setLine(t, f, "tuition", 600000, "monthly", "2026-09-01")

	for _, c := range []struct {
		month string
		want  money.Money
	}{
		{"2026-03-01", 0},
		{"2026-04-30", 500000},
		{"2026-08-15", 500000},
		{"2026-09-01", 600000},
		{"2027-02-01", 600000},
	} {
		plan, err := f.fees.GetPlan(ctx, w.p, w.classID, mustDate(t, c.month))
		if err != nil {
			t.Fatal(err)
		}
		line, found := fees.PlanLine{}, false
		for _, l := range plan.Lines {
			if l.FeeTypeID == w.types["tuition"] {
				line, found = l, true
			}
		}
		if c.want == 0 && found {
			t.Fatalf("%s: tuition should not be on the plan yet", c.month)
		}
		if c.want != 0 && (!found || line.Amount != c.want) {
			t.Fatalf("%s: got %+v, want amount %d", c.month, line, c.want)
		}
	}

	history, err := f.fees.PlanHistory(ctx, w.p, w.classID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Amount != 600000 || history[1].Amount != 500000 ||
		!history[1].EffectiveFrom.Equal(mustDate(t, "2026-04-01")) {
		t.Fatalf("history should hold both versions, newest first, April stored as the 1st: %+v", history)
	}

	_, err = f.fees.SetPlanLine(ctx, w.p, w.classID, fees.PlanLineInput{
		FeeTypeID: w.types["tuition"], EffectiveFrom: mustDate(t, "2026-09-20"), Amount: pkr(1), Frequency: "monthly",
	})
	if got := errCode(t, err); got != "fees.plan_line_exists" {
		t.Fatalf("same month twice: %s, want fees.plan_line_exists", got)
	}
}

// BR-FEE-001, backed by the database: versions cannot be edited or deleted, by anyone.
func TestBR_FEE_001_PlanAndOverrideVersionsCannotBeChanged(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-immut")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-04-01")
	if _, err := f.fees.SetOverride(ctx, w.p, w.studentID, fees.OverrideInput{
		FeeTypeID: w.types["tuition"], EffectiveFrom: mustDate(t, "2026-04-01"), Reason: "concession", Amount: pkr(400000),
	}); err != nil {
		t.Fatal(err)
	}

	for _, sql := range []string{
		`UPDATE fee_plan_lines SET amount = 1 WHERE tenant_id = $1`,
		`DELETE FROM fee_plan_lines WHERE tenant_id = $1`,
		`UPDATE student_fee_overrides SET amount = 1 WHERE tenant_id = $1`,
		`DELETE FROM student_fee_overrides WHERE tenant_id = $1`,
	} {
		t.Run("API role: "+sql, func(t *testing.T) {
			err := db.WithTenant(ctx, f.app, w.tenantID, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, sql, w.tenantID)
				return err
			})
			if pgCode(err) != "42501" {
				t.Fatalf("want permission denied (42501), got %v", err)
			}
		})
		t.Run("owner: "+sql, func(t *testing.T) {
			if _, err := f.admin.Exec(ctx, sql, w.tenantID); err == nil {
				t.Fatal("the owner changed a version; the append-only trigger must stop it")
			}
		})
	}
	// The API role may archive a fee type (status only), and may not rename or delete one.
	err := db.WithTenant(ctx, f.app, w.tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE fee_types SET name = 'Hacked' WHERE tenant_id = $1`, w.tenantID)
		return err
	})
	if pgCode(err) != "42501" {
		t.Fatalf("renaming a fee type should be refused by column privileges, got %v", err)
	}
}

func TestREQ_FEE_001_RemovingAFeeTypeFromAPlan(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-rm")
	w.setLine(t, f, "transport", 200000, "monthly", "2026-04-01")

	remove := func(from string) error {
		_, err := f.fees.SetPlanLine(ctx, w.p, w.classID, fees.PlanLineInput{
			FeeTypeID: w.types["transport"], EffectiveFrom: mustDate(t, from), Remove: true,
		})
		return err
	}
	if err := remove("2026-07-01"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := remove("2026-09-01"); errCode(t, err) != "fees.nothing_to_remove" {
		t.Fatalf("removing what is already gone: %v", err)
	}
	has := func(month string) bool {
		plan, err := f.fees.GetPlan(ctx, w.p, w.classID, mustDate(t, month))
		if err != nil {
			t.Fatal(err)
		}
		return len(plan.Lines) == 1
	}
	if !has("2026-06-01") || has("2026-07-01") || has("2026-12-01") {
		t.Fatal("transport should be on the plan until June and off from July")
	}
	w.setLine(t, f, "transport", 250000, "monthly", "2026-10-01") // a later version reinstates it
	if !has("2026-10-01") {
		t.Fatal("a later version should reinstate the fee type")
	}
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM fee_plan_lines WHERE tenant_id = $1 AND removed`, w.tenantID).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
}

func TestREQ_FEE_PlanRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-refuse")
	apr := mustDate(t, "2026-04-01")

	t.Run("an archived type cannot join a plan, and drops out of an existing one", func(t *testing.T) {
		w.setLine(t, f, "annual", 100000, "annual", "2026-04-01")
		if _, err := f.fees.ArchiveType(ctx, w.p, w.types["annual"]); err != nil {
			t.Fatal(err)
		}
		_, err := f.fees.SetPlanLine(ctx, w.p, w.classID, fees.PlanLineInput{
			FeeTypeID: w.types["annual"], EffectiveFrom: mustDate(t, "2026-09-01"), Amount: pkr(1), Frequency: "annual",
		})
		if got := errCode(t, err); got != "fees.type_archived" {
			t.Fatalf("code = %s, want fees.type_archived", got)
		}
		plan, err := f.fees.GetPlan(ctx, w.p, w.classID, mustDate(t, "2026-10-01"))
		if err != nil || len(plan.Lines) != 0 {
			t.Fatalf("an archived type must drop out of the plan: %v %+v", err, plan.Lines)
		}
	})
	t.Run("unknown fee type", func(t *testing.T) {
		_, err := f.fees.SetPlanLine(ctx, w.p, w.classID, fees.PlanLineInput{
			FeeTypeID: "5c9c6c26-e0e4-4938-af5c-f6fc35688270", EffectiveFrom: apr, Amount: pkr(1), Frequency: "monthly",
		})
		if got := errCode(t, err); got != "fees.type_not_found" {
			t.Fatalf("code = %s", got)
		}
	})
	t.Run("unknown class", func(t *testing.T) {
		_, err := f.fees.SetPlanLine(ctx, w.p, "5c9c6c26-e0e4-4938-af5c-f6fc35688270", fees.PlanLineInput{
			FeeTypeID: w.types["tuition"], EffectiveFrom: apr, Amount: pkr(1), Frequency: "monthly",
		})
		if got := errCode(t, err); got != "org.class_not_found" {
			t.Fatalf("code = %s", got)
		}
	})
	t.Run("an archived class", func(t *testing.T) {
		if _, err := f.org.ArchiveClass(ctx, w.p, w.classID); err != nil {
			t.Fatal(err)
		}
		_, err := f.fees.SetPlanLine(ctx, w.p, w.classID, fees.PlanLineInput{
			FeeTypeID: w.types["tuition"], EffectiveFrom: apr, Amount: pkr(1), Frequency: "monthly",
		})
		if got := errCode(t, err); got != "fees.class_archived" {
			t.Fatalf("code = %s", got)
		}
	})
	t.Run("validation lists every problem", func(t *testing.T) {
		_, err := f.fees.SetPlanLine(ctx, w.p, w.classID, fees.PlanLineInput{FeeTypeID: "nope", Frequency: "weekly"})
		if got := errCode(t, err); got != "fees.plan_line_invalid" {
			t.Fatalf("code = %s", got)
		}
	})
}

// REQ-FEE-001: a student override replaces the plan amount from a month; zero is a full concession;
// a cleared version returns to the plan.
func TestREQ_FEE_001_StudentOverridesAndTheFeeStructure(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-ovr")
	tuition := w.types["tuition"]
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-04-01")
	w.setLine(t, f, "transport", 200000, "monthly", "2026-04-01")

	structure := func(month string) []fees.Line {
		s, err := f.fees.FeeStructure(ctx, w.p, w.studentID, mustDate(t, month))
		if err != nil {
			t.Fatalf("structure %s: %v", month, err)
		}
		if s.ClassID != w.classID {
			t.Fatalf("the structure should come from the student's class %s, got %s", w.classID, s.ClassID)
		}
		return s.Lines
	}
	set := func(from string, amount *money.Money, clear bool, reason string) error {
		_, err := f.fees.SetOverride(ctx, w.p, w.studentID, fees.OverrideInput{
			FeeTypeID: tuition, EffectiveFrom: mustDate(t, from), Reason: reason, Amount: amount, Clear: clear,
		})
		return err
	}

	// Before any override: the plan, two lines, ordered by name.
	if lines := structure("2026-05-01"); len(lines) != 2 || lines[0].Name != "Transport" || lines[1].Name != "Tuition" {
		t.Fatalf("plan lines: %+v", lines)
	}

	if err := set("2026-09-10", pkr(400000), false, "sibling concession"); err != nil {
		t.Fatal(err)
	}
	if l, _ := lineFor(structure("2026-08-01"), tuition); l.Amount != 500000 || l.Source != "plan" {
		t.Fatalf("August is before the override: %+v", l)
	}
	if l, _ := lineFor(structure("2026-09-01"), tuition); l.Amount != 400000 || l.Source != "override" || l.PlanAmount != 500000 {
		t.Fatalf("September should use the override and still show the plan amount: %+v", l)
	}

	// A plan change later does not erase the override: the student keeps their concession amount.
	w.setLine(t, f, "tuition", 650000, "monthly", "2026-10-01")
	if l, _ := lineFor(structure("2026-10-01"), tuition); l.Amount != 400000 || l.PlanAmount != 650000 {
		t.Fatalf("October: override %d vs plan %d, want 400000 vs 650000", l.Amount, l.PlanAmount)
	}

	// Zero is a full concession; clearing returns to the plan.
	if err := set("2026-11-01", pkr(0), false, "scholarship"); err != nil {
		t.Fatal(err)
	}
	if l, _ := lineFor(structure("2026-11-01"), tuition); l.Amount != 0 || l.Source != "override" {
		t.Fatalf("a zero override is a full concession: %+v", l)
	}
	if err := set("2026-12-01", nil, true, "back to normal"); err != nil {
		t.Fatal(err)
	}
	if l, _ := lineFor(structure("2026-12-01"), tuition); l.Amount != 650000 || l.Source != "plan" {
		t.Fatalf("a cleared override returns to the plan: %+v", l)
	}

	list, err := f.fees.ListOverrides(ctx, w.p, w.studentID)
	if err != nil || len(list) != 3 || list[0].Cleared != true || list[2].Reason != "sibling concession" {
		t.Fatalf("overrides, newest first: %v %+v", err, list)
	}

	if err := set("2026-09-25", pkr(1), false, "same month again"); errCode(t, err) != "fees.override_exists" {
		t.Fatalf("same month twice: %v", err)
	}
}

func TestREQ_FEE_001_OverrideRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-ovr-no")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-04-01")

	in := func(code, from string) fees.OverrideInput {
		return fees.OverrideInput{FeeTypeID: w.types[code], EffectiveFrom: mustDate(t, from), Reason: "because", Amount: pkr(1)}
	}
	if _, err := f.fees.SetOverride(ctx, w.p, w.studentID, in("transport", "2026-05-01")); errCode(t, err) != "fees.override_needs_plan_line" {
		t.Fatalf("a type that is not on the plan: %v", err)
	}
	if _, err := f.fees.SetOverride(ctx, w.p, w.studentID, in("tuition", "2026-02-01")); errCode(t, err) != "fees.override_needs_plan_line" {
		t.Fatalf("a month before the plan line exists: %v", err)
	}
	if _, err := f.fees.SetOverride(ctx, w.p, "5c9c6c26-e0e4-4938-af5c-f6fc35688270", in("tuition", "2026-05-01")); errCode(t, err) != "students.not_found" {
		t.Fatalf("unknown student: %v", err)
	}
	bad := in("tuition", "2026-05-01")
	bad.Reason = "no"
	if _, err := f.fees.SetOverride(ctx, w.p, w.studentID, bad); errCode(t, err) != "fees.override_invalid" {
		t.Fatalf("short reason: %v", err)
	}
	if _, err := f.fees.FeeStructure(ctx, w.p, "5c9c6c26-e0e4-4938-af5c-f6fc35688270", mustDate(t, "2026-05-01")); errCode(t, err) != "students.not_found" {
		t.Fatalf("structure for an unknown student: %v", err)
	}
}

// A student who is not enrolled has no class, so no fee structure.
func TestREQ_FEE_AStudentWithoutAnEnrolmentHasNoFeeStructure(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-noenrol")
	if _, err := f.admin.Exec(ctx, `DELETE FROM enrolments WHERE tenant_id = $1`, w.tenantID); err != nil {
		t.Fatal(err)
	}
	_, err := f.fees.FeeStructure(ctx, w.p, w.studentID, mustDate(t, "2026-05-01"))
	if got := errCode(t, err); got != "students.not_enrolled" {
		t.Fatalf("code = %s, want students.not_enrolled", got)
	}
	if list, err := f.fees.ListOverrides(ctx, w.p, w.studentID); err != nil || len(list) != 0 {
		t.Fatalf("overrides can still be listed for a student with no enrolment: %v %d", err, len(list))
	}
}

// Fee setup is audited: who changed which plan or override, with the amount and the month.
func TestREQ_FEE_001_PlanChangesAreAudited(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "fee-audit")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-04-01")
	if _, err := f.fees.SetOverride(ctx, w.p, w.studentID, fees.OverrideInput{
		FeeTypeID: w.types["tuition"], EffectiveFrom: mustDate(t, "2026-04-01"), Reason: "concession", Amount: pkr(400000),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.fees.CreateType(ctx, w.p, fees.FeeTypeInput{Code: "lab_fee", Name: "Lab Fee", Kind: "lab"}); err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"fee_plan.line_set", "fee_override.set", "fee_type.created"} {
		var n int
		if err := f.admin.QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2 AND token_id IS NOT NULL`,
			w.tenantID, action).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s: want 1 audit row with a token, got %d (%v)", action, n, err)
		}
	}
	var amount int64
	if err := f.admin.QueryRow(ctx,
		`SELECT (summary->>'amount')::bigint FROM audit_log WHERE tenant_id = $1 AND action = 'fee_plan.line_set'`,
		w.tenantID).Scan(&amount); err != nil || amount != 500000 {
		t.Fatalf("the audit row should record the amount in paisa: %v %d", err, amount)
	}
}

// Tenant isolation for fee setup.
func TestRLS_FeeSetupIsIsolatedBetweenTenants(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newFeeWorld(t, f, "fee-a")
	b := newFeeWorld(t, f, "fee-b")
	a.setLine(t, f, "tuition", 500000, "monthly", "2026-04-01")

	if _, err := f.fees.GetPlan(ctx, b.p, a.classID, mustDate(t, "2026-05-01")); errCode(t, err) != "org.class_not_found" {
		t.Fatalf("B read A's class plan: %v", err)
	}
	if _, err := f.fees.PlanHistory(ctx, b.p, a.classID); errCode(t, err) != "org.class_not_found" {
		t.Fatalf("B read A's plan history: %v", err)
	}
	if _, err := f.fees.FeeStructure(ctx, b.p, a.studentID, mustDate(t, "2026-05-01")); errCode(t, err) != "students.not_found" {
		t.Fatalf("B read A's student's fees: %v", err)
	}
	_, err := f.fees.SetOverride(ctx, b.p, a.studentID, fees.OverrideInput{
		FeeTypeID: b.types["tuition"], EffectiveFrom: mustDate(t, "2026-05-01"), Reason: "attack", Amount: pkr(1),
	})
	if errCode(t, err) != "students.not_found" {
		t.Fatalf("B set an override on A's student: %v", err)
	}
	_, err = f.fees.SetPlanLine(ctx, b.p, a.classID, fees.PlanLineInput{
		FeeTypeID: b.types["tuition"], EffectiveFrom: mustDate(t, "2026-05-01"), Amount: pkr(1), Frequency: "monthly",
	})
	if errCode(t, err) != "org.class_not_found" {
		t.Fatalf("B changed A's class plan: %v", err)
	}
	// A's fee type id means nothing to B.
	if _, err := f.fees.ArchiveType(ctx, b.p, a.types["tuition"]); errCode(t, err) != "fees.type_not_found" {
		t.Fatalf("B archived A's fee type: %v", err)
	}
	// Each school has its own six suggested types, with different ids.
	if a.types["tuition"] == b.types["tuition"] {
		t.Fatal("two schools must not share a fee type row")
	}
}
