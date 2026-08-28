package parse

import (
	"reflect"

	"mvdan.cc/sh/v3/syntax"
)

// adapterAttempt is one compatibility adapter's gated entry point. Each
// receives the source, the name, and the concrete error the previous stage
// produced, and either returns a tree or returns that same error unchanged
// because its own gate did not match.
type adapterAttempt func(src []byte, name string, firstErr error) (*syntax.File, error)

// adapterChain returns the single ordered list of compatibility adapters.
//
// This is a function rather than a package-level variable on purpose: adapters
// call parseWithAdapters for their masked retry, and parseWithAdapters reads
// this list, so a package-level slice literal would be an initialization
// cycle.
//
// Every adapter that masks source and re-parses must retry through
// parseWithAdapters, not through parseTree and not through a hand-written
// subset of its peers. A masked retry is still an ordinary parse of a whole
// file, so any other Zsh construct anywhere in that file must remain
// parseable.
//
// Before this chain existed, adapters composed through hardcoded short lists
// (parseAfterAlternateIf named exactly two peers). That made composition
// depend on which adapter happened to run first: 40 of 42 ordered pairs of
// distinct adapter features failed to parse even though every feature parsed
// in isolation and native Zsh accepted every combination. The observable bug
// was `${~ZI[BIN_DIR]}` in z-shell/zi zi.zsh failing only because an alternate
// brace-form `if` appeared earlier in the file.
//
// Adding an adapter means adding it here, once. Do not reintroduce per-adapter
// composition lists.
func adapterChain() []adapterAttempt {
	return []adapterAttempt{
		parseNestedConditionalAlternation,
		parseAlternateIfBrace,
		parseAssociativeSubscript,
		parseRCExpandCaret,
		parseReverseSubscript,
		parseParamGlobToggle,
		parseFdVarRedirect,
		parseMultiNameFor,
		parseTryAlways,
		parseGroupedCasePattern,
		parseANSICHeredocDelimiter,
	}
}

// parseWithAdapters parses src, falling back through every compatibility
// adapter in order. Each adapter is gated on its own concrete parser error, so
// a non-matching adapter returns the incoming error untouched and the next one
// sees it. When no adapter matches, the original parser error is returned.
//
// This is the retry entry point for adapters themselves. A masked source may
// legitimately need a different adapter than the one that produced the mask.
//
// Most adapters mask one occurrence of their feature per pass and rely on
// re-entering themselves to handle a second occurrence in the same file, so
// self-recursion is allowed by default. An adapter whose masking would
// otherwise widen its own accepted grammar must opt out with retryExcluding.
// The grouped-case adapter is the current example: left to recurse, it masked
// one extra `)` per pass and accepted `case x in (x|y))) : ;; esac`, which
// native Zsh rejects.
func parseWithAdapters(src []byte, name string) (*syntax.File, error) {
	return parseWithAdaptersExcept(src, name, -1)
}

// parseWithAdaptersExcept is parseWithAdapters with the adapter at index skip
// disabled for the whole nested retry, preventing self-recursion.
func parseWithAdaptersExcept(src []byte, name string, skip int) (*syntax.File, error) {
	tree, err := parseTree(src, name)
	if err == nil {
		return tree, nil
	}
	for index, attempt := range adapterChain() {
		if index == skip {
			continue
		}
		tree, attemptErr := attempt(src, name, err)
		if attemptErr == nil {
			return tree, nil
		}
		err = attemptErr
	}
	return nil, err
}

// retryExcluding returns the retry parser an adapter must hand to its
// *WithParser helper: the full chain minus the caller itself. Adapters look
// themselves up by function identity so the chain order stays the single
// source of truth.
func retryExcluding(self adapterAttempt) func([]byte, string) (*syntax.File, error) {
	skip := -1
	selfPtr := reflect.ValueOf(self).Pointer()
	for index, attempt := range adapterChain() {
		if reflect.ValueOf(attempt).Pointer() == selfPtr {
			skip = index
			break
		}
	}
	return func(src []byte, name string) (*syntax.File, error) {
		return parseWithAdaptersExcept(src, name, skip)
	}
}
