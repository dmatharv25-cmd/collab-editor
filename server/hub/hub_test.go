package hub

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
)

type testClient struct {
	mu      sync.Mutex
	doc     *crdt.Doc
	pending []crdt.Op
	conn    *websocket.Conn
}

func dial(ctx context.Context, url, name string) (*testClient, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	c := &testClient{doc: crdt.New(name), conn: conn}
	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var op crdt.Op
			if json.Unmarshal(data, &op) == nil {
				c.receive(op)
			}
		}
	}()
	return c, nil
}

// receive applies a remote op, buffering it if its dependency has not arrived.
func (c *testClient) receive(op crdt.Op) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = append(c.pending, op)
	for progress := true; progress; {
		progress = false
		var still []crdt.Op
		for _, p := range c.pending {
			if err := c.doc.Apply(p); err != nil {
				still = append(still, p)
				continue
			}
			progress = true
		}
		c.pending = still
	}
}

func (c *testClient) text() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.doc.String()
}

// edit makes one random local edit and sends it to the server.
func (c *testClient) edit(ctx context.Context, rng *rand.Rand) error {
	c.mu.Lock()
	var op crdt.Op
	var err error
	if c.doc.Len() > 0 && rng.Intn(4) == 0 {
		op, err = c.doc.LocalDelete(rng.Intn(c.doc.Len()))
	} else {
		op, err = c.doc.LocalInsert(rng.Intn(c.doc.Len()+1), rune('a'+rng.Intn(26)))
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(op)
	return c.conn.Write(ctx, websocket.MessageText, b)
}

func TestClientsConverge(t *testing.T) {
	h := New()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.ServeWS)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?room=test"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var clients []*testClient
	for _, name := range []string{"A", "B", "C"} {
		c, err := dial(ctx, url, name)
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
	}

	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Add(1)
		go func(i int, c *testClient) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(i + 1)))
			for n := 0; n < 150; n++ {
				if err := c.edit(ctx, rng); err != nil {
					t.Error(err)
					return
				}
				time.Sleep(time.Duration(rng.Intn(3)) * time.Millisecond)
			}
		}(i, c)
	}
	wg.Wait()

	// A late joiner must catch up on the whole history (450 ops, more than the old 256 buffer).
	late, err := dial(ctx, url, "D")
	if err != nil {
		t.Fatal(err)
	}
	clients = append(clients, late)

	deadline := time.Now().Add(10 * time.Second)
	for {
		want := h.Text("test")
		ok := len(want) > 0
		for _, c := range clients {
			if c.text() != want {
				ok = false
			}
		}
		if ok {
			t.Logf("converged: %d characters across %d clients", len(want), len(clients))
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("did not converge: server has %d chars, clients have %d %d %d %d",
				len(want), len(clients[0].text()), len(clients[1].text()),
				len(clients[2].text()), len(clients[3].text()))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
