package crdt

import (
	"math/rand"
	"testing"
)

func TestMatchesSliceModel(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	d := New("A")
	var model []rune
	for i := 0; i < 5000; i++ {
		if len(model) > 0 && rng.Intn(3) == 0 {
			p := rng.Intn(len(model))
			if _, err := d.LocalDelete(p); err != nil {
				t.Fatal(err)
			}
			model = append(model[:p], model[p+1:]...)
		} else {
			p := rng.Intn(len(model) + 1)
			r := rune('a' + rng.Intn(26))
			if _, err := d.LocalInsert(p, r); err != nil {
				t.Fatal(err)
			}
			model = append(model, 0)
			copy(model[p+1:], model[p:])
			model[p] = r
		}
		if i%50 == 0 && d.String() != string(model) {
			t.Fatalf("step %d: doc and model differ", i)
		}
	}
	if d.String() != string(model) || d.Len() != len(model) {
		t.Fatal("final doc and model differ")
	}
}

func BenchmarkRandomInsert5k(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rng := rand.New(rand.NewSource(1))
		d := New("A")
		for j := 0; j < 5000; j++ {
			if _, err := d.LocalInsert(rng.Intn(d.Len()+1), 'x'); err != nil {
				b.Fatal(err)
			}
		}
	}
}
