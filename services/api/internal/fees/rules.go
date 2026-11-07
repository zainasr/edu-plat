// Package fees holds fee setup: fee types, class fee plans and student overrides (fees spec §2, §4.1).
// Plan lines and overrides are effective-dated, append-only versions, so a change never alters the
// past (BR-FEE-001). This slice moves no money and does not touch the ledger; challan generation
// (the next slice) reads the fee structure this package resolves.
package fees

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/money"
)

const (
	maxFeeName   = 60
	minReasonLen = 3
	maxReasonLen = 200
)

var (
	codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,29}$`)

	kinds = map[string]bool{
		"tuition": true, "admission": true, "registration": true, "annual": true, "transport": true,
		"lab": true, "library": true, "sports": true, "fine": true, "security_deposit": true, "custom": true,
	}
	// Quarterly and term frequencies are 1.1 (REQ-FEE-002).
	frequencies = map[string]bool{"monthly": true, "one_time": true, "annual": true, "ad_hoc": true}
)

// monthStart returns the first day of t's month at 00:00 UTC. Plan versions are dated by month.
func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// ---- inputs ---------------------------------------------------------------------------

// FeeTypeInput is a request to add a fee type.
type FeeTypeInput struct {
	Code string
	Name string
	Kind string
}

// PlanLineInput sets, or removes, one fee type on a class plan from a month.
type PlanLineInput struct {
	FeeTypeID     string
	EffectiveFrom time.Time
	Remove        bool
	Amount        *money.Money // required unless Remove
	Frequency     string       // required unless Remove
}

// OverrideInput sets, or clears, a student's amount for one fee type from a month.
type OverrideInput struct {
	FeeTypeID     string
	EffectiveFrom time.Time
	Reason        string
	Clear         bool
	Amount        *money.Money // required unless Clear; zero is a full concession
}

type fieldErrors []apperr.Field

func (f *fieldErrors) add(field, msg string) {
	*f = append(*f, apperr.Field{Field: field, Message: msg})
}

func (f fieldErrors) err(code string) error {
	if len(f) == 0 {
		return nil
	}
	return apperr.Validation(code, "Some fields need attention.", f...)
}

func validateFeeType(in FeeTypeInput) (FeeTypeInput, error) {
	var f fieldErrors
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > maxFeeName {
		f.add("name", "Required, up to 60 characters.")
	}
	if !codePattern.MatchString(in.Code) {
		f.add("code", "Lower-case letters, digits and underscores, 2 to 30 characters, starting with a letter.")
	}
	if !kinds[in.Kind] {
		f.add("kind", "Choose a fee kind from the list.")
	}
	return in, f.err("fees.type_invalid")
}

// validatePlanLine checks a plan change and normalises the month. A removal carries no amount.
func validatePlanLine(in PlanLineInput) (PlanLineInput, error) {
	var f fieldErrors
	if _, err := db.ParseUUID(in.FeeTypeID); err != nil {
		f.add("fee_type_id", "Must be a fee type id.")
	}
	if in.EffectiveFrom.IsZero() {
		f.add("effective_from", "Required: any date in the month the change starts.")
	} else {
		in.EffectiveFrom = monthStart(in.EffectiveFrom)
	}
	switch {
	case in.Remove:
		if in.Amount != nil {
			f.add("amount", "Do not send an amount when removing a fee type.")
		}
	default:
		if in.Amount == nil || *in.Amount <= 0 {
			f.add("amount", "Required, above zero. Amounts are whole paisa.")
		}
		if !frequencies[in.Frequency] {
			f.add("frequency", "Use monthly, one_time, annual or ad_hoc.")
		}
	}
	return in, f.err("fees.plan_line_invalid")
}

// validateOverride checks a student override and normalises the month.
func validateOverride(in OverrideInput) (OverrideInput, error) {
	var f fieldErrors
	if _, err := db.ParseUUID(in.FeeTypeID); err != nil {
		f.add("fee_type_id", "Must be a fee type id.")
	}
	if in.EffectiveFrom.IsZero() {
		f.add("effective_from", "Required: any date in the month the change starts.")
	} else {
		in.EffectiveFrom = monthStart(in.EffectiveFrom)
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < minReasonLen || len(in.Reason) > maxReasonLen {
		f.add("reason", "3 to 200 characters.")
	}
	switch {
	case in.Clear:
		if in.Amount != nil {
			f.add("amount", "Do not send an amount when clearing an override.")
		}
	default:
		if in.Amount == nil || *in.Amount < 0 {
			f.add("amount", "Required, zero or more. Zero is a full concession.")
		}
	}
	return in, f.err("fees.override_invalid")
}

// ---- resolution -----------------------------------------------------------------------

// TypeInfo is what the resolver needs to know about a fee type.
type TypeInfo struct {
	Name     string
	Kind     string
	Archived bool
}

// PlanVersion is one stored version of a class plan line.
type PlanVersion struct {
	FeeTypeID     string
	Name          string // filled in by resolvePlan
	Kind          string // filled in by resolvePlan
	Amount        money.Money
	Frequency     string
	EffectiveFrom time.Time
	Removed       bool
}

// OverrideVersion is one stored version of a student override.
type OverrideVersion struct {
	FeeTypeID     string
	Amount        money.Money
	EffectiveFrom time.Time
	Cleared       bool
}

// Line is one charge in a student's fee structure.
type Line struct {
	FeeTypeID  string
	Name       string
	Kind       string
	Amount     money.Money // what the student is charged
	PlanAmount money.Money // what the class plan says, always shown
	Frequency  string
	Source     string // "plan" or "override"
}

// resolvePlan returns the class plan as it stands in a month: for each fee type, the latest version
// on or before the month, leaving out removed and archived types. Ordered by name, then id. It is a
// pure function of its inputs, which is what makes BR-FEE-001 testable: a later version cannot change
// the answer for an earlier month.
func resolvePlan(month time.Time, versions []PlanVersion, types map[string]TypeInfo) []PlanVersion {
	month = monthStart(month)
	latest := make(map[string]PlanVersion)
	for _, v := range versions {
		if v.EffectiveFrom.After(month) {
			continue
		}
		if cur, ok := latest[v.FeeTypeID]; !ok || v.EffectiveFrom.After(cur.EffectiveFrom) {
			latest[v.FeeTypeID] = v
		}
	}
	out := make([]PlanVersion, 0, len(latest))
	for id, v := range latest {
		info, known := types[id]
		if v.Removed || !known || info.Archived {
			continue
		}
		v.Name, v.Kind = info.Name, info.Kind
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].FeeTypeID < out[j].FeeTypeID
	})
	return out
}

// resolveStructure lays a student's overrides over a resolved plan. For each plan line, the latest
// override on or before the month replaces the amount, unless that version is a "cleared" one. An
// override for a fee type that is not on the plan changes nothing: it is not a charge.
func resolveStructure(month time.Time, plan []PlanVersion, overrides []OverrideVersion) []Line {
	month = monthStart(month)
	latest := make(map[string]OverrideVersion)
	for _, o := range overrides {
		if o.EffectiveFrom.After(month) {
			continue
		}
		if cur, ok := latest[o.FeeTypeID]; !ok || o.EffectiveFrom.After(cur.EffectiveFrom) {
			latest[o.FeeTypeID] = o
		}
	}
	lines := make([]Line, 0, len(plan))
	for _, p := range plan {
		line := Line{
			FeeTypeID: p.FeeTypeID, Name: p.Name, Kind: p.Kind,
			Amount: p.Amount, PlanAmount: p.Amount, Frequency: p.Frequency, Source: "plan",
		}
		if o, ok := latest[p.FeeTypeID]; ok && !o.Cleared {
			line.Amount, line.Source = o.Amount, "override"
		}
		lines = append(lines, line)
	}
	return lines
}
