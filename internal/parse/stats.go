package parse

import "sync/atomic"

// Stats reports how much work the front end has done since the last
// ResetStats. It exists to measure the adapter chain's retry cost (#357,
// #366, #408) without a scratch build. The counters are process-wide, so a
// caller that wants per-file numbers parses one file at a time and resets in
// between; concurrent parses make MaxAdapterDepth meaningless.
type Stats struct {
	// TreeParses counts whole-source parses through the base parser,
	// including every masked retry an adapter makes. Fragment parses (word
	// sequences, probes) are not counted.
	TreeParses int64
	// MaxAdapterDepth is the deepest nesting of adapter retries: 0 when no
	// adapter retried the source, 1 when an adapter's retry parsed without a
	// further adapter, and so on. It is the length of the adapter index stack
	// #366 recorded.
	MaxAdapterDepth int64
}

var (
	treeParses      atomic.Int64
	adapterNesting  atomic.Int64
	maxAdapterDepth atomic.Int64
)

// ReadStats returns the counters accumulated since the last ResetStats.
func ReadStats() Stats {
	return Stats{
		TreeParses:      treeParses.Load(),
		MaxAdapterDepth: maxAdapterDepth.Load(),
	}
}

// ResetStats zeroes the counters.
func ResetStats() {
	treeParses.Store(0)
	maxAdapterDepth.Store(0)
}

// enterAdapterChain records one more level of parseWithAdaptersExcept nesting
// and returns the function that leaves it. The outermost level is the plain
// parse, so the adapter depth is the nesting minus one.
func enterAdapterChain() (leave func()) {
	depth := adapterNesting.Add(1) - 1
	for {
		deepest := maxAdapterDepth.Load()
		if depth <= deepest || maxAdapterDepth.CompareAndSwap(deepest, depth) {
			break
		}
	}
	return func() { adapterNesting.Add(-1) }
}
