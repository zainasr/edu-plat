package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/zainasr/edu-plat/services/api/internal/org"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

func session(t *testing.T, name, from, to string) org.SessionInput {
	t.Helper()
	return org.SessionInput{Name: name, StartsOn: mustDate(t, from), EndsOn: mustDate(t, to)}
}

// BR-ORG-001: a branch has exactly one current session. The first one is current; making
// another current clears the old one in the same transaction.
func TestBR_ORG_001_ExactlyOneCurrentSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "sessions")
	p := principalFor(t, f, slug, mobile)

	first, err := f.org.CreateSession(ctx, p, session(t, "2026-27", "2026-04-01", "2027-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	if !first.IsCurrent {
		t.Fatal("the first session of a branch must be current")
	}
	second, err := f.org.CreateSession(ctx, p, session(t, "2027-28", "2027-04-01", "2028-03-31"))
	if err != nil {
		t.Fatal(err)
	}
	if second.IsCurrent {
		t.Fatal("a later session must be created not current")
	}

	if _, err := f.org.MakeSessionCurrent(ctx, p, second.ID); err != nil {
		t.Fatalf("make current: %v", err)
	}
	list, err := f.org.ListSessions(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	current := 0
	for _, s := range list {
		if s.IsCurrent {
			current++
			if s.ID != second.ID {
				t.Fatalf("the wrong session is current: %s", s.Name)
			}
		}
	}
	if current != 1 {
		t.Fatalf("want exactly one current session, got %d", current)
	}
}

// BR-ORG-001: making an unknown session current changes nothing (the clear is rolled back).
func TestBR_ORG_001_FailedSwitchKeepsTheOldCurrentSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "switch")
	p := principalFor(t, f, slug, mobile)
	first, err := f.org.CreateSession(ctx, p, session(t, "2026-27", "2026-04-01", "2027-03-31"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.org.MakeSessionCurrent(ctx, p, "5c9c6c26-e0e4-4938-af5c-f6fc35688270")
	if got := errCode(t, err); got != "org.session_not_found" {
		t.Fatalf("code = %s, want org.session_not_found", got)
	}
	list, err := f.org.ListSessions(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != first.ID || !list[0].IsCurrent {
		t.Fatalf("the original session must still be current: %+v", list)
	}
}

// BR-ORG-001, backed by the database: even a direct insert cannot create a second current session.
func TestBR_ORG_001_DatabaseRefusesASecondCurrentSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "dbcurrent")
	p := principalFor(t, f, slug, mobile)
	if _, err := f.org.CreateSession(ctx, p, session(t, "2026-27", "2026-04-01", "2027-03-31")); err != nil {
		t.Fatal(err)
	}

	_, err := f.admin.Exec(ctx, `
		INSERT INTO sessions (tenant_id, branch_id, name, starts_on, ends_on, is_current)
		SELECT tenant_id, branch_id, 'sneaky', '2030-01-01', '2030-12-31', true
		FROM sessions WHERE tenant_id = $1 LIMIT 1`, tenantID)
	if !db.IsUniqueViolation(err) {
		t.Fatalf("want a unique violation from sessions_one_current, got %v", err)
	}
}

func TestREQ_ORG_006_DuplicateSessionNameIsAConflict(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "dupsession")
	p := principalFor(t, f, slug, mobile)
	if _, err := f.org.CreateSession(ctx, p, session(t, "2026-27", "2026-04-01", "2027-03-31")); err != nil {
		t.Fatal(err)
	}
	_, err := f.org.CreateSession(ctx, p, session(t, "2026-27", "2026-05-01", "2027-04-30"))
	if got := errCode(t, err); got != "org.session_exists" {
		t.Fatalf("code = %s, want org.session_exists", got)
	}
}

// REQ-ORG-005: a shift belongs to the section. 5-A Morning and 5-A Evening are two sections;
// a second 5-A Morning is a conflict.
func TestREQ_ORG_005_SameSectionNameInTwoShifts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "shifts")
	p := principalFor(t, f, slug, mobile)
	class, err := f.org.CreateClass(ctx, p, org.ClassInput{Name: "Class 5", GradeLevel: 5})
	if err != nil {
		t.Fatal(err)
	}

	morning, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: class.ID, Name: "A", Shift: "morning"})
	if err != nil {
		t.Fatal(err)
	}
	evening, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: class.ID, Name: "A", Shift: "evening"})
	if err != nil {
		t.Fatalf("5-A evening should be a separate section: %v", err)
	}
	if morning.ID == evening.ID {
		t.Fatal("the two shifts must be different sections")
	}

	_, err = f.org.CreateSection(ctx, p, org.SectionInput{ClassID: class.ID, Name: "A", Shift: "morning"})
	if got := errCode(t, err); got != "org.section_exists" {
		t.Fatalf("code = %s, want org.section_exists", got)
	}

	list, err := f.org.ListSections(ctx, p, class.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 sections, got %d", len(list))
	}
}

// BR-ORG-003: a class is archived, never deleted, and an archived class takes no new sections.
func TestBR_ORG_003_ArchivedClassTakesNoNewSections(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "archclass")
	p := principalFor(t, f, slug, mobile)
	class, err := f.org.CreateClass(ctx, p, org.ClassInput{Name: "Class 5", GradeLevel: 5})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := f.org.ArchiveClass(ctx, p, class.ID)
	if err != nil || archived.Status != "archived" {
		t.Fatalf("archive: %v (status %q)", err, archived.Status)
	}

	_, err = f.org.CreateSection(ctx, p, org.SectionInput{ClassID: class.ID, Name: "A", Shift: "morning"})
	if got := errCode(t, err); got != "org.class_archived" {
		t.Fatalf("code = %s, want org.class_archived", got)
	}
	classes, err := f.org.ListClasses(ctx, p)
	if err != nil || len(classes) != 1 {
		t.Fatalf("the archived class must still be listed (history): %v %d", err, len(classes))
	}
}

// BR-ORG-003 / DEC-011: a section with enrolments cannot be deleted. The database says no twice:
// the API role has no DELETE privilege, and even the owner hits the foreign key from enrolments.
func TestBR_ORG_003_SectionWithEnrolmentsCannotBeDeleted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "nodelete")
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)
	if _, err := f.students.Create(ctx, p, enrolInput(t, st.SectionID)); err != nil {
		t.Fatal(err)
	}

	err := db.WithTenant(ctx, f.app, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM sections WHERE id = $1`, st.SectionID)
		return err
	})
	if err == nil {
		t.Fatal("the API role deleted a section; it must have no DELETE privilege (DEC-011)")
	}

	_, err = f.admin.Exec(ctx, `DELETE FROM sections WHERE id = $1`, st.SectionID)
	if !db.IsForeignKeyViolation(err) {
		t.Fatalf("even the owner must hit the enrolments foreign key, got %v", err)
	}
}

// The default branch is created the first time the org module needs it, once, even when
// several requests race to create it.
func TestREQ_ORG_DefaultBranchIsCreatedOnceUnderConcurrency(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "branch")
	p := principalFor(t, f, slug, mobile)

	// A tenant that has not created anything lists nothing and creates no branch.
	if list, err := f.org.ListClasses(ctx, p); err != nil || len(list) != 0 {
		t.Fatalf("empty tenant: %v, %d classes", err, len(list))
	}
	var n int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM branches WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("reading must not create a branch: %v, %d", err, n)
	}

	const writers = 6
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			_, err := f.org.CreateClass(ctx, p, org.ClassInput{Name: "Class " + string(rune('A'+i)), GradeLevel: i})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create class: %v", err)
		}
	}

	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM branches WHERE tenant_id = $1`, tenantID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want exactly one default branch after %d concurrent creates, got %d", writers, n)
	}
}
