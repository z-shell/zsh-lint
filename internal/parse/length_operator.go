package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

// invalidCombinedLengthOperator is the exact error mvdan/sh (LangZsh,
// v3.14.1) reports for an expansion operator written under a length prefix,
// `${#name:#pattern}`. parser.go rejects any operator once pe.Length is set,
// because in POSIX and bash `${#name}` admits nothing after the name. Zsh
// applies `#` to the RESULT of the rest of the expansion (zshexpn, Parameter
// Expansion), so the two compose and every such row is valid Zsh.
//
// The same text is reported for a second subscript under a length prefix,
// `${#name[a][b]}`, which parseSecondSubscript handles; the two adapters tell
// the constructs apart by the byte at the reported position, `[` there and an
// operator byte here.
//
// Gating on this text is verdict-redundant with the positional scan below:
// measured across 189 tree fixtures and 83 probe rows, accepting any parse
// error here changed no verdict, because lengthPrefixBefore refuses anything
// that is not `${#name` before the reported offset. It is kept because it
// bounds the cost, one reparse per owned error rather than per parse error,
// and because it records which construct this adapter owns.
const invalidCombinedLengthOperator = "cannot combine multiple parameter expansion operators"

// parseLengthOperator retries only a native-Zsh expansion operator under a
// length prefix (zshexpn, Parameter Expansion):
//
//	${#a[@]:#x}   the count of the elements that do not match x
//	${#a//x/y}    the length of a after the replacement
//	${#a:-y}      the length of a, or of y when a is unset
//
// The retry masks the length `#` with `_`, which makes the prefix part of the
// parameter name and leaves the operator to parse in its own field. `_` is
// chosen because it is a name byte in every position, so the masked source
// stays one byte per original byte and every later position is unchanged.
//
// The name is then restored: the leading `_` is dropped from the parameter
// literal, its position is advanced past the byte it no longer holds, and
// Length is set so the typed tree says what the source says. A row whose mask
// does not produce exactly that shape returns the original parser error
// rather than an unverified tree.
func parseLengthOperator(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseLengthOperatorWithParser(src, name, firstErr, parseWithAdapters)
}

func parseLengthOperatorWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidCombinedLengthOperator {
		return nil, firstErr
	}

	hash, ok := lengthPrefixBefore(src, int(parseErr.Pos.Offset()))
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[hash] = '_'

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if !restoreLengthPrefix(tree, hash) {
		return nil, firstErr
	}
	return tree, nil
}

// lengthPrefixBefore reports the offset of the `#` that opens the expansion
// whose operator stands at operator, when that expansion is `${#name...` with
// a plain name.
//
// The scan walks back over the name rather than trusting a fixed distance,
// because the operator may follow a subscript (`${#a[@]:#x}`) or the name
// directly (`${#a:#x}`). It stops at the first byte that cannot be part of a
// name or a subscript, so only `${` immediately followed by `#` and a name
// byte is accepted.
//
// A name is required. `${#*:#x}` and `${#@:#x}` are valid Zsh, but `_*` and
// `_@` are not names, so the mask would not parse and the retry would fail
// anyway; refusing here keeps the parser's own error for them instead of a
// second, less specific one.
func lengthPrefixBefore(src []byte, operator int) (int, bool) {
	if operator <= 0 || operator >= len(src) {
		return 0, false
	}
	// A `[` here is a second subscript under a length prefix,
	// `${#name[a][b]}`, which reports this same error and belongs to
	// parseSecondSubscript. Declining keeps one construct in one adapter.
	if src[operator] == '[' {
		return 0, false
	}

	i := operator - 1
	if src[i] == ']' {
		end, ok := subscriptStartBefore(src, i)
		if !ok {
			return 0, false
		}
		i = end - 1
	}

	nameEnd := i
	for i >= 0 && isIdentByte(src[i]) {
		i--
	}
	if i == nameEnd {
		// No name between the prefix and the operator.
		return 0, false
	}
	// `${#` must stand immediately before the name.
	if i < 2 || src[i] != '#' || src[i-1] != '{' || src[i-2] != '$' {
		return 0, false
	}
	return i, true
}

// subscriptStartBefore returns the offset of the `[` that opens the subscript
// closed by the `]` at close, counting nested brackets. A newline inside the
// span refuses, matching how the other subscript scanners bound themselves.
func subscriptStartBefore(src []byte, close int) (int, bool) {
	depth := 0
	for i := close; i >= 0; i-- {
		switch src[i] {
		case ']':
			depth++
		case '[':
			depth--
			if depth == 0 {
				return i, true
			}
		case '\n':
			return 0, false
		}
	}
	return 0, false
}

// restoreLengthPrefix finds the expansion the mask produced, whose parameter
// name begins with the `_` written over the length `#`, drops that byte from
// the name, and records the length the source asked for.
//
// It reports false when no expansion matches, so the caller returns the
// original parser error instead of a tree that does not correspond to the
// source.
func restoreLengthPrefix(tree *syntax.File, hash int) bool {
	restored := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if restored {
			return false
		}
		exp, ok := node.(*syntax.ParamExp)
		if !ok || exp.Length || exp.Param == nil {
			return true
		}
		lit := exp.Param
		if int(lit.ValuePos.Offset()) != hash || len(lit.Value) < 2 || lit.Value[0] != '_' {
			return true
		}
		lit.Value = lit.Value[1:]
		lit.ValuePos = syntax.NewPos(lit.ValuePos.Offset()+1, lit.ValuePos.Line(), lit.ValuePos.Col()+1)
		exp.Length = true
		restored = true
		return false
	})
	return restored
}
