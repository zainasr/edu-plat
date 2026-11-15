package idempotency

import (
	"strings"
	"testing"
)

// REQ-API-010: what a key may look like.
func TestREQ_API_010_ValidateKey(t *testing.T) {
	good := []string{"7f3c9a10-aaaa-4bbb-8ccc-0123456789ab", "release:2026-08:run1", "abcdefgh", strings.Repeat("k", 128)}
	for _, k := range good {
		if err := validateKey(k); err != nil {
			t.Fatalf("%q must be allowed: %v", k, err)
		}
	}
	bad := []string{"", "short", strings.Repeat("k", 129), "has space in it", "tab\tinside!", "unicode-ключ-1234", "semi;colon;1234"}
	for _, k := range bad {
		if err := validateKey(k); err == nil {
			t.Fatalf("%q must be refused", k)
		}
	}
}

func TestREQ_API_010_RequestHash(t *testing.T) {
	base := requestHash("user-1", "/api/v1/challans/A/void", "", []byte(`{"a":1}`))
	if len(base) != 32 {
		t.Fatalf("hash must be 32 bytes")
	}
	if string(requestHash("user-1", "/api/v1/challans/A/void", "", []byte(`{"a":1}`))) != string(base) {
		t.Fatal("hash must be stable")
	}
	for name, other := range map[string][]byte{
		"another body":  requestHash("user-1", "/api/v1/challans/A/void", "", []byte(`{"a":2}`)),
		"another user":  requestHash("user-2", "/api/v1/challans/A/void", "", []byte(`{"a":1}`)),
		"another path":  requestHash("user-1", "/api/v1/challans/B/void", "", []byte(`{"a":1}`)),
		"another query": requestHash("user-1", "/api/v1/challans/A/void", "x=1", []byte(`{"a":1}`)),
		"parts shifted": requestHash("user-1/api/v1/challans", "/A/void", "", []byte(`{"a":1}`)),
	} {
		if string(other) == string(base) {
			t.Fatalf("%s must change the hash", name)
		}
	}
}

// The decision table: what to do with a request whose key already has a row.
func TestREQ_API_010_Decide(t *testing.T) {
	h1, h2 := requestHash("u", "/p", "", []byte("one")), requestHash("u", "/p", "", []byte("two"))
	const ep = "POST /api/v1/challans/release"
	row := func(status, endpoint string, hash []byte, expired bool) *Existing {
		return &Existing{Status: status, Endpoint: endpoint, Hash: hash, Expired: expired, Code: 200, Body: []byte("{}")}
	}
	cases := []struct {
		name string
		ex   *Existing
		ep   string
		hash []byte
		want Decision
	}{
		{"no row: run it", nil, ep, h1, Proceed},
		{"done, same request: replay", row("done", ep, h1, false), ep, h1, Replay},
		{"in progress, same request: 409", row("in_progress", ep, h1, false), ep, h1, InProgress},
		{"done, different body: reused", row("done", ep, h1, false), ep, h2, Reused},
		{"done, different endpoint: reused", row("done", "POST /api/v1/challans/generate", h1, false), ep, h1, Reused},
		{"in progress, different body: reused", row("in_progress", ep, h1, false), ep, h2, Reused},
		{"failed earlier: run it again", row("failed", ep, h1, false), ep, h2, Proceed},
		{"expired done row: run it again", row("done", ep, h1, true), ep, h2, Proceed},
		{"expired in-progress row (crashed): run it again", row("in_progress", ep, h1, true), ep, h1, Proceed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decide(c.ex, c.ep, c.hash); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
