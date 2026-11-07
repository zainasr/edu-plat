package integration

import (
	"context"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/org"
)

// REQ-STU-001: a student is enrolled into a section of the current session, and the response
// says which class, section, shift and session.
func TestREQ_STU_001_EnrolmentIsCreatedWithTheStudent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "enrol")
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)

	got, err := f.students.Create(ctx, p, enrolInput(t, st.SectionID))
	if err != nil {
		t.Fatal(err)
	}
	e := got.Enrolment
	if e == nil {
		t.Fatal("the new student has no enrolment")
	}
	if e.ClassName != "Class 5" || e.SectionName != "A" || e.Shift != "morning" ||
		e.SessionName != "2026-27" || e.Status != "active" || e.SessionID != st.SessionID || e.SectionID != st.SectionID {
		t.Fatalf("enrolment is wrong: %+v", *e)
	}
}

// REQ-STU-001: enrolling needs a section that exists in the caller's school, is active, and has
// a current session. Each refusal has its own code.
func TestREQ_STU_001_EnrolmentRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	t.Run("a school with no structure yet", func(t *testing.T) {
		slug, mobile, _ := newTenant(t, f, "nostruct")
		p := principalFor(t, f, slug, mobile)
		_, err := f.students.Create(ctx, p, enrolInput(t, "5c9c6c26-e0e4-4938-af5c-f6fc35688270"))
		if got := errCode(t, err); got != "org.not_set_up" {
			t.Fatalf("code = %s, want org.not_set_up", got)
		}
	})

	t.Run("an unknown section", func(t *testing.T) {
		slug, mobile, _ := newTenant(t, f, "nosection")
		p := principalFor(t, f, slug, mobile)
		newStructure(t, f, p)
		_, err := f.students.Create(ctx, p, enrolInput(t, "5c9c6c26-e0e4-4938-af5c-f6fc35688270"))
		if got := errCode(t, err); got != "org.section_not_found" {
			t.Fatalf("code = %s, want org.section_not_found", got)
		}
	})

	t.Run("no current session", func(t *testing.T) {
		slug, mobile, _ := newTenant(t, f, "nosession")
		p := principalFor(t, f, slug, mobile)
		class, err := f.org.CreateClass(ctx, p, org.ClassInput{Name: "Class 5", GradeLevel: 5})
		if err != nil {
			t.Fatal(err)
		}
		section, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: class.ID, Name: "A", Shift: "morning"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.students.Create(ctx, p, enrolInput(t, section.ID))
		if got := errCode(t, err); got != "org.no_current_session" {
			t.Fatalf("code = %s, want org.no_current_session", got)
		}
	})

	t.Run("an archived section keeps its students but takes no new ones", func(t *testing.T) {
		slug, mobile, _ := newTenant(t, f, "archsection")
		p := principalFor(t, f, slug, mobile)
		st := newStructure(t, f, p)
		existing, err := f.students.Create(ctx, p, enrolInput(t, st.SectionID))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.org.ArchiveSection(ctx, p, st.SectionID); err != nil {
			t.Fatal(err)
		}

		_, err = f.students.Create(ctx, p, enrolInput(t, st.SectionID))
		if got := errCode(t, err); got != "org.section_archived" {
			t.Fatalf("code = %s, want org.section_archived", got)
		}
		still, err := f.students.Get(ctx, p, existing.ID)
		if err != nil || still.Enrolment == nil || still.Enrolment.SectionID != st.SectionID {
			t.Fatalf("the enrolled student must stay enrolled: %v %+v", err, still.Enrolment)
		}
	})

	t.Run("a refused enrolment leaves nothing behind", func(t *testing.T) {
		slug, mobile, tenantID := newTenant(t, f, "atomic")
		p := principalFor(t, f, slug, mobile)
		newStructure(t, f, p)
		if _, err := f.students.Create(ctx, p, enrolInput(t, "5c9c6c26-e0e4-4938-af5c-f6fc35688270")); err == nil {
			t.Fatal("expected a refusal")
		}
		var n int
		if err := f.admin.QueryRow(ctx,
			`SELECT (SELECT count(*) FROM students WHERE tenant_id = $1)
			      + (SELECT count(*) FROM families WHERE tenant_id = $1)
			      + (SELECT count(*) FROM guardians WHERE tenant_id = $1)`, tenantID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("a refused enrolment left %d rows behind; it must be all or nothing", n)
		}
	})
}

// US-STU-005 and REQ-STU-008: moving a student closes the old enrolment with the reason and opens
// a new one. Nothing is overwritten.
func TestUS_STU_005_ChangeSectionKeepsHistory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "move")
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)
	sectionB, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: st.ClassID, Name: "B", Shift: "morning"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := f.students.Create(ctx, p, enrolInput(t, st.SectionID))
	if err != nil {
		t.Fatal(err)
	}

	moved, err := f.students.ChangeSection(ctx, p, student.ID, sectionB.ID, "correction")
	if err != nil {
		t.Fatalf("change section: %v", err)
	}
	if moved.Enrolment == nil || moved.Enrolment.SectionName != "B" || moved.Enrolment.Status != "active" {
		t.Fatalf("the student should now be in 5-B: %+v", moved.Enrolment)
	}

	history, err := f.students.ListEnrolments(ctx, p, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("want 2 history entries, got %d", len(history))
	}
	newest, oldest := history[0], history[1]
	if newest.SectionName != "B" || newest.Status != "active" || newest.EndedOn != nil {
		t.Fatalf("newest entry should be active 5-B: %+v", newest)
	}
	if oldest.SectionName != "A" || oldest.Status != "closed" || oldest.EndedOn == nil ||
		oldest.Reason == nil || *oldest.Reason != "correction" {
		t.Fatalf("oldest entry should be closed 5-A with the reason: %+v", oldest)
	}

	var n int
	if err := f.admin.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = 'student.section_changed'
		   AND summary->>'reason' = 'correction'`, tenantID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("want one student.section_changed audit row with the reason: %v, %d", err, n)
	}
}

// REQ-STU-008: every move is an entry, and at most one is active at a time.
func TestREQ_STU_008_EveryMoveIsAnEntryAndOneIsActive(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "manymoves")
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)
	b, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: st.ClassID, Name: "B", Shift: "morning"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := f.students.Create(ctx, p, enrolInput(t, st.SectionID))
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{b.ID, st.SectionID, b.ID} {
		if _, err := f.students.ChangeSection(ctx, p, student.ID, target, "trying it"); err != nil {
			t.Fatalf("move: %v", err)
		}
	}
	history, err := f.students.ListEnrolments(ctx, p, student.ID)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, e := range history {
		if e.Status == "active" {
			active++
		}
	}
	if len(history) != 4 || active != 1 {
		t.Fatalf("want 4 entries with exactly 1 active, got %d entries and %d active", len(history), active)
	}
}

// US-STU-005: the refusals of a move.
func TestUS_STU_005_ChangeSectionRefusals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "moverefuse")
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)
	b, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: st.ClassID, Name: "B", Shift: "morning"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := f.students.Create(ctx, p, enrolInput(t, st.SectionID))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		student  string
		section  string
		reason   string
		wantCode string
	}{
		{name: "same section", student: student.ID, section: st.SectionID, reason: "no change", wantCode: "students.same_section"},
		{name: "reason too short", student: student.ID, section: b.ID, reason: "no", wantCode: "students.reason_invalid"},
		{name: "reason blank", student: student.ID, section: b.ID, reason: "   ", wantCode: "students.reason_invalid"},
		{name: "unknown student", student: "5c9c6c26-e0e4-4938-af5c-f6fc35688270", section: b.ID, reason: "correction", wantCode: "students.not_found"},
		{name: "student id is not an id", student: "nope", section: b.ID, reason: "correction", wantCode: "students.not_found"},
		{name: "unknown section", student: student.ID, section: "5c9c6c26-e0e4-4938-af5c-f6fc35688270", reason: "correction", wantCode: "org.section_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.students.ChangeSection(ctx, p, tt.student, tt.section, tt.reason)
			if got := errCode(t, err); got != tt.wantCode {
				t.Fatalf("code = %s, want %s", got, tt.wantCode)
			}
		})
	}

	t.Run("an archived target", func(t *testing.T) {
		c, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: st.ClassID, Name: "C", Shift: "morning"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.org.ArchiveSection(ctx, p, c.ID); err != nil {
			t.Fatal(err)
		}
		_, err = f.students.ChangeSection(ctx, p, student.ID, c.ID, "correction")
		if got := errCode(t, err); got != "org.section_archived" {
			t.Fatalf("code = %s, want org.section_archived", got)
		}
	})

	t.Run("a refused move changes nothing", func(t *testing.T) {
		history, err := f.students.ListEnrolments(ctx, p, student.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 1 || history[0].Status != "active" {
			t.Fatalf("refused moves must leave one active entry, got %+v", history)
		}
	})
}

// A student created before sections existed has no enrolment. They read fine, and cannot be
// "moved" until they are enrolled.
func TestREQ_STU_008_StudentWithoutAnEnrolment(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "legacy")
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)
	b, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: st.ClassID, Name: "B", Shift: "morning"})
	if err != nil {
		t.Fatal(err)
	}
	student, err := f.students.Create(ctx, p, enrolInput(t, st.SectionID))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate pre-existing data by removing the enrolment as the owner.
	if _, err := f.admin.Exec(ctx, `DELETE FROM enrolments WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}

	got, err := f.students.Get(ctx, p, student.ID)
	if err != nil {
		t.Fatalf("a student without an enrolment must still be readable: %v", err)
	}
	if got.Enrolment != nil {
		t.Fatalf("want no enrolment, got %+v", *got.Enrolment)
	}
	_, err = f.students.ChangeSection(ctx, p, student.ID, b.ID, "correction")
	if code := errCode(t, err); code != "students.not_enrolled" {
		t.Fatalf("code = %s, want students.not_enrolled", code)
	}
}

// REQ-STU-008, backed by the database: a second active enrolment for the same student and
// session is refused even if a bug asks for it.
func TestREQ_STU_008_DatabaseRefusesTwoActiveEnrolmentsInASession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "twoactive")
	p := principalFor(t, f, slug, mobile)
	st := newStructure(t, f, p)
	if _, err := f.students.Create(ctx, p, enrolInput(t, st.SectionID)); err != nil {
		t.Fatal(err)
	}

	_, err := f.admin.Exec(ctx, `
		INSERT INTO enrolments (tenant_id, student_id, session_id, section_id, started_on)
		SELECT tenant_id, student_id, session_id, section_id, started_on
		FROM enrolments WHERE tenant_id = $1 LIMIT 1`, tenantID)
	if err == nil {
		t.Fatal("the database accepted a second active enrolment for one student in one session")
	}
}
