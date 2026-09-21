package parse

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// redundantSeparatorError is the parser's text for a `;` that opens a
// sublist. Zsh reads a list as zero or more sublists, so a `;` in command
// position terminates an empty one and is a no-op.
const redundantSeparatorError = "`;` can only immediately follow a statement"

// parseRedundantSeparator adapts a redundant `;` standing in command
// position (issue #332): `while true; ;`, `print x; ;`, a file that opens
// with `;`, and `; ; ;`.
//
// Zsh reads a list as zero or more sublists, so a `;` with no statement
// before it terminates an empty sublist and does nothing. Confirmed by
// running it rather than by reading the grammar: `print a; ;` prints `a`,
// and `while (( i++ < 2 )); ; do print loop; done` iterates twice. The
// parser (mvdan/sh through v3.14.1) requires a statement before every `;`
// and rejects the byte where it stands.
//
// The `do` (#238) and `then`/`else` (#297) adapters handle the same byte
// after a reserved word, where the keyword anchors the site and the retry
// can be verified against the construct it belongs to. A redundant `;` in
// plain command position has no such anchor, so this adapter blanks every
// site in one pass and lets the re-parse decide: an empty sublist owns no
// node, so a tree that parses is the tree the source describes.
//
// One pass rather than one site per re-entry. Masking a single occurrence
// and recursing costs a full re-parse per separator, which is quadratic in
// a file that uses the form more than once.
func parseRedundantSeparator(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseRedundantSeparatorWithParser(src, name, firstErr, parseWithAdapters)
}

func parseRedundantSeparatorWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != redundantSeparatorError {
		return nil, firstErr
	}

	sites := scanRedundantSeparatorSites(src)
	if len(sites) == 0 {
		return nil, firstErr
	}

	// No check that the reported offset is one of the sites. The parser
	// stops at the first `;` it cannot place, and every such `;` is a site
	// by construction: a `;` it can place is an ordinary terminator, and a
	// `;;` it cannot place is reported under a different text (either
	// "`;;` can only be used in a case clause" or a case-pattern error).
	// A membership check here would be unreachable, so it is left out
	// rather than shipped as a guard no test can pin.
	masked := bytes.Clone(src)
	for _, offset := range sites {
		masked[offset] = ' '
	}

	tree, err := parse(masked, name)
	if err != nil {
		return nil, firstErr
	}
	return tree, nil
}

// scanRedundantSeparatorSites returns the offset of every `;` that stands in
// command position, meaning the previous syntactically active byte is a list
// separator, a list opener, or nothing at all.
//
// A `;` that follows a statement is the ordinary terminator and is left
// alone. A `;;`, `;&` or `;|` is a case terminator, which Zsh rejects here
// too, so those are left to the parser. Bytes inside quotes, comments and
// escapes are skipped, so a `;` in `print 'a;'` is never a site.
func scanRedundantSeparatorSites(src []byte) []int {
	var sites []int

	// commandStart tracks whether the next active byte opens a sublist.
	// It begins true because a file may open with a redundant `;`.
	commandStart := true

	index := 0
	for index < len(src) {
		b := src[index]

		switch {
		case b == '\\' && index+1 < len(src):
			index += 2
			commandStart = false
			continue
		case b == '\'':
			index = skipRedundantSeparatorQuote(src, index, '\'')
			commandStart = false
			continue
		case b == '"':
			index = skipRedundantSeparatorDoubleQuote(src, index)
			commandStart = false
			continue
		case b == '#' && commandStart:
			for index < len(src) && src[index] != '\n' {
				index++
			}
			continue
		case b == ' ' || b == '\t' || b == '\r':
			index++
			continue
		case b == '\n' || b == '&' || b == '|' || b == '(' || b == '{':
			// Each of these opens a new sublist. `&&`, `||` and `;&` are
			// two bytes, but the second is handled on its own turn and
			// leaves commandStart true either way.
			index++
			commandStart = true
			continue
		case b == ';':
			if commandStart && !isCaseTerminator(src, index) {
				sites = append(sites, index)
				index++
				// A run such as `; ; ;` is several empty sublists, so the
				// next `;` is a site too.
				continue
			}
			// Step over a case terminator whole. Reading `:;;` one byte at a
			// time would leave the second `;` looking like an empty sublist
			// and mask it, silently deleting the terminator.
			if isCaseTerminator(src, index) {
				index += 2
				commandStart = true
				continue
			}
			index++
			commandStart = true
			continue
		default:
			index++
			commandStart = false
		}
	}

	return sites
}

// isCaseTerminator reports whether the `;` at index begins `;;`, `;&` or
// `;|`, which Zsh reads as a case terminator rather than a separator.
func isCaseTerminator(src []byte, index int) bool {
	return index+1 < len(src) && strings.IndexByte(";&|", src[index+1]) >= 0
}

// skipRedundantSeparatorQuote returns the offset just past the quote opened
// at index with mark, or the end of src when it is never closed.
func skipRedundantSeparatorQuote(src []byte, index int, mark byte) int {
	for cursor := index + 1; cursor < len(src); cursor++ {
		if src[cursor] == mark {
			return cursor + 1
		}
	}
	return len(src)
}

// skipRedundantSeparatorDoubleQuote returns the offset just past the double
// quote opened at index, honouring backslash escapes.
func skipRedundantSeparatorDoubleQuote(src []byte, index int) int {
	for cursor := index + 1; cursor < len(src); cursor++ {
		switch src[cursor] {
		case '\\':
			cursor++
		case '"':
			return cursor + 1
		}
	}
	return len(src)
}
