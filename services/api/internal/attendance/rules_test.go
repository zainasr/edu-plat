package attendance

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func code(t *testing.T, err error) string {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("want an apperr, got %T: %v", err, err)
	}
	return e.Code
}

func fields(err error) []string {
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

const stu1, stu2 = "5c9c6c26-e0e4-4938-af5c-f6fc35688270", "6c9c6c26-e0e4-4938-af5c-f6fc35688270"

// REQ-ATT-010: what can be marked.
func TestREQ_ATT_010_ValidateMarks(t *testing.T) {
	if err := validateMarks([]Mark{{StudentID: stu1, Status: "absent"}, {StudentID: stu2, Status: "leave"}}); err != nil {
		t.Fatalf("good marks: %v", err)
	}
	cases := map[string][]Mark{
		"none":             nil,
		"unknown status":   {{StudentID: stu1, Status: "holiday"}},
		"not an id":        {{StudentID: "abc", Status: "absent"}},
		"the same student": {{StudentID: stu1, Status: "absent"}, {StudentID: stu1, Status: "late"}},
		"too many":         make([]Mark, 501),
	}
	for name, marks := range cases {
		if err := validateMarks(marks); err == nil || code(t, err) != "attendance.marks_invalid" {
			t.Fatalf("%s must be refused: %v", name, err)
		}
	}
}

// REQ-ATT-003, US-ATT-005: a correction needs a reason and a real status.
func TestREQ_ATT_003_ValidateCorrection(t *testing.T) {
	got, err := validateCorrection(Correction{StudentID: stu1, Status: "present", Reason: "  marked wrong by mistake "})
	if err != nil || got.Reason != "marked wrong by mistake" {
		t.Fatalf("good: %+v %v", got, err)
	}
	for name, c := range map[string]Correction{
		"no reason":    {StudentID: stu1, Status: "present", Reason: " "},
		"short reason": {StudentID: stu1, Status: "present", Reason: "ab"},
		"long reason":  {StudentID: stu1, Status: "present", Reason: strings.Repeat("x", 201)},
		"bad status":   {StudentID: stu1, Status: "gone", Reason: "because"},
		"no student":   {Status: "present", Reason: "because"},
	} {
		if _, err := validateCorrection(c); err == nil || code(t, err) != "attendance.correction_invalid" {
			t.Fatalf("%s must be refused: %v", name, err)
		}
	}
	if _, err := validateCorrection(Correction{StudentID: stu1, Status: "present", Reason: "abc"}); err != nil {
		t.Fatalf("3 characters are enough: %v", err)
	}
}

// REQ-ATT-001: a register can be opened for today or the past, not the future; a date older than the
// correction window needs attendance.edit_any.
func TestREQ_ATT_003_CanOpenRegister(t *testing.T) {
	today := day(2026, time.November, 17)
	cases := []struct {
		name    string
		date    time.Time
		editAny bool
		want    string // "" for allowed
	}{
		{"today", today, false, ""},
		{"yesterday", day(2026, time.November, 16), false, ""},
		{"exactly 7 days ago", day(2026, time.November, 10), false, ""},
		{"8 days ago", day(2026, time.November, 9), false, "attendance.window_closed"},
		{"8 days ago with edit_any", day(2026, time.November, 9), true, ""},
		{"tomorrow", day(2026, time.November, 18), false, "attendance.date_invalid"},
		{"tomorrow even with edit_any", day(2026, time.November, 18), true, "attendance.date_invalid"},
		{"no date", time.Time{}, false, "attendance.date_invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := canOpen(c.date, today, 7, c.editAny)
			if c.want == "" && err != nil {
				t.Fatalf("must be allowed: %v", err)
			}
			if c.want != "" && (err == nil || code(t, err) != c.want) {
				t.Fatalf("want %s, got %v", c.want, err)
			}
		})
	}
}

// REQ-ATT-003: corrections within the window need edit_within_window (or edit_any); older ones edit_any.
func TestREQ_ATT_003_CanCorrect(t *testing.T) {
	today := day(2026, time.November, 17)
	cases := []struct {
		name        string
		date        time.Time
		window, any bool
		want        string
	}{
		{"within the window with the window permission", day(2026, time.November, 12), true, false, ""},
		{"within the window with edit_any", day(2026, time.November, 12), false, true, ""},
		{"within the window with neither", day(2026, time.November, 12), false, false, "attendance.window_closed"},
		{"just inside the window", day(2026, time.November, 10), true, false, ""},
		{"just outside the window", day(2026, time.November, 9), true, false, "attendance.window_closed"},
		{"outside the window with edit_any", day(2026, time.October, 1), false, true, ""},
		{"a future date", day(2026, time.November, 18), true, true, "attendance.date_invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := canCorrect(c.date, today, 7, c.window, c.any)
			if c.want == "" && err != nil {
				t.Fatalf("must be allowed: %v", err)
			}
			if c.want != "" && (err == nil || code(t, err) != c.want) {
				t.Fatalf("want %s, got %v", c.want, err)
			}
		})
	}
}

// Owner decision 2026-11-17: attendance is (present + late) over (present + late + absent); leave is excused.
func TestREQ_ATT_Percentage(t *testing.T) {
	cases := []struct {
		present, late, absent int
		expected              int
		percent               float64
	}{
		{18, 2, 0, 20, 100},
		{15, 0, 5, 20, 75},
		{10, 5, 5, 20, 75},
		{0, 0, 3, 3, 0},
		{0, 0, 0, 0, 100}, // nothing expected: not a defaulter
		{1, 0, 2, 3, 33.3},
		{2, 0, 1, 3, 66.7},
	}
	for _, c := range cases {
		exp, pct := attendancePercent(c.present, c.late, c.absent)
		if exp != c.expected || pct != c.percent {
			t.Fatalf("%d/%d/%d: got %d %.1f, want %d %.1f", c.present, c.late, c.absent, exp, pct, c.expected, c.percent)
		}
	}
	// Leave never appears in the formula, so it cannot lower the percentage.
	_, withLeave := attendancePercent(15, 0, 5)
	if withLeave != 75 {
		t.Fatal("leave is excused")
	}
}

func TestREQ_ATT_Tally(t *testing.T) {
	c := tally([]string{"present", "present", "absent", "late", "leave", "present"})
	if c != (Counts{Students: 6, Present: 3, Absent: 1, Late: 1, Leave: 1}) {
		t.Fatalf("tally: %+v", c)
	}
	// A student who left (moved or withdrew that day) is not counted anywhere.
	if left := tally([]string{"present", "left", "absent", "left"}); left != (Counts{Students: 2, Present: 1, Absent: 1}) {
		t.Fatalf("left must not count: %+v", left)
	}
	if tally(nil) != (Counts{}) {
		t.Fatal("empty")
	}
}

// REQ-ATT-016, US-ATT-008: a register is "missing" once the due time has passed on a school day.
func TestREQ_ATT_016_RegisterOverdue(t *testing.T) {
	due := time.Date(0, 1, 1, 10, 0, 0, 0, time.UTC)
	today := day(2026, time.November, 17)
	at := func(h, m int) time.Time { return time.Date(2026, time.November, 17, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name string
		now  time.Time
		date time.Time
		want bool
	}{
		{"before the due time today", at(9, 59), today, false},
		{"at the due time today", at(10, 0), today, true},
		{"after the due time today", at(14, 0), today, true},
		{"a past day", at(7, 0), day(2026, time.November, 16), true},
		{"a future day", at(23, 0), day(2026, time.November, 18), false},
	}
	for _, c := range cases {
		if got := registerOverdue(c.now, c.date, due); got != c.want {
			t.Fatalf("%s: got %v", c.name, got)
		}
	}
}

func TestREQ_ATT_ValidateSettings(t *testing.T) {
	got, err := validateSettings(SettingsInput{RegisterDueTime: "09:30", EditWindowDays: 14, LowAttendancePercent: 80, MorningStart: "08:00", EveningStart: "13:00", LateAfterMinutes: 10})
	if err != nil || got.DueHour != 9 || got.DueMinute != 30 {
		t.Fatalf("good: %+v %v", got, err)
	}
	for name, in := range map[string]SettingsInput{
		"bad time":      {RegisterDueTime: "25:00", EditWindowDays: 7, LowAttendancePercent: 75, MorningStart: "08:00", EveningStart: "13:00", LateAfterMinutes: 10},
		"not a time":    {RegisterDueTime: "late", EditWindowDays: 7, LowAttendancePercent: 75, MorningStart: "08:00", EveningStart: "13:00", LateAfterMinutes: 10},
		"window 0":      {RegisterDueTime: "10:00", EditWindowDays: 0, LowAttendancePercent: 75, MorningStart: "08:00", EveningStart: "13:00", LateAfterMinutes: 10},
		"window 91":     {RegisterDueTime: "10:00", EditWindowDays: 91, LowAttendancePercent: 75, MorningStart: "08:00", EveningStart: "13:00", LateAfterMinutes: 10},
		"threshold 0":   {RegisterDueTime: "10:00", EditWindowDays: 7, LowAttendancePercent: 0, MorningStart: "08:00", EveningStart: "13:00", LateAfterMinutes: 10},
		"threshold 101": {RegisterDueTime: "10:00", EditWindowDays: 7, LowAttendancePercent: 101, MorningStart: "08:00", EveningStart: "13:00", LateAfterMinutes: 10},
	} {
		if _, err := validateSettings(in); err == nil || code(t, err) != "attendance.settings_invalid" {
			t.Fatalf("%s must be refused: %v", name, err)
		}
	}
	if !slices.Equal(fields(func() error {
		_, e := validateSettings(SettingsInput{RegisterDueTime: "x", EditWindowDays: 0, LowAttendancePercent: 0})
		return e
	}()),
		[]string{"register_due_time", "edit_window_days", "low_attendance_percent", "morning_start", "evening_start"}) {
		t.Fatal("all three problems are reported together")
	}
}

// Marks can set four statuses; "left" is written by the system at submit, never by a person.
func TestREQ_ATT_010_LeftIsNotMarkable(t *testing.T) {
	if err := validateMarks([]Mark{{StudentID: stu1, Status: "left"}}); err == nil {
		t.Fatal("a person cannot mark a student as left")
	}
	if _, err := validateCorrection(Correction{StudentID: stu1, Status: "left", Reason: "because"}); err == nil {
		t.Fatal("a person cannot correct a student to left")
	}
}

// The low-attendance threshold compares the exact ratio, not the rounded percentage: 74.96% is below 75.
func TestREQ_ATT_BelowThreshold(t *testing.T) {
	cases := []struct {
		name                  string
		present, late, absent int
		threshold             int
		want                  bool
	}{
		{"exactly at the threshold", 15, 0, 5, 75, false},
		{"just under, rounds up to 75.0", 2999, 0, 1001, 75, true}, // 74.975%
		{"just over", 3001, 0, 999, 75, false},
		{"late counts as attended", 10, 5, 5, 75, false},
		{"all absent", 0, 0, 4, 1, true},
		{"nothing expected is never flagged", 0, 0, 0, 100, false},
		{"leave does not appear, so it cannot flag anyone", 0, 0, 0, 75, false},
	}
	for _, c := range cases {
		if got := belowThreshold(c.present, c.late, c.absent, c.threshold); got != c.want {
			t.Fatalf("%s: got %v", c.name, got)
		}
	}
	if _, pct := attendancePercent(2999, 0, 1001); pct != 75 {
		t.Fatal("the displayed percentage still rounds to one decimal")
	}
}

// A register is reconciled with the roster when it is submitted (owner-visible rule, review of slice 6a):
// students who joined the section that day are recorded present, students who are no longer in it are
// recorded as left, and a student already accounted for in another section's submitted register that day
// is left out so nobody counts twice.
func TestREQ_ATT_ReconcileRoster(t *testing.T) {
	roster := []string{"a", "b", "c", "d"}
	current := map[string]string{"a": "present", "b": "absent", "x": "present", "y": "left"}
	elsewhere := map[string]bool{"d": true}
	add, left := reconcileRoster(roster, current, elsewhere)
	if !slices.Equal(add, []string{"c"}) {
		t.Fatalf("c joined and has no record; d is counted in another section: add %v", add)
	}
	if !slices.Equal(left, []string{"x"}) {
		t.Fatalf("x is no longer on the roster (y already left): left %v", left)
	}
	if add, left := reconcileRoster(nil, nil, nil); len(add) != 0 || len(left) != 0 {
		t.Fatal("empty in, empty out")
	}
	// Prefill on open skips students counted elsewhere.
	if got := prefillIDs(roster, elsewhere); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("prefill: %v", got)
	}
}
