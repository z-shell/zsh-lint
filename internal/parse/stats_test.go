package parse

import (
	"strings"
	"testing"
)

// The counts are pinned on purpose: they are the retry cost of the adapter
// chain for each shape (#366). A change here means an adapter now parses more
// or less often, which the pull request must explain.
func TestStatsCountTreeParsesAndAdapterDepth(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		stats Stats
	}{
		{
			name:  "plain source parses once",
			src:   "print ok\n",
			stats: Stats{TreeParses: 1, MaxAdapterDepth: 0},
		},
		{
			name:  "one alternate form needs one adapter retry",
			src:   "if [[ -n $x ]] { print a }\n",
			stats: Stats{TreeParses: 2, MaxAdapterDepth: 1},
		},
		{
			name:  "a second repeat loop nests a second retry",
			src:   "repeat 3 do print a; done\nrepeat 2 do print b; done\n",
			stats: Stats{TreeParses: 5, MaxAdapterDepth: 2},
		},
		{
			name:  "three constructs nest three adapters",
			src:   "if [[ -n $x ]] { print a }\nfor x (a b) print $x\nrepeat 3 do print a; done\n",
			stats: Stats{TreeParses: 8, MaxAdapterDepth: 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetStats()
			if _, err := Parse(strings.NewReader(tt.src), "stats.zsh"); err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := ReadStats(); got != tt.stats {
				t.Fatalf("ReadStats() = %+v, want %+v", got, tt.stats)
			}
		})
	}
}

func TestResetStatsZeroesCounters(t *testing.T) {
	if _, err := Parse(strings.NewReader("repeat 3 do print a; done\n"), "stats.zsh"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ResetStats()
	if got := ReadStats(); got != (Stats{}) {
		t.Fatalf("ReadStats() after ResetStats = %+v, want zero", got)
	}
}
