package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type forEditKind int

const (
	editMaskExtraNames forEditKind = iota
	editShortForOpen
	editShortForClose
	editShortForDo
	editShortForDone
	editMaskListNewline
)

type forEdit struct {
	start int
	end   int
	kind  forEditKind
}

type forSourceMap struct {
	origByTransformed []int
}

func parseMultiNameFor(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseMultiNameForWithParser(src, name, firstErr, parseWithAdapters)
}

func parseMultiNameForWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	if !strings.HasPrefix(parseErr.Text, "`for ") || !strings.Contains(parseErr.Text, "must be followed by") {
		return nil, firstErr
	}

	seedOffset := int(parseErr.Pos.Offset())
	edits, ok := scanForEdits(src, seedOffset)
	if !ok {
		return nil, firstErr
	}

	transformed, sourceMap := applyForEdits(src, edits)
	lineStarts := originalLineStarts(src)
	tree, err := parse(transformed, name)
	if err != nil {
		return nil, rebaseForError(err, sourceMap, lineStarts)
	}

	if err := rebaseForPositions(reflect.ValueOf(tree), sourceMap, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing for loop positions: %w", name, err)
	}

	return tree, nil
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func scanForEdits(src []byte, seedOffset int) ([]forEdit, bool) {
	var edits []forEdit

	inSingleQuote := false
	inDoubleQuote := false
	inANSICQuote := false
	escaped := false
	inComment := false

	atWordStart := true
	atCommandStart := true

	seedRecognized := false

	// heredocs holds the here-documents opened on the current line; their
	// bodies are text, not commands, and are skipped at the next line (#429).
	// arithParens tracks an open `((` or `$((`, where `<<` is a shift.
	var heredocs []pendingHeredoc
	arithParens := 0

	i := 0
	for i < len(src) {
		if len(heredocs) > 0 && !inSingleQuote && !inDoubleQuote && !inANSICQuote && atUnescapedLineStart(src, i) {
			next, ok := consumeHeredocBodies(src, i, heredocs)
			if !ok {
				return nil, false
			}
			heredocs = nil
			i = next
			continue
		}
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
			atWordStart = false
			atCommandStart = false
			if end, ok := skipDoubleQuotedString(src, i); ok {
				i = end + 1
				continue
			}
			inDoubleQuote = true
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

		if atWordStart && atCommandStart && matchSourceWord(src, i, "for") {
			forStart := i
			i += 3
			ws := skipSpaces(src, i)
			if ws < len(src) && ws > i {
				i = ws
				if i+1 < len(src) && src[i] == '(' && src[i+1] == '(' {
					atWordStart = false
					atCommandStart = false
					continue
				}
				name1Start := i
				for i < len(src) && isIdentByte(src[i]) {
					i++
				}
				name1End := i
				if name1End > name1Start {
					afterName1 := skipSpaces(src, i)
					if afterName1 < len(src) {
						// A parenthesized word list may follow more than one
						// name: `for a b (1 2) { : }` is the multi-name
						// production and the alternate-form production
						// composed (#324). With no extra names this is the
						// single-name short form, unchanged.
						extraStart, extraEnd, afterNames := scanForExtraNames(src, afterName1)
						if afterNames < len(src) && src[afterNames] == '(' &&
							(afterNames+1 >= len(src) || src[afterNames+1] != '(') {
							parenOpen := afterNames
							parenClose, newlines, listOK := scanShortForList(src, parenOpen)
							if parenClose > parenOpen {
								braceOpen := skipSpacesAndComments(src, parenClose)
								if braceOpen < len(src) && src[braceOpen] == '{' {
									braceClose := scanClosingBrace(src, braceOpen)
									if braceClose > braceOpen && listOK {
										if forStart <= seedOffset && seedOffset <= braceClose {
											seedRecognized = true
										}
										if extraEnd > extraStart {
											edits = append(edits, forEdit{start: extraStart, end: extraEnd, kind: editMaskExtraNames})
										}
										edits = append(edits, forEdit{start: parenOpen, end: parenOpen + 1, kind: editShortForOpen})
										for _, nl := range newlines {
											edits = append(edits, forEdit{start: nl, end: nl + 1, kind: editMaskListNewline})
										}
										edits = append(edits, forEdit{start: parenClose - 1, end: parenClose, kind: editShortForClose})
										edits = append(edits, forEdit{start: braceOpen, end: braceOpen + 1, kind: editShortForDo})
										edits = append(edits, forEdit{start: braceClose - 1, end: braceClose, kind: editShortForDone})
										i = braceClose
										atWordStart = true
										atCommandStart = true
										continue
									}
								}
							}
						}

						curr := afterName1
						extraNamesStart := curr
						extraNamesEnd := curr
						hasExtraNames := false
						for curr < len(src) {
							wStart := curr
							for curr < len(src) && isIdentByte(src[curr]) {
								curr++
							}
							wEnd := curr
							if wEnd == wStart {
								break
							}
							word := string(src[wStart:wEnd])
							if word == "in" || word == "do" {
								extraNamesEnd = wStart
								curr = wStart
								break
							}
							hasExtraNames = true
							curr = skipSpaces(src, curr)
						}

						if hasExtraNames && curr < len(src) && matchSourceWord(src, curr, "in") {
							if forStart <= seedOffset && seedOffset <= curr+2 {
								seedRecognized = true
							}
							edits = append(edits, forEdit{
								start: extraNamesStart,
								end:   extraNamesEnd,
								kind:  editMaskExtraNames,
							})
							i = curr + 2
							atWordStart = false
							atCommandStart = false
							continue
						}
					}
				}
			}
		}

		switch {
		case arithParens > 0:
			switch b {
			case '(':
				arithParens++
			case ')':
				arithParens--
			}
		case b == '(' && i+1 < len(src) && src[i+1] == '(':
			arithParens = 1
		case b == '<':
			heredoc, end, isHeredoc, ok := heredocAt(src, i)
			if !ok {
				return nil, false
			}
			if isHeredoc || end > i {
				if isHeredoc {
					heredocs = append(heredocs, heredoc)
				}
				i = end
				atWordStart = false
				atCommandStart = false
				continue
			}
		}

		switch b {
		case ' ', '\t', '\n':
			atWordStart = true
			if b == '\n' {
				atCommandStart = true
			}
		case ';', '&', '|':
			atWordStart = true
			atCommandStart = true
		case '!':
			// A `!` word negates the pipeline and leaves the next word in
			// command position (issue #321), as scanCommandWords reads it;
			// a `!` inside a word ends the command position like any byte.
			bangWord := atWordStart && i+1 < len(src) && (src[i+1] == ' ' || src[i+1] == '\t')
			atWordStart = false
			if !bangWord {
				atCommandStart = false
			}
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

// scanForExtraNames reads the loop names after the first one, starting at from,
// and reports their extent together with the offset just past them.
//
// A `for` header may name more than one variable before its word list, and the
// names the upstream WordIter cannot hold are masked so the rewritten loop keeps
// its original byte length. The scan stops at `in` or `do`, which end the name
// list rather than continue it, so this never consumes a keyword. With no extra
// names it returns an empty extent at from, which leaves the single-name short
// form unchanged.
func scanForExtraNames(src []byte, from int) (start, end, next int) {
	start, end, next = from, from, from
	curr := from
	for curr < len(src) {
		wStart := curr
		for curr < len(src) && isIdentByte(src[curr]) {
			curr++
		}
		if curr == wStart {
			break
		}
		switch string(src[wStart:curr]) {
		case "in", "do":
			return start, end, wStart
		}
		end = curr
		next = skipSpaces(src, curr)
		curr = next
	}
	return start, end, next
}

func skipSpaces(src []byte, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return i
}

// scanShortForList finds the `)` that closes the word list of an alternate-form
// for loop opened at parenOpen, and the newlines inside that list which native
// zsh treats as plain word separators, so that the rewritten
// `for name in words; do` form, which cannot hold a newline before `do`,
// keeps every word. The word extents come from the front-end's own word lexer
// (syntax.Parser.WordsSeq) rather than a hand-written state machine, so a `)`
// inside a quoted word, a `${...}` operator, a `$(...)` case pattern, or a
// comment inside a command substitution never closes the list; the lexer stops
// at the first token that is not a word, and only a bare `)` there closes the
// list. The gaps between words hold only what the lexer skipped: blanks,
// newlines, `\`-newline continuations, and comments. A newline in a gap is
// masked unless a backslash precedes it, so a continuation keeps joining its
// two lines. parenClose is the offset just past the closing `)`, or -1 when
// the list never closes or the lexer rejects a word. A list carrying a `#`
// comment reports ok=false and is not rewritten at all: masking the comment
// would drop a `*syntax.Comment` node the suppression pass may read, so the
// parser error stands for that loop.
func scanShortForList(src []byte, parenOpen int) (parenClose int, newlines []int, ok bool) {
	parenClose, newlines, _, ok = scanParenWordList(src, parenOpen)
	return parenClose, newlines, ok
}

// scanParenWordList is scanShortForList returning the words' extents too,
// as [start, end) offsets in src, for a caller that verifies the rewritten
// loop's items against them (the select paren list adapter, #303).
func scanParenWordList(src []byte, parenOpen int) (parenClose int, newlines []int, words [][2]int, ok bool) {
	base := parenOpen + 1
	var spans [][2]int
	parser := syntax.NewParser(syntax.Variant(syntax.LangZsh))
	var err error
	for w, wordErr := range parser.WordsSeq(bytes.NewReader(src[base:])) {
		if wordErr != nil {
			err = wordErr
			break
		}
		spans = append(spans, [2]int{base + int(w.Pos().Offset()), base + int(w.End().Offset())})
	}
	var parseErr syntax.ParseError
	if !errors.As(err, &parseErr) || parseErr.Text != "`)` is not a valid word" {
		return -1, nil, nil, false
	}
	closeAt := base + int(parseErr.Pos.Offset())
	if closeAt >= len(src) || src[closeAt] != ')' {
		return -1, nil, nil, false
	}

	ok = true
	gapStart := base
	words = spans
	spans = append(spans, [2]int{closeAt, closeAt})
	for _, span := range spans {
		for i := gapStart; i < span[0]; i++ {
			switch src[i] {
			case ' ', '\t', '\r':
			case '\\':
				if i+1 < span[0] && src[i+1] == '\n' {
					i++
					continue
				}
				return -1, nil, nil, false
			case '\n':
				newlines = append(newlines, i)
			case '#':
				ok = false
				for i+1 < span[0] && src[i+1] != '\n' {
					i++
				}
			default:
				return -1, nil, nil, false
			}
		}
		gapStart = span[1]
	}

	return closeAt + 1, newlines, words, ok
}

func applyForEdits(src []byte, edits []forEdit) ([]byte, forSourceMap) {
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].start < edits[j].start
	})

	var transformed bytes.Buffer
	sm := forSourceMap{
		origByTransformed: make([]int, 0, len(src)+len(edits)*10),
	}

	appendSynthetic := func(s string, origOffset int) {
		for _, b := range []byte(s) {
			transformed.WriteByte(b)
			sm.origByTransformed = append(sm.origByTransformed, origOffset)
		}
	}

	appendOriginal := func(b byte, origOffset int) {
		transformed.WriteByte(b)
		sm.origByTransformed = append(sm.origByTransformed, origOffset)
	}

	last := 0
	for _, e := range edits {
		for i := last; i < e.start; i++ {
			appendOriginal(src[i], i)
		}
		switch e.kind {
		case editMaskExtraNames:
			for i := e.start; i < e.end; i++ {
				appendOriginal(' ', i)
			}
		case editShortForOpen:
			appendSynthetic(" in ", e.start)
		case editShortForClose:
			appendSynthetic("; ", e.start)
		case editShortForDo:
			appendSynthetic("do\n", e.start)
		case editShortForDone:
			appendSynthetic("\ndone\n", e.start)
		case editMaskListNewline:
			appendOriginal(' ', e.start)
		}
		last = e.end
	}
	for i := last; i < len(src); i++ {
		appendOriginal(src[i], i)
	}
	sm.origByTransformed = append(sm.origByTransformed, len(src))
	return transformed.Bytes(), sm
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
// transformed source so it points into the original file. The synthetic
// ` in `, `; `, `do\n`, and `\ndone\n` text otherwise shifts every later line
// number, and the retry error is the useful one: it names the gap that
// remains after the alternate-form for loop was accepted. An error the
// mapping cannot place is returned unchanged rather than dropped.
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
