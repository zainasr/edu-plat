// Package students holds the student record: enrolment, guardians and family links
// (students spec §2, §5; module 01).
package students

import (
	"strconv"
	"strings"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/phone"
)

// GuardianInput is one guardian as sent by the caller. Optional flags are pointers so that
// "not sent" (use the default) differs from "false".
type GuardianInput struct {
	FullName              string
	Mobile                string
	Relationship          string
	Primary               bool
	FeePayer              *bool
	ReceivesNotifications *bool
	EmergencyContact      *bool
	Access                string
}

// StudentInput is an enrolment request.
type StudentInput struct {
	FullName    string
	FatherName  string
	Gender      string
	DateOfBirth time.Time
	Guardians   []GuardianInput
}

// guardianRule is a guardian after validation and defaults; ready to store.
type guardianRule struct {
	FullName              string
	Mobile                string // E.164
	Relationship          string
	Access                string
	Primary               bool
	FeePayer              bool
	ReceivesNotifications bool
	EmergencyContact      bool
}

const (
	maxNameLen   = 120
	maxGuardians = 4 // REQ-STU-002
)

var (
	genders = map[string]bool{"male": true, "female": true, "other": true}

	// REQ-STU-002: a guardian is any responsible adult, and each relationship is named.
	relationships = map[string]bool{
		"father": true, "mother": true, "grandfather": true, "grandmother": true,
		"uncle": true, "aunt": true, "brother": true, "sister": true, "other": true,
	}
	accessLevels = map[string]bool{"full": true, "view_only": true, "none": true}
)

// validateEnrolment checks an enrolment request and applies the defaults. It returns every
// problem at once, as field details, so the caller can fix them in one pass.
// today is passed in so the date rule is testable.
func validateEnrolment(in StudentInput, today time.Time) ([]guardianRule, error) {
	var fields []apperr.Field
	add := func(field, msg string) { fields = append(fields, apperr.Field{Field: field, Message: msg}) }

	if n := strings.TrimSpace(in.FullName); n == "" || len(n) > maxNameLen {
		add("full_name", "Required, up to 120 characters.")
	}
	if n := strings.TrimSpace(in.FatherName); n == "" || len(n) > maxNameLen {
		add("father_name", `Required, up to 120 characters. Use "Not available" when unknown (REQ-STU-001).`)
	}
	if !genders[in.Gender] {
		add("gender", "Use male, female or other.")
	}
	if in.DateOfBirth.IsZero() || in.DateOfBirth.After(today) {
		add("date_of_birth", "Required, and not in the future.")
	}

	count := len(in.Guardians)
	if count < 1 || count > maxGuardians {
		add("guardians", "Between 1 and 4 guardians are required (REQ-STU-002).")
	}

	rules := make([]guardianRule, 0, count)
	seen := make(map[string]bool, count)
	primaries, feePayers := 0, 0
	for i, g := range in.Guardians {
		field := "guardians[" + strconv.Itoa(i) + "]"
		r := guardianRule{FullName: strings.TrimSpace(g.FullName), Relationship: g.Relationship, Primary: g.Primary}

		if r.FullName == "" || len(r.FullName) > maxNameLen {
			add(field+".full_name", "Required, up to 120 characters.")
		}
		mobile, err := phone.Normalize(g.Mobile)
		if err != nil {
			add(field+".mobile", "Enter a Pakistani mobile number, such as 03001234567.")
		} else if seen[mobile] {
			add(field+".mobile", "The same mobile number appears twice.")
		} else {
			seen[mobile] = true
			r.Mobile = mobile
		}
		if !relationships[g.Relationship] {
			add(field+".relationship", "Choose a relationship from the list.")
		}
		r.Access = g.Access
		if r.Access == "" {
			r.Access = "full"
		}
		if !accessLevels[r.Access] {
			add(field+".access", "Use full, view_only or none.")
		}
		r.ReceivesNotifications = valueOr(g.ReceivesNotifications, true)
		r.EmergencyContact = valueOr(g.EmergencyContact, false)
		r.FeePayer = valueOr(g.FeePayer, false)

		if g.Primary {
			primaries++
		}
		if r.FeePayer {
			feePayers++
		}
		rules = append(rules, r)
	}

	// BR-STU-005: exactly one primary guardian (the family login).
	if count > 0 && primaries != 1 {
		add("guardians", "Exactly one guardian must be primary (BR-STU-005).")
	}
	// BR-STU-006: the fee payer is one of the linked guardians. With none chosen, the primary pays.
	switch {
	case feePayers > 1:
		add("guardians", "Only one guardian can be the fee payer (BR-STU-006).")
	case feePayers == 0:
		for i := range rules {
			rules[i].FeePayer = rules[i].Primary
		}
	}
	// A view-only guardian cannot be the fee payer (US-STU-006).
	for i, r := range rules {
		if r.FeePayer && r.Access != "full" {
			add("guardians["+strconv.Itoa(i)+"].fee_payer", "The fee payer needs full access.")
		}
	}

	if len(fields) > 0 {
		return nil, apperr.Validation("students.invalid", "Some fields need attention.", fields...)
	}
	return rules, nil
}

func valueOr(p *bool, fallback bool) bool {
	if p == nil {
		return fallback
	}
	return *p
}
