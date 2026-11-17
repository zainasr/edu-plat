package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/fees"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

// payWorld is a school whose students have released challans, ready to be paid.
type payWorld struct {
	feeWorld
	familyID string
	month    time.Time
}

// newPayWorld bills the world's student (tuition 5,000) and releases the challan.
func newPayWorld(t *testing.T, f *fixture, label string) payWorld {
	t.Helper()
	w := newFeeWorld(t, f, label)
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	pw := payWorld{feeWorld: w, month: thisMonth()}
	pw.familyID = pw.releaseFor(t, f, pw.month, w.studentID)
	return pw
}

// releaseFor generates and releases the month for everyone and returns the family of the given student.
func (w payWorld) releaseFor(t *testing.T, f *fixture, month time.Time, studentID string) string {
	t.Helper()
	ctx := context.Background()
	res := w.generate(t, f, month, false)
	var ids []string
	for _, it := range res.Items {
		if it.ChallanID != "" {
			ids = append(ids, it.ChallanID)
		}
	}
	if len(ids) > 0 {
		if _, err := f.fees.Release(ctx, w.p, ids); err != nil {
			t.Fatalf("release: %v", err)
		}
	}
	page, err := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{StudentID: studentID})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("find the student's challan: %v", err)
	}
	return page.Items[0].FamilyID
}

// addSibling enrols another child under the same guardian, so they join the same family.
func (w payWorld) addSibling(t *testing.T, f *fixture, name string) string {
	t.Helper()
	ctx := context.Background()
	var mobile, section string
	if err := f.admin.QueryRow(ctx, `
		SELECT g.mobile FROM guardians g JOIN guardian_links l ON l.tenant_id = g.tenant_id AND l.guardian_id = g.id
		WHERE l.tenant_id = $1 AND l.student_id = $2 AND l.is_primary`, w.tenantID, w.studentID).Scan(&mobile); err != nil {
		t.Fatalf("find guardian: %v", err)
	}
	if err := f.admin.QueryRow(ctx, `SELECT section_id FROM enrolments WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, w.studentID).Scan(&section); err != nil {
		t.Fatalf("find section: %v", err)
	}
	in := enrolInput(t, section)
	in.FullName = name
	in.Guardians[0].Mobile = mobile
	s, err := f.students.Create(ctx, w.p, in)
	if err != nil {
		t.Fatalf("enrol sibling: %v", err)
	}
	return s.ID
}

func (w payWorld) pay(t *testing.T, f *fixture, amount int64, mods ...func(*fees.PaymentInput)) (fees.Payment, error) {
	in := fees.PaymentInput{FamilyID: w.familyID, Amount: money.Money(amount), Method: "cash", AccountCode: "1000"}
	for _, m := range mods {
		m(&in)
	}
	return f.fees.RecordPayment(context.Background(), w.p, in)
}

func (w payWorld) mustPay(t *testing.T, f *fixture, amount int64, mods ...func(*fees.PaymentInput)) fees.Payment {
	t.Helper()
	p, err := w.pay(t, f, amount, mods...)
	if err != nil {
		t.Fatalf("pay %d: %v", amount, err)
	}
	return p
}

func trialLine(t *testing.T, f *fixture, w feeWorld, code string) (debit, credit money.Money) {
	t.Helper()
	tb, err := f.ledger.TrialBalance(context.Background(), w.p, time.Now().In(karachi))
	if err != nil {
		t.Fatal(err)
	}
	if !tb.Balanced {
		t.Fatal("the trial balance must always balance")
	}
	for _, l := range tb.Lines {
		if l.Code == code {
			return l.Debit, l.Credit
		}
	}
	return 0, 0
}

// US-FEE-002, REQ-FEE-021, REQ-FEE-024: a counter payment is split, numbered, posted and reduces the dues.
func TestUS_FEE_002_CollectAtTheCounter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "pay-basic")

	pay := w.mustPay(t, f, 200000, func(in *fees.PaymentInput) { in.Reference = "slip 17" })
	wantNo := receipt(fiscalYearOf(time.Now().In(karachi)), 1)
	if pay.ReceiptNo != wantNo || pay.Status != "recorded" || pay.Amount != 200000 || pay.Method != "cash" ||
		pay.AccountCode != "1000" || pay.Reference == nil || *pay.Reference != "slip 17" || len(pay.Allocations) != 1 ||
		pay.Allocations[0].Amount != 200000 || pay.EntryID == "" {
		t.Fatalf("payment: %+v (want receipt %s)", pay, wantNo)
	}

	// The books: money in Cash, receivable down.
	if d, _ := trialLine(t, f, w.feeWorld, "1000"); d != 200000 {
		t.Fatalf("Cash in Hand debit = %d, want 200000", d)
	}
	if d, c := trialLine(t, f, w.feeWorld, "1200"); d-c != 300000 {
		t.Fatalf("Fee Receivable = %d, want 300000 still owed", d-c)
	}
	e, err := f.ledger.GetEntry(ctx, w.p, pay.EntryID)
	if err != nil || e.SourceType != "payment.recorded" || e.SourceID != pay.ID {
		t.Fatalf("journal entry: %v %+v", err, e)
	}

	// The challan is partly paid, and the family's dues show the rest.
	page, _ := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{StudentID: w.studentID})
	c := page.Items[0]
	if c.Paid != 200000 || c.PaymentStatus != "partial" || c.Total != 500000 {
		t.Fatalf("challan: %+v", c)
	}
	dues, err := f.fees.GetFamilyDues(ctx, w.p, w.familyID)
	if err != nil || dues.TotalDue != 300000 || len(dues.Items) != 1 || dues.Items[0].Balance != 300000 || dues.Items[0].Paid != 200000 {
		t.Fatalf("dues: %v %+v", err, dues)
	}

	// Pay the rest: the next receipt number, and the challan is paid.
	second := w.mustPay(t, f, 300000)
	if second.ReceiptNo != receipt(fiscalYearOf(time.Now().In(karachi)), 2) {
		t.Fatalf("second receipt: %s", second.ReceiptNo)
	}
	if got, _ := f.fees.GetChallan(ctx, w.p, c.ID); got.PaymentStatus != "paid" || got.Paid != 500000 {
		t.Fatalf("paid challan: %+v", got)
	}
	if dues, _ := f.fees.GetFamilyDues(ctx, w.p, w.familyID); dues.TotalDue != 0 || len(dues.Items) != 0 {
		t.Fatalf("nothing is owed now: %+v", dues)
	}
	if _, err := w.pay(t, f, 100); errCode(t, err) != "payments.no_dues" {
		t.Fatalf("paying a family that owes nothing: %v", err)
	}
	// One audit entry per payment.
	var n int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'payment.recorded'`, w.tenantID).Scan(&n)
	if n != 2 {
		t.Fatalf("want 2 audit rows, got %d", n)
	}
}

func receipt(fy, n int) string { return fmt.Sprintf("RCP-%d-%06d", fy, n) }

func fiscalYearOf(t time.Time) int {
	if t.Month() >= time.July {
		return t.Year()
	}
	return t.Year() - 1
}

// US-FEE-003, REQ-FEE-022: one payment for three children is split oldest first, with one receipt.
func TestUS_FEE_003_FamilyPaymentIsSplitOldestFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "pay-family")
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	pw := payWorld{feeWorld: w, month: thisMonth()}
	second := pw.addSibling(t, f, "Second Child")
	third := pw.addSibling(t, f, "Third Child")
	pw.familyID = pw.releaseFor(t, f, pw.month, w.studentID)

	dues, err := f.fees.GetFamilyDues(ctx, w.p, pw.familyID)
	if err != nil || len(dues.Items) != 3 || dues.TotalDue != 1500000 {
		t.Fatalf("three children, three challans: %v %+v", err, dues)
	}
	pay := pw.mustPay(t, f, 1250000)
	if len(pay.Allocations) != 3 {
		t.Fatalf("allocations: %+v", pay.Allocations)
	}
	var sum money.Money
	students := map[string]bool{}
	for _, a := range pay.Allocations {
		sum += a.Amount
		students[a.StudentID] = true
	}
	if sum != 1250000 || !students[w.studentID] || !students[second] || !students[third] {
		t.Fatalf("one payment, three children, adding up: %+v", pay.Allocations)
	}
	// Oldest first: the first two challans are paid in full, the third gets the rest.
	full, partial := 0, 0
	for _, a := range pay.Allocations {
		switch a.Amount {
		case 500000:
			full++
		case 250000:
			partial++
		}
	}
	if full != 2 || partial != 1 {
		t.Fatalf("want two full and one half: %+v", pay.Allocations)
	}
	if dues, _ := f.fees.GetFamilyDues(ctx, w.p, pw.familyID); dues.TotalDue != 250000 || len(dues.Items) != 1 {
		t.Fatalf("left to pay: %+v", dues)
	}
}

// REQ-FEE-022: the cashier may choose the split; refusals are specific.
func TestREQ_FEE_022_ChosenAllocationAndRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "pay-choose")
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	pw := payWorld{feeWorld: w, month: thisMonth()}
	second := pw.addSibling(t, f, "Second Child")
	pw.familyID = pw.releaseFor(t, f, pw.month, w.studentID)
	dues, _ := f.fees.GetFamilyDues(ctx, w.p, pw.familyID)
	var secondChallan, firstChallan string
	for _, d := range dues.Items {
		if d.StudentID == second {
			secondChallan = d.ChallanID
		} else {
			firstChallan = d.ChallanID
		}
	}

	// Pay the second child's challan first, on purpose.
	pay := pw.mustPay(t, f, 300000, func(in *fees.PaymentInput) {
		in.Allocations = []fees.Allocation{{ChallanID: secondChallan, Amount: 300000}}
	})
	if len(pay.Allocations) != 1 || pay.Allocations[0].ChallanID != secondChallan {
		t.Fatalf("chosen split: %+v", pay.Allocations)
	}

	refusals := map[string]struct {
		amount int64
		mod    func(*fees.PaymentInput)
		code   string
	}{
		"more than the family owes": {2000000, nil, "payments.exceeds_dues"},
		"split does not add up": {100000, func(in *fees.PaymentInput) {
			in.Allocations = []fees.Allocation{{ChallanID: firstChallan, Amount: 50000}}
		}, "payments.allocation_invalid"},
		"split over a challan": {250000, func(in *fees.PaymentInput) {
			in.Allocations = []fees.Allocation{{ChallanID: secondChallan, Amount: 250000}}
		}, "payments.allocation_invalid"},
		"future date":    {1000, func(in *fees.PaymentInput) { in.ReceivedOn = time.Now().AddDate(0, 0, 3) }, "payments.invalid"},
		"no such family": {1000, func(in *fees.PaymentInput) { in.FamilyID = "5c9c6c26-e0e4-4938-af5c-f6fc35688270" }, "students.family_not_found"},
		"bad method":     {1000, func(in *fees.PaymentInput) { in.Method = "bitcoin" }, "payments.invalid"},
	}
	for name, r := range refusals {
		t.Run(name, func(t *testing.T) {
			var mod []func(*fees.PaymentInput)
			if r.mod != nil {
				mod = append(mod, r.mod)
			}
			if _, err := pw.pay(t, f, r.amount, mod...); errCode(t, err) != r.code {
				t.Fatalf("want %s, got %v", r.code, err)
			}
		})
	}
	// A refused payment leaves no trace: the dues are what they were, and no receipt number was burnt.
	if dues, _ := f.fees.GetFamilyDues(ctx, w.p, pw.familyID); dues.TotalDue != 700000 {
		t.Fatalf("refusals must not change the dues: %+v", dues)
	}
	if next := pw.mustPay(t, f, 100000); next.ReceiptNo[len(next.ReceiptNo)-6:] != "000002" {
		t.Fatalf("no receipt number may be burnt by refusals: %s", next.ReceiptNo)
	}
}

// Six cashiers pay the same challan at once: it is never over-paid, and receipt numbers have no gaps.
func TestREQ_FEE_022_ConcurrentPaymentsNeverOverpayAndNeverSkipANumber(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "pay-race")

	const racers = 8 // 8 x 100,000 against a 500,000 challan: five fit
	results := make([]error, racers)
	numbers := make([]string, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := w.pay(t, f, 100000)
			results[i] = err
			if err == nil {
				numbers[i] = p.ReceiptNo
			}
		}()
	}
	wg.Wait()
	ok := 0
	seen := map[string]bool{}
	for i, err := range results {
		if err == nil {
			ok++
			seen[numbers[i]] = true
		} else if c := errCode(t, err); c != "payments.exceeds_dues" && c != "payments.no_dues" {
			t.Fatalf("unexpected error: %s", c)
		}
	}
	if ok != 5 {
		t.Fatalf("exactly five of eight fit, %d did", ok)
	}
	fy := fiscalYearOf(time.Now().In(karachi))
	for n := 1; n <= 5; n++ {
		want := receipt(fy, n)
		if !seen[want] {
			t.Fatalf("receipt numbers must be 1..5 with no gap; missing %s in %v", want, seen)
		}
	}
	page, _ := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{StudentID: w.studentID})
	if page.Items[0].Paid != 500000 || page.Items[0].PaymentStatus != "paid" {
		t.Fatalf("the challan is paid exactly once over: %+v", page.Items[0])
	}
	if d, c := trialLine(t, f, w.feeWorld, "1200"); d-c != 0 {
		t.Fatalf("receivable must be nil: %d", d-c)
	}
}

// US-FEE-004, REQ-FEE-025, BR-FEE-002, BR-FEE-003: voiding keeps the number, reverses the books, reopens the challan.
func TestUS_FEE_004_VoidAPayment(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "pay-void")
	pay := w.mustPay(t, f, 500000)
	if dues, _ := f.fees.GetFamilyDues(ctx, w.p, w.familyID); dues.TotalDue != 0 {
		t.Fatal("paid in full")
	}

	if _, err := f.fees.VoidPayment(ctx, w.p, pay.ID, "  "); errCode(t, err) != "payments.void_invalid" {
		t.Fatalf("void without a reason: %v", err)
	}
	voided, err := f.fees.VoidPayment(ctx, w.p, pay.ID, "entered twice by mistake")
	if err != nil || voided.Status != "voided" || voided.ReceiptNo != pay.ReceiptNo || voided.VoidReason == nil || voided.VoidedAt == nil || len(voided.Allocations) != 1 {
		t.Fatalf("void: %v %+v", err, voided)
	}
	// Books reversed: the cash is back to nothing and the receivable is owed again.
	if d, c := trialLine(t, f, w.feeWorld, "1000"); d-c != 0 {
		t.Fatalf("cash after void = %d, want 0", d-c)
	}
	if d, c := trialLine(t, f, w.feeWorld, "1200"); d-c != 500000 {
		t.Fatalf("receivable after void = %d, want 500000", d-c)
	}
	// The challan is open again, and a new payment gets the NEXT number, never the voided one.
	if dues, _ := f.fees.GetFamilyDues(ctx, w.p, w.familyID); dues.TotalDue != 500000 {
		t.Fatalf("the dues must come back: %+v", dues)
	}
	again := w.mustPay(t, f, 500000)
	if again.ReceiptNo == pay.ReceiptNo || again.ReceiptNo[len(again.ReceiptNo)-6:] != "000002" {
		t.Fatalf("a voided number is never reused: %s then %s", pay.ReceiptNo, again.ReceiptNo)
	}
	// Once only, and voided payments stay visible.
	if _, err := f.fees.VoidPayment(ctx, w.p, pay.ID, "again"); errCode(t, err) != "payments.already_voided" {
		t.Fatalf("void twice: %v", err)
	}
	if page, _ := f.fees.ListPayments(ctx, w.p, fees.PaymentFilter{Status: "voided"}); len(page.Items) != 1 || page.Items[0].ID != pay.ID {
		t.Fatalf("voided filter: %+v", page.Items)
	}
	if page, _ := f.fees.ListPayments(ctx, w.p, fees.PaymentFilter{FamilyID: w.familyID}); len(page.Items) != 2 {
		t.Fatalf("both stay in the history: %d", len(page.Items))
	}
	var audits int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'payment.voided'`, w.tenantID).Scan(&audits)
	if audits != 1 {
		t.Fatalf("one void audit row, got %d", audits)
	}
}

// Six people voiding the same payment at once: one wins, one reversal is posted.
func TestUS_FEE_004_ConcurrentVoidsReverseOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "pay-voidrace")
	pay := w.mustPay(t, f, 500000)

	const racers = 6
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.fees.VoidPayment(ctx, w.p, pay.ID, "duplicate click")
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if errCode(t, err) != "payments.already_voided" {
			t.Fatalf("unexpected: %v", err)
		}
	}
	var reversals int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1 AND reverses_entry_id IS NOT NULL AND source_type = 'ledger.reversal'`, w.tenantID).Scan(&reversals)
	if wins != 1 || reversals != 1 {
		t.Fatalf("one void wins and posts one reversal: %d wins, %d reversals", wins, reversals)
	}
}

// REQ-FEE-024: receipt numbers are gapless within a July-June fiscal year and restart in the next.
func TestREQ_FEE_024_ReceiptNumbersFollowTheFiscalYear(t *testing.T) {
	f := newFixture(t)
	w := newPayWorld(t, f, "pay-fy")
	on := func(s string) func(*fees.PaymentInput) {
		return func(in *fees.PaymentInput) { in.ReceivedOn = mustDate(t, s) }
	}
	a := w.mustPay(t, f, 100000, on("2025-06-30")) // last day of fiscal year 2024
	b := w.mustPay(t, f, 100000, on("2025-07-01")) // first day of 2025
	c := w.mustPay(t, f, 100000, on("2025-07-02"))
	d := w.mustPay(t, f, 100000, on("2025-06-15")) // back in 2024: continues that sequence
	for got, want := range map[string]string{a.ReceiptNo: "RCP-2024-000001", b.ReceiptNo: "RCP-2025-000001", c.ReceiptNo: "RCP-2025-000002", d.ReceiptNo: "RCP-2024-000002"} {
		if got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	}
	if a.FiscalYear != 2024 || b.FiscalYear != 2025 {
		t.Fatalf("fiscal years: %d %d", a.FiscalYear, b.FiscalYear)
	}
}

// BR-FEE-004: a security deposit on a challan is a liability when released, never income.
func TestBR_FEE_004_DepositsAreALiabilityNotIncome(t *testing.T) {
	f := newFixture(t)
	w := newFeeWorld(t, f, "pay-deposit")
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	w.setLine(t, f, "security_deposit", 300000, "one_time", "2020-01-01")
	pw := payWorld{feeWorld: w, month: thisMonth()}
	pw.familyID = pw.releaseFor(t, f, pw.month, w.studentID)

	if _, c := trialLine(t, f, w, "4000"); c != 500000 {
		t.Fatalf("Fee Income = %d, want only the 500000 of tuition", c)
	}
	if _, c := trialLine(t, f, w, "2100"); c != 300000 {
		t.Fatalf("Security Deposits Held = %d, want 300000", c)
	}
	if d, _ := trialLine(t, f, w, "1200"); d != 800000 {
		t.Fatalf("Fee Receivable = %d, want the whole 800000", d)
	}
	// Paying it moves cash in and clears the receivable; income and the liability do not move.
	pw.mustPay(t, f, 800000)
	if _, c := trialLine(t, f, w, "4000"); c != 500000 {
		t.Fatalf("income unchanged by payment: %d", c)
	}
	if d, c := trialLine(t, f, w, "1200"); d-c != 0 {
		t.Fatalf("receivable cleared: %d", d-c)
	}
}

// ADR-0002: school B cannot see, void or pay into school A's money.
func TestRLS_PaymentsAreIsolatedBetweenSchools(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newPayWorld(t, f, "pay-iso-a")
	b := newPayWorld(t, f, "pay-iso-b")
	pay := a.mustPay(t, f, 200000)

	if _, err := f.fees.GetPayment(ctx, b.p, pay.ID); errCode(t, err) != "payments.not_found" {
		t.Fatalf("B read A's payment: %v", err)
	}
	if _, err := f.fees.VoidPayment(ctx, b.p, pay.ID, "attack"); errCode(t, err) != "payments.not_found" {
		t.Fatalf("B voided A's payment: %v", err)
	}
	if _, err := f.fees.GetFamilyDues(ctx, b.p, a.familyID); errCode(t, err) != "students.family_not_found" {
		t.Fatalf("B read A's family dues: %v", err)
	}
	if _, err := f.fees.RecordPayment(ctx, b.p, fees.PaymentInput{FamilyID: a.familyID, Amount: 100, Method: "cash", AccountCode: "1000"}); errCode(t, err) != "students.family_not_found" {
		t.Fatalf("B paid into A's family: %v", err)
	}
	if page, _ := f.fees.ListPayments(ctx, b.p, fees.PaymentFilter{}); len(page.Items) != 0 {
		t.Fatalf("B's list leaked: %+v", page.Items)
	}
	if got, _ := f.fees.GetPayment(ctx, a.p, pay.ID); got.Status != "recorded" {
		t.Fatalf("A's payment was touched: %+v", got)
	}
}

// The database enforces what the service already checks, so a bug or a hand-written statement cannot
// break the money rules: allocations add up, challans are never over-paid, payments are history.
func TestREQ_FEE_022_TheDatabaseProtectsPaymentRules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "pay-db")
	pay := w.mustPay(t, f, 500000) // the challan is paid in full

	// Payments and allocations are history: not editable, not deletable, for any role.
	for name, stmt := range map[string]string{
		"change an amount":      `UPDATE payments SET amount = 1 WHERE id = $1`,
		"change the receipt no": `UPDATE payments SET receipt_no = 99 WHERE id = $1`,
		"delete a payment":      `DELETE FROM payments WHERE id = $1`,
		"change an allocation":  `UPDATE payment_allocations SET amount = 1 WHERE payment_id = $1`,
		"delete an allocation":  `DELETE FROM payment_allocations WHERE payment_id = $1`,
	} {
		if _, err := f.admin.Exec(ctx, stmt, pay.ID); err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}

	// A payment whose allocations do not add up to its amount cannot be committed.
	copyPayment := func(tx pgx.Tx, id string, receipt int, amount int64) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO payments (id, tenant_id, family_id, fiscal_year, receipt_no, amount, method, account_code, received_on, journal_entry_id)
			SELECT $1, tenant_id, family_id, fiscal_year, $2, $3, method, account_code, received_on, journal_entry_id FROM payments WHERE id = $4`,
			id, receipt, amount, pay.ID)
		return err
	}
	challan := pay.Allocations[0].ChallanID
	attempt := func(name string, build func(tx pgx.Tx) error) {
		t.Helper()
		tx, err := f.admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := build(tx); err != nil {
			return // refused immediately: also fine
		}
		err = tx.Commit(ctx)
		if err == nil {
			t.Fatalf("%s: the database committed it", name)
		}
		if !db.IsCheckViolation(err) {
			t.Fatalf("%s: want a check violation the service can turn into a retryable 409, got %v", name, err)
		}
	}
	attempt("allocations short of the amount", func(tx pgx.Tx) error {
		id := "6c9c6c26-e0e4-4938-af5c-f6fc35688201"
		if err := copyPayment(tx, id, 901, 100); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO payment_allocations (tenant_id, payment_id, challan_id, amount) SELECT tenant_id, $1, $2, 50 FROM payments WHERE id = $3`, id, challan, pay.ID)
		return err
	})
	attempt("a payment with no allocations", func(tx pgx.Tx) error {
		return copyPayment(tx, "6c9c6c26-e0e4-4938-af5c-f6fc35688202", 902, 100)
	})
	attempt("over-paying a challan that is already paid", func(tx pgx.Tx) error {
		id := "6c9c6c26-e0e4-4938-af5c-f6fc35688203"
		if err := copyPayment(tx, id, 903, 100); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO payment_allocations (tenant_id, payment_id, challan_id, amount) SELECT tenant_id, $1, $2, 100 FROM payments WHERE id = $3`, id, challan, pay.ID)
		return err
	})
	// A payment from one family cannot be allocated to another family's challan, even in this school.
	other := w.addStudent(t, f, "Other Family Child") // a fresh guardian mobile: a different family
	w.releaseFor(t, f, w.month, other)
	var otherChallan string
	if err := f.admin.QueryRow(ctx, `SELECT id FROM challans WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, other).Scan(&otherChallan); err != nil {
		t.Fatal(err)
	}
	attempt("allocating to another family's challan", func(tx pgx.Tx) error {
		id := "6c9c6c26-e0e4-4938-af5c-f6fc35688204"
		if err := copyPayment(tx, id, 904, 100); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO payment_allocations (tenant_id, payment_id, challan_id, amount) SELECT tenant_id, $1, $2, 100 FROM payments WHERE id = $3`, id, otherChallan, pay.ID)
		return err
	})
	var n int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM payments WHERE tenant_id = $1`, w.tenantID).Scan(&n)
	if n != 1 {
		t.Fatalf("none of the refused payments may remain: %d payments", n)
	}
}

// Payments racing the release of a challan: whatever the timing, no challan is ever paid more than
// its total, and the only errors are the business ones (nothing owed yet, or try again).
func TestREQ_FEE_022_PaymentsRacingAReleaseNeverOverpay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "pay-vs-release")
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	w.backdate(t, f, 30)
	family := ""
	for round := range 12 {
		month := thisMonth().AddDate(0, -round, 0)
		res := w.generate(t, f, month, false) // a draft, not yet released
		id := itemFor(t, res, w.studentID).ChallanID
		if family == "" {
			c, _ := f.fees.GetChallan(ctx, w.p, id)
			family = c.FamilyID
		}
		var wg sync.WaitGroup
		errs := make([]error, 4)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[0] = f.fees.Release(ctx, w.p, []string{id})
		}()
		for i := 1; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = f.fees.RecordPayment(ctx, w.p, fees.PaymentInput{FamilyID: family, Amount: 500000, Method: "cash", AccountCode: "1000"})
			}()
		}
		wg.Wait()
		if errs[0] != nil {
			t.Fatalf("round %d: release: %v", round, errs[0])
		}
		for _, err := range errs[1:] {
			if err == nil {
				continue
			}
			switch errCode(t, err) {
			case "payments.no_dues", "payments.exceeds_dues", "payments.conflict":
			default:
				t.Fatalf("round %d: unexpected error %v", round, err)
			}
		}
		var over int
		if err := f.admin.QueryRow(ctx, `
			SELECT count(*) FROM challans c WHERE c.tenant_id = $1 AND c.total <
			  COALESCE((SELECT sum(a.amount) FROM payment_allocations a JOIN payments p ON p.id = a.payment_id AND p.status = 'recorded' WHERE a.challan_id = c.id), 0)`, w.tenantID).Scan(&over); err != nil || over != 0 {
			t.Fatalf("round %d: %d challans are over-paid (%v)", round, over, err)
		}
	}
}
