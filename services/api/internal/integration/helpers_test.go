package integration

import (
	"context"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
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
