package parse

import "testing"

// A subscript's `[` may stand on a name, on the `}` of a nested expansion,
// or on the `]` of an earlier subscript of the same expansion (#252); the
// flag-pattern adapters find flag groups through this test.
func TestFlagGroupOpenerBases(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"${a[(i)x]}", true},
		{"a[(i)x]", true},
		{"${${a}[(i)x]}", true},
		{"${a[b][(i)x]}", true},
		{"${a[b][c][(i)x]}", true},
		{"${a[b[1]][(i)x]}", true},
		{"a[][(i)x]", true},
		{"${a[1,(i)x]}", true},
		{"${a[b][1,(i)x]}", true},
		{"[b][(i)x]", false},
		{"[(i)x]", false},
		{" [b][(i)x]", false},
		{"$(( a, (b) ))", false},
		{"x\n[b][(i)x]", false},
	}
	for _, test := range tests {
		t.Run(test.src, func(t *testing.T) {
			src := []byte(test.src)
			paren := -1
			for i := len(src) - 1; i >= 0; i-- {
				if src[i] == '(' {
					paren = i
					break
				}
			}
			if got := isFlagGroupOpener(src, paren); got != test.want {
				t.Fatalf("isFlagGroupOpener(%q, %d) = %v, want %v", test.src, paren, got, test.want)
			}
		})
	}
}

func TestMatchingOpenBracket(t *testing.T) {
	tests := []struct {
		src    string
		closer int
		want   int
	}{
		{"a[b]", 3, 1},
		{"a[]", 2, 1},
		{"a[b[c]]", 6, 1},
		{"a[b[c]]", 5, 3},
		{"[x]", 2, 0},
		{"x]", 1, -1},
		{"a[\nb]", 4, -1},
	}
	for _, test := range tests {
		if got := matchingOpenBracket([]byte(test.src), test.closer); got != test.want {
			t.Errorf("matchingOpenBracket(%q, %d) = %d, want %d", test.src, test.closer, got, test.want)
		}
	}
}
