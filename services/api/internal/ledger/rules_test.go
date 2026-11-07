package ledger

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

var day = time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)

// validEntry is a balanced two-line entry: a fee is billed (receivable up, income up).
func validEntry() Entry {
	return Entry{
		OccurredOn: day,
		Memo:       "April tuition, Ali Khan",
		SourceType: "challan.released",
		Lines: []Line{
			{AccountCode: AccountFeeReceivable, Debit: 500000},
			{AccountCode: AccountFeeIncome, Credit: 500000},
		},
	}
}

func codeOf(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Code
	}
	return ""
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

func TestADR_0004_ABalancedEntryIsAccepted(t *testing.T) {
	if err := validateEntry(validEntry()); err != nil {
		t.Fatalf("balanced entry rejected: %v", err)
	}
}

// ADR-0004 / BR-FEE-008: total debits must equal total credits, to the paisa.
func TestADR_0004_EntryMustBalance(t *testing.T) {
	tests := []struct {
		name  string
		lines []Line
		want  string // "" means valid
	}{
		{name: "equal", lines: []Line{{AccountCode: "1000", Debit: 100}, {AccountCode: "4000", Credit: 100}}},
		{name: "one paisa short", lines: []Line{{AccountCode: "1000", Debit: 100}, {AccountCode: "4000", Credit: 99}}, want: "ledger.unbalanced"},
		{name: "one paisa over", lines: []Line{{AccountCode: "1000", Debit: 100}, {AccountCode: "4000", Credit: 101}}, want: "ledger.unbalanced"},
		{name: "split across accounts", lines: []Line{
			{AccountCode: "1000", Debit: 700},
			{AccountCode: "1100", Debit: 300},
			{AccountCode: "1200", Credit: 1000},
		}},
		{name: "many lines still balance", lines: []Line{
			{AccountCode: "1200", Debit: 1000},
			{AccountCode: "5000", Debit: 200},
			{AccountCode: "4000", Credit: 1200},
		}},
		{name: "debits only", lines: []Line{{AccountCode: "1000", Debit: 100}, {AccountCode: "1100", Debit: 100}}, want: "ledger.unbalanced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEntry()
			e.Lines = tt.lines
			err := validateEntry(e)
			if got := codeOf(err); got != tt.want {
				t.Fatalf("code = %q, want %q (err %v)", got, tt.want, err)
			}
		})
	}
}

// A line is a debit or a credit, never both and never neither.
func TestADR_0004_ALineHasExactlyOneSide(t *testing.T) {
	tests := []struct {
		name string
		line Line
	}{
		{name: "both sides", line: Line{AccountCode: "1000", Debit: 50, Credit: 50}},
		{name: "neither side", line: Line{AccountCode: "1000"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEntry()
			e.Lines = append([]Line{tt.line}, e.Lines...)
			err := validateEntry(e)
			if !slices.Contains(fieldsOf(err), "lines[0]") {
				t.Fatalf("want an error on lines[0], got %v", err)
			}
		})
	}
}

func TestADR_0004_NegativeAmountsAreRefused(t *testing.T) {
	e := validEntry()
	e.Lines = []Line{
		{AccountCode: "1000", Debit: -100},
		{AccountCode: "4000", Credit: 100},
	}
	err := validateEntry(e)
	if !slices.Contains(fieldsOf(err), "lines[0]") {
		t.Fatalf("want an error on lines[0], got %v", err)
	}
}

func TestADR_0004_AnEntryNeedsAtLeastTwoLines(t *testing.T) {
	for _, n := range []int{0, 1} {
		e := validEntry()
		e.Lines = e.Lines[:n]
		if got := codeOf(validateEntry(e)); got != "ledger.entry_invalid" {
			t.Fatalf("%d lines: code = %q, want ledger.entry_invalid", n, got)
		}
	}
}

func TestADR_0004_TotalsThatOverflowAreRefused(t *testing.T) {
	e := validEntry()
	e.Lines = []Line{
		{AccountCode: "1000", Debit: money.Money(math.MaxInt64)},
		{AccountCode: "1100", Debit: 1},
		{AccountCode: "4000", Credit: money.Money(math.MaxInt64)},
	}
	if got := codeOf(validateEntry(e)); got != "ledger.entry_invalid" {
		t.Fatalf("code = %q, want ledger.entry_invalid (a wrapped total could look balanced)", got)
	}
}

func TestREQ_FEE_EntryHeaderRules(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Entry)
		wantField string
	}{
		{name: "memo required", mutate: func(e *Entry) { e.Memo = "  " }, wantField: "memo"},
		{name: "memo too long", mutate: func(e *Entry) { e.Memo = strings.Repeat("x", 301) }, wantField: "memo"},
		{name: "date required", mutate: func(e *Entry) { e.OccurredOn = time.Time{} }, wantField: "occurred_on"},
		{name: "source type required", mutate: func(e *Entry) { e.SourceType = "" }, wantField: "source_type"},
		{name: "source type shape", mutate: func(e *Entry) { e.SourceType = "Challan Released" }, wantField: "source_type"},
		{name: "source id must be a uuid", mutate: func(e *Entry) { e.SourceID = "nope" }, wantField: "source_id"},
		{name: "account code shape", mutate: func(e *Entry) { e.Lines[0].AccountCode = "cash" }, wantField: "lines[0]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEntry()
			e.Lines = slices.Clone(e.Lines)
			tt.mutate(&e)
			err := validateEntry(e)
			if !slices.Contains(fieldsOf(err), tt.wantField) {
				t.Fatalf("fields = %v, want %q (err %v)", fieldsOf(err), tt.wantField, err)
			}
		})
	}
}

// A valid source id is accepted.
func TestREQ_FEE_SourceIDIsOptionalButMustBeAUUID(t *testing.T) {
	e := validEntry()
	e.SourceID = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	if err := validateEntry(e); err != nil {
		t.Fatalf("a valid source id was rejected: %v", err)
	}
}

// The reversal of an entry swaps the sides of every line and keeps the amounts.
func TestADR_0004_ReversalSwapsEverySide(t *testing.T) {
	orig := []Line{
		{AccountCode: "1200", Debit: 1000},
		{AccountCode: "5000", Debit: 200},
		{AccountCode: "4000", Credit: 1200},
	}
	rev := reverseLines(orig)
	want := []Line{
		{AccountCode: "1200", Credit: 1000},
		{AccountCode: "5000", Credit: 200},
		{AccountCode: "4000", Debit: 1200},
	}
	if !slices.Equal(rev, want) {
		t.Fatalf("reversed = %+v, want %+v", rev, want)
	}
	e := validEntry()
	e.Lines = rev
	if err := validateEntry(e); err != nil {
		t.Fatalf("a reversed entry must itself be valid: %v", err)
	}
}
