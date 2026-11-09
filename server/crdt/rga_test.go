package crdt

import (
"fmt"
"math/rand"
"testing"
)

// applyAll applies ops, retrying ones whose dependencies haven't arrived.
func applyAll(t *testing.T, d *Doc, ops []Op) {
t.Helper()
pending := ops
for len(pending) > 0 {
var next []Op
for _, op := range pending {
if err := d.Apply(op); err != nil {
next = append(next, op)
}
}
if len(next) == len(pending) {
t.Fatalf("stuck: %d ops can never be applied", len(next))
}
pending = next
}
}

func typeString(t *testing.T, d *Doc, s string) []Op {
t.Helper()
var ops []Op
for _, r := range s {
op, err := d.LocalInsert(d.Len(), r)
if err != nil {
t.Fatal(err)
}
ops = append(ops, op)
}
return ops
}

func TestLocalTypingAndDelete(t *testing.T) {
d := New("A")
typeString(t, d, "hello")
if d.String() != "hello" {
t.Fatalf("got %q", d.String())
}
if _, err := d.LocalDelete(1); err != nil {
t.Fatal(err)
}
if d.String() != "hllo" || d.Len() != 4 {
t.Fatalf("got %q len %d", d.String(), d.Len())
}
}

func TestConcurrentInsertSamePosition(t *testing.T) {
a, b := New("A"), New("B")
opA, _ := a.LocalInsert(0, 'x')
opB, _ := b.LocalInsert(0, 'y')
a.Apply(opB)
b.Apply(opA)
if a.String() != b.String() || a.Len() != 2 {
t.Fatalf("diverged: %q vs %q", a.String(), b.String())
}
}

func TestConcurrentEditsOnSharedText(t *testing.T) {
a, b := New("A"), New("B")
ops := typeString(t, a, "hi")
applyAll(t, b, ops)
opA, _ := a.LocalInsert(2, '!')
opB, _ := b.LocalInsert(2, '?')
a.Apply(opB)
b.Apply(opA)
if a.String() != b.String() {
t.Fatalf("diverged: %q vs %q", a.String(), b.String())
}
}

func TestIdempotent(t *testing.T) {
a, b := New("A"), New("B")
ops := typeString(t, a, "abc")
applyAll(t, b, ops)
applyAll(t, b, ops) // duplicate delivery
if b.String() != "abc" {
t.Fatalf("got %q", b.String())
}
}

// ---- randomized convergence test ----

type replica struct {
doc *Doc
log []Op
has map[string]bool
}

func newReplica(name string) *replica {
return &replica{doc: New(name), has: map[string]bool{}}
}

func opKey(op Op) string {
return fmt.Sprintf("%d|%s:%d|%s:%d", op.Type, op.ID.Client, op.ID.Seq, op.Target.Client, op.Target.Seq)
}

func (r *replica) record(op Op) {
k := opKey(op)
if !r.has[k] {
r.has[k] = true
r.log = append(r.log, op)
}
}

func deliver(t *testing.T, from, to *replica) {
t.Helper()
var fresh []Op
for _, op := range from.log {
if !to.has[opKey(op)] {
fresh = append(fresh, op)
}
}
applyAll(t, to.doc, fresh)
for _, op := range fresh {
to.record(op)
}
}

func TestRandomConvergence(t *testing.T) {
for seed := int64(1); seed <= 20; seed++ {
rng := rand.New(rand.NewSource(seed))
reps := []*replica{newReplica("A"), newReplica("B"), newReplica("C")}

for step := 0; step < 300; step++ {
i := rng.Intn(len(reps))
r := reps[i]
var op Op
var err error
if r.doc.Len() > 0 && rng.Intn(4) == 0 {
op, err = r.doc.LocalDelete(rng.Intn(r.doc.Len()))
} else {
op, err = r.doc.LocalInsert(rng.Intn(r.doc.Len()+1), rune('a'+rng.Intn(26)))
}
if err != nil {
t.Fatal(err)
}
r.record(op)

if rng.Intn(3) == 0 { // partial, random syncing
j := rng.Intn(len(reps))
if j != i {
deliver(t, r, reps[j])
}
}
}

// Full sync: everyone sends to everyone, twice.
for round := 0; round < 2; round++ {
for _, from := range reps {
for _, to := range reps {
if from != to {
deliver(t, from, to)
}
}
}
}
want := reps[0].doc.String()
for _, r := range reps {
if r.doc.String() != want {
t.Fatalf("seed %d: replicas diverged: %q vs %q", seed, want, r.doc.String())
}
}

// A brand-new replica receiving ALL ops in shuffled order must match too.
all := append([]Op(nil), reps[0].log...)
rng.Shuffle(len(all), func(a, b int) { all[a], all[b] = all[b], all[a] })
fresh := New("Z")
applyAll(t, fresh, all)
if fresh.String() != want {
t.Fatalf("seed %d: shuffled replay diverged: %q vs %q", seed, want, fresh.String())
}
}
}
