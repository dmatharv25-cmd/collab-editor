package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
)

// Store persists the ops a room receives. A nil Store means memory only.
type Store interface {
	Append(room, key string, op crdt.Op) error
	Load(room string) ([]crdt.Op, error)
}

// Room is one shared document plus its connected clients.
type Room struct {
	mu      sync.Mutex
	name    string
	store   Store
	doc     *crdt.Doc
	log     []crdt.Op // every applied op, in the order this server applied it
	pending []crdt.Op // ops waiting for a dependency
	seen    map[string]struct{}
	clients map[*client]struct{}
}

type client struct {
	send chan []byte
}

// Hub holds all rooms.
type Hub struct {
	mu    sync.Mutex
	rooms map[string]*Room
	store Store
}

func New() *Hub {
	return &Hub{rooms: make(map[string]*Room)}
}

func NewWithStore(s Store) *Hub {
	return &Hub{rooms: make(map[string]*Room), store: s}
}

// opKey identifies an op on the wire. The client computes the same string.
func opKey(op crdt.Op) string {
	return fmt.Sprintf("%d|%s:%d|%s:%d", op.Type, op.ID.Client, op.ID.Seq, op.Target.Client, op.Target.Seq)
}

func (h *Hub) room(name string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[name]
	if !ok {
		r = &Room{
			name:    name,
			store:   h.store,
			doc:     crdt.New("server"),
			seen:    make(map[string]struct{}),
			clients: make(map[*client]struct{}),
		}
		if h.store != nil {
			ops, err := h.store.Load(name)
			if err != nil {
				log.Println("load room", name, ":", err)
			}
			r.restore(ops)
			if len(ops) > 0 {
				log.Printf("restored room %q: %d ops", name, len(ops))
			}
		}
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

// restore rebuilds the document from saved ops when a room is first opened.
func (r *Room) restore(ops []crdt.Op) {
	for _, op := range ops {
		r.seen[opKey(op)] = struct{}{}
		r.integrate(op, nil)
	}
}

// handle saves an op, acks it, then applies and broadcasts it.
// Duplicates (resends after a reconnect) are acked but otherwise ignored.
func (r *Room) handle(op crdt.Op, from *client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := opKey(op)
	if _, dup := r.seen[k]; !dup {
		// Save BEFORE acking: an ack means "durably stored".
		if r.store != nil {
			if err := r.store.Append(r.name, k, op); err != nil {
				log.Println("persist:", err)
				return // no ack, so the client keeps the op and resends later
			}
		}
		r.seen[k] = struct{}{}
		r.sendAck(from, k)
		r.integrate(op, from)
		return
	}
	r.sendAck(from, k)
}

func (r *Room) sendAck(to *client, key string) {
	ack, _ := json.Marshal([]string{"ack", key})
	select {
	case to.send <- ack:
	default:
	}
}

// integrate applies an op (and anything it unblocks) and broadcasts the result.
// from may be nil, for example while restoring a room from disk.
func (r *Room) integrate(op crdt.Op, from *client) {
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
