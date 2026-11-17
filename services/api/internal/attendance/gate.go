package attendance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"strings"
	"time"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

// The gate scan (REQ-ATT-011, ADR-0023). A student's ID card carries a signed code. Pure rules here: the
// signature, the late cutoff, the duplicate window and the scan time.

const (
	duplicateWindow = 2 * time.Minute
	maxScanSkew     = 2 * time.Minute
	cardIDHexLen    = 32
	sigLen          = 16 // base32 characters: 80 bits
	codeLen         = cardIDHexLen + 1 + sigLen
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// deriveCardKey makes the key that signs card codes from the server's secret, so the secret that protects
// sign-in codes is never itself used as the card key (key separation).
func deriveCardKey(secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("edu-plat/student-card-key/v1"))
	return mac.Sum(nil)
}

// signature is a keyed checksum of the school and the card, so a code made for one school or one card
// verifies for no other, and only the server's secret can make one.
func signature(secret []byte, tenantID, cardID string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("student-card:" + tenantID + ":" + strings.ReplaceAll(cardID, "-", "")))
	return b32.EncodeToString(mac.Sum(nil))[:sigLen]
}

// cardCode is what the QR on the card holds: the card id (32 hex digits) and a signature. It is the same
// every time, so a card can be reprinted from the database without storing the code.
func cardCode(secret []byte, tenantID, cardID string) string {
	return strings.ReplaceAll(cardID, "-", "") + "." + signature(secret, tenantID, cardID)
}

// verifyCardCode checks a scanned code and returns the card id it names. Anything malformed, signed by
// another secret, or made for another school is refused without a lookup.
func verifyCardCode(secret []byte, tenantID, code string) (cardID string, ok bool) {
	if len(code) != codeLen || code[cardIDHexLen] != '.' {
		return "", false
	}
	// Case does not matter (a scanner or a person may change it): the id is hex, the signature base32.
	code = strings.ToLower(code[:cardIDHexLen]) + "." + strings.ToUpper(code[cardIDHexLen+1:])
	raw, err := hex.DecodeString(code[:cardIDHexLen])
	if err != nil || len(raw) != 16 {
		return "", false
	}
	h := code[:cardIDHexLen]
	id := h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
	if !hmac.Equal([]byte(code[cardIDHexLen+1:]), []byte(signature(secret, tenantID, id))) {
		return "", false
	}
	return id, true
}

// scanStatus is present until the shift's start plus the grace, then late. at is local (Pakistan) time and
// startMin is minutes after midnight.
func scanStatus(at time.Time, startMin, lateAfterMinutes int) string {
	if at.Hour()*60+at.Minute() > startMin+lateAfterMinutes {
		return "late"
	}
	return "present"
}

// isDuplicateScan is true when another scan of the same student lies within 2 minutes of this one, in
// either direction, so an offline computer uploading out of order still finds the repeat.
func isDuplicateScan(previous []time.Time, at time.Time) bool {
	for _, p := range previous {
		d := at.Sub(p)
		if d < 0 {
			d = -d
		}
		if d < duplicateWindow {
			return true
		}
	}
	return false
}

// validateScanTime takes the time an offline computer recorded for a scan: not older than maxAge (the
// school's correction window, so a computer offline over a weekend can still upload) and not in the future
// beyond a little clock skew. With none given it is now.
func validateScanTime(at *time.Time, now time.Time, maxAge time.Duration) (time.Time, error) {
	if at == nil {
		return now, nil
	}
	if at.After(now.Add(maxScanSkew)) || at.Before(now.Add(-maxAge)) {
		return time.Time{}, apperr.Validation("attendance.scan_time_invalid", "The scan time must be recent (within the correction window) and not in the future.",
			apperr.Field{Field: "scanned_at", Message: "Within the correction window, not in the future."})
	}
	return *at, nil
}

func validateCardReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < minReason || len(reason) > maxReason {
		return "", apperr.Validation("attendance.card_invalid_reason", "A reason of 3 to 200 characters is required.",
			apperr.Field{Field: "reason", Message: "3 to 200 characters."})
	}
	return reason, nil
}
