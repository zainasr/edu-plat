package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/fees"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

func hold(in *fees.PaymentInput) { in.HoldExcessAsAdvance = true }

func net(t *testing.T, f *fixture, w feeWorld, code string) money.Money {
	t.Helper()
	d, c := trialLine(t, f, w, code)
	return d - c
}

func (w payWorld) refund(t *testing.T, f *fixture, paymentID string, amount int64, account string) (fees.Payment, error) {
	return f.fees.RefundAdvance(context.Background(), w.p, paymentID, fees.RefundInput{Amount: money.Money(amount), AccountCode: account, Reason: "family asked for it back"})
}

// nextMonth bills and releases the month after this one (one ahead is allowed), which is when a family's
// advance is applied.
func (w payWorld) nextMonth(t *testing.T, f *fixture) {
	t.Helper()
	w.releaseFor(t, f, w.month.AddDate(0, 1, 0), w.studentID)
}

// REQ-FEE-023, DEC-010, US-FEE-002: a payment above what is owed is refused unless asked to hold the
// extra as Advance; then it posts three balanced lines and is shown on the receipt and the dues.
func TestREQ_FEE_023_AdvanceIsHeldOnlyWhenAskedFor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "adv-hold")

	if _, err := w.pay(t, f, 800000); errCode(t, err) != "payments.exceeds_dues" {
		t.Fatalf("without the flag: %v", err)
	}
	pay := w.mustPay(t, f, 800000, hold)
	if pay.Advance != 300000 || pay.AdvanceAvailable != 300000 || pay.AdvanceApplied != 0 || pay.AdvanceRefunded != 0 ||
		len(pay.Allocations) != 1 || pay.Allocations[0].Amount != 500000 {
		t.Fatalf("payment: %+v", pay)
	}
	e, err := f.ledger.GetEntry(ctx, w.p, pay.EntryID)
	if err != nil || len(e.Lines) != 3 {
		t.Fatalf("entry: %v %+v", err, e)
	}
	if got := net(t, f, w.feeWorld, "1000"); got != 800000 {
		t.Fatalf("cash = %d", got)
	}
	if got := net(t, f, w.feeWorld, "1200"); got != 0 {
		t.Fatalf("receivable = %d, the challan is paid", got)
	}
	if _, c := trialLine(t, f, w.feeWorld, "2000"); c != 300000 {
		t.Fatalf("Advance Fees Received = %d, want 300000", c)
	}
	dues, _ := f.fees.GetFamilyDues(ctx, w.p, w.familyID)
	if dues.TotalDue != 0 || dues.AdvanceAvailable != 300000 {
		t.Fatalf("dues: %+v", dues)
	}
	// Nothing owed, nothing asked: refused; asked: the whole payment is advance against the next challan.
	if _, err := w.pay(t, f, 100000); errCode(t, err) != "payments.no_dues" {
		t.Fatalf("nothing owed: %v", err)
	}
	prepay := w.mustPay(t, f, 100000, hold)
	if prepay.Advance != 100000 || len(prepay.Allocations) != 0 {
		t.Fatalf("prepay: %+v", prepay)
	}
	if dues, _ := f.fees.GetFamilyDues(ctx, w.p, w.familyID); dues.AdvanceAvailable != 400000 {
		t.Fatalf("advance held: %+v", dues)
	}
}

// REQ-FEE-023: the next released challan uses the advance, oldest payment first, and posts debit
// Advance / credit Receivable, exactly once.
func TestREQ_FEE_023_AdvanceIsAppliedWhenTheNextChallanIsReleased(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "adv-apply")
	first := w.mustPay(t, f, 800000, hold)  // 300000 advance
	second := w.mustPay(t, f, 400000, hold) // nothing owed: all advance
	if first.Advance != 300000 || second.Advance != 400000 {
		t.Fatalf("setup: %d %d", first.Advance, second.Advance)
	}

	w.nextMonth(t, f) // a 500000 challan: 300000 from the first payment, 200000 from the second
	page, _ := f.fees.ListChallans(ctx, w.p, fees.ChallanFilter{StudentID: w.studentID, Month: w.month.AddDate(0, 1, 0)})
	c := page.Items[0]
	if c.Paid != 500000 || c.PaymentStatus != "paid" {
		t.Fatalf("the challan is paid from advance: %+v", c)
	}
	a, _ := f.fees.GetPayment(ctx, w.p, first.ID)
	b, _ := f.fees.GetPayment(ctx, w.p, second.ID)
	if a.AdvanceApplied != 300000 || a.AdvanceAvailable != 0 || b.AdvanceApplied != 200000 || b.AdvanceAvailable != 200000 {
		t.Fatalf("oldest first: first %+v / second %+v", a, b)
	}
	if got := net(t, f, w.feeWorld, "2000"); got != -200000 {
		t.Fatalf("Advance Fees Received should still hold 200000: %d", got)
	}
	if got := net(t, f, w.feeWorld, "1200"); got != 0 {
		t.Fatalf("receivable = %d", got)
	}
	var applied, audits int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM journal_entries WHERE tenant_id = $1 AND source_type = 'advance.applied'`, w.tenantID).Scan(&applied)
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'advance.applied'`, w.tenantID).Scan(&audits)
	if applied != 1 || audits != 1 {
		t.Fatalf("one entry and one audit row per challan: %d %d", applied, audits)
	}
	if dues, _ := f.fees.GetFamilyDues(ctx, w.p, w.familyID); dues.TotalDue != 0 || dues.AdvanceAvailable != 200000 {
		t.Fatalf("dues: %+v", dues)
	}
}

// REQ-FEE-023: advance not applied is paid back explicitly, never more than is held.
func TestREQ_FEE_023_RefundAdvance(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "adv-refund")
	pay := w.mustPay(t, f, 800000, hold) // 300000 advance

	out, err := w.refund(t, f, pay.ID, 100000, "1000")
	if err != nil || out.AdvanceRefunded != 100000 || out.AdvanceAvailable != 200000 || len(out.Refunds) != 1 ||
		out.Refunds[0].Amount != 100000 || out.Refunds[0].AccountCode != "1000" || out.Refunds[0].EntryID == "" {
		t.Fatalf("refund: %v %+v", err, out)
	}
	if got := net(t, f, w.feeWorld, "1000"); got != 700000 {
		t.Fatalf("cash after refund = %d", got)
	}
	if _, c := trialLine(t, f, w.feeWorld, "2000"); c != 300000 {
		t.Fatalf("credit side of 2000 stays: %d", c)
	}
	if got := net(t, f, w.feeWorld, "2000"); got != -200000 {
		t.Fatalf("advance still held = %d", got)
	}
	for name, c := range map[string]struct {
		amount  int64
		account string
		code    string
	}{
		"more than is left": {200001, "1000", "payments.refund_exceeds_advance"},
		"zero":              {0, "1000", "payments.refund_invalid"},
		"wrong account":     {1000, "1200", "payments.refund_invalid"},
	} {
		if _, err := w.refund(t, f, pay.ID, c.amount, c.account); errCode(t, err) != c.code {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := f.fees.RefundAdvance(ctx, w.p, pay.ID, fees.RefundInput{Amount: 1000, AccountCode: "1000", Reason: " "}); errCode(t, err) != "payments.refund_invalid" {
		t.Fatalf("no reason: %v", err)
	}
	if out, err := w.refund(t, f, pay.ID, 200000, "1100"); err != nil || out.AdvanceAvailable != 0 || len(out.Refunds) != 2 {
		t.Fatalf("the rest, to the bank: %v %+v", err, out)
	}
	if _, err := w.refund(t, f, pay.ID, 1, "1000"); errCode(t, err) != "payments.refund_exceeds_advance" {
		t.Fatalf("nothing left: %v", err)
	}
	var n int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'advance.refunded'`, w.tenantID).Scan(&n)
	if n != 2 {
		t.Fatalf("two audited refunds, got %d", n)
	}
}

// US-FEE-004: a payment whose advance is unused can be voided (everything reverses); once any advance
// has been applied or refunded it can no longer be voided.
func TestUS_FEE_004_VoidWithAdvance(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "adv-void")

	clean := w.mustPay(t, f, 800000, hold)
	if _, err := f.fees.VoidPayment(ctx, w.p, clean.ID, "entered on the wrong family"); err != nil {
		t.Fatalf("an untouched advance can be voided: %v", err)
	}
	// Cash and Advance are back to nothing; the challan the payment had paid is owed again.
	for code, want := range map[string]money.Money{"1000": 0, "1200": 500000, "2000": 0} {
		if got := net(t, f, w.feeWorld, code); got != want {
			t.Fatalf("account %s = %d, want %d", code, got, want)
		}
	}
	if v, _ := f.fees.GetPayment(ctx, w.p, clean.ID); v.AdvanceAvailable != 0 {
		t.Fatalf("a voided payment holds nothing: %+v", v)
	}

	refunded := w.mustPay(t, f, 800000, hold)
	if _, err := w.refund(t, f, refunded.ID, 50000, "1000"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.fees.VoidPayment(ctx, w.p, refunded.ID, "too late"); errCode(t, err) != "payments.advance_in_use" {
		t.Fatalf("refunded: %v", err)
	}
	if _, err := w.refund(t, f, clean.ID, 1, "1000"); errCode(t, err) != "payments.already_voided" {
		t.Fatalf("refund of a voided payment: %v", err)
	}
	// A voided payment's challans reopen, a payment with advance applied cannot be voided.
	w.nextMonth(t, f)
	if _, err := f.fees.VoidPayment(ctx, w.p, refunded.ID, "still too late"); errCode(t, err) != "payments.advance_in_use" {
		t.Fatalf("applied: %v", err)
	}
	// The database says the same, even to the table owner.
	if _, err := f.admin.Exec(ctx, `UPDATE payments SET status = 'voided', void_reason = 'sneaky', voided_at = now(), void_entry_id = journal_entry_id WHERE id = $1`, refunded.ID); err == nil {
		t.Fatal("the database must refuse to void a payment whose advance has been used")
	}
}

// BR-FEE-008, DEC-010: advance spent by two things at once (a release and a refund, or a release and a
// void) is never spent twice, whatever the timing.
func TestREQ_FEE_023_AdvanceIsNeverSpentTwice(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for round := range 8 {
		w := newFeeWorld(t, f, "adv-race")
		w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
		w.backdate(t, f, 6)
		pw := payWorld{feeWorld: w, month: thisMonth()}
		pw.familyID = pw.releaseFor(t, f, pw.month.AddDate(0, -1, 0), w.studentID)
		pay := pw.mustPay(t, f, 800000, hold)    // 300000 advance
		res := w.generate(t, f, pw.month, false) // a draft for this month: 500000
		draft := itemFor(t, res, w.studentID).ChallanID

		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[0] = f.fees.Release(ctx, w.p, []string{draft})
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if round%2 == 0 {
				_, errs[1] = pw.refund(t, f, pay.ID, 300000, "1000")
			} else {
				_, errs[1] = f.fees.VoidPayment(ctx, w.p, pay.ID, "duplicate click")
			}
		}()
		wg.Wait()
		if errs[0] != nil {
			t.Fatalf("round %d: release: %v", round, errs[0])
		}
		if errs[1] != nil {
			switch errCode(t, errs[1]) {
			case "payments.refund_exceeds_advance", "payments.advance_in_use":
			default:
				t.Fatalf("round %d: unexpected error %v", round, errs[1])
			}
		}
		var spent, held, voided int64
		_ = f.admin.QueryRow(ctx, `
			SELECT COALESCE((SELECT sum(amount) FROM advance_applications WHERE payment_id = $1), 0)
			     + COALESCE((SELECT sum(amount) FROM advance_refunds WHERE payment_id = $1), 0),
			       advance, (status = 'voided')::int FROM payments WHERE id = $1`, pay.ID).Scan(&spent, &held, &voided)
		if spent > held {
			t.Fatalf("round %d: %d of advance spent but only %d held", round, spent, held)
		}
		if voided == 1 && spent > 0 {
			t.Fatalf("round %d: a voided payment must not have spent advance", round)
		}
		if got := net(t, f, w, "2000"); got > 0 {
			t.Fatalf("round %d: Advance Fees Received went negative (debit-heavy): %d", round, got)
		}
	}
}

// ADR-0002: school B cannot refund, or see the advance of, school A's family.
func TestRLS_AdvanceIsIsolatedBetweenSchools(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newPayWorld(t, f, "adv-iso-a")
	b := newPayWorld(t, f, "adv-iso-b")
	pay := a.mustPay(t, f, 800000, hold)
	if _, err := b.refund(t, f, pay.ID, 1000, "1000"); errCode(t, err) != "payments.not_found" {
		t.Fatalf("B refunded A's advance: %v", err)
	}
	if _, err := f.fees.GetFamilyDues(ctx, b.p, a.familyID); errCode(t, err) != "students.family_not_found" {
		t.Fatalf("B read A's advance: %v", err)
	}
	if got, _ := f.fees.GetPayment(ctx, a.p, pay.ID); got.AdvanceAvailable != 300000 {
		t.Fatalf("A's advance was touched: %+v", got)
	}
}

// The database enforces the advance rules too: spending beyond what a payment held, applying to a challan
// that is already paid or belongs to another family, and editing applications or refunds are all refused.
func TestREQ_FEE_023_TheDatabaseProtectsAdvance(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "adv-db")
	sibling := w.addSibling(t, f, "Sibling Child") // same family: a second challan that can take advance
	w.releaseFor(t, f, w.month, sibling)
	dues, _ := f.fees.GetFamilyDues(ctx, w.p, w.familyID)
	var paidChallan string
	for _, d := range dues.Items {
		if d.StudentID == w.studentID {
			paidChallan = d.ChallanID
		}
	}
	// Pay the first child's challan in full and hold 300000 as advance; the sibling's challan still owes 500000.
	pay := w.mustPay(t, f, 800000, hold, func(in *fees.PaymentInput) {
		in.Allocations = []fees.Allocation{{ChallanID: paidChallan, Amount: 500000}}
	})
	if pay.Advance != 300000 {
		t.Fatalf("setup: %+v", pay)
	}
	var siblingChallan string
	if err := f.admin.QueryRow(ctx, `SELECT id FROM challans WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, sibling).Scan(&siblingChallan); err != nil {
		t.Fatal(err)
	}
	other := w.addStudent(t, f, "Other Family Child") // a different family
	w.releaseFor(t, f, w.month, other)
	var otherChallan string
	if err := f.admin.QueryRow(ctx, `SELECT id FROM challans WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, other).Scan(&otherChallan); err != nil {
		t.Fatal(err)
	}
	if _, err := w.refund(t, f, pay.ID, 1000, "1000"); err != nil { // leaves a refund row to try to edit
		t.Fatal(err)
	}

	attempt := func(name string, build func(tx pgx.Tx) error) {
		t.Helper()
		tx, err := f.admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := build(tx); err != nil {
			return
		}
		err = tx.Commit(ctx)
		if err == nil {
			t.Fatalf("%s: the database committed it", name)
		}
		if !db.IsCheckViolation(err) {
			t.Fatalf("%s: want a check violation, got %v", name, err)
		}
	}
	apply := func(challan string, amount int) func(tx pgx.Tx) error {
		return func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO advance_applications (tenant_id, payment_id, challan_id, amount)
				SELECT tenant_id, id, $2, $3 FROM payments WHERE id = $1`, pay.ID, challan, amount)
			return err
		}
	}
	attempt("advance applied to a challan that is already paid in full", apply(paidChallan, 1))
	attempt("advance applied to another family's challan", apply(otherChallan, 1))
	attempt("more advance applied than the payment held", apply(siblingChallan, 400000)) // challan owes 500000, advance held 299000 after the refund
	attempt("refunding more than is held", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO advance_refunds (tenant_id, payment_id, amount, account_code, reason, journal_entry_id)
			SELECT tenant_id, id, 300000, '1000', 'sneaky', journal_entry_id FROM payments WHERE id = $1`, pay.ID)
		return err
	})
	attempt("a payment whose advance and allocations do not add up", func(tx pgx.Tx) error {
		id := "6c9c6c26-e0e4-4938-af5c-f6fc35688211"
		_, err := tx.Exec(ctx, `INSERT INTO payments (id, tenant_id, family_id, fiscal_year, receipt_no, amount, advance, method, account_code, received_on, journal_entry_id)
			SELECT $1, tenant_id, family_id, fiscal_year, 7777, 1000, 400, method, account_code, received_on, journal_entry_id FROM payments WHERE id = $2`, id, pay.ID)
		return err
	})

	// Applications and refunds are history: not editable, not deletable.
	if _, err := f.admin.Exec(ctx, `INSERT INTO advance_applications (tenant_id, payment_id, challan_id, amount) SELECT tenant_id, id, $2, 100 FROM payments WHERE id = $1`, pay.ID, siblingChallan); err != nil {
		t.Fatalf("a legitimate application: %v", err)
	}
	for name, stmt := range map[string]string{
		"change an application": `UPDATE advance_applications SET amount = 1 WHERE payment_id = $1`,
		"delete an application": `DELETE FROM advance_applications WHERE payment_id = $1`,
		"change a refund":       `UPDATE advance_refunds SET amount = 1 WHERE payment_id = $1`,
		"delete a refund":       `DELETE FROM advance_refunds WHERE payment_id = $1`,
	} {
		if _, err := f.admin.Exec(ctx, stmt, pay.ID); err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}
}

// REQ-FEE-041, REQ-FEE-042: defaulters are families 30+ days overdue, most overdue first, with who to call.
func TestREQ_FEE_041_Defaulters(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "defaulters")
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	second := w.addStudent(t, f, "Other Family Child") // a different family
	w.backdate(t, f, 6)
	today := time.Now().In(karachi)
	day0 := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	older, recent := thisMonth().AddDate(0, -2, 0), thisMonth().AddDate(0, -1, 0)

	billAndRelease := func(month time.Time, dueDaysAgo int) {
		t.Helper()
		due := day0.AddDate(0, 0, -dueDaysAgo)
		res, err := f.fees.Generate(ctx, w.p, fees.GenerateInput{Month: month, DueDate: &due})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, it := range res.Items {
			if it.ChallanID != "" {
				ids = append(ids, it.ChallanID)
			}
		}
		if _, err := f.fees.Release(ctx, w.p, ids); err != nil {
			t.Fatal(err)
		}
	}
	// Only the first child is billable two months ago; both are billable last month.
	if _, err := f.admin.Exec(ctx, `UPDATE enrolments SET started_on = $3 WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, second, thisMonth()); err != nil {
		t.Fatal(err)
	}
	billAndRelease(older, 45)
	if _, err := f.admin.Exec(ctx, `UPDATE enrolments SET started_on = $3 WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, second, thisMonth().AddDate(0, -6, 0)); err != nil {
		t.Fatal(err)
	}
	billAndRelease(recent, 10)
	_ = recent

	list := func(minDays *int, limit int, cursor string) fees.DefaulterPage {
		t.Helper()
		page, err := f.fees.ListDefaulters(ctx, w.p, fees.DefaulterFilter{MinDays: minDays, Limit: limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("defaulters: %v", err)
		}
		return page
	}
	// Default window of 30 days: only the first family (its older challan is 45 days overdue).
	page := list(nil, 0, "")
	if len(page.Items) != 1 {
		t.Fatalf("default window: %+v", page.Items)
	}
	d := page.Items[0]
	if d.OverdueTotal != 500000 || d.DaysOverdue != 45 || len(d.Challans) != 1 || d.Challans[0].Balance != 500000 || d.Challans[0].StudentID != w.studentID {
		t.Fatalf("first family: %+v", d)
	}
	if d.FeePayer == nil || d.FeePayer.Name != "Kamran Khan" || d.FeePayer.Relationship != "uncle" || d.FeePayer.Mobile == "" {
		t.Fatalf("who to call: %+v", d.FeePayer)
	}

	// A shorter window brings in the second family, most overdue first, and pages.
	five := 5
	both := list(&five, 0, "")
	if len(both.Items) != 2 || both.Items[0].FamilyID != d.FamilyID || both.Items[0].OverdueTotal != 1000000 || both.Items[1].OverdueTotal != 500000 || both.Items[1].DaysOverdue != 10 {
		t.Fatalf("a 5 day window: %+v", both.Items)
	}
	p1 := list(&five, 1, "")
	if len(p1.Items) != 1 || p1.NextCursor == "" {
		t.Fatalf("page 1: %+v", p1)
	}
	p2 := list(&five, 1, p1.NextCursor)
	if len(p2.Items) != 1 || p2.NextCursor != "" || p2.Items[0].FamilyID == p1.Items[0].FamilyID {
		t.Fatalf("page 2: %+v", p2)
	}

	// A partial payment lowers what is overdue; paying it all takes the family off the list at once.
	famBefore := d.FamilyID
	if _, err := f.fees.RecordPayment(ctx, w.p, fees.PaymentInput{FamilyID: famBefore, Amount: 200000, Method: "cash", AccountCode: "1000"}); err != nil {
		t.Fatal(err)
	}
	if page := list(nil, 0, ""); len(page.Items) != 1 || page.Items[0].OverdueTotal != 300000 {
		t.Fatalf("after a partial payment: %+v", page.Items)
	}
	if _, err := f.fees.RecordPayment(ctx, w.p, fees.PaymentInput{FamilyID: famBefore, Amount: 800000, Method: "cash", AccountCode: "1000"}); err != nil {
		t.Fatal(err)
	}
	if page := list(nil, 0, ""); len(page.Items) != 0 {
		t.Fatalf("paid in full: %+v", page.Items)
	}
	// Refusals.
	for _, bad := range []int{0, 366, -1} {
		if _, err := f.fees.ListDefaulters(ctx, w.p, fees.DefaulterFilter{MinDays: &bad}); errCode(t, err) != "defaulters.invalid" {
			t.Fatalf("min_days %d: %v", bad, err)
		}
	}
	if _, err := f.fees.ListDefaulters(ctx, w.p, fees.DefaulterFilter{Cursor: "junk"}); errCode(t, err) != "defaulters.cursor_invalid" {
		t.Fatalf("bad cursor: %v", err)
	}
	// School B sees none of this.
	other := newPayWorld(t, f, "defaulters-b") // has its own overdue challan, but must not see A's families
	theirs, _ := f.fees.ListDefaulters(ctx, other.p, fees.DefaulterFilter{MinDays: &five})
	for _, it := range theirs.Items {
		if it.FamilyID == famBefore || it.FamilyID == both.Items[1].FamilyID {
			t.Fatalf("B sees A's defaulters: %+v", it)
		}
	}
}

// A family with two overdue challans, one of them paid, is listed for the one still owed.
func TestREQ_FEE_041_DefaultersListOnlyWhatIsStillOwed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newFeeWorld(t, f, "defaulters-paid")
	w.setLine(t, f, "tuition", 500000, "monthly", "2020-01-01")
	w.backdate(t, f, 6)
	today := time.Now().In(karachi)
	day0 := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	for i, daysAgo := range []int{50, 40} { // two months back and one month back, both well over 30 days overdue
		month := thisMonth().AddDate(0, -(2 - i), 0)
		due := day0.AddDate(0, 0, -daysAgo)
		res, err := f.fees.Generate(ctx, w.p, fees.GenerateInput{Month: month, DueDate: &due})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.fees.Release(ctx, w.p, []string{itemFor(t, res, w.studentID).ChallanID}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.fees.ListDefaulters(ctx, w.p, fees.DefaulterFilter{})
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Challans) != 2 || page.Items[0].OverdueTotal != 1000000 {
		t.Fatalf("both owed: %v %+v", err, page.Items)
	}
	older := page.Items[0].Challans[0] // oldest due date first
	if _, err := f.fees.RecordPayment(ctx, w.p, fees.PaymentInput{
		FamilyID: page.Items[0].FamilyID, Amount: 500000, Method: "cash", AccountCode: "1000",
		Allocations: []fees.Allocation{{ChallanID: older.ChallanID, Amount: 500000}},
	}); err != nil {
		t.Fatal(err)
	}
	page, _ = f.fees.ListDefaulters(ctx, w.p, fees.DefaulterFilter{})
	if len(page.Items) != 1 || page.Items[0].OverdueTotal != 500000 || len(page.Items[0].Challans) != 1 || page.Items[0].Challans[0].ChallanID == older.ChallanID {
		t.Fatalf("only the unpaid challan is listed: %+v", page.Items)
	}
}

// Six refunds of the same advance at once: only what is held can ever be paid back.
func TestREQ_FEE_023_ConcurrentRefundsNeverPayBackMoreThanIsHeld(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newPayWorld(t, f, "adv-refund-race")
	pay := w.mustPay(t, f, 800000, hold) // 300000 advance

	const racers = 6 // each asks for 100000: three fit
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = w.refund(t, f, pay.ID, 100000, "1000")
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if errCode(t, err) != "payments.refund_exceeds_advance" {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if ok != 3 {
		t.Fatalf("exactly three refunds of 100000 fit in 300000, %d did", ok)
	}
	got, _ := f.fees.GetPayment(ctx, w.p, pay.ID)
	if got.AdvanceRefunded != 300000 || got.AdvanceAvailable != 0 || len(got.Refunds) != 3 {
		t.Fatalf("payment: %+v", got)
	}
	if n := net(t, f, w.feeWorld, "2000"); n != 0 {
		t.Fatalf("Advance Fees Received must be fully paid out: %d", n)
	}
}
