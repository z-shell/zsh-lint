package parse

import (
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParseFunctionNameExpansion(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantName string
		rsrvWord bool
		parens   bool
	}{
		{
			name:     "paren definition with braced parameter expansion",
			src:      "_w_${cur}() { :; }\n",
			wantName: "_w_${cur}",
			rsrvWord: false,
			parens:   true,
		},
		{
			name:     "paren definition with bare parameter expansion",
			src:      "_w_$cur() { :; }\n",
			wantName: "_w_$cur",
			rsrvWord: false,
			parens:   true,
		},
		{
			name:     "function keyword definition with braced expansion",
			src:      "function _w_${cur} { :; }\n",
			wantName: "_w_${cur}",
			rsrvWord: true,
			parens:   false,
		},
		{
			name:     "function keyword definition with bare parameter",
			src:      "function $x { :; }\n",
			wantName: "$x",
			rsrvWord: true,
			parens:   false,
		},
		{
			name:     "parameter expansion alone with parens",
			src:      "${x}() { :; }\n",
			wantName: "${x}",
			rsrvWord: false,
			parens:   true,
		},
		{
			name:     "command substitution name with parens",
			src:      "$(x)() { :; }\n",
			wantName: "$(x)",
			rsrvWord: false,
			parens:   true,
		},
		{
			name:     "quoted name with parens",
			src:      "\"a\"() { :; }\n",
			wantName: "\"a\"",
			rsrvWord: false,
			parens:   true,
		},
		{
			name:     "function keyword definition with parens and expansion",
			src:      "function _w_${cur}() { :; }\n",
			wantName: "_w_${cur}",
			rsrvWord: true,
			parens:   true,
		},
		{
			name:     "F-Sy-H widget loop pattern",
			src:      "_fsh_widget_${cur_widget}() { :; _fsh_zle_highlight }\n",
			wantName: "_fsh_widget_${cur_widget}",
			rsrvWord: false,
			parens:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(tc.src), tc.name+".zsh")
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tc.src, err)
			}
			if len(file.AST().Stmts) == 0 {
				t.Fatal("len(Stmts) = 0, want at least 1")
			}
			decl, ok := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl)
			if !ok {
				t.Fatalf("Cmd is not *syntax.FuncDecl: %T", file.AST().Stmts[0].Cmd)
			}
			if decl.Name == nil {
				t.Fatal("decl.Name is nil, want *syntax.Lit")
			}
			if decl.Name.Value != tc.wantName {
				t.Errorf("decl.Name.Value = %q, want %q", decl.Name.Value, tc.wantName)
			}
			if decl.RsrvWord != tc.rsrvWord {
				t.Errorf("decl.RsrvWord = %v, want %v", decl.RsrvWord, tc.rsrvWord)
			}
			if decl.Parens != tc.parens {
				t.Errorf("decl.Parens = %v, want %v", decl.Parens, tc.parens)
			}
			if decl.Body == nil {
				t.Error("decl.Body is nil, want statement body")
			}
		})
	}
}

func TestParseFunctionNameExpansionPositions(t *testing.T) {
	const src = "_w_${cur}() { :; }\n"
	file, err := Parse(strings.NewReader(src), "positions.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	decl := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl)
	if decl.Pos().Col() != 1 {
		t.Errorf("decl.Pos().Col() = %d, want 1", decl.Pos().Col())
	}
	if decl.Name.Pos().Col() != 1 {
		t.Errorf("decl.Name.Pos().Col() = %d, want 1", decl.Name.Pos().Col())
	}
	if decl.Name.End().Col() != 10 {
		t.Errorf("decl.Name.End().Col() = %d, want 10", decl.Name.End().Col())
	}
	if decl.Name.Value != "_w_${cur}" {
		t.Errorf("decl.Name.Value = %q, want _w_${cur}", decl.Name.Value)
	}
}

func TestParseFunctionNameExpansionKeywordPositions(t *testing.T) {
	const src = "function _w_${cur} { :; }\n"
	file, err := Parse(strings.NewReader(src), "positions_keyword.zsh")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	decl := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl)
	if decl.Pos().Col() != 1 {
		t.Errorf("decl.Pos().Col() = %d, want 1", decl.Pos().Col())
	}
	if decl.Name.Pos().Col() != 10 {
		t.Errorf("decl.Name.Pos().Col() = %d, want 10", decl.Name.Pos().Col())
	}
	if decl.Name.End().Col() != 19 {
		t.Errorf("decl.Name.End().Col() = %d, want 19", decl.Name.End().Col())
	}
	if decl.Name.Value != "_w_${cur}" {
		t.Errorf("decl.Name.Value = %q, want _w_${cur}", decl.Name.Value)
	}
}

func TestParseInvalidBraceBodyWithoutKeyword(t *testing.T) {
	fixture, err := os.ReadFile("testdata/invalid-234-brace-body-without-keyword.txt")
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	_, err = Parse(strings.NewReader(string(fixture)), "invalid-234-brace-body-without-keyword.zsh")
	if err == nil {
		t.Fatal("Parse() succeeded, want parse error")
	}
	if !strings.Contains(err.Error(), "can only be used to close a block") {
		t.Errorf("err = %q, want closing block error", err.Error())
	}
}

// TestParseFunctionNameStops pins where a Zsh function name ends. Every row
// matches the `zsh -f -n` verdict. The rows once crashed the name loop, which
// read an empty word at a token that cannot start one.
func TestParseFunctionNameStops(t *testing.T) {
	t.Parallel()
	tests := []struct {
		src   string
		names []string
		ok    bool
	}{
		{src: "function foo > out { :; }\n", names: []string{"foo"}, ok: true},
		{src: "function foo >out\n{ :; }\n", names: []string{"foo"}, ok: true},
		{src: "function foo ((x))\n", names: []string{"foo"}, ok: true},
		{src: "function a${x} b { :; }\n", names: []string{"a${x}", "b"}, ok: true},
		{src: "function f$x () { :; }\n", names: []string{"f$x"}, ok: true},
		{src: "f$x(){ print; }\n", names: []string{"f$x"}, ok: true},
		{src: "function 'a b' { :; }\n", names: []string{"'a b'"}, ok: true},
		{src: "function foo &\n", ok: false},
		{src: "function foo(\n", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			t.Parallel()
			file, err := Parse(strings.NewReader(tc.src), "stops.zsh")
			if !tc.ok {
				if err == nil {
					t.Fatalf("Parse(%q) succeeded, want an error as zsh -n gives", tc.src)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tc.src, err)
			}
			decl, ok := file.AST().Stmts[0].Cmd.(*syntax.FuncDecl)
			if !ok {
				t.Fatalf("Cmd is %T, want *syntax.FuncDecl", file.AST().Stmts[0].Cmd)
			}
			var got []string
			if decl.Name != nil {
				got = append(got, decl.Name.Value)
			}
			for _, name := range decl.Names {
				got = append(got, name.Value)
			}
			if strings.Join(got, "|") != strings.Join(tc.names, "|") {
				t.Errorf("names = %q, want %q", got, tc.names)
			}
		})
	}
}
