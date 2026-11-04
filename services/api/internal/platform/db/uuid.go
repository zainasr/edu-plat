package db

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ParseUUID converts a canonical UUID string into the pgx type. Invalid input returns an error,
// so a malformed ID never reaches SQL.
func ParseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse uuid %q: %w", s, err)
	}
	return u, nil
}

// UUIDString formats a pgx UUID as 8-4-4-4-12 lower-case hex. An unset UUID formats as "".
func UUIDString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
