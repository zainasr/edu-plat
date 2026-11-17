package fees

import (
	"strconv"
	"testing"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func line(feeType, frequency string, amount money.Money) Line {
	return Line{FeeTypeID: feeType, Name: feeType, Kind: "custom", Amount: amount, PlanAmount: amount, Frequency: frequency, Source: "plan"}
}

func lineTypes(lines []Line) []string {
	var out []string
	for _, l := range lines {
		out = append(out, l.FeeTypeID)
	}
	return out
}

// REQ-FEE-002, REQ-FEE-003: which plan lines fall due on a month's challan.
func TestREQ_FEE_003_DueLines(t *testing.T) {
	lines := []Line{
		line("tuition", "monthly", 500000),
		line("admission", "one_time", 100000),
		line("annual", "annual", 250000),
		line("books", "ad_hoc", 80000),
		line("waived", "monthly", 0),
	}
	anchors := map[string]time.Time{"annual": month(2026, time.April), "admission": month(2026, time.April)}
	cases := []struct {
		name          string
		month         time.Time
		firstEnrolled time.Time
		want          []string
	}{
		{"admission month gets the one-time fee", month(2026, time.August), day(2026, time.August, 14), []string{"tuition", "admission"}},
		{"later months do not", month(2026, time.September), day(2026, time.August, 14), []string{"tuition"}},
		{"annual falls in the anchor month of the year", month(2027, time.April), day(2025, time.April, 1), []string{"tuition", "annual"}},
		{"annual does not fall in other months", month(2027, time.May), day(2025, time.April, 1), []string{"tuition"}},
		{"ad hoc is never generated and a zero line is dropped", month(2026, time.August), day(2020, time.January, 1), []string{"tuition"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := lineTypes(dueLines(c.month, lines, anchors, c.firstEnrolled))
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestREQ_FEE_003_AnnualWithoutAnAnchorIsNotDue(t *testing.T) {
	got := dueLines(month(2026, time.April), []Line{line("annual", "annual", 1000)}, nil, day(2020, time.April, 1))
	if len(got) != 0 {
		t.Fatalf("an annual line with no anchor must not be billed, got %v", lineTypes(got))
	}
}

func TestREQ_FEE_003_ValidateGenerate(t *testing.T) {
	now := day(2026, time.August, 19)
	m, due, err := validateGenerate(GenerateInput{Month: day(2026, time.August, 19)}, now)
	if err != nil || !m.Equal(month(2026, time.August)) || !due.Equal(day(2026, time.August, 10)) {
		t.Fatalf("defaults: month %v due %v err %v", m, due, err)
	}
	if _, _, err := validateGenerate(GenerateInput{}, now); err == nil {
		t.Fatal("a missing month must be refused")
	}
	early := day(2026, time.July, 31)
	if _, _, err := validateGenerate(GenerateInput{Month: month(2026, time.August), DueDate: &early}, now); err == nil {
		t.Fatal("a due date before the month starts must be refused")
	}
	same := month(2026, time.August)
	if _, _, err := validateGenerate(GenerateInput{Month: month(2026, time.August), DueDate: &same}, now); err != nil {
		t.Fatalf("a due date on the first of the month is allowed: %v", err)
	}
}

func TestREQ_FEE_003_ChallanTotal(t *testing.T) {
	total, err := challanTotal([]Line{line("a", "monthly", 500000), line("b", "monthly", 25050)})
	if err != nil || total != 525050 {
		t.Fatalf("total %d err %v", total, err)
	}
	if _, err := challanTotal([]Line{line("a", "monthly", money.Money(1<<62)), line("b", "monthly", money.Money(1<<62))}); err == nil {
		t.Fatal("an overflowing total must be an error, never a wrapped number")
	}
}

// REQ-FEE-006: the consumer number is 14 digits: 13 of the challan number, then a Luhn check digit.
func TestREQ_FEE_006_ConsumerNumber(t *testing.T) {
	got := consumerNumber(1)
	if len(got) != 14 || got[:13] != "0000000000001" {
		t.Fatalf("got %q", got)
	}
	if !luhnValid(got) {
		t.Fatalf("%q fails its own check digit", got)
	}
	seen := map[string]int64{}
	for n := int64(1); n <= 2000; n++ {
		c := consumerNumber(n)
		if !luhnValid(c) {
			t.Fatalf("%d -> %q has a bad check digit", n, c)
		}
		if prev, dup := seen[c]; dup {
			t.Fatalf("%d and %d share consumer number %s", prev, n, c)
		}
		seen[c] = n
	}
	// A known Luhn vector: payload 7992739871 has check digit 3.
	if d := luhnCheckDigit("7992739871"); d != '3' {
		t.Fatalf("luhn check digit of 7992739871 = %c, want 3", d)
	}
	if _, err := strconv.ParseInt(consumerNumber(9999999999999), 10, 64); err != nil {
		t.Fatalf("largest challan number must still give 14 digits: %v", err)
	}
}

// BR-FEE-008: release posts a balanced entry, debit Fee Receivable, credit Fee Income.
func TestBR_FEE_008_ReceivableEntry(t *testing.T) {
	e := receivableEntry("0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", 42, 525050, 0, day(2026, time.August, 3))
	if e.SourceType != "challan.released" || e.SourceID == "" || e.Memo == "" {
		t.Fatalf("header: %+v", e)
	}
	if len(e.Lines) != 2 {
		t.Fatalf("want two lines, got %+v", e.Lines)
	}
	dr, cr := e.Lines[0], e.Lines[1]
	if dr.AccountCode != "1200" || dr.Debit != 525050 || dr.Credit != 0 {
		t.Fatalf("debit line: %+v", dr)
	}
	if cr.AccountCode != "4000" || cr.Credit != 525050 || cr.Debit != 0 {
		t.Fatalf("credit line: %+v", cr)
	}
}

func TestREQ_FEE_004_ValidateVoidReason(t *testing.T) {
	if r, err := validateVoidReason("  duplicate  "); err != nil || r != "duplicate" {
		t.Fatalf("got %q %v", r, err)
	}
	for _, bad := range []string{"", "   "} {
		if _, err := validateVoidReason(bad); err == nil {
			t.Fatalf("%q must be refused", bad)
		}
	}
	long := make([]byte, 301)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := validateVoidReason(string(long)); err == nil {
		t.Fatal("301 characters must be refused")
	}
	if _, err := validateVoidReason(string(long[:300])); err != nil {
		t.Fatalf("300 characters are allowed: %v", err)
	}
}

// Owner decision 2026-11-15: past months are allowed (backfill); a month more than one ahead is not.
func TestREQ_FEE_003_GenerateMonthBounds(t *testing.T) {
	now := day(2026, time.August, 19)
	cases := []struct {
		name  string
		month time.Time
		ok    bool
	}{
		{"this month", month(2026, time.August), true},
		{"next month", month(2026, time.September), true},
		{"two months ahead", month(2026, time.October), false},
		{"far future", month(2031, time.January), false},
		{"last month (backfill)", month(2026, time.July), true},
		{"years ago (backfill)", month(2020, time.January), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := validateGenerate(GenerateInput{Month: c.month}, now)
			if c.ok && err != nil {
				t.Fatalf("must be allowed: %v", err)
			}
			if !c.ok {
				if err == nil {
					t.Fatal("must be refused")
				}
				if f := fieldsOf(err); len(f) != 1 || f[0] != "month" {
					t.Fatalf("the month field must be named, got %v", f)
				}
			}
		})
	}
	// "Next month" is decided by the clock in Pakistan: at 00:30 on 1 September there it is still 31 August in UTC.
	late := time.Date(2026, time.August, 31, 20, 0, 0, 0, time.UTC) // 01:00 on 1 September in Karachi
	if _, _, err := validateGenerate(GenerateInput{Month: month(2026, time.October)}, late.In(pakistanZone)); err != nil {
		t.Fatalf("on 1 September in Pakistan, October is next month: %v", err)
	}
}
