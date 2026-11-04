package iam

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zainasr/edu-plat/services/api/internal/iam/store/gen"
	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
	"github.com/zainasr/edu-plat/services/api/internal/platform/db"
	"github.com/zainasr/edu-plat/services/api/internal/platform/phone"
)

const (
	otpTTL         = 5 * time.Minute // US-IAM-003
	maxOTPAttempts = 5               // US-IAM-003: after 5 wrong codes the challenge is locked
	sessionTTL     = 12 * time.Hour  // web idle limit (REQ-IAM-011)
)

// OTPSender delivers a one-time code. The skeleton ships only a dev sender that logs the code;
// production needs a real provider (DEC-009) before the process will start outside dev.
type OTPSender interface {
	SendOTP(ctx context.Context, mobileE164, code string) error
}

// Service holds the identity rules. It is constructed once and shared by handlers.
type Service struct {
	pool   *pgxpool.Pool
	sender OTPSender
	secret []byte
	now    func() time.Time
}

// NewService builds the identity service. secret is the HMAC key for OTP codes (AUTH_SECRET).
func NewService(pool *pgxpool.Pool, sender OTPSender, secret []byte) *Service {
	return &Service{pool: pool, sender: sender, secret: secret, now: time.Now}
}

// Session is what a successful OTP verify returns. The token is shown to the caller once.
type Session struct {
	Token      string
	ExpiresAt  time.Time
	TenantID   string
	TenantSlug string
	TenantName string
}

// RequestOTP issues a code for a known number and sends it. For an unknown number it does
// nothing and returns the same answer, so the response never reveals who is registered.
func (s *Service) RequestOTP(ctx context.Context, rawMobile string) (time.Duration, error) {
	mobile, err := phone.Normalize(rawMobile)
	if err != nil {
		return 0, apperr.Validation("auth.mobile_invalid", "Enter a Pakistani mobile number, such as 03001234567.",
			apperr.Field{Field: "mobile", Message: "Not a Pakistani mobile number."})
	}

	q := gen.New(s.pool)
	if _, err := q.FindUserByMobile(ctx, mobile); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.InfoContext(ctx, "otp requested for unknown number", "mobile", phone.Mask(mobile))
			return otpTTL, nil
		}
		return 0, fmt.Errorf("find user: %w", err)
	}

	code, err := newOTPCode()
	if err != nil {
		return 0, err
	}
	expires := s.now().Add(otpTTL)
	if _, err := q.InsertOTPChallenge(ctx, gen.InsertOTPChallengeParams{
		Mobile:    mobile,
		CodeHash:  otpHash(s.secret, mobile, code),
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	}); err != nil {
		return 0, fmt.Errorf("store otp challenge: %w", err)
	}
	if err := s.sender.SendOTP(ctx, mobile, code); err != nil {
		return 0, fmt.Errorf("send otp to %s: %w", phone.Mask(mobile), err)
	}
	slog.InfoContext(ctx, "otp issued", "mobile", phone.Mask(mobile))
	return otpTTL, nil
}

// VerifyOTP checks the code and, if it is right and the number belongs to the tenant,
// returns a session for that tenant. Every failure except a malformed number returns the
// same error, so a wrong code, an unknown number and a non-member look identical.
func (s *Service) VerifyOTP(ctx context.Context, rawMobile, code, tenantSlug string) (Session, error) {
	mobile, err := phone.Normalize(rawMobile)
	if err != nil {
		return Session{}, apperr.Validation("auth.mobile_invalid", "Enter a Pakistani mobile number, such as 03001234567.",
			apperr.Field{Field: "mobile", Message: "Not a Pakistani mobile number."})
	}
	invalid := apperr.Unauthenticated("auth.otp_invalid", "The code is wrong or has expired.")

	q := gen.New(s.pool)
	user, err := q.FindUserByMobile(ctx, mobile)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, invalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("find user: %w", err)
	}

	ch, err := q.LatestOTPChallenge(ctx, mobile)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, invalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("load otp challenge: %w", err)
	}
	if ch.ConsumedAt.Valid || !s.now().Before(ch.ExpiresAt.Time) {
		return Session{}, invalid
	}
	if ch.Attempts >= maxOTPAttempts {
		return Session{}, apperr.Locked("auth.otp_locked", "Too many wrong codes. Request a new code after the current one expires.")
	}
	if !hmac.Equal(ch.CodeHash, otpHash(s.secret, mobile, code)) {
		if err := q.IncrementOTPAttempts(ctx, ch.ID); err != nil {
			return Session{}, fmt.Errorf("count otp attempt: %w", err)
		}
		slog.InfoContext(ctx, "otp rejected", "mobile", phone.Mask(mobile))
		return Session{}, invalid
	}

	// Consume atomically: two concurrent verifies with the same code cannot both succeed.
	consumed, err := q.ConsumeOTPChallenge(ctx, ch.ID)
	if err != nil {
		return Session{}, fmt.Errorf("consume otp challenge: %w", err)
	}
	if consumed == 0 {
		return Session{}, invalid
	}

	tenant, err := tenantBySlug(ctx, s.pool, tenantSlug)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, invalid
	}
	if err != nil {
		return Session{}, err
	}

	tenantUUID, err := db.ParseUUID(tenant.ID)
	if err != nil {
		return Session{}, fmt.Errorf("tenant id from directory: %w", err)
	}

	var session Session
	err = db.WithTenant(ctx, s.pool, tenant.ID, func(tx pgx.Tx) error {
		membership, err := gen.New(tx).ActiveMembership(ctx, gen.ActiveMembershipParams{
			TenantID: tenantUUID,
			UserID:   user.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid // not a member of this tenant
		}
		if err != nil {
			return fmt.Errorf("find membership: %w", err)
		}
		issued, err := issueToken(ctx, tx, s.now, tokenSpec{
			TenantID:     tenant.ID,
			MembershipID: db.UUIDString(membership),
			UserID:       db.UUIDString(user.ID),
			Kind:         KindSession,
			Name:         "web sign-in",
			TTL:          sessionTTL,
		})
		if err != nil {
			return err
		}
		session = Session{
			Token:      issued.Plaintext,
			ExpiresAt:  issued.ExpiresAt,
			TenantID:   tenant.ID,
			TenantSlug: tenant.Slug,
			TenantName: tenant.Name,
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	slog.InfoContext(ctx, "signed in", "mobile", phone.Mask(mobile), "tenant", tenant.Slug)
	return session, nil
}

// newOTPCode returns a uniformly random 6-digit code.
func newOTPCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("random otp: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// otpHash binds the code to the number and to the server key, so a stolen table is useless
// without AUTH_SECRET and a code cannot be replayed on another number.
func otpHash(secret []byte, mobileE164, code string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(mobileE164 + ":" + code))
	return mac.Sum(nil)
}
