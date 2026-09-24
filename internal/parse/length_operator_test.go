package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// A length prefix composes with every expansion operator in Zsh: `#` applies
// to the result of the rest of the expansion (zshexpn, Parameter Expansion).
// mvdan/sh rejects the combination outright, so each row below is valid Zsh
// that failed to parse before this adapter. Every row was verified with
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
		{"offset", "print ${#a:1}\n"},
		{"offset and length", "print ${#a:1:2}\n"},
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

// The mask writes `_` over the length `#`, so the tree must be checked, not
// just the absence of an error: a parse that succeeded with `_a` as the
// parameter name would report the wrong name and lose the length.
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

// Rows the adapter must decline, so the parser's own error stands. Each is
// either invalid Zsh or a construct that belongs to another adapter.
func TestLengthOperatorDeclines(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		src  string
	}{
		// `_*` and `_@` are not names, so the mask cannot parse. Valid
		// Zsh, but out of this adapter's reach; documented in the issue.
		{"a star parameter", "print ${#*:#x}\n"},
		{"an at parameter", "print ${#@:#x}\n"},
		// The `+` prefix is a different operator with its own semantics.
		{"an existence prefix", "print ${+a:#x}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseWithAdapters([]byte(test.src), "t.zsh"); err == nil {
				t.Fatalf("parse %q succeeded, want the parser error to stand", test.src)
			}
		})
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

// The error-text gate decides whether the adapter runs at all. It is
// verdict-redundant with the positional scan: measured across 189 tree
// fixtures and 83 probe rows, removing it changed no verdict, because
// lengthPrefixBefore refuses anything that is not `${#name` before the
// reported offset. It is kept because it bounds the cost (without it every
// parse error in the file would trigger a reparse) and because it says which
// construct this adapter owns. Since no source can discriminate it, it is
// pinned here directly.
func TestLengthOperatorGatesOnItsOwnError(t *testing.T) {
	t.Parallel()
	src := []byte("print ${#a[@]:#x}\n")
	parsed := false
	parse := func([]byte, string) (*syntax.File, error) {
		parsed = true
		return parseTree(src, "t.zsh")
	}

	t.Run("a different parse error is declined", func(t *testing.T) {
		other := syntax.ParseError{
			Filename: "t.zsh",
			Pos:      syntax.NewPos(13, 1, 14),
			Text:     "not a valid parameter expansion operator: `[`",
		}
		_, err := parseLengthOperatorWithParser(src, "t.zsh", other, parse)
		if !errors.Is(err, error(other)) && err.Error() != other.Error() {
			t.Errorf("error = %v, want the original %v", err, other)
		}
		if parsed {
			t.Error("the adapter reparsed on an error it does not own")
		}
	})

	t.Run("a non-parse error is declined", func(t *testing.T) {
		parsed = false
		plain := errors.New("read error")
		_, err := parseLengthOperatorWithParser(src, "t.zsh", plain, parse)
		if !errors.Is(err, plain) {
			t.Errorf("error = %v, want the original %v", err, plain)
		}
		if parsed {
			t.Error("the adapter reparsed on a non-parse error")
		}
	})
}

// lengthPrefixBefore is the gate. Test it directly: some of its refusals
// cannot be reached through Parse, because the parser fails earlier or with a
// different error, and an end-to-end test would pass whether or not the guard
// exists.
func TestLengthPrefixBefore(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		src      string
		operator int
		want     int
		wantOK   bool
	}{
		{"name directly before the operator", "print ${#a:#x}", 10, 9 - 1, true},
		{"a subscript before the operator", "print ${#a[@]:#x}", 13, 8, true},
		{"nested brackets in the subscript", "print ${#a[b[1]]:#x}", 16, 8, true},
		{"no name between prefix and operator", "print ${#:#x}", 9, 0, false},
		{"a second subscript is not ours", "print ${#a[1][2]}", 13, 0, false},
		{"no length prefix", "print ${a:#x}", 9, 0, false},
		{"a newline inside the subscript", "print ${#a[b\n]:#x}", 14, 0, false},
		{"operator at the start", "x", 0, 0, false},
		{"operator past the end", "x", 9, 0, false},
		{"unclosed subscript", "print ${#a]:#x}", 11, 0, false},
		// A `#` that is not an expansion's length prefix. Unreachable
		// through Parse, because the parser does not report this
		// adapter's error there, so it is pinned here instead: without
		// the `${` check the scan would accept the `#` in `${a#b:-c}`
		// and mask a byte that is an operator, not a prefix.
		{"a strip operator, not a length prefix", "print ${a#b:-c}", 11, 0, false},
		{"a hash outside any expansion", "print a#b:-c", 9, 0, false},
		{"a hash with no brace before it", "print $#a:-c", 9, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, ok := lengthPrefixBefore([]byte(test.src), test.operator)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if ok && got != test.want {
				t.Errorf("offset = %d, want %d", got, test.want)
			}
		})
	}
}
