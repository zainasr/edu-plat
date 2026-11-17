package attendance

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/attendance/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/students"
)

// Reports (spec §6). They are worked out from the registers as they stand and use SUBMITTED registers
// only, so a register nobody submitted is never read as all-absent (BR-ATT-005).

const maxReportDays = 366

// MissingRegister is a section whose register has not been submitted.
type MissingRegister struct {
	SectionID       string
	SectionName     string
	HasOpenRegister bool
}

// MissingRegisters is the answer for one date.
type MissingRegisters struct {
	Date      time.Time
	SchoolDay bool
	DueTime   string
	Overdue   bool
	Items     []MissingRegister
}

// SectionDay is one section's counts on a day.
type SectionDay struct {
	SectionID   string
	SectionName string
	Submitted   bool
	Counts      Counts
}

// DailySummary is one day's attendance, section by section.
type DailySummary struct {
	Date      time.Time
	SchoolDay bool
	Totals    Counts
	Sections  []SectionDay
}

// AbsentStudent is a student marked absent, with who to call.
type AbsentStudent struct {
	StudentID      string
	RegistrationNo string
	FullName       string
	SectionID      string
	SectionName    string
	Contact        *Contact
}

// Contact is a guardian to call.
type Contact struct {
	Name         string
	Mobile       string
	Relationship string
}

// Totals is a student's attendance over a period.
type Totals struct {
	Present      int
	Late         int
	Absent       int
	Leave        int
	ExpectedDays int
	Percent      float64
}

func totalsOf(present, late, absent, leave int) Totals {
	exp, pct := attendancePercent(present, late, absent)
	return Totals{Present: present, Late: late, Absent: absent, Leave: leave, ExpectedDays: exp, Percent: pct}
}

// LowAttendance is a student below the threshold.
type LowAttendance struct {
	StudentID      string
	RegistrationNo string
	FullName       string
	Totals         Totals
}

// LowAttendanceReport is the list for a period.
type LowAttendanceReport struct {
	From      time.Time
	To        time.Time
	Threshold int
	Items     []LowAttendance
}

// StudentDay is one day of a student's attendance.
type StudentDay struct {
	Date      time.Time
	SectionID string
	Status    string
	Method    string
	Corrected bool
}

// StudentAttendance is a student's days and totals over a period.
type StudentAttendance struct {
	StudentID string
	From      time.Time
	To        time.Time
	Totals    Totals
	Days      []StudentDay
}

func validRange(from, to time.Time) error {
	if from.IsZero() || to.IsZero() || to.Before(from) || daysBetween(from, to) >= maxReportDays {
		return apperr.Validation("attendance.range_invalid", "Choose a period of up to 366 days that ends on or after it starts.",
			apperr.Field{Field: "to", Message: "Up to 366 days after from."})
	}
	return nil
}

// MissingRegisters lists sections with no submitted register once the due time has passed (US-ATT-008).
func (s *Service) MissingRegisters(ctx context.Context, p iam.Principal, date time.Time) (MissingRegisters, error) {
	if date.IsZero() {
		return MissingRegisters{}, apperr.Validation("attendance.date_invalid", "A date is required.", apperr.Field{Field: "date", Message: "Required."})
	}
	date = dateOnly(date)
	out := MissingRegisters{Date: date, Items: []MissingRegister{}}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		cfg, err := loadSettings(ctx, q, p.TenantID)
		if err != nil {
			return err
		}
		out.DueTime = cfg.RegisterDueTime
		day, err := s.org.SchoolDay(ctx, tx, p.TenantID, date)
		if err != nil {
			return err
		}
		out.SchoolDay = day.SchoolDay
		out.Overdue = registerOverdue(s.now().In(pakistanZone), date, cfg.dueTime())
		if !out.SchoolDay || !out.Overdue {
			return nil
		}
		sections, err := s.org.ActiveSections(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		regs, err := q.ListRegistersForDay(ctx, gen.ListRegistersForDayParams{TenantID: tenant, Day: pgtype.Date{Time: date, Valid: true}})
		if err != nil {
			return fmt.Errorf("list registers: %w", err)
		}
		state := map[string]string{}
		for _, r := range regs {
			state[db.UUIDString(r.SectionID)] = r.Status
		}
		for _, sec := range sections {
			if state[sec.ID] == "submitted" {
				continue
			}
			out.Items = append(out.Items, MissingRegister{SectionID: sec.ID, SectionName: sectionLabel(sec), HasOpenRegister: state[sec.ID] == "open"})
		}
		return nil
	})
	return out, err
}

// DailySummary adds up a day section by section. Open registers are listed but not counted.
func (s *Service) DailySummary(ctx context.Context, p iam.Principal, date time.Time) (DailySummary, error) {
	if date.IsZero() {
		return DailySummary{}, apperr.Validation("attendance.date_invalid", "A date is required.", apperr.Field{Field: "date", Message: "Required."})
	}
	date = dateOnly(date)
	out := DailySummary{Date: date, Sections: []SectionDay{}}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		day, err := s.org.SchoolDay(ctx, tx, p.TenantID, date)
		if err != nil {
			return err
		}
		out.SchoolDay = day.SchoolDay
		rows, err := q.ListRegistersForDay(ctx, gen.ListRegistersForDayParams{TenantID: tenant, Day: pgtype.Date{Time: date, Valid: true}})
		if err != nil {
			return fmt.Errorf("list registers: %w", err)
		}
		regs, err := s.briefs(ctx, tx, q, p.TenantID, tenant, rows)
		if err != nil {
			return err
		}
		for _, r := range regs {
			sd := SectionDay{SectionID: r.SectionID, SectionName: r.SectionName, Submitted: r.Status == "submitted"}
			if sd.Submitted {
				sd.Counts = r.Counts
				out.Totals.Students += r.Counts.Students
				out.Totals.Present += r.Counts.Present
				out.Totals.Absent += r.Counts.Absent
				out.Totals.Late += r.Counts.Late
				out.Totals.Leave += r.Counts.Leave
			}
			out.Sections = append(out.Sections, sd)
		}
		sort.Slice(out.Sections, func(i, j int) bool { return out.Sections[i].SectionName < out.Sections[j].SectionName })
		return nil
	})
	return out, err
}

// AbsentOnDay lists students absent in submitted registers on a date, with the primary guardian to call.
func (s *Service) AbsentOnDay(ctx context.Context, p iam.Principal, date time.Time) ([]AbsentStudent, error) {
	if date.IsZero() {
		return nil, apperr.Validation("attendance.date_invalid", "A date is required.", apperr.Field{Field: "date", Message: "Required."})
	}
	out := []AbsentStudent{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		rows, err := q.AbsentOnDay(ctx, gen.AbsentOnDayParams{TenantID: tenant, Day: pgtype.Date{Time: dateOnly(date), Valid: true}})
		if err != nil {
			return fmt.Errorf("absent on day: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		studentIDs := make([]string, 0, len(rows))
		secIDs := make([]string, 0, len(rows))
		for _, r := range rows {
			studentIDs = append(studentIDs, db.UUIDString(r.StudentID))
			secIDs = append(secIDs, db.UUIDString(r.SectionID))
		}
		briefs, err := s.students.StudentBriefs(ctx, tx, p.TenantID, studentIDs)
		if err != nil {
			return err
		}
		var contacts map[string]students.FeePayer
		if p.Can(iam.PermAttendanceContacts) { // a guardian's mobile goes only to staff who call parents
			if contacts, err = s.students.PrimaryContacts(ctx, tx, p.TenantID, studentIDs); err != nil {
				return err
			}
		}
		sections, err := s.org.DescribeSections(ctx, tx, p.TenantID, secIDs)
		if err != nil {
			return err
		}
		for _, r := range rows {
			id, sec := db.UUIDString(r.StudentID), db.UUIDString(r.SectionID)
			a := AbsentStudent{StudentID: id, RegistrationNo: briefs[id].RegistrationNo, FullName: briefs[id].FullName, SectionID: sec, SectionName: sectionLabel(sections[sec])}
			if c, ok := contacts[id]; ok {
				a.Contact = &Contact{Name: c.Name, Mobile: c.Mobile, Relationship: c.Relationship}
			}
			out = append(out, a)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].SectionName != out[j].SectionName {
				return out[i].SectionName < out[j].SectionName
			}
			return out[i].RegistrationNo < out[j].RegistrationNo
		})
		return nil
	})
	return out, err
}

// LowAttendance lists students below the threshold over a period, lowest first. Threshold 0 means the
// school's setting.
func (s *Service) LowAttendance(ctx context.Context, p iam.Principal, from, to time.Time, threshold int) (LowAttendanceReport, error) {
	if err := validRange(from, to); err != nil {
		return LowAttendanceReport{}, err
	}
	if threshold != 0 && (threshold < 1 || threshold > 100) {
		return LowAttendanceReport{}, apperr.Validation("attendance.range_invalid", "threshold must be between 1 and 100.",
			apperr.Field{Field: "threshold", Message: "Between 1 and 100."})
	}
	out := LowAttendanceReport{From: dateOnly(from), To: dateOnly(to), Items: []LowAttendance{}}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		if threshold == 0 {
			cfg, err := loadSettings(ctx, q, p.TenantID)
			if err != nil {
				return err
			}
			threshold = cfg.LowAttendancePercent
		}
		out.Threshold = threshold
		rows, err := q.TotalsPerStudent(ctx, gen.TotalsPerStudentParams{
			TenantID: tenant, FromDay: pgtype.Date{Time: out.From, Valid: true}, ToDay: pgtype.Date{Time: out.To, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("totals per student: %w", err)
		}
		var low []LowAttendance
		var studentIDs []string
		for _, r := range rows {
			t := totalsOf(int(r.Present), int(r.Late), int(r.Absent), int(r.Leave))
			if belowThreshold(int(r.Present), int(r.Late), int(r.Absent), threshold) {
				id := db.UUIDString(r.StudentID)
				low = append(low, LowAttendance{StudentID: id, Totals: t})
				studentIDs = append(studentIDs, id)
			}
		}
		if len(low) == 0 {
			return nil
		}
		briefs, err := s.students.StudentBriefs(ctx, tx, p.TenantID, studentIDs)
		if err != nil {
			return err
		}
		for i := range low {
			b := briefs[low[i].StudentID]
			low[i].RegistrationNo, low[i].FullName = b.RegistrationNo, b.FullName
		}
		sort.Slice(low, func(i, j int) bool {
			if low[i].Totals.Percent != low[j].Totals.Percent {
				return low[i].Totals.Percent < low[j].Totals.Percent
			}
			return low[i].RegistrationNo < low[j].RegistrationNo
		})
		out.Items = low
		return nil
	})
	return out, err
}

// StudentHistory returns one student's days and totals over a period.
func (s *Service) StudentHistory(ctx context.Context, p iam.Principal, studentID string, from, to time.Time) (StudentAttendance, error) {
	if err := validRange(from, to); err != nil {
		return StudentAttendance{}, err
	}
	out := StudentAttendance{StudentID: studentID, From: dateOnly(from), To: dateOnly(to), Days: []StudentDay{}}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		if err := s.students.RequireStudent(ctx, tx, p.TenantID, studentID); err != nil {
			return err
		}
		student, err := db.ParseUUID(studentID)
		if err != nil {
			return err
		}
		rows, err := q.StudentDays(ctx, gen.StudentDaysParams{
			TenantID: tenant, StudentID: student, FromDay: pgtype.Date{Time: out.From, Valid: true}, ToDay: pgtype.Date{Time: out.To, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("student days: %w", err)
		}
		var present, late, absent, leave int
		for _, r := range rows {
			out.Days = append(out.Days, StudentDay{Date: r.Day.Time, SectionID: db.UUIDString(r.SectionID), Status: r.Status, Method: r.Method, Corrected: r.Corrected})
			switch r.Status {
			case "present":
				present++
			case "late":
				late++
			case "absent":
				absent++
			case "leave":
				leave++
			}
		}
		out.Totals = totalsOf(present, late, absent, leave)
		return nil
	})
	return out, err
}
