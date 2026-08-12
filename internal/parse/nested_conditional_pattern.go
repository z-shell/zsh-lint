package parse

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"
)

const invalidAlternationOperator = "not a valid test operator: `|`"
const nestedPatternMask byte = 'x'

type patternEdit struct {
	offset      int
	original    byte
	replacement byte
}

type indexedPatternEdit struct {
	originalIndex int
	edit          patternEdit
}

type patternRestoreProbe func()

type patternBatchProbe func()

type activeLegacyLookupProbe func()

type activeSourceQuote uint8

const (
	activeSourceUnquoted activeSourceQuote = iota
	activeSourceSingleQuoted
	activeSourceANSICQuoted
	activeSourceDoubleQuoted
)

type activeSourceFrameKind uint8

const (
	activeSourceRootFrame activeSourceFrameKind = iota
	activeSourceCommandSubstitutionFrame
	activeSourceProcessSubstitutionFrame
	activeSourceBacktickSubstitutionFrame
)

type activeCasePhase uint8

const (
	activeCaseSubject activeCasePhase = iota
	activeCaseAwaitingIn
	activeCasePattern
	activeCaseBody
)

type activeCaseContext struct {
	phase             activeCasePhase
	patternParenDepth int
	inBracketPattern  bool
}

type pendingHeredoc struct {
	delimiter string
	stripTabs bool
}

type activeSourceFrame struct {
	kind            activeSourceFrameKind
	quote           activeSourceQuote
	escaped         bool
	groupDepth      int
	arithmeticDepth int
	conditional     *activeConditionalContext
	inComment       bool
	atWordStart     bool
	atCommandStart  bool
	heredocs        []pendingHeredoc
	caseContexts    []activeCaseContext
	adaptedPattern  bool
	// legacyBacktick and legacyBacktickDepth describe the nearest active
	// legacy substitution at this frame. Command and process children inherit
	// them; quote and arithmetic states remain on their owning frame.
	legacyBacktick      *legacyBacktickSpan
	legacyBacktickDepth int
}

type patternPair struct {
	open  int
	close int
	depth int
}

type patternCandidate struct {
	conditionalStart int
	rhsStart         int
	rhsEnd           int
	edits            []patternEdit
	seed             bool
}

type activeConditionalContext struct {
	start   int
	pattern *activePatternState
}

type activePatternState struct {
	rhsStart            int
	openings            []int
	pairs               []patternPair
	inBracketExpression bool
	numericRangeEnd     int
	nestedAlternation   bool
	seed                bool
	invalid             bool
}

type legacyBacktickSpan struct {
	openStart          int
	openOffset         int
	closeStart         int
	closeOffset        int
	doubleQuotedParent bool
	affected           bool
	children           []*legacyBacktickSpan
}

type legacyBacktickIsland struct {
	root           *legacyBacktickSpan
	markedClosures []*legacyBacktickSpan
}

type nestedPatternCompatibilityBatch struct {
	edits           []patternEdit
	backtickIslands []legacyBacktickIsland
}

func parseNestedConditionalAlternation(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseNestedConditionalAlternationWithParser(src, name, firstErr, parseTree)
}

func parseNestedConditionalAlternationWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
	probes ...legacyBacktickWorkProbe,
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAlternationOperator {
		return nil, firstErr
	}
	batch, ok := nestedPatternBatch(src, int(parseErr.Pos.Offset()))
	if !ok {
		return nil, firstErr
	}
	masked := bytes.Clone(src)
	for _, edit := range batch.edits {
		if edit.offset < 0 || edit.offset >= len(masked) || masked[edit.offset] != edit.original {
			return nil, fmt.Errorf("%s: nested-pattern edit no longer matches source at byte %d", name, edit.offset)
		}
		masked[edit.offset] = edit.replacement
	}
	patternMasked := bytes.Clone(masked)
	if err := applyLegacyBacktickPlaceholders(masked, src, batch.backtickIslands); err != nil {
		return nil, fmt.Errorf("%s: applying legacy backtick placeholders: %w", name, err)
	}

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if err := parseAndGraftLegacyBacktickIslands(
		tree,
		patternMasked,
		src,
		name,
		batch.backtickIslands,
		parse,
		probes...,
	); err != nil {
		return nil, fmt.Errorf("%s: parsing legacy backtick islands: %w", name, err)
	}
	if err := restorePatternEdits(tree, src, batch.edits); err != nil {
		return nil, fmt.Errorf("%s: restoring nested conditional pattern: %w", name, err)
	}
	return tree, nil
}

func nestedPatternBatchEdits(
	src []byte,
	seedOffset int,
	probes ...patternBatchProbe,
) ([]patternEdit, bool) {
	batch, ok := nestedPatternBatch(src, seedOffset, probes...)
	if !ok {
		return nil, false
	}
	return batch.edits, true
}

func nestedPatternBatch(
	src []byte,
	seedOffset int,
	probes ...patternBatchProbe,
) (nestedPatternCompatibilityBatch, bool) {
	var inspect patternBatchProbe
	if len(probes) > 0 {
		inspect = probes[0]
	}
	candidates, backtickIslands, ok := collectNestedPatternCandidates(src, seedOffset, inspect)
	if !ok {
		return nestedPatternCompatibilityBatch{}, false
	}
	seedRecognized := false
	editCount := 0
	for _, candidate := range candidates {
		if inspect != nil {
			inspect()
		}
		if candidate.seed {
			seedRecognized = true
		}
		editCount += len(candidate.edits)
	}
	if !seedRecognized {
		return nestedPatternCompatibilityBatch{}, false
	}

	edits := make([]patternEdit, 0, editCount)
	seenOffsets := make(map[int]bool, editCount)
	for _, candidate := range candidates {
		for _, edit := range candidate.edits {
			if seenOffsets[edit.offset] {
				return nestedPatternCompatibilityBatch{}, false
			}
			seenOffsets[edit.offset] = true
			edits = append(edits, edit)
		}
	}
	return nestedPatternCompatibilityBatch{
		edits:           edits,
		backtickIslands: backtickIslands,
	}, true
}

func collectNestedPatternCandidates(
	src []byte,
	seedOffset int,
	inspect patternBatchProbe,
	legacyLookupProbes ...activeLegacyLookupProbe,
) ([]patternCandidate, []legacyBacktickIsland, bool) {
	var candidates []patternCandidate
	var backtickRoots []*legacyBacktickSpan
	var inspectLegacyLookup activeLegacyLookupProbe
	if len(legacyLookupProbes) > 0 {
		inspectLegacyLookup = legacyLookupProbes[0]
	}
	frames := []activeSourceFrame{newActiveSourceFrame(activeSourceRootFrame, nil)}

	for i := 0; i < len(src); i++ {
		if inspect != nil {
			inspect()
		}
		b := src[i]
		frame := &frames[len(frames)-1]
		if frame.escaped {
			frame.escaped = false
			if frame.kind != activeSourceBacktickSubstitutionFrame ||
				frame.quote != activeSourceUnquoted || b != '`' {
				continue
			}
		}

		switch frame.quote {
		case activeSourceSingleQuoted:
			if b == '\'' {
				frame.quote = activeSourceUnquoted
			}
			continue
		case activeSourceANSICQuoted:
			if b == '\\' {
				frame.escaped = true
				continue
			}
			if b == '\'' {
				frame.quote = activeSourceUnquoted
			}
			continue
		case activeSourceDoubleQuoted:
			if b == '$' && i+1 < len(src) && src[i+1] == '(' &&
				(i+2 >= len(src) || src[i+2] != '(') {
				invalidateActivePattern(frame, i)
				frames = append(frames, newActiveSourceFrame(activeSourceCommandSubstitutionFrame, frame))
				i++
				continue
			}
			if b == '`' {
				invalidateActivePattern(frame, i)
				pushLegacyBacktickFrame(&frames, &backtickRoots, src, i, true, inspectLegacyLookup)
				continue
			}
			if b == '\\' {
				frame.escaped = activeSourceDoubleQuoteEscapes(src, i)
				continue
			}
			if b == '"' {
				frame.quote = activeSourceUnquoted
			}
			continue
		}

		if b == '\n' && frame.arithmeticDepth == 0 && len(frame.heredocs) > 0 {
			next, ok := consumeHeredocBodies(src, i+1, frame.heredocs)
			if !ok {
				return nil, nil, false
			}
			frame.heredocs = nil
			frame.inComment = false
			frame.atWordStart = true
			frame.atCommandStart = true
			i = next - 1
			continue
		}
		if frame.inComment {
			if b == '\n' {
				frame.inComment = false
				frame.atWordStart = true
				frame.atCommandStart = true
			}
			continue
		}

		if frame.arithmeticDepth > 0 {
			if b == '$' && i+1 < len(src) && src[i+1] == '(' &&
				(i+2 >= len(src) || src[i+2] != '(') {
				invalidateActivePattern(frame, i)
				frames = append(frames, newActiveSourceFrame(activeSourceCommandSubstitutionFrame, frame))
				i++
				continue
			}
			if b == '`' {
				invalidateActivePattern(frame, i)
				pushLegacyBacktickFrame(&frames, &backtickRoots, src, i, false, inspectLegacyLookup)
				continue
			}
			if b == '\\' {
				frame.escaped = activeSourceBackslashEscapes(frame, src, i)
				continue
			}
			if b == '$' && i+1 < len(src) && src[i+1] == '\'' {
				frame.quote = activeSourceANSICQuoted
				i++
				continue
			}
			switch b {
			case '\'':
				frame.quote = activeSourceSingleQuoted
			case '"':
				frame.quote = activeSourceDoubleQuoted
			case '(':
				frame.arithmeticDepth++
			case ')':
				frame.arithmeticDepth--
				continue
			}
			continue
		}

		if frame.kind == activeSourceBacktickSubstitutionFrame && b == '`' {
			depth := activeLegacyBacktickDepth(frame, inspectLegacyLookup)
			escapeDepth, delimiterEscapes := legacyBacktickEscapeStateBefore(src, i)
			if escapeDepth == depth {
				invalidateActivePattern(frame, i)
				pushLegacyBacktickFrame(&frames, &backtickRoots, src, i, false, inspectLegacyLookup)
				continue
			}
			if escapeDepth != depth-1 {
				return nil, nil, false
			}
			if !activeSourceFrameCanClose(frame) {
				return nil, nil, false
			}
			frame.legacyBacktick.closeStart = i - delimiterEscapes
			frame.legacyBacktick.closeOffset = i
			frame.legacyBacktick.affected = frame.adaptedPattern
			adaptedPattern := frame.adaptedPattern
			frames = frames[:len(frames)-1]
			if adaptedPattern {
				frames[len(frames)-1].adaptedPattern = true
			}
			continue
		}

		var currentCase *activeCaseContext
		if len(frame.caseContexts) > 0 {
			currentCase = &frame.caseContexts[len(frame.caseContexts)-1]
		}
		if frame.conditional == nil && frame.atWordStart && frame.atCommandStart &&
			(currentCase == nil || currentCase.phase == activeCaseBody) &&
			activeSourceWordAt(src, i, "case") {
			frame.caseContexts = append(frame.caseContexts, activeCaseContext{phase: activeCaseSubject})
			i += len("case") - 1
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if currentCase != nil {
			switch currentCase.phase {
			case activeCaseSubject:
				if !shellSpace(b) {
					currentCase.phase = activeCaseAwaitingIn
				}
			case activeCaseAwaitingIn:
				if frame.atWordStart && activeSourceWordAt(src, i, "in") {
					currentCase.phase = activeCasePattern
					i += len("in") - 1
					frame.atWordStart = false
					frame.atCommandStart = false
					continue
				}
			case activeCasePattern:
				wordEnd := i + len("esac")
				if frame.atWordStart && activeSourceWordAt(src, i, "esac") &&
					(wordEnd >= len(src) || src[wordEnd] != ')') {
					frame.caseContexts = frame.caseContexts[:len(frame.caseContexts)-1]
					i += len("esac") - 1
					frame.atWordStart = false
					frame.atCommandStart = false
					continue
				}
			case activeCaseBody:
				if frame.atWordStart && frame.atCommandStart && activeSourceWordAt(src, i, "esac") {
					frame.caseContexts = frame.caseContexts[:len(frame.caseContexts)-1]
					i += len("esac") - 1
					frame.atWordStart = false
					frame.atCommandStart = false
					continue
				}
				if b == ';' && i+1 < len(src) &&
					(src[i+1] == ';' || src[i+1] == '&' || src[i+1] == '|') {
					currentCase.phase = activeCasePattern
					currentCase.patternParenDepth = 0
					currentCase.inBracketPattern = false
					i++
					frame.atWordStart = true
					frame.atCommandStart = true
					continue
				}
			}
		}

		if frame.conditional != nil && frame.conditional.pattern == nil {
			if operatorEnd, ok := activePatternOperatorEnd(src, i); ok {
				frame.conditional.pattern = &activePatternState{rhsStart: -1}
				i = operatorEnd - 1
				frame.atWordStart = false
				frame.atCommandStart = false
				continue
			}
		}
		if frame.conditional != nil && frame.conditional.pattern != nil {
			if activePatternByteConsumed(frame, src, i, seedOffset, &candidates) {
				frame.atWordStart = false
				frame.atCommandStart = false
				continue
			}
		}

		if b == '\\' {
			frame.escaped = activeSourceBackslashEscapes(frame, src, i)
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if b == '$' && i+1 < len(src) && src[i+1] == '\'' {
			frame.quote = activeSourceANSICQuoted
			i++
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if b == '\'' {
			frame.quote = activeSourceSingleQuoted
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if b == '"' {
			frame.quote = activeSourceDoubleQuoted
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if b == '`' {
			invalidateActivePattern(frame, i)
			frame.atWordStart = false
			frame.atCommandStart = false
			pushLegacyBacktickFrame(&frames, &backtickRoots, src, i, false, inspectLegacyLookup)
			continue
		}
		if b == '$' && i+2 < len(src) && src[i+1] == '(' && src[i+2] == '(' {
			invalidateActivePattern(frame, i)
			frame.arithmeticDepth = 2
			i += 2
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if frame.conditional == nil && b == '(' && i+1 < len(src) && src[i+1] == '(' {
			frame.arithmeticDepth = 2
			i++
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if b == '$' && i+1 < len(src) && src[i+1] == '(' {
			invalidateActivePattern(frame, i)
			frame.atWordStart = false
			frame.atCommandStart = false
			frames = append(frames, newActiveSourceFrame(activeSourceCommandSubstitutionFrame, frame))
			i++
			continue
		}
		if (b == '<' || b == '>' || b == '=') && i+1 < len(src) && src[i+1] == '(' {
			invalidateActivePattern(frame, i)
			frame.atWordStart = false
			frame.atCommandStart = false
			frames = append(frames, newActiveSourceFrame(activeSourceProcessSubstitutionFrame, frame))
			i++
			continue
		}
		if currentCase != nil && currentCase.phase == activeCasePattern {
			if currentCase.inBracketPattern {
				if b == ']' {
					currentCase.inBracketPattern = false
				}
				frame.atWordStart = false
				frame.atCommandStart = false
				continue
			}
			switch b {
			case '[':
				currentCase.inBracketPattern = true
				frame.atWordStart = false
				frame.atCommandStart = false
				continue
			case '(':
				currentCase.patternParenDepth++
				frame.atWordStart = false
				frame.atCommandStart = false
				continue
			case ')':
				if currentCase.patternParenDepth > 0 {
					currentCase.patternParenDepth--
					frame.atWordStart = false
					frame.atCommandStart = false
				} else {
					currentCase.phase = activeCaseBody
					frame.atWordStart = true
					frame.atCommandStart = true
				}
				continue
			}
			if shellSpace(b) {
				frame.atWordStart = true
				if b == '\n' {
					frame.atCommandStart = true
				}
			} else {
				frame.atWordStart = false
				frame.atCommandStart = false
			}
			continue
		}
		if b == '(' {
			frame.groupDepth++
			if frame.conditional != nil {
				frame.atWordStart = false
				frame.atCommandStart = false
			} else {
				frame.atWordStart = true
				frame.atCommandStart = true
			}
			continue
		}
		if b == ')' {
			if frame.groupDepth > 0 {
				frame.groupDepth--
				frame.atWordStart = false
				frame.atCommandStart = false
				continue
			}
			if frame.kind == activeSourceRootFrame {
				frame.atWordStart = false
				frame.atCommandStart = false
				continue
			}
			if !activeSourceFrameCanClose(frame) {
				return nil, nil, false
			}
			adaptedPattern := frame.adaptedPattern
			frames = frames[:len(frames)-1]
			if adaptedPattern {
				frames[len(frames)-1].adaptedPattern = true
			}
			continue
		}
		if b == '#' && frame.atWordStart && frame.conditional == nil {
			frame.inComment = true
			continue
		}
		if frame.conditional == nil && b == '<' && i+1 < len(src) && src[i+1] == '<' {
			if i+2 < len(src) && src[i+2] == '<' {
				i += 2
				frame.atWordStart = true
				continue
			}
			stripTabs := i+2 < len(src) && src[i+2] == '-'
			delimiterStart := i + 2
			if stripTabs {
				delimiterStart++
			}
			delimiter, end, ok := parseHeredocDelimiter(src, delimiterStart)
			if !ok {
				return nil, nil, false
			}
			frame.heredocs = append(frame.heredocs, pendingHeredoc{
				delimiter: delimiter,
				stripTabs: stripTabs,
			})
			i = end - 1
			frame.atWordStart = false
			continue
		}
		if frame.conditional == nil && b == '[' && i+1 < len(src) && src[i+1] == '[' {
			frame.conditional = &activeConditionalContext{start: i}
			i++
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if frame.conditional != nil && b == ']' && i+1 < len(src) && src[i+1] == ']' {
			finalizeActivePattern(frame, i, &candidates)
			frame.conditional = nil
			i++
			frame.atWordStart = false
			frame.atCommandStart = false
			continue
		}
		if shellSpace(b) {
			frame.atWordStart = true
			if b == '\n' {
				frame.atCommandStart = true
			}
			continue
		}
		switch b {
		case ';', '&', '|':
			frame.atWordStart = true
			frame.atCommandStart = true
		default:
			frame.atWordStart = false
			frame.atCommandStart = false
		}
	}

	if len(frames) == 1 && frames[0].conditional != nil {
		finalizeActivePattern(&frames[0], len(src), &candidates)
	}
	if len(frames) != 1 || !activeSourceRootFrameComplete(&frames[0]) {
		return nil, nil, false
	}
	islands, ok := affectedLegacyBacktickIslands(backtickRoots)
	if !ok {
		return nil, nil, false
	}
	return candidates, islands, true
}

func activePatternOperatorEnd(src []byte, start int) (int, bool) {
	if start < 0 || start >= len(src) {
		return 0, false
	}
	if start+2 <= len(src) {
		operator := string(src[start : start+2])
		if (operator == "==" || operator == "!=") && separated(src, start, start+2) {
			return start + 2, true
		}
	}
	if src[start] == '=' && (start+1 >= len(src) || src[start+1] != '~') &&
		separated(src, start, start+1) {
		return start + 1, true
	}
	return 0, false
}

func activePatternByteConsumed(
	frame *activeSourceFrame,
	src []byte,
	offset int,
	seedOffset int,
	candidates *[]patternCandidate,
) bool {
	pattern := frame.conditional.pattern
	b := src[offset]
	if offset < pattern.numericRangeEnd {
		return true
	}
	if pattern.inBracketExpression {
		if b == ']' {
			pattern.inBracketExpression = false
		}
		return true
	}
	if pattern.rhsStart < 0 && shellSpace(b) {
		return false
	}
	if len(pattern.openings) == 0 {
		if shellSpace(b) ||
			(offset+1 < len(src) &&
				(string(src[offset:offset+2]) == "&&" ||
					string(src[offset:offset+2]) == "||" ||
					string(src[offset:offset+2]) == "]]")) {
			finalizeActivePattern(frame, offset, candidates)
			return false
		}
	}
	if pattern.rhsStart < 0 {
		pattern.rhsStart = offset
	}
	if b == '[' {
		pattern.inBracketExpression = true
		return true
	}
	if b == '<' {
		if end, ok := activeNumericRangeEnd(src, offset); ok {
			pattern.numericRangeEnd = end
			return true
		}
	}
	switch b {
	case '&', ';', '<', '>':
		pattern.invalid = true
	case '(':
		pattern.openings = append(pattern.openings, offset)
	case ')':
		if len(pattern.openings) == 0 {
			pattern.invalid = true
			break
		}
		depth := len(pattern.openings)
		open := pattern.openings[depth-1]
		pattern.openings = pattern.openings[:depth-1]
		if depth >= 2 {
			pattern.pairs = append(pattern.pairs, patternPair{
				open:  open,
				close: offset,
				depth: depth,
			})
		}
	case '|':
		if len(pattern.openings) > 0 {
			pattern.nestedAlternation = true
			if offset == seedOffset {
				pattern.seed = true
			}
		}
	}
	return false
}

func activeNumericRangeEnd(src []byte, start int) (int, bool) {
	if start < 0 || start >= len(src) || src[start] != '<' {
		return 0, false
	}
	end := start + 1
	for end < len(src) && src[end] >= '0' && src[end] <= '9' {
		end++
	}
	if end >= len(src) || src[end] != '-' {
		return 0, false
	}
	end++
	for end < len(src) && src[end] >= '0' && src[end] <= '9' {
		end++
	}
	if end >= len(src) || src[end] != '>' {
		return 0, false
	}
	return end + 1, true
}

func finalizeActivePattern(
	frame *activeSourceFrame,
	rhsEnd int,
	candidates *[]patternCandidate,
) {
	if frame.conditional == nil || frame.conditional.pattern == nil {
		return
	}
	pattern := frame.conditional.pattern
	frame.conditional.pattern = nil
	if pattern.invalid || pattern.rhsStart < 0 || pattern.inBracketExpression ||
		len(pattern.openings) != 0 || !pattern.nestedAlternation || len(pattern.pairs) == 0 {
		return
	}
	edits := make([]patternEdit, 0, len(pattern.pairs)*2)
	for _, pair := range pattern.pairs {
		edits = append(edits,
			patternEdit{offset: pair.open, original: '(', replacement: nestedPatternMask},
			patternEdit{offset: pair.close, original: ')', replacement: nestedPatternMask},
		)
	}
	*candidates = append(*candidates, patternCandidate{
		conditionalStart: frame.conditional.start,
		rhsStart:         pattern.rhsStart,
		rhsEnd:           rhsEnd,
		edits:            edits,
		seed:             pattern.seed,
	})
	frame.adaptedPattern = true
}

func invalidateActivePattern(frame *activeSourceFrame, offset int) {
	if frame.conditional == nil || frame.conditional.pattern == nil {
		return
	}
	pattern := frame.conditional.pattern
	if pattern.rhsStart < 0 {
		pattern.rhsStart = offset
	}
	pattern.invalid = true
}

func activeSourceBackslashEscapes(frame *activeSourceFrame, src []byte, offset int) bool {
	if offset+1 >= len(src) {
		return false
	}
	if frame.kind != activeSourceBacktickSubstitutionFrame {
		return true
	}
	switch src[offset+1] {
	case '$', '`', '\\', '\n':
		return true
	default:
		return false
	}
}

func activeSourceDoubleQuoteEscapes(src []byte, offset int) bool {
	if offset+1 >= len(src) {
		return false
	}
	switch src[offset+1] {
	case '$', '`', '"', '\\', '\n':
		return true
	default:
		return false
	}
}

func pushLegacyBacktickFrame(
	frames *[]activeSourceFrame,
	roots *[]*legacyBacktickSpan,
	src []byte,
	openOffset int,
	doubleQuotedParent bool,
	inspect activeLegacyLookupProbe,
) {
	_, delimiterEscapes := legacyBacktickEscapeStateBefore(src, openOffset)
	parentFrame := &(*frames)[len(*frames)-1]
	if inspect != nil {
		inspect()
	}
	parentSpan := parentFrame.legacyBacktick
	parentDepth := parentFrame.legacyBacktickDepth
	span := &legacyBacktickSpan{
		openStart:          openOffset - delimiterEscapes,
		openOffset:         openOffset,
		closeStart:         -1,
		closeOffset:        -1,
		doubleQuotedParent: doubleQuotedParent,
	}
	if parentSpan != nil {
		parentSpan.children = append(parentSpan.children, span)
	} else {
		*roots = append(*roots, span)
	}
	frame := newActiveSourceFrame(activeSourceBacktickSubstitutionFrame, parentFrame)
	frame.legacyBacktick = span
	frame.legacyBacktickDepth = parentDepth + 1
	*frames = append(*frames, frame)
}

func activeLegacyBacktickDepth(frame *activeSourceFrame, inspect activeLegacyLookupProbe) int {
	if inspect != nil {
		inspect()
	}
	return frame.legacyBacktickDepth
}

func legacyBacktickEscapeStateBefore(src []byte, offset int) (depth, delimiterEscapes int) {
	rawBackslashes := 0
	for index := offset - 1; index >= 0 && src[index] == '\\'; index-- {
		rawBackslashes++
	}
	// Each legacy-backtick nesting layer consumes one quoting backslash after
	// escaped-backslash pairs have collapsed. The selected parser records that
	// surviving layer count in lastBquoteEsc and compares it with openBquotes.
	// In the raw run this is the number of trailing one bits: even runs leave an
	// unescaped delimiter, while 1, 3, and 7 backslashes encode depths 1, 2, and 3.
	for remaining := rawBackslashes; remaining&1 == 1; remaining >>= 1 {
		depth++
		delimiterEscapes = delimiterEscapes*2 + 1
	}
	return depth, delimiterEscapes
}

func affectedLegacyBacktickIslands(roots []*legacyBacktickSpan) ([]legacyBacktickIsland, bool) {
	islands := make([]legacyBacktickIsland, 0, len(roots))
	previousClose := -1
	for _, root := range roots {
		if root == nil || root.openOffset < 0 || root.closeStart <= root.openOffset ||
			root.closeOffset < root.closeStart || root.openStart < 0 ||
			root.openStart <= previousClose {
			return nil, false
		}
		previousClose = root.closeOffset
		if !root.affected {
			continue
		}
		island := legacyBacktickIsland{root: root}
		if !appendAffectedLegacyClosures(root, &island.markedClosures) {
			return nil, false
		}
		islands = append(islands, island)
	}
	return islands, true
}

func appendAffectedLegacyClosures(span *legacyBacktickSpan, marked *[]*legacyBacktickSpan) bool {
	if span == nil || span.openOffset < 0 || span.closeStart <= span.openOffset ||
		span.closeOffset < span.closeStart {
		return false
	}
	previousClose := span.openOffset
	for _, child := range span.children {
		if child == nil || child.openStart <= previousClose || child.closeOffset >= span.closeStart ||
			!appendAffectedLegacyClosures(child, marked) {
			return false
		}
		previousClose = child.closeOffset
	}
	if span.affected {
		*marked = append(*marked, span)
	}
	return true
}

func applyLegacyBacktickPlaceholders(
	masked []byte,
	original []byte,
	islands []legacyBacktickIsland,
) error {
	if len(masked) != len(original) {
		return fmt.Errorf("placeholder source length %d does not match original length %d", len(masked), len(original))
	}
	previousClose := -1
	for _, island := range islands {
		root := island.root
		if root == nil || root.openStart <= previousClose || root.openOffset < root.openStart ||
			root.closeStart <= root.openOffset || root.closeOffset < root.closeStart ||
			root.closeOffset >= len(original) {
			return fmt.Errorf("invalid or overlapping legacy backtick island")
		}
		previousClose = root.closeOffset
		preserved := make(map[int]struct{})
		if !markLegacyBacktickDelimiters(root, masked, original, preserved) {
			return fmt.Errorf("legacy backtick delimiter mismatch at byte %d", root.openOffset)
		}
		colonOffset := -1
		for offset := root.openOffset + 1; offset < root.closeStart; offset++ {
			if original[offset] == '\n' {
				masked[offset] = '\n'
				continue
			}
			if _, ok := preserved[offset]; ok {
				masked[offset] = original[offset]
				continue
			}
			masked[offset] = ' '
			if colonOffset < 0 {
				colonOffset = offset
			}
		}
		if colonOffset < 0 {
			return fmt.Errorf("legacy backtick island at byte %d has no usable body byte", root.openOffset)
		}
		masked[colonOffset] = ':'
	}
	return nil
}

func markLegacyBacktickDelimiters(
	span *legacyBacktickSpan,
	masked []byte,
	original []byte,
	preserved map[int]struct{},
) bool {
	if span == nil || span.openStart < 0 || span.openOffset < span.openStart ||
		span.closeStart <= span.openOffset || span.closeOffset < span.closeStart ||
		span.closeOffset >= len(original) || original[span.openOffset] != '`' ||
		original[span.closeOffset] != '`' {
		return false
	}
	for offset := span.openStart; offset <= span.openOffset; offset++ {
		if masked[offset] != original[offset] {
			return false
		}
		preserved[offset] = struct{}{}
	}
	for offset := span.closeStart; offset <= span.closeOffset; offset++ {
		if masked[offset] != original[offset] {
			return false
		}
		preserved[offset] = struct{}{}
	}
	for _, child := range span.children {
		if !markLegacyBacktickDelimiters(child, masked, original, preserved) {
			return false
		}
	}
	return true
}

func newActiveSourceFrame(kind activeSourceFrameKind, parent *activeSourceFrame) activeSourceFrame {
	frame := activeSourceFrame{
		kind:           kind,
		atWordStart:    true,
		atCommandStart: true,
	}
	if parent != nil {
		frame.legacyBacktick = parent.legacyBacktick
		frame.legacyBacktickDepth = parent.legacyBacktickDepth
	}
	return frame
}

func activeSourceFrameCanClose(frame *activeSourceFrame) bool {
	return frame.kind != activeSourceRootFrame &&
		frame.quote == activeSourceUnquoted && !frame.escaped &&
		frame.groupDepth == 0 && frame.arithmeticDepth == 0 &&
		frame.conditional == nil && !frame.inComment &&
		len(frame.heredocs) == 0 && len(frame.caseContexts) == 0
}

func activeSourceRootFrameComplete(frame *activeSourceFrame) bool {
	// Root conditional and grouping balance remains parser-owned so the single
	// compatibility retry can surface a later syntax error. Lexical states that
	// can hide candidate positions still fail closed here.
	return frame.kind == activeSourceRootFrame &&
		frame.quote == activeSourceUnquoted && !frame.escaped &&
		frame.arithmeticDepth == 0 &&
		len(frame.heredocs) == 0 && len(frame.caseContexts) == 0
}

func activeSourceWordAt(src []byte, start int, word string) bool {
	end := start + len(word)
	if start < 0 || end > len(src) || string(src[start:end]) != word {
		return false
	}
	return end == len(src) || activeSourceWordBoundary(src[end])
}

func activeSourceWordBoundary(b byte) bool {
	if shellSpace(b) {
		return true
	}
	switch b {
	case ';', '&', '|', '(', ')', '<', '>':
		return true
	default:
		return false
	}
}

func parseHeredocDelimiter(src []byte, start int) (string, int, bool) {
	for start < len(src) && (src[start] == ' ' || src[start] == '\t') {
		start++
	}
	var delimiter []byte
	consumed := false
	for i := start; i < len(src); {
		b := src[i]
		if heredocWordTerminator(b) {
			if !consumed {
				return "", 0, false
			}
			return string(delimiter), i, true
		}
		if b == '$' && i+1 < len(src) && src[i+1] == '\'' {
			decoded, end, ok := parseANSICQuotedHeredocSegment(src, i)
			if !ok {
				return "", 0, false
			}
			delimiter = append(delimiter, decoded...)
			i = end
			consumed = true
			continue
		}
		switch b {
		case '\\':
			if i+1 >= len(src) || src[i+1] == '\n' {
				return "", 0, false
			}
			delimiter = append(delimiter, src[i+1])
			i += 2
			consumed = true
		case '\'':
			consumed = true
			i++
			for {
				if i >= len(src) || src[i] == '\n' {
					return "", 0, false
				}
				if src[i] == '\'' {
					i++
					break
				}
				delimiter = append(delimiter, src[i])
				i++
			}
		case '"':
			consumed = true
			i++
			for {
				if i >= len(src) || src[i] == '\n' {
					return "", 0, false
				}
				if src[i] == '"' {
					i++
					break
				}
				if src[i] == '\\' && i+1 < len(src) {
					next := src[i+1]
					switch next {
					case '$', '`', '"', '\\':
						delimiter = append(delimiter, next)
						i += 2
						continue
					}
				}
				delimiter = append(delimiter, src[i])
				i++
			}
		default:
			delimiter = append(delimiter, b)
			i++
			consumed = true
		}
	}
	if !consumed {
		return "", 0, false
	}
	return string(delimiter), len(src), true
}

func parseANSICQuotedHeredocSegment(src []byte, start int) ([]byte, int, bool) {
	if start+1 >= len(src) || src[start] != '$' || src[start+1] != '\'' {
		return nil, 0, false
	}
	decoded := make([]byte, 0, 8)
	for i := start + 2; i < len(src); {
		b := src[i]
		if b == '\n' {
			return nil, 0, false
		}
		if b == '\'' {
			return decoded, i + 1, true
		}
		if b != '\\' {
			decoded = append(decoded, b)
			i++
			continue
		}
		if i+1 >= len(src) || src[i+1] == '\n' {
			return nil, 0, false
		}
		escaped := src[i+1]
		if escaped >= '0' && escaped <= '7' {
			value, end, ok := parseANSICDigits(src, i+1, 3, 8)
			if !ok || value == 0 || value > 0xff {
				return nil, 0, false
			}
			decoded = append(decoded, byte(value))
			i = end
			continue
		}
		switch escaped {
		case 'a':
			decoded = append(decoded, '\a')
			i += 2
		case 'b':
			decoded = append(decoded, '\b')
			i += 2
		case 'e', 'E':
			decoded = append(decoded, 0x1b)
			i += 2
		case 'f':
			decoded = append(decoded, '\f')
			i += 2
		case 'n':
			return nil, 0, false
		case 'r':
			decoded = append(decoded, '\r')
			i += 2
		case 't':
			decoded = append(decoded, '\t')
			i += 2
		case 'v':
			decoded = append(decoded, '\v')
			i += 2
		case 'x':
			value, end, ok := parseANSICDigits(src, i+2, 2, 16)
			if !ok || value == 0 {
				return nil, 0, false
			}
			decoded = append(decoded, byte(value))
			i = end
		case 'u', 'U':
			maxDigits := 4
			if escaped == 'U' {
				maxDigits = 8
			}
			value, end, ok := parseANSICDigits(src, i+2, maxDigits, 16)
			r := rune(value)
			if !ok || value == 0 || r == '\n' || !utf8.ValidRune(r) {
				return nil, 0, false
			}
			decoded = utf8.AppendRune(decoded, r)
			i = end
		case 'c', 'C', 'M':
			return nil, 0, false
		default:
			decoded = append(decoded, escaped)
			i += 2
		}
	}
	return nil, 0, false
}

func parseANSICDigits(src []byte, start, maxDigits, base int) (int, int, bool) {
	value := 0
	end := start
	for end < len(src) && end-start < maxDigits {
		digit, ok := ansiCDigitValue(src[end])
		if !ok || digit >= base {
			break
		}
		value = value*base + digit
		end++
	}
	return value, end, end > start
}

func ansiCDigitValue(b byte) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0'), true
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10, true
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10, true
	default:
		return 0, false
	}
}

func heredocWordTerminator(b byte) bool {
	if shellSpace(b) {
		return true
	}
	switch b {
	case ';', '&', '|', '<', '>', '(', ')':
		return true
	default:
		return false
	}
}

func consumeHeredocBodies(src []byte, start int, heredocs []pendingHeredoc) (int, bool) {
	offset := start
	for _, heredoc := range heredocs {
		for {
			if offset > len(src) {
				return 0, false
			}
			newline := bytes.IndexByte(src[offset:], '\n')
			lineEnd := len(src)
			if newline >= 0 {
				lineEnd = offset + newline
			}
			contentStart := offset
			if heredoc.stripTabs {
				for contentStart < lineEnd && src[contentStart] == '\t' {
					contentStart++
				}
			}
			if string(src[contentStart:lineEnd]) == heredoc.delimiter {
				if newline >= 0 {
					offset = lineEnd + 1
				} else {
					offset = lineEnd
				}
				break
			}
			if newline < 0 {
				return 0, false
			}
			offset = lineEnd + 1
		}
	}
	return offset, true
}

func shellSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func separated(src []byte, start, end int) bool {
	return start > 0 && end < len(src) && shellSpace(src[start-1]) && shellSpace(src[end])
}

func lowerBoundPatternEdit(edits []indexedPatternEdit, offset int) (index int, probes int) {
	low, high := 0, len(edits)
	for low < high {
		probes++
		middle := low + (high-low)/2
		if edits[middle].edit.offset < offset {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low, probes
}

func restorePatternEdits(
	tree *syntax.File,
	original []byte,
	edits []patternEdit,
	probes ...patternRestoreProbe,
) error {
	var inspectCandidate patternRestoreProbe
	if len(probes) > 0 {
		inspectCandidate = probes[0]
	}
	indexed := make([]indexedPatternEdit, len(edits))
	for i, edit := range edits {
		indexed[i] = indexedPatternEdit{originalIndex: i, edit: edit}
	}
	sort.Slice(indexed, func(i, j int) bool {
		return indexed[i].edit.offset < indexed[j].edit.offset
	})
	for i := 1; i < len(indexed); i++ {
		if indexed[i-1].edit.offset == indexed[i].edit.offset {
			return fmt.Errorf("duplicate edit offset at byte %d", indexed[i].edit.offset)
		}
	}

	restored := make([]int, len(edits))
	var restoreErr error

	syntax.Walk(tree, func(node syntax.Node) bool {
		if restoreErr != nil {
			return false
		}
		lit, ok := node.(*syntax.Lit)
		if !ok {
			return true
		}
		start := int(lit.ValuePos.Offset())
		end := int(lit.ValueEnd.Offset())
		index, searchProbes := lowerBoundPatternEdit(indexed, start)
		if inspectCandidate != nil {
			for range searchProbes {
				inspectCandidate()
			}
		}
		if index >= len(indexed) || indexed[index].edit.offset >= end {
			return true
		}
		if end-start != len(lit.Value) {
			restoreErr = fmt.Errorf("literal span %d:%d does not map byte-for-byte to its value", start, end)
			return false
		}
		value := []byte(lit.Value)
		for index < len(indexed) && indexed[index].edit.offset < end {
			if inspectCandidate != nil {
				inspectCandidate()
			}
			indexedEdit := indexed[index]
			edit := indexedEdit.edit
			if edit.offset >= len(original) || original[edit.offset] != edit.original {
				restoreErr = fmt.Errorf("original source mismatch at byte %d", edit.offset)
				return false
			}
			relative := edit.offset - start
			if value[relative] != edit.replacement {
				restoreErr = fmt.Errorf("mask mismatch at byte %d", edit.offset)
				return false
			}
			value[relative] = edit.original
			restored[indexedEdit.originalIndex]++
			index++
		}
		lit.Value = string(value)
		return true
	})
	if restoreErr != nil {
		return restoreErr
	}
	for i, count := range restored {
		if count != 1 {
			return fmt.Errorf("edit at byte %d restored %d times", edits[i].offset, count)
		}
	}
	return nil
}
