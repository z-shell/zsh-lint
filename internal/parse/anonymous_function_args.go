package parse

import (
	"bytes"
	"errors"
	"regexp"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type anonymousInvocationCandidate struct {
	close int
	words []*syntax.Word
	// seedErr is the error whose position seeded this candidate's discovery,
	// captured before it was masked. When this specific candidate turns out
	// not to be a genuine anonymous function, seedErr is the true, unmasked
	// error the source actually has here, distinct from firstErr (an
	// earlier, already-resolved candidate's error) or currentErr (a later
	// iteration's, possibly unrelated to this candidate).
	seedErr error
}

// parseAnonymousFunctionArgs closes the native-Zsh gap where an anonymous
// function declaration is immediately followed by invocation words. The full
// retry masks only those words with same-width spaces. Each masked word is
// parsed separately at its original offset and retained as typed metadata.
func parseAnonymousFunctionArgs(
	src []byte,
	name string,
	firstErr error,
) (*syntax.File, []AnonymousInvocation, error) {
	masked := bytes.Clone(src)
	currentErr := firstErr
	seen := make(map[int]bool)
	var candidates []anonymousInvocationCandidate

	for {
		close, end, words, ok := anonymousFunctionInvocationCandidate(src, name, currentErr)
		if !ok || seen[close] {
			// A seed candidate can land on an ordinary brace group followed
			// by a stray word (for example `{ :; } argument`), which is not
			// an anonymous function and must not have been masked: masking
			// it would hide the genuine syntax error it names. Rather than
			// pay the adaptive-parse validation cost inline as each
			// candidate is found (expensive: it re-enters the full adapter
			// chain on truncated buffers), validate every accepted
			// candidate once here, only on this rarer error-return path,
			// before trusting currentErr. On any failure, report that
			// specific candidate's own seedErr, the genuine, unmasked error
			// it hid, rather than firstErr: an earlier candidate in the
			// same file can already be genuine and resolved, in which case
			// firstErr's position no longer names a real problem.
			for _, candidate := range candidates {
				if !prefixEndsWithAnonymousFunction(masked, name, candidate.close) {
					return nil, nil, candidate.seedErr
				}
			}
			// currentErr, not firstErr: once every candidate has been
			// confirmed genuine and the whole-file retry still fails, the
			// retry's error names a real, separate gap at its own
			// (unrebased, width-preserving mask) position. Reverting to
			// firstErr here would re-report a construct that is already
			// resolved.
			return nil, nil, currentErr
		}
		seen[close] = true
		candidates = append(candidates, anonymousInvocationCandidate{close: close, words: words, seedErr: currentErr})
		for offset := close + 1; offset < end; offset++ {
			if masked[offset] != '\n' {
				masked[offset] = ' '
			}
		}

		tree, err := parseWithAdapters(masked, name)
		if err != nil {
			currentErr = err
			continue
		}
		invocations, ok, failed := bindAnonymousInvocations(tree, candidates)
		if !ok {
			return nil, nil, failed.seedErr
		}
		return tree, invocations, nil
	}
}

// closingTokenRe matches the trailing backtick-quoted token that every
// mvdan/sh "incomplete construct" error ends with: the still-needed closer
// (`matchingErr`'s right token, `quoteErr`'s quote character, `stmtEnd`'s end
// keyword, or an ordinary `followErr` single required token). A `followErr`
// built from a free-form description (for example "must be followed by `in`,
// `do`, `;`, or a newline") does not end in a backtick pair, so it correctly
// fails to match and is treated as non-extendable below. The one exception is
// the closer being a literal backtick itself (an unclosed legacy backtick
// command substitution): Go's "%#q" verb, which mvdan/sh formats every token
// with, falls back to a double-quoted Go string for any token whose text
// contains a backquote, so that case is handled separately below instead of
// by this regexp.
var closingTokenRe = regexp.MustCompile("`([^`]*)`$")

// closingBacktickSuffix is how mvdan/sh's "%#q" verb renders a literal
// backtick closer: Go's %#q falls back to a double-quoted string, not a
// backtick-delimited one, for any token text that itself contains a
// backquote.
const closingBacktickSuffix = `"` + "`" + `"`

// closingToken extracts the token that would resolve an "incomplete
// construct" parse error, if err has that shape.
func closingToken(err error) (string, bool) {
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) {
		return "", false
	}
	if strings.HasSuffix(parseErr.Text, closingBacktickSuffix) {
		return "`", true
	}
	m := closingTokenRe.FindStringSubmatch(parseErr.Text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// prefixEndsWithAnonymousFunction reports whether the `}` at close truly
// closes an anonymous *syntax.FuncDecl. It cannot validate by blanking
// everything after close and reparsing the same-width buffer: a candidate
// nested inside a still-open enclosing function (for example `f() { () {
// x; } y }`) would have that enclosing function's own closing brace blanked
// away too, so the prefix would never parse and every nested candidate would
// be rejected. It also cannot validate with a hand-rolled brace-depth scanner:
// zsh comments and typographic quoting (for example a doc comment like
// “ `autoload', `bindkey' “ with an odd backtick count) can desync a naive
// scanner's quote state for the rest of the file. Instead it drives the real
// parser: parse src[:close+1], and on each "incomplete construct" error,
// append the closer the error itself names and retry, until the prefix
// parses (then check for a FuncDecl ending at close) or the parser reports an
// error that is not an incompleteness (then close does not validly close an
// anonymous function).
//
// This is expensive: each retry re-enters the full adapter chain on a
// truncated, rewritten buffer, and some adapters themselves recurse into
// parseWithAdapters. The success path already gets an equivalent, single,
// whole-file check for free from bindAnonymousInvocations, so this is
// reserved for the rarer error-return path, where each already-accepted
// candidate is confirmed once before its mask is trusted.
//
// maxClosers bounds the retry: closingToken matches any error ending in a
// backtick-quoted (or literal-backtick) token, not just genuine
// incompleteness ("not a valid parameter expansion operator: `+`" matches
// too), and adapter rewrites on a truncated prefix can in principle ping-pong
// between requested closers instead of converging. A real anonymous function
// is never nested anywhere near this deep, so exhausting the bound only ever
// means a false rejection, reported as the true, unmasked firstErr by the
// caller, never a false acceptance.
func prefixEndsWithAnonymousFunction(src []byte, name string, close int) bool {
	if close < 0 || close >= len(src) || src[close] != '}' {
		return false
	}
	prefix := append([]byte(nil), src[:close+1]...)
	const maxClosers = 32
	for appended := 0; ; appended++ {
		tree, err := parseWithAdapters(prefix, name)
		if err == nil {
			return funcDeclEndsAt(tree, close)
		}
		if appended >= maxClosers {
			return false
		}
		token, ok := closingToken(err)
		if !ok {
			return false
		}
		prefix = append(prefix, '\n')
		prefix = append(prefix, token...)
		prefix = append(prefix, '\n')
	}
}

func funcDeclEndsAt(tree *syntax.File, close int) bool {
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		decl, ok := node.(*syntax.FuncDecl)
		if ok && decl.Name == nil && len(decl.Names) == 0 && int(decl.End().Offset())-1 == close {
			found = true
			return false
		}
		return true
	})
	return found
}

// logicalLineStart returns the offset of the start of the logical line
// containing pos, walking back across `\`-newline continuations so a `}`
// separated from the parse error's line only by a line continuation is still
// found. It does not track quote state: a continuation-shaped backslash run
// inside a string only widens the search window, and the candidate it might
// turn up still has to parse and bind as a real anonymous FuncDecl before
// anything is masked, so over-including here cannot mask a false positive.
func logicalLineStart(src []byte, pos int) int {
	lineStart := bytes.LastIndexByte(src[:pos], '\n') + 1
	for lineStart > 0 {
		backslashes := 0
		for i := lineStart - 2; i >= 0 && src[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			break
		}
		lineStart = bytes.LastIndexByte(src[:lineStart-1], '\n') + 1
	}
	return lineStart
}

func anonymousFunctionInvocationCandidate(
	src []byte,
	name string,
	parseFailure error,
) (int, int, []*syntax.Word, bool) {
	var parseErr syntax.ParseError
	if !errors.As(parseFailure, &parseErr) || parseErr.Text != tryAlwaysParseError {
		return 0, 0, nil, false
	}
	seed := int(parseErr.Pos.Offset())
	if seed >= len(src) {
		seed = len(src) - 1
	}
	if seed < 0 {
		return 0, 0, nil, false
	}

	lineStart := logicalLineStart(src, seed+1)
	close := seed
	for close >= lineStart && (src[close] == ' ' || src[close] == '\t') {
		close--
	}
	if close < lineStart || src[close] != '}' {
		close = bytes.LastIndexByte(src[lineStart:seed+1], '}')
		if close < 0 {
			return 0, 0, nil, false
		}
		close += lineStart
	}

	wordStart := close + 1
	for wordStart < len(src) && (src[wordStart] == ' ' || src[wordStart] == '\t') {
		wordStart++
	}
	if wordStart >= len(src) || src[wordStart] == '\n' || src[wordStart] == ';' {
		return 0, 0, nil, false
	}
	end, ok := anonymousInvocationEnd(src, wordStart)
	if !ok {
		return 0, 0, nil, false
	}
	words, ok := parseAnonymousInvocationWords(src, name, close, end)
	if !ok {
		return 0, 0, nil, false
	}
	return close, end, words, true
}

func anonymousInvocationEnd(src []byte, start int) (int, bool) {
	var quote byte
	escaped := false
	parenDepth := 0
	braceDepth := 0
	bracketDepth := 0

	for offset := start; offset < len(src); offset++ {
		b := src[offset]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			switch {
			case b == '\\' && quote != '\'':
				escaped = true
			case b == quote:
				quote = 0
			}
			continue
		}

		switch b {
		case '\\':
			escaped = true
		case '\'', '"', '`':
			quote = b
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '\n', ';':
			if parenDepth == 0 && braceDepth == 0 && bracketDepth == 0 {
				return offset, true
			}
		}
	}
	return len(src), quote == 0 && !escaped && parenDepth == 0 && braceDepth == 0 && bracketDepth == 0
}

func parseAnonymousInvocationWords(src []byte, name string, close, end int) ([]*syntax.Word, bool) {
	island := make([]byte, len(src))
	for offset, b := range src {
		if b == '\n' {
			island[offset] = '\n'
		} else {
			island[offset] = ' '
		}
	}
	island[close] = ':'
	copy(island[close+1:end], src[close+1:end])
	tree, err := parseWithAdapters(island, name)
	if err != nil {
		return nil, false
	}
	for _, stmt := range tree.Stmts {
		call, ok := stmt.Cmd.(*syntax.CallExpr)
		if !ok || len(call.Args) < 2 || getParseWordLiteral(call.Args[0]) != ":" {
			continue
		}
		return append([]*syntax.Word(nil), call.Args[1:]...), true
	}
	return nil, false
}

// bindAnonymousInvocations pairs each candidate with the FuncDecl it names.
// On failure it also returns the specific candidate that did not bind, so
// the caller can report that candidate's own seedErr instead of an
// unrelated one.
func bindAnonymousInvocations(
	tree *syntax.File,
	candidates []anonymousInvocationCandidate,
) ([]AnonymousInvocation, bool, anonymousInvocationCandidate) {
	functionsByClose := make(map[int]*syntax.FuncDecl)
	syntax.Walk(tree, func(node syntax.Node) bool {
		decl, ok := node.(*syntax.FuncDecl)
		if ok && decl.Name == nil && len(decl.Names) == 0 {
			functionsByClose[int(decl.End().Offset())-1] = decl
		}
		return true
	})

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].close < candidates[j].close })
	invocations := make([]AnonymousInvocation, 0, len(candidates))
	for _, candidate := range candidates {
		decl := functionsByClose[candidate.close]
		if decl == nil {
			return nil, false, candidate
		}
		invocations = append(invocations, AnonymousInvocation{
			Function: decl,
			Words:    candidate.words,
		})
	}
	return invocations, true, anonymousInvocationCandidate{}
}

func getParseWordLiteral(word *syntax.Word) string {
	if word == nil {
		return ""
	}
	var value bytes.Buffer
	for _, part := range word.Parts {
		if lit, ok := part.(*syntax.Lit); ok {
			value.WriteString(lit.Value)
		}
	}
	return value.String()
}
