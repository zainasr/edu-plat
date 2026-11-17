package org

import (
	"slices"
	"sort"
	"strings"
	"time"
)

// The school calendar (REQ-ORG-011): weekly off days, holidays, and special working days. Pure rules.

const (
	maxCalendarName  = 80
	maxCalendarDates = 60
)

// CalendarDay is one entry: a holiday or a special working day on one date.
type CalendarDay struct {
	ID     string
	Date   time.Time
	Kind   string // holiday or working
	Name   string
	Status string // active or removed
}

// CalendarDaysInput is a request to add holidays or working days for one date or a range.
type CalendarDaysInput struct {
	From time.Time
	To   *time.Time // inclusive; nil means just From
	Kind string
	Name string
}

// SchoolDayInfo answers "is this a school day?" and, if not, why.
type SchoolDayInfo struct {
	SchoolDay bool
	Reason    string
}

// isSchoolDay applies the calendar: a listed holiday is never a school day; a listed working day always is;
// otherwise a weekly off day is not. entry is the active entry for the date, if any.
func isSchoolDay(day time.Time, weeklyOff []int, entry *CalendarDay) (bool, string) {
	if entry != nil {
		if entry.Kind == "holiday" {
			return false, entry.Name
		}
		return true, ""
	}
	if slices.Contains(weeklyOff, int(day.Weekday())) {
		return false, "Weekly off day"
	}
	return true, ""
}

// validateWeeklyOff sorts and de-duplicates the days and refuses an impossible week: values outside 0 to 6,
// or all seven days off.
func validateWeeklyOff(days []int) ([]int, error) {
	var f fieldErrors
	seen := map[int]bool{}
	out := make([]int, 0, len(days))
	for _, d := range days {
		if d < 0 || d > 6 {
			f.add("days", "Days are numbered 0 (Sunday) to 6 (Saturday).")
			break
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	if len(out) > 6 {
		f.add("days", "A school needs at least one working day in the week.")
	}
	sort.Ints(out)
	return out, f.err("calendar.invalid")
}

// validateCalendarDays checks the request and returns every date it covers.
func validateCalendarDays(in CalendarDaysInput) ([]time.Time, CalendarDaysInput, error) {
	var f fieldErrors
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > maxCalendarName {
		f.add("name", "Required, up to 80 characters.")
	}
	if in.Kind != "holiday" && in.Kind != "working" {
		f.add("kind", "Choose holiday or working.")
	}
	if in.From.IsZero() {
		f.add("from", "Required.")
		return nil, in, f.err("calendar.invalid")
	}
	from := time.Date(in.From.Year(), in.From.Month(), in.From.Day(), 0, 0, 0, 0, time.UTC)
	to := from
	if in.To != nil {
		to = time.Date(in.To.Year(), in.To.Month(), in.To.Day(), 0, 0, 0, 0, time.UTC)
		if to.Before(from) {
			f.add("to", "Must not be before from.")
		}
	}
	var dates []time.Time
	if !to.Before(from) {
		n := int(to.Sub(from).Hours()/24) + 1
		if n > maxCalendarDates {
			f.add("to", "At most 60 dates at a time.")
		} else {
			for i := range n {
				dates = append(dates, from.AddDate(0, 0, i))
			}
		}
	}
	if err := f.err("calendar.invalid"); err != nil {
		return nil, in, err
	}
	return dates, in, nil
}
