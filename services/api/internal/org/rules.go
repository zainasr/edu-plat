// Package org holds the academic structure of a school: branches, sessions, classes and
// sections (organisation spec §1, §2). Other modules (students) refer to its rows by id and
// ask this package to describe them; they never read its tables.
package org

import (
	"strings"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

const (
	maxSessionName = 40
	maxClassName   = 40
	maxSectionName = 20
	minGradeLevel  = 0
	maxGradeLevel  = 16
)

// Shifts are a property of a section (REQ-ORG-005).
var shifts = map[string]bool{"morning": true, "evening": true}

// SessionInput is a request to create an academic session.
type SessionInput struct {
	Name     string
	StartsOn time.Time
	EndsOn   time.Time
}

// ClassInput is a request to create a class.
type ClassInput struct {
	Name       string
	GradeLevel int
}

// SectionInput is a request to create a section.
type SectionInput struct {
	ClassID string
	Name    string
	Shift   string
}

// fieldErrors collects problems so a caller sees all of them at once.
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

// validateSession trims the name and checks the dates (REQ-ORG-006).
func validateSession(in SessionInput) (SessionInput, error) {
	var f fieldErrors
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > maxSessionName {
		f.add("name", "Required, up to 40 characters.")
	}
	switch {
	case in.StartsOn.IsZero():
		f.add("starts_on", "Required.")
	case in.EndsOn.IsZero():
		f.add("ends_on", "Required.")
	case !in.EndsOn.After(in.StartsOn):
		f.add("ends_on", "Must be after the start date.")
	}
	return in, f.err("org.session_invalid")
}

// validateClass trims the name and checks the grade level (REQ-ORG-008).
func validateClass(in ClassInput) (ClassInput, error) {
	var f fieldErrors
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > maxClassName {
		f.add("name", "Required, up to 40 characters.")
	}
	if in.GradeLevel < minGradeLevel || in.GradeLevel > maxGradeLevel {
		f.add("grade_level", "Between 0 (early years) and 16.")
	}
	return in, f.err("org.class_invalid")
}

// validateSection trims the name and checks the class id and the shift (REQ-ORG-005).
func validateSection(in SectionInput) (SectionInput, error) {
	var f fieldErrors
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > maxSectionName {
		f.add("name", "Required, up to 20 characters.")
	}
	if _, err := db.ParseUUID(in.ClassID); err != nil {
		f.add("class_id", "Must be a class id.")
	}
	if !shifts[in.Shift] {
		f.add("shift", "Use morning or evening.")
	}
	return in, f.err("org.section_invalid")
}
