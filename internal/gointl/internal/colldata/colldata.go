// Package colldata is the model layer for collation: the tables that say what
// every character sorts as.
//
// A character sorts as one or more collation elements, each a weight at every
// level -- the letter, the accent, the case. The tables map characters to
// those elements, with the exceptions that make a language what it is: in
// Czech "ch" is one letter, so it is a contraction; in Japanese a length mark
// takes its weight from the kana before it, so it has a prefix.
//
// The root table covers every character. A tailoring holds only what a
// language changes, and everything else falls through to the root.
//
// These are ICU's own tables, as ICU 78.3 exports them for ICU4X. ICU builds
// them from CLDR's collation rules, and building them is a rule compiler of
// many thousands of lines; reading them is what every ICU runtime does. The
// element encodings are ICU's and are documented where they are decoded.
package colldata

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 3

// Data is the table of one collation: the root, or one language's changes to
// it.
type Data struct {
	// Trie gives each character its 32-bit element, which is either the
	// element itself or says where to find it.
	Trie Trie
	// CE32s and CEs are what expansions and some special elements point
	// into.
	CE32s U32s
	CEs   U64s
	// Contexts holds the contraction and prefix tries, each after a default
	// element.
	Contexts U16s
	// Unsafe is ICU's unsafe-backward set, as an inversion list: the start
	// of each range, then the character after it. A string ICU compares
	// from after the prefix it shares with another is compared from no
	// later than a character in it, which a contraction or a combining
	// mark might join to what precedes it. The root's is the set ICU
	// carries in collunsafe.h with the root table's own ranges; a
	// tailoring's holds only the ranges it adds to the root's.
	Unsafe U32s
}

// InUnsafe reports whether a character is in the data's unsafe-backward
// set.
func (d *Data) InUnsafe(c rune) bool { return d.UnsafeIntersects(c, c) }

// UnsafeIntersects reports whether any character from lo to hi is in the
// data's unsafe-backward set.
func (d *Data) UnsafeIntersects(lo, hi rune) bool {
	// The first boundary above lo: inside a range when it is a range's end,
	// or when it is a start no higher than hi.
	n := d.Unsafe.Len()
	i := sort.Search(n, func(i int) bool { return rune(d.Unsafe.At(i)) > lo })
	if i == n {
		return false
	}
	return i%2 == 1 || rune(d.Unsafe.At(i)) <= hi
}

// The arrays are little-endian bytes, read where they lie: decoding a table
// is then a matter of slicing it, which matters for the root, which is half
// a megabyte and is read by every collator built.

// U16s is an array of 16-bit values.
type U16s []byte

// U32s is an array of 32-bit values.
type U32s []byte

// U64s is an array of 64-bit values.
type U64s []byte

// MakeU16s lays out values as a U16s.
func MakeU16s(v []uint16) U16s { return U16s(u16s(v)) }

// MakeU32s lays out values as a U32s.
func MakeU32s(v []uint32) U32s { return U32s(u32s(v)) }

// MakeU64s lays out values as a U64s.
func MakeU64s(v []uint64) U64s { return U64s(u64s(v)) }

// Len is the number of values.
func (a U16s) Len() int { return len(a) / 2 }

// At is one value.
func (a U16s) At(i int) uint16 { return binary.LittleEndian.Uint16(a[2*i:]) }

// From is the values from one on.
func (a U16s) From(i int) U16s { return a[2*i:] }

// Len is the number of values.
func (a U32s) Len() int { return len(a) / 4 }

// At is one value.
func (a U32s) At(i int) uint32 { return binary.LittleEndian.Uint32(a[4*i:]) }

// Has reports whether the values from i hold n more, so that a table a
// Source corrupted cannot send a read past them.
func (a U16s) Has(i, n int) bool { return i >= 0 && n >= 0 && i <= a.Len()-n }

// Has reports whether the values from i hold n more.
func (a U32s) Has(i, n int) bool { return i >= 0 && n >= 0 && i <= a.Len()-n }

// Has reports whether the values from i hold n more.
func (a U64s) Has(i, n int) bool { return i >= 0 && n >= 0 && i <= a.Len()-n }

// Len is the number of values.
func (a U64s) Len() int { return len(a) / 8 }

// At is one value.
func (a U64s) At(i int) uint64 { return binary.LittleEndian.Uint64(a[8*i:]) }

// Reordering moves whole scripts: Bulgarian sorts Cyrillic before Latin.
type Reordering struct {
	// MinHighNoReorder is where the reordered ranges end.
	MinHighNoReorder uint32
	// Table gives each primary lead byte its new value, or zero where the
	// byte is split between scripts and Ranges decides.
	Table [256]byte
	// Ranges are (limit, offset) pairs for the split lead bytes: the upper
	// sixteen bits of an exclusive limit, and a signed offset in the lower.
	Ranges []uint32
}

// Metadata bits, as ICU4X exports them.
const (
	MaxVariableMask        = 0b11
	TailoredBit            = 1 << 3
	TailoredDiacriticsBit  = 1 << 4
	ReorderingBit          = 1 << 5
	LithuanianDotAboveBit  = 1 << 6
	BackwardSecondLevelBit = 1 << 7
	AlternateShiftedBit    = 1 << 8
	CaseFirstBit           = 1 << 9
	UpperFirstBit          = 1 << 10
)

// A Collation is one collation type of one locale -- "standard", "search",
// "phonebook" -- with the settings it carries.
type Collation struct {
	// Name is ICU's name for the type: "phonebook" where BCP 47 says
	// "phonebk".
	Name string
	// Meta holds the default settings, in the bits above.
	Meta uint32
	// Data is nil when the type changes no character, only settings.
	Data *Data
	// Reordering is nil when the type moves no script.
	Reordering *Reordering
	// Diacritics, when the type tailors them, replaces the root's.
	Diacritics []uint16
}

// A Locale is the collations one locale defines.
type Locale struct {
	// Default is the type used when none is asked for, when the locale says;
	// otherwise it is inherited, and at the root it is "standard".
	Default    string
	Collations []Collation
}

// Find returns a collation type, if the locale defines it.
func (l *Locale) Find(name string) (*Collation, bool) {
	for i := range l.Collations {
		if l.Collations[i].Name == name {
			return &l.Collations[i], true
		}
	}
	return nil, false
}

// Root is the table every collation falls back to, with the facts about its
// weights that the algorithm needs and that ICU keeps beside it rather than in
// it.
type Root struct {
	Data Data
	// LastPrimaries holds, for each group that can be made variable --
	// spaces, punctuation, symbols, currency -- the high sixteen bits of the
	// last primary weight in it.
	LastPrimaries [4]uint16
	// NumericPrimary is the lead byte of the weights numeric sorting gives a
	// run of digits.
	NumericPrimary byte
	// Diacritics are the secondary weights of the combining diacritics from
	// U+0300, which the export keeps out of the trie: a combining mark
	// there sorts as this weight alone.
	Diacritics []uint16
	// Jamo are the elements of the conjoining Hangul jamo, U+1100 to
	// U+11FF, which the export likewise keeps apart. They are elements of
	// the root table.
	Jamo []uint32
}

// JamoBase is the first character Jamo covers.
const JamoBase = 0x1100

// DiacriticsBase is the first character Diacritics covers.
const DiacriticsBase = 0x300

// Tree is how collation locales inherit from one another, which is ICU's
// collation tree rather than the ordinary one: Norwegian Bokmål takes its
// collation from Norwegian, and Cantonese from traditional Chinese.
type Tree struct {
	// Aliases replace a locale outright before anything is looked up.
	Aliases [][2]string
	// Parents override truncation for the locales named.
	Parents [][2]string
	// Installed are the locales ICU has a collation entry for, which is what
	// an ECMA-402 host negotiates against.
	Installed []string
}

// EncodeLocale writes one locale's collations.
func EncodeLocale(l *Locale) []byte {
	w := blob.NewWriter(Version)
	w.String(l.Default)
	w.Uint(len(l.Collations))
	for _, c := range l.Collations {
		w.String(c.Name)
		w.Uint64(int64(c.Meta))
		if c.Data == nil {
			w.Uint(0)
		} else {
			w.Uint(1)
			encodeData(w, c.Data)
		}
		if c.Reordering == nil {
			w.Uint(0)
		} else {
			w.Uint(1)
			w.Uint64(int64(c.Reordering.MinHighNoReorder))
			w.String(string(c.Reordering.Table[:]))
			w.String(u32s(c.Reordering.Ranges))
		}
		w.String(u16s(c.Diacritics))
	}
	return w.Bytes()
}

// DecodeLocale reads what EncodeLocale wrote.
func DecodeLocale(b []byte) (*Locale, error) {
	r, err := blob.NewReader(b, Version)
	if err != nil {
		return nil, err
	}
	var l Locale
	l.Default = r.String()
	n := r.Uint()
	if n > r.Left() {
		return nil, fmt.Errorf("colldata: %d collations with %d bytes left", n, r.Left())
	}
	for i := 0; i < n; i++ {
		var c Collation
		c.Name = r.String()
		c.Meta = uint32(r.Uint64())
		if r.Uint() == 1 {
			d, err := decodeData(r)
			if err != nil {
				return nil, err
			}
			c.Data = d
		}
		if r.Uint() == 1 {
			var o Reordering
			o.MinHighNoReorder = uint32(r.Uint64())
			table := r.String()
			if len(table) != len(o.Table) && r.Err() == nil {
				return nil, fmt.Errorf("colldata: a reordering table of %d bytes", len(table))
			}
			copy(o.Table[:], table)
			o.Ranges = toU32s(r.String())
			c.Reordering = &o
		}
		if dia := toU16s(r.String()); len(dia) > 0 {
			c.Diacritics = dia
		}
		l.Collations = append(l.Collations, c)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}

// EncodeRoot writes the root table.
func EncodeRoot(root *Root) []byte {
	w := blob.NewWriter(Version)
	encodeData(w, &root.Data)
	for _, p := range root.LastPrimaries {
		w.Uint(int(p))
	}
	w.Uint(int(root.NumericPrimary))
	w.String(u16s(root.Diacritics))
	w.String(u32s(root.Jamo))
	return w.Bytes()
}

// DecodeRoot reads what EncodeRoot wrote.
func DecodeRoot(b []byte) (*Root, error) {
	r, err := blob.NewReader(b, Version)
	if err != nil {
		return nil, err
	}
	d, err := decodeData(r)
	if err != nil {
		return nil, err
	}
	root := &Root{Data: *d}
	for i := range root.LastPrimaries {
		root.LastPrimaries[i] = uint16(r.Uint())
	}
	root.NumericPrimary = byte(r.Uint())
	root.Diacritics = toU16s(r.String())
	root.Jamo = toU32s(r.String())
	if err := r.Err(); err != nil {
		return nil, err
	}
	return root, nil
}

// EncodeTree writes the collation tree.
func EncodeTree(t *Tree) []byte {
	w := blob.NewWriter(Version)
	for _, pairs := range [][][2]string{t.Aliases, t.Parents} {
		w.Uint(len(pairs))
		for _, p := range pairs {
			w.String(p[0])
			w.String(p[1])
		}
	}
	w.Uint(len(t.Installed))
	for _, s := range t.Installed {
		w.String(s)
	}
	return w.Bytes()
}

// DecodeTree reads what EncodeTree wrote.
func DecodeTree(b []byte) (*Tree, error) {
	r, err := blob.NewReader(b, Version)
	if err != nil {
		return nil, err
	}
	var t Tree
	for _, pairs := range []*[][2]string{&t.Aliases, &t.Parents} {
		n := r.Uint()
		if n > r.Left() {
			return nil, fmt.Errorf("colldata: %d pairs with %d bytes left", n, r.Left())
		}
		for i := 0; i < n; i++ {
			from := r.String()
			to := r.String()
			*pairs = append(*pairs, [2]string{from, to})
		}
	}
	n := r.Uint()
	if n > r.Left() {
		return nil, fmt.Errorf("colldata: %d locales with %d bytes left", n, r.Left())
	}
	for i := 0; i < n; i++ {
		t.Installed = append(t.Installed, r.String())
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &t, nil
}

// The arrays are written as little-endian bytes. A varint would not hold a
// 64-bit element, whose top bit is often set, and the bytes are what the
// arrays are anyway.

func encodeData(w *blob.Writer, d *Data) {
	w.Uint(int(d.Trie.HighStart))
	if d.Trie.Fast {
		w.Uint(1)
	} else {
		w.Uint(0)
	}
	w.String(string(d.Trie.Index))
	w.String(string(d.Trie.Data))
	w.String(string(d.CE32s))
	w.String(string(d.CEs))
	w.String(string(d.Contexts))
	w.String(string(d.Unsafe))
}

func decodeData(r *blob.Reader) (*Data, error) {
	var d Data
	d.Trie.HighStart = rune(r.Uint())
	d.Trie.Fast = r.Uint() == 1
	d.Trie.Index = U16s(r.Bytes())
	d.Trie.Data = U32s(r.Bytes())
	d.CE32s = U32s(r.Bytes())
	d.CEs = U64s(r.Bytes())
	d.Contexts = U16s(r.Bytes())
	d.Unsafe = U32s(r.Bytes())
	if d.Unsafe.Len()%2 != 0 {
		return nil, fmt.Errorf("colldata: an unsafe-backward set of %d boundaries", d.Unsafe.Len())
	}
	if d.Trie.Data.Len() < highValueNegative || d.Trie.Index.Len() == 0 {
		return nil, fmt.Errorf("colldata: a trie with %d values", d.Trie.Data.Len())
	}
	return &d, nil
}

func u16s(v []uint16) string {
	b := make([]byte, 0, 2*len(v))
	for _, x := range v {
		b = binary.LittleEndian.AppendUint16(b, x)
	}
	return string(b)
}

func u32s(v []uint32) string {
	b := make([]byte, 0, 4*len(v))
	for _, x := range v {
		b = binary.LittleEndian.AppendUint32(b, x)
	}
	return string(b)
}

func u64s(v []uint64) string {
	b := make([]byte, 0, 8*len(v))
	for _, x := range v {
		b = binary.LittleEndian.AppendUint64(b, x)
	}
	return string(b)
}

func toU16s(s string) []uint16 {
	out := make([]uint16, len(s)/2)
	for i := range out {
		out[i] = binary.LittleEndian.Uint16([]byte(s[2*i : 2*i+2]))
	}
	return out
}

func toU32s(s string) []uint32 {
	out := make([]uint32, len(s)/4)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32([]byte(s[4*i : 4*i+4]))
	}
	return out
}

func toU64s(s string) []uint64 {
	out := make([]uint64, len(s)/8)
	for i := range out {
		out[i] = binary.LittleEndian.Uint64([]byte(s[8*i : 8*i+8]))
	}
	return out
}
