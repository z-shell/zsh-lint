package parse

// Source scanning helpers the compatibility adapters share. They read raw
// source bytes to find a construct's sites before the parser sees them.

func matchSourceWord(src []byte, i int, word string) bool {
	if i+len(word) > len(src) {
		return false
	}
	if string(src[i:i+len(word)]) != word {
		return false
	}
	if i+len(word) < len(src) {
		next := src[i+len(word)]
		if next != ' ' && next != '\t' && next != '\n' && next != ';' && next != '&' && next != '|' && next != '(' && next != '{' {
			return false
		}
	}
	return true
}

func skipSpacesAndComments(src []byte, i int) int {
	for i < len(src) {
		b := src[i]
		if b == ' ' || b == '\t' || b == '\n' {
			i++
			continue
		}
		if b == '#' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		break
	}
	return i
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// startsSourceComment reports whether the unquoted, unescaped `#` at i in
// active source begins a comment. A comment is a word that begins with `#`
// (zshmisc COMMENTS), so the `#` must start the file or follow a blank, a
// newline, a `;`, or a `{` that opens a brace group. A `{` or `}` glued to
// word text before the `#` is part of that word, as in `${#x}`, `${x}#y`,
// `a{#b`, `\{#b` or `}#c`, and so is the `#` (Zsh reads `}` as a closer only
// when it is a word of its own).
//
// The bytes before a `{` that may put it in command position are the
// separators, a subshell or substitution opener, the `)` of `f()`, another
// `{` and a backquote. A `{` after a blank is taken to open a group too,
// which is wrong for an argument such as `print {#b`: telling the two apart
// needs the command position, which the scanners using this do not track.
//
// This looks only at the bytes before the `#`. A `#` inside `${...}` after a
// blank or `;`, as in `${x:- #a}`, passes it; scanners also ask a
// parameterWordTracker, which knows that such a `#` begins no word.
func startsSourceComment(src []byte, i int) bool {
	if i == 0 {
		return true
	}
	switch src[i-1] {
	case ' ', '\t', '\n', ';':
		return true
	case '{':
		if i == 1 {
			return true
		}
		switch src[i-2] {
		case ' ', '\t', '\n', ';', '&', '|', '(', ')', '{', '`':
			return true
		}
	}
	return false
}

// parameterWordTracker follows the unquoted `${...}` expansions a flat site
// scanner passes, so the scanner can tell a `#` inside one from a `#` that
// starts a comment. Zsh ignores a word beginning with `#` up to the newline
// (zshmisc, COMMENTS), and a `${...}` is part of a word: the `#` of `${#x}`,
// or one after a blank, `;` or `{` of `${x:- #a}`, begins no word. A command
// substitution or backquoted command nested in the expansion holds a command
// list again, where a word may begin with `#`.
//
// A scanner calls open at every active `$` and backquote and asks inWord
// before it starts a comment. The extents come from the shared skip rules;
// where they cannot tell one, the tracker forgets every frame and the
// scanner reads on as it did without it.
type parameterWordTracker struct {
	frames []parameterWordFrame
}

type parameterWordFrame struct {
	end     int  // offset of the frame's last byte
	command bool // a command substitution or backquoted command
}

// open records the expansion or command opened at i: a `${`, or, inside an
// expansion's word, a `$(` command substitution or a backquote. It first
// drops the frames that end before i.
func (t *parameterWordTracker) open(src []byte, i int) {
	t.drop(i)
	var end int
	var ok bool
	command := true
	switch {
	case src[i] == '$' && i+1 < len(src) && src[i+1] == '{':
		end, ok = skipBracedParameter(src, i+1, false)
		command = false
	case !t.inWord(i):
		return
	case src[i] == '$' && i+2 < len(src) && src[i+1] == '(' && src[i+2] != '(':
		end, ok = skipCommandSubstitution(src, i+1)
	case src[i] == '`':
		end, ok = skipBackquoted(src, i)
	default:
		return
	}
	if !ok {
		t.frames = nil
		return
	}
	t.frames = append(t.frames, parameterWordFrame{end: end, command: command})
}

// inWord reports whether the byte at i lies in the word of an expansion
// rather than in a command list nested in it.
func (t *parameterWordTracker) inWord(i int) bool {
	t.drop(i)
	return len(t.frames) > 0 && !t.frames[len(t.frames)-1].command
}

func (t *parameterWordTracker) drop(i int) {
	for len(t.frames) > 0 && t.frames[len(t.frames)-1].end < i {
		t.frames = t.frames[:len(t.frames)-1]
	}
}
