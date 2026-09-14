package parse

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseNativeParameterExpansionCompatibility(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "rc expand caret",
			src:  "print -r -- ${^manpath}\n",
			want: "${^manpath}",
		},
		{
			name: "rc expand caret after split flag with default",
			src:  "for x in {a,i}${(s..)^:-'ab'}; do print -r -- \"$x\"; done\n",
			want: "${(s..)^:-'ab'}",
		},
		{
			name: "rc expand caret after flag with quoted brace",
			src:  "print -r -- ${(x'foo}bar')^name}\n",
			want: "${(x'foo}bar')^name}",
		},
		{
			name: "rc expand caret after multiline flag",
			src:  "print -r -- ${(x\"foo\nbar\")^name}\n",
			want: "${(x\"foo\nbar\")^name}",
		},
		{
			name: "reverse subscript pattern",
			src:  "print -r -- ${_comps[(I)-value-*]}\n",
			want: "[(I)-value-*]",
		},
		{
			name: "forward index subscript pattern",
			src:  "print -r -- ${_comps[(i)-value-*]}\n",
			want: "[(i)-value-*]",
		},
		{
			name: "reverse value subscript pattern",
			src:  "print -r -- ${_comps[(R)-value-*]}\n",
			want: "[(R)-value-*]",
		},
		{
			name: "forward value subscript pattern",
			src:  "print -r -- ${_comps[(r)-value-*]}\n",
			want: "[(r)-value-*]",
		},
		{
			name: "param glob toggle simple",
			src:  "print -r -- ${~pattern}\n",
			want: "${~pattern}",
		},
		{
			name: "param glob toggle in conditional",
			src:  "[[ \"$str\" == ${~pattern} ]] && print matched\n",
			want: "${~pattern}",
		},
		{
			name: "param glob toggle with flags",
			src:  "print -r -- ${(e)~pattern}\n",
			want: "${(e)~pattern}",
		},
		{
			name: "param glob toggle with modifiers",
			src:  "print -r -- ${~=pattern}\n",
			want: "${~=pattern}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			var rendered bytes.Buffer
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("print AST: %v", err)
			}
			if !strings.Contains(rendered.String(), test.want) {
				t.Errorf("printed AST = %q, want original expansion %q", rendered.String(), test.want)
			}
		})
	}
}

func TestParameterCompatibilityPreservesLaterErrorPosition(t *testing.T) {
	for _, src := range []string{
		"print -r -- ${^manpath}\n)\n",
		"print -r -- ${_comps[(I)-value-*]}\n)\n",
		"print -r -- ${~pattern}\n)\n",
	} {
		_, err := Parse(strings.NewReader(src), "later-error.zsh")
		if err == nil {
			t.Fatal("Parse() unexpectedly accepted a trailing unmatched parenthesis")
		}
		var parseErr syntax.ParseError
		if !errors.As(err, &parseErr) {
			t.Fatalf("error type = %T, want syntax.ParseError: %v", err, err)
		}
		if parseErr.Pos.Line() != 2 || parseErr.Pos.Col() != 1 {
			t.Errorf("error position = %d:%d, want 2:1", parseErr.Pos.Line(), parseErr.Pos.Col())
		}
	}
}

func TestParameterCompatibilityRejectsMalformedForms(t *testing.T) {
	for _, src := range []string{
		"print -r -- ${^manpath\n",
		"print -r -- ${(s..)^manpath\n",
		"print -r -- ${manpath^}\n",
		"print -r -- ${(xfoo\\)bar)^name}\n",
		"print -r -- ${(x\"foo)bar\")^name}\n",
		"print -r -- ${_comps[(I)-value-*}\n",
		"print -r -- ${~pattern\n",
	} {
		if _, err := Parse(strings.NewReader(src), "malformed.zsh"); err == nil {
			t.Fatalf("Parse(%q) unexpectedly succeeded", src)
		}
	}
}

func TestIsRCExpandCaretCandidate(t *testing.T) {
	// The flag group ends at the first `)`, exactly as mvdan/sh reads it.
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "direct rc expand", src: "${^name}", want: true},
		{name: "flagged rc expand", src: "${(s..)^:-'ab'}", want: true},
		{name: "empty flag group", src: "${()^name}", want: true},
		{name: "brace inside flags", src: "${(x'foo}bar')^name}", want: true},
		{name: "newline inside flags", src: "${(x\"foo\nbar\")^name}", want: true},
		{name: "open paren inside flags", src: "${(x(y)^name}", want: true},
		{name: "caret after name", src: "${name^}", want: false},
		{name: "caret after close paren outside flags", src: "$(name)^", want: false},
		{name: "close paren inside flags", src: "${(xfoo\\)bar)^name}", want: false},
		{name: "nested close paren before caret", src: "${(s..)foo$(echo hi)^}", want: false},
		{name: "flag group not at expansion start", src: "${x(y)^name}", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			offset := strings.IndexByte(test.src, '^')
			if offset < 0 {
				t.Fatalf("test case %q has no caret", test.src)
			}
			if got := isRCExpandCaretCandidate([]byte(test.src), offset); got != test.want {
				t.Fatalf("isRCExpandCaretCandidate(%q) = %v, want %v", test.src, got, test.want)
			}
		})
	}
}
