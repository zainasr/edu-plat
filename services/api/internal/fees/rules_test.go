package fees

import (
	"slices"
	"testing"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

func month(y int, m time.Month) time.Time { return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC) }

func amountp(m money.Money) *money.Money { return &m }

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

var types = map[string]TypeInfo{
	"tuition":   {Name: "Tuition", Kind: "tuition"},
	"transport": {Name: "Transport", Kind: "transport"},
	"lab":       {Name: "Lab Fee", Kind: "lab", Archived: true},
}

func ids(lines []PlanVersion) []string {
	var out []string
	for _, l := range lines {
		out = append(out, l.FeeTypeID)
	}
	return out
}

func TestREQ_FEE_001_MonthIsNormalisedToTheFirstDay(t *testing.T) {
	got := monthStart(time.Date(2026, 9, 17, 14, 30, 0, 0, time.FixedZone("PKT", 5*3600)))
	if !got.Equal(month(2026, time.September)) {
		t.Fatalf("monthStart = %v, want 2026-09-01", got)
	}
}

// REQ-FEE-001: the amount for a month is the latest version on or before that month.
func TestREQ_FEE_001_PlanUsesTheLatestVersionOnOrBeforeTheMonth(t *testing.T) {
	versions := []PlanVersion{
		{FeeTypeID: "tuition", Amount: 500000, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
		{FeeTypeID: "tuition", Amount: 600000, Frequency: "monthly", EffectiveFrom: month(2026, time.September)},
	}
	tests := []struct {
		name string
		in   time.Time
		want money.Money // 0 means "not on the plan"
	}{
		{"before the first version", month(2026, time.March), 0},
		{"the month of the first version", month(2026, time.April), 500000},
		{"between versions", month(2026, time.August), 500000},
		{"the month of the second version", month(2026, time.September), 600000},
		{"long after", month(2027, time.January), 600000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePlan(tt.in, versions, types)
			if tt.want == 0 {
				if len(got) != 0 {
					t.Fatalf("want no lines, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Amount != tt.want {
				t.Fatalf("got %+v, want amount %d", got, tt.want)
			}
		})
	}
}

// BR-FEE-001: adding a later version never changes what an earlier month resolved to.
func TestBR_FEE_001_ANewVersionDoesNotChangeEarlierMonths(t *testing.T) {
	versions := []PlanVersion{
		{FeeTypeID: "tuition", Amount: 500000, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
	}
	before := resolvePlan(month(2026, time.June), versions, types)

	versions = append(versions, PlanVersion{
		FeeTypeID: "tuition", Amount: 900000, Frequency: "monthly", EffectiveFrom: month(2026, time.October),
	})
	after := resolvePlan(month(2026, time.June), versions, types)

	if !slices.Equal(before, after) {
		t.Fatalf("June changed after an October version was added: %+v -> %+v", before, after)
	}
	if got := resolvePlan(month(2026, time.October), versions, types); got[0].Amount != 900000 {
		t.Fatalf("October should use the new amount, got %+v", got)
	}
}

// A removal version takes the fee off the plan from that month; a later version can bring it back.
func TestREQ_FEE_001_RemovalAndReinstatement(t *testing.T) {
	versions := []PlanVersion{
		{FeeTypeID: "transport", Amount: 200000, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
		{FeeTypeID: "transport", Removed: true, Frequency: "monthly", EffectiveFrom: month(2026, time.July)},
		{FeeTypeID: "transport", Amount: 250000, Frequency: "monthly", EffectiveFrom: month(2026, time.October)},
	}
	want := map[time.Month]int{time.May: 1, time.June: 1, time.July: 0, time.August: 0, time.October: 1}
	for m, n := range want {
		if got := resolvePlan(month(2026, m), versions, types); len(got) != n {
			t.Errorf("%v: %d lines, want %d", m, len(got), n)
		}
	}
	if got := resolvePlan(month(2026, time.November), versions, types); got[0].Amount != 250000 {
		t.Fatalf("November should use the reinstated amount, got %+v", got)
	}
}

// An archived fee type stops being charged from now on, whatever the plan says.
func TestREQ_FEE_ArchivedTypesDropOutOfThePlan(t *testing.T) {
	versions := []PlanVersion{
		{FeeTypeID: "tuition", Amount: 500000, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
		{FeeTypeID: "lab", Amount: 50000, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
	}
	got := resolvePlan(month(2026, time.May), versions, types)
	if !slices.Equal(ids(got), []string{"tuition"}) {
		t.Fatalf("lines = %v, want only tuition (lab is archived)", ids(got))
	}
}

// The result is ordered by fee type name, so lists and tests are stable.
func TestREQ_FEE_PlanIsOrderedByName(t *testing.T) {
	versions := []PlanVersion{
		{FeeTypeID: "transport", Amount: 1, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
		{FeeTypeID: "tuition", Amount: 1, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
	}
	got := resolvePlan(month(2026, time.May), versions, types)
	if !slices.Equal(ids(got), []string{"transport", "tuition"}) { // Transport < Tuition
		t.Fatalf("order = %v", ids(got))
	}
}

// REQ-FEE-001: a student override replaces the plan amount; a zero override is a full concession.
func TestREQ_FEE_001_OverrideReplacesThePlanAmount(t *testing.T) {
	plan := resolvePlan(month(2026, time.May), []PlanVersion{
		{FeeTypeID: "tuition", Amount: 500000, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
	}, types)

	tests := []struct {
		name       string
		overrides  []OverrideVersion
		wantAmount money.Money
		wantSource string
	}{
		{"no override", nil, 500000, "plan"},
		{"override applies", []OverrideVersion{{FeeTypeID: "tuition", Amount: 400000, EffectiveFrom: month(2026, time.April)}}, 400000, "override"},
		{"zero is a full concession", []OverrideVersion{{FeeTypeID: "tuition", Amount: 0, EffectiveFrom: month(2026, time.April)}}, 0, "override"},
		{"override starts later", []OverrideVersion{{FeeTypeID: "tuition", Amount: 400000, EffectiveFrom: month(2026, time.September)}}, 500000, "plan"},
		{"latest override wins", []OverrideVersion{
			{FeeTypeID: "tuition", Amount: 400000, EffectiveFrom: month(2026, time.April)},
			{FeeTypeID: "tuition", Amount: 300000, EffectiveFrom: month(2026, time.May)},
		}, 300000, "override"},
		{"a cleared version returns to the plan", []OverrideVersion{
			{FeeTypeID: "tuition", Amount: 400000, EffectiveFrom: month(2026, time.April)},
			{FeeTypeID: "tuition", Cleared: true, EffectiveFrom: month(2026, time.May)},
		}, 500000, "plan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := resolveStructure(month(2026, time.May), plan, tt.overrides)
			if len(lines) != 1 || lines[0].Amount != tt.wantAmount || lines[0].Source != tt.wantSource {
				t.Fatalf("got %+v, want amount %d from %s", lines, tt.wantAmount, tt.wantSource)
			}
			if lines[0].PlanAmount != 500000 {
				t.Fatalf("plan amount should always be shown, got %d", lines[0].PlanAmount)
			}
		})
	}
}

// An override for a fee type that is not on the plan is ignored, not turned into a charge.
func TestREQ_FEE_001_OverrideWithoutAPlanLineIsIgnored(t *testing.T) {
	plan := resolvePlan(month(2026, time.May), []PlanVersion{
		{FeeTypeID: "tuition", Amount: 500000, Frequency: "monthly", EffectiveFrom: month(2026, time.April)},
	}, types)
	lines := resolveStructure(month(2026, time.May), plan, []OverrideVersion{
		{FeeTypeID: "transport", Amount: 1, EffectiveFrom: month(2026, time.April)},
	})
	if len(lines) != 1 || lines[0].FeeTypeID != "tuition" {
		t.Fatalf("got %+v, want only tuition", lines)
	}
}

func TestREQ_FEE_FeeTypeRules(t *testing.T) {
	tests := []struct {
		name      string
		in        FeeTypeInput
		wantField string
	}{
		{"valid", FeeTypeInput{Code: "lab_fee", Name: "Lab Fee", Kind: "lab"}, ""},
		{"deposit kind", FeeTypeInput{Code: "deposit", Name: "Security Deposit", Kind: "security_deposit"}, ""},
		{"code with capitals", FeeTypeInput{Code: "Lab", Name: "Lab", Kind: "lab"}, "code"},
		{"code too short", FeeTypeInput{Code: "x", Name: "X", Kind: "lab"}, "code"},
		{"name blank", FeeTypeInput{Code: "lab_fee", Name: "  ", Kind: "lab"}, "name"},
		{"unknown kind", FeeTypeInput{Code: "lab_fee", Name: "Lab", Kind: "gold"}, "kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateFeeType(tt.in)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("valid type rejected: %v", err)
				}
				if got.Name != tt.in.Name {
					t.Fatalf("name = %q", got.Name)
				}
				return
			}
			if !slices.Contains(fieldsOf(err), tt.wantField) {
				t.Fatalf("fields = %v, want %q", fieldsOf(err), tt.wantField)
			}
		})
	}
}

func TestREQ_FEE_PlanLineRules(t *testing.T) {
	const typeID = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	apr := time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		in        PlanLineInput
		wantField string
	}{
		{"valid", PlanLineInput{FeeTypeID: typeID, EffectiveFrom: apr, Amount: amountp(500000), Frequency: "monthly"}, ""},
		{"valid removal", PlanLineInput{FeeTypeID: typeID, EffectiveFrom: apr, Remove: true}, ""},
		{"fee type must be an id", PlanLineInput{FeeTypeID: "tuition", EffectiveFrom: apr, Amount: amountp(1), Frequency: "monthly"}, "fee_type_id"},
		{"month required", PlanLineInput{FeeTypeID: typeID, Amount: amountp(1), Frequency: "monthly"}, "effective_from"},
		{"amount required", PlanLineInput{FeeTypeID: typeID, EffectiveFrom: apr, Frequency: "monthly"}, "amount"},
		{"amount must be positive", PlanLineInput{FeeTypeID: typeID, EffectiveFrom: apr, Amount: amountp(0), Frequency: "monthly"}, "amount"},
		{"negative amount", PlanLineInput{FeeTypeID: typeID, EffectiveFrom: apr, Amount: amountp(-5), Frequency: "monthly"}, "amount"},
		{"unknown frequency", PlanLineInput{FeeTypeID: typeID, EffectiveFrom: apr, Amount: amountp(1), Frequency: "weekly"}, "frequency"},
		{"removal takes no amount", PlanLineInput{FeeTypeID: typeID, EffectiveFrom: apr, Remove: true, Amount: amountp(5)}, "amount"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validatePlanLine(tt.in)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("valid line rejected: %v", err)
				}
				if !got.EffectiveFrom.Equal(month(2026, time.April)) {
					t.Fatalf("effective_from = %v, want the first of the month", got.EffectiveFrom)
				}
				return
			}
			if !slices.Contains(fieldsOf(err), tt.wantField) {
				t.Fatalf("fields = %v, want %q", fieldsOf(err), tt.wantField)
			}
		})
	}
}

func TestREQ_FEE_OverrideRules(t *testing.T) {
	const typeID = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	apr := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		in        OverrideInput
		wantField string
	}{
		{"valid", OverrideInput{FeeTypeID: typeID, EffectiveFrom: apr, Reason: "sibling concession", Amount: amountp(400000)}, ""},
		{"zero is allowed", OverrideInput{FeeTypeID: typeID, EffectiveFrom: apr, Reason: "scholarship", Amount: amountp(0)}, ""},
		{"valid clear", OverrideInput{FeeTypeID: typeID, EffectiveFrom: apr, Reason: "back to normal", Clear: true}, ""},
		{"reason required", OverrideInput{FeeTypeID: typeID, EffectiveFrom: apr, Reason: "no", Amount: amountp(1)}, "reason"},
		{"amount required", OverrideInput{FeeTypeID: typeID, EffectiveFrom: apr, Reason: "because"}, "amount"},
		{"negative amount", OverrideInput{FeeTypeID: typeID, EffectiveFrom: apr, Reason: "because", Amount: amountp(-1)}, "amount"},
		{"clear takes no amount", OverrideInput{FeeTypeID: typeID, EffectiveFrom: apr, Reason: "because", Clear: true, Amount: amountp(1)}, "amount"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateOverride(tt.in)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("valid override rejected: %v", err)
				}
				return
			}
			if !slices.Contains(fieldsOf(err), tt.wantField) {
				t.Fatalf("fields = %v, want %q", fieldsOf(err), tt.wantField)
			}
		})
	}
}
