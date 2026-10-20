// Package database opens the SQLite database, applies connection-level PRAGMAs
// and runs embedded migrations. It must not know about HTTP, LLM or business rules.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Open opens (and creates if needed) the SQLite database at path.
//
// Every connection gets foreign_keys=1 and a busy timeout, as required by the
// spec. WAL is enabled once at startup, outside of any transaction, because
// journal_mode is a database-level setting that cannot run inside a transaction.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	dsn := "file:" + path + "?" + query.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", path, err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	// Enforce the busy timeout and verify foreign keys on the live connection.
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}
	if err := checkForeignKeys(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func checkForeignKeys(ctx context.Context, db *sql.DB) error {
	var enabled int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		return fmt.Errorf("read foreign_keys pragma: %w", err)
	}
	if enabled != 1 {
		return fmt.Errorf("sqlite foreign_keys pragma is disabled; refusing to start")
	}
	return nil
}

// Now returns the current UTC time truncated to seconds, which keeps stored
// RFC3339 timestamps stable and comparable as strings.
func Now() time.Time { return time.Now().UTC().Truncate(time.Second) }
