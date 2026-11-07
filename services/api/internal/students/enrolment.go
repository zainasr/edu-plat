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
