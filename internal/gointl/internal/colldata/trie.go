package colldata

import "fmt"

// A Trie maps every code point to a 32-bit value. It is ICU's code point trie
// (UCPTrie), read as ICU writes it: collation data is exported in this form,
// and reading it where it lies is less to get wrong than converting it.
//
// Most code points share a value with their neighbours, so the trie stores
// blocks of values and an index of blocks, and the blocks that repeat are
// stored once. A lookup is two or three array reads.
type Trie struct {
	Index U16s
	Data  U32s
	// HighStart is where the values stop varying: every code point from here
	// on has the value at the end of Data.
	HighStart rune
	// Fast tries index the whole of the Basic Multilingual Plane directly;
	// small ones only its first 4,096 code points.
	Fast bool
}

// The constants are ICU's, from ucptrie.h and ucptrie_impl.h.
const (
	fastShift          = 6
	fastDataMask       = 1<<fastShift - 1
	smallMax           = 0xfff
	bmpIndexLength     = 0x10000 >> fastShift
	smallIndexLength   = 0x1000 >> fastShift
	shift3             = 4
	shift2             = shift3 + 5
	shift1             = shift2 + 5
	omittedBMPIndex1   = 0x10000 >> shift1
	index2Mask         = 1<<(shift1-shift2) - 1
	index3Mask         = 1<<(shift2-shift3) - 1
	smallDataMask      = 1<<shift3 - 1
	errorValueNegative = 1
	highValueNegative  = 2
)

// Get returns a code point's value.
func (t *Trie) Get(c rune) uint32 {
	i := t.dataIndex(c)
	if i < 0 || i >= t.Data.Len() {
		return t.Data.At(t.Data.Len() - errorValueNegative)
	}
	return t.Data.At(i)
}

func (t *Trie) dataIndex(c rune) int {
	fastMax := rune(smallMax)
	if t.Fast {
		fastMax = 0xffff
	}
	switch {
	case c < 0 || c > 0x10ffff:
		return t.Data.Len() - errorValueNegative
	case c <= fastMax:
		return t.index(int(c>>fastShift)) + int(c&fastDataMask)
	case c >= t.HighStart:
		return t.Data.Len() - highValueNegative
	}
	return t.smallIndex(c)
}

func (t *Trie) smallIndex(c rune) int {
	i1 := int(c >> shift1)
	if t.Fast {
		i1 += bmpIndexLength - omittedBMPIndex1
	} else {
		i1 += smallIndexLength
	}
	i3Block := t.index(t.index(i1) + int((c>>shift2)&index2Mask))
	i3 := int((c >> shift3) & index3Mask)
	var dataBlock int
	if i3Block&0x8000 == 0 {
		// Sixteen-bit indexes.
		dataBlock = t.index(i3Block + i3)
	} else {
		// Eighteen-bit indexes, stored as groups of nine units for eight
		// indexes: one unit of high bits, then the eight low halves.
		i3Block = (i3Block & 0x7fff) + (i3 &^ 7) + (i3 >> 3)
		i3 &= 7
		dataBlock = (t.index(i3Block) << (2 + 2*i3)) & 0x30000
		dataBlock |= t.index(i3Block + 1 + i3)
	}
	return dataBlock + int(c&smallDataMask)
}

// index is one entry of the index, or, past its end, as a corrupt table
// may point, one that points past the data, which Get answers with the
// error value.
func (t *Trie) index(i int) int {
	if i < 0 || i >= t.Index.Len() {
		return 0xffff << 2
	}
	return int(t.Index.At(i))
}

// BuildTrie lays out a fast trie holding what get gives every code point,
// for a generator that has a table in another form. Repeated blocks are
// stored once and nothing else is compacted. The trie it makes answers
// exactly as get does; the generator should check that it does.
func BuildTrie(get func(rune) uint32, errorValue uint32) (Trie, error) {
	const (
		bmpBlock   = 1 << fastShift // values per BMP data block
		smallBlock = 1 << shift3    // values per supplementary data block
		index2Len  = 1 << (shift1 - shift2)
		index3Len  = 1 << (shift2 - shift3)
	)
	highValue := get(0x10ffff)
	highStart := rune(0x10000)
	for c := rune(0x10ffff); c >= 0x10000; c-- {
		if get(c) != highValue {
			highStart = (c + 1<<shift1) &^ (1<<shift1 - 1)
			break
		}
	}

	var data []uint32
	dataBlocks := map[string]int{}
	addData := func(from rune, n int) (uint16, error) {
		key := make([]byte, 0, 4*n)
		vals := make([]uint32, n)
		for i := range vals {
			vals[i] = get(from + rune(i))
			key = append(key, byte(vals[i]), byte(vals[i]>>8), byte(vals[i]>>16), byte(vals[i]>>24))
		}
		at, ok := dataBlocks[string(key)]
		if !ok {
			at = len(data)
			dataBlocks[string(key)] = at
			data = append(data, vals...)
		}
		if at > 0xffff {
			return 0, fmt.Errorf("colldata: a trie of more than %d values", 0xffff)
		}
		return uint16(at), nil
	}

	index := make([]uint16, bmpIndexLength)
	for b := range index {
		at, err := addData(rune(b*bmpBlock), bmpBlock)
		if err != nil {
			return Trie{}, err
		}
		index[b] = at
	}
	// The supplementary planes below highStart: an index-1 entry for each
	// 1<<shift1 code points, pointing at an index-2 block of index-3 blocks
	// of data block offsets.
	n1 := int(highStart-0x10000) >> shift1
	index1 := len(index)
	index = append(index, make([]uint16, n1)...)
	blocks := map[string]int{}
	addIndex := func(block []uint16) int {
		key := make([]byte, 0, 2*len(block))
		for _, v := range block {
			key = append(key, byte(v), byte(v>>8))
		}
		at, ok := blocks[string(key)]
		if !ok {
			at = len(index)
			blocks[string(key)] = at
			index = append(index, block...)
		}
		return at
	}
	for i1 := 0; i1 < n1; i1++ {
		c1 := 0x10000 + rune(i1)<<shift1
		i2 := make([]uint16, index2Len)
		for j := range i2 {
			c2 := c1 + rune(j)<<shift2
			i3 := make([]uint16, index3Len)
			for k := range i3 {
				at, err := addData(c2+rune(k)<<shift3, smallBlock)
				if err != nil {
					return Trie{}, err
				}
				i3[k] = at
			}
			i2[j] = uint16(addIndex(i3))
		}
		index[index1+i1] = uint16(addIndex(i2))
	}
	if len(index) >= 0x8000 {
		// Past this an index-2 entry would read as eighteen-bit.
		return Trie{}, fmt.Errorf("colldata: a trie index of %d entries", len(index))
	}
	data = append(data, highValue, errorValue)
	idx := make([]uint16, len(index))
	copy(idx, index)
	return Trie{Index: MakeU16s(idx), Data: MakeU32s(data), HighStart: highStart, Fast: true}, nil
}
