package service

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrQuotaExceeded = errors.New("quota exceeded")
)

// PostgreSQL SQLSTATE codes we branch on
// (https://www.postgresql.org/docs/current/errcodes-appendix.html).
const (
	errCodeUniqueViolation     = "23505" // duplicate key / constraint violation
	errCodeForeignKeyViolation = "23503" // referenced row missing
)
