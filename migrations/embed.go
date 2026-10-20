// Package migrations embeds the SQLite schema migrations so the binary can
// migrate itself without shipping loose files at runtime.
package migrations

import "embed"

// FS holds every *.sql migration in this directory. File names must be
// immutable once released: the numeric prefix is the schema version.
//
//go:embed *.sql
var FS embed.FS
