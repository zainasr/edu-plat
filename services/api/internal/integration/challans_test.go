package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/fees"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

var karachi = time.FixedZone("PKT", 5*60*60)

// thisMonth is the first day of the current month in Pakistan, the month new students are enrolled in.
func thisMonth() time.Time {
	n := time.Now().In(karachi)
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (w feeWorld) generate(t *testing.T, f *fixture, month time.Time, dry bool) fees.GenerateResult {
	t.Helper()
	res, err := f.fees.Generate(context.Background(), w.p, fees.GenerateInput{Month: month, DryRun: dry})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return res
}

func itemFor(t *testing.T, res fees.GenerateResult, studentID string) fees.GenerateItem {
	t.Helper()
	for _, it := range res.Items {
		if it.StudentID == studentID {
			return it
		}
	}
	t.Fatalf("student %s is not in the result: %+v", studentID, res.Items)
	return fees.GenerateItem{}
}

// addStudent enrols another student in the world's school, in the world's class and section.
func (w feeWorld) addStudent(t *testing.T, f *fixture, name string) string {
	t.Helper()
	var sectionID string
	if err := f.admin.QueryRow(context.Background(),
		`SELECT section_id FROM enrolments WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, w.studentID).Scan(&sectionID); err != nil {
		t.Fatalf("find section: %v", err)
	}
	in := enrolInput(t, sectionID)
	in.FullName = name
	s, err := f.students.Create(context.Background(), w.p, in)
	if err != nil {
		t.Fatalf("enrol %s: %v", name, err)
	}
	return s.ID
}

// REQ-FEE-003, REQ-FEE-004, US-FEE-001: Generate Fee drafts what is due, previews without writing,
// and is idempotent. One-time fees fall in the admission month only.
func TestREQ_FEE_003_GenerateIsAPreviewThenIdempotent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-gen")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	w.setLine(t, f, "admission", 100000, "one_time", "2026-01-01")
	m := thisMonth()

	// Preview: "will_generate", nothing written.
	prev := w.generate(t, f, m, true)
	if it := itemFor(t, prev, w.studentID); it.Outcome != "will_generate" || it.Total != 600000 || !it.NewAdmission || it.ChallanID != "" {
		t.Fatalf("preview: %+v", it)
	}
	if prev.Generated != 0 {
		t.Fatalf("a dry run must write nothing, generated %d", prev.Generated)
	}
	page, err := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("after a dry run there must be no challans: %v %+v", err, page.Items)
	}

	// Real run: one draft with both lines, due on the 10th.
	res := w.generate(t, f, m, false)
	it := itemFor(t, res, w.studentID)
	if it.Outcome != "generated" || res.Generated != 1 || it.ChallanID == "" || it.Total != 600000 {
		t.Fatalf("generate: %+v (generated %d)", it, res.Generated)
	}
	if res.DueDate.Day() != 10 {
		t.Fatalf("default due date should be the 10th, got %v", res.DueDate)
	}
	c, err := f.fees.GetChallan(ctx, w.p, it.ChallanID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "draft" || c.Total != 600000 || len(c.Lines) != 2 || c.ChallanNo != nil || c.ConsumerNo != nil || c.StudentID != w.studentID {
		t.Fatalf("draft: %+v", c)
	}

	// Run again: skipped, "has_challan", and still exactly one challan.
	again := w.generate(t, f, m, false)
	if got := itemFor(t, again, w.studentID); got.Outcome != "has_challan" || again.Generated != 0 {
		t.Fatalf("second run: %+v", got)
	}
	if page, _ := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{}); len(page.Items) != 1 {
		t.Fatalf("want one challan, got %d", len(page.Items))
	}

	// Next month: only the monthly line.
	next := w.generate(t, f, m.AddDate(0, 1, 0), false)
	if got := itemFor(t, next, w.studentID); got.Total != 500000 || got.NewAdmission {
		t.Fatalf("next month: %+v", got)
	}
}

// BR-FEE-001: a plan change after the draft was made does not change the draft's lines.
func TestBR_FEE_001_DraftLinesAreFrozenAtGeneration(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-frozen")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	m := thisMonth()
	id := itemFor(t, w.generate(t, f, m, false), w.studentID).ChallanID

	w.setLine(t, f, "tuition", 900000, "monthly", m.Format("2006-01-02"))
	c, err := f.fees.GetChallan(ctx, w.p, id)
	if err != nil || c.Total != 500000 || c.Lines[0].Amount != 500000 {
		t.Fatalf("a later plan change altered a draft: %v %+v", err, c)
	}
}

// Overrides and concessions flow into the challan; a full concession leaves nothing to bill.
func TestREQ_FEE_003_OverridesAndConcessions(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "chal-ovr")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	w.setLine(t, f, "transport", 200000, "monthly", "2026-01-01")
	other := w.addStudent(t, f, "Sara Khan")
	m := thisMonth()
	set := func(student, code string, amount int64) {
		t.Helper()
		if _, err := f.fees.SetOverride(context.Background(), w.p, student, fees.OverrideInput{
			FeeTypeID: w.types[code], EffectiveFrom: m, Reason: "agreed with the principal", Amount: pkr(amount),
		}); err != nil {
			t.Fatal(err)
		}
	}
	set(w.studentID, "tuition", 400000) // a discount
	set(other, "tuition", 0)            // a full concession on tuition
	set(other, "transport", 0)          // and on transport: nothing left to bill

	res := w.generate(t, f, m, false)
	if got := itemFor(t, res, w.studentID); got.Total != 600000 {
		t.Fatalf("discounted student: %+v", got)
	}
	if got := itemFor(t, res, other); got.Outcome != "nothing_to_bill" || got.ChallanID != "" {
		t.Fatalf("fully conceded student: %+v", got)
	}
}

// REQ-FEE-002: annual lines fall in the month of the year the plan line first took effect, ad hoc never.
func TestREQ_FEE_002_AnnualAndAdHocLines(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "chal-annual")
	m := thisMonth()
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	w.setLine(t, f, "annual", 250000, "annual", m.Format("2006-01-02")) // anchored to this month of the year
	w.setLine(t, f, "registration", 80000, "ad_hoc", "2026-01-01")

	this := itemFor(t, w.generate(t, f, m, true), w.studentID)
	if this.Total != 750000 {
		t.Fatalf("anchor month: want tuition + annual = 750000, got %+v", this)
	}
	nextYear := itemFor(t, w.generate(t, f, m.AddDate(1, 0, 0), true), w.studentID)
	if nextYear.Total != 750000 {
		t.Fatalf("the annual fee falls again a year later: %+v", nextYear)
	}
	other := itemFor(t, w.generate(t, f, m.AddDate(0, 3, 0), true), w.studentID)
	if other.Total != 500000 {
		t.Fatalf("three months on: only tuition: %+v", other)
	}
}

// BR-FEE-006: a student who has left (no active enrolment) gets no new challan. A student whose enrolment starts after
// the month ends is not billed for it either.
func TestBR_FEE_006_WithdrawnAndFutureStudentsAreNotBilled(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "chal-bill")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	gone := w.addStudent(t, f, "Left School")
	m := thisMonth()
	if _, err := f.admin.Exec(context.Background(), `UPDATE enrolments SET status = 'closed', ended_on = current_date, reason = 'withdrawn' WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, gone); err != nil {
		t.Fatal(err)
	}
	res := w.generate(t, f, m, false)
	for _, it := range res.Items {
		if it.StudentID == gone {
			t.Fatalf("a withdrawn student was billed: %+v", it)
		}
	}
	// Last month: the student enrolled this month, so there is nothing for last month.
	for _, it := range w.generate(t, f, m.AddDate(0, -1, 0), true).Items {
		if it.StudentID == w.studentID {
			t.Fatalf("billed for a month before the enrolment began: %+v", it)
		}
	}
}

// REQ-FEE-004, BR-FEE-008: release numbers the challans without gaps and posts the receivable.
func TestREQ_FEE_004_ReleasePostsTheReceivableAndNumbersChallans(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-rel")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	second := w.addStudent(t, f, "Sara Khan")
	m := thisMonth()
	res := w.generate(t, f, m, false)
	ids := []string{itemFor(t, res, w.studentID).ChallanID, itemFor(t, res, second).ChallanID}

	released, err := f.fees.Release(ctx, w.p, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 2 {
		t.Fatalf("want 2 released, got %d", len(released))
	}
	seen := map[int64]bool{}
	for _, c := range released {
		if c.Status != "released" || c.ChallanNo == nil || c.ConsumerNo == nil || len(*c.ConsumerNo) != 14 || c.ReleasedAt == nil {
			t.Fatalf("released challan: %+v", c)
		}
		seen[*c.ChallanNo] = true
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("challan numbers must be 1 and 2 with no gap, got %v", seen)
	}

	// The ledger holds one balanced entry per challan: debit Fee Receivable, credit Fee Income.
	page, err := f.ledger.ListEntries(ctx, w.p, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	posted := 0
	for _, e := range page.Items {
		if e.SourceType != "challan.released" {
			continue
		}
		posted++
		if len(e.Lines) != 2 || e.Lines[0].AccountCode != "1200" || e.Lines[0].Debit != 500000 ||
			e.Lines[1].AccountCode != "4000" || e.Lines[1].Credit != 500000 {
			t.Fatalf("receivable entry: %+v", e)
		}
	}
	if posted != 2 {
		t.Fatalf("want 2 receivable entries, got %d", posted)
	}
	tb, err := f.ledger.TrialBalance(ctx, w.p, time.Now().In(karachi))
	if err != nil || !tb.Balanced {
		t.Fatalf("trial balance: %v %+v", err, tb)
	}
	for _, l := range tb.Lines {
		if l.Code == "1200" && l.Debit != 1000000 {
			t.Fatalf("Fee Receivable should be 1,000,000, got %+v", l)
		}
	}

	// One audit row per release.
	var n int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'challan.released'`, w.tenantID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("want 2 release audit rows, got %d (%v)", n, err)
	}
}

// BR-FEE-001: a released challan can be neither released again, voided, nor edited, by any role.
func TestBR_FEE_001_ReleasedChallansNeverChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-immut")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	id := itemFor(t, w.generate(t, f, thisMonth(), false), w.studentID).ChallanID
	if _, err := f.fees.Release(ctx, w.p, []string{id}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.fees.Release(ctx, w.p, []string{id}); errCode(t, err) != "challans.not_draft" {
		t.Fatalf("second release: %v", err)
	}
	if _, err := f.fees.Void(ctx, w.p, id, "mistake"); errCode(t, err) != "challans.not_draft" {
		t.Fatalf("void a released challan: %v", err)
	}
	// Even the table owner cannot change a released challan or its lines (database trigger).
	for name, stmt := range map[string]string{
		"change the total":  `UPDATE challans SET total = 1 WHERE id = $1`,
		"change the status": `UPDATE challans SET status = 'void', void_reason = 'x', voided_at = now() WHERE id = $1`,
		"delete":            `DELETE FROM challans WHERE id = $1`,
		"change a line":     `UPDATE challan_lines SET amount = 1 WHERE challan_id = $1`,
		"delete a line":     `DELETE FROM challan_lines WHERE challan_id = $1`,
	} {
		if _, err := f.admin.Exec(ctx, stmt, id); err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}
	// The API role may not delete, and may not touch the amount columns at all (grants, not only the trigger).
	for name, stmt := range map[string]string{
		"app role changes the total": `UPDATE challans SET total = 1 WHERE id = $1`,
		"app role deletes":           `DELETE FROM challans WHERE id = $1`,
		"app role changes a line":    `UPDATE challan_lines SET amount = 1 WHERE challan_id = $1`,
	} {
		err := db.WithTenant(ctx, f.app, w.tenantID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, stmt, id)
			return err
		})
		if err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}
}

// Release is all or nothing: one bad id leaves every other challan a draft and burns no number.
func TestREQ_FEE_004_ReleaseIsAllOrNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-aon")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	second := w.addStudent(t, f, "Sara Khan")
	res := w.generate(t, f, thisMonth(), false)
	good := itemFor(t, res, w.studentID).ChallanID
	other := itemFor(t, res, second).ChallanID
	if _, err := f.fees.Release(ctx, w.p, []string{other}); err != nil { // other is released, number 1
		t.Fatal(err)
	}

	if _, err := f.fees.Release(ctx, w.p, []string{good, other}); errCode(t, err) != "challans.not_draft" {
		t.Fatalf("a batch with a released challan: %v", err)
	}
	if c, _ := f.fees.GetChallan(ctx, w.p, good); c.Status != "draft" {
		t.Fatalf("the failed batch released a challan: %+v", c)
	}
	if _, err := f.fees.Release(ctx, w.p, []string{"5c9c6c26-e0e4-4938-af5c-f6fc35688270"}); errCode(t, err) != "challans.not_found" {
		t.Fatalf("unknown id: %v", err)
	}
	// The next real release takes number 2: nothing was burnt by the failures.
	out, err := f.fees.Release(ctx, w.p, []string{good})
	if err != nil || *out[0].ChallanNo != 2 {
		t.Fatalf("want challan number 2, got %v %+v", err, out)
	}
	// Request validation.
	if _, err := f.fees.Release(ctx, w.p, nil); errCode(t, err) != "fees.release_invalid" {
		t.Fatalf("empty batch: %v", err)
	}
	if _, err := f.fees.Release(ctx, w.p, []string{good, good}); errCode(t, err) != "fees.release_invalid" {
		t.Fatalf("duplicate ids: %v", err)
	}
}

// Two people releasing the same drafts at once: each challan is released and posted exactly once.
func TestREQ_FEE_004_ConcurrentReleaseHappensOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-race")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	id := itemFor(t, w.generate(t, f, thisMonth(), false), w.studentID).ChallanID

	const racers = 6
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.fees.Release(ctx, w.p, []string{id})
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if errCode(t, err) != "challans.not_draft" {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("exactly one release must win, %d did", ok)
	}
	var entries int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1 AND source_type = 'challan.released'`, w.tenantID).Scan(&entries); err != nil || entries != 1 {
		t.Fatalf("want one receivable entry, got %d (%v)", entries, err)
	}
}

// Concurrent Generate for the same month: one draft per student, never two.
func TestREQ_FEE_003_ConcurrentGenerateMakesOneDraftPerStudent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-grace")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	m := thisMonth()
	var wg sync.WaitGroup
	results := make([]fees.GenerateResult, 6)
	errs := make([]error, 6)
	for i := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = f.fees.Generate(ctx, w.p, fees.GenerateInput{Month: m})
		}()
	}
	wg.Wait()
	generated := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("a concurrent generate failed: %v", errs[i])
		}
		generated += results[i].Generated
	}
	if generated != 1 {
		t.Fatalf("exactly one run may draft the challan, %d did", generated)
	}
	var n int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM challans WHERE tenant_id = $1 AND student_id = $2 AND month = $3 AND status <> 'void'`, w.tenantID, w.studentID, m).Scan(&n); err != nil || n != 1 {
		t.Fatalf("want exactly one live challan, got %d (%v)", n, err)
	}
}

// REQ-FEE-004: voiding a draft frees the student to be generated again; the void is audited.
func TestREQ_FEE_004_VoidADraftThenRegenerate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-void")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	m := thisMonth()
	id := itemFor(t, w.generate(t, f, m, false), w.studentID).ChallanID

	if _, err := f.fees.Void(ctx, w.p, id, "  "); errCode(t, err) != "fees.void_invalid" {
		t.Fatalf("void without a reason: %v", err)
	}
	c, err := f.fees.Void(ctx, w.p, id, "wrong class plan")
	if err != nil || c.Status != "void" || c.VoidReason == nil || *c.VoidReason != "wrong class plan" {
		t.Fatalf("void: %v %+v", err, c)
	}
	if _, err := f.fees.Release(ctx, w.p, []string{id}); errCode(t, err) != "challans.not_draft" {
		t.Fatalf("release a voided challan: %v", err)
	}
	again := itemFor(t, w.generate(t, f, m, false), w.studentID)
	if again.Outcome != "generated" || again.ChallanID == id {
		t.Fatalf("regenerate after a void: %+v", again)
	}
	var n int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'challan.voided'`, w.tenantID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("want one void audit row, got %d (%v)", n, err)
	}
	// A draft that was never released holds no number and posted nothing.
	var entries int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1`, w.tenantID).Scan(&entries)
	if entries != 0 {
		t.Fatalf("drafts and voids must not post to the ledger, found %d entries", entries)
	}
}

// Filters and cursor pagination.
func TestREQ_FEE_004_ListFiltersAndPaging(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-list")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	second := w.addStudent(t, f, "Sara Khan")
	third := w.addStudent(t, f, "Omar Khan")
	m := thisMonth()
	res := w.generate(t, f, m, false)
	if _, err := f.fees.Release(ctx, w.p, []string{itemFor(t, res, second).ChallanID}); err != nil {
		t.Fatal(err)
	}
	_ = third

	var all []string
	cursor := ""
	for {
		page, err := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range page.Items {
			all = append(all, c.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(all) != 3 {
		t.Fatalf("paging through 3 challans in pages of 2 saw %d", len(all))
	}
	if page, _ := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{Status: "released"}); len(page.Items) != 1 {
		t.Fatalf("released filter: %d", len(page.Items))
	}
	if page, _ := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{Status: "draft"}); len(page.Items) != 2 {
		t.Fatalf("draft filter: %d", len(page.Items))
	}
	if page, _ := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{StudentID: w.studentID}); len(page.Items) != 1 {
		t.Fatalf("student filter: %d", len(page.Items))
	}
	if page, _ := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{Month: m.AddDate(0, 1, 0)}); len(page.Items) != 0 {
		t.Fatalf("month filter: %d", len(page.Items))
	}
	if _, err := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{Status: "bogus"}); errCode(t, err) != "fees.status_invalid" {
		t.Fatalf("bad status: %v", err)
	}
	if _, err := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{Cursor: "not-a-cursor"}); errCode(t, err) != "challans.cursor_invalid" {
		t.Fatalf("bad cursor: %v", err)
	}
}

// ADR-0002: tenant B cannot see, release or void tenant A's challans, and its generate run ignores A's students.
func TestRLS_ChallansAreIsolatedBetweenTenants(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newFeeWorld(t, f, "chal-a")
	b := newFeeWorld(t, f, "chal-b")
	a.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	b.setLine(t, f, "tuition", 300000, "monthly", "2026-01-01")
	m := thisMonth()
	aID := itemFor(t, a.generate(t, f, m, false), a.studentID).ChallanID

	res := b.generate(t, f, m, false)
	if len(res.Items) != 1 || res.Items[0].StudentID != b.studentID || res.Items[0].Total != 300000 {
		t.Fatalf("B's run must only cover B's student: %+v", res.Items)
	}
	if _, err := f.fees.GetChallan(ctx, b.p, aID); errCode(t, err) != "challans.not_found" {
		t.Fatalf("B read A's challan: %v", err)
	}
	if _, err := f.fees.Release(ctx, b.p, []string{aID}); errCode(t, err) != "challans.not_found" {
		t.Fatalf("B released A's challan: %v", err)
	}
	if _, err := f.fees.Void(ctx, b.p, aID, "attack"); errCode(t, err) != "challans.not_found" {
		t.Fatalf("B voided A's challan: %v", err)
	}
	if page, _ := f.fees.ListChallans(ctx, b.p, fees.ChallanFilter{}); len(page.Items) != 1 || page.Items[0].StudentID != b.studentID {
		t.Fatalf("B's list leaked: %+v", page.Items)
	}
	if c, _ := f.fees.GetChallan(ctx, a.p, aID); c.Status != "draft" {
		t.Fatalf("A's challan was touched: %+v", c)
	}
}

// Generate input refusals.
func TestREQ_FEE_003_GenerateRefusals(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "chal-bad")
	ctx := context.Background()
	if _, err := f.fees.Generate(ctx, w.p, fees.GenerateInput{}); errCode(t, err) != "fees.generate_invalid" {
		t.Fatalf("no month: %v", err)
	}
	early := thisMonth().AddDate(0, 0, -1)
	if _, err := f.fees.Generate(ctx, w.p, fees.GenerateInput{Month: thisMonth(), DueDate: &early}); errCode(t, err) != "fees.generate_invalid" {
		t.Fatalf("due before the month: %v", err)
	}
}

// Overlapping multi-challan batches must finish, not deadlock: A releases {0,1,2}, B releases {2,3,4}.
// Whoever loses the shared challan gets challans.not_draft, never a database deadlock (a 500). The
// race is a matter of timing, so it runs many rounds, each with fresh drafts (one month per round).
func TestREQ_FEE_004_OverlappingReleaseBatchesDoNotDeadlock(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-dead")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	for _, n := range []string{"Two", "Three", "Four", "Five"} {
		w.addStudent(t, f, n+" Khan")
	}
	for round := range 30 {
		res := w.generate(t, f, thisMonth().AddDate(0, round, 0), false)
		ids := make([]string, 0, len(res.Items))
		for _, it := range res.Items {
			ids = append(ids, it.ChallanID)
		}
		if len(ids) != 5 {
			t.Fatalf("round %d: want 5 drafts, got %d", round, len(ids))
		}
		batches := [][]string{ids[:3], ids[2:]} // they share ids[2]
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, batch := range batches {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = f.fees.Release(ctx, w.p, batch)
			}()
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil && errCode(t, err) != "challans.not_draft" {
				t.Fatalf("round %d: overlapping release: %v", round, err)
			}
		}
	}
	var released, entries int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM challans WHERE tenant_id = $1 AND status = 'released'`, w.tenantID).Scan(&released)
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1 AND source_type = 'challan.released'`, w.tenantID).Scan(&entries)
	if released != entries {
		t.Fatalf("each released challan must have exactly one receivable entry: %d released, %d entries", released, entries)
	}
}

// REQ-FEE-003: the preview shows "has_challan" for a student who already has one, and writes nothing.
func TestREQ_FEE_003_PreviewShowsHasChallan(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "chal-prev")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	m := thisMonth()
	w.generate(t, f, m, false)
	prev := w.generate(t, f, m, true)
	if got := itemFor(t, prev, w.studentID); got.Outcome != "has_challan" || prev.Generated != 0 {
		t.Fatalf("preview after generate: %+v", got)
	}
}

// REQ-FEE-003: the due date is configurable and is stored on the draft.
func TestREQ_FEE_003_CustomDueDate(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "chal-due")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	due := thisMonth().AddDate(0, 0, 19) // the 20th
	res, err := f.fees.Generate(context.Background(), w.p, fees.GenerateInput{Month: thisMonth(), DueDate: &due})
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.fees.GetChallan(context.Background(), w.p, itemFor(t, res, w.studentID).ChallanID)
	if err != nil || !c.DueDate.Equal(due) {
		t.Fatalf("due date %v, want %v (%v)", c.DueDate, due, err)
	}
}

// BR-FEE-006: a student who leaves keeps the challans they already have; later months are not billed.
func TestBR_FEE_006_ExistingChallanSurvivesWithdrawal(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "chal-leave")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	m := thisMonth()
	id := itemFor(t, w.generate(t, f, m, false), w.studentID).ChallanID
	if _, err := f.fees.Release(ctx, w.p, []string{id}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `UPDATE enrolments SET status = 'closed', ended_on = current_date, reason = 'withdrawn' WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, w.studentID); err != nil {
		t.Fatal(err)
	}
	if c, err := f.fees.GetChallan(ctx, w.p, id); err != nil || c.Status != "released" || c.Total != 500000 {
		t.Fatalf("a withdrawn student's challan must stay readable and released: %v %+v", err, c)
	}
	for _, it := range w.generate(t, f, m.AddDate(0, 1, 0), true).Items {
		if it.StudentID == w.studentID {
			t.Fatalf("billed after withdrawal: %+v", it)
		}
	}
}

// REQ-FEE-002: a one-time fee is charged once, in the admission month, and not again.
func TestREQ_FEE_002_OneTimeOnlyInTheAdmissionMonth(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "chal-once")
	w.setLine(t, f, "tuition", 500000, "monthly", "2026-01-01")
	w.setLine(t, f, "admission", 100000, "one_time", "2026-01-01")
	m := thisMonth()
	if got := itemFor(t, w.generate(t, f, m, true), w.studentID); got.Total != 600000 {
		t.Fatalf("admission month: %+v", got)
	}
	for i := 1; i <= 14; i++ {
		if got := itemFor(t, w.generate(t, f, m.AddDate(0, i, 0), true), w.studentID); got.Total != 500000 || got.NewAdmission {
			t.Fatalf("month +%d must not carry the one-time fee: %+v", i, got)
		}
	}
}
