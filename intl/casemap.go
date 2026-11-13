// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"sync"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/liustack/quickjs"
	gointl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/hostmem"
	"github.com/liustack/quickjs/internal/intlhook"
	"github.com/liustack/quickjs/internal/vmhook"
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
	c, err := goCanonicalizer()
	if err != nil {
		return "", err
	}

	l, err := c.Canonicalize(tag)
	if err != nil {
		return "", rangeError("Incorrect locale information provided")
	}

	switch lang := l.Language.String(); lang {
	case "tr", "az":
		return "tr", nil
	case "lt", "el":
		return lang, nil
	}
	return "", nil
}

// caseRunes is s, a string as the engine holds it in WTF-8, as code points,
// a lone surrogate as itself.
func caseRunes(w *walk, s string, units int) []rune {
	r := w.runes(units)
	for i := 0; i < len(s); {
		w.step()
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

// A walk counts the steps of the passes of one call over a long text, the
// scans back and forth for a condition of the case rules included, and
// checks the call every intlhook.Piece steps, so that no pass runs whole
// with no check: toLocaleUpperCase('el') of 8 Mi units took 0.9 s on an
// Apple M4 with none. It also counts what Go takes for the call, before
// each allocation that grows with the text, and ends the call with the out
// of memory error when the VM has no room for it (TestCaseRoomCoversGo).
type walk struct {
	vm    *quickjs.VM
	steps int
	held  int64
	// gives gives back the arrays of code points of the call (runes).
	gives []func()
	// room is the room of the VM, -1 for no limit, or unknownRoom before it
	// is asked: what Go takes does not change it, so it is asked once, when
	// the call first holds more than the door's fixed room, which the door
	// checked when it read the first argument.
	room int64
}

// unknownRoom is the room of a walk that has not asked the VM yet.
const unknownRoom = -2

// newWalk is the walk of a call on vm.
func newWalk(vm *quickjs.VM) *walk { return &walk{vm: vm, room: unknownRoom} }

// read tells w that the string was read, which may have joined it in the
// VM and taken some of the room: the room is asked again.
func (w *walk) read() { w.room = unknownRoom }

func (w *walk) step() {
	if w.steps++; w.steps == intlhook.Piece {
		w.check()
	}
}

// check is step's check of the call, apart so that step inlines.
//
//go:noinline
func (w *walk) check() {
	w.steps = 0
	intlhook.Check(w.vm)
}

// take counts n bytes more that Go takes for the call.
func (w *walk) take(n int64) {
	if w.held += n; w.held > intlhook.RoomFixed {
		w.over()
	}
}

// over checks the room for what the call holds past the door's fixed room.
//
//go:noinline
func (w *walk) over() {
	if w.room == unknownRoom {
		w.room = vmhook.Room(w.vm)
	}
	if w.room >= 0 && w.held > w.room {
		intlhook.Room(w.vm, w.held)
	}
}

// runes is an empty array with room for n code points: from 1 MiB a block
// (intlhook.Take), which done gives back. Go clears an allocation in one go,
// with no check of the call, while the OS hands over its pages; the pages of
// a block come as the walk writes them, a step at a time.
func (w *walk) runes(n int) []rune {
	if !hostmem.Mapped[rune](n) {
		return make([]rune, 0, n)
	}
	r, give := intlhook.Take[rune](w.vm, n)
	w.gives = append(w.gives, give)
	return r[:0]
}

// bytes is runes for an array of n bytes.
func (w *walk) bytes(n int) []byte {
	if !hostmem.Mapped[byte](n) {
		return make([]byte, 0, n)
	}
	b, give := intlhook.Take[byte](w.vm, n)
	w.gives = append(w.gives, give)
	return b[:0]
}

// done gives back the arrays of the walk: nothing may use them after.
func (w *walk) done() {
	for _, give := range w.gives {
		give()
	}
	w.gives = nil
}

// put appends the code points of from to out, which has room for them, a
// piece at a time with a check of the call between two.
func (w *walk) put(out, from []rune) []rune {
	for lo := 0; lo < len(from); lo += intlhook.Piece {
		if lo > 0 {
			w.check()
		}
		out = append(out, from[lo:min(lo+intlhook.Piece, len(from))]...)
	}
	return out
}

// begin is the output of a rule that first changes in at i: the code points
// before i as they are, in an array with room for those of in and extra
// more, counted first.
func (w *walk) begin(in []rune, i, extra int) []rune {
	w.take(4 * int64(len(in)+extra))
	return w.put(w.runes(len(in)+extra), in[:i])
}

// add appends c to out, in an array twice as long when out has no room for
// it, counted first.
func (w *walk) add(out []rune, c rune) []rune {
	if len(out) == cap(out) {
		out = w.grow(out)
	}
	return append(out, c)
}

// grow is out in an array of twice its length, and one more, apart so that
// add inlines.
//
//go:noinline
func (w *walk) grow(out []rune) []rune {
	w.take(8 * int64(len(out)+1))
	return w.put(w.runes(2*len(out)+1), out)
}

// pieceBytes is what Go takes for a piece of the result besides its text:
// its place in the list, which append may double, its value boxed in an
// interface, the smallest buffer of a builder, and the javascript value
// EmitMixed makes of it: about 124 bytes, in a text of lone surrogates and
// letters in turn.
const pieceBytes = 160

// premapCase maps in s, as WTF-8, what the case rules of lang, from
// caseLanguage, change, for the engine to map the rest: ok is false when
// they change nothing. The result is the text in pieces: strings, and lone
// surrogates as their code units, which a Go string cannot carry to the
// engine.
func premapCase(w *walk, s string, units int, lang string, upper bool) (pieces []any, ok bool, err error) {
	w.take(4 * int64(units))
	in := caseRunes(w, s, units)
	var out []rune
	switch {
	case lang == "tr" && upper:
		out = turkishUpper(w, in)
	case lang == "tr":
		out, err = turkishLower(w, in)
	case lang == "lt" && upper:
		out, err = lithuanianUpper(w, in)
	case lang == "lt":
		out, err = lithuanianLower(w, in)
	case lang == "el" && upper:
		out, err = greekUpper(w, in)
	}
	if out == nil || err != nil {
		return nil, false, err
	}

	// The strings are written in one array of bytes, counted first, and
	// are over it: they live until the walk is done, after the engine has
	// its own.
	size := 0
	var buf [utf8.UTFMax]byte
	for _, c := range out {
		w.step()
		if !isSurrogate(c) {
			size += len(utf8.AppendRune(buf[:0], c))
		}
	}
	w.take(int64(size))
	b := w.bytes(size)
	at := 0
	piece := func(p any) {
		w.take(pieceBytes)
		pieces = append(pieces, p)
	}
	text := func() {
		piece(unsafe.String(unsafe.SliceData(b[at:]), len(b)-at))
		at = len(b)
	}
	for _, c := range out {
		w.step()
		if isSurrogate(c) {
			if len(b) > at {
				text()
			}
			piece(int(c))
			continue
		}
		b = utf8.AppendRune(b, c)
	}
	if len(b) > at || len(pieces) == 0 {
		text()
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
func turkishUpper(w *walk, in []rune) []rune {
	var out []rune
	for i, c := range in {
		w.step()
		if c == 'i' {
			if out == nil {
				out = w.begin(in, i, 0)
			}
			c = 'İ'
		}
		if out != nil {
			out = w.add(out, c)
		}
	}
	return out
}

// turkishLower maps "İ" to "i", an "I" that no dot above follows to "ı",
// and drops the dot above after an "I", or answers nil where none is.
func turkishLower(w *walk, in []rune) ([]rune, error) {
	n, err := caseNormalizer()
	if err != nil {
		return nil, err
	}

	var out []rune
	for i, c := range in {
		w.step()
		m, drop := c, false
		switch c {
		case 'İ':
			m = 'i'
		case 'I':
			if !followedByDotAbove(w, n, in[i+1:]) {
				m = 'ı'
			}
		case 0x307:
			drop = precededByI(w, n, in[:i])
		}
		if out == nil && (m != c || drop) {
			out = w.begin(in, i, 0)
		}
		if out != nil && !drop {
			out = w.add(out, m)
		}
	}
	return out, nil
}

// followedByDotAbove is SpecialCasing.txt's Before_Dot for what follows a
// character.
func followedByDotAbove(w *walk, n *gointl.Normalizer, after []rune) bool {
	for _, c := range after {
		w.step()
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
func precededByI(w *walk, n *gointl.Normalizer, before []rune) bool {
	for i := len(before) - 1; i >= 0; i-- {
		w.step()
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
func lithuanianUpper(w *walk, in []rune) ([]rune, error) {
	n, err := caseNormalizer()
	if err != nil {
		return nil, err
	}

	var out []rune
	for i, c := range in {
		w.step()
		drop := c == 0x307 && precededBySoftDotted(w, n, in[:i])
		if out == nil && drop {
			out = w.begin(in, i, 0)
		}
		if out != nil && !drop {
			out = w.add(out, c)
		}
	}
	return out, nil
}

// precededBySoftDotted is SpecialCasing.txt's After_Soft_Dotted.
func precededBySoftDotted(w *walk, n *gointl.Normalizer, before []rune) bool {
	for i := len(before) - 1; i >= 0; i-- {
		w.step()
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
func lithuanianLower(w *walk, in []rune) ([]rune, error) {
	n, err := caseNormalizer()
	if err != nil {
		return nil, err
	}

	var out []rune
	for i, c := range in {
		w.step()
		var mb [3]rune
		var m []rune
		switch c {
		case 'I', 'J', 'Į':
			if followedByMoreAbove(w, n, in[i+1:]) {
				m = append(mb[:0], unicode.ToLower(c), 0x307)
			}
		case 'Ì':
			m = append(mb[:0], 'i', 0x307, 0x300)
		case 'Í':
			m = append(mb[:0], 'i', 0x307, 0x301)
		case 'Ĩ':
			m = append(mb[:0], 'i', 0x307, 0x303)
		}
		if out == nil && m != nil {
			out = w.begin(in, i, 2)
		}
		switch {
		case m != nil:
			for _, r := range m {
				out = w.add(out, r)
			}
		case out != nil:
			out = w.add(out, c)
		}
	}
	return out, nil
}

// followedByMoreAbove is SpecialCasing.txt's More_Above.
func followedByMoreAbove(w *walk, n *gointl.Normalizer, after []rune) bool {
	for _, c := range after {
		w.step()
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
func followedByCased(w *walk, after []rune) bool {
	for _, c := range after {
		w.step()
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
func greekUpper(w *walk, in []rune) ([]rune, error) {
	letters, err := greekLetters()
	if err != nil {
		return nil, err
	}

	var out []rune
	state := 0
	for i := 0; i < len(in); {
		w.step()
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
				out = w.add(out, c)
			}
			i, state = next, nextState
			continue
		}

		if out == nil {
			out = w.begin(in, i, 4)
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
			w.step()
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
			!followedByCased(w, in[next:]):
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
		out = w.add(out, up)
		if data&greekEitherDialytika != 0 {
			out = w.add(out, 0x308)
		}
		if addTonos {
			out = w.add(out, 0x301)
		}
		for ; ypogegrammeni > 0; ypogegrammeni-- {
			out = w.add(out, 'Ι')
		}
		i, state = next, nextState
	}
	return out, nil
}
