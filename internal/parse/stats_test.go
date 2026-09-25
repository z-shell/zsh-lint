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
			// The parser fork reads the brace-form if itself (#446).
			name:  "a brace-form if parses without a retry",
			src:   "if [[ -n $x ]] { print a }\n",
			stats: Stats{TreeParses: 1, MaxAdapterDepth: 0},
		},
		{
			name:  "a second site nests a second retry",
			src:   "print ${x::=value}\nprint ${y::=value}\n",
			stats: Stats{TreeParses: 3, MaxAdapterDepth: 2},
		},
		{
			// The parser fork reads the brace-form if, the alternate for and
			// repeat (#446, #459, #281), so only the `::=` site retries.
			name:  "one adapter after native loop forms",
			src:   "if [[ -n $x ]] { print a }\nfor x (a b) print $x\nrepeat 3 do print a; done\nprint ${x::=1}\n",
			stats: Stats{TreeParses: 2, MaxAdapterDepth: 1},
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
	if _, err := Parse(strings.NewReader("print ${x::=value}\n"), "stats.zsh"); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ResetStats()
	if got := ReadStats(); got != (Stats{}) {
		t.Fatalf("ReadStats() after ResetStats = %+v, want zero", got)
	}
}
