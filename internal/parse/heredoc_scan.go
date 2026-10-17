package parse

import (
	"bytes"
	"strings"
)

// sourceHeredocSkips shares the active-source scanner's substitution ownership
// with flat adapter scanners. Only real bodies are skipped at a newline;
// documents abandoned at a substitution close never consume later source.
func sourceHeredocSkips(src []byte) (map[int]int, bool) {
	if !bytes.Contains(src, []byte("<<")) {
		return nil, true
	}
	scan, ok := scanConditionalPatterns(src, -1, nil)
	return scan.heredocSkips, ok && scan.finding == nil
}

// heredocAt reads the redirection at src[i] when it opens a here-document:
// `<<` or `<<-` followed by the delimiter word. It returns the pending body
// and the offset just past the delimiter. isHeredoc is false for anything
// else; for the here-string `<<<`, whose word is ordinary command text, end
// is just past the operator so a caller steps over all three `<` and never
// reads its last two as a here-document.
// ok is false only when a here-document operator's delimiter cannot be read,
// in which case a scanner cannot tell where the body ends and must give up.
//
// The site scanners share it so that each one skips a here-document body the
// same way (#429): a quote character in the body is text, not quoting.
// backtickDepth is the owning legacy substitution's depth, or zero outside
// one; an attached closing backtick ends the delimiter as well as the command.
func heredocAt(src []byte, i, backtickDepth int) (heredoc pendingHeredoc, end int, isHeredoc, ok bool) {
	if i+1 >= len(src) || src[i] != '<' || src[i+1] != '<' {
		return pendingHeredoc{}, i, false, true
	}
	if i+2 < len(src) && src[i+2] == '<' {
		return pendingHeredoc{}, i + 3, false, true
	}
	stripTabs := i+2 < len(src) && src[i+2] == '-'
	delimiterStart := i + 2
	if stripTabs {
		delimiterStart++
	}
	delimiter, end, ok := parseHeredocDelimiter(src, delimiterStart, backtickDepth)
	if !ok {
		return pendingHeredoc{}, i, true, false
	}
	if stripTabs {
		delimiter = strings.TrimLeft(delimiter, "\t")
	}
	return pendingHeredoc{delimiter: delimiter, stripTabs: stripTabs}, end, true, true
}

// atUnescapedLineStart reports whether src[i] begins a new line: the byte
// before it is a newline that a backslash does not continue.
func atUnescapedLineStart(src []byte, i int) bool {
	if i == 0 || src[i-1] != '\n' {
		return false
	}
	backslashes := 0
	for j := i - 2; j >= 0 && src[j] == '\\'; j-- {
		backslashes++
	}
	return backslashes%2 == 0
}
