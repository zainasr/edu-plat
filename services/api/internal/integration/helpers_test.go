package integration

import (
	"context"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/org"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// principalFor signs in through the real OTP flow and returns the authenticated principal.
func principalFor(t *testing.T, f *fixture, slug, mobile string) iam.Principal {
	t.Helper()
	p, err := f.iam.Authenticate(context.Background(), signIn(t, f, mobile, slug))
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	return p
}

// structure is a tenant's session, class and section, ready to enrol students in.
type structure struct {
	SessionID string
	ClassID   string
	SectionID string
}

// newStructure creates a current session, "Class 5" and its morning section "A" for the
// principal's tenant, through the real org service. Call it once per tenant.
func newStructure(t *testing.T, f *fixture, p iam.Principal) structure {
	t.Helper()
	ctx := context.Background()
	session, err := f.org.CreateSession(ctx, p, org.SessionInput{
		Name:     "2026-27",
		StartsOn: mustDate(t, "2026-04-01"),
		EndsOn:   mustDate(t, "2027-03-31"),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	class, err := f.org.CreateClass(ctx, p, org.ClassInput{Name: "Class 5", GradeLevel: 5})
	if err != nil {
		t.Fatalf("create class: %v", err)
	}
	section, err := f.org.CreateSection(ctx, p, org.SectionInput{ClassID: class.ID, Name: "A", Shift: "morning"})
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	return structure{SessionID: session.ID, ClassID: class.ID, SectionID: section.ID}
}

// errCode returns the stable code of an *apperr.Error. It fails the test if err is nil or is
// not a client-facing error, so a test cannot pass on the wrong kind of failure.
func errCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("want an apperr, got %T: %v", err, err)
	}
	return e.Code
}
