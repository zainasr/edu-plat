package students

import (
	"errors"
	"testing"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

var today = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)

func validStudent() StudentInput {
	return StudentInput{
		SectionID:   "5c9c6c26-e0e4-4938-af5c-f6fc35688270",
		FullName:    "Ali Khan",
		FatherName:  "Imran Khan",
		Gender:      "male",
		DateOfBirth: time.Date(2018, 4, 12, 0, 0, 0, 0, time.UTC),
		Guardians: []GuardianInput{
			{FullName: "Kamran Khan", Mobile: "03001234567", Relationship: "uncle", Primary: true},
		},
	}
}

func boolp(b bool) *bool { return &b }

// fieldsOf returns the field names of an apperr.Validation, or nil when err is not one.
func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
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

func TestREQ_STU_001_MandatoryFieldsAreChecked(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*StudentInput)
		wantField string
	}{
		{name: "missing full name", mutate: func(in *StudentInput) { in.FullName = " " }, wantField: "full_name"},
		{name: "missing father name", mutate: func(in *StudentInput) { in.FatherName = "" }, wantField: "father_name"},
		{name: "unknown gender", mutate: func(in *StudentInput) { in.Gender = "unknown" }, wantField: "gender"},
		{name: "missing section", mutate: func(in *StudentInput) { in.SectionID = "" }, wantField: "section_id"},
		{name: "section is not an id", mutate: func(in *StudentInput) { in.SectionID = "5-A" }, wantField: "section_id"},
		{name: "date of birth in the future", mutate: func(in *StudentInput) { in.DateOfBirth = today.AddDate(0, 0, 1) }, wantField: "date_of_birth"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validStudent()
			tt.mutate(&in)
			_, err := validateEnrolment(in, today)
			if err == nil {
				t.Fatalf("expected a validation error on %s", tt.wantField)
			}
			if got := fieldsOf(t, err); !contains(got, tt.wantField) {
				t.Fatalf("fields = %v, want to include %q", got, tt.wantField)
			}
		})
	}
}

func TestREQ_STU_001_ValidEnrolmentPasses(t *testing.T) {
	if _, err := validateEnrolment(validStudent(), today); err != nil {
		t.Fatalf("valid enrolment rejected: %v", err)
	}
}

// US-STU-006: the father is not required to be a guardian.
func TestUS_STU_006_FatherNeedNotBeAGuardian(t *testing.T) {
	in := validStudent()
	in.FatherName = "Imran Khan"
	in.Guardians[0].Relationship = "uncle"
	if _, err := validateEnrolment(in, today); err != nil {
		t.Fatalf("uncle-only guardian rejected: %v", err)
	}
}

func TestREQ_STU_002_RelationshipMustBeKnown(t *testing.T) {
	in := validStudent()
	in.Guardians[0].Relationship = "neighbour"
	_, err := validateEnrolment(in, today)
	if !contains(fieldsOf(t, err), "guardians[0].relationship") {
		t.Fatalf("expected relationship error, got %v", err)
	}
}

func TestREQ_STU_002_GuardianCountIsOneToFour(t *testing.T) {
	tests := []struct {
		name  string
		count int
		ok    bool
	}{
		{name: "none", count: 0, ok: false},
		{name: "one", count: 1, ok: true},
		{name: "four", count: 4, ok: true},
		{name: "five", count: 5, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validStudent()
			in.Guardians = nil
			for i := range tt.count {
				in.Guardians = append(in.Guardians, GuardianInput{
					FullName:     "Guardian",
					Mobile:       "0300123456" + string(rune('0'+i)),
					Relationship: "other",
					Primary:      i == 0,
				})
			}
			_, err := validateEnrolment(in, today)
			if (err == nil) != tt.ok {
				t.Fatalf("count %d: err = %v, want ok=%v", tt.count, err, tt.ok)
			}
		})
	}
}

func TestBR_STU_005_ExactlyOnePrimaryGuardian(t *testing.T) {
	tests := []struct {
		name    string
		primary []bool
		ok      bool
	}{
		{name: "none primary", primary: []bool{false}, ok: false},
		{name: "one primary", primary: []bool{true, false}, ok: true},
		{name: "two primaries", primary: []bool{true, true}, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validStudent()
			in.Guardians = nil
			for i, p := range tt.primary {
				in.Guardians = append(in.Guardians, GuardianInput{
					FullName: "Guardian", Mobile: "0300123456" + string(rune('0'+i)),
					Relationship: "other", Primary: p,
				})
			}
			_, err := validateEnrolment(in, today)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

// BR-STU-006: with no fee payer chosen, the primary guardian pays.
func TestBR_STU_006_FeePayerDefaultsToPrimary(t *testing.T) {
	rules, err := validateEnrolment(validStudent(), today)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rules[0].FeePayer || !rules[0].Primary {
		t.Fatalf("primary should default to fee payer, got %+v", rules[0])
	}
}

// BR-STU-006: at most one fee payer.
func TestBR_STU_006_AtMostOneFeePayer(t *testing.T) {
	in := validStudent()
	in.Guardians = append(in.Guardians, GuardianInput{
		FullName: "Bilal's mother", Mobile: "03111234567", Relationship: "mother",
		FeePayer: boolp(true),
	})
	in.Guardians[0].FeePayer = boolp(true)
	_, err := validateEnrolment(in, today)
	if err == nil {
		t.Fatal("two fee payers must be rejected")
	}
}

// REQ-STU-015 defaults: receives notifications on, emergency off, access full.
func TestREQ_STU_015_FlagDefaults(t *testing.T) {
	rules, err := validateEnrolment(validStudent(), today)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := rules[0]
	if !r.ReceivesNotifications || r.EmergencyContact || r.Access != "full" {
		t.Fatalf("defaults wrong: %+v", r)
	}
}

// US-STU-006: a view-only guardian must not be the fee payer.
func TestUS_STU_006_ViewOnlyGuardianCannotPay(t *testing.T) {
	in := validStudent()
	in.Guardians[0].Access = "view_only"
	in.Guardians = append(in.Guardians, GuardianInput{
		FullName: "Imran Khan", Mobile: "03111234567", Relationship: "father",
		FeePayer: boolp(true), Access: "view_only",
	})
	_, err := validateEnrolment(in, today)
	if !contains(fieldsOf(t, err), "guardians[1].fee_payer") {
		t.Fatalf("expected fee payer access error, got %v", err)
	}
}

func TestREQ_STU_004_DuplicateMobileInOneRequestIsRejected(t *testing.T) {
	in := validStudent()
	in.Guardians = append(in.Guardians, GuardianInput{
		FullName: "Other", Mobile: "03001234567", Relationship: "aunt",
	})
	_, err := validateEnrolment(in, today)
	if !contains(fieldsOf(t, err), "guardians[1].mobile") {
		t.Fatalf("expected duplicate mobile error, got %v", err)
	}
}

func TestValidationErrorsAreAllReturnedAtOnce(t *testing.T) {
	in := StudentInput{}
	_, err := validateEnrolment(in, today)
	if err == nil {
		t.Fatal("empty request must fail")
	}
	var e *apperr.Error
	if !errors.As(err, &e) || len(e.Details) < 4 {
		t.Fatalf("want several field errors at once, got %v", err)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
