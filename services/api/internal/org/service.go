package org

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/audit"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// Session is an academic year of a branch.
type Session struct {
	ID        string
	Name      string
	StartsOn  time.Time
	EndsOn    time.Time
	IsCurrent bool
}

// Class is a class of a branch, such as "Class 5".
type Class struct {
	ID         string
	Name       string
	GradeLevel int
	Status     string
}

// Section is a section of a class in one shift.
type Section struct {
	ID        string
	ClassID   string
	ClassName string
	Name      string
	Shift     string
	Status    string
}

// EnrolmentTarget is where a student can be enrolled right now: an active section of an
// active class, in the branch's current session.
type EnrolmentTarget struct {
	SessionID   string
	SessionName string
	Section     Section
}

// Service holds the structure rules.
type Service struct {
	pool *pgxpool.Pool
}

// NewService builds the org service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

var errNotSetUp = apperr.Validation("org.not_set_up", "Create a session, a class and a section first.")

// ---- sessions -------------------------------------------------------------------------

// CreateSession adds an academic session. The first session of a branch becomes current;
// later ones are created not current (BR-ORG-001).
func (s *Service) CreateSession(ctx context.Context, p iam.Principal, in SessionInput) (Session, error) {
	in, err := validateSession(in)
	if err != nil {
		return Session{}, err
	}
	var out Session
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, branch, actor, err := s.writeScope(ctx, q, p)
		if err != nil {
			return err
		}
		_, err = q.CurrentSession(ctx, gen.CurrentSessionParams{TenantID: tenant, BranchID: branch})
		isCurrent := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !isCurrent {
			return fmt.Errorf("check current session: %w", err)
		}
		row, err := q.InsertSession(ctx, gen.InsertSessionParams{
			TenantID:  tenant,
			BranchID:  branch,
			Name:      in.Name,
			StartsOn:  pgtype.Date{Time: in.StartsOn, Valid: true},
			EndsOn:    pgtype.Date{Time: in.EndsOn, Valid: true},
			IsCurrent: isCurrent,
			CreatedBy: actor,
		})
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("org.session_exists", "A session with this name already exists.")
		}
		if err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		out = Session{ID: db.UUIDString(row.ID), Name: row.Name, StartsOn: row.StartsOn.Time, EndsOn: row.EndsOn.Time, IsCurrent: row.IsCurrent}
		return s.audit(ctx, tx, p, "session.created", "session", out.ID, map[string]any{"name": out.Name, "current": out.IsCurrent})
	})
	return out, err
}

// ListSessions returns the branch's sessions, newest first. A tenant with no branch yet has none.
func (s *Service) ListSessions(ctx context.Context, p iam.Principal) ([]Session, error) {
	out := []Session{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		branch, err := q.DefaultBranch(ctx, tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find branch: %w", err)
		}
		rows, err := q.ListSessions(ctx, gen.ListSessionsParams{TenantID: tenant, BranchID: branch})
		if err != nil {
			return fmt.Errorf("list sessions: %w", err)
		}
		for _, r := range rows {
			out = append(out, Session{ID: db.UUIDString(r.ID), Name: r.Name, StartsOn: r.StartsOn.Time, EndsOn: r.EndsOn.Time, IsCurrent: r.IsCurrent})
		}
		return nil
	})
	return out, err
}

// MakeSessionCurrent switches the branch's current session. The old one is cleared in the same
// transaction, so there is never a moment with two (BR-ORG-001).
func (s *Service) MakeSessionCurrent(ctx context.Context, p iam.Principal, id string) (Session, error) {
	sessionID, err := db.ParseUUID(id)
	if err != nil {
		return Session{}, apperr.NotFound("org.session_not_found", "Session not found.")
	}
	var out Session
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		branch, err := q.DefaultBranch(ctx, tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("org.session_not_found", "Session not found.")
		}
		if err != nil {
			return fmt.Errorf("find branch: %w", err)
		}
		if err := q.ClearCurrentSession(ctx, gen.ClearCurrentSessionParams{TenantID: tenant, BranchID: branch}); err != nil {
			return fmt.Errorf("clear current session: %w", err)
		}
		row, err := q.SetCurrentSession(ctx, gen.SetCurrentSessionParams{TenantID: tenant, BranchID: branch, ID: sessionID})
		if errors.Is(err, pgx.ErrNoRows) {
			// Returning an error rolls back the clear above: the old session stays current.
			return apperr.NotFound("org.session_not_found", "Session not found.")
		}
		if err != nil {
			return fmt.Errorf("set current session: %w", err)
		}
		out = Session{ID: db.UUIDString(row.ID), Name: row.Name, StartsOn: row.StartsOn.Time, EndsOn: row.EndsOn.Time, IsCurrent: row.IsCurrent}
		return s.audit(ctx, tx, p, "session.made_current", "session", out.ID, map[string]any{"name": out.Name})
	})
	return out, err
}

// ---- classes --------------------------------------------------------------------------

// CreateClass adds a class to the branch.
func (s *Service) CreateClass(ctx context.Context, p iam.Principal, in ClassInput) (Class, error) {
	in, err := validateClass(in)
	if err != nil {
		return Class{}, err
	}
	var out Class
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, branch, actor, err := s.writeScope(ctx, q, p)
		if err != nil {
			return err
		}
		row, err := q.InsertClass(ctx, gen.InsertClassParams{
			TenantID: tenant, BranchID: branch, Name: in.Name, GradeLevel: int32(in.GradeLevel), CreatedBy: actor,
		})
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("org.class_exists", "A class with this name already exists.")
		}
		if err != nil {
			return fmt.Errorf("insert class: %w", err)
		}
		out = Class{ID: db.UUIDString(row.ID), Name: row.Name, GradeLevel: int(row.GradeLevel), Status: row.Status}
		return s.audit(ctx, tx, p, "class.created", "class", out.ID, map[string]any{"name": out.Name})
	})
	return out, err
}

// ListClasses returns the branch's classes ordered by grade level.
func (s *Service) ListClasses(ctx context.Context, p iam.Principal) ([]Class, error) {
	out := []Class{}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		branch, err := q.DefaultBranch(ctx, tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find branch: %w", err)
		}
		rows, err := q.ListClasses(ctx, gen.ListClassesParams{TenantID: tenant, BranchID: branch})
		if err != nil {
			return fmt.Errorf("list classes: %w", err)
		}
		for _, r := range rows {
			out = append(out, Class{ID: db.UUIDString(r.ID), Name: r.Name, GradeLevel: int(r.GradeLevel), Status: r.Status})
		}
		return nil
	})
	return out, err
}

// ArchiveClass marks a class archived. A class is never deleted (BR-ORG-003, DEC-011).
func (s *Service) ArchiveClass(ctx context.Context, p iam.Principal, id string) (Class, error) {
	classID, err := db.ParseUUID(id)
	if err != nil {
		return Class{}, apperr.NotFound("org.class_not_found", "Class not found.")
	}
	var out Class
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		branch, err := q.DefaultBranch(ctx, tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("org.class_not_found", "Class not found.")
		}
		if err != nil {
			return fmt.Errorf("find branch: %w", err)
		}
		row, err := q.ArchiveClass(ctx, gen.ArchiveClassParams{TenantID: tenant, BranchID: branch, ID: classID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("org.class_not_found", "Class not found.")
		}
		if err != nil {
			return fmt.Errorf("archive class: %w", err)
		}
		out = Class{ID: db.UUIDString(row.ID), Name: row.Name, GradeLevel: int(row.GradeLevel), Status: row.Status}
		return s.audit(ctx, tx, p, "class.archived", "class", out.ID, map[string]any{"name": out.Name})
	})
	return out, err
}

// ---- sections -------------------------------------------------------------------------

// CreateSection adds a section, in a shift, to an active class.
func (s *Service) CreateSection(ctx context.Context, p iam.Principal, in SectionInput) (Section, error) {
	in, err := validateSection(in)
	if err != nil {
		return Section{}, err
	}
	var out Section
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, branch, actor, err := s.writeScope(ctx, q, p)
		if err != nil {
			return err
		}
		classID, err := db.ParseUUID(in.ClassID)
		if err != nil {
			return err
		}
		class, err := q.GetClass(ctx, gen.GetClassParams{TenantID: tenant, BranchID: branch, ID: classID})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("org.class_not_found", "Class not found.")
		}
		if err != nil {
			return fmt.Errorf("get class: %w", err)
		}
		if class.Status != "active" {
			return apperr.Validation("org.class_archived", "This class is archived; sections cannot be added.")
		}
		id, err := q.InsertSection(ctx, gen.InsertSectionParams{
			TenantID: tenant, ClassID: classID, Name: in.Name, Shift: in.Shift, CreatedBy: actor,
		})
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("org.section_exists", "This class already has a section with that name in that shift.")
		}
		if err != nil {
			return fmt.Errorf("insert section: %w", err)
		}
		out = Section{
			ID: db.UUIDString(id), ClassID: in.ClassID, ClassName: class.Name,
			Name: in.Name, Shift: in.Shift, Status: "active",
		}
		return s.audit(ctx, tx, p, "section.created", "section", out.ID,
			map[string]any{"class": class.Name, "name": in.Name, "shift": in.Shift})
	})
	return out, err
}

// ListSections returns the branch's sections, optionally of one class.
func (s *Service) ListSections(ctx context.Context, p iam.Principal, classID string) ([]Section, error) {
	out := []Section{}
	var filter pgtype.UUID // invalid (NULL) means "all classes"
	if classID != "" {
		id, err := db.ParseUUID(classID)
		if err != nil {
			return nil, apperr.Validation("org.class_invalid", "class_id must be a class id.",
				apperr.Field{Field: "class_id", Message: "Must be a class id."})
		}
		filter = id
	}
	err := db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		branch, err := q.DefaultBranch(ctx, tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find branch: %w", err)
		}
		rows, err := q.ListSections(ctx, gen.ListSectionsParams{TenantID: tenant, BranchID: branch, ClassID: filter})
		if err != nil {
			return fmt.Errorf("list sections: %w", err)
		}
		for _, r := range rows {
			out = append(out, Section{
				ID: db.UUIDString(r.ID), ClassID: db.UUIDString(r.ClassID), ClassName: r.ClassName,
				Name: r.Name, Shift: r.Shift, Status: r.Status,
			})
		}
		return nil
	})
	return out, err
}

// ArchiveSection marks a section archived. Enrolled students stay enrolled; new enrolments
// into it are refused. A section is never deleted (BR-ORG-003).
func (s *Service) ArchiveSection(ctx context.Context, p iam.Principal, id string) (Section, error) {
	sectionID, err := db.ParseUUID(id)
	if err != nil {
		return Section{}, apperr.NotFound("org.section_not_found", "Section not found.")
	}
	var out Section
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		tenant, err := db.ParseUUID(p.TenantID)
		if err != nil {
			return err
		}
		if _, err := q.ArchiveSection(ctx, gen.ArchiveSectionParams{TenantID: tenant, ID: sectionID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.NotFound("org.section_not_found", "Section not found.")
			}
			return fmt.Errorf("archive section: %w", err)
		}
		infos, err := s.DescribeSections(ctx, tx, p.TenantID, []string{id})
		if err != nil {
			return err
		}
		out = infos[id]
		return s.audit(ctx, tx, p, "section.archived", "section", out.ID,
			map[string]any{"class": out.ClassName, "name": out.Name, "shift": out.Shift})
	})
	return out, err
}

// ---- used by other modules, inside their own tenant transaction -----------------------

// ResolveEnrolmentTarget checks that a student can be enrolled in a section now, and returns
// the section and the current session. The caller passes its own open tenant transaction, so
// the check and the enrolment commit together. Nothing here writes.
func (s *Service) ResolveEnrolmentTarget(ctx context.Context, tx pgx.Tx, tenantID, sectionID string) (EnrolmentTarget, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return EnrolmentTarget{}, err
	}
	section, err := db.ParseUUID(sectionID)
	if err != nil {
		return EnrolmentTarget{}, apperr.Validation("org.section_invalid", "section_id must be a section id.",
			apperr.Field{Field: "section_id", Message: "Must be a section id."})
	}
	q := gen.New(tx)
	branch, err := q.DefaultBranch(ctx, tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return EnrolmentTarget{}, errNotSetUp
	}
	if err != nil {
		return EnrolmentTarget{}, fmt.Errorf("find branch: %w", err)
	}
	rows, err := q.SectionsByIDs(ctx, gen.SectionsByIDsParams{TenantID: tenant, Ids: []pgtype.UUID{section}})
	if err != nil {
		return EnrolmentTarget{}, fmt.Errorf("load section: %w", err)
	}
	if len(rows) == 0 || rows[0].BranchID != branch {
		return EnrolmentTarget{}, apperr.NotFound("org.section_not_found", "Section not found.")
	}
	row := rows[0]
	if row.Status != "active" || row.ClassStatus != "active" {
		return EnrolmentTarget{}, apperr.Validation("org.section_archived", "This section is archived; students cannot be enrolled in it.",
			apperr.Field{Field: "section_id", Message: "Archived."})
	}
	current, err := q.CurrentSession(ctx, gen.CurrentSessionParams{TenantID: tenant, BranchID: branch})
	if errors.Is(err, pgx.ErrNoRows) {
		return EnrolmentTarget{}, apperr.Validation("org.no_current_session", "There is no current session. Create one and make it current first.")
	}
	if err != nil {
		return EnrolmentTarget{}, fmt.Errorf("find current session: %w", err)
	}
	return EnrolmentTarget{
		SessionID:   db.UUIDString(current.ID),
		SessionName: current.Name,
		Section: Section{
			ID: db.UUIDString(row.ID), ClassID: db.UUIDString(row.ClassID), ClassName: row.ClassName,
			Name: row.Name, Shift: row.Shift, Status: row.Status,
		},
	}, nil
}

// DescribeSections returns the sections for the given ids, keyed by id. Unknown ids are absent.
func (s *Service) DescribeSections(ctx context.Context, tx pgx.Tx, tenantID string, ids []string) (map[string]Section, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	parsed, err := parseUUIDs(ids)
	if err != nil {
		return nil, err
	}
	rows, err := gen.New(tx).SectionsByIDs(ctx, gen.SectionsByIDsParams{TenantID: tenant, Ids: parsed})
	if err != nil {
		return nil, fmt.Errorf("describe sections: %w", err)
	}
	out := make(map[string]Section, len(rows))
	for _, r := range rows {
		id := db.UUIDString(r.ID)
		out[id] = Section{
			ID: id, ClassID: db.UUIDString(r.ClassID), ClassName: r.ClassName,
			Name: r.Name, Shift: r.Shift, Status: r.Status,
		}
	}
	return out, nil
}

// DescribeSessions returns the sessions for the given ids, keyed by id.
func (s *Service) DescribeSessions(ctx context.Context, tx pgx.Tx, tenantID string, ids []string) (map[string]Session, error) {
	tenant, err := db.ParseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	parsed, err := parseUUIDs(ids)
	if err != nil {
		return nil, err
	}
	rows, err := gen.New(tx).SessionsByIDs(ctx, gen.SessionsByIDsParams{TenantID: tenant, Ids: parsed})
	if err != nil {
		return nil, fmt.Errorf("describe sessions: %w", err)
	}
	out := make(map[string]Session, len(rows))
	for _, r := range rows {
		id := db.UUIDString(r.ID)
		out[id] = Session{ID: id, Name: r.Name, StartsOn: r.StartsOn.Time, EndsOn: r.EndsOn.Time, IsCurrent: r.IsCurrent}
	}
	return out, nil
}

// ---- helpers --------------------------------------------------------------------------

// writeScope ensures the tenant has its default branch and returns the ids a write needs.
func (s *Service) writeScope(ctx context.Context, q *gen.Queries, p iam.Principal) (tenant, branch, actor pgtype.UUID, err error) {
	if tenant, err = db.ParseUUID(p.TenantID); err != nil {
		return
	}
	if actor, err = db.ParseUUID(p.UserID); err != nil {
		return
	}
	if err = q.EnsureDefaultBranch(ctx, tenant); err != nil {
		err = fmt.Errorf("ensure default branch: %w", err)
		return
	}
	if branch, err = q.DefaultBranch(ctx, tenant); err != nil {
		err = fmt.Errorf("find default branch: %w", err)
	}
	return
}

func (s *Service) audit(ctx context.Context, tx pgx.Tx, p iam.Principal, action, entity, entityID string, summary map[string]any) error {
	return audit.Record(ctx, tx, audit.Entry{
		TenantID:    p.TenantID,
		Source:      audit.SourceAPI,
		ActorUserID: p.UserID,
		TokenID:     p.TokenID,
		Action:      action,
		Entity:      entity,
		EntityID:    entityID,
		Summary:     summary,
	})
}

func parseUUIDs(ids []string) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		u, err := db.ParseUUID(id)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}
