package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// A length prefix composes with every expansion operator in Zsh: `#` applies
// to the result of the rest of the expansion (zshexpn, Parameter Expansion).
// The fork retains the prefix and suffix directly, without a masked retry. Every row was verified with
// `zsh -f -n` AND by running it, because `-n` only parses.
func TestParseLengthOperator(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		src  string
	}{
		{"filter under a length, the reported gap", "print ${#a[@]:#x}\n"},
		{"filter without a subscript", "print ${#a:#x}\n"},
		{"the zi install.zsh idiom", "(( ${#profiles[@]:#$profile} > 0 ))\n"},
		{"replacement", "print ${#a//x/y}\n"},
		{"single replacement", "print ${#a/x/y}\n"},
		{"default value", "print ${#a:-y}\n"},
		{"alternate value", "print ${#a:+y}\n"},
		{"assign default", "print ${#a:=y}\n"},
		{"prefix strip", "print ${#a#p}\n"},
		{"suffix strip", "print ${#a%p}\n"},
		{"subscript then replacement", "print ${#a[@]//x/y}\n"},
		{"a positional name", "print ${#0:#x}\n"},
		{"inside arithmetic", "x=$(( ${#a[@]:#y} ))\n"},
		{"inside double quotes", "print \"${#a[@]:#x}\"\n"},
		{"a flag in the subscript", "print ${#a[(r)x]:#y}\n"},
		{"twice in one line", "print ${#a:#x} ${#b:#y}\n"},
		{"nested in another expansion", "print ${x:-${#a[@]:#y}}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseWithAdapters([]byte(test.src), "t.zsh"); err != nil {
				t.Fatalf("parse %q: %v", test.src, err)
			}
		})
	}
}

// The typed tree must retain both the length prefix and the suffix, with the
// original parameter name and byte positions.
func TestLengthOperatorRestoresNameAndLength(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		src       string
		wantName  string
		wantOp    syntax.ParExpOperator
		wantIndex bool
		// byte offset the name literal must start at, which is the byte
		// after the length `#`
		wantNameOffset int
	}{
		{
			name:           "filter with a subscript",
			src:            "print ${#a[@]:#x}\n",
			wantName:       "a",
			wantOp:         syntax.MatchEmpty,
			wantIndex:      true,
			wantNameOffset: 9,
		},
		{
			name:           "filter without a subscript",
			src:            "print ${#ab:#x}\n",
			wantName:       "ab",
			wantOp:         syntax.MatchEmpty,
			wantNameOffset: 9,
		},
		{
			name:           "replacement",
			src:            "print ${#name//x/y}\n",
			wantName:       "name",
			wantNameOffset: 9,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tree, err := parseWithAdapters([]byte(test.src), "t.zsh")
			if err != nil {
				t.Fatalf("parse %q: %v", test.src, err)
			}
			var exp *syntax.ParamExp
			syntax.Walk(tree, func(node syntax.Node) bool {
				if found, ok := node.(*syntax.ParamExp); ok && exp == nil {
					exp = found
				}
				return exp == nil
			})
			if exp == nil {
				t.Fatalf("no parameter expansion in %q", test.src)
			}
			if !exp.Length {
				t.Errorf("Length = false, want true: the source wrote a length prefix")
			}
			if exp.Param == nil {
				t.Fatalf("Param = nil, want the name %q", test.wantName)
			}
			if exp.Param.Value != test.wantName {
				t.Errorf("name = %q, want %q (the mask byte must not survive)", exp.Param.Value, test.wantName)
			}
			if got := int(exp.Param.ValuePos.Offset()); got != test.wantNameOffset {
				t.Errorf("name offset = %d, want %d", got, test.wantNameOffset)
			}
			// The column must agree with the offset, since positions are
			// reported to users from the column.
			if got := int(exp.Param.ValuePos.Col()); got != test.wantNameOffset+1 {
				t.Errorf("name column = %d, want %d", got, test.wantNameOffset+1)
			}
			if (exp.Index != nil) != test.wantIndex {
				t.Errorf("Index present = %v, want %v", exp.Index != nil, test.wantIndex)
			}
			if test.wantOp != 0 {
				if exp.Exp == nil {
					t.Fatalf("Exp = nil, want operator %v", test.wantOp)
				}
				if exp.Exp.Op != test.wantOp {
					t.Errorf("operator = %v, want %v", exp.Exp.Op, test.wantOp)
				}
			}
		})
	}
}

// The printer reproduces the source only when the tree really says what the
// source said. This is the end-to-end check that the mask byte is gone.
func TestLengthOperatorRoundTrips(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		"print ${#a[@]:#x}\n",
		"print ${#a:#x}\n",
		"print ${#a//x/y}\n",
		"print ${#a:-y}\n",
	} {
		t.Run(strings.TrimSpace(src), func(t *testing.T) {
			t.Parallel()
			tree, err := parseWithAdapters([]byte(src), "t.zsh")
			if err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			var sb strings.Builder
			if err := syntax.NewPrinter().Print(&sb, tree); err != nil {
				t.Fatalf("print: %v", err)
			}
			if sb.String() != src {
				t.Errorf("round trip = %q, want %q", sb.String(), src)
			}
		})
	}
}

// The existence prefix has different semantics and remains unsupported here.
func TestLengthOperatorDoesNotWidenExistencePrefix(t *testing.T) {
	t.Parallel()
	if _, err := parseWithAdapters([]byte("print ${+a:#x}\n"), "t.zsh"); err == nil {
		t.Fatal("existence prefix accepted a suffix")
	}
}

// A second subscript under a length prefix reports this adapter's error text
// but belongs to parseSecondSubscript. It must keep working, and the
// expansion must still carry its length.
func TestLengthOperatorLeavesSecondSubscriptAlone(t *testing.T) {
	t.Parallel()
	src := []byte("print ${#a[1][2]}\n")
	tree, err := parseWithAdapters(src, "t.zsh")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	found := bindSecondSubscripts(tree, src)
	if len(found) != 1 {
		t.Fatalf("second subscripts = %d, want 1", len(found))
	}
	if !found[0].Expansion.Length {
		t.Error("Length = false, want true")
	}
	if got := found[0].Expansion.Param.Value; got != "a" {
		t.Errorf("name = %q, want \"a\"", got)
	}
}

// Rows native Zsh rejects, which must stay rejected. The fixtures live in
// testdata/invalid-373-*.txt per the repo convention (a `.txt` suffix keeps
// the repo-wide `zsh -n` gate away from deliberately broken source), and the
// error family and position are asserted so a future change that rejects
// them for a different reason is visible.
func TestLengthOperatorRejectsInvalidSource(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		file    string
		wantMsg string
		wantCol int
	}{
		{"testdata/invalid-373-unclosed-expansion.txt", "reached EOF without matching `${` with `}`", 7},
		{"testdata/invalid-373-unclosed-subscript.txt", "ternary operator missing `?` before `:`", 13},
		{"testdata/invalid-373-space-after-length.txt", "not a valid parameter expansion operator: ` `", 10},
		// Rows an earlier revision of this adapter accepted: the mask
		// turned them into the un-prefixed form, where upstream is more
		// permissive than Zsh. Found by adversarial review; both are
		// rejected on main and must stay rejected.
		{"testdata/invalid-373-unrecognized-modifier.txt", "cannot combine multiple parameter expansion operators", 11},
		{"testdata/invalid-373-negative-slice-length.txt", "cannot combine multiple parameter expansion operators", 11},
	} {
		t.Run(test.file, func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(test.file)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			_, parseErr := parseWithAdapters(src, "t.zsh")
			if parseErr == nil {
				t.Fatalf("parse %q succeeded, want a rejection", bytes.TrimSpace(src))
			}
			var typed syntax.ParseError
			if !errors.As(parseErr, &typed) {
				t.Fatalf("error %v is not a syntax.ParseError", parseErr)
			}
			if typed.Text != test.wantMsg {
				t.Errorf("error = %q, want %q", typed.Text, test.wantMsg)
			}
			if got := int(typed.Pos.Col()); got != test.wantCol {
				t.Errorf("column = %d, want %d", got, test.wantCol)
			}
		})
	}
}

// A file with two length operators must not report the first one forever.
// Fixing one exposes the next error; returning the original error there blames
// a construct this adapter just proved readable, which is what hid zi's real
// blocker behind line 307. The later position must win.
func TestLengthOperatorReportsTheLaterBlocker(t *testing.T) {
	t.Parallel()
	// Two readable length operators, then an unrelated syntax error.
	const src = "print ${#a[@]:#x}\nprint ${#b[@]:#y}\nprint ${c:\n"
	_, err := Parse(bytes.NewReader([]byte(src)), "t.zsh")
	if err == nil {
		t.Fatalf("expected the trailing error to survive")
	}
	var perr syntax.ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("expected a ParseError, got %T: %v", err, err)
	}
	if perr.Pos.Line() != 3 {
		t.Fatalf("expected the error on line 3, the real blocker, got line %d: %v",
			perr.Pos.Line(), err)
	}
	if strings.Contains(perr.Text, "_") {
		t.Fatalf("the mask byte reached a diagnostic: %q", perr.Text)
	}
}
