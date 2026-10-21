package intl

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/liustack/quickjs/internal/gointl/internal/blob"
	"github.com/liustack/quickjs/internal/gointl/internal/layout"
)

// unicodeProps are the Unicode properties formatting reads, in the Unicode
// ICU carries (data/properties.bin, which propgen writes), rather than Go's
// unicode tables, whose version is whatever the toolchain building the
// program has: Go 1.24's has no Garay digit, which Unicode 16 added.
type unicodeProps struct {
	sets map[string]byteRanges
}

// propertySets are the sets data/properties.bin holds.
var propertySets = []string{"gc S", "gc Z", "gc L", "gc N", "gc Nd", "gc Zs",
	"Bidi_Control", "Variation_Selector", "sc Hebrew"}

func loadUnicodeProps(src Source) (*unicodeProps, error) {
	b, err := src.Open(MarkerProperties, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the Unicode properties: %w", err)
	}
	index, err := blob.ReadIndex(b, layout.Properties)
	if err != nil {
		return nil, fmt.Errorf("intl: the Unicode properties: %w", err)
	}
	p := &unicodeProps{sets: map[string]byteRanges{}}
	for _, name := range propertySets {
		v, ok := index.Find(name)
		if !ok || len(v) == 0 || len(v)%8 != 0 {
			return nil, fmt.Errorf("intl: the Unicode property %s is %d bytes", name, len(v))
		}
		p.sets[name] = byteRanges(v)
	}
	return p, nil
}

// in reports whether a character is in a set.
func (p *unicodeProps) in(set string, r rune) bool { return p.sets[set].contains(r) }

// ignorable is ICU's DEFAULT_IGNORABLES: the space separators, the tab,
// the bidirectional controls and the variation selectors.
func (p *unicodeProps) ignorable(r rune) bool {
	return r == '\t' || p.in("gc Zs", r) || p.in("Bidi_Control", r) || p.in("Variation_Selector", r)
}

// byteRanges is a set of code points as sorted ranges, read where they
// lie: two little-endian uint32s each, the first and the last.
type byteRanges []byte

func (s byteRanges) contains(c rune) bool {
	at := func(i, field int) rune { return rune(binary.LittleEndian.Uint32(s[8*i+4*field:])) }
	n := len(s) / 8
	i := sort.Search(n, func(i int) bool { return at(i, 1) >= c })
	return i < n && at(i, 0) <= c
}
