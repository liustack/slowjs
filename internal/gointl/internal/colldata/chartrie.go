package colldata

// A CharTrie walk matches a string one character at a time against ICU's
// UCharsTrie, the form collation keeps its contractions and prefixes in: the
// characters that may follow (or precede) one character and change what it
// sorts as, each ending in a value.
//
// The trie is serialized sixteen-bit units, read where they lie. The layout is
// ICU's, from ucharstrie.h.

// Match is how a step of a walk went.
type Match int

const (
	// NoMatch means the characters so far start nothing in the trie. Every
	// later step also fails.
	NoMatch Match = iota
	// NoValue means the characters so far start something but are not
	// themselves an entry.
	NoValue
	// FinalValue means the characters so far are an entry and nothing longer
	// is.
	FinalValue
	// IntermediateValue means the characters so far are an entry and
	// something longer may be too.
	IntermediateValue
)

// HasValue reports whether the characters so far are an entry.
func (m Match) HasValue() bool { return m >= FinalValue }

// HasNext reports whether a longer string could still match.
func (m Match) HasNext() bool { return m == NoValue || m == IntermediateValue }

const (
	maxBranchLinearSubNodeLength = 5
	minLinearMatch               = 0x30
	maxLinearMatchLength         = 0x10
	minValueLead                 = minLinearMatch + maxLinearMatchLength
	nodeTypeMask                 = minValueLead - 1
	valueIsFinal                 = 0x8000
	maxOneUnitValue              = 0x3fff
	minTwoUnitValueLead          = maxOneUnitValue + 1
	maxOneUnitNodeValue          = 0xff
	minTwoUnitNodeValueLead      = minValueLead + (maxOneUnitNodeValue+1)<<6
	threeUnitNodeValueLead       = 0x7fc0
	threeUnitValueLead           = 0x7fff
	maxOneUnitDelta              = 0xfbff
	minTwoUnitDeltaLead          = maxOneUnitDelta + 1
	threeUnitDeltaLead           = 0xffff
)

// A CharTrie is a position in a walk. It is a value: copying it saves the
// position, which is how a match backs up to the longest entry it found.
type CharTrie struct {
	units U16s
	// pos is the next unit to read, or -1 once nothing can match.
	pos int
	// remaining is what is left of a linear-match node, less one, or -1
	// outside one.
	remaining int
	value     int32
}

// NewCharTrie starts a walk at the beginning of a serialized trie.
func NewCharTrie(units U16s) CharTrie {
	return CharTrie{units: units, remaining: -1}
}

// Value is the value of the entry the last step ended on, when it ended on
// one.
func (t *CharTrie) Value() uint32 { return uint32(t.value) }

// Next takes one step with a character.
func (t *CharTrie) Next(c rune) Match {
	if c <= 0xffff {
		return t.next16(uint16(c))
	}
	lead := uint16((c >> 10) + 0xd7c0)
	trail := uint16(c&0x3ff | 0xdc00)
	if m := t.next16(lead); !m.HasNext() {
		t.pos = -1
		return NoMatch
	}
	return t.next16(trail)
}

func (t *CharTrie) unit(i int) uint16 {
	if i < 0 || i >= t.units.Len() {
		return 0
	}
	return t.units.At(i)
}

func (t *CharTrie) next16(c uint16) Match {
	pos := t.pos
	if pos < 0 {
		return NoMatch
	}
	if length := t.remaining; length >= 0 {
		// The rest of a linear-match node.
		if c != t.unit(pos) {
			t.pos = -1
			return NoMatch
		}
		pos++
		t.pos = pos
		if length == 0 {
			t.remaining = -1
			if node := t.unit(pos); node >= minValueLead {
				return t.valueResult(pos)
			}
		} else {
			t.remaining = length - 1
		}
		return NoValue
	}
	return t.nextImpl(pos, c)
}

func (t *CharTrie) nextImpl(pos int, c uint16) Match {
	node := t.unit(pos)
	pos++
	for {
		switch {
		case node < minLinearMatch:
			return t.branchNext(pos, int(node), c)
		case node < minValueLead:
			// Match the first of length+1 units.
			length := int(node - minLinearMatch)
			if c != t.unit(pos) {
				t.pos = -1
				return NoMatch
			}
			pos++
			t.pos = pos
			if length == 0 {
				t.remaining = -1
				if node := t.unit(pos); node >= minValueLead {
					return t.valueResult(pos)
				}
				return NoValue
			}
			t.remaining = length - 1
			return NoValue
		case node&valueIsFinal != 0:
			// Nothing follows a final value.
			t.pos = -1
			return NoMatch
		default:
			// Skip an intermediate value and read the node it belongs to.
			pos = skipNodeValue(pos, node)
			node &= nodeTypeMask
		}
	}
}

func (t *CharTrie) branchNext(pos, length int, c uint16) Match {
	if length == 0 {
		length = int(t.unit(pos))
		pos++
	}
	length++
	// A branch is a binary search written out, down to a few entries that
	// are searched in a line.
	for length > maxBranchLinearSubNodeLength {
		if c < t.unit(pos) {
			length >>= 1
			pos = t.jumpByDelta(pos + 1)
		} else {
			length -= length >> 1
			pos = t.skipDelta(pos + 1)
		}
	}
	for {
		if c == t.unit(pos) {
			pos++
			node := t.unit(pos)
			if node&valueIsFinal != 0 {
				t.pos = pos
				return t.valueResult(pos)
			}
			// A non-final value is the distance to the node that follows.
			pos++
			switch {
			case node < minTwoUnitValueLead:
				pos += int(node)
			case node < threeUnitValueLead:
				pos += int(node-minTwoUnitValueLead)<<16 | int(t.unit(pos))
				pos++
			default:
				pos += int(t.unit(pos))<<16 | int(t.unit(pos+1))
				pos += 2
			}
			node = t.unit(pos)
			t.pos = pos
			if node >= minValueLead {
				return t.valueResult(pos)
			}
			return NoValue
		}
		length--
		pos = skipValue(pos+2, t.unit(pos+1)&0x7fff)
		if length <= 1 {
			break
		}
	}
	if c == t.unit(pos) {
		pos++
		t.pos = pos
		if node := t.unit(pos); node >= minValueLead {
			return t.valueResult(pos)
		}
		return NoValue
	}
	t.pos = -1
	return NoMatch
}

func skipValue(pos int, lead uint16) int {
	switch {
	case lead < minTwoUnitValueLead:
		return pos
	case lead < threeUnitValueLead:
		return pos + 1
	}
	return pos + 2
}

func skipNodeValue(pos int, lead uint16) int {
	switch {
	case lead < minTwoUnitNodeValueLead:
		return pos
	case lead < threeUnitNodeValueLead:
		return pos + 1
	}
	return pos + 2
}

func (t *CharTrie) jumpByDelta(pos int) int {
	delta := t.unit(pos)
	switch {
	case delta < minTwoUnitDeltaLead:
		return pos + 1 + int(delta)
	case delta == threeUnitDeltaLead:
		return pos + 3 + (int(t.unit(pos+1))<<16 | int(t.unit(pos+2)))
	}
	return pos + 2 + (int(delta-minTwoUnitDeltaLead)<<16 | int(t.unit(pos+1)))
}

func (t *CharTrie) skipDelta(pos int) int {
	delta := t.unit(pos)
	switch {
	case delta < minTwoUnitDeltaLead:
		return pos + 1
	case delta == threeUnitDeltaLead:
		return pos + 3
	}
	return pos + 2
}

func (t *CharTrie) valueResult(pos int) Match {
	lead := t.unit(pos)
	if lead&valueIsFinal != 0 {
		t.value = t.readValue(pos+1, lead&0x7fff)
		return FinalValue
	}
	t.value = t.readNodeValue(pos+1, lead)
	return IntermediateValue
}

func (t *CharTrie) readValue(pos int, lead uint16) int32 {
	switch {
	case lead < minTwoUnitValueLead:
		return int32(lead)
	case lead < threeUnitValueLead:
		return int32(lead-minTwoUnitValueLead)<<16 | int32(t.unit(pos))
	}
	return int32(t.unit(pos))<<16 | int32(t.unit(pos+1))
}

func (t *CharTrie) readNodeValue(pos int, lead uint16) int32 {
	switch {
	case lead < minTwoUnitNodeValueLead:
		return int32(lead>>6) - 1
	case lead < threeUnitNodeValueLead:
		return int32((lead&0x7fc0)-minTwoUnitNodeValueLead)<<10 | int32(t.unit(pos))
	}
	return int32(t.unit(pos))<<16 | int32(t.unit(pos+1))
}
