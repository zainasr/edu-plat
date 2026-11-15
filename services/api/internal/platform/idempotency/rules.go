// Package idempotency makes retried POST requests safe (REQ-API-010, ADR-0018). A client sends an
// Idempotency-Key; the first request runs and its response is stored; a repeat with the same key and
// the same body gets that response back and does nothing again.
package idempotency

import (
	"crypto/sha256"
	"regexp"

	"github.com/zainasr/edu-plat/services/api/internal/platform/apperr"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

// Decision is what to do with a request once we know whether its key was seen before.
type Decision int

const (
	Proceed    Decision = iota // run the request (the key is new, expired, or its earlier run failed)
	Replay                     // return the stored response
	InProgress                 // the first request has not finished: 409
	Reused                     // same key, different request: 422
)

func (d Decision) String() string {
	return [...]string{"proceed", "replay", "in_progress", "reused"}[d]
}

// Existing is the stored row for a key.
type Existing struct {
	Endpoint string
	Hash     []byte
	Status   string // in_progress, done or failed
	Code     int
	Body     []byte
	Expired  bool // older than 24 hours, judged by the database clock
}

func validateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return apperr.Validation("idempotency.key_invalid",
			"Idempotency-Key must be 8 to 128 characters: letters, digits, '.', '_', ':' or '-'.")
	}
	return nil
}

// requestHash identifies a request by who sent it, the concrete URL (so /challans/A/void and
// /challans/B/void differ) and the body. A different user with the same key therefore gets "reused",
// never someone else's stored response.
func requestHash(actor, path, query string, body []byte) []byte {
	h := sha256.New()
	for _, part := range []string{actor, path, query} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	h.Write(body)
	return h.Sum(nil)
}

// decide compares a request with the row stored for its key. A failed or expired row is as good as no
// row: the request runs again. Otherwise the key belongs to the first request, and only an identical
// request (same endpoint, same body) may share it.
func decide(ex *Existing, endpoint string, hash []byte) Decision {
	if ex == nil || ex.Status == "failed" || ex.Expired {
		return Proceed
	}
	if ex.Endpoint != endpoint || string(ex.Hash) != string(hash) {
		return Reused
	}
	if ex.Status == "in_progress" {
		return InProgress
	}
	return Replay
}
