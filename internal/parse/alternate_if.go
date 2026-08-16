package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"mvdan.cc/sh/v3/syntax"
)

const statementSeparatorRequired = "statements must be separated by &, ; or a newline"

type alternateIfEditKind int

const (
	editIfThen alternateIfEditKind = iota
	editElifThen
	editElse
	editChain
	editFi
	editWhileDo
	editDone
)

type alternateIfEdit struct {
	offset int
	kind   alternateIfEditKind
}

type alternateIfSourceMap struct {
	origByTransformed []int
	synthetic         map[int]struct{}
}

func parseAlternateIfBrace(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseAlternateIfBraceWithParser(src, name, firstErr, parseAfterAlternateIf)
}

// parseAfterAlternateIf lets independently gated, same-boundary adapters
// compose when one real file contains more than one unsupported Zsh form.
// Each adapter still activates only from its own concrete parser error.
func parseAfterAlternateIf(src []byte, name string) (*syntax.File, error) {
	tree, err := parseTree(src, name)
	if err != nil {
		tree, err = parseAssociativeSubscriptWithParser(src, name, err, parseAfterAlternateIf)
		if err != nil {
			tree, err = parseMultiNameForWithParser(src, name, err, parseAfterAlternateIf)
		}
	}
	return tree, err
}

func parseAlternateIfBraceWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != statementSeparatorRequired {
		return nil, firstErr
	}

	seedOffset := int(parseErr.Pos.Offset())
	edits, ok := scanAlternateIfEdits(src, seedOffset)
	if !ok {
		return nil, firstErr
	}

	transformed, sourceMap := applyAlternateIfEdits(src, edits)
	tree, err := parse(transformed, name)
	if err != nil {
		return nil, err
	}

	lineStarts := originalLineStarts(src)
	if err := rebaseAlternateIfPositions(reflect.ValueOf(tree), sourceMap, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing alternate if positions: %w", name, err)
	}

	return tree, nil
}

func scanAlternateIfEdits(src []byte, seedOffset int) ([]alternateIfEdit, bool) {
	var edits []alternateIfEdit

	inSingleQuote := false
	inDoubleQuote := false
	inANSICQuote := false
	escaped := false
	inComment := false

	atWordStart := true
	atCommandStart := true

	type blockKind int
	const (
		kindNormalBlock blockKind = iota
		kindIfThen
		kindElifThen
		kindElse
		kindWhileDo
	)
	type blockFrame struct {
		kind       blockKind
		openOffset int
	}
	var blockStack []blockFrame

	type ifState int
	const (
		ifNone ifState = iota
		ifSawIf
		ifSawElif
		ifSawElse
		ifSawWhile
	)
	currentIf := ifNone

	i := 0
	seedRecognized := false

	for i < len(src) {
		b := src[i]

		if escaped {
			escaped = false
			atWordStart = false
			atCommandStart = false
			i++
			continue
		}

		if inSingleQuote {
			if b == '\'' {
				inSingleQuote = false
			}
			i++
			continue
		}
		if inANSICQuote {
			switch b {
			case '\\':
				escaped = true
			case '\'':
				inANSICQuote = false
			}
			i++
			continue
		}
		if inDoubleQuote {
			switch b {
			case '\\':
				escaped = true
			case '"':
				inDoubleQuote = false
			}
			i++
			continue
		}
		if inComment {
			if b == '\n' {
				inComment = false
				atWordStart = true
				atCommandStart = true
			}
			i++
			continue
		}

		if b == '\\' {
			escaped = true
			atWordStart = false
			atCommandStart = false
			i++
			continue
		}

		if b == '#' && atWordStart {
			inComment = true
			i++
			continue
		}

		if b == '\'' {
			inSingleQuote = true
			atWordStart = false
			atCommandStart = false
			i++
			continue
		}
		if b == '"' {
			inDoubleQuote = true
			atWordStart = false
			atCommandStart = false
			i++
			continue
		}
		if b == '$' && i+1 < len(src) && src[i+1] == '\'' {
			inANSICQuote = true
			i += 2
			atWordStart = false
			atCommandStart = false
			continue
		}

		if atWordStart && atCommandStart {
			if matchSourceWord(src, i, "if") {
				currentIf = ifSawIf
				i += 2
				atWordStart = false
				atCommandStart = false
				continue
			}
			if matchSourceWord(src, i, "elif") {
				currentIf = ifSawElif
				i += 4
				atWordStart = false
				atCommandStart = false
				continue
			}
			if matchSourceWord(src, i, "else") {
				currentIf = ifSawElse
				i += 4
				atWordStart = false
				atCommandStart = false
				continue
			}
			if matchSourceWord(src, i, "while") {
				currentIf = ifSawWhile
				i += 5
				atWordStart = false
				atCommandStart = false
				continue
			}
		}

		if currentIf == ifSawIf || currentIf == ifSawElif || currentIf == ifSawWhile {
			if b == '[' && i+1 < len(src) && src[i+1] == '[' {
				end := scanClosingDoubleBracket(src, i)
				if end > i {
					i = end
					braceOffset := scanAlternateConditionBrace(src, i)
					if braceOffset < len(src) && src[braceOffset] == '{' {
						if braceOffset == seedOffset {
							seedRecognized = true
						}
						kind := editIfThen
						if currentIf == ifSawElif {
							kind = editElifThen
						} else if currentIf == ifSawWhile {
							kind = editWhileDo
						}
						edits = append(edits, alternateIfEdit{offset: braceOffset, kind: kind})
						bk := kindIfThen
						if currentIf == ifSawElif {
							bk = kindElifThen
						} else if currentIf == ifSawWhile {
							bk = kindWhileDo
						}
						blockStack = append(blockStack, blockFrame{kind: bk, openOffset: braceOffset})
						currentIf = ifNone
						i = braceOffset + 1
						atWordStart = true
						atCommandStart = true
						continue
					}
				}
			}
			if b == '(' && i+1 < len(src) && src[i+1] == '(' {
				end := scanClosingDoubleParen(src, i)
				if end > i {
					i = end
					braceOffset := scanAlternateConditionBrace(src, i)
					if braceOffset < len(src) && src[braceOffset] == '{' {
						if braceOffset == seedOffset {
							seedRecognized = true
						}
						kind := editIfThen
						if currentIf == ifSawElif {
							kind = editElifThen
						} else if currentIf == ifSawWhile {
							kind = editWhileDo
						}
						edits = append(edits, alternateIfEdit{offset: braceOffset, kind: kind})
						bk := kindIfThen
						if currentIf == ifSawElif {
							bk = kindElifThen
						} else if currentIf == ifSawWhile {
							bk = kindWhileDo
						}
						blockStack = append(blockStack, blockFrame{kind: bk, openOffset: braceOffset})
						currentIf = ifNone
						i = braceOffset + 1
						atWordStart = true
						atCommandStart = true
						continue
					}
				}
			}
			if b == '{' {
				end := scanClosingBrace(src, i)
				if end > i {
					braceOffset := skipSpacesAndComments(src, end)
					if braceOffset < len(src) && src[braceOffset] == '{' {
						if braceOffset == seedOffset {
							seedRecognized = true
						}
						kind := editIfThen
						if currentIf == ifSawElif {
							kind = editElifThen
						}
						edits = append(edits, alternateIfEdit{offset: braceOffset, kind: kind})
						bk := kindIfThen
						if currentIf == ifSawElif {
							bk = kindElifThen
						}
						blockStack = append(blockStack, blockFrame{kind: bk, openOffset: braceOffset})
						currentIf = ifNone
						i = braceOffset + 1
						atWordStart = true
						atCommandStart = true
						continue
					}
				}
			}
		}

		if currentIf == ifSawElse {
			braceOffset := skipSpacesAndComments(src, i)
			if braceOffset < len(src) && src[braceOffset] == '{' {
				if braceOffset == seedOffset {
					seedRecognized = true
				}
				edits = append(edits, alternateIfEdit{offset: braceOffset, kind: editElse})
				blockStack = append(blockStack, blockFrame{kind: kindElse, openOffset: braceOffset})
				currentIf = ifNone
				i = braceOffset + 1
				atWordStart = true
				atCommandStart = true
				continue
			}
		}

		if b == '{' {
			blockStack = append(blockStack, blockFrame{kind: kindNormalBlock, openOffset: i})
			i++
			atWordStart = true
			atCommandStart = true
			continue
		}

		if b == '}' {
			if len(blockStack) > 0 {
				top := blockStack[len(blockStack)-1]
				blockStack = blockStack[:len(blockStack)-1]
				nextWordOffset := skipSpacesAndComments(src, i+1)
				if top.kind == kindWhileDo {
					edits = append(edits, alternateIfEdit{offset: i, kind: editDone})
					i++
					atWordStart = true
					atCommandStart = true
					continue
				}
				if top.kind == kindIfThen || top.kind == kindElifThen || top.kind == kindElse {
					if nextWordOffset < len(src) && (matchSourceWord(src, nextWordOffset, "elif") || matchSourceWord(src, nextWordOffset, "else")) {
						edits = append(edits, alternateIfEdit{offset: i, kind: editChain})
					} else {
						edits = append(edits, alternateIfEdit{offset: i, kind: editFi})
					}
					i++
					atWordStart = true
					atCommandStart = true
					continue
				}
				if nextWordOffset < len(src) && (matchSourceWord(src, nextWordOffset, "elif") || matchSourceWord(src, nextWordOffset, "else")) {
					edits = append(edits, alternateIfEdit{offset: i, kind: editChain})
					i++
					atWordStart = true
					atCommandStart = true
					continue
				}
			}
			i++
			atWordStart = true
			atCommandStart = true
			continue
		}

		switch b {
		case ' ', '\t', '\n':
			atWordStart = true
			if b == '\n' {
				atCommandStart = true
				currentIf = ifNone
			}
		case ';', '&', '|':
			atWordStart = true
			atCommandStart = true
			currentIf = ifNone
		default:
			atWordStart = false
			atCommandStart = false
		}
		i++
	}

	if !seedRecognized || len(edits) == 0 {
		return nil, false
	}
	return edits, true
}

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

func scanAlternateConditionBrace(src []byte, i int) int {
	for {
		i = skipAlternateConditionSpaces(src, i)
		if i < len(src) && src[i] == '{' {
			return i
		}
		if i+1 >= len(src) || !((src[i] == '&' && src[i+1] == '&') || (src[i] == '|' && src[i+1] == '|')) {
			return i
		}
		i = skipAlternateConditionSpaces(src, i+2)
		for i < len(src) && src[i] == '!' {
			i = skipAlternateConditionSpaces(src, i+1)
		}
		switch {
		case i+1 < len(src) && src[i] == '[' && src[i+1] == '[':
			i = scanClosingDoubleBracket(src, i)
		case i+1 < len(src) && src[i] == '(' && src[i+1] == '(':
			i = scanClosingDoubleParen(src, i)
		default:
			return i
		}
		if i < 0 {
			return len(src)
		}
	}
}

func skipAlternateConditionSpaces(src []byte, i int) int {
	for {
		next := skipSpacesAndComments(src, i)
		if next < len(src) && src[next] == '\\' && next+1 < len(src) && src[next+1] == '\n' {
			i = next + 2
			continue
		}
		return next
	}
}

func scanClosingDoubleBracket(src []byte, start int) int {
	i := start + 2
	for i+1 < len(src) {
		if src[i] == ']' && src[i+1] == ']' {
			return i + 2
		}
		i++
	}
	return -1
}

func scanClosingDoubleParen(src []byte, start int) int {
	i := start + 2
	depth := 1
	for i < len(src) {
		if src[i] == '(' && i+1 < len(src) && src[i+1] == '(' {
			depth++
			i += 2
			continue
		}
		if src[i] == ')' && i+1 < len(src) && src[i+1] == ')' {
			depth--
			if depth == 0 {
				return i + 2
			}
			i += 2
			continue
		}
		i++
	}
	return -1
}

func scanClosingBrace(src []byte, start int) int {
	i := start + 1
	depth := 1
	for i < len(src) {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return -1
}

func applyAlternateIfEdits(src []byte, edits []alternateIfEdit) ([]byte, alternateIfSourceMap) {
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].offset < edits[j].offset
	})
	editsByOffset := make(map[int]alternateIfEdit, len(edits))
	for _, e := range edits {
		editsByOffset[e.offset] = e
	}

	var transformed bytes.Buffer
	sm := alternateIfSourceMap{
		origByTransformed: make([]int, 0, len(src)+len(edits)*10),
		synthetic:         make(map[int]struct{}),
	}

	appendSynthetic := func(s string, origOffset int) {
		for _, b := range []byte(s) {
			offset := transformed.Len()
			transformed.WriteByte(b)
			sm.origByTransformed = append(sm.origByTransformed, origOffset)
			sm.synthetic[offset] = struct{}{}
		}
	}

	appendOriginal := func(b byte, origOffset int) {
		transformed.WriteByte(b)
		sm.origByTransformed = append(sm.origByTransformed, origOffset)
	}

	for i := 0; i < len(src); i++ {
		if edit, ok := editsByOffset[i]; ok {
			switch edit.kind {
			case editIfThen, editElifThen:
				appendSynthetic("; then\n", i)
			case editWhileDo:
				appendSynthetic("; do\n", i)
			case editElse:
				appendSynthetic("\n", i)
			case editChain:
				appendSynthetic("\n", i)
			case editFi:
				appendSynthetic("\nfi\n", i)
			case editDone:
				appendSynthetic("\ndone\n", i)
			}
			continue
		}
		appendOriginal(src[i], i)
	}
	sm.origByTransformed = append(sm.origByTransformed, len(src))
	return transformed.Bytes(), sm
}

func rebaseAlternateIfPositions(value reflect.Value, sm alternateIfSourceMap, lineStarts []int) error {
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
		return rebaseAlternateIfPositions(value.Elem(), sm, lineStarts)
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			if field.CanSet() {
				if err := rebaseAlternateIfPositions(field, sm, lineStarts); err != nil {
					return err
				}
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if err := rebaseAlternateIfPositions(value.Index(i), sm, lineStarts); err != nil {
				return err
			}
		}
	}
	return nil
}
