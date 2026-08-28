package parse

import (
	"bytes"
	"errors"
	"sort"

	"mvdan.cc/sh/v3/syntax"
)

type anonymousInvocationCandidate struct {
	close int
	words []*syntax.Word
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
		if !ok {
			close, end, words, ok = fallbackAnonymousFunctionInvocationCandidate(src, name, seen)
		}
		if !ok || seen[close] {
			return nil, nil, firstErr
		}
		seen[close] = true
		candidates = append(candidates, anonymousInvocationCandidate{close: close, words: words})
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
		invocations, ok := bindAnonymousInvocations(tree, candidates)
		if !ok {
			return nil, nil, firstErr
		}
		return tree, invocations, nil
	}
}

func fallbackAnonymousFunctionInvocationCandidate(
	src []byte,
	name string,
	seen map[int]bool,
) (int, int, []*syntax.Word, bool) {
	for close, b := range src {
		if b != '}' || seen[close] {
			continue
		}
		wordStart := close + 1
		for wordStart < len(src) && (src[wordStart] == ' ' || src[wordStart] == '\t') {
			wordStart++
		}
		if wordStart == close+1 || wordStart >= len(src) || src[wordStart] == '\n' || src[wordStart] == ';' {
			continue
		}
		end, ok := anonymousInvocationEnd(src, wordStart)
		if !ok {
			continue
		}
		words, ok := parseAnonymousInvocationWords(src, name, close, end)
		if !ok || !prefixEndsWithAnonymousFunction(src, name, close) {
			continue
		}
		return close, end, words, true
	}
	return 0, 0, nil, false
}

func prefixEndsWithAnonymousFunction(src []byte, name string, close int) bool {
	prefix := bytes.Clone(src)
	for offset := close + 1; offset < len(prefix); offset++ {
		if prefix[offset] != '\n' {
			prefix[offset] = ' '
		}
	}
	tree, err := parseWithAdapters(prefix, name)
	if err != nil {
		return false
	}
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

	lineStart := bytes.LastIndexByte(src[:seed+1], '\n') + 1
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

func bindAnonymousInvocations(
	tree *syntax.File,
	candidates []anonymousInvocationCandidate,
) ([]AnonymousInvocation, bool) {
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
			return nil, false
		}
		invocations = append(invocations, AnonymousInvocation{
			Function: decl,
			Words:    candidate.words,
		})
	}
	return invocations, true
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
