package students

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/audit"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/students/store/gen"
)

const (
	minReasonLen = 3
	maxReasonLen = 200
)

// Enrolment is one spell of a student in a section of a session (REQ-STU-008).
// The names come from the org module; this module stores only the ids.
type Enrolment struct {
	ID          string
	SessionID   string
	SessionName string
	SectionID   string
	ClassName   string
	SectionName string
	Shift       string
	Status      string // "active" or "closed"
	StartedOn   time.Time
	EndedOn     *time.Time
	Reason      *string
}

// ChangeSection moves a student to another section. The current enrolment is closed with the
// reason and a new one is opened; nothing is overwritten, so the history stays (US-STU-005).
func (s *Service) ChangeSection(ctx context.Context, p iam.Principal, id, sectionID, reason string) (Student, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < minReasonLen || len(reason) > maxReasonLen {
		return Student{}, apperr.Validation("students.reason_invalid", "A reason of 3 to 200 characters is required.",
			apperr.Field{Field: "reason", Message: "3 to 200 characters."})
	}
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return Student{}, err
	}
	actor, err := db.ParseUUID(p.UserID)
	if err != nil {
		return Student{}, err
	}
	studentID, err := db.ParseUUID(id)
	if err != nil {
		return Student{}, apperr.NotFound("students.not_found", "Student not found.")
	}

	var moved Student
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		if _, err := s.readStudent(ctx, tx, tenant, studentID); err != nil {
			return err // includes the 404
		}
		active, err := q.ActiveEnrolmentForStudent(ctx, gen.ActiveEnrolmentForStudentParams{TenantID: tenant, StudentID: studentID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.Validation("students.not_enrolled", "This student has no current enrolment to move.")
			}
			return fmt.Errorf("find active enrolment: %w", err)
		}
		target, err := s.org.ResolveEnrolmentTarget(ctx, tx, p.TenantID, sectionID)
		if err != nil {
			return err
		}
		if db.UUIDString(active.SectionID) == target.Section.ID && db.UUIDString(active.SessionID) == target.SessionID {
			return apperr.Validation("students.same_section", "The student is already in that section.",
				apperr.Field{Field: "section_id", Message: "Already enrolled here."})
		}

		today := dateOnly(s.now().In(pakistanZone))
		closed, err := q.CloseEnrolment(ctx, gen.CloseEnrolmentParams{
			TenantID: tenant,
			ID:       active.ID,
			EndedOn:  pgtype.Date{Time: today, Valid: true},
			Reason:   pgtype.Text{String: reason, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("close enrolment: %w", err)
		}
		if closed == 0 {
			return apperr.Conflict("students.enrolment_changed", "The enrolment changed while you were moving the student. Try again.")
		}
		sessionID, err := db.ParseUUID(target.SessionID)
		if err != nil {
			return err
		}
		targetSection, err := db.ParseUUID(target.Section.ID)
		if err != nil {
			return err
		}
		if _, err := q.InsertEnrolment(ctx, gen.InsertEnrolmentParams{
			TenantID:  tenant,
			StudentID: studentID,
			SessionID: sessionID,
			SectionID: targetSection,
			StartedOn: pgtype.Date{Time: today, Valid: true},
			CreatedBy: actor,
		}); err != nil {
			return fmt.Errorf("insert enrolment: %w", err)
		}

		if err := audit.Record(ctx, tx, audit.Entry{
			TenantID:    p.TenantID,
			Source:      audit.SourceAPI,
			ActorUserID: p.UserID,
			TokenID:     p.TokenID,
			Action:      "student.section_changed",
			Entity:      "student",
			EntityID:    id,
			Summary: map[string]any{
				"from_section": db.UUIDString(active.SectionID),
				"to_section":   target.Section.ID,
				"reason":       reason,
			},
		}); err != nil {
			return err
		}
		moved, err = s.readStudent(ctx, tx, tenant, studentID)
		return err
	})
	if err != nil {
		return Student{}, err
	}
	return moved, nil
}

// ListEnrolments returns the whole history of one student, newest first (REQ-STU-008).
func (s *Service) ListEnrolments(ctx context.Context, p iam.Principal, id string) ([]Enrolment, error) {
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return nil, err
	}
	studentID, err := db.ParseUUID(id)
	if err != nil {
		return nil, apperr.NotFound("students.not_found", "Student not found.")
	}
	var out []Enrolment
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		if _, err := s.readStudent(ctx, tx, tenant, studentID); err != nil {
			return err
		}
		byStudent, err := s.describeEnrolments(ctx, tx, p.TenantID, tenant, []pgtype.UUID{studentID})
		if err != nil {
			return err
		}
		out = byStudent[id]
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []Enrolment{}
	}
	return out, nil
}

// RequireStudent returns a 404 unless the student exists in the caller's tenant. It runs inside the
// caller's open tenant transaction and is used by modules that hang data off a student.
func (s *Service) RequireStudent(ctx context.Context, tx pgx.Tx, tenantID, studentID string) error {
	notFound := apperr.NotFound("students.not_found", "Student not found.")
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return err
	}
	id, err := db.ParseUUID(studentID)
	if err != nil {
		return notFound
	}
	if _, err := gen.New(tx).GetStudent(ctx, gen.GetStudentParams{TenantID: tenant, ID: id}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound
		}
		return fmt.Errorf("get student: %w", err)
	}
	return nil
}

// CurrentClassID returns the id of the class the student is enrolled in now, inside the caller's
// open tenant transaction. It is what the fees module asks to find a student's class plan. A student
// with no active enrolment is a 400 (students.not_enrolled); an unknown student is a 404.
func (s *Service) CurrentClassID(ctx context.Context, tx pgx.Tx, tenantID, studentID string) (string, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return "", err
	}
	id, err := db.ParseUUID(studentID)
	if err != nil {
		return "", apperr.NotFound("students.not_found", "Student not found.")
	}
	q := gen.New(tx)
	if _, err := q.GetStudent(ctx, gen.GetStudentParams{TenantID: tenant, ID: id}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperr.NotFound("students.not_found", "Student not found.")
		}
		return "", fmt.Errorf("get student: %w", err)
	}
	active, err := q.ActiveEnrolmentForStudent(ctx, gen.ActiveEnrolmentForStudentParams{TenantID: tenant, StudentID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.Validation("students.not_enrolled", "This student has no current enrolment, so has no class.")
	}
	if err != nil {
		return "", fmt.Errorf("find active enrolment: %w", err)
	}
	sectionID := db.UUIDString(active.SectionID)
	sections, err := s.org.DescribeSections(ctx, tx, tenantID, []string{sectionID})
	if err != nil {
		return "", err
	}
	section, ok := sections[sectionID]
	if !ok {
		return "", fmt.Errorf("enrolment points at section %s, which org cannot find", sectionID)
	}
	return section.ClassID, nil
}

// describeEnrolments loads the enrolments of the given students, newest first per student, and
// fills in the session and section names by asking the org module. Keyed by student id.
func (s *Service) describeEnrolments(ctx context.Context, tx pgx.Tx, tenantID string, tenant pgtype.UUID, studentIDs []pgtype.UUID) (map[string][]Enrolment, error) {
	out := make(map[string][]Enrolment)
	if len(studentIDs) == 0 {
		return out, nil
	}
	rows, err := gen.New(tx).ListEnrolmentsForStudents(ctx, gen.ListEnrolmentsForStudentsParams{
		TenantID: tenant, StudentIds: studentIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("load enrolments: %w", err)
	}
	if len(rows) == 0 {
		return out, nil
	}

	sectionIDs := make([]string, 0, len(rows))
	sessionIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		sectionIDs = append(sectionIDs, db.UUIDString(r.SectionID))
		sessionIDs = append(sessionIDs, db.UUIDString(r.SessionID))
	}
	sections, err := s.org.DescribeSections(ctx, tx, tenantID, sectionIDs)
	if err != nil {
		return nil, err
	}
	sessions, err := s.org.DescribeSessions(ctx, tx, tenantID, sessionIDs)
	if err != nil {
		return nil, err
	}

	for _, r := range rows {
		sec := sections[db.UUIDString(r.SectionID)]
		ses := sessions[db.UUIDString(r.SessionID)]
		e := Enrolment{
			ID:          db.UUIDString(r.ID),
			SessionID:   db.UUIDString(r.SessionID),
			SessionName: ses.Name,
			SectionID:   db.UUIDString(r.SectionID),
			ClassName:   sec.ClassName,
			SectionName: sec.Name,
			Shift:       sec.Shift,
			Status:      r.Status,
			StartedOn:   r.StartedOn.Time,
		}
		if r.EndedOn.Valid {
			t := r.EndedOn.Time
			e.EndedOn = &t
		}
		if r.Reason.Valid {
			reason := r.Reason.String
			e.Reason = &reason
		}
		key := db.UUIDString(r.StudentID)
		out[key] = append(out[key], e)
	}
	return out, nil
}

// currentOf returns the newest active enrolment in a newest-first list, or nil if there is none.
func currentOf(list []Enrolment) *Enrolment {
	for i := range list {
		if list[i].Status == "active" {
			return &list[i]
		}
	}
	return nil
}

// dateOnly drops the clock, keeping the calendar date of t.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// BillableStudent is what the fees module needs to bill one student for a month.
type BillableStudent struct {
	ID              string
	FamilyID        string
	RegistrationNo  string
	FullName        string
	ClassID         string
	FirstEnrolledOn time.Time
}

// BillableStudents lists active students whose current enrolment began on or before the given day,
// with their class and the date of their first enrolment. It runs inside the caller's open tenant
// transaction. Deactivated students are left out (BR-FEE-006); their dues stay collectable.
func (s *Service) BillableStudents(ctx context.Context, tx pgx.Tx, tenantID string, onOrBefore time.Time) ([]BillableStudent, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := gen.New(tx).ListBillableStudents(ctx, gen.ListBillableStudentsParams{
		TenantID:   tenant,
		OnOrBefore: pgtype.Date{Time: onOrBefore, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list billable students: %w", err)
	}
	sectionIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		sectionIDs = append(sectionIDs, db.UUIDString(r.SectionID))
	}
	sections, err := s.org.DescribeSections(ctx, tx, tenantID, sectionIDs)
	if err != nil {
		return nil, err
	}
	out := make([]BillableStudent, 0, len(rows))
	for _, r := range rows {
		section, ok := sections[db.UUIDString(r.SectionID)]
		if !ok {
			return nil, fmt.Errorf("enrolment points at section %s, which org cannot find", db.UUIDString(r.SectionID))
		}
		out = append(out, BillableStudent{
			ID: db.UUIDString(r.ID), FamilyID: db.UUIDString(r.FamilyID), RegistrationNo: r.RegistrationNo,
			FullName: r.FullName, ClassID: section.ClassID, FirstEnrolledOn: r.FirstStartedOn.Time,
		})
	}
	return out, nil
}

// FirstEnrolmentDates returns the date each given student first enrolled, keyed by student id, inside
// the caller's open tenant transaction. Unknown students are absent.
func (s *Service) FirstEnrolmentDates(ctx context.Context, tx pgx.Tx, tenantID string, studentIDs []string) (map[string]time.Time, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	ids := make([]pgtype.UUID, 0, len(studentIDs))
	for _, id := range studentIDs {
		u, err := db.ParseUUID(id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, u)
	}
	rows, err := gen.New(tx).FirstEnrolmentDates(ctx, gen.FirstEnrolmentDatesParams{TenantID: tenant, StudentIds: ids})
	if err != nil {
		return nil, fmt.Errorf("first enrolment dates: %w", err)
	}
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		out[db.UUIDString(r.StudentID)] = r.FirstStartedOn.Time
	}
	return out, nil
}

// RequireFamily returns a 404 unless the family exists in the caller's school. It runs inside the
// caller's open tenant transaction; fees uses it before taking a payment (ADR-0013).
func (s *Service) RequireFamily(ctx context.Context, tx pgx.Tx, tenantID, familyID string) error {
	notFound := apperr.NotFound("students.family_not_found", "Family not found.")
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return err
	}
	id, err := db.ParseUUID(familyID)
	if err != nil {
		return notFound
	}
	if _, err := gen.New(tx).FamilyExists(ctx, gen.FamilyExistsParams{TenantID: tenant, ID: id}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound
		}
		return fmt.Errorf("get family: %w", err)
	}
	return nil
}

// FeePayer is the guardian to contact about a family's fees.
type FeePayer struct {
	Name         string
	Mobile       string
	Relationship string
}

// FamilyFeePayers returns, for each given family, the guardian marked as the fee payer, else the primary
// guardian (REQ-FEE-008), inside the caller's open tenant transaction. Families with no guardian are absent.
func (s *Service) FamilyFeePayers(ctx context.Context, tx pgx.Tx, tenantID string, familyIDs []string) (map[string]FeePayer, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	ids := make([]pgtype.UUID, 0, len(familyIDs))
	for _, id := range familyIDs {
		u, err := db.ParseUUID(id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, u)
	}
	rows, err := gen.New(tx).FamilyFeePayers(ctx, gen.FamilyFeePayersParams{TenantID: tenant, FamilyIds: ids})
	if err != nil {
		return nil, fmt.Errorf("family fee payers: %w", err)
	}
	out := make(map[string]FeePayer, len(rows))
	for _, r := range rows {
		out[db.UUIDString(r.FamilyID)] = FeePayer{Name: r.FullName, Mobile: r.Mobile, Relationship: r.Relationship}
	}
	return out, nil
}

// RosterEntry is a student on a section's roster.
type RosterEntry struct {
	StudentID      string
	RegistrationNo string
	FullName       string
}

// SectionRoster lists the active students enrolled in a section on a date, for the attendance register,
// inside the caller's open tenant transaction.
func (s *Service) SectionRoster(ctx context.Context, tx pgx.Tx, tenantID, sectionID string, on time.Time) ([]RosterEntry, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	section, err := db.ParseUUID(sectionID)
	if err != nil {
		return nil, err
	}
	rows, err := gen.New(tx).SectionRoster(ctx, gen.SectionRosterParams{
		TenantID: tenant, SectionID: section, OnDay: pgtype.Date{Time: on, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("section roster: %w", err)
	}
	out := make([]RosterEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, RosterEntry{StudentID: db.UUIDString(r.ID), RegistrationNo: r.RegistrationNo, FullName: r.FullName})
	}
	return out, nil
}

// StudentBrief is the registration number and name of a student.
type StudentBrief struct {
	RegistrationNo string
	FullName       string
}

// StudentBriefs returns the registration number and name of each given student, keyed by id.
func (s *Service) StudentBriefs(ctx context.Context, tx pgx.Tx, tenantID string, ids []string) (map[string]StudentBrief, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	parsed := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		u, err := db.ParseUUID(id)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, u)
	}
	rows, err := gen.New(tx).StudentBriefs(ctx, gen.StudentBriefsParams{TenantID: tenant, Ids: parsed})
	if err != nil {
		return nil, fmt.Errorf("student briefs: %w", err)
	}
	out := make(map[string]StudentBrief, len(rows))
	for _, r := range rows {
		out[db.UUIDString(r.ID)] = StudentBrief{RegistrationNo: r.RegistrationNo, FullName: r.FullName}
	}
	return out, nil
}

// PrimaryContacts returns one guardian per student to call (the primary guardian, else the fee payer).
func (s *Service) PrimaryContacts(ctx context.Context, tx pgx.Tx, tenantID string, studentIDs []string) (map[string]FeePayer, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	parsed := make([]pgtype.UUID, 0, len(studentIDs))
	for _, id := range studentIDs {
		u, err := db.ParseUUID(id)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, u)
	}
	rows, err := gen.New(tx).PrimaryContacts(ctx, gen.PrimaryContactsParams{TenantID: tenant, StudentIds: parsed})
	if err != nil {
		return nil, fmt.Errorf("primary contacts: %w", err)
	}
	out := make(map[string]FeePayer, len(rows))
	for _, r := range rows {
		out[db.UUIDString(r.StudentID)] = FeePayer{Name: r.FullName, Mobile: r.Mobile, Relationship: r.Relationship}
	}
	return out, nil
}
