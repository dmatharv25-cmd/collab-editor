package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"

	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
	_ "modernc.org/sqlite"
)

const maxBatch = 128

var errClosed = errors.New("store closed")

type req struct {
	room, key, data string
	done            chan error
}

// SQLite persists every op a room receives. Appends from all rooms are
// grouped into one transaction per disk write (group commit).
type SQLite struct {
	db   *sql.DB
	reqs chan req
	quit chan struct{}
	once sync.Once
	wg   sync.WaitGroup
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
	s := &SQLite{db: db, reqs: make(chan req, 1024), quit: make(chan struct{})}
	s.wg.Add(1)
	go s.writer()
	return s, nil
}

func (s *SQLite) Close() error {
	s.once.Do(func() { close(s.quit) })
	s.wg.Wait()
	return s.db.Close()
}

// Append saves one op and blocks until it is on disk. Saving the same
// (room, key) twice is a no-op.
func (s *SQLite) Append(room, key string, op crdt.Op) error {
	b, err := json.Marshal(op)
	if err != nil {
		return err
	}
	r := req{room: room, key: key, data: string(b), done: make(chan error, 1)}
	select {
	case s.reqs <- r:
	case <-s.quit:
		return errClosed
	}
	select {
	case err := <-r.done:
		return err
	case <-s.quit:
		return errClosed // no ack; the client will resend
	}
}

// writer takes one request, then grabs everything else already waiting,
// and writes the whole group in a single transaction.
func (s *SQLite) writer() {
	defer s.wg.Done()
	for {
		var batch []req
		select {
		case r := <-s.reqs:
			batch = append(batch, r)
		case <-s.quit:
			return
		}
	drain:
		for len(batch) < maxBatch {
			select {
			case r := <-s.reqs:
				batch = append(batch, r)
			default:
				break drain
			}
		}
		err := s.flush(batch)
		for _, r := range batch {
			r.done <- err
		}
	}
}

func (s *SQLite) flush(batch []req) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO ops (room, key, data) VALUES (?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range batch {
		if _, err := stmt.Exec(r.room, r.key, r.data); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
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
