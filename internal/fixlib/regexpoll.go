// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// fixRegexPoll makes the regexp parser count its steps toward the
// interrupt counter of the interpreter, the Go form of the same change in
// lib/internal/patch/quickjs.diff (see lib/regexpoll.go):
//
//   - _re_parse_alternative counts a step before each term, and a step
//     and the code it moves before it moves a term of a lookbehind.
//   - _re_parse_disjunction counts a step and the code it moves before it
//     inserts the split of an alternative.
//   - _re_parse_nested_class counts a step and the ranges it copies before
//     each atom of a class.
//   - _js_compile_regexp throws the interrupt when lre_compile fails with
//     the message of an interrupt.
//
// The interpreter calls the interrupt handler once in
// JS_INTERRUPT_COUNTER_INIT polls, and a compile was one poll however long
// it took, so a loop that compiled long patterns ran on for hundreds of
// milliseconds past its timeout. _re_poll_interrupts counts on the counter
// of the context, as js_poll_interrupts does, and lets
// lre_check_timeout call the handler, so the text of those functions and of
// the three it changes is pinned in regexPollSites: when a release changes
// one of them, fixRegexPoll stops, and the new code must be read against
// lib/regexpoll.go before the fingerprints are updated (go run
// ./internal/fixlib -repoll prints them). The fingerprints are of the text
// without the change, after every other change before it.
func fixRegexPoll(b []byte, file string) ([]byte, error) {
	got, err := regexPollFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := regexPollSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/regexpoll.go, then update regexPollSites (go run ./internal/fixlib -repoll)", file, "the regexp parser, lre_check_timeout or __js_poll_interrupts", got, want)
	}
	for _, f := range []struct {
		name string
		fix  func([]byte) ([]byte, error)
	}{
		{"_re_parse_alternative", pollAlternative},
		{"_re_parse_disjunction", pollDisjunction},
		{"_re_parse_nested_class", pollClass},
	} {
		body, err := funcBody(b, funcHead(f.name), f.name)
		if err != nil {
			return nil, err
		}
		fixed, err := f.fix(body)
		if err != nil {
			return nil, err
		}
		b = bytes.Replace(b, body, fixed, 1)
	}
	if n := bytes.Count(b, []byte("_re_poll_interrupts(")); n != 4 {
		return nil, fmt.Errorf("want 4 calls of _re_poll_interrupts, found %d", n)
	}
	return pollCompileRegexp(b)
}

var (
	reAlternativeHead = regexp.MustCompile(`\nfunc _re_parse_alternative\(tls \*libc\.TLS, (\w+) uintptr, is_backward_dir TBOOL\) \(r int32\) \{\n`)
	reDisjunctionHead = regexp.MustCompile(`\nfunc _re_parse_disjunction\(tls \*libc\.TLS, (\w+) uintptr, is_backward_dir TBOOL\) \(r int32\) \{\n`)
	reClassHead       = regexp.MustCompile(`\nfunc _re_parse_nested_class\(tls \*libc\.TLS, (\w+) uintptr, (\w+) uintptr, pp uintptr\) \(r int32\) \{\n`)
	// The length of the alternatives before a split, which the split
	// moves.
	reSplitLen = regexp.MustCompile(`\n\t\t(\w+) = (?:libc\.Int32FromUint(?:32|64)|int32)\(\(\*TREParseState\)\(unsafe\.Pointer\((\w+)\)\)\.Fbyte_code\.Fsize - (?:libc\.Uint(?:32|64)FromInt32|uint(?:32|64))\(start\)\)\n\t\t/\* insert a split before the first alternative \*/\n`)
	// The test of the end of a class, at the top of the loop over its
	// atoms.
	reClassLoop    = regexp.MustCompile(`\n\t/\* handle unions \*/\n\tis_first = int32\(ETRUE\)\n\tfor \{\n\t\tif [^\n]* == int32\('\]'\) \{\n\t\t\tbreak\n\t\t\}\n`)
	regexPollFuncs = []string{"_re_parse_alternative", "_re_parse_disjunction", "_re_parse_nested_class", "_re_parse_error", "Xlre_check_timeout", "___js_poll_interrupts"}
)

// pollTerm is the step counted before each term, with s the parse state.
func pollTerm(s string) string {
	return "\t\t// Changed by internal/fixlib: count the term toward the interrupt counter (see fixRegexPoll).\n" +
		"\t\tif _re_poll_interrupts(tls, " + s + ", 0) != 0 {\n\t\t\treturn -int32(1)\n\t\t}\n"
}

// pollBackward is the step counted before a term of a lookbehind is moved
// in front of the terms before it.
func pollBackward(s string) string {
	return "\t\t\t// Changed by internal/fixlib: count the code the term moves (see fixRegexPoll).\n" +
		"\t\t\tif _re_poll_interrupts(tls, " + s + ", end-start) != 0 {\n\t\t\t\treturn -int32(1)\n\t\t\t}\n"
}

// pollSplit is the step counted before the split of an alternative is
// inserted in front of the n bytes of the alternatives before it.
func pollSplit(s, n string) string {
	return "\t\t// Changed by internal/fixlib: count the alternative and the code its split moves (see fixRegexPoll).\n" +
		"\t\tif _re_poll_interrupts(tls, " + s + ", Tsize_t(" + n + ")) != 0 {\n\t\t\treturn -int32(1)\n\t\t}\n"
}

// pollAtom is the step counted before each atom of a class, whose union
// copies the ranges of cr, 4 bytes for each of its points.
func pollAtom(s, cr string) string {
	return "\t\t// Changed by internal/fixlib: count the atom and the ranges its union copies (see fixRegexPoll).\n" +
		"\t\tif _re_poll_interrupts(tls, " + s + ", Tsize_t((*TREStringList)(unsafe.Pointer(" + cr + ")).Fcr.Flen1)*4) != 0 {\n\t\t\tgoto fail\n\t\t}\n"
}

// pollAlternative counts each term of an alternative, and the code a term
// of a lookbehind moves.
func pollAlternative(body []byte) ([]byte, error) {
	s := string(reAlternativeHead.FindSubmatch(body)[1])
	term := "\t\tret = _re_parse_term(tls, " + s + ", is_backward_dir)\n"
	body, err := insertAt("_re_parse_alternative", body, pollTerm(s), func(body []byte) (int, error) {
		return before(body, term, "", "_re_parse_alternative: cannot find the call of _re_parse_term")
	})
	if err != nil {
		return nil, err
	}
	moved := "\t\t\tterm_size = end - term_start\n"
	return insertAt("_re_parse_alternative", body, pollBackward(s), func(body []byte) (int, error) {
		// The term is moved in front of the code from start, which grows
		// it to end.
		return after(body, moved, "\t\t\tlibc.X", "_re_parse_alternative: cannot find where a term of a lookbehind is moved")
	})
}

// pollDisjunction counts each alternative after the first, and the code
// the split in front of the alternatives before it moves.
func pollDisjunction(body []byte) ([]byte, error) {
	s := string(reDisjunctionHead.FindSubmatch(body)[1])
	m := findAllSubmatch(reSplitLen, reSplitPoll(s).ReplaceAll(body, nil))
	if len(m) != 1 || string(m[0][2]) != s {
		return nil, fmt.Errorf("_re_parse_disjunction: cannot find the length of the code the split moves")
	}
	comment := "\t\t/* insert a split before the first alternative */\n"
	return insertAt("_re_parse_disjunction", body, pollSplit(s, string(m[0][1])), func(body []byte) (int, error) {
		return before(body, comment, "_dbuf_insert(tls, "+s+", start, int32(5))", "_re_parse_disjunction: cannot find the split of an alternative")
	})
}

// reSplitPoll matches pollSplit for the parse state s, whatever the name
// of the length.
func reSplitPoll(s string) *regexp.Regexp {
	parts := strings.Split(pollSplit(s, "\x00"), "\x00")
	return regexp.MustCompile(regexp.QuoteMeta(parts[0]) + `\w+` + regexp.QuoteMeta(parts[1]))
}

// pollClass counts each atom of a class, and the ranges its union copies.
func pollClass(body []byte) ([]byte, error) {
	m := reClassHead.FindSubmatch(body)
	s, cr := string(m[1]), string(m[2])
	return insertAt("_re_parse_nested_class", body, pollAtom(s, cr), func(body []byte) (int, error) {
		loc := findAll(reClassLoop, body)
		if len(loc) != 1 || !bytes.Contains(body, []byte("\nfail:\n")) {
			return 0, fmt.Errorf("_re_parse_nested_class: cannot find the loop over the atoms of a class and its fail label")
		}
		return loc[0][1], nil
	})
}

// regexInterruptBranch is the branch of _js_compile_regexp that throws the
// interrupt when lre_compile wrote the message of an interrupt to msg,
// between the branch for running out of memory and the SyntaxError.
func regexInterruptBranch(msg, ctx string) string {
	return "} else if _js_regexp_interrupted(" + msg + ") {\n" +
		"\t\t\t// Changed by internal/fixlib: the regexp parser was interrupted (see fixRegexPoll).\n" +
		"\t\t\t_JS_ThrowInterrupted(tls, " + ctx + ")\n\t\t"
}

// pollCompileRegexp puts regexInterruptBranch into the test fixOutOfMemory
// writes around the SyntaxError of _js_compile_regexp.
func pollCompileRegexp(b []byte) ([]byte, error) {
	body, err := funcBody(b, reCompileRegexp, "_js_compile_regexp")
	if err != nil {
		return nil, err
	}
	ctx := string(reCompileRegexp.FindSubmatch(b)[1])
	m := regexp.MustCompile(`\n\t\tif _js_regexp_out_of_memory\((bp\+\d+)\) \{\n\t\t\tXJS_ThrowOutOfMemory\(tls, `+regexp.QuoteMeta(ctx)+`\)\n\t\t`).FindAllSubmatchIndex(body, -1)
	if len(m) != 1 {
		return nil, fmt.Errorf("_js_compile_regexp: cannot find the test fixOutOfMemory writes")
	}
	at, branch := m[0][1], regexInterruptBranch(string(body[m[0][2]:m[0][3]]), ctx)
	switch n := bytes.Count(body, []byte("_js_regexp_interrupted(")); {
	case n == 1 && bytes.HasPrefix(body[at:], []byte(branch+"} else {\n")):
		return b, nil
	case n != 0 || !bytes.HasPrefix(body[at:], []byte("} else {\n")):
		return nil, fmt.Errorf("_js_compile_regexp: _js_regexp_interrupted is not where fixlib puts it")
	}
	fixed := splice(body, at, branch)
	return bytes.Replace(b, body, fixed, 1), nil
}

// unfixRegexPoll gives text without the steps fixRegexPoll counts.
func unfixRegexPoll(text []byte) []byte {
	for _, re := range []*regexp.Regexp{reAlternativeHead, reDisjunctionHead, reClassHead} {
		m := re.FindSubmatch(text)
		if m == nil {
			continue
		}
		s := string(m[1])
		for _, add := range []string{pollTerm(s), pollBackward(s)} {
			text = bytes.ReplaceAll(text, []byte(add), nil)
		}
		if re == reClassHead {
			text = bytes.ReplaceAll(text, []byte(pollAtom(s, string(m[2]))), nil)
		}
		if re == reDisjunctionHead {
			text = reSplitPoll(s).ReplaceAll(text, nil)
		}
	}
	return text
}

// regexPollFingerprint hashes regexPollFuncs, without the change of
// fixRegexPoll, with each C string written out in place of its offset.
func regexPollFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, regexPollFuncs, unfixRegexPoll)
}
