package intl

import (
	"github.com/liustack/quickjs/internal/gointl/internal/colldata"
)

// Collation elements, and comparing strings by them.
//
// Every character is turned into collation elements, 64 bits each:
//
//	bits 63..32  primary weight: the letter
//	bits 31..16  secondary weight: the accent
//	bits 15..14  case
//	bits 13..8   tertiary weight, with bits 5..0: the variant
//	bits  7..6   quaternary weight
//
// The tables store 32-bit elements, most of which expand to a 64-bit one by
// shifting bits into place. The rest are special: their low byte is 0xC0 or
// above, the four bits under it are a tag, and the tag says where the real
// elements are -- an expansion list, a contraction trie, a computed range.
// The encoding and the tags are ICU's, from collation.h.
//
// The text is decomposed first. ICU exports its tables for ICU4X without the
// canonical closure -- the entries a precomposed character would need -- on the
// understanding that the text they are used on is decomposed, and ECMA-402
// requires canonically equivalent strings to compare equal in any case.

const (
	specialCE32LowByte = 0xc0
	fallbackCE32       = specialCE32LowByte

	tagFallback       = 0
	tagLongPrimary    = 1
	tagLongSecondary  = 2
	tagLatinExpansion = 4
	tagExpansion32    = 5
	tagExpansion      = 6
	tagPrefix         = 8
	tagContraction    = 9
	tagDigit          = 10
	tagU0000          = 11
	tagHangul         = 12
	tagOffset         = 14
	tagImplicit       = 15

	contractSingleCPNoMatch = 0x100
	contractNextCCC         = 0x200
	contractTrailingCCC     = 0x400

	commonSecondaryCE = 0x05000000
	commonTertiaryCE  = 0x0500
	commonSecAndTerCE = commonSecondaryCE | commonTertiaryCE

	// The end of the text is an element of its own, with weights below
	// every real one at each level, so that the shorter string sorts first.
	noCE                  uint64 = 0x101000100
	noCEPrimary                  = 1
	noCEWeight16                 = 0x0100
	fffdCE32                     = 0xfffd0505
	mergeSeparatorPrimary        = 0x02000000

	unassignedImplicitByte = 0xfe

	tertiaryMask     = 0x3f3f
	caseMask         = 0xc000
	caseTertiaryMask = caseMask | tertiaryMask
)

// elementWriter turns decomposed text into collation elements.
type elementWriter struct {
	c    *Collator
	text []rune
	// consumed marks the combining marks a discontiguous contraction took
	// out of the middle of a run, so they are not weighed again.
	consumed []bool
	// classEnds is, for each mark, the index after the marks of its class
	// that follow it, made when a discontiguous contraction first needs it.
	classEnds []int
	out       []uint64
}

func (c *Collator) elements(s string) []uint64 { return c.elementsOf([]rune(s)) }

// elementsOf is elements, of the characters themselves.
func (c *Collator) elementsOf(text []rune) []uint64 { return c.elementsFrom(text, 0) }

// elementsFrom is elementsOf of the characters from index from, those
// before it read only as the context a prefix matches.
func (c *Collator) elementsFrom(text []rune, from int) []uint64 {
	w := elementWriter{c: c, text: text}
	w.out = make([]uint64, 0, len(w.text)-from+1)
	for i := from; i < len(w.text); {
		if w.consumed != nil && w.consumed[i] {
			i++
			continue
		}
		r := w.text[i]
		if c.lithuanianDotAbove && r == 0x307 {
			// Lithuanian writes a dot above an i that carries another
			// accent and does not weigh it. ICU4X keeps this as a setting
			// rather than as contractions; ICU4X's collator applies it to
			// a dot immediately followed by a grave, an acute or a tilde.
			if next := w.next(i); next < len(w.text) {
				switch w.text[next] {
				case 0x300, 0x301, 0x303:
					i++
					continue
				}
			}
		}
		// A lone surrogate weighs as a character no table names does, by
		// its code point, as ICU weighs one.
		if r >= 0xd800 && r <= 0xdfff {
			w.out = append(w.out, primaryCE(unassignedPrimary(r)))
			i = w.next(i)
			continue
		}
		// The combining diacritics weigh as an accent alone, from their own
		// table.
		if k := int(r) - colldata.DiacriticsBase; k >= 0 && k < len(c.diacritics) {
			w.out = append(w.out, uint64(c.diacritics[k])<<16|commonTertiaryCE)
			i = w.next(i)
			continue
		}
		d, ce32 := c.lookup(r)
		i = w.appendCE32(d, ce32, i)
	}
	return append(w.out, noCE)
}

// lookup finds a character's element in the tailoring, falling back to the
// root. The root's conjoining jamo are kept apart by the export, in a table
// of their own; a tailoring that changes them -- the search collations,
// Korean's searchjl -- is ICU's compiled one, which has them in its trie.
func (c *Collator) lookup(r rune) (*colldata.Data, uint32) {
	if c.tailoring != nil {
		if ce32 := c.tailoring.Trie.Get(r); ce32 != fallbackCE32 {
			return c.tailoring, ce32
		}
	}
	if k := int(r) - colldata.JamoBase; k >= 0 && k < len(c.root.Jamo) {
		return &c.root.Data, c.root.Jamo[k]
	}
	return &c.root.Data, c.root.Data.Trie.Get(r)
}

// next returns the index of the next character not yet consumed.
func (w *elementWriter) next(i int) int {
	for i++; i < len(w.text) && w.consumed != nil && w.consumed[i]; i++ {
	}
	return i
}

func (w *elementWriter) ccc(i int) uint8 {
	return w.c.normalizer.tables.Class(w.text[i])
}

func isSpecial(ce32 uint32) bool { return ce32&0xff >= specialCE32LowByte }

func tagOf(ce32 uint32) int { return int(ce32 & 0xf) }

// simpleCE expands an element that is not special, or is a long primary or a
// long secondary.
func simpleCE(ce32 uint32) (uint64, bool) {
	low := ce32 & 0xff
	if low < specialCE32LowByte {
		v := uint64(ce32)
		return (v&0xffff0000)<<32 | (v&0xff00)<<16 | uint64(low)<<8, true
	}
	switch tagOf(ce32) {
	case tagLongPrimary:
		return uint64(ce32&0xffffff00)<<32 | commonSecAndTerCE, true
	case tagLongSecondary:
		return uint64(ce32 & 0xffffff00), true
	}
	return 0, false
}

func primaryCE(p uint32) uint64 { return uint64(p)<<32 | commonSecAndTerCE }

// appendCE32 appends the elements one character's element stands for, and
// returns the index to go on from.
func (w *elementWriter) appendCE32(d *colldata.Data, ce32 uint32, i int) int {
	r := w.text[i]
	next := w.next(i)
	for steps := 0; ; steps++ {
		if steps > 16 {
			// Tables that point in a circle; ICU would loop.
			w.out = append(w.out, primaryCE(0xfffd0000))
			return next
		}
		if ce, ok := simpleCE(ce32); ok {
			w.out = append(w.out, ce)
			return next
		}
		switch tagOf(ce32) {
		case tagFallback:
			d = &w.c.root.Data
			ce32 = d.Trie.Get(r)
		case tagLatinExpansion:
			w.out = append(w.out,
				uint64(ce32&0xff000000)<<32|commonSecondaryCE|uint64(ce32&0xff0000)>>8,
				uint64(ce32&0xff00)<<16|commonTertiaryCE)
			return next
		case tagExpansion32:
			index, length := int(ce32>>13), int(ce32>>8&31)
			if !d.CE32s.Has(index, length) {
				w.out = append(w.out, primaryCE(0xfffd0000))
				return next
			}
			for k := index; k < index+length; k++ {
				ce, ok := simpleCE(d.CE32s.At(k))
				if !ok {
					ce = primaryCE(0xfffd0000)
				}
				w.out = append(w.out, ce)
			}
			return next
		case tagExpansion:
			index, length := int(ce32>>13), int(ce32>>8&31)
			if !d.CEs.Has(index, length) {
				w.out = append(w.out, primaryCE(0xfffd0000))
				return next
			}
			for k := index; k < index+length; k++ {
				w.out = append(w.out, d.CEs.At(k))
			}
			return next
		case tagPrefix:
			ce32 = w.prefix(d, ce32, i)
		case tagContraction:
			ce32, next = w.contraction(d, ce32, i)
		case tagDigit:
			if w.c.numeric {
				return w.numeric(i, ce32)
			}
			if !d.CE32s.Has(int(ce32>>13), 1) {
				ce32 = fffdCE32
				continue
			}
			ce32 = d.CE32s.At(int(ce32 >> 13))
		case tagU0000:
			if !d.CE32s.Has(0, 1) {
				ce32 = fffdCE32
				continue
			}
			ce32 = d.CE32s.At(0)
		case tagOffset:
			if !d.CEs.Has(int(ce32>>13), 1) {
				w.out = append(w.out, primaryCE(0xfffd0000))
				return next
			}
			w.out = append(w.out, primaryCE(offsetPrimary(d.CEs.At(int(ce32>>13)), r)))
			return next
		case tagImplicit:
			w.out = append(w.out, primaryCE(unassignedPrimary(r)))
			return next
		default:
			// Hangul syllables were decomposed before they got here, and
			// the other tags are never in runtime data.
			w.out = append(w.out, primaryCE(0xfffd0000))
			return next
		}
	}
}

// contextTrie reads a context entry: a default element, then a trie.
func contextTrie(d *colldata.Data, ce32 uint32) (uint32, colldata.CharTrie) {
	index := int(ce32 >> 13)
	if !d.Contexts.Has(index, 2) {
		// A context past the table, as a corrupt one may point to: U+FFFD's
		// weight and nothing to match.
		return fffdCE32, colldata.NewCharTrie(nil)
	}
	def := uint32(d.Contexts.At(index))<<16 | uint32(d.Contexts.At(index+1))
	return def, colldata.NewCharTrie(d.Contexts.From(index + 2))
}

// prefix matches the characters before this one, nearest first, against the
// prefixes the character has: a Japanese length mark weighs as the vowel
// before it.
func (w *elementWriter) prefix(d *colldata.Data, ce32 uint32, i int) uint32 {
	ce32, trie := contextTrie(d, ce32)
	for j := i - 1; j >= 0; j-- {
		m := trie.Next(w.text[j])
		if m.HasValue() {
			ce32 = trie.Value()
		}
		if !m.HasNext() {
			break
		}
	}
	return ce32
}

// contraction matches the characters after this one against the contractions
// it starts. It returns the element and the index to go on from.
//
// The longest contiguous match wins. After it, UCA's discontiguous matching
// may reach past combining marks for one that extends the match -- an "a"
// with a dot below and a ring above can contract with the ring -- as long as
// no skipped mark blocks it, by having an equal or higher combining class.
func (w *elementWriter) contraction(d *colldata.Data, ce32 uint32, i int) (uint32, int) {
	flags := ce32
	result, trie := contextTrie(d, ce32)
	k := w.next(i)
	if k >= len(w.text) {
		return result, k
	}
	if flags&contractNextCCC != 0 && w.ccc(k) == 0 {
		// Every suffix starts with a combining mark, and this is not one.
		return result, k
	}
	matched := flags&contractSingleCPNoMatch == 0
	end := k
	saved := trie
	m := trie.Next(w.text[k])
	for {
		if m.HasValue() {
			result = trie.Value()
			end = w.next(k)
			saved = trie
			matched = true
			if !m.HasNext() || end >= len(w.text) {
				return result, end
			}
			k = end
			m = trie.Next(w.text[k])
			continue
		}
		if m == colldata.NoMatch || w.next(k) >= len(w.text) {
			break
		}
		k = w.next(k)
		m = trie.Next(w.text[k])
	}
	// Back up to just after the last match.
	if flags&contractTrailingCCC != 0 && matched && end < len(w.text) && w.ccc(end) != 0 {
		result = w.discontiguous(saved, result, end)
	}
	return result, end
}

// discontiguous tries to extend a match with combining marks further along
// the run, skipping the mark at first that did not match. The marks it uses
// are marked consumed; the ones it skips are weighed after the contraction,
// in order.
func (w *elementWriter) discontiguous(state colldata.CharTrie, result uint32, first int) uint32 {
	prevCC := w.ccc(first)
	pos := w.next(first)
	if pos >= len(w.text) || w.ccc(pos) == 0 {
		// The mark that did not match is the only one.
		return result
	}
	for {
		cc := w.ccc(pos)
		matched := false
		if prevCC < cc {
			try := state
			if m := try.Next(w.text[pos]); m.HasValue() {
				result = try.Value()
				if w.consumed == nil {
					w.consumed = make([]bool, len(w.text))
				}
				w.consumed[pos] = true
				matched = true
				if !m.HasNext() {
					return result
				}
				state = try
			}
		}
		if !matched {
			if prevCC == cc {
				// No mark of this class can match either: they are blocked
				// as this one is. The text is in canonical order, which sorts
				// a run's marks by class, so they are the ones that follow,
				// and going past them at once keeps a long run of marks that
				// each begin a contraction, Tibetan's U+0F71, linear.
				pos = w.classEnd(pos) - 1
			}
			prevCC = cc
		}
		pos = w.next(pos)
		if pos >= len(w.text) || w.ccc(pos) == 0 {
			return result
		}
	}
}

// classEnd is the index after the marks of the class of the one at i that
// follow it.
func (w *elementWriter) classEnd(i int) int {
	if w.classEnds == nil {
		w.classEnds = make([]int, len(w.text))
		for k := len(w.text) - 1; k >= 0; k-- {
			cc := w.ccc(k)
			if k+1 < len(w.text) && cc != 0 && w.ccc(k+1) == cc {
				w.classEnds[k] = w.classEnds[k+1]
			} else {
				w.classEnds[k] = k + 1
			}
		}
	}
	return w.classEnds[i]
}

// numeric weighs a run of digits by its value, as ICU's
// CollationIterator::appendNumericCEs does, and returns the index after it.
// The first digit is the element in hand, as ICU's is, rather than looked
// up again, which a corrupt table could answer otherwise, leaving no digit
// and the writer where it was for ever.
func (w *elementWriter) numeric(i int, ce32 uint32) int {
	digits := []byte{byte(ce32 >> 8 & 0xf)}
	j := w.next(i)
	for j < len(w.text) {
		_, ce32 := w.c.lookup(w.text[j])
		if !isSpecial(ce32) || tagOf(ce32) != tagDigit {
			break
		}
		digits = append(digits, byte(ce32>>8&0xf))
		j = w.next(j)
	}
	lead := uint32(w.c.root.NumericPrimary) << 24
	pos := 0
	for pos < len(digits) {
		// Leading zeros do not count, but a number that is all zeros keeps
		// one.
		for pos < len(digits)-1 && digits[pos] == 0 {
			pos++
		}
		n := len(digits) - pos
		if n > 254 {
			n = 254
		}
		w.appendNumericSegment(lead, digits[pos:pos+n])
		pos += n
	}
	return j
}

func (w *elementWriter) appendNumericSegment(lead uint32, digits []byte) {
	// The second primary byte encodes the size of the number:
	//     74 byte values   2.. 75 for small numbers in two-byte primaries,
	//     40 byte values  76..115 for medium numbers in three-byte primaries,
	//     16 byte values 116..131 for large numbers in four-byte primaries,
	//    124 byte values 132..255 for very large numbers, 4..127 digit pairs.
	if len(digits) <= 7 {
		value := uint32(0)
		for _, d := range digits {
			value = value*10 + uint32(d)
		}
		firstByte, numBytes := uint32(2), uint32(74)
		if value < numBytes {
			w.out = append(w.out, primaryCE(lead|(firstByte+value)<<16))
			return
		}
		value -= numBytes
		firstByte += numBytes
		numBytes = 40
		if value < numBytes*254 {
			w.out = append(w.out, primaryCE(lead|(firstByte+value/254)<<16|(2+value%254)<<8))
			return
		}
		value -= numBytes * 254
		firstByte += numBytes
		numBytes = 16
		if value < numBytes*254*254 {
			p := lead | (2 + value%254)
			value /= 254
			p |= (2 + value%254) << 8
			value /= 254
			p |= (firstByte + value%254) << 16
			w.out = append(w.out, primaryCE(p))
			return
		}
	}
	// Digit pairs, each 11 + 2*pair so that the lowest bit is free to mark
	// the last one, with trailing 00 pairs left off.
	length := len(digits)
	pairs := uint32(length+1) / 2
	p := lead | (132-4+pairs)<<16
	for length >= 2 && digits[length-1] == 0 && digits[length-2] == 0 {
		length -= 2
	}
	k := 0
	var pair uint32
	if length&1 == 1 {
		pair = uint32(digits[0])
		k = 1
	} else {
		pair = uint32(digits[0])*10 + uint32(digits[1])
		k = 2
	}
	pair = 11 + 2*pair
	shift := uint32(8)
	for k+1 < length {
		if shift == 0 {
			p |= pair
			w.out = append(w.out, primaryCE(p))
			p = lead
			shift = 16
		} else {
			p |= pair << shift
			shift -= 8
		}
		pair = 11 + 2*(uint32(digits[k])*10+uint32(digits[k+1]))
		k += 2
	}
	p |= (pair - 1) << shift
	w.out = append(w.out, primaryCE(p))
}

// offsetPrimary computes a primary for a character in a range whose primaries
// run in code point order, as ICU's getThreeBytePrimaryForOffsetData does.
func offsetPrimary(dataCE uint64, c rune) uint32 {
	p := uint32(dataCE >> 32)
	lower := int32(uint32(dataCE))
	offset := (int32(c) - lower>>8) * (lower & 0x7f)
	compressible := lower&0x80 != 0
	offset += int32(p>>8&0xff) - 2
	primary := uint32(offset%254+2) << 8
	offset /= 254
	if compressible {
		offset += int32(p>>16&0xff) - 4
		primary |= uint32(offset%251+4) << 16
		offset /= 251
	} else {
		offset += int32(p>>16&0xff) - 2
		primary |= uint32(offset%254+2) << 16
		offset /= 254
	}
	return primary | (p&0xff000000 + uint32(offset)<<24)
}

// unassignedPrimary is the primary UCA computes for a character no table
// names, which sorts it by code point after everything that is named.
func unassignedPrimary(c rune) uint32 {
	n := uint32(c) + 1
	primary := 2 + n%18*14
	n /= 18
	primary |= (2 + n%254) << 8
	n /= 254
	primary |= (4 + n%251) << 16
	return primary | unassignedImplicitByte<<24
}

// reorder moves a primary to where the collation's script order puts it.
func (c *Collator) reorder(p uint32) uint32 {
	o := c.reordering
	if o == nil {
		return p
	}
	if b := o.Table[p>>24]; b != 0 || p <= noCEPrimary {
		return uint32(b)<<24 | p&0xffffff
	}
	if p >= o.MinHighNoReorder {
		return p
	}
	q := p | 0xffff
	for _, r := range o.Ranges {
		if q < r {
			return p + r<<24
		}
	}
	return p
}

func sign(less bool) int {
	if less {
		return -1
	}
	return 1
}

// compareElements compares two element lists level by level, as ICU's
// CollationCompare::compareUpToQuaternary does. The lists have had their
// variable elements shifted already.
func (c *Collator) compareElements(left, right []uint64) int {

	// Primary. A shifted element keeps its primary for the quaternary level
	// and has nothing else, which no ordinary element with a primary is
	// without; it does not count here.
	for li, ri := 0, 0; ; li, ri = li+1, ri+1 {
		for left[li]>>32 == 0 || uint32(left[li]) == 0 {
			li++
		}
		for right[ri]>>32 == 0 || uint32(right[ri]) == 0 {
			ri++
		}
		lp, rp := uint32(left[li]>>32), uint32(right[ri]>>32)
		if lp != rp {
			if c.reordering != nil {
				lp, rp = c.reorder(lp), c.reorder(rp)
			}
			return sign(lp < rp)
		}
		if lp == noCEPrimary {
			break
		}
	}

	// Secondary.
	if c.strength >= strengthSecondary {
		if !c.backwardSecondary {
			if r := compareLevel(left, right, secondaryOf); r != 0 {
				return r
			}
		} else if r := compareBackwardSecondary(left, right); r != 0 {
			return r
		}
	}

	// Case, when asked for as a level of its own.
	if c.caseLevel {
		if r := c.compareCaseLevel(left, right); r != 0 {
			return r
		}
	}

	// Tertiary.
	if c.strength < strengthTertiary {
		return 0
	}
	mask := uint16(tertiaryMask)
	if c.caseFirst != CaseFirstFalse && !c.caseLevel {
		mask = caseTertiaryMask
	}
	upperFirst := c.caseFirst == CaseFirstUpper
	anyQuaternary := uint32(0)
	for li, ri := 0, 0; ; li, ri = li+1, ri+1 {
		var lt, rt uint16
		for {
			anyQuaternary |= uint32(left[li])
			if lt = uint16(left[li]) & mask; lt != 0 {
				break
			}
			li++
		}
		for {
			anyQuaternary |= uint32(right[ri])
			if rt = uint16(right[ri]) & mask; rt != 0 {
				break
			}
			ri++
		}
		if lt != rt {
			if upperFirst {
				lt = upperFirstTertiary(lt, left[li])
				rt = upperFirstTertiary(rt, right[ri])
			}
			return sign(lt < rt)
		}
		if lt == noCEWeight16 {
			break
		}
	}
	if c.strength < strengthQuaternary {
		return 0
	}
	if !hasShifted(left) && !hasShifted(right) && anyQuaternary&0xc0 == 0 {
		return 0
	}

	// Quaternary.
	for li, ri := 0, 0; ; li, ri = li+1, ri+1 {
		var lq, rq uint32
		for {
			lq = quaternaryOf(left[li])
			if lq != 0 {
				break
			}
			li++
		}
		for {
			rq = quaternaryOf(right[ri])
			if rq != 0 {
				break
			}
			ri++
		}
		if lq != rq {
			if c.reordering != nil {
				lq, rq = c.reorder(lq), c.reorder(rq)
			}
			return sign(lq < rq)
		}
		if lq == noCEPrimary {
			break
		}
	}
	return 0
}

// shiftVariables rewrites variable elements in place: a variable element -- a
// space, a punctuation mark -- keeps only its primary, which moves to the
// quaternary level, and the ignorable elements after it go with it.
func (c *Collator) shiftVariables(ces []uint64) {
	shifting := false
	for i, ce := range ces {
		p := uint32(ce >> 32)
		switch {
		case p < c.variableTop && p > mergeSeparatorPrimary:
			ces[i] = ce &^ 0xffffffff
			shifting = true
		case p == 0 && shifting:
			ces[i] = 0
		case p != 0:
			shifting = false
		}
	}
}

// hasShifted reports whether a list has a shifted element: one with a
// primary and nothing else.
func hasShifted(ces []uint64) bool {
	for _, ce := range ces {
		if ce>>32 != 0 && uint32(ce) == 0 {
			return true
		}
	}
	return false
}

func secondaryOf(ce uint64) uint32 { return uint32(ce>>16) & 0xffff }

// compareLevel compares one level's weights, skipping the zeros.
func compareLevel(left, right []uint64, weight func(uint64) uint32) int {
	for li, ri := 0, 0; ; li, ri = li+1, ri+1 {
		var lw, rw uint32
		for lw = weight(left[li]); lw == 0; lw = weight(left[li]) {
			li++
		}
		for rw = weight(right[ri]); rw == 0; rw = weight(right[ri]) {
			ri++
		}
		if lw != rw {
			return sign(lw < rw)
		}
		if lw == noCEWeight16 {
			return 0
		}
	}
}

// compareBackwardSecondary compares the secondaries of each segment from its
// end, which is how Canadian French weighs accents: the last accent decides.
// Segments are separated by merge separators, which ECMA-402 never writes, so
// in practice the whole string is one segment.
func compareBackwardSecondary(left, right []uint64) int {
	ls, rs := 0, 0
	for ls < len(left) {
		le, re := segmentEnd(left, ls), segmentEnd(right, rs)
		li, ri := le-1, re-1
		for {
			var lw, rw uint32
			for ; li >= ls && secondaryOf(left[li]) == 0; li-- {
			}
			for ; ri >= rs && secondaryOf(right[ri]) == 0; ri-- {
			}
			if li >= ls {
				lw = secondaryOf(left[li])
			} else {
				lw = noCEWeight16
			}
			if ri >= rs {
				rw = secondaryOf(right[ri])
			} else {
				rw = noCEWeight16
			}
			if lw != rw {
				return sign(lw < rw)
			}
			if lw == noCEWeight16 {
				break
			}
			li--
			ri--
		}
		ls, rs = le+1, re+1
	}
	return 0
}

// segmentEnd finds the end of a segment: the next merge separator or the end
// of the text.
func segmentEnd(ces []uint64, from int) int {
	for i := from; i < len(ces); i++ {
		if p := uint32(ces[i] >> 32); p != 0 && p <= mergeSeparatorPrimary {
			return i
		}
	}
	return len(ces)
}

func (c *Collator) compareCaseLevel(left, right []uint64) int {
	upperFirst := c.caseFirst == CaseFirstUpper
	// At primary strength a case weight counts only where there is a letter,
	// or "ä" would sort after "a"; above it, only where there is an accent
	// weight, so that a tertiary difference cannot pose as a case one.
	skip := func(ce uint64) bool {
		if c.strength == strengthPrimary {
			return ce>>32 == 0 || uint32(ce) == 0
		}
		return secondaryOf(ce) == 0
	}
	for li, ri := 0, 0; ; li, ri = li+1, ri+1 {
		for skip(left[li]) {
			li++
		}
		for skip(right[ri]) {
			ri++
		}
		lc, rc := uint16(left[li])&caseMask, uint16(right[ri])&caseMask
		if lc != rc {
			if upperFirst {
				return sign(lc > rc)
			}
			return sign(lc < rc)
		}
		if secondaryOf(left[li]) == noCEWeight16 {
			return 0
		}
	}
}

// upperFirstTertiary flips the case bits so that upper case sorts first,
// leaving the end-of-text weight below everything, and gives an element with
// no secondary a case above every real one, as ICU does.
func upperFirstTertiary(t uint16, ce uint64) uint16 {
	if t <= noCEWeight16 {
		return t
	}
	if secondaryOf(ce) != 0 {
		return t ^ 0xc000
	}
	return t + 0x4000
}

func quaternaryOf(ce uint64) uint32 {
	if uint16(ce) <= noCEWeight16 {
		return uint32(ce >> 32)
	}
	return uint32(ce) | 0xffffff3f
}
