package parse

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// restoreLengthPrefix returns after its first match, so a line holding two
// masked expansions is the shape most likely to leave one unrestored. The
// chain re-enters on each parse error, so the second should be fixed by a
// later pass -- this asserts it, rather than trusting the exit code.
func TestLengthOperatorRestoresEveryExpansion(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		src       string
		wantNames []string
	}{
		{"two on one line", "print ${#a:#x} ${#b:#y}\n", []string{"a", "b"}},
		{"two with subscripts", "print ${#a[@]:#x} ${#b[@]:#y}\n", []string{"a", "b"}},
		{"three concatenated", "print ${#a:#x}${#b:#y}${#c:#z}\n", []string{"a", "b", "c"}},
		{"inside an assignment", "x=${#a:#p}${#b:#q}\n", []string{"a", "b"}},
		{"inside double quotes", "print \"${#a:#x}${#b:#y}\"\n", []string{"a", "b"}},
		{"one nested in the other's word", "print ${#a:-${#b:-z}}\n", []string{"a", "b"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tree, err := parseWithAdapters([]byte(test.src), "t.zsh")
			if err != nil {
				t.Fatalf("parse %q: %v", test.src, err)
			}
			var names []string
			syntax.Walk(tree, func(node syntax.Node) bool {
				exp, ok := node.(*syntax.ParamExp)
				if !ok || exp.Param == nil {
					return true
				}
				if !exp.Length {
					t.Errorf("expansion %q has Length = false, want true", exp.Param.Value)
				}
				if strings.HasPrefix(exp.Param.Value, "_") {
					t.Errorf("name %q still carries the mask byte", exp.Param.Value)
				}
				names = append(names, exp.Param.Value)
				return true
			})
			if len(names) != len(test.wantNames) {
				t.Fatalf("names = %v, want %v", names, test.wantNames)
			}
			for i, want := range test.wantNames {
				if names[i] != want {
					t.Errorf("name[%d] = %q, want %q", i, names[i], want)
				}
			}
			// The printer reproduces the source only when every tree
			// node matches it, which is the end-to-end proof that no
			// mask byte survived anywhere.
			var sb strings.Builder
			if err := syntax.NewPrinter().Print(&sb, tree); err != nil {
				t.Fatalf("print: %v", err)
			}
			if sb.String() != test.src {
				t.Errorf("round trip = %q, want %q", sb.String(), test.src)
			}
		})
	}
}

// Positions reported to users must match the ORIGINAL source, because the
// mask is byte-preserving. A construct placed after a fixed expansion is the
// direct check: its column is computed from the source string itself, so the
// assertion cannot drift with the implementation.
func TestLengthOperatorKeepsLaterPositions(t *testing.T) {
	t.Parallel()
	const trailer = "$UNSET_XYZ"
	for _, src := range []string{
		"print ${#a[@]:#x} " + trailer + "\n",
		"print ${#a:#x} " + trailer + "\n",
		"print ${#a//x/y} " + trailer + "\n",
		"print ${#a:-y} " + trailer + "\n",
		"print ${#a:#x} ${#b:#y} " + trailer + "\n",
		// The already-working nested spelling, as the control.
		"print ${#${a[@]:#x}} " + trailer + "\n",
	} {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			t.Parallel()
			tree, err := parseWithAdapters([]byte(src), "t.zsh")
			if err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			wantOffset := strings.Index(src, trailer)
			found := false
			syntax.Walk(tree, func(node syntax.Node) bool {
				exp, ok := node.(*syntax.ParamExp)
				if !ok || exp.Param == nil || exp.Param.Value != "UNSET_XYZ" {
					return true
				}
				found = true
				if got := int(exp.Pos().Offset()); got != wantOffset {
					t.Errorf("trailing expansion offset = %d, want %d", got, wantOffset)
				}
				if got := int(exp.Pos().Col()); got != wantOffset+1 {
					t.Errorf("trailing expansion column = %d, want %d", got, wantOffset+1)
				}
				return false
			})
			if !found {
				t.Fatalf("trailing expansion not found in %q", src)
			}
		})
	}
}

// A one-character name makes the restored value exactly one byte, which is
// the boundary of the `len(lit.Value) < 2` guard, and a name that already
// starts with `_` is the case where the mask byte is indistinguishable from
// source text by value alone (the offset check is what separates them).
func TestLengthOperatorNameBoundaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		src      string
		wantName string
	}{
		{"print ${#a:#x}\n", "a"},
		{"print ${#_:#x}\n", "_"},
		{"print ${#_a:#x}\n", "_a"},
		{"print ${#__a:#x}\n", "__a"},
		{"print ${#a1:#x}\n", "a1"},
		{"print ${#A:#x}\n", "A"},
	} {
		t.Run(strings.TrimSpace(test.src), func(t *testing.T) {
			t.Parallel()
			tree, err := parseWithAdapters([]byte(test.src), "t.zsh")
			if err != nil {
				t.Fatalf("parse %q: %v", test.src, err)
			}
			var got *syntax.ParamExp
			syntax.Walk(tree, func(node syntax.Node) bool {
				if exp, ok := node.(*syntax.ParamExp); ok && got == nil {
					got = exp
				}
				return got == nil
			})
			if got == nil || got.Param == nil {
				t.Fatalf("no named expansion in %q", test.src)
			}
			if got.Param.Value != test.wantName {
				t.Errorf("name = %q, want %q", got.Param.Value, test.wantName)
			}
			if !got.Length {
				t.Error("Length = false, want true")
			}
			var sb strings.Builder
			if err := syntax.NewPrinter().Print(&sb, tree); err != nil {
				t.Fatalf("print: %v", err)
			}
			if sb.String() != test.src {
				t.Errorf("round trip = %q, want %q", sb.String(), test.src)
			}
		})
	}
}

// The flat spelling and the nested spelling mean the same thing in Zsh, and
// the nested one already parsed before this change. Their trees should agree
// on the length, the operator and the pattern.
func TestLengthOperatorAgreesWithNestedSpelling(t *testing.T) {
	t.Parallel()
	flat, err := parseWithAdapters([]byte("print ${#a[@]:#x}\n"), "t.zsh")
	if err != nil {
		t.Fatalf("parse flat: %v", err)
	}
	nested, err := parseWithAdapters([]byte("print ${#${a[@]:#x}}\n"), "t.zsh")
	if err != nil {
		t.Fatalf("parse nested: %v", err)
	}

	lengthOf := func(tree *syntax.File) (outer *syntax.ParamExp, op syntax.ParExpOperator) {
		syntax.Walk(tree, func(node syntax.Node) bool {
			exp, ok := node.(*syntax.ParamExp)
			if !ok {
				return true
			}
			if exp.Length && outer == nil {
				outer = exp
			}
			if exp.Exp != nil && op == 0 {
				op = exp.Exp.Op
			}
			return true
		})
		return outer, op
	}

	flatExp, flatOp := lengthOf(flat)
	nestedExp, nestedOp := lengthOf(nested)
	if flatExp == nil || nestedExp == nil {
		t.Fatal("both spellings must carry a length")
	}
	if flatOp != nestedOp {
		t.Errorf("operator: flat = %v, nested = %v", flatOp, nestedOp)
	}
	if flatOp != syntax.MatchEmpty {
		t.Errorf("operator = %v, want %v", flatOp, syntax.MatchEmpty)
	}
}
