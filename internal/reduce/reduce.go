// Package reduce shrinks a source while a predicate holds, by delta
// debugging over lines and then over shell tokens (#562).
//
// It never interprets the source: the predicate decides what a candidate
// means. Character-level reduction is deliberately absent, because most
// candidates it tries are no longer valid Zsh, which a parser-gap predicate
// rejects outright.
//
// Every candidate carries the offset in the original source of each of its
// bytes, so a predicate can require that an error stays on the same original
// byte rather than moving to another construct with the same message.
package reduce

import (
	"encoding/binary"
	"hash/fnv"
	"strings"
)

// Options bounds a reduction.
type Options struct {
	// MaxTests caps the predicate calls; a repeated candidate is answered
	// from a cache and not counted. Zero means DefaultMaxTests.
	MaxTests int
}

// DefaultMaxTests is the predicate-call cap when Options.MaxTests is zero.
const DefaultMaxTests = 5000

// Stats reports what a reduction cost.
type Stats struct {
	// Tests is the number of predicate calls, the original's included.
	Tests int
	// Capped is set when the reduction stopped at MaxTests, so the result
	// may still shrink further.
	Capped bool
}

// Candidate is a source the predicate judges.
type Candidate struct {
	// Text is the source.
	Text string
	// Origin holds, for each byte of Text, its offset in the original
	// source. A word Reduce shortened maps every byte to the word's start.
	Origin []int
}

// OriginAt returns the original offset of the byte at offset in c, and
// the original source's length for the offset just past the end, where a
// parser reports an error at the end of input. It returns -1 for an offset
// outside Text.
func (c Candidate) OriginAt(offset, originalLength int) int {
	switch {
	case offset >= 0 && offset < len(c.Origin):
		return c.Origin[offset]
	case offset == len(c.Text):
		return originalLength
	default:
		return -1
	}
}

// piece is a run of candidate bytes with their origins.
type piece struct {
	text   string
	origin []int
}

func join(pieces []piece) piece {
	var text strings.Builder
	var origin []int
	for _, p := range pieces {
		text.WriteString(p.text)
		origin = append(origin, p.origin...)
	}
	return piece{text: text.String(), origin: origin}
}

func (p piece) slice(start, end int) piece {
	return piece{text: p.text[start:end], origin: p.origin[start:end]}
}

// Reduce returns the smallest source it finds for which interesting holds,
// starting from src, for which it must hold too; ok is false when it does
// not. A final newline is kept on every candidate, because Zsh judges a file
// without one differently.
//
// Each round removes lines (ddmin), removes or unwraps each balanced block,
// removes tokens (ddmin), removes pairs of tokens such as an opener and its
// closer, keeps the shortest run of tokens that still holds, shortens each
// word to `a`, and drops blanks at line ends. Rounds repeat until one
// changes nothing or the test budget runs out.
func Reduce(src string, interesting func(Candidate) bool, opts Options) (reduced string, stats Stats, ok bool) {
	limit := opts.MaxTests
	if limit <= 0 {
		limit = DefaultMaxTests
	}
	var suffix piece
	if strings.HasSuffix(src, "\n") {
		suffix = piece{text: "\n", origin: []int{len(src) - 1}}
	}
	seen := map[[2]uint64]bool{}
	test := func(pieces []piece) bool {
		candidate := join(append(append([]piece{}, pieces...), suffix))
		key := candidateKey(candidate)
		if verdict, done := seen[key]; done {
			return verdict
		}
		if stats.Tests >= limit {
			stats.Capped = true
			return false
		}
		stats.Tests++
		verdict := interesting(Candidate{Text: candidate.text, Origin: candidate.origin})
		seen[key] = verdict
		return verdict
	}
	body := src[:len(src)-len(suffix.text)]
	origin := make([]int, len(body))
	for i := range origin {
		origin[i] = i
	}
	current := piece{text: body, origin: origin}
	if !test([]piece{current}) {
		return src, stats, false
	}
	for {
		before := current.text
		current = join(ddmin(lines(current), test))
		current = join(blocks(tokens(current), test))
		current = join(ddmin(tokens(current), test))
		current = join(pairs(tokens(current), test))
		current = join(window(tokens(current), test))
		current = join(shorten(tokens(current), test))
		current = join(trimBlanks(tokens(current), test))
		if current.text == before || stats.Capped {
			return current.text + suffix.text, stats, true
		}
	}
}

// trimBlanks drops the blanks at the end of each line when test holds
// without them.
func trimBlanks(units []piece, test func([]piece) bool) []piece {
	trimmed := make([]piece, len(units))
	changed := false
	for i, unit := range units {
		trimmed[i] = unit
		if i+1 < len(units) && units[i+1].text != "\n" {
			continue
		}
		if text := strings.TrimRight(unit.text, " 	"); text != unit.text {
			trimmed[i] = unit.slice(0, len(text))
			changed = true
		}
	}
	if changed && test(trimmed) {
		return trimmed
	}
	return units
}

// closers maps each opener token to its closer.
var closers = map[string]string{
	"{": "}", "(": ")", "if": "fi", "case": "esac", "do": "done",
}

// blocks removes each balanced block, from an opener to its closer (`{` to
// `}`, `if` to `fi`, `do` to `done`), while test holds, and otherwise tries
// unwrapping it: removing the opener and the closer and keeping what they
// enclose. A line pass cannot do either when a block spans lines, since
// removing one of its lines leaves invalid Zsh, and pairs is too costly on
// a whole file. Closers are matched by counting tokens, not by parsing, so
// a mismatched pair only costs a failed test.
func blocks(units []piece, test func([]piece) bool) []piece {
	for i := 0; i < len(units); i++ {
		opener := strings.TrimRight(units[i].text, " 	")
		closer, ok := closers[opener]
		if !ok {
			continue
		}
		j, depth := i+1, 1
		for ; j < len(units); j++ {
			switch strings.TrimRight(units[j].text, " 	") {
			case opener:
				depth++
			case closer:
				depth--
			}
			if depth == 0 {
				break
			}
		}
		if j == len(units) {
			continue
		}
		removed := append(append([]piece{}, units[:i]...), units[j+1:]...)
		if test(removed) {
			units = removed
			i--
			continue
		}
		unwrapped := append(append(append([]piece{}, units[:i]...), units[i+1:j]...), units[j+1:]...)
		if test(unwrapped) {
			units = unwrapped
			i--
		}
	}
	return units
}

// candidateKey identifies a candidate by a hash of its text and of its
// origins, so two candidates with the same text whose bytes came from
// different places are judged apart.
func candidateKey(c piece) [2]uint64 {
	text := fnv.New64a()
	_, _ = text.Write([]byte(c.text))
	origin := fnv.New64a()
	var buf [8]byte
	for _, o := range c.origin {
		binary.LittleEndian.PutUint64(buf[:], uint64(o))
		_, _ = origin.Write(buf[:])
	}
	return [2]uint64{text.Sum64(), origin.Sum64()}
}

// ddmin removes units while test holds for the rest, in the
// complement-only form of Zeller's ddmin: it tries removing each of n
// chunks, keeps the first removal that holds, and otherwise doubles n until
// the chunks are single units. The result is 1-minimal unless test stops
// answering true, as it does once the budget is spent.
func ddmin(units []piece, test func([]piece) bool) []piece {
	n := 2
	for len(units) >= 2 {
		size := (len(units) + n - 1) / n
		removed := false
		for start := 0; start < len(units); start += size {
			end := min(start+size, len(units))
			candidate := append(append([]piece{}, units[:start]...), units[end:]...)
			if test(candidate) {
				units = candidate
				n = max(n-1, 2)
				removed = true
				break
			}
		}
		if removed {
			continue
		}
		if n >= len(units) {
			break
		}
		n = min(n*2, len(units))
	}
	if len(units) == 1 && test(nil) {
		return nil
	}
	return units
}

// maxQuadraticUnits bounds the input of the passes that try every pair of
// units; each tries up to n*(n-1)/2 candidates per removal.
const maxQuadraticUnits = 40

// pairs removes two units at a time while test holds, for an opener and its
// closer (`{` and `}`, `if` and `fi`) that ddmin cannot remove one at a
// time, since neither half alone leaves valid Zsh. It is skipped above
// maxQuadraticUnits units.
func pairs(units []piece, test func([]piece) bool) []piece {
	if len(units) > maxQuadraticUnits {
		return units
	}
	for removed := true; removed; {
		removed = false
		for i := 0; i < len(units) && !removed; i++ {
			for j := i + 1; j < len(units) && !removed; j++ {
				candidate := append(append(append([]piece{}, units[:i]...), units[i+1:j]...), units[j+1:]...)
				if test(candidate) {
					units = candidate
					removed = true
				}
			}
		}
	}
	return units
}

// window returns the shortest contiguous run of units for which test holds,
// or units when none does, which removes a construct wrapped around the part
// that matters, such as `case x in x) ... esac`. It is skipped above
// maxQuadraticUnits units.
func window(units []piece, test func([]piece) bool) []piece {
	n := len(units)
	if n > maxQuadraticUnits {
		return units
	}
	for size := 1; size < n; size++ {
		for start := 0; start+size <= n; start++ {
			if test(units[start : start+size]) {
				return units[start : start+size]
			}
		}
	}
	return units
}

// shorten replaces each word longer than one byte with `a`, keeping its
// trailing blanks, while test holds.
func shorten(units []piece, test func([]piece) bool) []piece {
	units = append([]piece{}, units...)
	for i, unit := range units {
		word := strings.TrimRight(unit.text, " \t")
		if len(word) < 2 || isBreak(word[0]) {
			continue
		}
		origin := append([]int{unit.origin[0]}, unit.origin[len(word):]...)
		units[i] = piece{text: "a" + unit.text[len(word):], origin: origin}
		if !test(units) {
			units[i] = unit
		}
	}
	return units
}

// Lines splits src into lines, each keeping its newline.
func Lines(src string) []string {
	return texts(lines(piece{text: src, origin: make([]int, len(src))}))
}

// Tokens splits src into shell-token units that concatenate back to src:
// each newline alone, each control operator (`;`, `&&`, `|&`, `;;` ...),
// each `(`, `)`, `{` and `}`, and each run of other non-blank bytes. Blanks
// after a unit belong to it, so removing a unit removes its trailing blanks
// too; leading blanks on a line are a unit of their own. Quotes are not
// tracked, since a predicate that asks Zsh rejects a candidate with an
// unbalanced quote.
func Tokens(src string) []string {
	return texts(tokens(piece{text: src, origin: make([]int, len(src))}))
}

func texts(pieces []piece) []string {
	out := make([]string, len(pieces))
	for i, p := range pieces {
		out[i] = p.text
	}
	return out
}

func lines(p piece) []piece {
	var out []piece
	for start := 0; start < len(p.text); {
		end := strings.IndexByte(p.text[start:], '\n')
		if end < 0 {
			end = len(p.text)
		} else {
			end += start + 1
		}
		out = append(out, p.slice(start, end))
		start = end
	}
	return out
}

// operators are the shell's control operators, longest first.
var operators = []string{";;", ";&", ";|", "&&", "||", "|&", "&|", "&!", ";", "&", "|"}

func tokens(p piece) []piece {
	src := p.text
	var out []piece
	i := 0
	for i < len(src) {
		start := i
		switch c := src[i]; {
		case c == '\n':
			i++
		case c == ' ' || c == '\t':
			// Leading blanks; trailing blanks are taken below.
		case c == '(' || c == ')' || c == '{' || c == '}':
			i++
		case isOperator(c):
			i += len(operatorAt(src[i:]))
		default:
			for i < len(src) && !isBreak(src[i]) {
				i++
			}
		}
		if src[start] != '\n' {
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
		}
		out = append(out, p.slice(start, i))
	}
	return out
}

func operatorAt(s string) string {
	for _, op := range operators {
		if strings.HasPrefix(s, op) {
			return op
		}
	}
	return s[:1]
}

func isOperator(c byte) bool {
	return c == ';' || c == '&' || c == '|'
}

func isBreak(c byte) bool {
	return c == '\n' || c == ' ' || c == '\t' || c == '(' || c == ')' || c == '{' || c == '}' || isOperator(c)
}
