// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	gointl "github.com/liustack/quickjs/internal/gointl"
)

// toLocaleUpperCase and toLocaleLowerCase map case as ICU does for the
// languages V8 hands it: Turkish and Azeri, Lithuanian and, upper case
// only, Greek. Every other language maps as the engine's toUpperCase and
// toLowerCase do, which are Unicode's default full mappings. So Go maps only
// what the language's rules change, before the engine maps the rest: a
// Turkish "i" to "İ", which the engine then leaves as it is.

// caseLanguage is the language whose case rules apply to tag, the first of
// the locales as V8 reads them: "tr" for Turkish and Azeri, "lt", "el", or
// "" for the default rules. A tag that is not one is a RangeError.
func caseLanguage(tag string) (string, error) {
	l, err := goLocales([]string{tag})
	if err != nil {
		return "", err
	}

	switch lang := l[0].Language.String(); lang {
	case "tr", "az":
		return "tr", nil
	case "lt", "el":
		return lang, nil
	}
	return "", nil
}

// caseRunes is s, a string as the engine holds it in WTF-8, as code points,
// a lone surrogate as itself.
func caseRunes(s string) []rune {
	r := make([]rune, 0, len(s))
	for i := 0; i < len(s); {
		if i+2 < len(s) && s[i] == 0xed && s[i+1] >= 0xa0 {
			r = append(r, rune(s[i]&0x0f)<<12|rune(s[i+1]&0x3f)<<6|rune(s[i+2]&0x3f))
			i += 3
			continue
		}

		c, n := utf8.DecodeRuneInString(s[i:])
		r = append(r, c)
		i += n
	}
	return r
}

func isSurrogate(c rune) bool { return c >= 0xd800 && c <= 0xdfff }

// premapCase maps in s, as WTF-8, what the case rules of lang, from
// caseLanguage, change, for the engine to map the rest: ok is false when
// they change nothing. The result is the text in pieces: strings, and lone
// surrogates as their code units, which a Go string cannot carry to the
// engine.
func premapCase(s, lang string, upper bool) (pieces []any, ok bool, err error) {
	in := caseRunes(s)
	var out []rune
	switch {
	case lang == "tr" && upper:
		out = turkishUpper(in)
	case lang == "tr":
		out, err = turkishLower(in)
	case lang == "lt" && upper:
		out, err = lithuanianUpper(in)
	case lang == "lt":
		out, err = lithuanianLower(in)
	case lang == "el" && upper:
		out, err = greekUpper(in)
	}
	if out == nil || err != nil {
		return nil, false, err
	}

	var b strings.Builder
	for _, c := range out {
		if isSurrogate(c) {
			if b.Len() > 0 {
				pieces = append(pieces, b.String())
				b.Reset()
			}
			pieces = append(pieces, int(c))
			continue
		}

		b.WriteRune(c)
	}
	if b.Len() > 0 || len(pieces) == 0 {
		pieces = append(pieces, b.String())
	}
	return pieces, true, nil
}

var caseNormalizer = sync.OnceValues(gointl.NewNormalizer)

// dotType is ICU's dot type of c, which the conditions of SpecialCasing.txt
// look at: soft dotted, as "i", a combining mark above, of combining class
// 230, another combining mark, or none of those.
type dotType int

const (
	noDot dotType = iota
	softDotted
	above
	otherAccent
)

func dotTypeOf(n *gointl.Normalizer, c rune) dotType {
	if unicode.Is(unicode.Soft_Dotted, c) {
		return softDotted
	}
	switch n.CombiningClass(c) {
	case 0:
		return noDot
	case 230:
		return above
	}
	return otherAccent
}

// turkishUpper maps "i" to "İ", or answers nil where there is none.
func turkishUpper(in []rune) []rune {
	var out []rune
	for i, c := range in {
		if c == 'i' {
			if out == nil {
				out = append(make([]rune, 0, len(in)), in[:i]...)
			}
			c = 'İ'
		}
		if out != nil {
			out = append(out, c)
		}
	}
	return out
}

// turkishLower maps "İ" to "i", an "I" that no dot above follows to "ı",
// and drops the dot above after an "I", or answers nil where none is.
func turkishLower(in []rune) ([]rune, error) {
	n, err := caseNormalizer()
	if err != nil {
		return nil, err
	}

	var out []rune
	for i, c := range in {
		m, drop := c, false
		switch c {
		case 'İ':
			m = 'i'
		case 'I':
			if !followedByDotAbove(n, in[i+1:]) {
				m = 'ı'
			}
		case 0x307:
			drop = precededByI(n, in[:i])
		}
		if out == nil && (m != c || drop) {
			out = append(make([]rune, 0, len(in)), in[:i]...)
		}
		if out != nil && !drop {
			out = append(out, m)
		}
	}
	return out, nil
}

// followedByDotAbove is SpecialCasing.txt's Before_Dot for what follows a
// character.
func followedByDotAbove(n *gointl.Normalizer, after []rune) bool {
	for _, c := range after {
		if c == 0x307 {
			return true
		}
		if dotTypeOf(n, c) != otherAccent {
			return false
		}
	}
	return false
}

// precededByI is SpecialCasing.txt's After_I for what precedes a character.
func precededByI(n *gointl.Normalizer, before []rune) bool {
	for i := len(before) - 1; i >= 0; i-- {
		if before[i] == 'I' {
			return true
		}
		if dotTypeOf(n, before[i]) != otherAccent {
			return false
		}
	}
	return false
}

// lithuanianUpper drops a dot above after a soft dotted letter, which the
// upper case letter has no dot for, or answers nil where none is.
func lithuanianUpper(in []rune) ([]rune, error) {
	n, err := caseNormalizer()
	if err != nil {
		return nil, err
	}

	var out []rune
	for i, c := range in {
		drop := c == 0x307 && precededBySoftDotted(n, in[:i])
		if out == nil && drop {
			out = append(make([]rune, 0, len(in)), in[:i]...)
		}
		if out != nil && !drop {
			out = append(out, c)
		}
	}
	return out, nil
}

// precededBySoftDotted is SpecialCasing.txt's After_Soft_Dotted.
func precededBySoftDotted(n *gointl.Normalizer, before []rune) bool {
	for i := len(before) - 1; i >= 0; i-- {
		switch dotTypeOf(n, before[i]) {
		case softDotted:
			return true
		case otherAccent:
		default:
			return false
		}
	}
	return false
}

// lithuanianLower keeps the dot of "i" and "j" with an accent above: "I",
// "J" and "Į" with one following, and "Ì", "Í" and "Ĩ", which have one,
// map with a dot above. It answers nil where none is.
func lithuanianLower(in []rune) ([]rune, error) {
	n, err := caseNormalizer()
	if err != nil {
		return nil, err
	}

	var out []rune
	for i, c := range in {
		var m []rune
		switch c {
		case 'I', 'J', 'Į':
			if followedByMoreAbove(n, in[i+1:]) {
				m = []rune{unicode.ToLower(c), 0x307}
			}
		case 'Ì':
			m = []rune{'i', 0x307, 0x300}
		case 'Í':
			m = []rune{'i', 0x307, 0x301}
		case 'Ĩ':
			m = []rune{'i', 0x307, 0x303}
		}
		if out == nil && m != nil {
			out = append(make([]rune, 0, len(in)+2), in[:i]...)
		}
		switch {
		case m != nil:
			out = append(out, m...)
		case out != nil:
			out = append(out, c)
		}
	}
	return out, nil
}

// followedByMoreAbove is SpecialCasing.txt's More_Above.
func followedByMoreAbove(n *gointl.Normalizer, after []rune) bool {
	for _, c := range after {
		switch dotTypeOf(n, c) {
		case above:
			return true
		case otherAccent:
		default:
			return false
		}
	}
	return false
}

// The Greek upper case of ICU (ucasemap.cpp's GreekUpper): a letter is its
// capital without accents or breathings, but for the disjunctive "ή", a
// dialytika stays, and is added to an iota or upsilon after a vowel that
// lost its accent, as "άι" is "ΑΪ", and an iota subscript is a capital iota.
// A letter's data is its capital and these flags.
const (
	greekVowel              = 0x1000
	greekYpogegrammeni      = 0x2000
	greekAccent             = 0x4000
	greekDialytika          = 0x8000
	greekCombiningDialytika = 0x10000
	greekOtherDiacritic     = 0x20000
	greekUpperMask          = 0x3ff
	greekEitherDialytika    = greekDialytika | greekCombiningDialytika

	// The states between letters.
	greekAfterCased                      = 1
	greekAfterVowelWithPrecomposedAccent = 2
	greekAfterVowelWithCombiningAccent   = 4
	greekAfterVowelWithAccent            = greekAfterVowelWithPrecomposedAccent | greekAfterVowelWithCombiningAccent
	greekVowelAccentAndDialytka          = greekVowel | greekAccent | greekDialytika
)

// greekLetters are the data of the Greek letters and their spellings with
// diacritics, worked out from their decompositions as ICU's tables are.
var greekLetters = sync.OnceValues(func() (map[rune]uint32, error) {
	n, err := caseNormalizer()
	if err != nil {
		return nil, err
	}

	m := map[rune]uint32{}
	add := func(c rune) {
		d := []rune(n.Normalize(string(c), gointl.NFD))
		base := d[0]
		if !unicode.Is(unicode.Greek, base) || !unicode.IsLetter(base) {
			return
		}

		up := unicode.ToUpper(base)
		if up == base && !unicode.IsUpper(base) || up > greekUpperMask {
			return
		}

		data := uint32(up)
		switch up {
		case 'Α', 'Ε', 'Η', 'Ι', 'Ο', 'Υ', 'Ω':
			data |= greekVowel
		}
		for _, mark := range d[1:] {
			f := greekDiacritic(mark)
			if f == greekCombiningDialytika {
				// Spelled with the letter, which a combining one after it
				// is told apart from.
				f = greekDialytika
			}
			data |= f
		}
		m[c] = data
	}
	for c := rune(0x370); c <= 0x3ff; c++ {
		add(c)
	}
	for c := rune(0x1f00); c <= 0x1fff; c++ {
		add(c)
	}
	add(0x2126)
	return m, nil
})

// greekDiacritic is the flag of a combining diacritic after a Greek letter,
// or 0 for any other character.
func greekDiacritic(c rune) uint32 {
	switch c {
	case 0x300, 0x301, 0x342, 0x302, 0x303, 0x311:
		return greekAccent
	case 0x308:
		return greekCombiningDialytika
	case 0x344:
		return greekCombiningDialytika | greekAccent
	case 0x345:
		return greekYpogegrammeni
	case 0x304, 0x306, 0x313, 0x314, 0x343:
		return greekOtherDiacritic
	}
	return 0
}

// isCased and isCaseIgnorable are Unicode's Cased and Case_Ignorable, as
// ICU's ucase_getTypeOrIgnorable has them, from Go's tables.
func isCased(c rune) bool {
	return unicode.IsUpper(c) || unicode.IsLower(c) || unicode.IsTitle(c) ||
		unicode.Is(unicode.Other_Lowercase, c) || unicode.Is(unicode.Other_Uppercase, c)
}

func isCaseIgnorable(c rune) bool {
	switch c {
	// Word_Break MidLetter, MidNumLet and Single_Quote.
	case 0x27, 0x2e, 0x3a, 0xb7, 0x387, 0x55f, 0x5f4, 0x2018, 0x2019, 0x2024, 0x2027, 0xfe13, 0xfe52, 0xfe55,
		0xff07, 0xff0e, 0xff1a:
		return true
	}
	return unicode.In(c, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

// followedByCased reports whether a cased letter follows, past case
// ignorable characters.
func followedByCased(after []rune) bool {
	for _, c := range after {
		switch {
		case isCaseIgnorable(c):
		case isCased(c):
			return true
		default:
			return false
		}
	}
	return false
}

// greekUpper maps the Greek letters of in, and the diacritics after them,
// to their capitals, or answers nil where it has none.
func greekUpper(in []rune) ([]rune, error) {
	letters, err := greekLetters()
	if err != nil {
		return nil, err
	}

	var out []rune
	state := 0
	for i := 0; i < len(in); {
		c := in[i]
		next := i + 1
		nextState := 0
		switch {
		case isCaseIgnorable(c):
			nextState |= state & greekAfterCased
		case isCased(c):
			nextState |= greekAfterCased
		}
		data := letters[c]
		if data == 0 {
			if out != nil {
				out = append(out, c)
			}
			i, state = next, nextState
			continue
		}

		if out == nil {
			out = append(make([]rune, 0, len(in)+4), in[:i]...)
		}
		up := rune(data & greekUpperMask)
		if data&greekVowel != 0 && state&greekAfterVowelWithAccent != 0 && (up == 'Ι' || up == 'Υ') {
			// Spelled as the accent was: "α\u0301ι" is "ΑΙ\u0308".
			if state&greekAfterVowelWithPrecomposedAccent != 0 {
				data |= greekDialytika
			} else {
				data |= greekCombiningDialytika
			}
		}
		ypogegrammeni := 0
		if data&greekYpogegrammeni != 0 {
			ypogegrammeni = 1
		}
		precomposedAccent := data&greekAccent != 0
		for next < len(in) {
			f := greekDiacritic(in[next])
			if f == 0 {
				break
			}

			data |= f
			if f&greekYpogegrammeni != 0 {
				ypogegrammeni++
			}
			next++
		}
		if data&greekVowelAccentAndDialytka == greekVowel|greekAccent {
			if precomposedAccent {
				nextState |= greekAfterVowelWithPrecomposedAccent
			} else {
				nextState |= greekAfterVowelWithCombiningAccent
			}
		}
		addTonos := false
		switch {
		case up == 'Η' && data&greekAccent != 0 && ypogegrammeni == 0 && state&greekAfterCased == 0 &&
			!followedByCased(in[next:]):
			// The disjunctive "ή", "or", keeps its accent.
			if precomposedAccent {
				up = 'Ή'
			} else {
				addTonos = true
			}
		case data&greekDialytika != 0 && up == 'Ι':
			up, data = 'Ϊ', data&^greekEitherDialytika
		case data&greekDialytika != 0 && up == 'Υ':
			up, data = 'Ϋ', data&^greekEitherDialytika
		}
		out = append(out, up)
		if data&greekEitherDialytika != 0 {
			out = append(out, 0x308)
		}
		if addTonos {
			out = append(out, 0x301)
		}
		for ; ypogegrammeni > 0; ypogegrammeni-- {
			out = append(out, 'Ι')
		}
		i, state = next, nextState
	}
	return out, nil
}
