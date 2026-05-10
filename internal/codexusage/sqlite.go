package codexusage

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func openReadonlySQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000", filepath.ToSlash(path)))
}

func hasTable(conn *sql.DB, name string) (bool, error) {
	var tableName string
	err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&tableName)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func isOptionalCodexDBError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "file is not a database") ||
		strings.Contains(msg, "database disk image is malformed") ||
		strings.Contains(msg, "unable to open database file") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "no such column")
}
