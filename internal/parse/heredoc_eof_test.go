package parse

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Issue #686: a here-document ends at the end of the file in Zsh, so the
// sources below fail for another reason, which zsh -f -n reports too.
func TestHeredocEOFInvalidFixtures(t *testing.T) {
	for _, tc := range []struct{ file, err string }{
		// The function body is still open at the end of the file.
		{"invalid-686-function-body.txt", "6:5: reached EOF without matching `{` with `}`"},
		// The body started inside the substitution and read its `)`.
		{"invalid-686-cmdsubst.txt", "7:7: reached EOF without matching `$(` with `)`"},
		// The same, inside a glob group, whose reader stops at the `)`.
		{"invalid-686-group-subst.txt", "7:16: unclosed here-document `EOF`"},
	} {
		src, err := os.ReadFile("testdata/" + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		_, err = Parse(bytes.NewReader(src), "")
		var pe syntax.ParseError
		if !errors.As(err, &pe) || err.Error() != tc.err {
			t.Errorf("%s: error = %v, want %s", tc.file, err, tc.err)
		}
	}
}

// The scanners of the always-block, nested conditional-pattern and
// declaration-brace adapters read here-document bodies to place their masks;
// a body that runs to the end of the file must not make them decline.
func TestHeredocEOFAfterAdapterConstructs(t *testing.T) {
	for _, src := range []string{
		"{ true } always { true }\ncat <<EOF\nbody\n",
		"cat <<EOF; { true } always { true }\nbody\n",
		"{ cat <<EOF; } always { true }\nbody\n",
		"[[ $line == ((a|b)|(x)) ]]\ncat <<EOF\nbody\n",
		"{ typeset -g C=1 }\ncat <<EOF\nbody\n",
		"cat <<A <<B; { true } always { true }\na\nA\nb\n",
	} {
		file, err := Parse(strings.NewReader(src), "eof.zsh")
		if err != nil {
			t.Errorf("Parse(%q): %v", src, err)
			continue
		}
		var last *syntax.Redirect
		syntax.Walk(file.AST(), func(node syntax.Node) bool {
			if r, ok := node.(*syntax.Redirect); ok && r.Op == syntax.Hdoc {
				last = r
			}
			return true
		})
		if last == nil || last.ClosePos.IsValid() {
			t.Errorf("Parse(%q): want a last here-document ended at EOF", src)
			continue
		}
		want := "body\n"
		if strings.Contains(src, "<<B") {
			want = "b\n"
		}
		if got := heredocBody(t, last); got != want {
			t.Errorf("Parse(%q): body = %q, want %q", src, got, want)
		}
	}
}

// A nested arithmetic expansion is read as a command substitution first
// (#361), so `x << 2` is a here-document there. In Zsh one whose body has not
// started ends at the substitution's `)`, so the rest of the file stays
// statements, and the resolver still turns the span into arithmetic.
func TestHeredocEOFNestedArithmeticShift(t *testing.T) {
	file, err := Parse(strings.NewReader("print ${$(( x << 2 ))}\necho after\n"), "shift.zsh")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stmts := file.AST().Stmts
	if len(stmts) != 2 {
		t.Fatalf("got %d statements, want 2", len(stmts))
	}
	var arith *syntax.ArithmExp
	syntax.Walk(stmts[0], func(node syntax.Node) bool {
		if a, ok := node.(*syntax.ArithmExp); ok {
			arith = a
		}
		return arith == nil
	})
	if arith == nil {
		t.Fatal("first statement holds no arithmetic expansion")
	}
	if b, ok := arith.X.(*syntax.BinaryArithm); !ok || b.Op != syntax.Shl {
		t.Fatalf("arithmetic = %#v, want x << 2", arith.X)
	}
}
