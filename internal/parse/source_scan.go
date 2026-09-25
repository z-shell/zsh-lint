package parse

import (
	"errors"
	"fmt"
	"reflect"
	"sort"

	"mvdan.cc/sh/v3/syntax"
)

// Source scanning helpers the compatibility adapters share. They read raw
// source bytes to find a construct's sites before the parser sees them.

func matchSourceWord(src []byte, i int, word string) bool {
	if i+len(word) > len(src) {
		return false
	}
	if string(src[i:i+len(word)]) != word {
		return false
	}
	if i+len(word) < len(src) {
		next := src[i+len(word)]
		if next != ' ' && next != '\t' && next != '\n' && next != ';' && next != '&' && next != '|' && next != '(' && next != '{' {
			return false
		}
	}
	return true
}

// skipInlineSpaces skips spaces, tabs, and line continuations, stopping at a
// newline, comment, or any other byte.
func skipInlineSpaces(src []byte, i int) int {
	for i < len(src) {
		switch {
		case src[i] == ' ' || src[i] == '\t':
			i++
		case src[i] == '\\' && i+1 < len(src) && src[i+1] == '\n':
			i += 2
		default:
			return i
		}
	}
	return i
}

func skipSpacesAndComments(src []byte, i int) int {
	for i < len(src) {
		b := src[i]
		if b == ' ' || b == '\t' || b == '\n' {
			i++
			continue
		}
		if b == '#' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		break
	}
	return i
}

// forSourceMap maps each byte of a source an adapter rewrote with synthetic
// text back to the original offset it stands for. The `for` adapter that
// introduced it moved into the parser fork (#459); the repeat adapter still
// maps its synthetic `do` and `done` through it.
type forSourceMap struct {
	origByTransformed []int
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func rebaseForPositions(value reflect.Value, sm forSourceMap, lineStarts []int) error {
	if !value.IsValid() {
		return nil
	}
	if value.Type() == syntaxPosType {
		if !value.CanSet() {
			return nil
		}
		position := value.Interface().(syntax.Pos)
		if !position.IsValid() {
			return nil
		}
		transformedOffset := int(position.Offset())
		if transformedOffset < 0 || transformedOffset >= len(sm.origByTransformed) {
			return fmt.Errorf("transformed position %d is outside source map", transformedOffset)
		}
		origOffset := sm.origByTransformed[transformedOffset]
		if origOffset < 0 {
			origOffset = 0
		}
		lineIndex := sort.Search(len(lineStarts), func(index int) bool {
			return lineStarts[index] > origOffset
		}) - 1
		if lineIndex < 0 {
			lineIndex = 0
		}
		line := lineIndex + 1
		col := origOffset - lineStarts[lineIndex] + 1
		value.Set(reflect.ValueOf(syntax.NewPos(uint(origOffset), uint(line), uint(col))))
		return nil
	}

	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			return nil
		}
		return rebaseForPositions(value.Elem(), sm, lineStarts)
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if field.CanSet() {
				if err := rebaseForPositions(field, sm, lineStarts); err != nil {
					return err
				}
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if err := rebaseForPositions(value.Index(i), sm, lineStarts); err != nil {
				return err
			}
		}
	}
	return nil
}

// rebaseForError rewrites the position of a parser error raised on the
// transformed source so it points into the original file. The synthetic text
// otherwise shifts every later line number, and the retry error is the useful
// one: it names the gap that remains after the rewritten construct was
// accepted. An error the mapping cannot place is returned unchanged rather
// than dropped.
func rebaseForError(err error, sm forSourceMap, lineStarts []int) error {
	var parseErr syntax.ParseError
	if errors.As(err, &parseErr) && parseErr.Pos.IsValid() {
		if rebased, mapErr := rebaseForPos(parseErr.Pos, sm, lineStarts); mapErr == nil {
			parseErr.Pos = rebased
			return parseErr
		}
		return err
	}
	var langErr syntax.LangError
	if errors.As(err, &langErr) && langErr.Pos.IsValid() {
		if rebased, mapErr := rebaseForPos(langErr.Pos, sm, lineStarts); mapErr == nil {
			langErr.Pos = rebased
			return langErr
		}
	}
	return err
}

// rebaseForPos maps one transformed position back to the original source.
func rebaseForPos(position syntax.Pos, sm forSourceMap, lineStarts []int) (syntax.Pos, error) {
	transformedOffset := int(position.Offset())
	if transformedOffset < 0 || transformedOffset >= len(sm.origByTransformed) {
		return syntax.Pos{}, fmt.Errorf("transformed position %d is outside source map", transformedOffset)
	}
	origOffset := sm.origByTransformed[transformedOffset]
	if origOffset < 0 {
		origOffset = 0
	}
	lineIndex := sort.Search(len(lineStarts), func(index int) bool {
		return lineStarts[index] > origOffset
	}) - 1
	if lineIndex < 0 {
		lineIndex = 0
	}
	line := lineIndex + 1
	col := origOffset - lineStarts[lineIndex] + 1
	return syntax.NewPos(uint(origOffset), uint(line), uint(col)), nil
}

// ifShortFormAnchor returns the offset of the first byte after the sublist
// statement that belongs to something else: the next statement of the list
// that holds it, or the token that closes that list. The parent's positions
// may themselves be the source-mapped result of another adapter, which is
// fine: the anchor only bounds the backward scan for the sublist's last
// byte.
func ifShortFormAnchor(src []byte, parents map[syntax.Node]syntax.Node, stmt *syntax.Stmt) int {
	parent := parents[stmt]
	if next := followingStmt(parent, stmt); next != nil {
		return int(next.Pos().Offset())
	}
	inList := func(list []*syntax.Stmt) bool {
		for _, candidate := range list {
			if candidate == stmt {
				return true
			}
		}
		return false
	}
	switch parent := parent.(type) {
	case *syntax.File:
		return len(src)
	case *syntax.Block:
		return int(parent.Rbrace.Offset())
	case *syntax.Subshell:
		return int(parent.Rparen.Offset())
	case *syntax.CmdSubst:
		return int(parent.Right.Offset())
	case *syntax.ProcSubst:
		return int(parent.Rparen.Offset())
	case *syntax.CaseItem:
		if parent.OpPos.IsValid() {
			return int(parent.OpPos.Offset())
		}
		if clause, ok := parents[parent].(*syntax.CaseClause); ok {
			return int(clause.Esac.Offset())
		}
	case *syntax.IfClause:
		if inList(parent.Cond) {
			return int(parent.ThenPos.Offset())
		}
		if parent.Else != nil {
			return int(parent.Else.Position.Offset())
		}
		return int(parent.FiPos.Offset())
	case *syntax.WhileClause:
		if inList(parent.Cond) {
			return int(parent.DoPos.Offset())
		}
		return int(parent.DonePos.Offset())
	case *syntax.ForClause:
		return int(parent.DonePos.Offset())
	}
	return -1
}

// ifShortFormCloser returns the offset after the last byte of the sublist
// statement starting at start and bounded by anchor: blanks, newlines,
// separators and whole comments before the anchor are not part of it, but a
// comment on the same line as the last byte is kept, so it stays attached
// to the statement it trails.
func ifShortFormCloser(src []byte, start, anchor int, comments []*syntax.Comment) int {
	end := anchor
scan:
	for end > start {
		switch src[end-1] {
		case ' ', '\t', '\n', '\r', ';', '&':
			end--
			continue
		}
		for _, comment := range comments {
			if at := int(comment.Pos().Offset()); at < end && end <= int(comment.End().Offset()) && at >= start {
				end = at
				continue scan
			}
		}
		break
	}
	trailing := end
	for trailing < anchor && (src[trailing] == ' ' || src[trailing] == '\t') {
		trailing++
	}
	if trailing < anchor && src[trailing] == '#' {
		for _, comment := range comments {
			if int(comment.Pos().Offset()) == trailing {
				return int(comment.End().Offset())
			}
		}
	}
	return end
}
