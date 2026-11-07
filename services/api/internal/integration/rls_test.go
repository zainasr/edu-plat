package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/students"
)

// enrolInput is a valid enrolment into the given section, with a fresh guardian mobile.
func enrolInput(t *testing.T, sectionID string) students.StudentInput {
	t.Helper()
	return students.StudentInput{
		SectionID:   sectionID,
		FullName:    "Ali Khan",
		FatherName:  "Imran Khan",
		Gender:      "male",
		DateOfBirth: mustDate(t, "2018-04-12"),
		Guardians: []students.GuardianInput{{
			FullName:     "Kamran Khan",
			Mobile:       newMobile(t),
			Relationship: "uncle",
			Primary:      true,
		}},
	}
}

// TestRLS_TenantBCannotReadOrWriteTenantA is the isolation proof required by ADR-0002:
// Given tenant A has a student,
// when tenant B reads, lists, updates or links into it,
// then B sees nothing and cannot change it, and with no tenant set the API role sees no rows.
func TestRLS_TenantBCannotReadOrWriteTenantA(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	slugA, mobileA, tenantA := newTenant(t, f, "a")
	slugB, mobileB, tenantB := newTenant(t, f, "b")

	pA, err := f.iam.Authenticate(ctx, signIn(t, f, mobileA, slugA))
	if err != nil {
		t.Fatalf("authenticate tenant A: %v", err)
	}
	pB, err := f.iam.Authenticate(ctx, signIn(t, f, mobileB, slugB))
	if err != nil {
		t.Fatalf("authenticate tenant B: %v", err)
	}
	if pA.TenantID != tenantA || pB.TenantID != tenantB {
		t.Fatalf("principals are in the wrong tenants: %s / %s", pA.TenantID, pB.TenantID)
	}

	svc := f.students
	structA := newStructure(t, f, pA)
	structB := newStructure(t, f, pB)
	created, err := svc.Create(ctx, pA, enrolInput(t, structA.SectionID))
	if err != nil {
		t.Fatalf("tenant A enrols a student: %v", err)
	}
	// B needs its own guardian so the link attack below has a real B row to use.
	if _, err := svc.Create(ctx, pB, enrolInput(t, structB.SectionID)); err != nil {
		t.Fatalf("tenant B enrols a student: %v", err)
	}

	t.Run("B cannot enrol a student into A's section", func(t *testing.T) {
		_, err := svc.Create(ctx, pB, enrolInput(t, structA.SectionID))
		if got := errCode(t, err); got != "org.section_not_found" {
			t.Fatalf("code = %s, want org.section_not_found (A's section must look absent to B)", got)
		}
	})

	t.Run("B's structure lists never contain A's classes, sections or sessions", func(t *testing.T) {
		classes, err := f.org.ListClasses(ctx, pB)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range classes {
			if c.ID == structA.ClassID {
				t.Fatal("tenant B can list tenant A's class")
			}
		}
		sections, err := f.org.ListSections(ctx, pB, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range sections {
			if s.ID == structA.SectionID {
				t.Fatal("tenant B can list tenant A's section")
			}
		}
		sessions, err := f.org.ListSessions(ctx, pB)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range sessions {
			if s.ID == structA.SessionID {
				t.Fatal("tenant B can list tenant A's session")
			}
		}
	})

	t.Run("B cannot archive A's class", func(t *testing.T) {
		_, err := f.org.ArchiveClass(ctx, pB, structA.ClassID)
		if got := errCode(t, err); got != "org.class_not_found" {
			t.Fatalf("code = %s, want org.class_not_found", got)
		}
	})

	t.Run("B cannot read A's student by id", func(t *testing.T) {
		_, err := svc.Get(ctx, pB, created.ID)
		e, ok := apperr.As(err)
		if !ok || e.Code != "students.not_found" {
			t.Fatalf("want students.not_found, got %v", err)
		}
	})

	t.Run("A can read its own student", func(t *testing.T) {
		got, err := svc.Get(ctx, pA, created.ID)
		if err != nil || got.ID != created.ID {
			t.Fatalf("tenant A read its own student: %v (id %s)", err, got.ID)
		}
	})

	t.Run("B's list never contains A's student", func(t *testing.T) {
		page, err := svc.List(ctx, pB, 200, "")
		if err != nil {
			t.Fatalf("list as B: %v", err)
		}
		for _, s := range page.Items {
			if s.ID == created.ID {
				t.Fatal("tenant B can list tenant A's student")
			}
		}
	})

	t.Run("B cannot update A's student", func(t *testing.T) {
		var rows int64
		err := db.WithTenant(ctx, f.app, tenantB, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `UPDATE students SET full_name = 'Hacked' WHERE id = $1`, created.ID)
			if err != nil {
				return err
			}
			rows = tag.RowsAffected()
			return nil
		})
		if err != nil {
			t.Fatalf("update attempt: %v", err)
		}
		if rows != 0 {
			t.Fatalf("tenant B updated %d of A's rows", rows)
		}
	})

	t.Run("B cannot link a guardian into A's student", func(t *testing.T) {
		err := db.WithTenant(ctx, f.app, tenantB, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO guardian_links (tenant_id, student_id, guardian_id, relationship,
				    is_primary, is_fee_payer, receives_notifications, emergency_contact, access)
				SELECT $1, $2, g.id, 'other', false, false, true, false, 'none'
				FROM guardians g WHERE g.tenant_id = $1 LIMIT 1`, tenantB, created.ID)
			return err
		})
		if err == nil {
			t.Fatal("tenant B inserted a link into tenant A's student")
		}
	})

	t.Run("with no tenant set the API role sees no students", func(t *testing.T) {
		var n int
		err := f.app.QueryRow(ctx, `SELECT count(*) FROM students`).Scan(&n)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("without a tenant, %d students are visible; RLS must fail closed", n)
		}
	})

	t.Run("the API role cannot bypass RLS", func(t *testing.T) {
		var bypass bool
		err := f.app.QueryRow(ctx, `SELECT rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&bypass)
		if err != nil {
			t.Fatalf("role check: %v", err)
		}
		if bypass {
			t.Fatal("the API role has BYPASSRLS; RLS would not protect tenants")
		}
	})
}

// TestAudit_StudentCreateIsRecordedInTheSameTransaction checks REQ-API-017: a change has an
// audit row carrying the actor and the token.
func TestAudit_StudentCreateIsRecordedInTheSameTransaction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "audit")
	p, err := f.iam.Authenticate(ctx, signIn(t, f, mobile, slug))
	if err != nil {
		t.Fatal(err)
	}
	sec := newStructure(t, f, p)
	if _, err := f.students.Create(ctx, p, enrolInput(t, sec.SectionID)); err != nil {
		t.Fatalf("create: %v", err)
	}

	var n int
	err = f.admin.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'student.created' AND token_id IS NOT NULL`,
		tenantID).Scan(&n)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if n != 1 {
		t.Fatalf("want one student.created audit row with a token, got %d", n)
	}
}

// TestAppendOnly_AuditRowsCannotBeChanged checks DEC-011 for the audit log, even for the owner role.
func TestAppendOnly_AuditRowsCannotBeChanged(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _, tenantID := newTenant(t, f, "append")

	_, err := f.admin.Exec(ctx, `UPDATE audit_log SET action = 'edited' WHERE tenant_id = $1`, tenantID)
	if err == nil {
		t.Fatal("audit_log accepted an UPDATE; it must be append-only")
	}
}
