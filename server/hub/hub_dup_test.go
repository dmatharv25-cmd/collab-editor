package hub

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
	"github.com/dmatharv25-cmd/collab-editor/server/store"
)

func TestConcurrentDuplicateOps(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := NewWithStore(s)
	r := h.room("d")

	doc := crdt.New("A")
	op, _ := doc.LocalInsert(0, 'x')

	clients := make([]*client, 20)
	var wg sync.WaitGroup
	for i := range clients {
		clients[i] = &client{send: make(chan []byte, 64)}
		wg.Add(1)
		go func(c *client) {
			defer wg.Done()
			r.handle(op, c)
		}(clients[i])
	}
	wg.Wait()

	if got := h.Text("d"); got != "x" {
		t.Fatalf("text = %q, want %q", got, "x")
	}
	ops, err := s.Load("d")
	if err != nil || len(ops) != 1 {
		t.Fatalf("stored ops = %d (err %v), want 1", len(ops), err)
	}
	for i, c := range clients {
		if n := len(c.send); n != 1 {
			t.Fatalf("client %d got %d messages, want exactly 1 ack", i, n)
		}
	}
}
