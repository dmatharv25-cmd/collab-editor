package crdt

import "errors"

var (
	ErrOutOfRange        = errors.New("crdt: position out of range")
	ErrMissingDependency = errors.New("crdt: operation depends on an element not yet seen")
)

type OpType int

const (
	OpInsert OpType = iota
	OpDelete
)

// Op is the unit that gets sent over the network.
type Op struct {
	Type   OpType
	ID     ID   // insert: ID of the new character
	After  ID   // insert: the character it goes after (zero = start of doc)
	Value  rune // insert: the character itself
	Target ID   // delete: the character to delete
}

// node is one character in a doubly linked list (tombstones included).
type node struct {
	id         ID
	value      rune
	deleted    bool // tombstone
	prev, next *node
}

// Doc is one replica of the document.
type Doc struct {
	client  string
	clock   uint64
	head    *node // sentinel before the first character
	index   map[ID]*node
	visible int

	// Finger: a remembered spot in the list. fCount is the number of
	// visible characters at or before fNode. It makes sequential local
	// edits O(1) instead of O(n).
	fNode  *node
	fCount int
}

func New(client string) *Doc {
	h := &node{}
	return &Doc{client: client, head: h, index: make(map[ID]*node), fNode: h}
}

// Len returns the number of visible characters.
func (d *Doc) Len() int { return d.visible }

// String returns the visible text.
func (d *Doc) String() string {
	out := make([]rune, 0, d.visible)
	for n := d.head.next; n != nil; n = n.next {
		if !n.deleted {
			out = append(out, n.value)
		}
	}
	return string(out)
}

func (d *Doc) resetFinger() {
	d.fNode, d.fCount = d.head, 0
}

// nodeAt returns the node holding the visible character at pos (0-based)
// and moves the finger to it. The caller must ensure 0 <= pos < visible.
func (d *Doc) nodeAt(pos int) *node {
	target := pos + 1
	cur, c := d.fNode, d.fCount
	for c < target { // walk forward
		cur = cur.next
		if !cur.deleted {
			c++
		}
	}
	for c > target || (c == target && cur.deleted) { // walk backward
		if !cur.deleted {
			c--
		}
		cur = cur.prev
	}
	d.fNode, d.fCount = cur, c
	return cur
}

// LocalInsert inserts r at visible position pos and returns the op to broadcast.
func (d *Doc) LocalInsert(pos int, r rune) (Op, error) {
	if pos < 0 || pos > d.visible {
		return Op{}, ErrOutOfRange
	}
	var after ID
	if pos > 0 {
		after = d.nodeAt(pos - 1).id
	}
	d.clock++
	op := Op{Type: OpInsert, ID: ID{Client: d.client, Seq: d.clock}, After: after, Value: r}
	n, err := d.integrateInsert(op)
	if err != nil {
		return Op{}, err
	}
	d.fNode, d.fCount = n, pos+1
	return op, nil
}

// LocalDelete deletes the visible character at pos and returns the op to broadcast.
func (d *Doc) LocalDelete(pos int) (Op, error) {
	if pos < 0 || pos >= d.visible {
		return Op{}, ErrOutOfRange
	}
	n := d.nodeAt(pos)
	n.deleted = true
	d.visible--
	d.fNode, d.fCount = n, pos
	return Op{Type: OpDelete, Target: n.id}, nil
}

// Apply integrates an operation (local or remote). It is idempotent.
// It returns ErrMissingDependency if the op refers to an element
// that has not arrived yet; the caller should retry later.
func (d *Doc) Apply(op Op) error {
	switch op.Type {
	case OpInsert:
		n, err := d.integrateInsert(op)
		if err != nil {
			return err
		}
		if n != nil {
			d.resetFinger()
		}
	case OpDelete:
		n, ok := d.index[op.Target]
		if !ok {
			return ErrMissingDependency
		}
		if !n.deleted {
			n.deleted = true
			d.visible--
			d.resetFinger()
		}
	}
	return nil
}

// integrateInsert places a new node in the list. It returns (nil, nil)
// for a duplicate op.
func (d *Doc) integrateInsert(op Op) (*node, error) {
	if _, ok := d.index[op.ID]; ok {
		return nil, nil // duplicate delivery
	}
	prev := d.head
	if !op.After.IsZero() {
		p, ok := d.index[op.After]
		if !ok {
			return nil, ErrMissingDependency
		}
		prev = p
	}
	// Skip over elements with a greater ID: they win the tie-break.
	for prev.next != nil && prev.next.id.Greater(op.ID) {
		prev = prev.next
	}
	n := &node{id: op.ID, value: op.Value, prev: prev, next: prev.next}
	if prev.next != nil {
		prev.next.prev = n
	}
	prev.next = n
	d.index[op.ID] = n
	d.visible++
	if op.ID.Seq > d.clock {
		d.clock = op.ID.Seq // keep the Lamport clock ahead of everything seen
	}
	return n, nil
}
