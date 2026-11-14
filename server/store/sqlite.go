package store

import (
	"database/sql"
	"encoding/json"

	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
	_ "modernc.org/sqlite"
)

// SQLite persists every op a room receives.
type SQLite struct {
	db *sql.DB
}

func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer; simple and safe
	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS ops (
id   INTEGER PRIMARY KEY AUTOINCREMENT,
room TEXT NOT NULL,
key  TEXT NOT NULL,
data TEXT NOT NULL,
UNIQUE (room, key)
)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &SQLite{db: db}, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

// Append saves one op. Saving the same (room, key) twice is a no-op.
func (s *SQLite) Append(room, key string, op crdt.Op) error {
	b, err := json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO ops (room, key, data) VALUES (?, ?, ?)`, room, key, string(b))
	return err
}

// Load returns a room's ops in the order they were received.
func (s *SQLite) Load(room string) ([]crdt.Op, error) {
	rows, err := s.db.Query(`SELECT data FROM ops WHERE room = ? ORDER BY id`, room)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ops []crdt.Op
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var op crdt.Op
		if err := json.Unmarshal([]byte(data), &op); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}
