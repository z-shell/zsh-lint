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
	return parseAlternateIfBraceWithParser(src, name, firstErr, parseWithAdapters)
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
	edits, withdrew, ok := scanAlternateIfEdits(src, seedOffset)
	if !ok {
		return nil, firstErr
	}
	if withdrew {
		// A withdrawn construct is left as raw brace-form text for the
		// retry to reject. Re-entering this adapter on the transformed
		// text would undo that: a synthetic `fi` or `done` line now follows
		// the withdrawn body's `}`, which gives it a tail it lacks in the
		// original source (`if [[ x ]] { if f && [[ c ]] { : } }`).
		parse = retryExcluding(parseAlternateIfBrace)
	}

	transformed, sourceMap := applyAlternateIfEdits(src, edits)
	lineStarts := originalLineStarts(src)
	tree, err := parse(transformed, name)
	if err != nil {
		return nil, rebaseAlternateIfError(err, sourceMap, lineStarts)
	}

	if err := rebaseAlternateIfPositions(reflect.ValueOf(tree), sourceMap, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing alternate if positions: %w", name, err)
	}

	return tree, nil
}

func scanAlternateIfEdits(src []byte, seedOffset int) ([]alternateIfEdit, bool, bool) {
	var edits []alternateIfEdit

	inSingleQuote := false
	inDoubleQuote := false
	inANSICQuote := false
	escaped := false
	inComment := false

	atWordStart := true
	atCommandStart := true

	type ifState int
	const (
		ifNone ifState = iota
		ifSawIf
		ifSawElif
		ifSawElse
		ifSawWhile
	)
	type blockKind int
	const (
		kindNormalBlock blockKind = iota
		kindIfThen
		kindElifThen
		kindElse
		kindWhileDo
		kindParamExpansion
		kindCondGroup
	)
	type blockFrame struct {
		kind       blockKind
		openOffset int
		// listCond marks a body whose chain has a condition that
		// continued past a connective (#376); see listConditionTailOK.
		listCond bool
		// chain holds the indices in edits of this body's opener and of
		// every opener and chain edit before it in the same if chain, so
		// the whole construct can be withdrawn.
		chain []int
		// savedIf is the armed condition a kindCondGroup frame suspends
		// while its contents are scanned.
		savedIf ifState
	}
	var blockStack []blockFrame

	currentIf := ifNone
	// condAfterConnective records that the armed condition has continued
	// past a `&&`, `||` or pipe (#376). What follows the connective is a
	// new command: a delimited test there may lead to the body brace, and a
	// brace group there is a condition element whose own `}` may.
	condAfterConnective := false
	// condParens counts `(` opened inside the armed condition, and
	// inCondBacktick tracks a backquoted command substitution there. A
	// `&&`, `|` or newline inside either (`$(f | g)`, `(a|b)`) belongs to
	// the nested list, not the condition list.
	condParens := 0
	inCondBacktick := false
	// condAtElement is set by the keyword and by a connective, and cleared
	// by the next word other than `!` or `time`: only at that position can
	// `[[`, `((` or `{` open a condition element. `print {a} {b}` and
	// `() { g }` are arguments and a function, not a group.
	condAtElement := false
	arm := func(state ifState) {
		currentIf = state
		condAfterConnective = false
		condParens = 0
		inCondBacktick = false
		condAtElement = state == ifSawIf || state == ifSawElif || state == ifSawWhile
	}
	disarm := func() { arm(ifNone) }
	// chainListCond and chainEdits carry a closed if or elif body's state
	// to the elif or else body chained directly after it, since one
	// synthetic `fi` closes the whole chain. The body that opens next
	// consumes them.
	chainListCond := false
	var chainEdits []int
	// withdrawn marks edits of a construct the scan gave up on.
	withdrawn := map[int]bool{}
	pushBody := func(bk blockKind, offset int) {
		var listCond bool
		var chain []int
		switch bk {
		case kindElifThen:
			listCond = condAfterConnective || chainListCond
			chain = append(chain, chainEdits...)
		case kindElse:
			listCond = chainListCond
			chain = append(chain, chainEdits...)
		default:
			listCond = condAfterConnective
		}
		chainListCond = false
		chainEdits = nil
		chain = append(chain, len(edits)-1)
		blockStack = append(blockStack, blockFrame{kind: bk, openOffset: offset, listCond: listCond, chain: chain})
	}
	bodyEdit := func(state ifState) (alternateIfEditKind, blockKind) {
		switch state {
		case ifSawElif:
			return editElifThen, kindElifThen
		case ifSawWhile:
			return editWhileDo, kindWhileDo
		}
		return editIfThen, kindIfThen
	}

	i := 0
	seedRecognized := false

	for i < len(src) {
		b := src[i]
		// Inside `${...}` every byte is part of a word: `${a## ##}` has no
		// comment, `${a:-if}` has no keyword, and a nested `{` is not a
		// block. Only quotes, escapes, and brace depth matter there.
		inParamExpansion := len(blockStack) > 0 && blockStack[len(blockStack)-1].kind == kindParamExpansion

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
			// A line continuation is removed by the lexer, so it changes
			// neither the word nor the command position: `} \` newline
			// `else {` is the same chain as `} else {`.
			if i+1 < len(src) && src[i+1] == '\n' {
				i += 2
				continue
			}
			escaped = true
			atWordStart = false
			atCommandStart = false
			i++
			continue
		}

		if b == '#' && atWordStart && !inParamExpansion {
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

		if inParamExpansion {
			switch {
			case b == '$' && i+1 < len(src) && src[i+1] == '{':
				blockStack = append(blockStack, blockFrame{kind: kindParamExpansion, openOffset: i})
				i += 2
			case b == '{':
				blockStack = append(blockStack, blockFrame{kind: kindParamExpansion, openOffset: i})
				i++
			case b == '}':
				blockStack = blockStack[:len(blockStack)-1]
				i++
			default:
				i++
			}
			atWordStart = false
			atCommandStart = false
			continue
		}

		if atWordStart && atCommandStart {
			if matchSourceWord(src, i, "if") {
				arm(ifSawIf)
				chainListCond = false
				chainEdits = nil
				i += 2
				atWordStart = false
				atCommandStart = false
				continue
			}
			if matchSourceWord(src, i, "elif") {
				arm(ifSawElif)
				i += 4
				atWordStart = false
				atCommandStart = false
				continue
			}
			if matchSourceWord(src, i, "else") {
				arm(ifSawElse)
				i += 4
				atWordStart = false
				atCommandStart = false
				continue
			}
			// `until list { list }` is the same alternate form as `while`;
			// the rewritten `until ...; do ... done` keeps the keyword.
			if matchSourceWord(src, i, "while") || matchSourceWord(src, i, "until") {
				arm(ifSawWhile)
				i += 5
				atWordStart = false
				atCommandStart = false
				continue
			}
		}

		if currentIf == ifSawIf || currentIf == ifSawElif || currentIf == ifSawWhile {
			// A `while` searches past newlines for its body brace, but not
			// once the condition has continued past a connective: Zsh reads
			// a brace on the next line as one more condition element there
			// (`while false && [[ a ]]`, newline, `{ print X; break }`
			// prints X), so a #376 shape must not claim it as the body.
			// Before any connective, `[[` and `((` open a test wherever
			// main found one; a `{` and anything after a connective must
			// sit where a command starts.
			testElement := condParens == 0 && !inCondBacktick && (condAtElement || !condAfterConnective)
			condElement := condParens == 0 && !inCondBacktick && condAtElement
			if condAtElement && b != ' ' && b != '\t' && b != '\n' {
				if n := conditionPrefixWordLen(src, i); n > 0 && atWordStart {
					i += n
					atWordStart = false
					atCommandStart = false
					continue
				}
				condAtElement = false
			}
			if testElement && b == '[' && i+1 < len(src) && src[i+1] == '[' {
				end := scanClosingDoubleBracket(src, i)
				if end > i {
					i = end
					braceOffset := scanAlternateConditionBrace(src, i, currentIf == ifSawWhile && !condAfterConnective)
					if braceOffset < len(src) && src[braceOffset] == '{' {
						if braceOffset == seedOffset {
							seedRecognized = true
						}
						var kind alternateIfEditKind
						var bk blockKind
						switch currentIf {
						case ifSawElif:
							kind = editElifThen
							bk = kindElifThen
						case ifSawWhile:
							kind = editWhileDo
							bk = kindWhileDo
						default:
							kind = editIfThen
							bk = kindIfThen
						}
						edits = append(edits, alternateIfEdit{offset: braceOffset, kind: kind})
						pushBody(bk, braceOffset)
						disarm()
						i = braceOffset + 1
						atWordStart = true
						atCommandStart = true
						continue
					}
					if condAfterConnective {
						// Resume after the test rather than fall through
						// with the stale `[` byte.
						atWordStart = false
						atCommandStart = false
						continue
					}
					// main steps over the byte after the test without
					// disarming, so `if [[ a ]]; [[ b ]] { ... }` and the
					// newline form keep their brace body. Keep that, and
					// let a `;` or newline there start a new element.
					if i < len(src) {
						condAtElement = src[i] == ';' || src[i] == '\n'
						i++
					}
					atWordStart = condAtElement
					atCommandStart = false
					continue
				}
			}
			if testElement && b == '(' && i+1 < len(src) && src[i+1] == '(' {
				end := scanClosingDoubleParen(src, i)
				if end > i {
					i = end
					braceOffset := scanAlternateConditionBrace(src, i, currentIf == ifSawWhile && !condAfterConnective)
					if braceOffset < len(src) && src[braceOffset] == '{' {
						if braceOffset == seedOffset {
							seedRecognized = true
						}
						var kind alternateIfEditKind
						var bk blockKind
						switch currentIf {
						case ifSawElif:
							kind = editElifThen
							bk = kindElifThen
						case ifSawWhile:
							kind = editWhileDo
							bk = kindWhileDo
						default:
							kind = editIfThen
							bk = kindIfThen
						}
						edits = append(edits, alternateIfEdit{offset: braceOffset, kind: kind})
						pushBody(bk, braceOffset)
						disarm()
						i = braceOffset + 1
						atWordStart = true
						atCommandStart = true
						continue
					}
					// The condition is not followed by a brace body, so this is
					// not the alternate form. Disarm before moving on: a `while`
					// searches past newlines for its `{`, so leaving the state
					// armed lets a brace on a later line be claimed as this
					// loop's body. `while (( i < 3 )) (( i++ ))` followed by a
					// line starting `{ ... } always { ... }` did exactly that,
					// masking the try block into a loop body and rejecting
					// valid Zsh (#337).
					//
					// A connective after the test continues the condition
					// list instead, so the body may still follow a later
					// element: `if (( 1 )) && f && (( 2 )) { ... }` (#376).
					if conditionConnectiveLen(src, skipInlineSpaces(src, i)) == 0 {
						disarm()
					}
					// Resume at the byte after the test rather than fall
					// through with the stale `(` byte: that would count an
					// open paren while armed, and once disarmed the switch
					// below would consume the byte after `))` as a word
					// byte. A blank there then hid a following comment
					// (`(( 1 )) # {`), and a newline or `;` hid the command
					// start of `if [[ a ]] { : }` on the next line.
					atWordStart = false
					atCommandStart = false
					continue
				}
			}
			if b == '{' && condAfterConnective && atWordStart && condElement {
				// A brace group after a connective is a condition element
				// (scanAlternateConditionBrace documents the rule). Scan
				// its contents like any block, so quoting and nested
				// alternate forms stay in step, and suspend the condition
				// until its `}`; the body is a brace after that.
				blockStack = append(blockStack, blockFrame{kind: kindCondGroup, openOffset: i, savedIf: currentIf})
				disarm()
				i++
				atWordStart = true
				atCommandStart = true
				continue
			}
			if b == '{' && currentIf != ifNone && !condAfterConnective && condElement {
				end := scanClosingBrace(src, i)
				if end > i {
					// The body brace sits on the group's line for every
					// form: after a newline Zsh reads a brace as one more
					// condition element (#330), so `until { true }`, newline,
					// `{ : }` has no body brace.
					braceOffset := skipInlineSpaces(src, end)
					if braceOffset < len(src) && src[braceOffset] == '{' {
						if braceOffset == seedOffset {
							seedRecognized = true
						}
						var kind alternateIfEditKind
						var bk blockKind
						if currentIf == ifSawElif {
							kind = editElifThen
							bk = kindElifThen
						} else {
							kind = editIfThen
							bk = kindIfThen
						}
						edits = append(edits, alternateIfEdit{offset: braceOffset, kind: kind})
						pushBody(bk, braceOffset)
						disarm()
						i = braceOffset + 1
						atWordStart = true
						atCommandStart = true
						continue
					}
				}
			}
		}

		if currentIf == ifSawElse {
			braceOffset := skipAlternateConditionSpaces(src, i)
			if braceOffset < len(src) && src[braceOffset] == '{' {
				if braceOffset == seedOffset {
					seedRecognized = true
				}
				edits = append(edits, alternateIfEdit{offset: braceOffset, kind: editElse})
				pushBody(kindElse, braceOffset)
				disarm()
				i = braceOffset + 1
				atWordStart = true
				atCommandStart = true
				continue
			}
		}

		if b == '$' && i+1 < len(src) && src[i+1] == '{' {
			// `${` opens a parameter expansion, not a block. The frame keeps
			// brace depth balanced while the bytes up to its `}` are scanned
			// as word bytes above: `${#a}` is the length operator, not a
			// comment that would swallow the body's closing brace.
			blockStack = append(blockStack, blockFrame{kind: kindParamExpansion, openOffset: i})
			i += 2
			atWordStart = false
			atCommandStart = false
			continue
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
				if top.kind == kindCondGroup {
					arm(top.savedIf)
					condAfterConnective = true
					// The body follows the group on the same line after a
					// blank: `if f && { g } { body }` runs body, and Zsh
					// rejects `if f && { g }{ body }`.
					braceOffset := skipInlineSpaces(src, i+1)
					if braceOffset > i+1 && braceOffset < len(src) && src[braceOffset] == '{' {
						if braceOffset == seedOffset {
							seedRecognized = true
						}
						kind, bk := bodyEdit(currentIf)
						edits = append(edits, alternateIfEdit{offset: braceOffset, kind: kind})
						pushBody(bk, braceOffset)
						disarm()
						i = braceOffset + 1
						atWordStart = true
						atCommandStart = true
						continue
					}
					i++
					atWordStart = false
					atCommandStart = false
					continue
				}
				// Native Zsh continues a brace-form if with else or elif
				// only on the same logical line as the closing brace. After a
				// newline, `;`, or comment the if is complete and a following
				// else or elif belongs to an enclosing classic if.
				nextWordOffset := skipInlineSpaces(src, i+1)
				chains := nextWordOffset < len(src) && (matchSourceWord(src, nextWordOffset, "elif") || matchSourceWord(src, nextWordOffset, "else"))
				// Only an if or elif body chains; `else { } else { }` is a
				// Zsh parse error. main accepts that for the plain form, so
				// the refusal is scoped to the new shapes.
				chainable := chains && (top.kind == kindIfThen || top.kind == kindElifThen)
				if top.listCond && !chainable && !listConditionTailOK(src, i+1) {
					// Withdraw the construct rather than the whole scan, so
					// every other alternate form in the file is still
					// rewritten and the retry reports this one's error.
					for _, e := range top.chain {
						withdrawn[e] = true
					}
					i++
					atWordStart = true
					atCommandStart = true
					continue
				}
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
						chainListCond = top.listCond
						chainEdits = append(append([]int(nil), top.chain...), len(edits)-1)
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

		// An alternate-form condition is a list, so a pipeline or `&&`/`||`
		// connective continues it: `if f arg && [[ -z $x ]] { ... }` has
		// the plain command `f arg` as its head and the brace after the
		// delimited test as its body (#376). Stay armed across the operator
		// and treat what follows as a new command. `;`, `&` and a bare
		// newline still end the scan's interest in the condition, and
		// disarm clears the connective state with it.
		if currentIf == ifSawIf || currentIf == ifSawElif || currentIf == ifSawWhile {
			switch b {
			case '(':
				condParens++
			case ')':
				if condParens > 0 {
					condParens--
				}
			case '`':
				inCondBacktick = !inCondBacktick
			}
			if b == '|' && i > 0 && src[i-1] == '>' {
				// `>|` is a clobbering redirect, not a pipe.
				i++
				atWordStart = false
				atCommandStart = false
				continue
			}
			if condParens > 0 || inCondBacktick {
				// A byte of a nested list, `$(f && g)` or `(a|b)`: its
				// operators and newlines are not the condition's.
				i++
				atWordStart = false
				atCommandStart = false
				continue
			}
			if n := conditionConnectiveLen(src, i); n > 0 && condParens == 0 && !inCondBacktick {
				condAfterConnective = true
				condAtElement = true
				i += n
				atWordStart = true
				atCommandStart = true
				continue
			}
		}

		switch b {
		case ' ', '\t', '\n':
			atWordStart = true
			if b == '\n' {
				atCommandStart = true
				// A newline directly after a connective is a continuation
				// of the same condition list, not a separator.
				if !endsWithConditionConnective(src, i) {
					disarm()
				}
			}
		case ';', '&', '|':
			atWordStart = true
			atCommandStart = true
			disarm()
		default:
			atWordStart = false
			atCommandStart = false
		}
		i++
	}

	if len(withdrawn) > 0 {
		kept := edits[:0:0]
		seedRecognized = false
		for k, e := range edits {
			if withdrawn[k] {
				continue
			}
			kept = append(kept, e)
			if e.offset == seedOffset && e.kind != editChain && e.kind != editFi && e.kind != editDone {
				seedRecognized = true
			}
		}
		edits = kept
	}
	if !seedRecognized || len(edits) == 0 {
		return nil, false, false
	}
	return edits, len(withdrawn) > 0, true
}

// conditionConnectiveLen returns the length of the pipeline or list connective
// at i that joins two elements of one condition list (`&&`, `||`, `|&`, `|`),
// or 0. A lone `&` is a list terminator, not a connective, so it is not one.
func conditionConnectiveLen(src []byte, i int) int {
	if i >= len(src) {
		return 0
	}
	if i+1 < len(src) {
		switch string(src[i : i+2]) {
		case "&&", "||", "|&":
			return 2
		}
	}
	if src[i] == '|' {
		return 1
	}
	return 0
}

// conditionPrefixWordLen returns the length of a `!` or `time` word at i,
// which may precede a condition element without ending the search for one,
// or 0.
func conditionPrefixWordLen(src []byte, i int) int {
	for _, word := range []string{"!", "time"} {
		if matchSourceWord(src, i, word) {
			return len(word)
		}
	}
	return 0
}

// endsWithConditionConnective reports whether the bytes before the newline at
// i, ignoring blanks, end with a connective that carries the condition list
// onto the next line.
func endsWithConditionConnective(src []byte, i int) bool {
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
		j--
	}
	if j < 0 {
		return false
	}
	if src[j] == '|' {
		return true
	}
	return j >= 1 && (src[j] == '&' && (src[j-1] == '&' || src[j-1] == '|'))
}

// listConditionTailOK reports whether the bytes after the `}` at i-1 that
// closes a list-condition body (#376) may end the construct: the end of the
// source, a newline, a `;` that is not `;;`, or a comment after a blank.
//
// The retry closes the body with a synthetic `fi` or `done` framed by
// newlines, which detaches whatever follows on the same line, so main already
// accepts tails native Zsh rejects after a brace-form closer (a word, a
// subshell, a glued comment, and a redirect after an if; #275). The new
// shapes refuse every other tail instead, so they never gain an acceptance;
// a refused construct is withdrawn, and the retry then runs without this
// adapter so its synthetic lines cannot supply the missing tail.
// Native Zsh does allow a connective, pipe, `&` or redirect after
// `while ... { }` and after an if chain's else body; main rejects those for
// the plain forms as well, and they stay rejected here until #275 lands.
func listConditionTailOK(src []byte, i int) bool {
	j := skipInlineSpaces(src, i)
	if j >= len(src) {
		return true
	}
	switch src[j] {
	case '\n':
		return true
	case ';':
		return j+1 >= len(src) || src[j+1] != ';'
	case '#':
		return j > i
	}
	return false
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

// scanAlternateConditionBrace returns the offset of the `{` that opens an
// alternate-form body, or an offset holding no `{` when the condition has no
// body brace.
//
// The caller has already scanned past the condition's first delimited test, so
// a `{` found before any connective is the body. After a `&&` or a `||`, only
// a further delimited test keeps the body in reach: Alternate Forms For
// Complex Commands requires the test to be "suitably delimited, such as by
// `[[ ... ]]` or `(( ... ))`, else the end of the test will not be
// recognized", so a `{ ... }` written directly after a connective is another
// element of the condition list rather than the body.
//
//	if [[ -n $b ]] { body }                    # body
//	if [[ -n $b ]] && [[ -n $c ]] { body }     # body, after a delimited test
//	if [[ -n $b ]] && { print cond; }          # no body: a parse error in Zsh
//	if [[ -n $b ]] && { print cond; } { body } # the SECOND brace is the body
//
// Treating the brace after a connective as the body made an ordinary
// `if ... ; then` whose condition holds a brace group fail to parse whenever
// the same file also held an alternate-form construct, since the adapter
// rewrote that condition brace into a body opener.
func scanAlternateConditionBrace(src []byte, i int, allowNewline bool) int {
	skipSpaces := skipInlineSpaces
	if allowNewline {
		skipSpaces = skipAlternateConditionSpaces
	}
	// The caller scanned the condition's first delimited test, so the first
	// brace found is the body. A connective clears that until a further
	// delimited test, or a condition-list brace group, is scanned past.
	afterDelimitedTest := true
	for {
		i = skipSpaces(src, i)
		if i < len(src) && src[i] == '{' {
			if !afterDelimitedTest {
				// A brace group in the condition list. The body, if the source
				// has one, is the brace after this group's `}`.
				end := scanClosingBrace(src, i)
				if end < 0 {
					return len(src)
				}
				afterDelimitedTest = true
				i = end
				continue
			}
			return i
		}
		if i+1 >= len(src) || (src[i] != '&' || src[i+1] != '&') && (src[i] != '|' || src[i+1] != '|') {
			return i
		}
		i = skipSpaces(src, i+2)
		for i < len(src) && src[i] == '!' {
			i = skipSpaces(src, i+1)
		}
		switch {
		case i+1 < len(src) && src[i] == '[' && src[i+1] == '[':
			i = scanClosingDoubleBracket(src, i)
		case i+1 < len(src) && src[i] == '(' && src[i+1] == '(':
			i = scanClosingDoubleParen(src, i)
		case i < len(src) && src[i] == '{':
			// A brace group after the connective is part of the condition
			// list, never the body.
			afterDelimitedTest = false
			continue
		default:
			// Nothing after this connective can open a body. The offset
			// itself is inert: the caller only acts on a returned `{`, and
			// this branch is reached precisely when the byte here is not one.
			return i
		}
		if i < 0 {
			return len(src)
		}
	}
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

// scanClosingDoubleBracket returns the offset just past the `]]` that closes
// the conditional expression opened at start, or -1. Like the parser, it
// accepts `]]` only as a whole word: `x]]`, `[^\]]`, `([]])`, `(x)]]`, and
// `"]]"` are part of the pattern or string that contains them. A `]]` counts
// only at a word start or right after the `)` that closes a condition group,
// and before whitespace, a separator, `)`, or the end of the source.
//
// A `(` opens a condition group only where a condition may start: after
// `[[`, another group open, `&&`, `||`, or `!`. Elsewhere it is part of a
// pattern word, as in `$a == (x)]] ]]`, and its `)` does not end the group.
func scanClosingDoubleBracket(src []byte, start int) int {
	var (
		inSingle, inDouble, inANSIC bool
		groupDepth                  int // open condition groups
		patternDepth                int // open pattern parens in the current word
		wordLen                     int
		wordIsBang                  bool
		atWordStart                 = true
		atConditionStart            = true // a `(` here opens a group
		afterGroupClose             bool
	)
	endWord := func() {
		if wordLen > 0 {
			atConditionStart = wordIsBang
		}
		wordLen = 0
		wordIsBang = false
		patternDepth = 0
		atWordStart = true
	}
	wordByte := func(b byte) {
		wordIsBang = wordLen == 0 && b == '!'
		wordLen++
		atWordStart = false
		afterGroupClose = false
	}
	for i := start + 2; i < len(src); i++ {
		b := src[i]
		switch {
		case inSingle:
			inSingle = b != '\''
		case inANSIC:
			switch b {
			case '\\':
				i++
			case '\'':
				inANSIC = false
			}
		case inDouble:
			switch b {
			case '\\':
				i++
			case '"':
				inDouble = false
			}
		case b == '\\':
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
				endWord()
				continue
			}
			wordByte(b)
			i++
		case b == '\'':
			inSingle = true
			wordByte(b)
		case b == '"':
			inDouble = true
			wordByte(b)
		case b == '$' && i+1 < len(src) && src[i+1] == '\'':
			inANSIC = true
			wordByte(b)
			i++
		case isConditionWordSpace(b):
			// Whitespace inside `$( f )`, `$(( a + 1 ))`, or `(x|y z)` stays
			// inside the word.
			if patternDepth == 0 {
				endWord()
			}
		case (b == '&' || b == '|') && i+1 < len(src) && src[i+1] == b:
			endWord()
			atConditionStart = true
			afterGroupClose = false
			i++
		case b == '(':
			if atWordStart && atConditionStart {
				groupDepth++
				afterGroupClose = false
				continue
			}
			patternDepth++
			wordByte(b)
		case b == ')':
			if patternDepth > 0 {
				patternDepth--
				wordByte(b)
				continue
			}
			if groupDepth == 0 {
				return -1
			}
			groupDepth--
			endWord()
			atConditionStart = false
			afterGroupClose = true
		case b == ']' && i+1 < len(src) && src[i+1] == ']' && (atWordStart || afterGroupClose) && (i+2 == len(src) || isConditionWordEnd(src[i+2])):
			return i + 2
		default:
			wordByte(b)
		}
	}
	return -1
}

func isConditionWordSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n'
}

func isConditionWordEnd(b byte) bool {
	return isConditionWordSpace(b) || b == ';' || b == '&' || b == '|' || b == ')'
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
		rebased, err := rebaseAlternateIfPos(position, sm, lineStarts)
		if err != nil {
			return err
		}
		value.Set(reflect.ValueOf(rebased))
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

// rebaseAlternateIfPos maps one transformed position back to the original
// source.
func rebaseAlternateIfPos(position syntax.Pos, sm alternateIfSourceMap, lineStarts []int) (syntax.Pos, error) {
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

// rebaseAlternateIfError rewrites the position of a parser error raised on
// the transformed source so it points into the original file. The synthetic
// `; then`, `fi`, and `done` lines otherwise shift every later line number,
// and the retry error is the useful one: it names the gap that remains after
// the brace-form if was accepted. An error the mapping cannot place is
// returned unchanged rather than dropped.
func rebaseAlternateIfError(err error, sm alternateIfSourceMap, lineStarts []int) error {
	var parseErr syntax.ParseError
	if errors.As(err, &parseErr) && parseErr.Pos.IsValid() {
		if rebased, mapErr := rebaseAlternateIfPos(parseErr.Pos, sm, lineStarts); mapErr == nil {
			parseErr.Pos = rebased
			return parseErr
		}
		return err
	}
	var langErr syntax.LangError
	if errors.As(err, &langErr) && langErr.Pos.IsValid() {
		if rebased, mapErr := rebaseAlternateIfPos(langErr.Pos, sm, lineStarts); mapErr == nil {
			langErr.Pos = rebased
			return langErr
		}
	}
	return err
}
