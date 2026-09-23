package parse

import (
	"bytes"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #362: an associative key holding `--` or `++` is a literal key in
// native Zsh, `g[${o}--$s]` subscripts `g` with the key `x--y`, but mvdan/sh
// reads the subscript as arithmetic and the doubled sign as a decrement or
// increment with nothing it may apply to. It reports that three ways:
//
//   - "`--` must follow a name", a postfix operator after an expansion or a
//     number (arithmExprValue's postfix check),
//   - "`--` must be followed by a literal", a prefix operator before an
//     expansion (arithmExprValue's prefix check),
//   - "not a valid arithmetic operator: `b`", a postfix operator after a name,
//     which it accepts, followed by a word it cannot join.
//
// Each row is valid under `zsh -f -n` and was evaluated against zsh 5.9 to
// confirm the key is the literal text: see the corpus fixture.
func TestParseAssociativeKeyDoubledSign(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		parts []keyPart
	}{
		{
			name:  "braced expansion either side, assignment",
			src:   "g[${o}--$s]=1\n",
			parts: []keyPart{{param: "o", col: 3}, {lit: "--", col: 7}, {param: "s", col: 9}},
		},
		{
			name:  "bare expansion either side, assignment",
			src:   "g[$o--$s]=1\n",
			parts: []keyPart{{param: "o", col: 3}, {lit: "--", col: 5}, {param: "s", col: 7}},
		},
		{
			name:  "bare words, assignment",
			src:   "g[a--b]=1\n",
			parts: []keyPart{{lit: "a--b", col: 3}},
		},
		{
			name:  "braced expansion either side, expansion",
			src:   "print ${g[${o}--$s]}\n",
			parts: []keyPart{{param: "o", col: 11}, {lit: "--", col: 15}, {param: "s", col: 17}},
		},
		{
			name:  "bare words, expansion",
			src:   "print ${g[a--b]}\n",
			parts: []keyPart{{lit: "a--b", col: 11}},
		},
		{
			name:  "increment between expansions",
			src:   "g[$o++$s]=1\n",
			parts: []keyPart{{param: "o", col: 3}, {lit: "++", col: 5}, {param: "s", col: 7}},
		},
		{
			name:  "increment between words, expansion",
			src:   "print ${g[a++b]}\n",
			parts: []keyPart{{lit: "a++b", col: 11}},
		},
		{
			name:  "leading doubled sign before an expansion",
			src:   "g[--$s]=1\n",
			parts: []keyPart{{lit: "--", col: 3}, {param: "s", col: 5}},
		},
		{
			name:  "trailing doubled sign after an expansion",
			src:   "g[${o}--]=1\n",
			parts: []keyPart{{param: "o", col: 3}, {lit: "--", col: 7}},
		},
		{
			name:  "number before the doubled sign",
			src:   "g[1--x]=1\n",
			parts: []keyPart{{lit: "1--x", col: 3}},
		},
		{
			name:  "two doubled signs",
			src:   "g[a--b--c]=1\n",
			parts: []keyPart{{lit: "a--b--c", col: 3}},
		},
		{
			name:  "append assignment, the zi autoload.zsh:2351 shape",
			src:   "owner_to_group[${o}--$s]+=\"$c;\"\n",
			parts: []keyPart{{param: "o", col: 16}, {lit: "--", col: 20}, {param: "s", col: 22}},
		},
		{
			name:  "declaration assignment",
			src:   "typeset g[a--b]=1\n",
			parts: []keyPart{{lit: "a--b", col: 11}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := Parse(strings.NewReader(test.src), test.name+".zsh")
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
// arithmetic keeps its decrement or increment. These must not become keys.
func TestArithmeticSubscriptKeepsDoubledSigns(t *testing.T) {
	for _, src := range []string{
		"a[x--]=1\n",
		"a[--x]=1\n",
		"print ${a[x--]}\n",
		"print ${a[--x]}\n",
		"print ${a[i++]}\n",
		"(( a[x--] ))\n",
		"print $(( x-- ))\n",
		"(( --x ))\n",
		"(( x-- - y ))\n",
	} {
		file, err := Parse(strings.NewReader(src), "arithmetic.zsh")
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", src, err)
		}
		var unary int
		syntax.Walk(file.AST(), func(node syntax.Node) bool {
			if u, ok := node.(*syntax.UnaryArithm); ok && (u.Op == syntax.Inc || u.Op == syntax.Dec) {
				unary++
			}
			return true
		})
		if unary != 1 {
			t.Errorf("Parse(%q) increment/decrement nodes = %d, want 1", src, unary)
		}
	}
}

// The gate ties each error to a doubled sign inside the key the scanner
// recognizes. An error from the same text elsewhere keeps its verdict.
func TestAssociativeKeyDoubledSignGateStaysNarrow(t *testing.T) {
	tests := []struct {
		name string
		src  string
		text string
	}{
		// Arithmetic, not a subscript: no key to recognize. Native Zsh
		// accepts these under -n and fails them at run time; the front
		// end's verdict is unchanged by this fix either way.
		{"decrement then word in an arithmetic command", "(( a--b ))\n", "not a valid arithmetic operator: `b`"},
		{"decrement then word in an arithmetic expansion", "print $(( a--b ))\n", "not a valid arithmetic operator: `b`"},
		// A space ends the bare key, so the scanner declines.
		{"space after the doubled sign", "print ${g[x-- y]}\n", "not a valid arithmetic operator: `y`"},
		// Quotes end the bare key too.
		{"quoted expansion before", "g[\"${o}\"--$s]=1\n", "`--` must follow a name"},
		// An unclosed subscript has no key to recognize.
		{"unclosed subscript", "print ${g[a--b}\n", "not a valid arithmetic operator: `b`"},
		// Valid Zsh, but an expansion followed by `@` is issue #253's
		// shape, not a doubled sign. The postfix-operand gate must not
		// claim it by accident; #253's fix will move this row.
		{"expansion then at sign, issue #253", "(( g[a.$b@] ))\n", "not a valid arithmetic operator: `@`"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(test.src), "narrow.zsh")
			if err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", test.src)
			}
			if !strings.Contains(err.Error(), test.text) {
				t.Errorf("Parse(%q) error = %v, want text %q", test.src, err, test.text)
			}
		})
	}
}

// The postfix error names the word after the operator, so the gate checks
// the two bytes before it are the doubled sign. A single sign before the word
// must not pass as one.
func TestSubscriptDoubledSignHelpers(t *testing.T) {
	if sign, ok := subscriptDoubledSignError("`--` must follow a name"); !ok || sign != '-' {
		t.Errorf("postfix decrement = %q, %v; want '-', true", sign, ok)
	}
	if sign, ok := subscriptDoubledSignError("`++` must be followed by a literal"); !ok || sign != '+' {
		t.Errorf("prefix increment = %q, %v; want '+', true", sign, ok)
	}
	for _, text := range []string{
		"`--` must follow a name like a[i]",
		"`-` must follow a name",
		"`--` must be followed by an expression",
		"`**` must follow a name",
	} {
		if _, ok := subscriptDoubledSignError(text); ok {
			t.Errorf("subscriptDoubledSignError(%q) accepted", text)
		}
	}

	src := []byte("g[a--b]=1 h[a-b]=1 k[a-+b]=1 m[a..b]=1 --n[a]=1")
	key := func(open int) associativeKey {
		k, ok := scanBareAssociativeKey(src, open)
		if !ok {
			t.Fatalf("scanBareAssociativeKey(%d) declined", open)
		}
		return k
	}
	if !isDoubledSignAt(src, key(1), 3) {
		t.Error("`--` in g[a--b] not recognized")
	}
	if isDoubledSignAt(src, key(11), 13) {
		t.Error("single `-` in h[a-b] recognized as doubled")
	}
	if isDoubledSignAt(src, key(20), 22) {
		t.Error("mixed `-+` in k[a-+b] recognized as doubled")
	}
	// `.` is key punctuation too, but only a sign doubles into an operator.
	if isDoubledSignAt(src, key(30), 32) {
		t.Error("`..` in m[a..b] recognized as a doubled sign")
	}
	// A doubled sign outside the key, before `n`, is not the key's.
	if isDoubledSignAt(src, key(42), 39) {
		t.Error("`--` before the key recognized as the key's")
	}
	if isDoubledSignAt(src, key(1), -1) || isDoubledSignAt(src, key(1), len(src)-1) {
		t.Error("out-of-range offset recognized")
	}
}
