package org

import (
	"slices"
	"testing"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
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

func has(list []string, want string) bool {
	return slices.Contains(list, want)
}

// REQ-ORG-006: a session has a name and a start before its end.
func TestREQ_ORG_006_SessionRules(t *testing.T) {
	tests := []struct {
		name      string
		in        SessionInput
		wantField string // "" means valid
	}{
		{name: "valid", in: SessionInput{Name: "2026-27", StartsOn: date(2026, 4, 1), EndsOn: date(2027, 3, 31)}},
		{name: "name is trimmed and required", in: SessionInput{Name: "  ", StartsOn: date(2026, 4, 1), EndsOn: date(2027, 3, 31)}, wantField: "name"},
		{name: "name too long", in: SessionInput{Name: "this session name is far too long to be a sensible label", StartsOn: date(2026, 4, 1), EndsOn: date(2027, 3, 31)}, wantField: "name"},
		{name: "ends before it starts", in: SessionInput{Name: "bad", StartsOn: date(2027, 3, 31), EndsOn: date(2026, 4, 1)}, wantField: "ends_on"},
		{name: "ends the day it starts", in: SessionInput{Name: "bad", StartsOn: date(2026, 4, 1), EndsOn: date(2026, 4, 1)}, wantField: "ends_on"},
		{name: "missing dates", in: SessionInput{Name: "no dates"}, wantField: "starts_on"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateSession(tt.in)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("valid session rejected: %v", err)
				}
				if got.Name != "2026-27" {
					t.Fatalf("name = %q", got.Name)
				}
				return
			}
			if !has(fieldsOf(err), tt.wantField) {
				t.Fatalf("fields = %v, want %q", fieldsOf(err), tt.wantField)
			}
		})
	}
}

// REQ-ORG-008: a class has a name and a numeric grade level for ordering and promotion.
func TestREQ_ORG_008_ClassRules(t *testing.T) {
	tests := []struct {
		name      string
		in        ClassInput
		wantField string
	}{
		{name: "valid", in: ClassInput{Name: "Class 5", GradeLevel: 5}},
		{name: "early years is grade 0", in: ClassInput{Name: "Nursery", GradeLevel: 0}},
		{name: "name required", in: ClassInput{Name: "", GradeLevel: 5}, wantField: "name"},
		{name: "grade below range", in: ClassInput{Name: "Class X", GradeLevel: -1}, wantField: "grade_level"},
		{name: "grade above range", in: ClassInput{Name: "Class X", GradeLevel: 17}, wantField: "grade_level"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateClass(tt.in)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("valid class rejected: %v", err)
				}
				return
			}
			if !has(fieldsOf(err), tt.wantField) {
				t.Fatalf("fields = %v, want %q", fieldsOf(err), tt.wantField)
			}
		})
	}
}

// REQ-ORG-005: a shift is a property of a section; only the known shifts are allowed.
func TestREQ_ORG_005_SectionRules(t *testing.T) {
	const classID = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
	tests := []struct {
		name      string
		in        SectionInput
		wantField string
	}{
		{name: "morning", in: SectionInput{ClassID: classID, Name: "A", Shift: "morning"}},
		{name: "evening", in: SectionInput{ClassID: classID, Name: "A", Shift: "evening"}},
		{name: "unknown shift", in: SectionInput{ClassID: classID, Name: "A", Shift: "night"}, wantField: "shift"},
		{name: "name required", in: SectionInput{ClassID: classID, Name: " ", Shift: "morning"}, wantField: "name"},
		{name: "class id must be a uuid", in: SectionInput{ClassID: "not-a-uuid", Name: "A", Shift: "morning"}, wantField: "class_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateSection(tt.in)
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("valid section rejected: %v", err)
				}
				return
			}
			if !has(fieldsOf(err), tt.wantField) {
				t.Fatalf("fields = %v, want %q", fieldsOf(err), tt.wantField)
			}
		})
	}
}
