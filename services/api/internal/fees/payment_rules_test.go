package fees

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

func codeOf(t *testing.T, err error) string {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("want an apperr, got %T: %v", err, err)
	}
	return e.Code
}

func bal(id, student string, due time.Time, no int64, total, paid money.Money) Balance {
	return Balance{ChallanID: id, StudentID: student, DueDate: due, ChallanNo: no, Total: total, Paid: paid}
}

// REQ-FEE-024 (owner decision 2026-11-17): the fiscal year runs July to June and is named by the year it starts.
func TestREQ_FEE_024_FiscalYear(t *testing.T) {
	cases := []struct {
		on   time.Time
		want int
	}{
		{day(2026, time.July, 1), 2026},
		{day(2026, time.December, 31), 2026},
		{day(2027, time.January, 1), 2026},
		{day(2027, time.June, 30), 2026},
		{day(2027, time.July, 1), 2027},
	}
	for _, c := range cases {
		if got := fiscalYear(c.on); got != c.want {
			t.Fatalf("%v: got %d, want %d", c.on.Format("2006-01-02"), got, c.want)
		}
	}
}

func TestREQ_FEE_024_ReceiptNumber(t *testing.T) {
	for _, c := range []struct {
		fy   int
		n    int64
		want string
	}{{2026, 1, "RCP-2026-000001"}, {2026, 123456, "RCP-2026-123456"}, {2027, 1, "RCP-2027-000001"}, {2026, 1234567, "RCP-2026-1234567"}} {
		if got := receiptNumber(c.fy, c.n); got != c.want {
			t.Fatalf("got %q, want %q", got, c.want)
		}
	}
}

// REQ-FEE-021: what a counter payment must say.
func TestREQ_FEE_021_ValidatePayment(t *testing.T) {
	today := day(2026, time.August, 20)
	const fam = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	ok := PaymentInput{FamilyID: fam, Amount: 1200000, Method: "cash", AccountCode: "1000"}
	got, err := validatePayment(ok, today)
	if err != nil || !got.ReceivedOn.Equal(today) {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	cases := []struct {
		name  string
		edit  func(*PaymentInput)
		field string
	}{
		{"no family", func(p *PaymentInput) { p.FamilyID = "" }, "family_id"},
		{"family not an id", func(p *PaymentInput) { p.FamilyID = "abc" }, "family_id"},
		{"zero amount", func(p *PaymentInput) { p.Amount = 0 }, "amount"},
		{"negative amount", func(p *PaymentInput) { p.Amount = -5 }, "amount"},
		{"no method", func(p *PaymentInput) { p.Method = "" }, "method"},
		{"unknown method", func(p *PaymentInput) { p.Method = "bitcoin" }, "method"},
		{"unknown account", func(p *PaymentInput) { p.AccountCode = "4000" }, "account_code"},
		{"future date", func(p *PaymentInput) { p.ReceivedOn = day(2026, time.August, 21) }, "received_on"},
		{"long reference", func(p *PaymentInput) { p.Reference = string(make([]byte, 101)) }, "reference"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := ok
			c.edit(&in)
			_, err := validatePayment(in, today)
			if err == nil || !slices.Contains(fieldsOf(err), c.field) {
				t.Fatalf("want an error on %q, got %v", c.field, err)
			}
		})
	}
	past := ok
	past.ReceivedOn = day(2026, time.August, 1)
	past.Reference = "  CHQ-0042 "
	if g, err := validatePayment(past, today); err != nil || g.Reference != "CHQ-0042" || !g.ReceivedOn.Equal(day(2026, time.August, 1)) {
		t.Fatalf("a past date and a trimmed reference are fine: %+v %v", g, err)
	}
	same := ok
	same.ReceivedOn = today
	if _, err := validatePayment(same, today); err != nil {
		t.Fatalf("today is allowed: %v", err)
	}
}

// REQ-FEE-022: the default split is oldest challan first.
func TestREQ_FEE_022_AllocateOldestFirst(t *testing.T) {
	jul, aug, sep := day(2026, time.July, 10), day(2026, time.August, 10), day(2026, time.September, 10)
	balances := []Balance{
		bal("c-sep", "s1", sep, 7, 500000, 0),
		bal("c-jul-b", "s2", jul, 3, 400000, 100000), // 300000 left
		bal("c-jul-a", "s1", jul, 2, 500000, 0),
		bal("c-aug", "s1", aug, 5, 500000, 0),
	}
	cases := []struct {
		name   string
		amount money.Money
		want   map[string]money.Money
	}{
		{"fits in the oldest", 200000, map[string]money.Money{"c-jul-a": 200000}},
		{"ties break by challan number", 500000, map[string]money.Money{"c-jul-a": 500000}},
		{"spills into the next", 600000, map[string]money.Money{"c-jul-a": 500000, "c-jul-b": 100000}},
		{"pays two exactly", 800000, map[string]money.Money{"c-jul-a": 500000, "c-jul-b": 300000}},
		{"everything", 1800000, map[string]money.Money{"c-jul-a": 500000, "c-jul-b": 300000, "c-aug": 500000, "c-sep": 500000}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := allocate(c.amount, balances, nil)
			if err != nil {
				t.Fatal(err)
			}
			var sum money.Money
			for _, a := range got {
				if c.want[a.ChallanID] != a.Amount {
					t.Fatalf("%s: got %d, want %d (%+v)", a.ChallanID, a.Amount, c.want[a.ChallanID], got)
				}
				sum += a.Amount
			}
			if len(got) != len(c.want) || sum != c.amount {
				t.Fatalf("got %+v, want %v", got, c.want)
			}
		})
	}
}

func TestREQ_FEE_022_AllocateRefusals(t *testing.T) {
	due := day(2026, time.August, 10)
	balances := []Balance{bal("c1", "s1", due, 1, 500000, 0), bal("c2", "s2", due, 2, 300000, 300000)} // c2 is paid
	if _, err := allocate(500001, balances, nil); err == nil || codeOf(t, err) != "payments.exceeds_dues" {
		t.Fatalf("more than owed: %v", err)
	}
	if got, err := allocate(500000, balances, nil); err != nil || len(got) != 1 || got[0].ChallanID != "c1" {
		t.Fatalf("exactly what is owed, and a paid challan is skipped: %v %+v", err, got)
	}
	if _, err := allocate(100, nil, nil); err == nil || codeOf(t, err) != "payments.no_dues" {
		t.Fatalf("nothing owed at all: %v", err)
	}
	if _, err := allocate(100, []Balance{bal("c2", "s2", due, 2, 300000, 300000)}, nil); err == nil || codeOf(t, err) != "payments.no_dues" {
		t.Fatalf("only paid challans: %v", err)
	}
}

// The cashier may choose the split; it must add up and respect each balance.
func TestREQ_FEE_022_ExplicitAllocation(t *testing.T) {
	due := day(2026, time.August, 10)
	balances := []Balance{bal("c1", "s1", due, 1, 500000, 100000), bal("c2", "s2", due, 2, 300000, 0)}
	ok, err := allocate(600000, balances, []Allocation{{ChallanID: "c2", Amount: 300000}, {ChallanID: "c1", Amount: 300000}})
	if err != nil || len(ok) != 2 {
		t.Fatalf("a valid split: %v %+v", err, ok)
	}
	bad := map[string][]Allocation{
		"does not add up":          {{ChallanID: "c1", Amount: 100000}},
		"over a challan's balance": {{ChallanID: "c1", Amount: 450000}, {ChallanID: "c2", Amount: 150000}},
		"unknown challan":          {{ChallanID: "nope", Amount: 600000}},
		"same challan twice":       {{ChallanID: "c2", Amount: 100000}, {ChallanID: "c2", Amount: 500000}},
		"zero line":                {{ChallanID: "c1", Amount: 0}, {ChallanID: "c2", Amount: 600000}},
		"negative line":            {{ChallanID: "c1", Amount: -1}, {ChallanID: "c2", Amount: 600001}},
	}
	for name, split := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := allocate(600000, balances, split); err == nil || codeOf(t, err) != "payments.allocation_invalid" {
				t.Fatalf("want payments.allocation_invalid, got %v", err)
			}
		})
	}
}

// What a payment posts: debit the account that took the money, credit Fee Receivable (BR-FEE-008).
func TestBR_FEE_008_PaymentEntry(t *testing.T) {
	e := paymentEntry("0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", "RCP-2026-000042", "1100", 1200000, day(2026, time.August, 20))
	if e.SourceType != "payment.recorded" || e.SourceID == "" || e.Memo == "" || !e.OccurredOn.Equal(day(2026, time.August, 20)) {
		t.Fatalf("header: %+v", e)
	}
	if len(e.Lines) != 2 || e.Lines[0].AccountCode != "1100" || e.Lines[0].Debit != 1200000 ||
		e.Lines[1].AccountCode != "1200" || e.Lines[1].Credit != 1200000 {
		t.Fatalf("lines: %+v", e.Lines)
	}
}

// BR-FEE-004 (owner decision 2026-11-17): a deposit is booked as a liability at release, never as income.
func TestBR_FEE_004_ReleaseBooksDepositsAsALiability(t *testing.T) {
	e := receivableEntry("0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", 7, 800000, 500000, day(2026, time.August, 3))
	want := map[string][2]money.Money{"1200": {800000, 0}, "4000": {0, 300000}, "2100": {0, 500000}}
	if len(e.Lines) != 3 {
		t.Fatalf("lines: %+v", e.Lines)
	}
	for _, l := range e.Lines {
		if w, ok := want[l.AccountCode]; !ok || l.Debit != w[0] || l.Credit != w[1] {
			t.Fatalf("line %+v, want %v", l, want)
		}
	}
	// A challan that is only a deposit has no income line, and one with no deposit has no liability line.
	only := receivableEntry("0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", 8, 500000, 500000, day(2026, time.August, 3))
	if len(only.Lines) != 2 || only.Lines[1].AccountCode != "2100" {
		t.Fatalf("deposit only: %+v", only.Lines)
	}
	none := receivableEntry("0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", 9, 500000, 0, day(2026, time.August, 3))
	if len(none.Lines) != 2 || none.Lines[1].AccountCode != "4000" {
		t.Fatalf("no deposit: %+v", none.Lines)
	}
}

func TestREQ_FEE_063_PaymentProgress(t *testing.T) {
	for _, c := range []struct {
		status      string
		total, paid money.Money
		want        string
	}{
		{"released", 500000, 0, "unpaid"}, {"released", 500000, 1, "partial"}, {"released", 500000, 499999, "partial"},
		{"released", 500000, 500000, "paid"}, {"draft", 500000, 0, ""}, {"void", 500000, 0, ""},
	} {
		if got := paymentProgress(c.status, c.total, c.paid); got != c.want {
			t.Fatalf("%s %d/%d: got %q, want %q", c.status, c.paid, c.total, got, c.want)
		}
	}
}

func TestREQ_FEE_025_ValidatePaymentVoidReason(t *testing.T) {
	if r, err := validatePaymentVoidReason("  entered twice "); err != nil || r != "entered twice" {
		t.Fatalf("got %q %v", r, err)
	}
	for _, bad := range []string{"", "  ", "ab", string(make([]byte, 201))} {
		if _, err := validatePaymentVoidReason(bad); err == nil {
			t.Fatalf("%q must be refused", bad)
		}
	}
	if _, err := validatePaymentVoidReason("abc"); err != nil {
		t.Fatalf("3 characters are allowed: %v", err)
	}
}

// A refusal only the database can see at commit (or a deadlock) is a retryable 409, never a 500.
func TestREQ_FEE_022_CommitTimeRefusalsAreRetryable(t *testing.T) {
	for _, code := range []string{"23514", "40P01", "40001"} {
		err := retryable(fmt.Errorf("commit tenant transaction: %w", &pgconn.PgError{Code: code}))
		if codeOf(t, err) != "payments.conflict" {
			t.Fatalf("%s must become payments.conflict, got %v", code, err)
		}
	}
	plain := errors.New("connection refused")
	if retryable(plain) != plain {
		t.Fatal("other errors pass through unchanged")
	}
	if retryable(nil) != nil {
		t.Fatal("nil stays nil")
	}
	if got := retryable(&pgconn.PgError{Code: "23505"}); codeOf2(got) == "payments.conflict" {
		t.Fatal("a duplicate-key error is not a retry")
	}
}

func codeOf2(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Code
	}
	return ""
}
