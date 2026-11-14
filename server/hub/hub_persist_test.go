package hub

import (
	"path/filepath"
	"testing"

	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
	"github.com/dmatharv25-cmd/collab-editor/server/store"
)

func TestPersistenceSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s1, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h1 := NewWithStore(s1)
	r1 := h1.room("p")
	c := &client{send: make(chan []byte, 64)}

	doc := crdt.New("A")
	var ops []crdt.Op
	for _, ch := range "abc" {
		op, _ := doc.LocalInsert(doc.Len(), ch)
		ops = append(ops, op)
	}
	// Deliver out of order: "c" and "b" must wait for their dependencies.
	r1.handle(ops[2], c)
	r1.handle(ops[1], c)
	r1.handle(ops[0], c)
	r1.handle(ops[0], c) // duplicate resend
	if got := h1.Text("p"); got != "abc" {
		t.Fatalf("before restart: got %q", got)
	}
	s1.Close()

	// "Restart": a brand-new hub and store on the same file.
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	h2 := NewWithStore(s2)
	if got := h2.Text("p"); got != "abc" {
		t.Fatalf("after restart: got %q, want %q", got, "abc")
	}

	// And the restored room keeps accepting edits.
	op, _ := doc.LocalInsert(doc.Len(), 'd')
	h2.room("p").handle(op, &client{send: make(chan []byte, 64)})
	if got := h2.Text("p"); got != "abcd" {
		t.Fatalf("after new edit: got %q", got)
	}
}
