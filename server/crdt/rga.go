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

type element struct {
id      ID
value   rune
deleted bool // tombstone
}

// Doc is one replica of the document.
type Doc struct {
client  string
clock   uint64
elems   []element // all elements, including tombstones, in document order
seen    map[ID]struct{}
visible int
}

func New(client string) *Doc {
return &Doc{client: client, seen: make(map[ID]struct{})}
}

// Len returns the number of visible characters.
func (d *Doc) Len() int { return d.visible }

// String returns the visible text.
func (d *Doc) String() string {
out := make([]rune, 0, d.visible)
for _, e := range d.elems {
if !e.deleted {
out = append(out, e.value)
}
}
return string(out)
}

// LocalInsert inserts r at visible position pos and returns the op to broadcast.
func (d *Doc) LocalInsert(pos int, r rune) (Op, error) {
if pos < 0 || pos > d.visible {
return Op{}, ErrOutOfRange
}
var after ID
if pos > 0 {
after = d.elems[d.visibleIndex(pos-1)].id
}
d.clock++
op := Op{Type: OpInsert, ID: ID{Client: d.client, Seq: d.clock}, After: after, Value: r}
if err := d.Apply(op); err != nil {
return Op{}, err
}
return op, nil
}

// LocalDelete deletes the visible character at pos and returns the op to broadcast.
func (d *Doc) LocalDelete(pos int) (Op, error) {
if pos < 0 || pos >= d.visible {
return Op{}, ErrOutOfRange
}
op := Op{Type: OpDelete, Target: d.elems[d.visibleIndex(pos)].id}
if err := d.Apply(op); err != nil {
return Op{}, err
}
return op, nil
}

// Apply integrates an operation (local or remote). It is idempotent.
// It returns ErrMissingDependency if the op refers to an element
// that has not arrived yet; the caller should retry later.
func (d *Doc) Apply(op Op) error {
switch op.Type {
case OpInsert:
if _, ok := d.seen[op.ID]; ok {
return nil // duplicate delivery
}
idx := 0
if !op.After.IsZero() {
i := d.indexOf(op.After)
if i < 0 {
return ErrMissingDependency
}
idx = i + 1
}
// Skip over elements with a greater ID: they win the tie-break.
for idx < len(d.elems) && d.elems[idx].id.Greater(op.ID) {
idx++
}
d.elems = append(d.elems, element{})
copy(d.elems[idx+1:], d.elems[idx:])
d.elems[idx] = element{id: op.ID, value: op.Value}
d.seen[op.ID] = struct{}{}
d.visible++
if op.ID.Seq > d.clock {
d.clock = op.ID.Seq // keep the Lamport clock ahead of everything seen
}
case OpDelete:
i := d.indexOf(op.Target)
if i < 0 {
return ErrMissingDependency
}
if !d.elems[i].deleted {
d.elems[i].deleted = true
d.visible--
}
}
return nil
}

// indexOf finds an element by ID (linear scan; we will optimise this later).
func (d *Doc) indexOf(id ID) int {
for i := range d.elems {
if d.elems[i].id == id {
return i
}
}
return -1
}

// visibleIndex maps a visible position to an index in elems.
func (d *Doc) visibleIndex(pos int) int {
n := 0
for i := range d.elems {
if d.elems[i].deleted {
continue
}
if n == pos {
return i
}
n++
}
return -1
}
