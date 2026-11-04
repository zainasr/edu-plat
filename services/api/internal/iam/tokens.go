package iam

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// Token kinds (ADR-0011 §5).
const (
	KindSession = "session"
	KindPAT     = "pat"
)

const tokenPrefix = "edu_"

// newToken makes a token of the form edu_<kind>_<43 base64url chars> (256 random bits).
// It returns the plaintext (shown once) and its SHA-256, which is all the database keeps.
func newToken(kind string) (plain string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("random token: %w", err)
	}
	plain = tokenPrefix + kind + "_" + base64.RawURLEncoding.EncodeToString(raw)
	return plain, hashToken(plain), nil
}

// hashToken is the stored form of a token. Tokens are high-entropy, so a fast hash is safe.
func hashToken(plain string) []byte {
	sum := sha256.Sum256([]byte(plain))
	return sum[:]
}

// kindOf reads the kind from a token's prefix, or "" if the shape is wrong.
func kindOf(plain string) string {
	rest, ok := strings.CutPrefix(plain, tokenPrefix)
	if !ok {
		return ""
	}
	kind, _, ok := strings.Cut(rest, "_")
	if !ok {
		return ""
	}
	return kind
}
