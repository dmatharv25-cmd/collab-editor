package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
)

func TestDuplicateOpsAreAckedAndIgnored(t *testing.T) {
	h := New()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.ServeWS)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?room=dup"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	doc := crdt.New("A")
	ins, _ := doc.LocalInsert(0, 'x')
	del, _ := doc.LocalDelete(0)

	// The client computes the same keys; pin the format so both sides stay in sync.
	if opKey(ins) != "0|A:1|:0" || opKey(del) != "1|:0|A:1" {
		t.Fatalf("unexpected keys: %q %q", opKey(ins), opKey(del))
	}

	// Send the insert three times (a client resending after reconnects), then the delete twice.
	sends := []crdt.Op{ins, ins, ins, del, del}
	for _, op := range sends {
		b, _ := json.Marshal(op)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatal(err)
		}
	}
	// Every send must be acked, duplicates included.
	for i, op := range sends {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var msg []string
		if err := json.Unmarshal(data, &msg); err != nil || len(msg) != 2 || msg[0] != "ack" || msg[1] != opKey(op) {
			t.Fatalf("message %d: expected ack for %q, got %s", i, opKey(op), data)
		}
	}

	r := h.room("dup")
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.log) != 2 {
		t.Fatalf("expected 2 logged ops (insert + delete), got %d", len(r.log))
	}
	if r.doc.String() != "" {
		t.Fatalf("expected empty text after delete, got %q", r.doc.String())
	}
}
