package hub

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
)

// Room is one shared document plus its connected clients.
type Room struct {
	mu      sync.Mutex
	name    string
	doc     *crdt.Doc
	log     []crdt.Op // every applied op, in the order this server applied it
	pending []crdt.Op // ops waiting for a dependency
	clients map[*client]struct{}
}

type client struct {
	send chan []byte
}

// Hub holds all rooms.
type Hub struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

func New() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

func (h *Hub) room(name string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[name]
	if !ok {
		r = &Room{name: name, doc: crdt.New("server"), clients: make(map[*client]struct{})}
		h.rooms[name] = r
	}
	return r
}

// Text returns the current text of a room (used by tests and a debug endpoint).
func (h *Hub) Text(name string) string {
	r := h.room(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.doc.String()
}

// ServeWS upgrades the HTTP request and runs the connection until it closes.
func (h *Hub) ServeWS(w http.ResponseWriter, req *http.Request) {
	name := req.URL.Query().Get("room")
	if name == "" {
		http.Error(w, "missing ?room=", http.StatusBadRequest)
		return
	}
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Println("accept:", err)
		return
	}
	defer conn.CloseNow()

	room := h.room(name)
	c := &client{send: make(chan []byte, 1024)}

	// Register and snapshot the log under one lock so no op is missed.
	// Ops that arrive after this point queue up in c.send.
	room.mu.Lock()
	room.clients[c] = struct{}{}
	snapshot := append([]crdt.Op(nil), room.log...)
	room.mu.Unlock()

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	defer func() {
		room.mu.Lock()
		delete(room.clients, c)
		room.mu.Unlock()
	}()

	// Send history directly, in order, before the writer goroutine starts.
	for _, op := range snapshot {
		b, err := json.Marshal(op)
		if err != nil {
			continue
		}
		wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
		err = conn.Write(wctx, websocket.MessageText, b)
		wcancel()
		if err != nil {
			return
		}
	}

	// Writer goroutine: from now on the only place that writes to this connection.
	go func() {
		for {
			select {
			case msg := <-c.send:
				wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				wcancel()
				if err != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Reader loop.
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var op crdt.Op
		if err := json.Unmarshal(data, &op); err != nil {
			continue
		}
		room.handle(op, c)
	}
}

// handle applies an op from a client and broadcasts it to everyone else.
func (r *Room) handle(op crdt.Op, from *client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pending = append(r.pending, op)
	// Keep applying until no pending op can make progress.
	for progress := true; progress; {
		progress = false
		var still []crdt.Op
		for _, p := range r.pending {
			if err := r.doc.Apply(p); err != nil {
				still = append(still, p)
				continue
			}
			progress = true
			r.log = append(r.log, p)
			b, _ := json.Marshal(p)
			for c := range r.clients {
				if c == from && p == op {
					continue // don't echo the original op back to its sender
				}
				select {
				case c.send <- b:
				default: // slow client: drop; it will resync on reconnect
				}
			}
		}
		r.pending = still
	}
}
