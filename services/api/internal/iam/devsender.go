package iam

import (
	"context"
	"log/slog"

	"github.com/zainasr/edu-plat/services/api/internal/platform/phone"
)

// DevSender prints the code to the process log instead of sending it. It exists only for
// APP_ENV=dev so a developer can sign in without an SMS provider. main refuses to start
// outside dev without a real sender (ADR-0011 §4). The mobile is masked; the code is not.
type DevSender struct{}

// SendOTP logs the code for the developer. Never wire this in production.
func (DevSender) SendOTP(ctx context.Context, mobileE164, code string) error {
	slog.WarnContext(ctx, "DEV ONLY: OTP printed instead of sent",
		"mobile", phone.Mask(mobileE164), "code", code)
	return nil
}
