// Package db opens the SQLite database and holds its schema migrations and
// queries.
package db

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a database/sql handle. Query methods are added to this type.
type DB struct {
	*sql.DB
}

// Open opens the SQLite database at path. The database uses WAL mode, has
// foreign key enforcement enabled on every connection, and waits up to five
// seconds for a lock before returning an error.
func Open(path string) (*DB, error) {
	dsn, err := dataSourceName(path)
	if err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("open database: %w", err)
	}

	return &DB{DB: sqlDB}, nil
}

func dataSourceName(path string) (string, error) {
	if path == "" {
		return "", errors.New("database path must not be empty")
	}

	pragmas := url.Values{}
	// _pragma entries are applied by the driver to every new connection.
	pragmas.Add("_pragma", "busy_timeout(5000)")
	pragmas.Add("_pragma", "foreign_keys(1)")
	pragmas.Add("_pragma", "journal_mode(WAL)")

	if path == ":memory:" {
		return "file::memory:?cache=shared&" + pragmas.Encode(), nil
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}

	// Escape the path without escaping the slashes that make it a path.
	escaped := (&url.URL{Path: absolute}).EscapedPath()
	return "file:" + escaped + "?" + pragmas.Encode(), nil
}
