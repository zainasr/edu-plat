package attendance

import (
	"strings"
	"testing"
	"time"
)

var secret = []byte("test-secret-at-least-32-bytes-long!!")

const (
	tenantA = "11111111-1111-4111-8111-111111111111"
	tenantB = "22222222-2222-4222-8222-222222222222"
	cardID  = "5c9c6c26-e0e4-4938-af5c-f6fc35688270"
)

// BR-ATT-006, REQ-ATT-011: the card code is signed. Only the server's secret can make a valid one, a code
// is bound to its school, and a garbled scan is refused before any lookup.
func TestREQ_ATT_011_CardCodeIsSigned(t *testing.T) {
	code := cardCode(secret, tenantA, cardID)
	if len(code) != 49 || strings.Count(code, ".") != 1 {
		t.Fatalf("shape: %q", code)
	}
	if code != cardCode(secret, tenantA, cardID) {
		t.Fatal("the code is the same every time, so a card can be reprinted")
	}
	got, ok := verifyCardCode(secret, tenantA, code)
	if !ok || got != cardID {
		t.Fatalf("a valid code verifies: %v %q", ok, got)
	}
	for name, bad := range map[string]string{
		"empty":                 "",
		"no dot":                strings.ReplaceAll(code, ".", ""),
		"a changed signature":   code[:len(code)-1] + flip(code[len(code)-1]),
		"a changed card id":     "5c9c6c26e0e44938af5cf6fc35688271" + code[32:],
		"another school's":      cardCode(secret, tenantB, cardID),
		"a different secret":    cardCode([]byte("another-secret-another-secret-123456"), tenantA, cardID),
		"not hex":               strings.Repeat("z", 32) + code[32:],
		"too long":              code + "AAAA",
		"too short":             code[:40],
		"whitespace around":     " " + code + " ",
		"too short a signature": code[:44],
	} {
		if _, ok := verifyCardCode(secret, tenantA, bad); ok {
			t.Fatalf("%s must not verify: %q", name, bad)
		}
	}
	// A scanner or a person may change the case of what it reads: the code is case-insensitive.
	if got, ok := verifyCardCode(secret, tenantA, strings.ToLower(code)); !ok || got != cardID {
		t.Fatalf("a lower-case scan must verify: %v %q", ok, got)
	}
	if got, ok := verifyCardCode(secret, tenantA, strings.ToUpper(code)); !ok || got != cardID {
		t.Fatalf("an upper-case scan must verify: %v %q", ok, got)
	}
	// The signature is 80 bits, not 50.
	if len(code[33:]) != 16 {
		t.Fatalf("signature length %d", len(code[33:]))
	}
	// The key is derived, so the OTP secret and the card key are never the same key.
	if string(deriveCardKey(secret)) == string(secret) || len(deriveCardKey(secret)) != 32 {
		t.Fatal("the card key must be derived from the secret, not the secret itself")
	}
	first, second := deriveCardKey(secret), deriveCardKey(secret)
	other := deriveCardKey([]byte("another-secret-another-secret-123456"))
	if string(first) != string(second) || string(first) == string(other) {
		t.Fatal("the derivation is stable and depends on the secret")
	}
	// The code carries the school: the same card id gives another code for another school.
	if cardCode(secret, tenantA, cardID) == cardCode(secret, tenantB, cardID) {
		t.Fatal("codes must differ between schools")
	}
}

func flip(c byte) string {
	if c == 'A' {
		return "B"
	}
	return "A"
}

// REQ-ATT-002 (owner decision 2026-11-17): late after the shift start plus a grace, per shift.
func TestREQ_ATT_002_ScanStatus(t *testing.T) {
	start := minutes(8, 0)
	cases := []struct {
		at        time.Time
		lateAfter int
		want      string
	}{
		{at(7, 52), 10, "present"},
		{at(8, 0), 10, "present"},
		{at(8, 10), 10, "present"}, // exactly the grace: still on time
		{at(8, 11), 10, "late"},
		{at(9, 30), 10, "late"},
		{at(8, 1), 0, "late"},
		{at(8, 0), 0, "present"},
		{at(6, 0), 10, "present"},
	}
	for _, c := range cases {
		if got := scanStatus(c.at, start, c.lateAfter); got != c.want {
			t.Fatalf("%s grace %d: got %s, want %s", c.at.Format("15:04"), c.lateAfter, got, c.want)
		}
	}
}

func at(h, m int) time.Time { return time.Date(2026, time.November, 17, h, m, 0, 0, time.UTC) }

func minutes(h, m int) int { return h*60 + m }

// US-ATT-003: a second scan of the same card within 2 minutes is ignored.
func TestUS_ATT_003_DuplicateWindow(t *testing.T) {
	first := at(7, 52)
	cases := []struct {
		name string
		prev []time.Time
		now  time.Time
		want bool
	}{
		{"no earlier scan", nil, first, false},
		{"30 seconds later", []time.Time{first}, first.Add(30 * time.Second), true},
		{"just inside 2 minutes", []time.Time{first}, first.Add(2*time.Minute - time.Second), true},
		{"exactly 2 minutes", []time.Time{first}, first.Add(2 * time.Minute), false},
		{"3 minutes later", []time.Time{first}, first.Add(3 * time.Minute), false},
		{"an offline scan uploaded out of order", []time.Time{first.Add(time.Minute)}, first, true},
		{"one old and one near", []time.Time{first.Add(-time.Hour), first.Add(-time.Minute)}, first, true},
	}
	for _, c := range cases {
		if got := isDuplicateScan(c.prev, c.now); got != c.want {
			t.Fatalf("%s: got %v", c.name, got)
		}
	}
}

// Offline scans carry their own time; it must be recent and not in the future.
func TestUS_ATT_003_ValidateScanTime(t *testing.T) {
	now := at(8, 0)
	day := 24 * time.Hour
	if got, err := validateScanTime(nil, now, day); err != nil || !got.Equal(now) {
		t.Fatalf("default is now: %v %v", got, err)
	}
	ok := now.Add(-23 * time.Hour)
	if got, err := validateScanTime(&ok, now, day); err != nil || !got.Equal(ok) {
		t.Fatalf("a queued scan from earlier: %v %v", got, err)
	}
	skew := now.Add(90 * time.Second)
	if _, err := validateScanTime(&skew, now, day); err != nil {
		t.Fatalf("a little clock skew is fine: %v", err)
	}
	for name, bad := range map[string]time.Time{
		"the future":   now.Add(10 * time.Minute),
		"two days ago": now.Add(-48 * time.Hour),
		"just too old": now.Add(-24*time.Hour - time.Second),
	} {
		if _, err := validateScanTime(&bad, now, day); err == nil || code(t, err) != "attendance.scan_time_invalid" {
			t.Fatalf("%s must be refused: %v", name, err)
		}
	}
}

func TestREQ_ATT_ValidateGateSettings(t *testing.T) {
	good := SettingsInput{RegisterDueTime: "10:00", EditWindowDays: 7, LowAttendancePercent: 75, MorningStart: "08:00", EveningStart: "13:30", LateAfterMinutes: 15}
	got, err := validateSettings(good)
	if err != nil || got.MorningStartMin != 480 || got.EveningStartMin != 810 || got.LateAfterMinutes != 15 {
		t.Fatalf("good: %+v %v", got, err)
	}
	for name, edit := range map[string]func(*SettingsInput){
		"morning not a time": func(s *SettingsInput) { s.MorningStart = "early" },
		"evening 25:00":      func(s *SettingsInput) { s.EveningStart = "25:00" },
		"grace negative":     func(s *SettingsInput) { s.LateAfterMinutes = -1 },
		"grace over 120":     func(s *SettingsInput) { s.LateAfterMinutes = 121 },
	} {
		in := good
		edit(&in)
		if _, err := validateSettings(in); err == nil || code(t, err) != "attendance.settings_invalid" {
			t.Fatalf("%s must be refused: %v", name, err)
		}
	}
	if _, err := validateSettings(SettingsInput{RegisterDueTime: "10:00", EditWindowDays: 7, LowAttendancePercent: 75, MorningStart: "08:00", EveningStart: "13:00", LateAfterMinutes: 0}); err != nil {
		t.Fatalf("no grace at all is allowed: %v", err)
	}
}

func TestREQ_ATT_ValidateCardDisable(t *testing.T) {
	if r, err := validateCardReason("  lost on the bus "); err != nil || r != "lost on the bus" {
		t.Fatalf("got %q %v", r, err)
	}
	for _, bad := range []string{"", "  ", "ab", strings.Repeat("x", 201)} {
		if _, err := validateCardReason(bad); err == nil || code(t, err) != "attendance.card_invalid_reason" {
			t.Fatalf("%q must be refused: %v", bad, err)
		}
	}
}

// The service signs cards with a key derived from the secret, never with the secret itself.
func TestREQ_ATT_011_ServiceUsesTheDerivedKey(t *testing.T) {
	svc := NewService(nil, nil, nil, secret)
	if string(svc.cardKey) == string(secret) {
		t.Fatal("the card key must not be the raw secret")
	}
	if string(svc.cardKey) != string(deriveCardKey(secret)) {
		t.Fatal("the service must use the derived key")
	}
	if cardCode(svc.cardKey, tenantA, cardID) == cardCode(secret, tenantA, cardID) {
		t.Fatal("a code signed with the raw secret must not equal the service's")
	}
}
