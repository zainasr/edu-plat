package attendance

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/attendance/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/audit"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/students"
)

var pakistanZone = time.FixedZone("PKT", 5*60*60)

// Service holds the attendance rules. It asks org for sections and the school calendar, and students for
// rosters and names, inside its own transaction, and never reads their tables (ADR-0013).
type Service struct {
	pool     *pgxpool.Pool
	org      *org.Service
	students *students.Service
	now      func() time.Time
}

// NewService builds the attendance service.
func NewService(pool *pgxpool.Pool, structure *org.Service, enrolment *students.Service) *Service {
	return &Service{pool: pool, org: structure, students: enrolment, now: time.Now}
}

// Settings are the school's attendance settings.
type Settings struct {
	RegisterDueTime      string // HH:MM
	EditWindowDays       int
	LowAttendancePercent int
}

// Record is a student's current status in a register.
type Record struct {
	StudentID      string
	RegistrationNo string
	FullName       string
	Status         string
	Method         string // prefill, manual or correction
	Version        int
	Corrected      bool
}

// Register is one section's attendance for one day.
type Register struct {
	ID          string
	SectionID   string
	SectionName string
	Date        time.Time
	Status      string // open or submitted
	SubmittedAt *time.Time
	Counts      Counts
	Records     []Record
}

var (
	errRegisterNotFound = apperr.NotFound("attendance.register_not_found", "Register not found.")
	errSubmitted        = apperr.Conflict("attendance.register_submitted", "This register has been submitted. Change it with a correction and a reason.")
)

// ---- settings --------------------------------------------------------------------------

// GetSettings returns the settings; until they are saved the defaults apply (10:00, 7 days, 75%).
func (s *Service) GetSettings(ctx context.Context, p iam.Principal) (Settings, error) {
	var out Settings
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = loadSettings(ctx, gen.New(tx), p.TenantID)
		return err
	})
	return out, err
}

func loadSettings(ctx context.Context, q *gen.Queries, tenantID string) (Settings, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return Settings{}, err
	}
	r, err := q.GetSettings(ctx, tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		// Never saved: the defaults apply, and a read writes nothing.
		return Settings{RegisterDueTime: "10:00", EditWindowDays: defaultWindow, LowAttendancePercent: defaultThreshold}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("get settings: %w", err)
	}
	mins := int(r.RegisterDueTime.Microseconds / 60_000_000)
	return Settings{
		RegisterDueTime: fmt.Sprintf("%02d:%02d", mins/60, mins%60), EditWindowDays: int(r.EditWindowDays),
		LowAttendancePercent: int(r.LowAttendancePercent),
	}, nil
}

func (s Settings) dueTime() time.Time {
	var h, m int
	_, _ = fmt.Sscanf(s.RegisterDueTime, "%d:%d", &h, &m)
	return time.Date(0, 1, 1, h, m, 0, 0, time.UTC)
}

// SetSettings changes the settings, audited.
func (s *Service) SetSettings(ctx context.Context, p iam.Principal, in SettingsInput) (Settings, error) {
	parsed, err := validateSettings(in)
	if err != nil {
		return Settings{}, err
	}
	var out Settings
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		actor, err := db.ParseUUID(p.UserID)
		if err != nil {
			return err
		}
		due := pgtype.Time{Microseconds: int64(parsed.DueHour*60+parsed.DueMinute) * 60_000_000, Valid: true}
		if err := q.SetSettings(ctx, gen.SetSettingsParams{
			TenantID: tenant, RegisterDueTime: due, EditWindowDays: int32(parsed.EditWindowDays),
			LowAttendancePercent: int32(parsed.LowAttendancePercent), UpdatedBy: actor,
		}); err != nil {
			return fmt.Errorf("set settings: %w", err)
		}
		if err := s.audit(ctx, tx, p, "attendance.settings_changed", "attendance_settings", p.TenantID, map[string]any{
			"register_due_time": in.RegisterDueTime, "edit_window_days": parsed.EditWindowDays, "low_attendance_percent": parsed.LowAttendancePercent,
		}); err != nil {
			return err
		}
		out, err = loadSettings(ctx, q, p.TenantID)
		return err
	})
	return out, err
}

// ---- registers -------------------------------------------------------------------------

// OpenRegister opens a section's register for a date, pre-filled with everyone enrolled that day marked
// present, so only the exceptions need changing (REQ-ATT-015). Opening twice returns the same register.
func (s *Service) OpenRegister(ctx context.Context, p iam.Principal, sectionID string, date time.Time) (Register, error) {
	var out Register
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		cfg, err := loadSettings(ctx, q, p.TenantID)
		if err != nil {
			return err
		}
		today := s.now().In(pakistanZone)
		if err := canOpen(date, today, cfg.EditWindowDays, p.Can(iam.PermAttendanceEditAny)); err != nil {
			return err
		}
		date = dateOnly(date)
		sections, err := s.org.DescribeSections(ctx, tx, p.TenantID, []string{sectionID})
		if err != nil {
			return apperr.NotFound("attendance.section_not_found", "Section not found.")
		}
		section, ok := sections[sectionID]
		if !ok {
			return apperr.NotFound("attendance.section_not_found", "Section not found.")
		}
		if section.Status != "active" {
			return apperr.Conflict("attendance.section_archived", "This section is archived.")
		}
		day, err := s.org.SchoolDay(ctx, tx, p.TenantID, date)
		if err != nil {
			return err
		}
		if !day.SchoolDay {
			return apperr.Conflict("attendance.not_school_day", "No attendance on this day: "+day.Reason+". (BR-ATT-001)")
		}
		sectionUUID, err := db.ParseUUID(sectionID)
		if err != nil {
			return err
		}
		pgDate := pgtype.Date{Time: date, Valid: true}
		id, err := q.InsertRegister(ctx, gen.InsertRegisterParams{TenantID: tenant, SectionID: sectionUUID, Day: pgDate, CreatedBy: actor})
		if err == nil {
			roster, err := s.students.SectionRoster(ctx, tx, p.TenantID, sectionID, date)
			if err != nil {
				return err
			}
			rosterIDs := make([]string, 0, len(roster))
			for _, r := range roster {
				rosterIDs = append(rosterIDs, r.StudentID)
			}
			elsewhere, err := countedElsewhere(ctx, q, tenant, sectionUUID, pgDate, rosterIDs)
			if err != nil {
				return err
			}
			prefill := prefillIDs(rosterIDs, elsewhere)
			for _, sid := range prefill {
				student, err := db.ParseUUID(sid)
				if err != nil {
					return err
				}
				if err := q.InsertRecord(ctx, gen.InsertRecordParams{
					TenantID: tenant, RegisterID: id, StudentID: student, Version: 1, Status: "present", Method: "prefill", ChangedBy: actor,
				}); err != nil {
					return fmt.Errorf("prefill record: %w", err)
				}
			}
			if err := s.audit(ctx, tx, p, "attendance.register_opened", "register", db.UUIDString(id), map[string]any{
				"section_id": sectionID, "date": date.Format("2006-01-02"), "students": len(prefill),
			}); err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("insert register: %w", err)
		}
		reg, err := q.RegisterBySectionDay(ctx, gen.RegisterBySectionDayParams{TenantID: tenant, SectionID: sectionUUID, Day: pgDate})
		if err != nil {
			return fmt.Errorf("read register: %w", err)
		}
		out, err = s.readRegister(ctx, tx, q, p.TenantID, tenant, regRow(reg))
		return err
	})
	return out, err
}

type regRow = gen.GetRegisterRow

// GetRegister returns one register with every student's current status.
func (s *Service) GetRegister(ctx context.Context, p iam.Principal, id string) (Register, error) {
	var out Register
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		uid, err := db.ParseUUID(id)
		if err != nil {
			return errRegisterNotFound
		}
		row, err := q.GetRegister(ctx, gen.GetRegisterParams{TenantID: tenant, ID: uid})
		if errors.Is(err, pgx.ErrNoRows) {
			return errRegisterNotFound
		}
		if err != nil {
			return fmt.Errorf("get register: %w", err)
		}
		out, err = s.readRegister(ctx, tx, q, p.TenantID, tenant, row)
		return err
	})
	return out, err
}

// ListRegisters returns the registers opened for a date, without records.
func (s *Service) ListRegisters(ctx context.Context, p iam.Principal, date time.Time) ([]Register, error) {
	if date.IsZero() {
		return nil, apperr.Validation("attendance.date_invalid", "A date is required.", apperr.Field{Field: "date", Message: "Required."})
	}
	var out []Register
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, _, err := ids(p)
		if err != nil {
			return err
		}
		rows, err := q.ListRegistersForDay(ctx, gen.ListRegistersForDayParams{TenantID: tenant, Day: pgtype.Date{Time: dateOnly(date), Valid: true}})
		if err != nil {
			return fmt.Errorf("list registers: %w", err)
		}
		out, err = s.briefs(ctx, tx, q, p.TenantID, tenant, rows)
		sort.Slice(out, func(i, j int) bool { return out[i].SectionName < out[j].SectionName })
		return err
	})
	return out, err
}

// MarkRecords changes students' statuses in an OPEN register. A mark equal to the current status changes
// nothing; every real change adds a version (BR-ATT-002). A student who joined the section after the
// register was opened can be added if they are enrolled in it that day.
func (s *Service) MarkRecords(ctx context.Context, p iam.Principal, registerID string, marks []Mark) (Register, error) {
	if err := validateMarks(marks); err != nil {
		return Register{}, err
	}
	var out Register
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		uid, err := db.ParseUUID(registerID)
		if err != nil {
			return errRegisterNotFound
		}
		reg, err := q.LockRegister(ctx, gen.LockRegisterParams{TenantID: tenant, ID: uid})
		if errors.Is(err, pgx.ErrNoRows) {
			return errRegisterNotFound
		}
		if err != nil {
			return fmt.Errorf("lock register: %w", err)
		}
		if reg.Status != "open" {
			return errSubmitted
		}
		if err := s.checkWorkable(ctx, tx, q, p, reg.Day.Time); err != nil {
			return err
		}
		current, err := currentMap(ctx, q, tenant, uid)
		if err != nil {
			return err
		}
		var roster map[string]bool
		changed := 0
		for _, m := range marks {
			student, err := db.ParseUUID(m.StudentID)
			if err != nil {
				return err
			}
			cur, known := current[m.StudentID]
			if !known {
				if roster == nil {
					list, err := s.students.SectionRoster(ctx, tx, p.TenantID, db.UUIDString(reg.SectionID), reg.Day.Time)
					if err != nil {
						return err
					}
					roster = make(map[string]bool, len(list))
					for _, r := range list {
						roster[r.StudentID] = true
					}
				}
				if !roster[m.StudentID] {
					return apperr.Validation("attendance.student_not_in_section", "A student is not enrolled in this section on this day.",
						apperr.Field{Field: "marks", Message: "Student " + m.StudentID + " is not in this section."})
				}
			} else if cur.status == m.Status {
				continue
			}
			version := int32(1)
			if known {
				version = cur.version + 1
			}
			if err := q.InsertRecord(ctx, gen.InsertRecordParams{
				TenantID: tenant, RegisterID: uid, StudentID: student, Version: version, Status: m.Status, Method: "manual", ChangedBy: actor,
			}); err != nil {
				return fmt.Errorf("insert record: %w", err)
			}
			changed++
		}
		if changed > 0 {
			if err := s.audit(ctx, tx, p, "attendance.marked", "register", registerID, map[string]any{"changed": changed}); err != nil {
				return err
			}
		}
		out, err = s.readRegister(ctx, tx, q, p.TenantID, tenant, gen.GetRegisterRow(reg))
		return err
	})
	return out, err
}

// SubmitRegister submits an open register, once (BR-ATT-005).
func (s *Service) SubmitRegister(ctx context.Context, p iam.Principal, registerID string) (Register, error) {
	var out Register
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		uid, err := db.ParseUUID(registerID)
		if err != nil {
			return errRegisterNotFound
		}
		reg, err := q.LockRegister(ctx, gen.LockRegisterParams{TenantID: tenant, ID: uid})
		if errors.Is(err, pgx.ErrNoRows) {
			return errRegisterNotFound
		}
		if err != nil {
			return fmt.Errorf("lock register: %w", err)
		}
		if reg.Status != "open" {
			return apperr.Conflict("attendance.already_submitted", "This register has already been submitted.")
		}
		if err := s.checkWorkable(ctx, tx, q, p, reg.Day.Time); err != nil {
			return err
		}
		if err := s.reconcile(ctx, tx, q, p, tenant, actor, reg); err != nil {
			return err
		}
		n, err := q.SubmitRegister(ctx, gen.SubmitRegisterParams{TenantID: tenant, ID: uid, SubmittedBy: actor})
		if err != nil {
			return fmt.Errorf("submit register: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("submit register: %d rows changed, want 1", n)
		}
		if err := s.audit(ctx, tx, p, "attendance.register_submitted", "register", registerID, map[string]any{}); err != nil {
			return err
		}
		row, err := q.GetRegister(ctx, gen.GetRegisterParams{TenantID: tenant, ID: uid})
		if err != nil {
			return fmt.Errorf("read register: %w", err)
		}
		out, err = s.readRegister(ctx, tx, q, p.TenantID, tenant, row)
		return err
	})
	return out, err
}

// CorrectRecord changes one student's status in a SUBMITTED register, with a reason. The old value stays
// as history (US-ATT-005). The caller's permission decides how far back it may go (REQ-ATT-003).
func (s *Service) CorrectRecord(ctx context.Context, p iam.Principal, registerID string, in Correction) (Register, error) {
	in, err := validateCorrection(in)
	if err != nil {
		return Register{}, err
	}
	var out Register
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, actor, err := ids(p)
		if err != nil {
			return err
		}
		uid, err := db.ParseUUID(registerID)
		if err != nil {
			return errRegisterNotFound
		}
		reg, err := q.LockRegister(ctx, gen.LockRegisterParams{TenantID: tenant, ID: uid})
		if errors.Is(err, pgx.ErrNoRows) {
			return errRegisterNotFound
		}
		if err != nil {
			return fmt.Errorf("lock register: %w", err)
		}
		cfg, err := loadSettings(ctx, q, p.TenantID)
		if err != nil {
			return err
		}
		if err := canCorrect(reg.Day.Time, s.now().In(pakistanZone), cfg.EditWindowDays,
			p.Can(iam.PermAttendanceEditWindow), p.Can(iam.PermAttendanceEditAny)); err != nil {
			return err
		}
		if reg.Status != "submitted" {
			return apperr.Conflict("attendance.register_open", "This register is still open. Mark it directly; corrections are for submitted registers.")
		}
		current, err := currentMap(ctx, q, tenant, uid)
		if err != nil {
			return err
		}
		cur, ok := current[in.StudentID]
		if !ok {
			return apperr.NotFound("attendance.record_not_found", "This student has no record in this register.")
		}
		if cur.status == in.Status {
			return apperr.Conflict("attendance.no_change", "The student is already marked "+in.Status+".")
		}
		student, err := db.ParseUUID(in.StudentID)
		if err != nil {
			return err
		}
		if err := q.InsertRecord(ctx, gen.InsertRecordParams{
			TenantID: tenant, RegisterID: uid, StudentID: student, Version: cur.version + 1, Status: in.Status, Method: "correction",
			Reason: pgtype.Text{String: in.Reason, Valid: true}, ChangedBy: actor,
		}); err != nil {
			return fmt.Errorf("insert correction: %w", err)
		}
		if err := s.audit(ctx, tx, p, "attendance.corrected", "register", registerID, map[string]any{
			"student_id": in.StudentID, "from": cur.status, "to": in.Status, "reason": in.Reason, "date": reg.Day.Time.Format("2006-01-02"),
		}); err != nil {
			return err
		}
		out, err = s.readRegister(ctx, tx, q, p.TenantID, tenant, gen.GetRegisterRow(reg))
		return err
	})
	return out, err
}

// ---- helpers ---------------------------------------------------------------------------

func ids(p iam.Principal) (tenant, actor pgtype.UUID, err error) {
	if tenant, err = db.ParseUUID(p.TenantID); err != nil {
		return
	}
	actor, err = db.ParseUUID(p.UserID)
	return
}

type current struct {
	status  string
	version int32
}

func currentMap(ctx context.Context, q *gen.Queries, tenant, register pgtype.UUID) (map[string]current, error) {
	rows, err := q.CurrentRecords(ctx, gen.CurrentRecordsParams{TenantID: tenant, RegisterIds: []pgtype.UUID{register}})
	if err != nil {
		return nil, fmt.Errorf("current records: %w", err)
	}
	out := make(map[string]current, len(rows))
	for _, r := range rows {
		out[db.UUIDString(r.StudentID)] = current{status: r.Status, version: r.Version}
	}
	return out, nil
}

func sectionLabel(sec org.Section) string {
	return fmt.Sprintf("%s %s (%s)", sec.ClassName, sec.Name, sec.Shift)
}

// readRegister builds a register with every student's current record, names, and counts.
func (s *Service) readRegister(ctx context.Context, tx pgx.Tx, q *gen.Queries, tenantID string, tenant pgtype.UUID, row regRow) (Register, error) {
	secID := db.UUIDString(row.SectionID)
	sections, err := s.org.DescribeSections(ctx, tx, tenantID, []string{secID})
	if err != nil {
		return Register{}, err
	}
	rows, err := q.CurrentRecords(ctx, gen.CurrentRecordsParams{TenantID: tenant, RegisterIds: []pgtype.UUID{row.ID}})
	if err != nil {
		return Register{}, fmt.Errorf("current records: %w", err)
	}
	studentIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		studentIDs = append(studentIDs, db.UUIDString(r.StudentID))
	}
	briefs, err := s.students.StudentBriefs(ctx, tx, tenantID, studentIDs)
	if err != nil {
		return Register{}, err
	}
	reg := Register{ID: db.UUIDString(row.ID), SectionID: secID, SectionName: sectionLabel(sections[secID]), Date: row.Day.Time, Status: row.Status, Records: []Record{}}
	if row.SubmittedAt.Valid {
		t := row.SubmittedAt.Time
		reg.SubmittedAt = &t
	}
	statuses := make([]string, 0, len(rows))
	for _, r := range rows {
		id := db.UUIDString(r.StudentID)
		b := briefs[id]
		reg.Records = append(reg.Records, Record{
			StudentID: id, RegistrationNo: b.RegistrationNo, FullName: b.FullName, Status: r.Status, Method: r.Method,
			Version: int(r.Version), Corrected: r.Corrected,
		})
		statuses = append(statuses, r.Status)
	}
	sort.Slice(reg.Records, func(i, j int) bool {
		if reg.Records[i].RegistrationNo != reg.Records[j].RegistrationNo {
			return reg.Records[i].RegistrationNo < reg.Records[j].RegistrationNo
		}
		return reg.Records[i].StudentID < reg.Records[j].StudentID
	})
	reg.Counts = tally(statuses)
	return reg, nil
}

// briefs builds registers without records, with their counts (from current records).
func (s *Service) briefs(ctx context.Context, tx pgx.Tx, q *gen.Queries, tenantID string, tenant pgtype.UUID, rows []gen.ListRegistersForDayRow) ([]Register, error) {
	out := make([]Register, 0, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	regIDs := make([]pgtype.UUID, 0, len(rows))
	secIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		regIDs = append(regIDs, r.ID)
		secIDs = append(secIDs, db.UUIDString(r.SectionID))
	}
	sections, err := s.org.DescribeSections(ctx, tx, tenantID, secIDs)
	if err != nil {
		return nil, err
	}
	recs, err := q.CurrentRecords(ctx, gen.CurrentRecordsParams{TenantID: tenant, RegisterIds: regIDs})
	if err != nil {
		return nil, fmt.Errorf("current records: %w", err)
	}
	byReg := map[string][]string{}
	for _, r := range recs {
		k := db.UUIDString(r.RegisterID)
		byReg[k] = append(byReg[k], r.Status)
	}
	for _, r := range rows {
		id := db.UUIDString(r.ID)
		sec := db.UUIDString(r.SectionID)
		reg := Register{ID: id, SectionID: sec, SectionName: sectionLabel(sections[sec]), Date: r.Day.Time, Status: r.Status, Counts: tally(byReg[id])}
		if r.SubmittedAt.Valid {
			t := r.SubmittedAt.Time
			reg.SubmittedAt = &t
		}
		out = append(out, reg)
	}
	return out, nil
}

func (s *Service) audit(ctx context.Context, tx pgx.Tx, p iam.Principal, action, entity, entityID string, summary map[string]any) error {
	return audit.Record(ctx, tx, audit.Entry{
		TenantID: p.TenantID, Source: audit.SourceAPI, ActorUserID: p.UserID, TokenID: p.TokenID,
		Action: action, Entity: entity, EntityID: entityID, Summary: summary,
	})
}

// checkWorkable refuses to mark or submit a register that is older than the correction window (unless the
// caller holds attendance.edit_any: REQ-ATT-003) or that sits on a day the calendar has since made a
// non-school day (BR-ATT-001). Submitted data stays as it is; this only guards new changes.
func (s *Service) checkWorkable(ctx context.Context, tx pgx.Tx, q *gen.Queries, p iam.Principal, date time.Time) error {
	cfg, err := loadSettings(ctx, q, p.TenantID)
	if err != nil {
		return err
	}
	if err := canOpen(date, s.now().In(pakistanZone), cfg.EditWindowDays, p.Can(iam.PermAttendanceEditAny)); err != nil {
		return err
	}
	day, err := s.org.SchoolDay(ctx, tx, p.TenantID, date)
	if err != nil {
		return err
	}
	if !day.SchoolDay {
		return apperr.Conflict("attendance.not_school_day", "No attendance on this day: "+day.Reason+". (BR-ATT-001)")
	}
	return nil
}

// reconcile brings a register in line with who was enrolled in the section that day, just before it is
// submitted: students who joined and were never marked are recorded present, and students who moved or
// withdrew are recorded as left (they are counted nowhere). Anyone already counted in another section's
// submitted register that day is not added, so a same-day move counts once.
func (s *Service) reconcile(ctx context.Context, tx pgx.Tx, q *gen.Queries, p iam.Principal, tenant, actor pgtype.UUID, reg gen.LockRegisterRow) error {
	roster, err := s.students.SectionRoster(ctx, tx, p.TenantID, db.UUIDString(reg.SectionID), reg.Day.Time)
	if err != nil {
		return err
	}
	rosterIDs := make([]string, 0, len(roster))
	for _, r := range roster {
		rosterIDs = append(rosterIDs, r.StudentID)
	}
	cur, err := currentMap(ctx, q, tenant, reg.ID)
	if err != nil {
		return err
	}
	statuses := make(map[string]string, len(cur))
	for id, c := range cur {
		statuses[id] = c.status
	}
	elsewhere, err := countedElsewhere(ctx, q, tenant, reg.SectionID, reg.Day, rosterIDs)
	if err != nil {
		return err
	}
	add, left := reconcileRoster(rosterIDs, statuses, elsewhere)
	for _, id := range add {
		student, err := db.ParseUUID(id)
		if err != nil {
			return err
		}
		if err := q.InsertRecord(ctx, gen.InsertRecordParams{
			TenantID: tenant, RegisterID: reg.ID, StudentID: student, Version: 1, Status: "present", Method: "prefill", ChangedBy: actor,
		}); err != nil {
			return fmt.Errorf("add roster student: %w", err)
		}
	}
	for _, id := range left {
		student, err := db.ParseUUID(id)
		if err != nil {
			return err
		}
		if err := q.InsertRecord(ctx, gen.InsertRecordParams{
			TenantID: tenant, RegisterID: reg.ID, StudentID: student, Version: cur[id].version + 1, Status: "left", Method: "manual", ChangedBy: actor,
		}); err != nil {
			return fmt.Errorf("mark left: %w", err)
		}
	}
	if len(add)+len(left) > 0 {
		return s.audit(ctx, tx, p, "attendance.reconciled", "register", db.UUIDString(reg.ID), map[string]any{"added": len(add), "left": len(left)})
	}
	return nil
}

// countedElsewhere returns which of the given students are already counted in another section's
// submitted register on the same day.
func countedElsewhere(ctx context.Context, q *gen.Queries, tenant, section pgtype.UUID, day pgtype.Date, studentIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(studentIDs) == 0 {
		return out, nil
	}
	ids := make([]pgtype.UUID, 0, len(studentIDs))
	for _, id := range studentIDs {
		u, err := db.ParseUUID(id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, u)
	}
	rows, err := q.CountedElsewhere(ctx, gen.CountedElsewhereParams{TenantID: tenant, Day: day, SectionID: section, StudentIds: ids})
	if err != nil {
		return nil, fmt.Errorf("counted elsewhere: %w", err)
	}
	for _, r := range rows {
		out[db.UUIDString(r)] = true
	}
	return out, nil
}
