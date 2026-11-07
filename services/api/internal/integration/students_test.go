package integration

import (
	"context"
	"sync"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/students"
)

// enrolWith is an enrolment whose single primary guardian has the given mobile.
func enrolWith(t *testing.T, childName, guardianMobile string) students.StudentInput {
	t.Helper()
	in := enrolInput(t)
	in.FullName = childName
	in.Guardians[0].Mobile = guardianMobile
	return in
}

// REQ-STU-004: a guardian whose mobile is already on file joins that guardian's family,
// so siblings share one family and one guardian record.
func TestREQ_STU_004_KnownGuardianJoinsTheirFamily(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "family")
	p := principalFor(t, f, slug, mobile)
	svc := students.NewService(f.app)

	shared := newMobile(t)
	first, err := svc.Create(ctx, p, enrolWith(t, "Ali Khan", shared))
	if err != nil {
		t.Fatalf("first child: %v", err)
	}
	second, err := svc.Create(ctx, p, enrolWith(t, "Bilal Khan", shared))
	if err != nil {
		t.Fatalf("second child: %v", err)
	}

	if first.FamilyID != second.FamilyID || first.FamilyNumber != second.FamilyNumber {
		t.Fatalf("siblings should share a family: %s/%s vs %s/%s",
			first.FamilyID, first.FamilyNumber, second.FamilyID, second.FamilyNumber)
	}
	if first.Guardians[0].GuardianID != second.Guardians[0].GuardianID {
		t.Fatal("the same mobile should reuse the same guardian record")
	}
	if first.RegistrationNo == second.RegistrationNo {
		t.Fatalf("each child needs its own registration number, both got %s", first.RegistrationNo)
	}
}

// REQ-STU-004: guardians from two different families cannot be mixed in one enrolment.
func TestREQ_STU_004_GuardiansFromTwoFamiliesAreRejected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "conflict")
	p := principalFor(t, f, slug, mobile)
	svc := students.NewService(f.app)

	m1, m2 := newMobile(t), newMobile(t)
	if _, err := svc.Create(ctx, p, enrolWith(t, "Child One", m1)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, p, enrolWith(t, "Child Two", m2)); err != nil {
		t.Fatal(err)
	}

	mixed := enrolWith(t, "Child Three", m1)
	mixed.Guardians = append(mixed.Guardians, students.GuardianInput{
		FullName: "Other Family Aunt", Mobile: m2, Relationship: "aunt",
	})
	_, err := svc.Create(ctx, p, mixed)
	if got := errCode(t, err); got != "students.family_conflict" {
		t.Fatalf("code = %s, want students.family_conflict", got)
	}
}

// REQ-STU-003: registration numbers are unique, even for enrolments at the same moment.
func TestREQ_STU_003_RegistrationNumbersAreUniqueUnderConcurrency(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "concurrent")
	p := principalFor(t, f, slug, mobile)
	svc := students.NewService(f.app)

	const n = 10
	// Build inputs on the test goroutine: t.Fatal must not be called from other goroutines.
	inputs := make([]students.StudentInput, n)
	for i := range inputs {
		inputs[i] = enrolInput(t)
	}

	type result struct {
		reg string
		err error
	}
	results := make(chan result, n)
	var wg sync.WaitGroup
	for _, in := range inputs {
		wg.Go(func() {
			st, err := svc.Create(ctx, p, in)
			results <- result{reg: st.RegistrationNo, err: err}
		})
	}
	wg.Wait()
	close(results)

	seen := make(map[string]bool, n)
	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent create failed: %v", r.err)
		}
		if seen[r.reg] {
			t.Fatalf("registration number %s was issued twice", r.reg)
		}
		seen[r.reg] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d distinct registration numbers, want %d", len(seen), n)
	}
}

// REQ-STU-003: numbering restarts for each school; one tenant's counter never affects another's.
func TestREQ_STU_003_CountersArePerTenant(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slugA, mobileA, _ := newTenant(t, f, "counter-a")
	slugB, mobileB, _ := newTenant(t, f, "counter-b")
	pA := principalFor(t, f, slugA, mobileA)
	pB := principalFor(t, f, slugB, mobileB)
	svc := students.NewService(f.app)

	a, err := svc.Create(ctx, pA, enrolInput(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(ctx, pB, enrolInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if a.RegistrationNo != b.RegistrationNo {
		t.Fatalf("the first student of each school should get the same number, got %s and %s",
			a.RegistrationNo, b.RegistrationNo)
	}
	if a.FamilyNumber != "F-00001" || b.FamilyNumber != "F-00001" {
		t.Fatalf("the first family of each school should be F-00001, got %s and %s", a.FamilyNumber, b.FamilyNumber)
	}
}

// REQ-API cursor pagination: pages are newest first, do not overlap, and end with no cursor.
func TestREQ_API_CursorPaginationWalksAllRowsOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "pages")
	p := principalFor(t, f, slug, mobile)
	svc := students.NewService(f.app)

	var created []string
	for range 3 {
		st, err := svc.Create(ctx, p, enrolInput(t))
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, st.ID)
	}

	page1, err := svc.List(ctx, p, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 2 || page1.NextCursor == "" {
		t.Fatalf("page 1: %d items, cursor %q; want 2 items and a cursor", len(page1.Items), page1.NextCursor)
	}
	page2, err := svc.List(ctx, p, 2, page1.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.NextCursor != "" {
		t.Fatalf("page 2: %d items, cursor %q; want 1 item and no cursor", len(page2.Items), page2.NextCursor)
	}

	// Newest first: the last created is first on page 1; the first created is last on page 2.
	if page1.Items[0].ID != created[2] || page1.Items[1].ID != created[1] || page2.Items[0].ID != created[0] {
		t.Fatalf("order is wrong: got %s %s %s", page1.Items[0].ID, page1.Items[1].ID, page2.Items[0].ID)
	}
}

func TestREQ_API_BadPagingInputsAreRejected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "badpaging")
	p := principalFor(t, f, slug, mobile)
	svc := students.NewService(f.app)

	_, err := svc.List(ctx, p, 2, "not-a-cursor!")
	if got := errCode(t, err); got != "students.cursor_invalid" {
		t.Fatalf("bad cursor: code = %s, want students.cursor_invalid", got)
	}
	for _, limit := range []int{0, 201} {
		_, err := svc.List(ctx, p, limit, "")
		if got := errCode(t, err); got != "students.limit_invalid" {
			t.Fatalf("limit %d: code = %s, want students.limit_invalid", limit, got)
		}
	}
}
