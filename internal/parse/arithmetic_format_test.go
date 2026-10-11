package parse

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
	"mvdan.cc/sh/v3/syntax/typedjson"
)

func TestArithmeticFormatAndCharacterTree(t *testing.T) {
	src := "print $(( [##16] #key + ##$MATCH ))\nprint after\n"
	ResetStats()
	f, err := Parse(strings.NewReader(src), "arithmetic.zsh")
	if err != nil {
		t.Fatal(err)
	}
	if got := ReadStats(); got != (Stats{TreeParses: 1}) {
		t.Fatalf("arithmetic requires an adapter: %+v", got)
	}
	call := f.AST().Stmts[0].Cmd.(*syntax.CallExpr)
	ar := call.Args[1].Parts[0].(*syntax.ArithmExp)
	sum := ar.X.(*syntax.BinaryArithm)
	base := sum.X.(*syntax.OutputBaseArithm)
	char := base.X.(*syntax.CharArithm)
	literal := sum.Y.(*syntax.CharArithm)
	if ar.Unsigned || sum.Op != syntax.Add || base.Post || base.Format.Value != "##16" || char.Literal || char.Value.Lit() != "key" || !literal.Literal {
		t.Fatalf("arithmetic meanings lost: %+v %+v %+v", base, char, literal)
	}
	if base.Pos().Offset() != 10 || base.Format.Pos().Offset() != 11 || base.Rbrack.Offset() != 15 || char.Pos().Offset() != 17 || char.End().Offset() != 21 || literal.Pos().Offset() != 24 || literal.End().Offset() != 32 {
		t.Fatalf("original positions lost: %+v %+v %+v", base, char, literal)
	}
	visited, names := 0, 0
	syntax.Walk(ar, func(n syntax.Node) bool {
		if pe, ok := n.(*syntax.ParamExp); ok && pe.Param.Value == "MATCH" {
			visited++
		}
		if lit, ok := n.(*syntax.Lit); ok && lit.Value == "key" {
			names++
		}
		return true
	})
	if visited != 1 || names != 1 || f.AST().Stmts[1].Pos().Offset() != uint(strings.Index(src, "print after")) {
		t.Fatal("character expansion or following statement lost to walker")
	}
	var encoded bytes.Buffer
	if err := typedjson.Encode(&encoded, f.AST()); err != nil {
		t.Fatal(err)
	}
	decoded, err := typedjson.Decode(&encoded)
	if err != nil || !reflect.DeepEqual(decoded, f.AST()) {
		t.Fatalf("exported arithmetic tree lost in typed JSON: %v", err)
	}
}

func TestArithmeticFormatUnsupportedEvaluation(t *testing.T) {
	for _, source := range []string{"[#16] 255", "#key", "##a", "++#key", "#key++", "#key = 2", "key [#16] = 2"} {
		expr, err := syntax.NewParser(syntax.Variant(syntax.LangZsh)).Arithmetic(strings.NewReader(source))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := expand.Arithm(&expand.Config{}, expr); err == nil || err.Error() != "unsupported" {
			t.Fatalf("Zsh arithmetic evaluation error = %v", err)
		}
	}
}

func TestArithmeticFormatAfterPostfixTree(t *testing.T) {
	for _, operator := range []string{"++", "--"} {
		src := "(( key" + operator + " [#16] [##8] + 1 ))\nprint after\n"
		ResetStats()
		f, err := Parse(strings.NewReader(src), "postfix.zsh")
		if err != nil {
			t.Fatal(err)
		}
		if got := ReadStats(); got != (Stats{TreeParses: 1}) {
			t.Fatalf("postfix formats require an adapter: %+v", got)
		}
		sum := f.AST().Stmts[0].Cmd.(*syntax.ArithmCmd).X.(*syntax.BinaryArithm)
		last := sum.X.(*syntax.OutputBaseArithm)
		first := last.X.(*syntax.OutputBaseArithm)
		post := first.X.(*syntax.UnaryArithm)
		wantOp := syntax.Inc
		if operator == "--" {
			wantOp = syntax.Dec
		}
		if sum.Op != syntax.Add || !first.Post || !last.Post || !post.Post || post.Op != wantOp || first.Format.Value != "#16" || last.Format.Value != "##8" {
			t.Fatalf("postfix/format meanings lost: %+v %+v %+v", first, last, post)
		}
		if _, err := expand.Arithm(&expand.Config{}, first); err == nil || err.Error() != "unsupported" {
			t.Fatalf("postfix format evaluation error = %v", err)
		}
		if post.Pos().Offset() != 3 || post.End().Offset() != 8 || post.OpPos.Offset() != 6 || first.Pos().Offset() != 3 || first.Lbrack.Offset() != 9 || first.End().Offset() != 14 || last.Lbrack.Offset() != 15 || last.End().Offset() != 20 {
			t.Fatalf("postfix format positions lost: %+v %+v %+v", first, last, post)
		}
		var literals []string
		syntax.Walk(sum, func(n syntax.Node) bool {
			if lit, ok := n.(*syntax.Lit); ok {
				literals = append(literals, lit.Value)
			}
			return true
		})
		if strings.Join(literals, "/") != "key/#16/##8/1" || f.AST().Stmts[1].Pos().Offset() != uint(strings.Index(src, "print after")) {
			t.Fatalf("postfix tree traversal or statement boundary lost: %v", literals)
		}
		var encoded bytes.Buffer
		if err := typedjson.Encode(&encoded, f.AST()); err != nil {
			t.Fatal(err)
		}
		decoded, err := typedjson.Decode(&encoded)
		if err != nil || !reflect.DeepEqual(decoded, f.AST()) {
			t.Fatalf("postfix arithmetic tree lost in typed JSON: %v", err)
		}
	}
}
