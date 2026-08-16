package parse

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const unclosedHeredocPrefix = "unclosed here-document"

// parseANSICHeredocDelimiter retries native Zsh heredocs whose delimiter uses
// ANSI-C quoting with escape sequences (e.g. cat <<$'E\x4fF'). mvdan/sh does not
// expand escape sequences in the delimiter before matching against heredoc body
// lines, resulting in a spurious unclosed heredoc error. The adapter decodes the
// ANSI-C delimiter, runs a same-width padded retry, and restores the original
// ANSI-C literal in the resulting AST.
func parseANSICHeredocDelimiter(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseANSICHeredocDelimiterWithParser(src, name, firstErr, parseANSICHeredocDelimiters)
}

func parseANSICHeredocDelimiters(src []byte, name string) (*syntax.File, error) {
	tree, err := parseTree(src, name)
	if err != nil {
		return parseANSICHeredocDelimiterWithParser(src, name, err, parseANSICHeredocDelimiters)
	}
	return tree, nil
}

func parseANSICHeredocDelimiterWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || !strings.HasPrefix(parseErr.Text, unclosedHeredocPrefix) {
		return nil, firstErr
	}

	seed := int(parseErr.Pos.Offset())
	if seed < 0 || seed >= len(src) {
		return nil, firstErr
	}

	redirPos := -1
	minOffset := seed - 2
	if minOffset < 0 {
		minOffset = 0
	}
	maxOffset := seed + 2
	if maxOffset > len(src)-2 {
		maxOffset = len(src) - 2
	}
	for i := minOffset; i <= maxOffset; i++ {
		if src[i] == '<' && src[i+1] == '<' {
			redirPos = i
			break
		}
	}
	if redirPos < 0 {
		return nil, firstErr
	}

	delimStart := redirPos + 2
	if delimStart < len(src) && src[delimStart] == '-' {
		delimStart++
	}
	for delimStart < len(src) && (src[delimStart] == ' ' || src[delimStart] == '\t') {
		delimStart++
	}

	if delimStart+1 >= len(src) || src[delimStart] != '$' || src[delimStart+1] != '\'' {
		return nil, firstErr
	}

	decoded, delimEnd, ok := parseANSICQuotedHeredocSegment(src, delimStart)
	if !ok || !bytes.Contains(src[delimStart:delimEnd], []byte("\\")) {
		return nil, firstErr
	}

	origLen := delimEnd - delimStart
	replacement := make([]byte, 0, origLen)
	replacement = append(replacement, '$', '\'')
	replacement = append(replacement, decoded...)
	replacement = append(replacement, '\'')
	if len(replacement) > origLen {
		return nil, firstErr
	}
	pad := origLen - len(replacement)
	if pad > 0 {
		replacement = append(replacement, bytes.Repeat([]byte(" "), pad)...)
	}

	masked := bytes.Clone(src)
	copy(masked[delimStart:delimEnd], replacement)

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}

	rawInsideQuotes := string(src[delimStart+2 : delimEnd-1])
	restored := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if restored {
			return false
		}
		redir, ok := node.(*syntax.Redirect)
		if !ok || redir.Word == nil || len(redir.Word.Parts) != 1 {
			return true
		}
		sgl, ok := redir.Word.Parts[0].(*syntax.SglQuoted)
		if !ok || !sgl.Dollar || int(sgl.Left.Offset()) != delimStart {
			return true
		}
		sgl.Value = rawInsideQuotes
		restored = true
		return false
	})

	if !restored {
		return nil, firstErr
	}
	return tree, nil
}
