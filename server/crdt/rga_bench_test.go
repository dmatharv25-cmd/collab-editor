package crdt

import "testing"

func benchTyping(b *testing.B, n int) {
	for i := 0; i < b.N; i++ {
		d := New("A")
		for j := 0; j < n; j++ {
			if _, err := d.LocalInsert(d.Len(), 'x'); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkAppend1k(b *testing.B)  { benchTyping(b, 1000) }
func BenchmarkAppend5k(b *testing.B)  { benchTyping(b, 5000) }
func BenchmarkAppend10k(b *testing.B) { benchTyping(b, 10000) }

func BenchmarkRemoteApply5k(b *testing.B) {
	src := New("A")
	ops := make([]Op, 0, 5000)
	for j := 0; j < 5000; j++ {
		op, _ := src.LocalInsert(src.Len(), 'x')
		ops = append(ops, op)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := New("B")
		for _, op := range ops {
			if err := d.Apply(op); err != nil {
				b.Fatal(err)
			}
		}
	}
}
