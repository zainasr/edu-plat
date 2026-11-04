package students

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/audit"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/students/store/gen"
)

// pakistanZone is Asia/Karachi (UTC+5, no DST). Registration years use it (data model §1).
var pakistanZone = time.FixedZone("PKT", 5*60*60)

// Student is the read model returned by the API.
type Student struct {
	ID             string
	RegistrationNo string
	FullName       string
	FatherName     string
	Gender         string
	DateOfBirth    time.Time
	Status         string
	FamilyID       string
	FamilyNumber   string
	Guardians      []GuardianLink
	CreatedAt      time.Time
}

// GuardianLink is one guardian of one student, with the REQ-STU-015 flags.
type GuardianLink struct {
	GuardianID            string
	FullName              string
	Mobile                string
	Relationship          string
	Primary               bool
	FeePayer              bool
	ReceivesNotifications bool
	EmergencyContact      bool
	Access                string
}

// Page is one page of students. NextCursor is empty on the last page.
type Page struct {
	Items      []Student
	NextCursor string
}

// Service holds the enrolment and read rules. Callers check permissions first
// (iam.Require) and pass the principal in, so every write is attributed.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewService builds the students service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, now: time.Now}
}

// Create enrols a student and its guardians in one tenant transaction, with an audit entry.
// A guardian whose mobile already exists joins that guardian's family (REQ-STU-004).
func (s *Service) Create(ctx context.Context, p iam.Principal, in StudentInput) (Student, error) {
	guardians, err := validateEnrolment(in, s.now().In(pakistanZone))
	if err != nil {
		return Student{}, err
	}
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return Student{}, err
	}
	actor, err := db.ParseUUID(p.UserID)
	if err != nil {
		return Student{}, err
	}

	var created Student
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)

		// 1. Find the family: existing guardians decide it, and they must agree.
		familyID, guardianIDs, err := matchFamily(ctx, q, tenant, guardians)
		if err != nil {
			return err
		}
		if !familyID.Valid {
			n, err := q.NextCounter(ctx, gen.NextCounterParams{TenantID: tenant, Name: "family"})
			if err != nil {
				return fmt.Errorf("family counter: %w", err)
			}
			familyID, err = q.InsertFamily(ctx, gen.InsertFamilyParams{
				TenantID:  tenant,
				Number:    fmt.Sprintf("F-%05d", n),
				CreatedBy: actor,
			})
			if err != nil {
				return fmt.Errorf("insert family: %w", err)
			}
		}
		for i, g := range guardians {
			if guardianIDs[i].Valid {
				continue
			}
			id, err := q.InsertGuardian(ctx, gen.InsertGuardianParams{
				TenantID:  tenant,
				FamilyID:  familyID,
				FullName:  g.FullName,
				Mobile:    g.Mobile,
				CreatedBy: actor,
			})
			if err != nil {
				return fmt.Errorf("insert guardian: %w", err)
			}
			guardianIDs[i] = id
		}

		// 2. Registration number from an atomic per-year counter (REQ-STU-003).
		year := s.now().In(pakistanZone).Year()
		seq, err := q.NextCounter(ctx, gen.NextCounterParams{TenantID: tenant, Name: fmt.Sprintf("student-%d", year)})
		if err != nil {
			return fmt.Errorf("registration counter: %w", err)
		}
		st, err := q.InsertStudent(ctx, gen.InsertStudentParams{
			TenantID:       tenant,
			FamilyID:       familyID,
			RegistrationNo: fmt.Sprintf("REG-%d-%05d", year, seq),
			FullName:       strings.TrimSpace(in.FullName),
			FatherName:     strings.TrimSpace(in.FatherName),
			Gender:         in.Gender,
			DateOfBirth:    pgtype.Date{Time: in.DateOfBirth, Valid: true},
			CreatedBy:      actor,
		})
		if err != nil {
			return fmt.Errorf("insert student: %w", err)
		}

		// 3. The guardian links, one per guardian, with their flags.
		for i, g := range guardians {
			if err := q.InsertGuardianLink(ctx, gen.InsertGuardianLinkParams{
				TenantID:              tenant,
				StudentID:             st.ID,
				GuardianID:            guardianIDs[i],
				Relationship:          g.Relationship,
				IsPrimary:             g.Primary,
				IsFeePayer:            g.FeePayer,
				ReceivesNotifications: g.ReceivesNotifications,
				EmergencyContact:      g.EmergencyContact,
				Access:                g.Access,
				CreatedBy:             actor,
			}); err != nil {
				return fmt.Errorf("insert guardian link: %w", err)
			}
		}

		// 4. Audit, in the same transaction as the change.
		studentID := db.UUIDString(st.ID)
		if err := audit.Record(ctx, tx, audit.Entry{
			TenantID:    p.TenantID,
			Source:      audit.SourceAPI,
			ActorUserID: p.UserID,
			TokenID:     p.TokenID,
			Action:      "student.created",
			Entity:      "student",
			EntityID:    studentID,
			Summary:     map[string]any{"guardians": len(guardians), "family_id": db.UUIDString(familyID)},
		}); err != nil {
			return err
		}

		created, err = readStudent(ctx, q, tenant, st.ID)
		return err
	})
	if err != nil {
		return Student{}, err
	}
	return created, nil
}

// Get returns one student. An ID that is not in the caller's tenant is a 404 (API §3).
func (s *Service) Get(ctx context.Context, p iam.Principal, id string) (Student, error) {
	notFound := apperr.NotFound("students.not_found", "Student not found.")
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return Student{}, err
	}
	studentID, err := db.ParseUUID(id)
	if err != nil {
		return Student{}, notFound
	}
	var st Student
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		var err error
		st, err = readStudent(ctx, gen.New(tx), tenant, studentID)
		return err
	})
	if err != nil {
		return Student{}, err
	}
	return st, nil
}

// List returns one page of students, newest first. limit is 1..200 (the API default is 50).
func (s *Service) List(ctx context.Context, p iam.Principal, limit int, cursor string) (Page, error) {
	if limit < 1 || limit > 200 {
		return Page{}, apperr.Validation("students.limit_invalid", "limit must be between 1 and 200.",
			apperr.Field{Field: "limit", Message: "Between 1 and 200."})
	}
	after, err := decodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	tenant, err := db.ParseUUID(p.TenantID)
	if err != nil {
		return Page{}, err
	}

	var page Page
	err = db.WithTenant(ctx, s.pool, p.TenantID, func(tx pgx.Tx) error {
		q := gen.New(tx)
		params := gen.ListStudentsPageParams{TenantID: tenant, PageLimit: int32(limit + 1)}
		if after != nil {
			params.AfterCreatedAt = pgtype.Timestamptz{Time: after.createdAt, Valid: true}
			params.AfterID = after.id
		}
		rows, err := q.ListStudentsPage(ctx, params)
		if err != nil {
			return fmt.Errorf("list students: %w", err)
		}

		hasMore := len(rows) > limit
		if hasMore {
			rows = rows[:limit]
		}
		ids := make([]pgtype.UUID, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		linksByStudent, err := loadLinks(ctx, q, tenant, ids)
		if err != nil {
			return err
		}
		for _, r := range rows {
			page.Items = append(page.Items, toStudent(r.ID, r.RegistrationNo, r.FullName, r.FatherName,
				r.Gender, r.DateOfBirth, r.Status, r.FamilyID, r.FamilyNumber, r.CreatedAt,
				linksByStudent[db.UUIDString(r.ID)]))
		}
		if hasMore && len(rows) > 0 {
			last := page.Items[len(page.Items)-1]
			page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
		}
		return nil
	})
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// matchFamily finds the family of any guardian already on file by mobile. It returns the
// guardian IDs it found (invalid for new guardians) and rejects guardians spread over two families.
func matchFamily(ctx context.Context, q *gen.Queries, tenant pgtype.UUID, guardians []guardianRule) (pgtype.UUID, []pgtype.UUID, error) {
	var familyID pgtype.UUID
	ids := make([]pgtype.UUID, len(guardians))
	for i, g := range guardians {
		row, err := q.GuardianByMobile(ctx, gen.GuardianByMobileParams{TenantID: tenant, Mobile: g.Mobile})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return pgtype.UUID{}, nil, fmt.Errorf("find guardian: %w", err)
		}
		if familyID.Valid && familyID != row.FamilyID {
			return pgtype.UUID{}, nil, apperr.Validation("students.family_conflict",
				"These guardians already belong to different families. Merge the families first.")
		}
		familyID = row.FamilyID
		ids[i] = row.ID
	}
	return familyID, ids, nil
}

// readStudent loads one student and its guardian links. The tenant transaction must be open.
func readStudent(ctx context.Context, q *gen.Queries, tenant, id pgtype.UUID) (Student, error) {
	row, err := q.GetStudent(ctx, gen.GetStudentParams{TenantID: tenant, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Student{}, apperr.NotFound("students.not_found", "Student not found.")
	}
	if err != nil {
		return Student{}, fmt.Errorf("get student: %w", err)
	}
	links, err := loadLinks(ctx, q, tenant, []pgtype.UUID{id})
	if err != nil {
		return Student{}, err
	}
	return toStudent(row.ID, row.RegistrationNo, row.FullName, row.FatherName, row.Gender,
		row.DateOfBirth, row.Status, row.FamilyID, row.FamilyNumber, row.CreatedAt,
		links[db.UUIDString(row.ID)]), nil
}

func loadLinks(ctx context.Context, q *gen.Queries, tenant pgtype.UUID, studentIDs []pgtype.UUID) (map[string][]GuardianLink, error) {
	out := make(map[string][]GuardianLink)
	if len(studentIDs) == 0 {
		return out, nil
	}
	rows, err := q.ListGuardianLinksForStudents(ctx, gen.ListGuardianLinksForStudentsParams{
		TenantID:   tenant,
		StudentIds: studentIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("load guardian links: %w", err)
	}
	for _, r := range rows {
		key := db.UUIDString(r.StudentID)
		out[key] = append(out[key], GuardianLink{
			GuardianID:            db.UUIDString(r.GuardianID),
			FullName:              r.FullName,
			Mobile:                r.Mobile,
			Relationship:          r.Relationship,
			Primary:               r.IsPrimary,
			FeePayer:              r.IsFeePayer,
			ReceivesNotifications: r.ReceivesNotifications,
			EmergencyContact:      r.EmergencyContact,
			Access:                r.Access,
		})
	}
	return out, nil
}

func toStudent(id pgtype.UUID, regNo, fullName, fatherName, gender string, dob pgtype.Date,
	status string, familyID pgtype.UUID, familyNumber string, createdAt pgtype.Timestamptz,
	links []GuardianLink) Student {
	return Student{
		ID:             db.UUIDString(id),
		RegistrationNo: regNo,
		FullName:       fullName,
		FatherName:     fatherName,
		Gender:         gender,
		DateOfBirth:    dob.Time,
		Status:         status,
		FamilyID:       db.UUIDString(familyID),
		FamilyNumber:   familyNumber,
		Guardians:      links,
		CreatedAt:      createdAt.Time,
	}
}

// cursor is the keyset position of the last row on a page.
type cursor struct {
	createdAt time.Time
	id        pgtype.UUID
}

// encodeCursor makes an opaque cursor from (created_at, id). Clients must not parse it.
func encodeCursor(createdAt time.Time, id string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

var errCursor = apperr.Validation("students.cursor_invalid", "The cursor is not valid. Start again without a cursor.")

// decodeCursor returns nil for an empty cursor (the first page).
func decodeCursor(s string) (*cursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, errCursor
	}
	ts, idStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, errCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return nil, errCursor
	}
	id, err := db.ParseUUID(idStr)
	if err != nil {
		return nil, errCursor
	}
	return &cursor{createdAt: createdAt, id: id}, nil
}
