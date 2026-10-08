// Package pgerr classifies PostgreSQL driver errors that indicate the database
// schema does not match the running code, so callers can respond with an
// actionable message instead of leaking raw SQL errors.
package pgerr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Postgres SQLSTATE classes that mean "the schema does not match the code".
const (
	pgUndefinedTable  = "42P01"
	pgUndefinedColumn = "42703"
)

// IsUndefinedRelation reports whether err is (or wraps) "relation does not
// exist" — i.e. the database has not been migrated.
func IsUndefinedRelation(err error) bool { return hasPgCode(err, pgUndefinedTable) }

// IsSchemaMismatch is true for a missing table OR column: both mean the
// database schema is behind the running code.
func IsSchemaMismatch(err error) bool {
	return hasPgCode(err, pgUndefinedTable) || hasPgCode(err, pgUndefinedColumn)
}

func hasPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
