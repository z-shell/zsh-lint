package parse

import (
	"bytes"

	"mvdan.cc/sh/v3/syntax"
)

// carriageReturnError is Zsh's own wording for the token it rejects, so the
// diagnostic names the byte that is actually wrong rather than the construct
// that appears to be unterminated.
const carriageReturnError = "parse error near `\\r`"

// carriageReturnProbeByte stands in for `\r` while the source is re-parsed.
// Zsh lexes `\r` as an ordinary word character, so any byte with no syntactic
// meaning reproduces its verdict; the substitution is one byte wide, which
// keeps every offset in the probe identical to the original.
const carriageReturnProbeByte = 'r'

// rejectCarriageReturns returns a parse error at the first `\r` that Zsh's
// lexer would reject (issue #333).
//
// Zsh treats `\r` as an ordinary word character, not as whitespace, so a file
// with CRLF line endings is usually a parse error: `}`, `done`, `fi`, `esac`
// and `)` cannot be followed by a word, and `done\r` or `} \r` is exactly
// that. mvdan/sh (v3.14.1) skips `\r` as whitespace and accepts the file, so
// without this guard the linter reports success for a script Zsh refuses to
// run, which is the worst failure mode a linter has.
//
// The verdict comes from re-parsing with `\r` replaced by an ordinary word
// byte, because the rule is not "a file containing `\r` is invalid": `x=1\r`,
// `print x\r` and a `\r` inside quotes are all valid Zsh, since a word may
// appear there. Deciding it with the parser rather than a hand-written
// position rule keeps the two in agreement as the front end grows.
//
// The probe's own error is discarded. It names whatever construct the extra
// word broke, typically the enclosing `while` or `if`, which would send a
// reader to the wrong line; the position reported here is the `\r` itself.
func rejectCarriageReturns(src []byte, name string) error {
	index := bytes.IndexByte(src, '\r')
	if index < 0 {
		return nil
	}

	probe := bytes.ReplaceAll(src, []byte{'\r'}, []byte{carriageReturnProbeByte})
	if _, err := parseWithAdapters(probe, name); err == nil {
		return nil
	}

	return syntax.ParseError{
		Filename: name,
		Pos:      carriageReturnPos(src, index),
		Text:     carriageReturnError,
	}
}

// carriageReturnPos builds the position of the byte at index, counting lines
// and columns the way the parser does: lines are 1-based and separated by
// `\n`, and a column is a 1-based byte offset within its line.
func carriageReturnPos(src []byte, index int) syntax.Pos {
	line := 1 + bytes.Count(src[:index], []byte{'\n'})
	lineStart := bytes.LastIndexByte(src[:index], '\n') + 1
	col := index - lineStart + 1
	return syntax.NewPos(uint(index), uint(line), uint(col))
}
