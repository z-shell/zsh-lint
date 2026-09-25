package parse

import (
	"strings"
	"testing"
)

// scanCommandWords outlived the repeat adapter it was written for (#281);
// do_leading_separator.go still relies on it. This pins which words it
// reports in command position: never inside quotes, comments, heredoc
// bodies or arithmetic, never an argument, and again after a separator, an
// opening `(` or `{`, a case pattern's `)`, `!`, `&&`, `|` or a prefix word.
func TestScanCommandWords(t *testing.T) {
	src := "cmd1 a\n" +
		"x=1; cmd2 b\n" +
		"if true; then cmd3 c; fi\n" +
		"print word4\n" +
		"'quoted5' x\n" +
		"$(cmd6 d)\n" +
		"( cmd7 e )\n" +
		"case x in (x) cmd8 f ;; esac\n" +
		"# cmd9 g\n" +
		"cat <<EOT\ncmd10 h\nEOT\n" +
		"! cmd13 k\n" +
		"{ cmd14 l }\n" +
		"true && cmd15 m\n" +
		"a=(word16 n)\n" +
		"print \"cmd17 o\"\n" +
		"(( cmd18 ))\n" +
		"time cmd19 p | cmd20 q\n" +
		"`cmd21`\n"
	var got []string
	scanCommandWords([]byte(src), func(start, end int, word string) (int, bool) {
		if src[start:end] != word {
			t.Errorf("word %q at [%d, %d) reads %q", word, start, end, src[start:end])
		}
		got = append(got, word)
		return 0, false
	})
	want := []string{
		"cmd1", "cmd2", "if", "true", "then", "cmd3", "fi", "print", "cmd6", "cmd7",
		"case", "x", "cmd8", "esac", "cat", "cmd13", "cmd14", "true", "cmd15",
		"print", "time", "cmd19", "cmd20", "cmd21",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("command words = %q, want %q", got, want)
	}
}

// A visit that consumes the word's operands resumes the scan at the offset it
// returns, in command position again.
func TestScanCommandWordsResume(t *testing.T) {
	src := "skip a b; cmd c\nskip x\ncmd y\n"
	var got []string
	scanCommandWords([]byte(src), func(start, end int, word string) (int, bool) {
		got = append(got, word)
		if word != "skip" {
			return 0, false
		}
		i := end
		for i < len(src) && src[i] != ';' && src[i] != '\n' {
			i++
		}
		return i, true
	})
	if want := "skip,cmd,skip,cmd"; strings.Join(got, ",") != want {
		t.Fatalf("command words = %q, want %s", got, want)
	}
}

// Escapes, every quote form, expansions, comments inside a word, `!`,
// redirections, here-strings, arithmetic, backquotes and `<<-` heredocs
// each leave or restore command position as native Zsh does, and the word
// after a `repeat` count is in command position again (#281).
func TestScanCommandWordsContexts(t *testing.T) {
	src := "\\esc1 a\n" +
		"\\\ncont2 b\n" +
		"print \"a\\\"b\" w3; cmd4\n" +
		"print $'a\\'b' w7; cmd8\n" +
		"${x} w9; cmd10\n" +
		"print a#b w11; cmd12\n" +
		"!glued13 x\n" +
		"! cmd14 y\n" +
		"cmd15 2>&1 w16\n" +
		"cmd17 <<<w18 w19; cmd20\n" +
		"x=$((1 + 2)) w21; cmd22\n" +
		"print `cmd23 z` w24\n" +
		"cat <<-EOT; cmd32\n\tbody33\n\tEOT\ncmd34\n" +
		"cmd35 >& w36\n" +
		"cmd37 &| cmd38\n" +
		"f() { cmd40 }\n" +
		"repeat 3 cmd41 a\n" +
		"repeat \"$n\" cmd42\n" +
		"repeat $(( n )) do cmd43; done\n"
	var got []string
	scanCommandWords([]byte(src), func(start, end int, word string) (int, bool) {
		got = append(got, word)
		return 0, false
	})
	want := []string{
		"cont2", "print", "cmd4", "print", "cmd8", "cmd10", "print", "cmd12", "cmd14",
		"cmd15", "cmd17", "cmd20", "cmd22", "print", "cmd23", "cat", "cmd32", "cmd34",
		"cmd35", "cmd37", "cmd38", "f", "cmd40",
		"repeat", "cmd41", "repeat", "cmd42", "repeat", "do", "cmd43", "done",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("command words = %q, want %q", got, want)
	}
}
