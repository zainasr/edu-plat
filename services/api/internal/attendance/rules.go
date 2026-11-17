// Package attendance holds daily attendance registers (attendance spec, slice 6a, ADR-0022). A register is
// one section's attendance for one day. Its records are versions: every mark and every correction adds a
// row and nothing is overwritten (BR-ATT-002). Only submitted registers feed reports and alerts (BR-ATT-005).
package attendance

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

const (
	maxMarks         = 500
	minReason        = 3
	maxReason        = 200
	minWindowDays    = 1
	maxWindowDays    = 90
	defaultWindow    = 7
	defaultThreshold = 75
)

var statuses = map[string]bool{"present": true, "absent": true, "late": true, "leave": true}

// Mark is one student's status in a register.
type Mark struct {
	StudentID string
	Status    string
}

// Correction changes one student's status in a submitted register.
type Correction struct {
	StudentID string
	Status    string
	Reason    string
}

// Counts is how a register (or a day) adds up.
type Counts struct {
	Students int
	Present  int
	Absent   int
	Late     int
	Leave    int
}

// SettingsInput is a request to change the attendance settings.
type SettingsInput struct {
	RegisterDueTime      string // HH:MM
	EditWindowDays       int
	LowAttendancePercent int
}

// ParsedSettings is SettingsInput after checking.
type ParsedSettings struct {
	DueHour, DueMinute   int
	EditWindowDays       int
	LowAttendancePercent int
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

func validStatus(s string) bool { return statuses[s] }

// validateMarks checks a batch of marks: 1 to 500, each a real student id and status, no student twice.
func validateMarks(marks []Mark) error {
	var f fieldErrors
	if len(marks) == 0 || len(marks) > maxMarks {
		f.add("marks", "Send between 1 and 500 marks.")
	}
	seen := map[string]bool{}
	for _, m := range marks {
		if _, err := db.ParseUUID(m.StudentID); err != nil {
			f.add("marks", "Each mark needs a student id.")
			break
		}
		if !validStatus(m.Status) {
			f.add("marks", "Status must be present, absent, late or leave.")
			break
		}
		if seen[m.StudentID] {
			f.add("marks", "A student may appear once.")
			break
		}
		seen[m.StudentID] = true
	}
	return f.err("attendance.marks_invalid")
}

func validateCorrection(c Correction) (Correction, error) {
	var f fieldErrors
	if _, err := db.ParseUUID(c.StudentID); err != nil {
		f.add("student_id", "Required.")
	}
	if !validStatus(c.Status) {
		f.add("status", "Status must be present, absent, late or leave.")
	}
	c.Reason = strings.TrimSpace(c.Reason)
	if len(c.Reason) < minReason || len(c.Reason) > maxReason {
		f.add("reason", "A reason of 3 to 200 characters is required.")
	}
	return c, f.err("attendance.correction_invalid")
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func daysBetween(from, to time.Time) int {
	return int(dateOnly(to).Sub(dateOnly(from)).Hours() / 24)
}

var errFuture = apperr.Validation("attendance.date_invalid", "A register cannot be for a future date.",
	apperr.Field{Field: "date", Message: "Today or earlier."})

// canOpen decides whether a register may be opened for a date: never in the future, and not older than the
// correction window unless the caller holds attendance.edit_any (REQ-ATT-003).
func canOpen(date, today time.Time, windowDays int, editAny bool) error {
	if date.IsZero() || dateOnly(date).After(dateOnly(today)) {
		return errFuture
	}
	if daysBetween(date, today) > windowDays && !editAny {
		return errWindowClosed(windowDays)
	}
	return nil
}

// canCorrect decides whether a submitted register of a date may be corrected: within the window with
// attendance.edit_within_window (or edit_any), older with edit_any only (REQ-ATT-003).
func canCorrect(date, today time.Time, windowDays int, editWindow, editAny bool) error {
	if date.IsZero() || dateOnly(date).After(dateOnly(today)) {
		return errFuture
	}
	if editAny {
		return nil
	}
	if daysBetween(date, today) <= windowDays && editWindow {
		return nil
	}
	return errWindowClosed(windowDays)
}

func errWindowClosed(windowDays int) error {
	return apperr.Forbidden("attendance.window_closed",
		fmt.Sprintf("Only attendance.edit_any can change a day more than %d days back, or without edit_within_window.", windowDays))
}

// attendancePercent is (present + late) over (present + late + absent), to one decimal. Approved leave is
// excused and is not in the formula (owner decision 2026-11-17). With nothing expected it is 100.
func attendancePercent(present, late, absent int) (expected int, percent float64) {
	expected = present + late + absent
	if expected == 0 {
		return 0, 100
	}
	return expected, math.Round(float64(present+late)*1000/float64(expected)) / 10
}

// tally counts statuses. A student who left (moved or withdrew that day) is not counted anywhere.
func tally(statuses []string) Counts {
	var c Counts
	for _, s := range statuses {
		if s == "left" {
			continue
		}
		c.Students++
		switch s {
		case "present":
			c.Present++
		case "absent":
			c.Absent++
		case "late":
			c.Late++
		case "leave":
			c.Leave++
		}
	}
	return c
}

// registerOverdue says a register is missing once its due time has passed: on the day itself from the due
// time on, and for any earlier day always. A future day is never overdue. now is in Pakistan time and due is
// a time of day (REQ-ATT-016).
func registerOverdue(now, date, due time.Time) bool {
	today := dateOnly(now)
	d := dateOnly(date)
	switch {
	case d.After(today):
		return false
	case d.Before(today):
		return true
	}
	return now.Hour()*60+now.Minute() >= due.Hour()*60+due.Minute()
}

func validateSettings(in SettingsInput) (ParsedSettings, error) {
	var f fieldErrors
	var out ParsedSettings
	parts := strings.Split(in.RegisterDueTime, ":")
	h, errH := 0, error(nil)
	m, errM := 0, error(nil)
	if len(parts) == 2 {
		h, errH = strconv.Atoi(parts[0])
		m, errM = strconv.Atoi(parts[1])
	}
	if len(parts) != 2 || errH != nil || errM != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		f.add("register_due_time", "Use HH:MM, such as 10:00.")
	}
	out.DueHour, out.DueMinute = h, m
	if in.EditWindowDays < minWindowDays || in.EditWindowDays > maxWindowDays {
		f.add("edit_window_days", "Between 1 and 90.")
	}
	if in.LowAttendancePercent < 1 || in.LowAttendancePercent > 100 {
		f.add("low_attendance_percent", "Between 1 and 100.")
	}
	out.EditWindowDays, out.LowAttendancePercent = in.EditWindowDays, in.LowAttendancePercent
	return out, f.err("attendance.settings_invalid")
}

// belowThreshold decides who is on the low-attendance list from the exact ratio, never the rounded
// percentage: 74.96% is below 75. (present + late) / (present + late + absent) < threshold / 100.
func belowThreshold(present, late, absent, threshold int) bool {
	expected := present + late + absent
	return expected > 0 && (present+late)*100 < threshold*expected
}

// reconcileRoster says what a register needs when it is submitted: students on the roster with no record
// (they joined the section that day) are added as present, and students with a record who are no longer on
// the roster (moved or withdrew) are marked as left. A student already counted in another section's
// submitted register that day is not added, so nobody is counted twice. current maps student to status.
func reconcileRoster(roster []string, current map[string]string, countedElsewhere map[string]bool) (add, left []string) {
	onRoster := make(map[string]bool, len(roster))
	for _, id := range roster {
		onRoster[id] = true
		if _, has := current[id]; !has && !countedElsewhere[id] {
			add = append(add, id)
		}
	}
	for id, status := range current {
		if !onRoster[id] && status != "left" {
			left = append(left, id)
		}
	}
	sort.Strings(left)
	return add, left
}

// prefillIDs is who a new register opens with: the roster, less anyone already counted in another
// section's submitted register that day.
func prefillIDs(roster []string, countedElsewhere map[string]bool) []string {
	out := make([]string, 0, len(roster))
	for _, id := range roster {
		if !countedElsewhere[id] {
			out = append(out, id)
		}
	}
	return out
}
