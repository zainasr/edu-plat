package integration

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/attendance"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// attendWorld is a school with one section and some students, and no weekly off day, so tests do not
// depend on the weekday they run on.
type attendWorld struct {
	slug, tenantID string
	owner          iam.Principal
	token          string
	sectionID      string
	students       []string
}

func today() time.Time {
	n := time.Now().In(karachi)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func ago(days int) time.Time { return today().AddDate(0, 0, -days) }

func newAttendWorld(t *testing.T, f *fixture, label string, n int) attendWorld {
	t.Helper()
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, label)
	token := signIn(t, f, mobile, slug)
	p := mustPrincipal(t, f, token)
	st := newStructure(t, f, p)
	if _, err := f.org.SetWeeklyOff(ctx, p, []int{}); err != nil {
		t.Fatal(err)
	}
	w := attendWorld{slug: slug, tenantID: tenantID, owner: p, token: token, sectionID: st.SectionID}
	for i := range n {
		in := enrolInput(t, st.SectionID)
		in.FullName = fmt.Sprintf("Student %d", i+1)
		s, err := f.students.Create(ctx, p, in)
		if err != nil {
			t.Fatalf("enrol: %v", err)
		}
		w.students = append(w.students, s.ID)
	}
	return w
}

// backdate makes everyone enrolled in the school start on the given date, so earlier days have a roster.
func (w attendWorld) backdate(t *testing.T, f *fixture, from time.Time) {
	t.Helper()
	if _, err := f.admin.Exec(context.Background(), `UPDATE enrolments SET started_on = $2 WHERE tenant_id = $1`, w.tenantID, from); err != nil {
		t.Fatal(err)
	}
}

func (w attendWorld) open(t *testing.T, f *fixture, date time.Time) attendance.Register {
	t.Helper()
	r, err := f.attendance.OpenRegister(context.Background(), w.owner, w.sectionID, date)
	if err != nil {
		t.Fatalf("open register: %v", err)
	}
	return r
}

func (w attendWorld) mark(t *testing.T, f *fixture, regID string, marks ...attendance.Mark) attendance.Register {
	t.Helper()
	r, err := f.attendance.MarkRecords(context.Background(), w.owner, regID, marks)
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	return r
}

func (w attendWorld) submit(t *testing.T, f *fixture, regID string) attendance.Register {
	t.Helper()
	r, err := f.attendance.SubmitRegister(context.Background(), w.owner, regID)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return r
}

func statusOf(r attendance.Register, student string) string {
	for _, rec := range r.Records {
		if rec.StudentID == student {
			return rec.Status
		}
	}
	return ""
}

// US-ATT-006, REQ-ATT-015: the register opens with everyone present; only the exceptions are marked; then submit.
func TestUS_ATT_006_AbsenteesOnlyRegister(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-basic", 5)

	reg := w.open(t, f, today())
	if reg.Status != "open" || len(reg.Records) != 5 || reg.Counts.Present != 5 || reg.Counts.Students != 5 {
		t.Fatalf("a new register is everyone present: %+v", reg.Counts)
	}
	for _, r := range reg.Records {
		if r.Status != "present" || r.Method != "prefill" || r.Version != 1 || r.Corrected || r.FullName == "" || r.RegistrationNo == "" {
			t.Fatalf("prefilled record: %+v", r)
		}
	}
	// Opening again is the same register, not a second one.
	if again := w.open(t, f, today()); again.ID != reg.ID || len(again.Records) != 5 {
		t.Fatalf("opening twice: %+v", again)
	}

	marked := w.mark(t, f, reg.ID,
		attendance.Mark{StudentID: w.students[1], Status: "absent"},
		attendance.Mark{StudentID: w.students[2], Status: "late"},
		attendance.Mark{StudentID: w.students[3], Status: "leave"})
	if marked.Counts.Present != 2 || marked.Counts.Absent != 1 || marked.Counts.Late != 1 || marked.Counts.Leave != 1 {
		t.Fatalf("counts: %+v", marked.Counts)
	}
	// A mark equal to the current status adds nothing: versions stay put.
	again := w.mark(t, f, reg.ID, attendance.Mark{StudentID: w.students[1], Status: "absent"}, attendance.Mark{StudentID: w.students[0], Status: "present"})
	for _, r := range again.Records {
		want := 1
		if r.StudentID == w.students[1] || r.StudentID == w.students[2] || r.StudentID == w.students[3] {
			want = 2
		}
		if r.Version != want {
			t.Fatalf("%s version %d, want %d", r.FullName, r.Version, want)
		}
	}

	sub := w.submit(t, f, reg.ID)
	if sub.Status != "submitted" || sub.SubmittedAt == nil {
		t.Fatalf("submitted: %+v", sub)
	}
	if _, err := f.attendance.SubmitRegister(ctx, w.owner, reg.ID); errCode(t, err) != "attendance.already_submitted" {
		t.Fatalf("submit twice: %v", err)
	}
	if _, err := f.attendance.MarkRecords(ctx, w.owner, reg.ID, []attendance.Mark{{StudentID: w.students[0], Status: "absent"}}); errCode(t, err) != "attendance.register_submitted" {
		t.Fatalf("mark after submit: %v", err)
	}
	var audits int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action IN ('attendance.register_opened','attendance.marked','attendance.register_submitted')`, w.tenantID).Scan(&audits)
	if audits != 3 {
		t.Fatalf("one audit row for opening, marking and submitting: %d", audits)
	}
}

func TestREQ_ATT_010_MarkRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-refuse", 2)
	reg := w.open(t, f, today())

	// A student who joins the section after the register opened can be marked; a stranger cannot.
	in := enrolInput(t, w.sectionID)
	in.FullName = "Late Joiner"
	joiner, err := f.students.Create(ctx, w.owner, in)
	if err != nil {
		t.Fatal(err)
	}
	if r := w.mark(t, f, reg.ID, attendance.Mark{StudentID: joiner.ID, Status: "absent"}); statusOf(r, joiner.ID) != "absent" || len(r.Records) != 3 {
		t.Fatalf("late joiner: %+v", r.Records)
	}
	other := newAttendWorld(t, f, "att-refuse-b", 1)
	if _, err := f.attendance.MarkRecords(ctx, w.owner, reg.ID, []attendance.Mark{{StudentID: other.students[0], Status: "absent"}}); errCode(t, err) != "attendance.student_not_in_section" {
		t.Fatalf("another school's student: %v", err)
	}
	if _, err := f.attendance.MarkRecords(ctx, w.owner, reg.ID, nil); errCode(t, err) != "attendance.marks_invalid" {
		t.Fatalf("no marks: %v", err)
	}
	if _, err := f.attendance.MarkRecords(ctx, w.owner, "5c9c6c26-e0e4-4938-af5c-f6fc35688270", []attendance.Mark{{StudentID: w.students[0], Status: "absent"}}); errCode(t, err) != "attendance.register_not_found" {
		t.Fatalf("unknown register: %v", err)
	}
}

// BR-ATT-001: no register on a holiday, a weekly off day, or in the future; a special working day overrides a weekly off day.
func TestBR_ATT_001_NoRegisterOnANonSchoolDay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-holiday", 2)
	w.backdate(t, f, ago(10))

	day := ago(2)
	if _, err := f.org.AddCalendarDays(ctx, w.owner, org.CalendarDaysInput{From: day, Kind: "holiday", Name: "Independence Day"}); err != nil {
		t.Fatal(err)
	}
	_, err := f.attendance.OpenRegister(ctx, w.owner, w.sectionID, day)
	if errCode(t, err) != "attendance.not_school_day" || !strings.Contains(err.Error(), "Independence Day") {
		t.Fatalf("a holiday: %v", err)
	}
	// A weekly off day.
	off := ago(3)
	if _, err := f.org.SetWeeklyOff(ctx, w.owner, []int{int(off.Weekday())}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.attendance.OpenRegister(ctx, w.owner, w.sectionID, off); errCode(t, err) != "attendance.not_school_day" {
		t.Fatalf("a weekly off day: %v", err)
	}
	// ...unless it is listed as a working day.
	if _, err := f.org.AddCalendarDays(ctx, w.owner, org.CalendarDaysInput{From: off, Kind: "working", Name: "Make-up day"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.attendance.OpenRegister(ctx, w.owner, w.sectionID, off); err != nil {
		t.Fatalf("a special working day: %v", err)
	}
	// Removing the holiday makes the date open-able again.
	days, _ := f.org.GetCalendar(ctx, w.owner, day, day)
	if _, err := f.org.RemoveCalendarDay(ctx, w.owner, days.Days[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.attendance.OpenRegister(ctx, w.owner, w.sectionID, day); err != nil {
		t.Fatalf("after removing the holiday: %v", err)
	}
	// The future, and an archived section.
	if _, err := f.attendance.OpenRegister(ctx, w.owner, w.sectionID, today().AddDate(0, 0, 1)); errCode(t, err) != "attendance.date_invalid" {
		t.Fatalf("tomorrow: %v", err)
	}
	if _, err := f.attendance.OpenRegister(ctx, w.owner, "5c9c6c26-e0e4-4938-af5c-f6fc35688270", today()); errCode(t, err) != "attendance.section_not_found" {
		t.Fatalf("unknown section: %v", err)
	}
	if _, err := f.org.ArchiveSection(ctx, w.owner, w.sectionID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.attendance.OpenRegister(ctx, w.owner, w.sectionID, today()); errCode(t, err) != "attendance.section_archived" {
		t.Fatalf("archived section: %v", err)
	}
}

// US-ATT-005, REQ-ATT-003, BR-ATT-002: a correction keeps the old value, who, when and why; the window
// depends on the permission.
func TestUS_ATT_005_CorrectionsAreVersionedAndWindowed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-correct", 3)
	w.backdate(t, f, ago(20))
	clerk := mustPrincipal(t, f, addStaff(t, f, w.tenantID, w.slug, "attendance_clerk"))

	recent := w.open(t, f, ago(3))
	w.mark(t, f, recent.ID, attendance.Mark{StudentID: w.students[0], Status: "absent"})
	w.submit(t, f, recent.ID)
	old := w.open(t, f, ago(12)) // the owner may open an old day (edit_any)
	w.mark(t, f, old.ID, attendance.Mark{StudentID: w.students[0], Status: "absent"})
	w.submit(t, f, old.ID)

	// An open register is marked, not corrected.
	open := w.open(t, f, today())
	if _, err := f.attendance.CorrectRecord(ctx, w.owner, open.ID, attendance.Correction{StudentID: w.students[0], Status: "late", Reason: "slip"}); errCode(t, err) != "attendance.register_open" {
		t.Fatalf("correct an open register: %v", err)
	}

	// The clerk (window only) corrects within 7 days, with a reason...
	if _, err := f.attendance.CorrectRecord(ctx, clerk, recent.ID, attendance.Correction{StudentID: w.students[0], Status: "present", Reason: " "}); errCode(t, err) != "attendance.correction_invalid" {
		t.Fatalf("no reason: %v", err)
	}
	fixed, err := f.attendance.CorrectRecord(ctx, clerk, recent.ID, attendance.Correction{StudentID: w.students[0], Status: "present", Reason: "marked absent from the wrong slip"})
	if err != nil || statusOf(fixed, w.students[0]) != "present" {
		t.Fatalf("clerk correction: %v", err)
	}
	for _, r := range fixed.Records {
		if r.StudentID == w.students[0] && (r.Version != 3 || !r.Corrected || r.Method != "correction") {
			t.Fatalf("record after correction: %+v", r)
		}
	}
	// ...but not beyond it; the owner (edit_any) can.
	if _, err := f.attendance.CorrectRecord(ctx, clerk, old.ID, attendance.Correction{StudentID: w.students[0], Status: "present", Reason: "too old for the clerk"}); errCode(t, err) != "attendance.window_closed" {
		t.Fatalf("clerk beyond the window: %v", err)
	}
	if _, err := f.attendance.CorrectRecord(ctx, w.owner, old.ID, attendance.Correction{StudentID: w.students[0], Status: "present", Reason: "owner corrects an old day"}); err != nil {
		t.Fatalf("owner beyond the window: %v", err)
	}
	// No change, and an unknown student.
	if _, err := f.attendance.CorrectRecord(ctx, w.owner, recent.ID, attendance.Correction{StudentID: w.students[0], Status: "present", Reason: "same again"}); errCode(t, err) != "attendance.no_change" {
		t.Fatalf("no change: %v", err)
	}
	if _, err := f.attendance.CorrectRecord(ctx, w.owner, recent.ID, attendance.Correction{StudentID: "5c9c6c26-e0e4-4938-af5c-f6fc35688270", Status: "present", Reason: "who?"}); errCode(t, err) != "attendance.record_not_found" {
		t.Fatalf("unknown student: %v", err)
	}

	// Every version is kept: the history shows old, new, who and why (BR-ATT-002).
	rows, err := f.admin.Query(ctx, `SELECT version, status, method, COALESCE(reason, ''), changed_by::text FROM attendance_records WHERE register_id = $1 AND student_id = $2 ORDER BY version`, recent.ID, w.students[0])
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var v int
		var status, method, reason, by string
		_ = rows.Scan(&v, &status, &method, &reason, &by)
		if by == "" {
			t.Fatal("who changed it must be recorded")
		}
		got = append(got, fmt.Sprintf("%d:%s:%s:%s", v, status, method, reason))
	}
	want := []string{"1:present:prefill:", "2:absent:manual:", "3:present:correction:marked absent from the wrong slip"}
	if !slices.Equal(got, want) {
		t.Fatalf("history %v, want %v", got, want)
	}
	// The same version history is audited.
	var n int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'attendance.corrected'`, w.tenantID).Scan(&n)
	if n != 2 {
		t.Fatalf("two audited corrections, got %d", n)
	}
	// A day older than the window cannot even be opened by the clerk.
	if _, err := f.attendance.OpenRegister(ctx, clerk, w.sectionID, ago(15)); errCode(t, err) != "attendance.window_closed" {
		t.Fatalf("clerk opening a very old day: %v", err)
	}
}

// REQ-ATT-016, US-ATT-008: a section with no submitted register is "missing" after the due time, never absent.
func TestREQ_ATT_016_MissingRegisters(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-missing", 2)
	second, err := f.org.CreateSection(ctx, w.owner, org.SectionInput{ClassID: mustClassID(t, f, w), Name: "B", Shift: "morning"})
	if err != nil {
		t.Fatal(err)
	}
	in := enrolInput(t, second.ID)
	in.FullName = "In Section B"
	if _, err := f.students.Create(ctx, w.owner, in); err != nil {
		t.Fatal(err)
	}

	// Before the due time: nothing is overdue yet.
	if _, err := f.attendance.SetSettings(ctx, w.owner, attendance.SettingsInput{RegisterDueTime: "23:59", EditWindowDays: 7, LowAttendancePercent: 75}); err != nil {
		t.Fatal(err)
	}
	if m, _ := f.attendance.MissingRegisters(ctx, w.owner, today()); m.Overdue || len(m.Items) != 0 {
		t.Fatalf("before the due time: %+v", m)
	}
	// After it: A has an open register, B has none; both are missing, and nobody is marked absent.
	if _, err := f.attendance.SetSettings(ctx, w.owner, attendance.SettingsInput{RegisterDueTime: "00:00", EditWindowDays: 7, LowAttendancePercent: 75}); err != nil {
		t.Fatal(err)
	}
	a := w.open(t, f, today())
	m, err := f.attendance.MissingRegisters(ctx, w.owner, today())
	if err != nil || !m.Overdue || len(m.Items) != 2 {
		t.Fatalf("after the due time: %v %+v", err, m)
	}
	for _, it := range m.Items {
		if it.SectionID == w.sectionID && !it.HasOpenRegister || it.SectionID == second.ID && it.HasOpenRegister {
			t.Fatalf("open flag: %+v", it)
		}
	}
	if absent, _ := f.attendance.AbsentOnDay(ctx, w.owner, today()); len(absent) != 0 {
		t.Fatalf("a missing register is never read as all-absent: %+v", absent)
	}
	w.submit(t, f, a.ID)
	m, _ = f.attendance.MissingRegisters(ctx, w.owner, today())
	if len(m.Items) != 1 || m.Items[0].SectionID != second.ID {
		t.Fatalf("after A is submitted: %+v", m.Items)
	}
	// A holiday has none.
	if _, err := f.org.AddCalendarDays(ctx, w.owner, org.CalendarDaysInput{From: ago(1), Kind: "holiday", Name: "Closed"}); err != nil {
		t.Fatal(err)
	}
	if m, _ := f.attendance.MissingRegisters(ctx, w.owner, ago(1)); m.SchoolDay || len(m.Items) != 0 {
		t.Fatalf("a holiday: %+v", m)
	}
}

func mustClassID(t *testing.T, f *fixture, w attendWorld) string {
	t.Helper()
	var id string
	if err := f.admin.QueryRow(context.Background(), `SELECT class_id FROM sections WHERE id = $1`, w.sectionID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Owner decision 2026-11-17: attendance is (present + late) / (present + late + absent); leave is excused.
// Reports use submitted registers only.
func TestREQ_ATT_ReportsFromSubmittedRegisters(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-reports", 3)
	w.backdate(t, f, ago(10))
	s0, s1, s2 := w.students[0], w.students[1], w.students[2]

	// Four school days. s0: always present. s1: absent 3 of 4 days (25%). s2: absent 1, leave 1, present 2 (66.7%).
	plan := []map[string]string{
		{s1: "absent", s2: "absent"},
		{s1: "absent", s2: "leave"},
		{s1: "absent"},
		{s1: "late"},
	}
	for i, marks := range plan {
		reg := w.open(t, f, ago(4-i))
		for stu, st := range marks {
			w.mark(t, f, reg.ID, attendance.Mark{StudentID: stu, Status: st})
		}
		w.submit(t, f, reg.ID)
	}
	// Today's register is open and never submitted: it must not count anywhere.
	openToday := w.open(t, f, today())
	w.mark(t, f, openToday.ID, attendance.Mark{StudentID: s0, Status: "absent"})

	sum, err := f.attendance.DailySummary(ctx, w.owner, ago(4))
	if err != nil || !sum.SchoolDay || sum.Totals.Present != 1 || sum.Totals.Absent != 2 || len(sum.Sections) != 1 || !sum.Sections[0].Submitted {
		t.Fatalf("summary: %v %+v", err, sum)
	}
	todaySum, _ := f.attendance.DailySummary(ctx, w.owner, today())
	if len(todaySum.Sections) != 1 || todaySum.Sections[0].Submitted || todaySum.Totals.Students != 0 {
		t.Fatalf("an open register is listed but not counted: %+v", todaySum)
	}

	absent, err := f.attendance.AbsentOnDay(ctx, w.owner, ago(4))
	if err != nil || len(absent) != 2 || absent[0].Contact == nil || absent[0].Contact.Name != "Kamran Khan" || absent[0].Contact.Mobile == "" {
		t.Fatalf("absent list with a contact to call: %v %+v", err, absent)
	}
	if none, _ := f.attendance.AbsentOnDay(ctx, w.owner, today()); len(none) != 0 {
		t.Fatalf("today's register is not submitted: %+v", none)
	}

	low, err := f.attendance.LowAttendance(ctx, w.owner, ago(4), today(), 0) // the school's 75% setting; today's open register must not count
	if err != nil || low.Threshold != 75 || len(low.Items) != 2 {
		t.Fatalf("low attendance: %v %+v", err, low)
	}
	if low.Items[0].StudentID != s1 || low.Items[0].Totals.Percent != 25 || low.Items[0].Totals.ExpectedDays != 4 {
		t.Fatalf("lowest first: %+v", low.Items[0])
	}
	if low.Items[1].StudentID != s2 || low.Items[1].Totals.Percent != 66.7 || low.Items[1].Totals.Leave != 1 || low.Items[1].Totals.ExpectedDays != 3 {
		t.Fatalf("leave is excused, not counted against: %+v", low.Items[1])
	}
	if strict, _ := f.attendance.LowAttendance(ctx, w.owner, ago(4), today(), 30); len(strict.Items) != 1 || strict.Items[0].StudentID != s1 {
		t.Fatalf("a 30%% threshold: %+v", strict.Items)
	}

	h, err := f.attendance.StudentHistory(ctx, w.owner, s2, ago(4), today())
	if err != nil || len(h.Days) != 4 || h.Totals.Percent != 66.7 || h.Days[1].Status != "leave" {
		t.Fatalf("student history: %v %+v", err, h)
	}
	// s0 was marked absent in today's register, which is still open: it appears in no total and no day.
	if h0, _ := f.attendance.StudentHistory(ctx, w.owner, s0, ago(4), today()); len(h0.Days) != 4 || h0.Totals.Absent != 0 || h0.Totals.Percent != 100 {
		t.Fatalf("an open register must not count for the student: %+v", h0)
	}
	if _, err := f.attendance.StudentHistory(ctx, w.owner, "5c9c6c26-e0e4-4938-af5c-f6fc35688270", ago(4), ago(1)); errCode(t, err) != "students.not_found" {
		t.Fatalf("unknown student: %v", err)
	}
	for name, fn := range map[string]func() error{
		"no from":  func() error { _, e := f.attendance.LowAttendance(ctx, w.owner, time.Time{}, ago(1), 0); return e },
		"too long": func() error { _, e := f.attendance.LowAttendance(ctx, w.owner, ago(400), today(), 0); return e },
		"backward": func() error { _, e := f.attendance.StudentHistory(ctx, w.owner, s0, ago(1), ago(4)); return e },
	} {
		if errCode(t, fn()) != "attendance.range_invalid" {
			t.Fatalf("%s must be refused", name)
		}
	}
}

// Two clerks pressing Submit, marking, or opening at once: the register is submitted once, versions stay
// contiguous, and the roster is written once.
func TestUS_ATT_007_ConcurrentClerksDoNotCorruptARegister(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-race", 6)

	// Six clerks open the same register at once.
	regs := make([]attendance.Register, 6)
	var wg sync.WaitGroup
	for i := range regs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			regs[i] = w.open(t, f, today())
		}()
	}
	wg.Wait()
	var registers, records int
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM attendance_registers WHERE tenant_id = $1`, w.tenantID).Scan(&registers)
	_ = f.admin.QueryRow(ctx, `SELECT count(*) FROM attendance_records WHERE tenant_id = $1`, w.tenantID).Scan(&records)
	if registers != 1 || records != 6 {
		t.Fatalf("one register and one roster: %d registers, %d records", registers, records)
	}
	id := regs[0].ID

	// Alternating marks of the same student from many clerks: versions climb with no gap and no duplicate.
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := []string{"absent", "late"}[i%2]
			_, _ = f.attendance.MarkRecords(ctx, w.owner, id, []attendance.Mark{{StudentID: w.students[0], Status: status}})
		}()
	}
	wg.Wait()
	rows, _ := f.admin.Query(ctx, `SELECT version FROM attendance_records WHERE register_id = $1 AND student_id = $2 ORDER BY version`, id, w.students[0])
	defer rows.Close()
	last := 0
	for rows.Next() {
		var v int
		_ = rows.Scan(&v)
		if v != last+1 {
			t.Fatalf("versions must be contiguous: %d after %d", v, last)
		}
		last = v
	}

	// Six clerks submit at once: one wins.
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.attendance.SubmitRegister(ctx, w.owner, id)
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if errCode(t, err) != "attendance.already_submitted" {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one submit wins, %d did", wins)
	}
}

// ADR-0002: school B cannot open, read, mark or correct school A's registers.
func TestRLS_AttendanceIsIsolatedBetweenSchools(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := newAttendWorld(t, f, "att-iso-a", 2)
	b := newAttendWorld(t, f, "att-iso-b", 2)
	reg := a.open(t, f, today())
	a.submit(t, f, reg.ID)

	if _, err := f.attendance.OpenRegister(ctx, b.owner, a.sectionID, today()); errCode(t, err) != "attendance.section_not_found" {
		t.Fatalf("B opened A's section: %v", err)
	}
	if _, err := f.attendance.GetRegister(ctx, b.owner, reg.ID); errCode(t, err) != "attendance.register_not_found" {
		t.Fatalf("B read A's register: %v", err)
	}
	if _, err := f.attendance.MarkRecords(ctx, b.owner, reg.ID, []attendance.Mark{{StudentID: a.students[0], Status: "absent"}}); errCode(t, err) != "attendance.register_not_found" {
		t.Fatalf("B marked A's register: %v", err)
	}
	if _, err := f.attendance.CorrectRecord(ctx, b.owner, reg.ID, attendance.Correction{StudentID: a.students[0], Status: "absent", Reason: "attack"}); errCode(t, err) != "attendance.register_not_found" {
		t.Fatalf("B corrected A's register: %v", err)
	}
	if list, _ := f.attendance.ListRegisters(ctx, b.owner, today()); len(list) != 0 {
		t.Fatalf("B's list leaked: %+v", list)
	}
	if _, err := f.attendance.StudentHistory(ctx, b.owner, a.students[0], ago(2), today()); errCode(t, err) != "students.not_found" {
		t.Fatalf("B read A's student history: %v", err)
	}
	if got, _ := f.attendance.GetRegister(ctx, a.owner, reg.ID); got.Status != "submitted" {
		t.Fatalf("A's register was touched: %+v", got)
	}
}

// The database enforces the register lifecycle: even the table owner cannot overwrite history, mark a
// submitted register, correct an open one, skip a version, reopen a register, or edit the calendar.
func TestBR_ATT_002_TheDatabaseProtectsAttendanceHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-db", 2)
	reg := w.open(t, f, today())
	openID, stu := reg.ID, w.students[0]

	insert := func(register string, version int, method string, reason any) error {
		_, err := f.admin.Exec(ctx, `INSERT INTO attendance_records (tenant_id, register_id, student_id, version, status, method, reason)
			VALUES ($1, $2, $3, $4, 'absent', $5, $6)`, w.tenantID, register, stu, version, method, reason)
		return err
	}
	if err := insert(openID, 3, "manual", nil); err == nil {
		t.Fatal("a version may not skip ahead")
	}
	if err := insert(openID, 2, "correction", "reason given"); err == nil {
		t.Fatal("an open register cannot be corrected")
	}
	if err := insert(openID, 2, "correction", nil); err == nil {
		t.Fatal("a correction needs a reason")
	}
	if err := insert(openID, 2, "manual", nil); err != nil {
		t.Fatalf("the next version of an open register is fine: %v", err)
	}
	for name, stmt := range map[string]string{
		"change a record":     `UPDATE attendance_records SET status = 'present' WHERE register_id = $1`,
		"delete a record":     `DELETE FROM attendance_records WHERE register_id = $1`,
		"delete the register": `DELETE FROM attendance_registers WHERE id = $1`,
	} {
		if _, err := f.admin.Exec(ctx, stmt, openID); err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}
	w.submit(t, f, openID)
	if err := insert(openID, 3, "manual", nil); err == nil {
		t.Fatal("a submitted register changes only by correction")
	}
	if err := insert(openID, 3, "correction", "a real reason"); err != nil {
		t.Fatalf("a correction of a submitted register is fine: %v", err)
	}
	for name, stmt := range map[string]string{
		"reopen the register": `UPDATE attendance_registers SET status = 'open', submitted_at = NULL WHERE id = $1`,
		"move the register":   `UPDATE attendance_registers SET day = day - 1 WHERE id = $1`,
	} {
		if _, err := f.admin.Exec(ctx, stmt, openID); err == nil {
			t.Fatalf("%s: the database allowed it", name)
		}
	}
	// The API role can do none of it either: it may not delete, and may not touch the history columns.
	for name, stmt := range map[string]string{
		"app role deletes a record":   `DELETE FROM attendance_records WHERE register_id = $1`,
		"app role edits a record":     `UPDATE attendance_records SET status = 'present' WHERE register_id = $1`,
		"app role moves the register": `UPDATE attendance_registers SET day = day - 1 WHERE id = $1`,
	} {
		err := db.WithTenant(ctx, f.app, w.tenantID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, stmt, openID)
			return err
		})
		if err == nil {
			t.Fatalf("%s: allowed", name)
		}
	}
}

// Attendance over HTTP: shapes, status codes and the "required query param is checked after permission" rule.
func TestREQ_API_AttendanceOverHTTP(t *testing.T) {
	f := newFixture(t)
	w := newAttendWorld(t, f, "att-http", 3)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students, f.ledger, f.fees, f.attendance), f.iam)
	d := today().Format("2006-01-02")

	rec := do(t, h, http.MethodPost, "/api/v1/attendance/registers", w.token, `{"section_id":"`+w.sectionID+`","date":"`+d+`"}`)
	var reg struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Records []struct {
			StudentID string `json:"student_id"`
			Status    string `json:"status"`
			Version   int    `json:"version"`
		} `json:"records"`
		Counts struct {
			Present int `json:"present"`
		} `json:"counts"`
	}
	mustJSON(t, rec.Body.Bytes(), &reg)
	if rec.Code != http.StatusOK || reg.Status != "open" || reg.Counts.Present != 3 || len(reg.Records) != 3 {
		t.Fatalf("open: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPut, "/api/v1/attendance/registers/"+reg.ID+"/records", w.token,
		`{"marks":[{"student_id":"`+reg.Records[0].StudentID+`","status":"absent"}]}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"absent":1`) {
		t.Fatalf("mark: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPut, "/api/v1/attendance/registers/"+reg.ID+"/records", w.token, `{"marks":[{"student_id":"`+reg.Records[0].StudentID+`","status":"vanished"}]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/attendance/registers/"+reg.ID+"/submit", w.token, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"submitted"`) {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/attendance/registers/"+reg.ID+"/submit", w.token, ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "attendance.already_submitted") {
		t.Fatalf("submit twice: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPut, "/api/v1/attendance/registers/"+reg.ID+"/records", w.token, `{"marks":[{"student_id":"`+reg.Records[1].StudentID+`","status":"absent"}]}`); rec.Code != http.StatusConflict {
		t.Fatalf("mark a submitted register: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/attendance/registers/"+reg.ID+"/corrections", w.token,
		`{"student_id":"`+reg.Records[0].StudentID+`","status":"present","reason":"wrong slip"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"corrected":true`) {
		t.Fatalf("correct: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/attendance/summary?date="+d, w.token, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"school_day":true`) {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body)
	}
	// A missing date is a 400 with the code, only after the permission check.
	if rec := do(t, h, http.MethodGet, "/api/v1/attendance/summary", w.token, ""); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "attendance.date_invalid") {
		t.Fatalf("missing date: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/attendance/summary", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/attendance/settings", w.token, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"register_due_time":"10:00"`) {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPut, "/api/v1/attendance/settings", w.token, `{"register_due_time":"25:00","edit_window_days":7,"low_attendance_percent":75}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "attendance.settings_invalid") {
		t.Fatalf("bad settings: %d %s", rec.Code, rec.Body)
	}
}

// Review of slice 6a (F1): a student who moves section in the middle of a day is counted once that day, in
// the section they are in at the end of it. The old register records them as having left.
func TestUS_ATT_007_ASameDayMoveCountsOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-move", 3)
	second, err := f.org.CreateSection(ctx, w.owner, org.SectionInput{ClassID: mustClassID(t, f, w), Name: "B", Shift: "morning"})
	if err != nil {
		t.Fatal(err)
	}
	mover := w.students[0]

	// A's register opens at 08:00 with the student prefilled present; at 09:00 they move to B.
	a := w.open(t, f, today())
	if _, err := f.students.ChangeSection(ctx, w.owner, mover, second.ID, "moved to B"); err != nil {
		t.Fatal(err)
	}
	// B's register opens after the move and includes them.
	b, err := f.attendance.OpenRegister(ctx, w.owner, second.ID, today())
	if err != nil || statusOf(b, mover) != "present" {
		t.Fatalf("B includes the mover: %v %+v", err, b.Records)
	}
	// Submitting A records the mover as left, and counts only the two who stayed.
	subA := w.submit(t, f, a.ID)
	if subA.Counts.Students != 2 || statusOf(subA, mover) != "left" {
		t.Fatalf("A after submit: %+v mover=%s", subA.Counts, statusOf(subA, mover))
	}
	if _, err := f.attendance.MarkRecords(ctx, w.owner, b.ID, []attendance.Mark{{StudentID: mover, Status: "absent"}}); err != nil {
		t.Fatal(err)
	}
	subB := w.submit(t, f, b.ID)
	if subB.Counts.Students != 1 || subB.Counts.Absent != 1 {
		t.Fatalf("B counts the mover once: %+v", subB.Counts)
	}
	sum, _ := f.attendance.DailySummary(ctx, w.owner, today())
	if sum.Totals.Students != 3 || sum.Totals.Absent != 1 || sum.Totals.Present != 2 {
		t.Fatalf("the school counts three students, not four: %+v", sum.Totals)
	}
	h, _ := f.attendance.StudentHistory(ctx, w.owner, mover, today(), today())
	if len(h.Days) != 1 || h.Days[0].Status != "absent" {
		t.Fatalf("the mover has one day, in B: %+v", h.Days)
	}
	if absent, _ := f.attendance.AbsentOnDay(ctx, w.owner, today()); len(absent) != 1 {
		t.Fatalf("absent once, not twice: %+v", absent)
	}

	// A move AFTER the first register was already submitted: the second section does not count them again.
	w2 := newAttendWorld(t, f, "att-move2", 2)
	sec2, _ := f.org.CreateSection(ctx, w2.owner, org.SectionInput{ClassID: mustClassID(t, f, w2), Name: "B", Shift: "morning"})
	first := w2.open(t, f, today())
	w2.submit(t, f, first.ID)
	if _, err := f.students.ChangeSection(ctx, w2.owner, w2.students[0], sec2.ID, "moved after the register"); err != nil {
		t.Fatal(err)
	}
	late, err := f.attendance.OpenRegister(ctx, w2.owner, sec2.ID, today())
	if err != nil || len(late.Records) != 0 {
		t.Fatalf("already counted in A that day: %v %+v", err, late.Records)
	}
}

// Review of slice 6a (F4, F5): enrolment dates decide the roster, a student who joined after the register
// opened and was never marked is recorded present at submit, and can then be corrected like anyone.
func TestREQ_ATT_015_RosterFollowsEnrolmentDates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-roster", 2)
	w.backdate(t, f, ago(10))

	// A student who withdrew yesterday still belongs to the days before, even though they have left now.
	gone := w.students[1]
	if _, err := f.admin.Exec(ctx, `UPDATE enrolments SET status = 'closed', ended_on = $3, reason = 'withdrawn' WHERE tenant_id = $1 AND student_id = $2`, w.tenantID, gone, ago(1)); err != nil {
		t.Fatal(err)
	}
	before := w.open(t, f, ago(3))
	if len(before.Records) != 2 {
		t.Fatalf("three days ago both were enrolled: %+v", before.Records)
	}
	// On the day they left (ended_on is the first day not enrolled) and after, they are not on the roster.
	for _, d := range []time.Time{ago(1), today()} {
		r := w.open(t, f, d)
		if len(r.Records) != 1 || statusOf(r, gone) != "" {
			t.Fatalf("%s: %+v", d.Format("2006-01-02"), r.Records)
		}
	}

	// A student who joins after the register opened is added as present when it is submitted.
	reg := w.open(t, f, today())
	in := enrolInput(t, w.sectionID)
	in.FullName = "Joined At Noon"
	joiner, err := f.students.Create(ctx, w.owner, in)
	if err != nil {
		t.Fatal(err)
	}
	sub := w.submit(t, f, reg.ID)
	if statusOf(sub, joiner.ID) != "present" || sub.Counts.Students != 2 {
		t.Fatalf("the joiner is recorded present at submit: %+v", sub.Records)
	}
	fixed, err := f.attendance.CorrectRecord(ctx, w.owner, reg.ID, attendance.Correction{StudentID: joiner.ID, Status: "absent", Reason: "was not at school"})
	if err != nil || statusOf(fixed, joiner.ID) != "absent" {
		t.Fatalf("correct the joiner: %v", err)
	}
}

// Review of slice 6a (F2, F7): an old open register cannot be marked or submitted without attendance.edit_any,
// and a day the calendar has since closed cannot be marked either.
func TestREQ_ATT_003_OldOrClosedDaysCannotBeWorked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-worked", 2)
	w.backdate(t, f, ago(30))
	clerk := mustPrincipal(t, f, addStaff(t, f, w.tenantID, w.slug, "attendance_clerk"))

	// The owner opens a 20-day-old register (edit_any) and leaves it open: the clerk cannot work it.
	old := w.open(t, f, ago(20))
	if _, err := f.attendance.MarkRecords(ctx, clerk, old.ID, []attendance.Mark{{StudentID: w.students[0], Status: "absent"}}); errCode(t, err) != "attendance.window_closed" {
		t.Fatalf("the clerk marking an old register: %v", err)
	}
	if _, err := f.attendance.SubmitRegister(ctx, clerk, old.ID); errCode(t, err) != "attendance.window_closed" {
		t.Fatalf("the clerk submitting an old register: %v", err)
	}
	if _, err := f.attendance.SubmitRegister(ctx, w.owner, old.ID); err != nil {
		t.Fatalf("the owner may: %v", err)
	}

	// A holiday added after a register was opened stops further marking and submitting, and keeps what was submitted.
	recent := w.open(t, f, ago(2))
	done := w.open(t, f, ago(3))
	w.submit(t, f, done.ID)
	for _, d := range []time.Time{ago(2), ago(3)} {
		if _, err := f.org.AddCalendarDays(ctx, w.owner, org.CalendarDaysInput{From: d, Kind: "holiday", Name: "Declared later"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.attendance.MarkRecords(ctx, w.owner, recent.ID, []attendance.Mark{{StudentID: w.students[0], Status: "absent"}}); errCode(t, err) != "attendance.not_school_day" {
		t.Fatalf("marking a day that became a holiday: %v", err)
	}
	if _, err := f.attendance.SubmitRegister(ctx, w.owner, recent.ID); errCode(t, err) != "attendance.not_school_day" {
		t.Fatalf("submitting it: %v", err)
	}
	sum, _ := f.attendance.DailySummary(ctx, w.owner, ago(3))
	if sum.SchoolDay || sum.Totals.Students != 2 {
		t.Fatalf("a submitted register keeps its counts and the summary says it was not a school day: %+v", sum)
	}
}

// Review of slice 6a (F8): reading settings or the calendar writes nothing.
func TestREQ_ATT_ReadsDoNotWrite(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "att-lazy")
	p := mustPrincipal(t, f, signIn(t, f, mobile, slug))
	if cfg, err := f.attendance.GetSettings(ctx, p); err != nil || cfg.RegisterDueTime != "10:00" || cfg.EditWindowDays != 7 || cfg.LowAttendancePercent != 75 {
		t.Fatalf("defaults: %v %+v", err, cfg)
	}
	if cal, err := f.org.GetCalendar(ctx, p, time.Time{}, time.Time{}); err != nil || len(cal.WeeklyOffDays) != 1 || cal.WeeklyOffDays[0] != 0 {
		t.Fatalf("default calendar: %v %+v", err, cal)
	}
	var settings, calendar int
	_ = f.admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM attendance_settings WHERE tenant_id = $1), (SELECT count(*) FROM calendar_settings WHERE tenant_id = $1)`, tenantID).Scan(&settings, &calendar)
	if settings != 0 || calendar != 0 {
		t.Fatalf("a read must not create rows: %d %d", settings, calendar)
	}
	// Saving creates them, and saving again changes them.
	if _, err := f.attendance.SetSettings(ctx, p, attendance.SettingsInput{RegisterDueTime: "09:15", EditWindowDays: 3, LowAttendancePercent: 80}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.attendance.SetSettings(ctx, p, attendance.SettingsInput{RegisterDueTime: "09:45", EditWindowDays: 5, LowAttendancePercent: 70}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := f.attendance.GetSettings(ctx, p); cfg.RegisterDueTime != "09:45" || cfg.EditWindowDays != 5 || cfg.LowAttendancePercent != 70 {
		t.Fatalf("saved: %+v", cfg)
	}
	if _, err := f.org.SetWeeklyOff(ctx, p, []int{6}); err != nil {
		t.Fatal(err)
	}
	if cal, _ := f.org.GetCalendar(ctx, p, time.Time{}, time.Time{}); len(cal.WeeklyOffDays) != 1 || cal.WeeklyOffDays[0] != 6 {
		t.Fatalf("saved weekly off: %+v", cal)
	}
}

// Guardian phone numbers on the absent list go only to roles that call parents (attendance.contacts).
func TestREQ_ATT_ContactsOnlyForRolesThatCall(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-contacts", 2)
	reg := w.open(t, f, today())
	w.mark(t, f, reg.ID, attendance.Mark{StudentID: w.students[0], Status: "absent"})
	w.submit(t, f, reg.ID)
	for role, wantContact := range map[string]bool{"attendance_clerk": true, "receptionist": true} {
		p := mustPrincipal(t, f, addStaff(t, f, w.tenantID, w.slug, role))
		list, err := f.attendance.AbsentOnDay(ctx, p, today())
		if err != nil || len(list) != 1 {
			t.Fatalf("%s: %v %+v", role, err, list)
		}
		if (list[0].Contact != nil) != wantContact {
			t.Fatalf("%s: contact present = %v, want %v", role, list[0].Contact != nil, wantContact)
		}
	}
}

// The low-attendance threshold compares the exact ratio: 17 days of 21 is 80.95%, which displays as 81.0 but
// is below an 81% threshold.
func TestREQ_ATT_ThresholdUsesTheExactRatio(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := newAttendWorld(t, f, "att-exact", 1)
	w.backdate(t, f, ago(30))
	stu := w.students[0]
	absentDays := map[int]bool{20: true, 15: true, 9: true, 3: true} // 4 absent of 21 school days
	for d := 21; d >= 1; d-- {
		reg := w.open(t, f, ago(d))
		if absentDays[d] {
			w.mark(t, f, reg.ID, attendance.Mark{StudentID: stu, Status: "absent"})
		}
		w.submit(t, f, reg.ID)
	}
	rep, err := f.attendance.LowAttendance(ctx, w.owner, ago(21), ago(1), 81)
	if err != nil || len(rep.Items) != 1 || rep.Items[0].Totals.Percent != 81 || rep.Items[0].Totals.ExpectedDays != 21 {
		t.Fatalf("80.95%% shows as 81.0 and is still below 81: %v %+v", err, rep.Items)
	}
	if rep, _ := f.attendance.LowAttendance(ctx, w.owner, ago(21), ago(1), 80); len(rep.Items) != 0 {
		t.Fatalf("80.95%% is not below 80: %+v", rep.Items)
	}
}
