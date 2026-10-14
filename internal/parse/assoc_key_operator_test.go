package parse

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #691: an associative key holding `%`, `*`, `?`, `~`, `,` or `=` is
// a literal key in native Zsh, `z[%]+=x` appends to the key `%`
// and `e[keycap_*]=1` sets the key `keycap_*`, but mvdan/sh reads the
// subscript as arithmetic and the byte as an operator with a missing
// operand. It reports that five ways:
//
//   - "`%` must follow an expression" or "`*` must be followed by an
//     expression", an operator without a left or right operand,
//   - "`=` must follow a name", an assignment with no name before it,
//   - "not a valid arithmetic operator: `a`", a word after a leading `*`,
//     which the subscript reader takes as the whole-array `*`,
//   - "ternary operator missing `:` after `?`", a `?` with no `:`,
//   - "reached `~` without matching `[` with `]`", a `~` after an operand,
//     where only a binary operator may follow; it is reported at the `[`.
//
// Each row is valid under `zsh -f -n`, and the corpus fixture runs the same
// keys under Zsh to confirm the key is the literal text.
func TestParseAssociativeKeyOperatorBytes(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		parts []keyPart
	}{
		{
			name:  "lone percent, append assignment, define-composed-chars:207",
			src:   "z[%]+=\" x\"\n",
			parts: []keyPart{{lit: "%", col: 3}},
		},
		{
			name:  "trailing percent",
			src:   "z[a%]=1\n",
			parts: []keyPart{{lit: "a%", col: 3}},
		},
		{
			name:  "leading percent",
			src:   "z[%a]=1\n",
			parts: []keyPart{{lit: "%a", col: 3}},
		},
		{
			name:  "trailing star, emoji-char-definitions.zsh:4086",
			src:   "emoji[keycap_*]=1\n",
			parts: []keyPart{{lit: "keycap_*", col: 7}},
		},
		{
			name:  "leading star",
			src:   "e[*a]=1\n",
			parts: []keyPart{{lit: "*a", col: 3}},
		},
		{
			name:  "trailing question mark",
			src:   "e[a?]=1\n",
			parts: []keyPart{{lit: "a?", col: 3}},
		},
		{
			name:  "lone question mark",
			src:   "e[?]=1\n",
			parts: []keyPart{{lit: "?", col: 3}},
		},
		{
			name:  "leading question mark",
			src:   "e[?a]=1\n",
			parts: []keyPart{{lit: "?a", col: 3}},
		},
		{
			name:  "question mark without a colon",
			src:   "e[a?b]=1\n",
			parts: []keyPart{{lit: "a?b", col: 3}},
		},
		{
			name:  "lone tilde",
			src:   "e[~]=1\n",
			parts: []keyPart{{lit: "~", col: 3}},
		},
		{
			name:  "trailing tilde",
			src:   "e[a~]=1\n",
			parts: []keyPart{{lit: "a~", col: 3}},
		},
		{
			name:  "tilde between words",
			src:   "e[a~b]=1\n",
			parts: []keyPart{{lit: "a~b", col: 3}},
		},
		{
			name:  "lone comma, append assignment, define-composed-chars:381",
			src:   "z[,]+=\" ' B8\"\n",
			parts: []keyPart{{lit: ",", col: 3}},
		},
		{
			name:  "trailing comma",
			src:   "e[a,]=1\n",
			parts: []keyPart{{lit: "a,", col: 3}},
		},
		{
			name:  "leading comma",
			src:   "e[,a]=1\n",
			parts: []keyPart{{lit: ",a", col: 3}},
		},
		{
			name:  "lone equals sign, append assignment, define-composed-chars:424",
			src:   "z[=]+=\" / 2021\"\n",
			parts: []keyPart{{lit: "=", col: 3}},
		},
		{
			name:  "trailing equals sign",
			src:   "e[a=]=1\n",
			parts: []keyPart{{lit: "a=", col: 3}},
		},
		{
			name:  "leading equals sign",
			src:   "e[=a]=1\n",
			parts: []keyPart{{lit: "=a", col: 3}},
		},
		{
			name:  "percent in a braced expansion",
			src:   "print ${j[%]}\n",
			parts: []keyPart{{lit: "%", col: 11}},
		},
		{
			name:  "percent in a short expansion",
			src:   "print $j[%]\n",
			parts: []keyPart{{lit: "%", col: 10}},
		},
		{
			name:  "star in a braced expansion",
			src:   "print ${e[keycap_*]}\n",
			parts: []keyPart{{lit: "keycap_*", col: 11}},
		},
		{
			name:  "tilde in a braced expansion",
			src:   "print ${e[a~b]}\n",
			parts: []keyPart{{lit: "a~b", col: 11}},
		},
		{
			name:  "percent and question mark before a positional parameter, prompt_bart_setup:91",
			src:   "[[ -n $jobtexts[%?$2] ]]\n",
			parts: []keyPart{{lit: "%?", col: 17}, {param: "2", col: 19}},
		},
		{
			name:  "percent after a braced expansion",
			src:   "print ${e[${k}%]}\n",
			parts: []keyPart{{param: "k", col: 11}, {lit: "%", col: 15}},
		},
		{
			name:  "angle brackets and percent in a braced expansion",
			src:   "print ${e[a<%>]}\n",
			parts: []keyPart{{lit: "a<%>", col: 11}},
		},
		{
			name:  "declaration assignment",
			src:   "typeset e[a%]=1\n",
			parts: []keyPart{{lit: "a%", col: 11}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), "key.zsh")
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			index := singleSubscriptIndex(t, file.AST())
			word, ok := index.(*syntax.Word)
			if !ok {
				t.Fatalf("Index = %T, want *syntax.Word (a literal key, not arithmetic)", index)
			}
			if len(word.Parts) != len(test.parts) {
				t.Fatalf("Index parts = %d, want %d", len(word.Parts), len(test.parts))
			}
			for i, want := range test.parts {
				part := word.Parts[i]
				if got := part.Pos().Col(); got != want.col {
					t.Errorf("part %d column = %d, want %d", i, got, want.col)
				}
				if want.lit != "" {
					lit, ok := part.(*syntax.Lit)
					if !ok {
						t.Fatalf("part %d = %T, want *syntax.Lit", i, part)
					}
					if lit.Value != want.lit {
						t.Errorf("part %d literal = %q, want %q", i, lit.Value, want.lit)
					}
					continue
				}
				exp, ok := part.(*syntax.ParamExp)
				if !ok {
					t.Fatalf("part %d = %T, want *syntax.ParamExp", i, part)
				}
				if exp.Param.Value != want.param {
					t.Errorf("part %d parameter = %q, want %q", i, exp.Param.Value, want.param)
				}
			}
			var rendered bytes.Buffer
			if err := syntax.NewPrinter().Print(&rendered, file.AST()); err != nil {
				t.Fatalf("print AST: %v", err)
			}
			if got := strings.TrimSuffix(rendered.String(), "\n"); got != strings.TrimSuffix(test.src, "\n") {
				t.Errorf("printed AST = %q, want the original source", got)
			}
		})
	}
}

// The retry only runs after an error, so a subscript that parses as
// arithmetic keeps its operators. These must not become keys. The front end
// does not know a parameter's type, so a key that is also valid arithmetic,
// as `emoji[keycap_*_unqualified]` at emoji-char-definitions.zsh:4088 is,
// stays arithmetic too, as `e[a-b]` already does.
func TestArithmeticSubscriptKeepsOperatorBytes(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"e[keycap_*_unqualified]=1\n", "*syntax.BinaryArithm"},
		{"a[i%2]=1\n", "*syntax.BinaryArithm"},
		{"print ${a[i*2]}\n", "*syntax.BinaryArithm"},
		{"print $a[i*2]\n", "*syntax.BinaryArithm"},
		{"print ${a[i?1:2]}\n", "*syntax.BinaryArithm"},
		{"print ${a[~i]}\n", "*syntax.UnaryArithm"},
		{"print ${a[!i]}\n", "*syntax.UnaryArithm"},
		{"print ${a[i,2]}\n", "*syntax.BinaryArithm"},
		{"a[i=2]=1\n", "*syntax.BinaryArithm"},
		{"(( a[i%2] ))\n", "*syntax.BinaryArithm"},
	}
	for _, test := range tests {
		file, err := Parse(strings.NewReader(test.src), "arithmetic.zsh")
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", test.src, err)
		}
		index := singleSubscriptIndex(t, file.AST())
		if got := fmt.Sprintf("%T", index); got != test.want {
			t.Errorf("Parse(%q) index = %s, want %s", test.src, got, test.want)
		}
	}
}

// The gate ties each error to an operator byte inside the key the scanner
// recognizes. The same error anywhere else keeps its verdict.
func TestAssociativeKeyOperatorGateStaysNarrow(t *testing.T) {
	tests := []struct {
		name string
		src  string
		text string
	}{
		// Arithmetic, not a subscript: no key to recognize. Native Zsh
		// accepts these under -n and fails them at run time.
		{"percent in an arithmetic command", "(( a % ))\n", "`%` must be followed by an expression"},
		{"question mark in an arithmetic expansion", "print $(( a ? b ))\n", "ternary operator missing `:` after `?`"},
		{"tilde after an operand in an arithmetic command", "(( a ~ b ))\n", "not a valid arithmetic operator: `~`"},
		// Quotes end the bare key, so the scanner declines.
		{"quoted word before the percent", "e[\"a\"%]=1\n", "`%` must be followed by an expression"},
		// A key without a name before its `[` is not a subscript.
		{"percent in a glob bracket", "print [%]\n", ""},
		{"equals sign without a name in an arithmetic command", "(( 1 = 2 ))\n", "`=` must follow a name"},
		// `^` and `!` stay outside this fix, since they open a negated
		// bracket expression that the pattern-after-comma adapter reads
		// (#277); so do `&`, `|` and `;`, which end a word outside a
		// subscript. Native Zsh accepts every row here.
		{"caret alone, not in the key set", "e[^]=1\n", "`^` must follow an expression"},
		// Zsh can end an unbraced word at `<` or `>`: `print $e[a<%>]` is
		// `print $e[a` with two redirections, which Zsh rejects at the top
		// level (`invalid subscript`) and in a conditional (`parse error
		// near <`), and `e[%>=]=1` redirects to `=]=1`. The retry keeps
		// such a key rejected outside `${...}`, `e[a<%>]=1` included,
		// although Zsh accepts that one.
		{"short expansion with angle brackets and percent", "print $e[a<%>]\n", "`%` must follow an expression"},
		{"short expansion with a trailing angle bracket", "print $e[a%<]\n", "`%` must be followed by an expression"},
		{"short expansion with angle brackets in a conditional", "[[ -n $e[a<%>] ]]\n", "`%` must follow an expression"},
		{"assignment with a redirection-like key", "e[%>=]=1\n", "`%` must follow an expression"},
		{"assignment with angle brackets and percent", "e[a<%>]=1\n", "`%` must follow an expression"},
		{"exclamation mark, not in the key set", "e[a!b]=1\n", "reached `!` without matching `[` with `]`"},
		{"ampersand, not in the key set", "e[&]=1\n", "`&` must follow an expression"},
		{"vertical bar, not in the key set", "e[a|]=1\n", "`|` must be followed by an expression"},
		{"semicolon, not in the key set", "e[a;b]=1\n", "reached `;` without matching `[` with `]`"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.src), "narrow.zsh")
			if test.text == "" {
				if err != nil {
					t.Fatalf("Parse(%q) error: %v", test.src, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", test.src)
			}
			if !strings.Contains(err.Error(), test.text) {
				t.Errorf("Parse(%q) error = %v, want text %q", test.src, err, test.text)
			}
		})
	}
}

func TestSubscriptOperatorErrorHelper(t *testing.T) {
	for _, test := range []struct {
		text string
		op   byte
		ok   bool
	}{
		{"`=` must follow a name", '=', true},
		{"`%` must follow an expression", '%', true},
		{"`,` must be followed by an expression", ',', true},
		{"`-` must follow a name", 0, false},
		{"`=` must be followed by a literal", 0, false},
		{"`&` must follow an expression", 0, false},
		{"`^` must follow an expression", 0, false},
	} {
		if op, ok := subscriptOperatorError(test.text); op != test.op || ok != test.ok {
			t.Errorf("subscriptOperatorError(%q) = %q, %v; want %q, %v", test.text, op, ok, test.op, test.ok)
		}
	}
}

func TestUnbracedAngleKeyWithOperatorByte(t *testing.T) {
	for _, test := range []struct {
		src  string
		want bool
	}{
		{"$e[a<%]", true},
		{"$e[a>,]", true},
		{"$e[?>b]", true},
		{"x$e[a=<]", true},
		{"${e[a<%]}", false},
		{"e[a<%]", true},
		{"$e[a<b]", false},
		{"$e[a%]", false},
		{"$e[a<.]", false},
		{"e[a>*]", true},
		{"${(k)e[a>*]}", false},
	} {
		open := strings.IndexByte(test.src, '[')
		key, ok := scanBareAssociativeKey([]byte(test.src), open)
		if !ok {
			t.Fatalf("scanBareAssociativeKey(%q) declined", test.src)
		}
		if got := isUnbracedAngleKeyWithOperatorByte([]byte(test.src), key); got != test.want {
			t.Errorf("isUnbracedAngleKeyWithOperatorByte(%q) = %v, want %v", test.src, got, test.want)
		}
	}
}
