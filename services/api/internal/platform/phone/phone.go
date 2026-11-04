// Package phone normalises Pakistani mobile numbers to E.164 and masks them for logs.
// Storage and comparison use E.164 (+923XXXXXXXXX). Display uses 03XXXXXXXXX (data model §1).
package phone

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalid is returned for anything that is not a Pakistani mobile number.
var ErrInvalid = errors.New("phone: not a Pakistani mobile number")

var mobilePattern = regexp.MustCompile(`^(\+92|0)(3[0-9]{9})$`)

// Normalize accepts 03XXXXXXXXX or +923XXXXXXXXX (spaces and dashes are ignored)
// and returns +923XXXXXXXXX.
func Normalize(raw string) (string, error) {
	compact := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(raw))
	m := mobilePattern.FindStringSubmatch(compact)
	if m == nil {
		return "", ErrInvalid
	}
	return "+92" + m[2], nil
}

// Mask hides the middle digits for logs and messages: +923001234567 -> +92300****567.
// Invalid input is fully masked.
func Mask(e164 string) string {
	if len(e164) < 8 {
		return "****"
	}
	return e164[:6] + strings.Repeat("*", len(e164)-9) + e164[len(e164)-3:]
}
