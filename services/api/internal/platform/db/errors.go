package db

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes we react to (see the PostgreSQL manual, Appendix A).
const (
	codeUniqueViolation = "23505"
	codeFKViolation     = "23503"
	codeCheckViolation  = "23514"
	codeDeadlock        = "40P01"
	codeSerialization   = "40001"
)

// IsCheckViolation reports whether err is Postgres refusing a row or a commit because a CHECK
// constraint or a constraint trigger said no (for example a payment that over-pays a challan).
func IsCheckViolation(err error) bool { return hasCode(err, codeCheckViolation) }

// IsRetryable reports whether err is a deadlock or serialization failure: nothing was done, and the
// same request can safely be tried again.
func IsRetryable(err error) bool {
	return hasCode(err, codeDeadlock) || hasCode(err, codeSerialization)
}

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
