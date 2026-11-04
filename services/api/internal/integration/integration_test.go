// Package integration runs the walking skeleton against a real Postgres (ADR-0002, ADR-0011).
// It needs DATABASE_URL (the app_rt role) and ADMIN_DATABASE_URL (the migration owner),
// a migrated database (make migrate) and a running Postgres (make db-up).
package integration

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/iam"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
)

// captureSender remembers the last code per number instead of sending it.
type captureSender struct {
	mu    sync.Mutex
	codes map[string]string
}

func (c *captureSender) SendOTP(_ context.Context, mobileE164, code string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.codes[mobileE164] = code
	return nil
}

func (c *captureSender) codeFor(mobileE164 string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.codes[mobileE164]
}

// env returns the two database URLs or fails the test with a clear message.
func env(t *testing.T) (appURL, adminURL string) {
	t.Helper()
	appURL = os.Getenv("DATABASE_URL")
	adminURL = os.Getenv("ADMIN_DATABASE_URL")
	if appURL == "" || adminURL == "" {
		t.Fatal("integration tests need DATABASE_URL and ADMIN_DATABASE_URL (see make test)")
	}
	return appURL, adminURL
}

// randomSuffix gives unique slugs and numbers per run, so tests never collide.
func randomSuffix(t *testing.T) string {
	t.Helper()
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000_000))
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%09d", n.Int64())
}

// newMobile returns a fresh Pakistani mobile in E.164 form.
func newMobile(t *testing.T) string {
	t.Helper()
	return "+923" + randomSuffix(t)
}

type fixture struct {
	app    *pgxpool.Pool
	admin  *pgxpool.Pool
	sender *captureSender
	iam    *iam.Service
}

// newTenant creates a tenant with one owner through the real bootstrap path.
func newTenant(t *testing.T, f *fixture, label string) (slug, mobile, tenantID string) {
	t.Helper()
	slug = "it-" + label + "-" + randomSuffix(t)
	mobile = newMobile(t)
	tenantID, err := iam.Bootstrap(context.Background(), f.admin, iam.BootstrapInput{
		TenantSlug:  slug,
		TenantName:  "Integration " + label,
		OwnerMobile: mobile,
		OwnerName:   "Owner " + label,
	})
	if err != nil {
		t.Fatalf("bootstrap tenant %s: %v", label, err)
	}
	return slug, mobile, tenantID
}

// signIn runs the real OTP flow and returns the session token for the tenant.
func signIn(t *testing.T, f *fixture, mobile, slug string) string {
	t.Helper()
	ctx := context.Background()
	if _, err := f.iam.RequestOTP(ctx, mobile); err != nil {
		t.Fatalf("request otp: %v", err)
	}
	sess, err := f.iam.VerifyOTP(ctx, mobile, f.sender.codeFor(mobile), slug)
	if err != nil {
		t.Fatalf("verify otp: %v", err)
	}
	return sess.Token
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	appURL, adminURL := env(t)
	ctx := context.Background()

	app, err := db.Connect(ctx, appURL)
	if err != nil {
		t.Fatalf("connect as app role: %v", err)
	}
	t.Cleanup(app.Close)
	admin, err := db.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connect as owner: %v", err)
	}
	t.Cleanup(admin.Close)

	sender := &captureSender{codes: map[string]string{}}
	secret := []byte("integration-test-secret-0123456789abcdef")
	return &fixture{
		app:    app,
		admin:  admin,
		sender: sender,
		iam:    iam.NewService(app, sender, secret),
	}
}

// mustDate parses a YYYY-MM-DD fixture date.
func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
