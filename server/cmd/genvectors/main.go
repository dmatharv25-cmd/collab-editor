package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"

	"github.com/dmatharv25-cmd/collab-editor/server/crdt"
)

// Includes non-ASCII characters and an emoji to test code point handling.
var alphabet = []rune{'a', 'b', 'c', 'd', 'e', '\u00e9', '\u4e16', '\U0001F600'}

type Vector struct {
	Name     string    `json:"name"`
	Ops      []crdt.Op `json:"ops"`
	Expected string    `json:"expected"`
}

type replica struct {
	doc *crdt.Doc
	log []crdt.Op
	has map[string]bool
}

func newReplica(name string) *replica {
	return &replica{doc: crdt.New(name), has: map[string]bool{}}
}

func key(op crdt.Op) string {
	return fmt.Sprintf("%d|%s:%d|%s:%d", op.Type, op.ID.Client, op.ID.Seq, op.Target.Client, op.Target.Seq)
}

func (r *replica) record(op crdt.Op) {
	k := key(op)
	if !r.has[k] {
		r.has[k] = true
		r.log = append(r.log, op)
	}
}

func applyAll(d *crdt.Doc, ops []crdt.Op) {
	pending := ops
	for len(pending) > 0 {
		var next []crdt.Op
		for _, op := range pending {
			if err := d.Apply(op); err != nil {
				next = append(next, op)
			}
		}
		if len(next) == len(pending) {
			panic("stuck: ops can never be applied")
		}
		pending = next
	}
}

func deliver(from, to *replica) {
	var fresh []crdt.Op
	for _, op := range from.log {
		if !to.has[key(op)] {
			fresh = append(fresh, op)
		}
	}
	applyAll(to.doc, fresh)
	for _, op := range fresh {
		to.record(op)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func scenario(seed int64, steps int) Vector {
	rng := rand.New(rand.NewSource(seed))
	reps := []*replica{newReplica("A"), newReplica("B"), newReplica("C")}

	for step := 0; step < steps; step++ {
		i := rng.Intn(len(reps))
		r := reps[i]
		var op crdt.Op
		var err error
		if r.doc.Len() > 0 && rng.Intn(4) == 0 {
			op, err = r.doc.LocalDelete(rng.Intn(r.doc.Len()))
		} else {
			op, err = r.doc.LocalInsert(rng.Intn(r.doc.Len()+1), alphabet[rng.Intn(len(alphabet))])
		}
		must(err)
		r.record(op)
		if rng.Intn(3) == 0 {
			j := rng.Intn(len(reps))
			if j != i {
				deliver(r, reps[j])
			}
		}
	}

	for round := 0; round < 2; round++ {
		for _, from := range reps {
			for _, to := range reps {
				if from != to {
					deliver(from, to)
				}
			}
		}
	}
	want := reps[0].doc.String()
	for _, r := range reps {
		if r.doc.String() != want {
			panic("replicas diverged")
		}
	}

	all := append([]crdt.Op(nil), reps[0].log...)
	rng.Shuffle(len(all), func(a, b int) { all[a], all[b] = all[b], all[a] })
	check := crdt.New("check")
	applyAll(check, all)
	if check.String() != want {
		panic("shuffled replay diverged")
	}
	return Vector{Name: fmt.Sprintf("random-seed-%d", seed), Ops: all, Expected: want}
}

func main() {
	var vectors []Vector
	for seed := int64(1); seed <= 5; seed++ {
		vectors = append(vectors, scenario(seed, 250))
	}
	out, err := json.Marshal(vectors)
	must(err)
	must(os.MkdirAll("../conformance", 0o755))
	must(os.WriteFile("../conformance/vectors.json", out, 0o644))
	fmt.Printf("wrote %d vectors\n", len(vectors))
}
