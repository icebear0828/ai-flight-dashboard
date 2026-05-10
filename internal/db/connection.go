package db

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func New(dsn string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Enforce single connection pool to prevent concurrent write locks in WAL mode
	conn.SetMaxOpenConns(1)

	// WAL mode for concurrent read/write
	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")

	if err := initSchema(conn); err != nil {
		return nil, err
	}

	return &DB{conn: conn}, nil
}
func (d *DB) Close() error {
	return d.conn.Close()
}

// UpsertKnownDir records a directory known to contain JSONL files.
