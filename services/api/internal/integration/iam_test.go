package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/zainasr/edu-plat/services/api/internal/api"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// US-IAM-003: after 5 wrong codes the number is locked, even for the right code.
func TestUS_IAM_003_FifthWrongCodeLocks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "lock")

	if _, err := f.iam.RequestOTP(ctx, mobile); err != nil {
		t.Fatal(err)
	}
	right := f.sender.codeFor(mobile)

	for i := 1; i <= 5; i++ {
		_, err := f.iam.VerifyOTP(ctx, mobile, "000000", slug)
		if got := errCode(t, err); got != "auth.otp_invalid" {
			t.Fatalf("wrong code %d: code = %s, want auth.otp_invalid", i, got)
		}
	}

	_, err := f.iam.VerifyOTP(ctx, mobile, right, slug)
	e, ok := apperr.As(err)
	if !ok || e.Status != http.StatusLocked || e.Code != "auth.otp_locked" {
		t.Fatalf("after 5 wrong codes the right code must be locked (423), got %v", err)
	}
}

// US-IAM-003: a code works once. The second use is refused like any wrong code.
func TestUS_IAM_003_CodeIsSingleUse(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "once")

	if _, err := f.iam.RequestOTP(ctx, mobile); err != nil {
		t.Fatal(err)
	}
	code := f.sender.codeFor(mobile)

	if _, err := f.iam.VerifyOTP(ctx, mobile, code, slug); err != nil {
		t.Fatalf("first use should work: %v", err)
	}
	_, err := f.iam.VerifyOTP(ctx, mobile, code, slug)
	if got := errCode(t, err); got != "auth.otp_invalid" {
		t.Fatalf("second use: code = %s, want auth.otp_invalid", got)
	}
}

// US-IAM-003: a code expires after 5 minutes. The test ages the stored row instead of waiting.
func TestUS_IAM_003_ExpiredCodeIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "expire")

	if _, err := f.iam.RequestOTP(ctx, mobile); err != nil {
		t.Fatal(err)
	}
	code := f.sender.codeFor(mobile)
	if _, err := f.admin.Exec(ctx,
		`UPDATE otp_challenges SET expires_at = now() - interval '1 minute' WHERE mobile = $1`, mobile); err != nil {
		t.Fatalf("age the challenge: %v", err)
	}

	_, err := f.iam.VerifyOTP(ctx, mobile, code, slug)
	if got := errCode(t, err); got != "auth.otp_invalid" {
		t.Fatalf("expired code: code = %s, want auth.otp_invalid", got)
	}
}

// US-IAM-003: an unknown number gets the same answer and no code is stored,
// so the response does not reveal who is registered.
func TestUS_IAM_003_UnknownNumberStoresNothingAndAnswersTheSame(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	unknown := newMobile(t)

	if _, err := f.iam.RequestOTP(ctx, unknown); err != nil {
		t.Fatalf("an unknown number must not return an error: %v", err)
	}
	var n int
	if err := f.admin.QueryRow(ctx, `SELECT count(*) FROM otp_challenges WHERE mobile = $1`, unknown).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("stored %d challenges for an unknown number, want 0", n)
	}
}

// US-IAM-003: a real user who is not a member of the tenant looks the same as a wrong code.
func TestUS_IAM_003_NonMemberGetsTheSameErrorAsAWrongCode(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, mobileA, _ := newTenant(t, f, "member")
	slugB, _, _ := newTenant(t, f, "outsider")

	if _, err := f.iam.RequestOTP(ctx, mobileA); err != nil {
		t.Fatal(err)
	}
	_, err := f.iam.VerifyOTP(ctx, mobileA, f.sender.codeFor(mobileA), slugB)
	if got := errCode(t, err); got != "auth.otp_invalid" {
		t.Fatalf("non-member: code = %s, want auth.otp_invalid (no membership leak)", got)
	}
}

// REQ-IAM-015: a revoked token stops working at once.
func TestREQ_IAM_015_RevokedTokenIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "revoked")
	p := principalFor(t, f, slug, mobile)

	pat, err := f.iam.CreatePAT(ctx, p, "to-revoke", 30)
	if err != nil {
		t.Fatalf("create PAT: %v", err)
	}
	if _, err := f.iam.Authenticate(ctx, pat.Plaintext); err != nil {
		t.Fatalf("the token should work before revocation: %v", err)
	}
	if _, err := f.admin.Exec(ctx, `UPDATE api_tokens SET revoked_at = now() WHERE id = $1`, pat.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	_, err = f.iam.Authenticate(ctx, pat.Plaintext)
	if got := errCode(t, err); got != "auth.token_invalid" {
		t.Fatalf("revoked token: code = %s, want auth.token_invalid", got)
	}
}

// REQ-IAM-015: an expired token is refused with its own code, so a client can ask the user to sign in again.
func TestREQ_IAM_015_ExpiredTokenIsRefused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "expired")
	p := principalFor(t, f, slug, mobile)

	pat, err := f.iam.CreatePAT(ctx, p, "to-expire", 30)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `UPDATE api_tokens SET expires_at = now() - interval '1 minute' WHERE id = $1`, pat.ID); err != nil {
		t.Fatalf("age the token: %v", err)
	}

	_, err = f.iam.Authenticate(ctx, pat.Plaintext)
	if got := errCode(t, err); got != "auth.token_expired" {
		t.Fatalf("expired token: code = %s, want auth.token_expired", got)
	}
}

// REQ-IAM-015: a PAT never carries more than a day to a year. 0 means the 90-day default.
func TestREQ_IAM_015_ExpiryOutOfRangeIsRejected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, _ := newTenant(t, f, "range")
	p := principalFor(t, f, slug, mobile)

	for _, days := range []int{-1, 366} {
		_, err := f.iam.CreatePAT(ctx, p, "bad-range", days)
		if got := errCode(t, err); got != "token.expiry_invalid" {
			t.Fatalf("days=%d: code = %s, want token.expiry_invalid", days, got)
		}
	}
	if _, err := f.iam.CreatePAT(ctx, p, "default-range", 0); err != nil {
		t.Fatalf("0 days should use the default: %v", err)
	}
}

// REQ-IAM-002 and API conventions §3: a signed-in caller without the permission gets 403.
func TestREQ_IAM_002_CallerWithoutThePermissionGets403(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "noperm")
	session := signIn(t, f, mobile, slug)
	h := api.Handler(api.NewServer(f.iam, f.org, f.students), f.iam)

	// Remove every role: the membership stays active but holds no permission.
	if _, err := f.admin.Exec(ctx, `DELETE FROM role_assignments WHERE tenant_id = $1`, tenantID); err != nil {
		t.Fatalf("remove roles: %v", err)
	}

	rec := do(t, h, http.MethodPost, "/api/v1/students", session, `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %s", rec.Code, rec.Body)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	mustJSON(t, rec.Body.Bytes(), &body)
	if body.Error.Code != "auth.permission_denied" {
		t.Fatalf("error code = %q, want auth.permission_denied", body.Error.Code)
	}
}

// REQ-IAM-020: every sign-in is audited, with the session token that was issued.
func TestREQ_IAM_020_SignInIsAudited(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	slug, mobile, tenantID := newTenant(t, f, "signin-audit")
	signIn(t, f, mobile, slug)

	var n int
	err := f.admin.QueryRow(ctx,
		`SELECT count(*) FROM audit_log
		  WHERE tenant_id = $1 AND action = 'auth.signed_in' AND token_id IS NOT NULL AND actor_user_id IS NOT NULL`,
		tenantID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want one auth.signed_in audit row with actor and token, got %d", n)
	}
}
