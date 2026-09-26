package parse

import (
	"strings"
	"testing"
	"time"
)

// A function head ended by `)` or `}` instead of a separator, as in
// `(function a)`, made the name scan in findFunctionSemicolonBody stop
// without consuming anything, so it never advanced (#480). These rows are
// native-valid; whether they parse is #479. Here they must return, with the
// parser's error.
func TestParseFunctionHeadBeforeCloserTerminates(t *testing.T) {
	for _, src := range []string{
		"(function a)\n",
		"( function a b )\n",
		"( function )\n",
		"x=$(function a)\n",
		"print $(function a)\n",
		"print \"$(function a)\"\n",
		"[[ -n $(function a) ]]\n",
		"( function a ) ; print z\n",
	} {
		t.Run(src, func(t *testing.T) {
			done := make(chan error, 1)
			go func() {
				_, err := Parse(strings.NewReader(src), "hang.zsh")
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("Parse(%q) succeeded; want the parser error until #479", src)
				}
				if !strings.Contains(err.Error(), "must be followed by a statement") {
					t.Fatalf("Parse(%q) error = %v; want the original parser error", src, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("Parse(%q) did not return within 5s", src)
			}
		})
	}
}

func TestFindFunctionSemicolonBodyStopsAtACloser(t *testing.T) {
	for _, tc := range []struct {
		src  string
		seed int
	}{
		{"(function a)", 0},
		{"(function a)", 1},
		{"( function )", 0},
		{"( function )", 2},
		{"{ function a }", 2},
		{"(a b)", 1},
	} {
		done := make(chan bool, 1)
		go func() {
			_, ok := findFunctionSemicolonBody([]byte(tc.src), tc.seed)
			done <- ok
		}()
		select {
		case ok := <-done:
			if ok {
				t.Errorf("findFunctionSemicolonBody(%q, %d) found a separator; want none", tc.src, tc.seed)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("findFunctionSemicolonBody(%q, %d) did not return within 5s", tc.src, tc.seed)
		}
	}

	// A closer after the separator does not hide it: the body check
	// refuses `)`, so this is still no site.
	if _, ok := findFunctionSemicolonBody([]byte("( function a; )"), 2); ok {
		t.Error("findFunctionSemicolonBody accepted `( function a; )`; `)` cannot open a body")
	}
	// The fix stops the scan only where it made no progress, so a head
	// with a real separator is still found.
	if semi, ok := findFunctionSemicolonBody([]byte("function a b; print z"), 0); !ok || semi != 12 {
		t.Errorf("findFunctionSemicolonBody(`function a b; print z`) = %d, %v; want 12, true", semi, ok)
	}
}
