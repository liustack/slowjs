package intl

import "unicode/utf8"

// ICU's identical-prefix test, the IdenticalPrefix divergence.
//
// RuleBasedCollator::doCompare does not weigh the prefix two strings share:
// it compares them from where they differ, backed up to a character it may
// start from. A character it may not start before is in its unsafe-backward
// set -- a combining mark, a character a contraction continues with, a
// trail surrogate, a lead surrogate of a supplementary character that is
// any of those -- or, with numeric sorting, a digit, so that a number is
// read whole. Where that holds, what follows weighs as it does in the whole
// string, and the answer is UCA's; where it does not, ICU's differs. ICU's
// digit test reads only the collation's own table, which for a tailoring
// holds no character it does not tailor: in Arabic, which tailors none of
// its digits, the shared "1" of 15 and 100 is skipped and 5 is compared
// with 00. And a backward level is read from the end of what follows the
// prefix, not of the whole string.
//
// ICU compares UTF-16, and so does this: a JavaScript string's units, the
// lone surrogates WTF-8 carries among them.

// comparisonStart is the byte offset, the same in both, from which ICU
// compares two different strings.
func (c *Collator) comparisonStart(a, b string) int {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	// Back to the start of the character the bytes differ in.
	for i > 0 && i < len(a) && a[i]&0xc0 == 0x80 {
		i--
	}
	ra, na := decodeWTF8(a, i)
	rb, nb := decodeWTF8(b, i)
	// The UTF-16 units the strings first differ in, which are the trail
	// surrogates when they share a lead.
	diff := unitPos{off: i}
	ua, ub := firstUnit(ra), firstUnit(rb)
	if na > 0 && nb > 0 && ra >= 0x10000 && rb >= 0x10000 && ua == ub {
		diff.trail = true
		ua, ub = trailUnit(ra), trailUnit(rb)
	}
	if diff.isStart() {
		return 0
	}
	if !(na > 0 && c.unsafeBackward(ua) || nb > 0 && c.unsafeBackward(ub)) {
		return i
	}
	p := diff.prev(a)
	for !p.isStart() && c.unsafeBackward(p.unit(a)) {
		p = p.prev(a)
	}
	return p.off
}

// unsafeBackward is CollationData::isUnsafeBackward, of a UTF-16 unit.
func (c *Collator) unsafeBackward(u uint16) bool {
	r := rune(u)
	if c.root.Data.InUnsafe(r) || c.tailoring != nil && c.tailoring.InUnsafe(r) {
		return true
	}
	if r >= 0xd800 && r < 0xdc00 {
		// CollationDataReader marks a lead surrogate unsafe when any of
		// the characters it leads is.
		lo := 0x10000 + (r-0xd800)<<10
		if c.root.Data.UnsafeIntersects(lo, lo+0x3ff) ||
			c.tailoring != nil && c.tailoring.UnsafeIntersects(lo, lo+0x3ff) {
			return true
		}
	}
	return c.numeric && c.isDigit(r)
}

// isDigit is CollationData::isDigit: an ASCII digit, or a character the
// collation's own table gives a digit's element. A tailoring's table falls
// back to the root's for what it does not tailor, which this does not.
func (c *Collator) isDigit(r rune) bool {
	if r < 0x660 {
		return r >= '0' && r <= '9'
	}
	d := &c.root.Data
	if c.tailoring != nil {
		d = c.tailoring
	}
	ce32 := d.Trie.Get(r)
	return isSpecial(ce32) && tagOf(ce32) == tagDigit
}

// A unitPos is a UTF-16 unit of a string: the character's byte offset, and
// whether the unit is its trail surrogate.
type unitPos struct {
	off   int
	trail bool
}

func (p unitPos) isStart() bool { return p.off == 0 && !p.trail }

// prev is the unit before p.
func (p unitPos) prev(s string) unitPos {
	if p.trail {
		return unitPos{off: p.off}
	}
	r, n := decodeLastWTF8(s[:p.off])
	return unitPos{off: p.off - n, trail: r >= 0x10000}
}

// unit is the unit at p.
func (p unitPos) unit(s string) uint16 {
	r, _ := decodeWTF8(s, p.off)
	if p.trail {
		return trailUnit(r)
	}
	return firstUnit(r)
}

func firstUnit(r rune) uint16 {
	if r >= 0x10000 {
		return uint16(0xd800 + (r-0x10000)>>10)
	}
	return uint16(r)
}

func trailUnit(r rune) uint16 { return uint16(0xdc00 + (r-0x10000)&0x3ff) }

// decodeWTF8 is the character at byte i and its length, 0 at the end: a
// lone surrogate, which WTF-8 writes as UTF-8 would were it a character,
// as itself.
func decodeWTF8(s string, i int) (rune, int) {
	if i >= len(s) {
		return 0, 0
	}
	if i+2 < len(s) && s[i] == 0xed && s[i+1] >= 0xa0 && s[i+1] <= 0xbf {
		return 0xd000 | rune(s[i+1]&0x3f)<<6 | rune(s[i+2]&0x3f), 3
	}
	return utf8.DecodeRuneInString(s[i:])
}

// decodeLastWTF8 is decodeWTF8 of the last character.
func decodeLastWTF8(s string) (rune, int) {
	if n := len(s); n >= 3 && s[n-3] == 0xed && s[n-2] >= 0xa0 && s[n-2] <= 0xbf {
		return 0xd000 | rune(s[n-2]&0x3f)<<6 | rune(s[n-1]&0x3f), 3
	}
	return utf8.DecodeLastRuneInString(s)
}
