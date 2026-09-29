package reduce

import (
	"fmt"
	"strings"
	"testing"
)

func TestTokensAndLinesConcatenateBack(t *testing.T) {
	sources := []string{
		"",
		"print a\n",
		"  if true; then print a | ; cat; fi\n\tx=${a[1]} &&\n",
		"a;;b;&c&|d&!e||f|&g\n",
		"no newline at end",
		"{a}(b)\n\n\n",
	}
	for _, src := range sources {
		if got := strings.Join(Tokens(src), ""); got != src {
			t.Errorf("Tokens(%q) joins to %q", src, got)
		}
		if got := strings.Join(Lines(src), ""); got != src {
			t.Errorf("Lines(%q) joins to %q", src, got)
		}
	}
}

func TestTokensSplitShellUnits(t *testing.T) {
	got := Tokens("  print a|& ;cat {x}\n$(f) && g ;;\n")
	want := []string{"  ", "print ", "a", "|& ", ";", "cat ", "{", "x", "}", "\n", "$", "(", "f", ") ", "&& ", "g ", ";;", "\n"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Tokens = %q\nwant     %q", got, want)
	}
}

// balanced reports whether the braces in s pair up, as a stand-in for "Zsh
// accepts it".
func balanced(s string) bool {
	depth := 0
	for _, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func TestReduceKeepsOnlyWhatThePredicateNeeds(t *testing.T) {
	src := "print one\nf() {\n  print two\n  if x {\n    GAP here\n  }\n}\nprint three\n"
	var sawUnbalanced bool
	got, stats, ok := Reduce(src, func(c Candidate) bool {
		if !balanced(c.Text) {
			sawUnbalanced = true
			return false
		}
		return strings.Contains(c.Text, "GAP")
	}, Options{})
	if !ok {
		t.Fatal("Reduce reported the original as uninteresting")
	}
	if got != "GAP\n" {
		t.Fatalf("Reduce = %q, want %q", got, "GAP\n")
	}
	if !sawUnbalanced {
		t.Fatal("the predicate never saw an unbalanced candidate, so the test does not exercise its refusal")
	}
	if stats.Capped || stats.Tests == 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

// Removing a block that spans lines is what the line pass cannot do, since
// dropping any one of its lines leaves it unbalanced.
func TestReduceRemovesAndUnwrapsMultiLineBlocks(t *testing.T) {
	src := "{\n  a\n  {\n    b\n  }\n}\nif c\n  GAP\nfi\n"
	got, _, ok := Reduce(src, func(c Candidate) bool {
		return balanced(c.Text) && strings.Count(c.Text, "if") == strings.Count(c.Text, "fi") && strings.Contains(c.Text, "GAP")
	}, Options{})
	if !ok || got != "GAP\n" {
		t.Fatalf("Reduce = %q, %v, want %q", got, ok, "GAP\n")
	}
}

func TestReduceKeepsTheFinalNewline(t *testing.T) {
	var sawMissing bool
	check := func(c Candidate) bool {
		if !strings.HasSuffix(c.Text, "\n") {
			sawMissing = true
		}
		return strings.Contains(c.Text, "x")
	}
	if got, _, _ := Reduce("a\nb x c\nd\n", check, Options{}); got != "x\n" {
		t.Fatalf("Reduce = %q, want %q", got, "x\n")
	}
	if sawMissing {
		t.Fatal("a candidate lost the final newline")
	}
	if got, _, _ := Reduce("a x", func(c Candidate) bool { return strings.Contains(c.Text, "x") }, Options{}); got != "x" {
		t.Fatalf("Reduce without a final newline = %q, want %q", got, "x")
	}
}

func TestReduceShortensWords(t *testing.T) {
	got, _, _ := Reduce("print   long-word GAP\n", func(c Candidate) bool {
		return strings.Contains(c.Text, "GAP") && strings.Count(c.Text, " ") >= 1
	}, Options{})
	if got != "a GAP\n" {
		t.Fatalf("Reduce = %q, want %q", got, "a GAP\n")
	}
}

// The origins let a predicate pin an error to one byte of the original, so
// that a second occurrence of the same construct cannot take its place.
func TestReduceTracksOrigins(t *testing.T) {
	src := "keep X\nother X\n"
	target := strings.Index(src, "X")
	got, _, ok := Reduce(src, func(c Candidate) bool {
		i := strings.Index(c.Text, "X")
		return i >= 0 && c.OriginAt(i, len(src)) == target
	}, Options{})
	if !ok || got != "X\n" {
		t.Fatalf("Reduce = %q, %v, want %q", got, ok, "X\n")
	}

	// Without the origin check the same predicate text keeps either X;
	// with it, a candidate holding only the second X is refused.
	second := strings.LastIndex(src, "X")
	var refused bool
	Reduce(src, func(c Candidate) bool {
		i := strings.Index(c.Text, "X")
		if i >= 0 && c.OriginAt(i, len(src)) == second {
			refused = true
			return false
		}
		return i >= 0
	}, Options{})
	if !refused {
		t.Fatal("no candidate held only the second X; the origin check is untested")
	}
}

func TestOriginAt(t *testing.T) {
	c := Candidate{Text: "ab", Origin: []int{3, 7}}
	for offset, want := range map[int]int{0: 3, 1: 7, 2: 10, 3: -1, -1: -1} {
		if got := c.OriginAt(offset, 10); got != want {
			t.Errorf("OriginAt(%d) = %d, want %d", offset, got, want)
		}
	}
}

func TestReduceShortenedWordMapsToItsStart(t *testing.T) {
	src := "  longword GAP\n"
	gap := strings.Index(src, "GAP")
	var last Candidate
	got, _, _ := Reduce(src, func(c Candidate) bool {
		i := strings.Index(c.Text, "GAP")
		if i <= 0 || strings.TrimSpace(c.Text[:i]) == "" || c.OriginAt(i, len(src)) != gap {
			return false
		}
		last = c
		return true
	}, Options{})
	if got != "a GAP\n" {
		t.Fatalf("Reduce = %q, want %q", got, "a GAP\n")
	}
	if last.Text != got || last.Origin[0] != strings.Index(src, "longword") {
		t.Fatalf("the shortened word's origin = %v, want it to start at %d", last.Origin, strings.Index(src, "longword"))
	}
}

func TestReduceRefusesAnUninterestingSource(t *testing.T) {
	got, stats, ok := Reduce("a\n", func(Candidate) bool { return false }, Options{})
	if ok || got != "a\n" || stats.Tests != 1 {
		t.Fatalf("Reduce = %q, %+v, %v; want the source back after one test", got, stats, ok)
	}
}

func TestReduceStopsAtTheBudget(t *testing.T) {
	src := strings.Repeat("filler line\n", 50) + "GAP\n"
	calls := 0
	_, stats, ok := Reduce(src, func(c Candidate) bool {
		calls++
		return strings.Contains(c.Text, "GAP")
	}, Options{MaxTests: 7})
	if !ok || !stats.Capped || stats.Tests != 7 || calls != 7 {
		t.Fatalf("stats = %+v, calls = %d, ok = %v; want 7 calls and Capped", stats, calls, ok)
	}
}

func TestReduceAsksOncePerCandidate(t *testing.T) {
	seen := map[string]bool{}
	Reduce("a b c\nd e\n{ GAP f }\n", func(c Candidate) bool {
		key := fmt.Sprint(c.Text, c.Origin)
		if seen[key] {
			t.Errorf("%q with origins %v judged twice", c.Text, c.Origin)
		}
		seen[key] = true
		return strings.Contains(c.Text, "GAP")
	}, Options{})
}
