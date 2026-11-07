package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes we react to (see the PostgreSQL manual, Appendix A).
const (
	codeUniqueViolation = "23505"
	codeFKViolation     = "23503"
)

// IsUniqueViolation reports whether err is Postgres refusing a duplicate in a UNIQUE
// constraint or unique index. Services turn it into a 409 with a code the client can read.
func IsUniqueViolation(err error) bool {
	return hasCode(err, codeUniqueViolation)
}

// IsForeignKeyViolation reports whether err is Postgres refusing a row that points at nothing,
// or a delete of a row something still points at.
func IsForeignKeyViolation(err error) bool {
	return hasCode(err, codeFKViolation)
}

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
