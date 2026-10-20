// Package sqlite implements the repository contracts with database/sql and
// explicit parameterised SQL.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sqlitedrv "modernc.org/sqlite"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
)

// Store implements repository.Store. A Store is either bound to a *sql.DB or to
// a *sql.Tx; WithTx returns a transaction-scoped Store.
type Store struct {
	db *sql.DB
	tx *sql.Tx
}

// New builds a Store on top of an open database handle.
func New(db *sql.DB) *Store { return &Store{db: db} }

type dbConn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) conn() dbConn {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Ping verifies the database is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// WithTx runs fn inside a transaction. Nested calls reuse the outer transaction
// so services can compose transactional steps safely.
func (s *Store) WithTx(ctx context.Context, fn func(repository.Store) error) error {
	if s.tx != nil {
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	txStore := &Store{db: s.db, tx: tx}
	if err := fn(txStore); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return fmt.Errorf("%w (rollback failed: %v)", err, rbErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return translateErr(err)
	}
	return nil
}

// ---- error translation ----

// translateErr maps driver errors onto domain sentinels so upper layers never
// depend on SQLite error codes or message text.
func translateErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var serr *sqlitedrv.Error
	if errors.As(err, &serr) {
		// Primary result code 19 == SQLITE_CONSTRAINT (unique/foreign key/check).
		if serr.Code()&0xff == 19 {
			return fmt.Errorf("%w: %v", domain.ErrConflict, err)
		}
		return err
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unique constraint"), strings.Contains(msg, "constraint failed"):
		return fmt.Errorf("%w: %v", domain.ErrConflict, err)
	case strings.Contains(msg, "database is locked"), strings.Contains(msg, "database table is locked"):
		return fmt.Errorf("%w: %v", domain.ErrConflict, err)
	}
	return err
}

// ---- value helpers ----

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tsPtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return ts(*t)
}

// parseTS parses a stored timestamp; empty/invalid values become the zero time.
func parseTS(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// parseTSPtr parses an optional timestamp stored as TEXT.
func parseTSPtr(s string) *time.Time {
	t := parseTS(s)
	if t.IsZero() {
		return nil
	}
	return &t
}

func jsonObject(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func jsonList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := []string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func limitOrDefault(limit, def, max int) int {
	if limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}
