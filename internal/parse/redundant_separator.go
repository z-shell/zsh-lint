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
		case b == '\\' && index+1 < len(src) && src[index+1] == '\n':
			// A line continuation is removed before the line is read, so
			// it neither ends nor starts a word: `print a; \` + newline +
			// `;` is `print a; ;` (#467).
			index += 2
			continue
		case b == '\\' && index+1 < len(src):
			index += 2
			commandStart = false
			continue
		case b == '\'':
			index = skipRedundantSeparatorQuote(src, index, '\'')
			commandStart = false
			continue
		case b == '"':
			sites = append(sites, redundantSeparatorSitesInDoubleQuote(src, index)...)
			index = skipRedundantSeparatorDoubleQuote(src, index)
			commandStart = false
			continue
		case b == '`':
			// A backquoted command is a list of its own, so its body starts
			// in command position (#569).
			if end, ok := skipBackquoted(src, index); ok {
				sites = append(sites, nestedRedundantSeparatorSites(src, index+1, end)...)
				index = end + 1
				commandStart = false
				continue
			}
			index++
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
			if isRedirectionOperatorByte(src, index) {
				// `>&`, `<&`, `>|` and `>&|` are redirection operators whose
				// word still follows, so a `;` after one is a parse error,
				// not an empty sublist (#569 review).
				index++
				commandStart = false
				continue
			}
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

// isRedirectionOperatorByte reports whether the `&` or `|` at index ends a
// redirection operator: `>&`, `<&`, `>|`, or the `|` of `>&|`. The same rule
// keeps skipCommandSubstitution from reading those bytes as list operators.
func isRedirectionOperatorByte(src []byte, index int) bool {
	if index == 0 || (src[index] != '&' && src[index] != '|') {
		return false
	}
	prev := src[index-1]
	if prev == '<' || prev == '>' {
		return true
	}
	return src[index] == '|' && prev == '&' && index >= 2 && src[index-2] == '>'
}

// nestedRedundantSeparatorSites scans the command list src[from:to], the body
// of a command substitution or backquoted command, and returns its sites as
// offsets into src. A body holding `$'` or `<<` reports none: the scanner
// does not read `$'...'` escapes or here-document bodies as Zsh does, so a
// site there could be a byte inside a string, and the retry would change it.
func nestedRedundantSeparatorSites(src []byte, from, to int) []int {
	body := src[from:to]
	if bytes.Contains(body, []byte("$'")) || bytes.Contains(body, []byte("<<")) {
		return nil
	}
	sites := scanRedundantSeparatorSites(body)
	for i := range sites {
		sites[i] += from
	}
	return sites
}

// redundantSeparatorSitesInDoubleQuote returns the sites inside the command
// substitutions and backquoted commands of the double-quoted string opened at
// open (#569): `print "$( ; print a )"` runs the list `; print a`, whose `;`
// ends an empty sublist as it does unquoted. Arithmetic and parameter
// expansions are stepped over, not entered. When the string's extent is not
// certain (an unclosed string, backquote or substitution, or one
// skipCommandSubstitution refuses), no site is reported and the string keeps
// the verdict it had before.
func redundantSeparatorSitesInDoubleQuote(src []byte, open int) []int {
	var sites []int
	for i := open + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '"':
			return sites
		case '`':
			end, ok := skipBackquoted(src, i)
			if !ok {
				return nil
			}
			sites = append(sites, nestedRedundantSeparatorSites(src, i+1, end)...)
			i = end
		case '$':
			if i+2 < len(src) && src[i+1] == '(' && src[i+2] != '(' {
				end, ok := skipCommandSubstitution(src, i+1)
				if !ok {
					return nil
				}
				sites = append(sites, nestedRedundantSeparatorSites(src, i+2, end)...)
				i = end
				continue
			}
			end, ok := skipDollarExpansion(src, i, true)
			if !ok {
				return nil
			}
			i = end
		}
	}
	return nil
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
	if end, ok := skipDoubleQuotedString(src, index); ok {
		return end + 1
	}
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
