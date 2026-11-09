package crdt

// ID uniquely identifies one character across all replicas.
type ID struct {
	Client string // which replica created it
	Seq    uint64 // Lamport clock value
}

// IsZero reports whether this is the "head of document" sentinel.
func (a ID) IsZero() bool {
	return a.Client == "" && a.Seq == 0
}

// Greater defines the total order used to break ties between
// concurrent inserts: higher Seq wins, then higher Client name.
func (a ID) Greater(b ID) bool {
	if a.Seq != b.Seq {
		return a.Seq > b.Seq
	}
	return a.Client > b.Client
}
