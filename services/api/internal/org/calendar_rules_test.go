package org

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

// REQ-ORG-011: a day is a school day unless it is a weekly off day or a holiday; a listed working day
// overrides a weekly off day.
func TestREQ_ORG_011_IsSchoolDay(t *testing.T) {
	sunday := []int{0}
	cases := []struct {
		name  string
		day   time.Time
		off   []int
		entry *CalendarDay
		want  bool
		why   string
	}{
		{"an ordinary Monday", d(2026, time.November, 16), sunday, nil, true, ""},
		{"a Sunday", d(2026, time.November, 15), sunday, nil, false, "Weekly off day"},
		{"a holiday on a Monday", d(2026, time.August, 14), sunday, &CalendarDay{Kind: "holiday", Name: "Independence Day"}, false, "Independence Day"},
		{"a special working Sunday", d(2026, time.November, 15), sunday, &CalendarDay{Kind: "working", Name: "Make-up day"}, true, ""},
		{"a working entry on a normal day changes nothing", d(2026, time.November, 16), sunday, &CalendarDay{Kind: "working", Name: "x"}, true, ""},
		{"friday and saturday off", d(2026, time.November, 20), []int{5, 6}, nil, false, "Weekly off day"},
		{"no weekly off days at all", d(2026, time.November, 15), nil, nil, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, why := isSchoolDay(c.day, c.off, c.entry)
			if ok != c.want || why != c.why {
				t.Fatalf("got %v %q, want %v %q", ok, why, c.want, c.why)
			}
		})
	}
}

func TestREQ_ORG_011_ValidateWeeklyOff(t *testing.T) {
	got, err := validateWeeklyOff([]int{6, 5, 5})
	if err != nil || !slices.Equal(got, []int{5, 6}) {
		t.Fatalf("sorted and de-duplicated: %v %v", got, err)
	}
	if got, err := validateWeeklyOff(nil); err != nil || len(got) != 0 {
		t.Fatalf("no off days is allowed: %v %v", got, err)
	}
	for _, bad := range [][]int{{7}, {-1}, {0, 1, 2, 3, 4, 5, 6}} {
		if _, err := validateWeeklyOff(bad); err == nil {
			t.Fatalf("%v must be refused (a school needs a working day)", bad)
		}
	}
}

func TestREQ_ORG_011_ValidateCalendarDays(t *testing.T) {
	dates, in, err := validateCalendarDays(CalendarDaysInput{From: d(2026, time.August, 14), Kind: "holiday", Name: "  Independence Day "})
	if err != nil || len(dates) != 1 || in.Name != "Independence Day" {
		t.Fatalf("one day: %v %+v %v", dates, in, err)
	}
	to := d(2026, time.March, 25)
	dates, _, err = validateCalendarDays(CalendarDaysInput{From: d(2026, time.March, 20), To: &to, Kind: "holiday", Name: "Eid"})
	if err != nil || len(dates) != 6 || !dates[0].Equal(d(2026, time.March, 20)) || !dates[5].Equal(to) {
		t.Fatalf("a range of six: %v %v", dates, err)
	}
	long := d(2026, time.May, 19) // 60 dates from 20 Mar to 18 May; one more is too many
	lastOK := d(2026, time.May, 18)
	if dates, _, err := validateCalendarDays(CalendarDaysInput{From: d(2026, time.March, 20), To: &lastOK, Kind: "holiday", Name: "Summer"}); err != nil || len(dates) != 60 {
		t.Fatalf("60 dates are allowed: %d %v", len(dates), err)
	}
	before := d(2026, time.March, 19)
	for name, in := range map[string]CalendarDaysInput{
		"no date":     {Kind: "holiday", Name: "x"},
		"61 dates":    {From: d(2026, time.March, 20), To: &long, Kind: "holiday", Name: "x"},
		"end before":  {From: d(2026, time.March, 20), To: &before, Kind: "holiday", Name: "x"},
		"bad kind":    {From: d(2026, time.March, 20), Kind: "party", Name: "x"},
		"no name":     {From: d(2026, time.March, 20), Kind: "holiday", Name: "  "},
		"a long name": {From: d(2026, time.March, 20), Kind: "holiday", Name: strings.Repeat("x", 81)},
	} {
		if _, _, err := validateCalendarDays(in); err == nil {
			t.Fatalf("%s must be refused", name)
		}
	}
}
