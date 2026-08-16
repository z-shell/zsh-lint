package parse

import (
	"fmt"

	"mvdan.cc/sh/v3/syntax"
)

func validateConditionalPatterns(src []byte, name string) error {
	scan, ok := scanConditionalPatterns(src, -1, nil)
	if !ok || scan.finding == nil {
		return nil
	}
	pos, ok := sourcePos(src, scan.finding.offset)
	if !ok {
		return fmt.Errorf(
			"%s: conditional-pattern finding outside source at byte %d",
			name,
			scan.finding.offset,
		)
	}
	return syntax.ParseError{
		Filename: name,
		Pos:      pos,
		Text:     scan.finding.text,
	}
}

func sourcePos(src []byte, offset int) (syntax.Pos, bool) {
	if offset < 0 || offset > len(src) {
		return syntax.Pos{}, false
	}
	line, column := uint(1), uint(1)
	for _, b := range src[:offset] {
		if b == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
	}
	return syntax.NewPos(uint(offset), line, column), true
}
