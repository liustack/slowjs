// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
)

// fixClassEscapes makes the regexp parser read the characters of a class of
// a regexp with the v flag (unicodeSets) as the specification does, the Go
// form of the same change in lib/internal/patch/quickjs.diff:
//
//   - _get_class_atom takes \ and one of & ! # % , : ; < = > @ ` ~ (a
//     ClassSetReservedPunctuator other than -, which it took already) as
//     that character in a class of a v regexp. It threw "invalid escape
//     sequence", so [\&\;] and [\!\|], which shiki's grammars are full of,
//     did not compile. Anywhere else the escape stays an error with u or v.
//   - _get_class_atom stops on a doubled punctuator, such as &&, and on
//     ( ) [ ] { } / - | only in a class, as upstream's commit a479eff
//     ("fixed regexp unicode set parsing") does: the parser reads a
//     character outside a class with it too, so /-/v, /a&&b/v and /\//v
//     did not compile.
//   - _parse_class_string_disjunction reads the characters of \q{...} as
//     characters of a class, so that \q{\-} compiles, and \q{(} and
//     \q{&&} still throw once _get_class_atom checks those only in a class.
//
// The text of the two functions and of lre_parse_escape, which still reads
// every other escape, is pinned in classEscapeSites: when a release changes
// one of them, fixClassEscapes stops, and the new code must be read before
// the fingerprints are updated (go run ./internal/fixlib -classescape
// prints them). The fingerprints are of the text without the change, after
// every other change before it.
func fixClassEscapes(b []byte, file string) ([]byte, error) {
	b, err := unfixClassEscapes(b)
	if err != nil {
		return nil, err
	}
	got, err := classEscapeFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := classEscapeSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against fixClassEscapes, then update classEscapeSites (go run ./internal/fixlib -classescape)", file, "get_class_atom, parse_class_string_disjunction or lre_parse_escape", got, want)
	}
	body, err := funcBody(b, reClassAtomHead, "_get_class_atom")
	if err != nil {
		return nil, err
	}
	fixed, err := classAtomFix(body)
	if err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)
	if body, err = funcBody(b, reClassStringHead, "_parse_class_string_disjunction"); err != nil {
		return nil, err
	}
	m := findAllSubmatch(reClassStringAtom, body)
	if len(m) != 1 {
		return nil, fmt.Errorf("_parse_class_string_disjunction: want 1 call of _get_class_atom outside a class, found %d", len(m))
	}
	fixed = bytes.Replace(body, m[0][0], []byte(classStringAtom(string(m[0][1]), string(m[0][2]), string(m[0][3]), string(m[0][4]))), 1)
	return bytes.Replace(b, body, fixed, 1), nil
}

var (
	reClassAtomHead   = regexp.MustCompile(`\nfunc _get_class_atom\(tls \*libc\.TLS, (\w+) uintptr, \w+ uintptr, pp uintptr, inclass TBOOL\) \(r int32\) \{\n`)
	reClassStringHead = regexp.MustCompile(`\nfunc _parse_class_string_disjunction\(tls \*libc\.TLS, \w+ uintptr, \w+ uintptr, pp uintptr\) \(r int32\) \{\n`)
	// The call that reads one character of \q{...}: the indent, the
	// result, the parse state and the room of the pointer.
	reClassStringAtom = regexp.MustCompile(`\n(\t+)(\w+) = _get_class_atom\(tls, (\w+), libc\.UintptrFromInt32\(0\), (bp(?:\+\d+)?), int32\(EFALSE\)\)\n`)
	// The switch over the character after \, and the label it goes to when
	// it is done, as C's break.
	reEscapeSwitch = regexp.MustCompile(`\n\tswitch c \{\n\tcase uint32\('d'\):\n\t\tgoto _\d+\n(?:\t(?:case [^\n]*|default):\n\t\tgoto _\d+\n)*\t\}\n\tgoto (_\d+)\n`)
	// The tests of a doubled punctuator and of the characters a class of
	// a v regexp cannot hold, by their comments.
	reDoubleTest = regexp.MustCompile(`\n\tif \(\*TREParseState\)\(unsafe\.Pointer\((\w+)\)\)\.Funicode_sets != 0 && [^\n]* == c \{\n\t\t/\* forbidden double characters \*/\n`)
	reSyntaxTest = regexp.MustCompile(`\n\tif \(\*TREParseState\)\(unsafe\.Pointer\((\w+)\)\)\.Funicode_sets != 0 \{\n\t\t/\* invalid characters in unicode sets \*/\n`)

	// The goto of classEscape, and the call of classStringAtom with its
	// comment.
	reClassEscapeDone  = regexp.MustCompile(`\n\t\t\tgoto (_\d+)\n`)
	reClassStringFixed = regexp.MustCompile(`\n(\t+)` + regexp.QuoteMeta(classStringNote) + `(\t+)(\w+) = _get_class_atom\(tls, (\w+), libc\.UintptrFromInt32\(0\), (bp(?:\+\d+)?), int32\(ETRUE\)\)\n`)

	classEscapeFuncs = []string{"_get_class_atom", "_parse_class_string_disjunction", "Xlre_parse_escape"}
)

const (
	classEscapeNote = "\t// Changed by internal/fixlib: \\ and a ClassSetReservedPunctuator is that character in a class of a v regexp (see fixClassEscapes).\n"
	classTestNote   = "\t// Changed by internal/fixlib: only a class of a v regexp is checked (see fixClassEscapes).\n"
	classStringNote = "// Changed by internal/fixlib: the characters of \\q{...} are read as in a class (see fixClassEscapes).\n"
)

// classEscape is the test fixClassEscapes puts before the switch over the
// character after \, with s the parse state and done the label of the end
// of the switch.
func classEscape(s, done string) string {
	return classEscapeNote +
		"\tif inclass != 0 && (*TREParseState)(unsafe.Pointer(" + s + ")).Funicode_sets != 0 {\n" +
		"\t\tswitch c {\n" +
		"\t\tcase uint32('&'), uint32('!'), uint32('#'), uint32('%'), uint32(','), uint32(':'), uint32(';'), uint32('<'), uint32('='), uint32('>'), uint32('@'), uint32('`'), uint32('~'):\n" +
		"\t\t\tgoto " + done + "\n" +
		"\t\t}\n" +
		"\t}\n"
}

// classStringAtom is the call that reads one character of \q{...} as a
// character of a class.
func classStringAtom(indent, c, s, p string) string {
	return "\n" + indent + classStringNote +
		indent + c + " = _get_class_atom(tls, " + s + ", libc.UintptrFromInt32(0), " + p + ", int32(ETRUE))\n"
}

// classAtomFix makes the changes to body, the text of _get_class_atom
// without them.
func classAtomFix(body []byte) ([]byte, error) {
	s := string(reClassAtomHead.FindSubmatch(body)[1])
	sw := findAll(reEscapeSwitch, body)
	if len(sw) != 1 {
		return nil, fmt.Errorf("_get_class_atom: want 1 switch over the character after \\, found %d", len(sw))
	}
	done := string(body[sw[0][2]:sw[0][3]])
	if n := bytes.Count(body, []byte("\n"+done+":\n")); n != 1 {
		return nil, fmt.Errorf("_get_class_atom: want 1 label %s after the switch over the character after \\, found %d", done, n)
	}
	for _, re := range []*regexp.Regexp{reDoubleTest, reSyntaxTest} {
		m := findAllSubmatch(re, body)
		if len(m) != 1 || string(m[0][1]) != s {
			return nil, fmt.Errorf("_get_class_atom: cannot find the test %q of a v regexp", re.String())
		}
	}
	body = reSyntaxTest.ReplaceAllFunc(body, func(m []byte) []byte {
		return append([]byte("\n"+classTestNote+"\tif inclass != 0 && "), m[len("\n\tif "):]...)
	})
	body = reDoubleTest.ReplaceAllFunc(body, func(m []byte) []byte {
		return append([]byte("\n"+classTestNote+"\tif inclass != 0 && "), m[len("\n\tif "):]...)
	})
	return splice(body, sw[0][0]+1, classEscape(s, done)), nil
}

// unfixClassEscapes gives b without the change of fixClassEscapes, and
// checks that what it takes out is all of the change, in the form fixlib
// writes it.
func unfixClassEscapes(b []byte) ([]byte, error) {
	if !bytes.Contains(b, []byte("(see fixClassEscapes)")) {
		return b, nil
	}
	body, err := funcBody(b, reClassAtomHead, "_get_class_atom")
	if err != nil {
		return nil, err
	}
	s := string(reClassAtomHead.FindSubmatch(body)[1])
	bare := bytes.ReplaceAll(body, []byte(classTestNote+"\tif inclass != 0 && "), []byte("\tif "))
	if i := bytes.Index(bare, []byte(classEscapeNote)); i >= 0 {
		// The label is the one the first goto after the comment names.
		if done := reClassEscapeDone.FindSubmatch(bare[i:]); done != nil {
			bare = bytes.Replace(bare, []byte(classEscape(s, string(done[1]))), nil, 1)
		}
	}
	b = bytes.Replace(b, body, bare, 1)
	if body, err = funcBody(b, reClassStringHead, "_parse_class_string_disjunction"); err != nil {
		return nil, err
	}
	for _, m := range reClassStringFixed.FindAllSubmatch(body, -1) {
		if string(m[1]) != string(m[2]) {
			continue
		}
		bare := bytes.Replace(body, m[0], []byte("\n"+string(m[1])+string(m[3])+" = _get_class_atom(tls, "+string(m[4])+", libc.UintptrFromInt32(0), "+string(m[5])+", int32(EFALSE))\n"), 1)
		b = bytes.Replace(b, body, bare, 1)
		body = bare
	}
	if bytes.Contains(b, []byte("(see fixClassEscapes)")) {
		return nil, fmt.Errorf("the change of fixClassEscapes is not as fixlib writes it: read the code around \"(see fixClassEscapes)\"")
	}
	return b, nil
}

// classEscapeFingerprint hashes classEscapeFuncs, without the change of
// fixClassEscapes, with each C string written out in place of its offset.
func classEscapeFingerprint(b []byte) (string, error) {
	b, err := unfixClassEscapes(b)
	if err != nil {
		return "", err
	}
	return funcsFingerprint(b, classEscapeFuncs, func(text []byte) []byte { return text })
}
