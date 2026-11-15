// Package normdata is the model layer for Unicode normalization: how a
// character comes apart and goes back together.
//
// Two strings can spell the same text in more than one way. An e with an acute
// accent is one character or two, and a run of accents can be written in more
// than one order. Normalizing settles both questions, which is what lets text
// from different places be compared -- and the collator sorts on the
// decomposed form, so everything it does rests on this.
//
// What is stored is the Unicode Character Database's own mappings. Hangul is
// not among them: its syllables come apart by arithmetic, which is smaller
// than the table would be.
package normdata

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 3

// A Decomposition is one character and what it comes apart into.
type Decomposition struct {
	Rune rune
	To   string
}

// A Combining is one character and its canonical combining class, which says
// how accents order among themselves. Only the characters with a class other
// than zero are stored.
type Combining struct {
	Rune  rune
	Class uint8
}

// A Composition is a pair of characters and the one they make together. It is
// Unicode's canonical mapping read backwards, keyed for looking up.
type Composition struct {
	First, Second, To rune
}

// Built is the tables as a generator builds them, which Encode writes.
type Built struct {
	// Canonical and Compatibility are sorted by rune. Canonical mappings are
	// stored fully worked out, so nothing has to be applied twice.
	Canonical     []Decomposition
	Compatibility []Decomposition
	// Classes is sorted by rune.
	Classes []Combining
	// Excluded are the characters that decompose but must not be put back
	// together, sorted.
	Excluded []rune
	// Compositions are the pairs that make one character, sorted.
	//
	// They come from the mappings as the database writes them, which are at
	// most two characters long, rather than from the worked-out ones: an
	// s-with-two-dots is written as an s-with-one-dot and the other dot, and
	// working that out into three characters loses the pair.
	Compositions []Composition
}

// Tables are everything normalization needs, read where they lie: sorted
// arrays of fixed-width records, and the decompositions as indexes keyed by
// the rune's four bytes, big-endian, so that their order is the runes'.
// Building a normalizer decodes nothing.
type Tables struct {
	canonical, compatibility runeIndex
	classes                  []byte // rune<<8 | class, four bytes each
	excluded                 []byte // runes, four bytes each
	compositions             []byte // first, second, to: twelve bytes each
	combinesBack             []byte // runes, four bytes each
}

func runeKey(r rune) string {
	return string([]byte{byte(r >> 24), byte(r >> 16), byte(r >> 8), byte(r)})
}

// A runeIndex is an index keyed by a character. Its keys are the rune's
// four bytes, big-endian, so a key is compared as the number it was
// written from rather than built back into a string and compared as one,
// and it holds the first and last character it has a record for, so that a
// character outside that range -- which the text a normalizer is given is
// mostly made of -- is answered without searching at all.
type runeIndex struct {
	x           blob.Index
	first, last rune
}

func newRuneIndex(x blob.Index) runeIndex {
	// An empty index holds no character: first after last leaves nothing
	// for the range to let through.
	t := runeIndex{x: x, first: 1, last: 0}
	if n := x.Len(); n > 0 {
		t.first, t.last = keyRune(x, 0), keyRune(x, n-1)
	}
	return t
}

// keyRune is record i's key as the character it was written for. A record
// a Source corrupted, whose key is not four bytes, reads as a character
// before every other, so that the search passes over it rather than
// matching it.
func keyRune(x blob.Index, i int) rune {
	k, _ := x.At(i)
	if len(k) != 4 {
		return -1
	}
	return rune(binary.BigEndian.Uint32(k))
}

func (t runeIndex) find(r rune) ([]byte, bool) {
	if r < t.first || r > t.last {
		return nil, false
	}
	n := t.x.Len()
	i := sort.Search(n, func(i int) bool { return keyRune(t.x, i) >= r })
	if i < n {
		if k, v := t.x.At(i); len(k) == 4 && rune(binary.BigEndian.Uint32(k)) == r {
			return v, true
		}
	}
	return nil, false
}

func u32(b []byte, i int) uint32 { return binary.LittleEndian.Uint32(b[4*i:]) }

// Decompose finds a character's canonical decomposition.
func (t *Tables) Decompose(r rune) (string, bool) {
	v, ok := t.canonical.find(r)
	return string(v), ok
}

// DecomposeCompatibility finds a character's compatibility decomposition,
// which is the canonical one where it has no other.
func (t *Tables) DecomposeCompatibility(r rune) (string, bool) {
	if v, ok := t.compatibility.find(r); ok {
		return string(v), true
	}
	return t.Decompose(r)
}

// Class returns a character's canonical combining class.
func (t *Tables) Class(r rune) uint8 {
	n := len(t.classes) / 4
	i := sort.Search(n, func(i int) bool { return rune(u32(t.classes, i)>>8) >= r })
	if i < n && rune(u32(t.classes, i)>>8) == r {
		return uint8(u32(t.classes, i))
	}
	return 0
}

// IsExcluded reports whether a character must not be composed back together.
func (t *Tables) IsExcluded(r rune) bool { return inRunes(t.excluded, r) }

// CombinesBack reports whether a character composes with one before it:
// whether it is the second of some composition.
func (t *Tables) CombinesBack(r rune) bool { return inRunes(t.combinesBack, r) }

func inRunes(b []byte, r rune) bool {
	n := len(b) / 4
	i := sort.Search(n, func(i int) bool { return rune(u32(b, i)) >= r })
	return i < n && rune(u32(b, i)) == r
}

// Compose returns what two characters make together.
func (t *Tables) Compose(first, second rune) (rune, bool) {
	n := len(t.compositions) / 12
	at := func(i int) (rune, rune) {
		return rune(u32(t.compositions, 3*i)), rune(u32(t.compositions, 3*i+1))
	}
	i := sort.Search(n, func(i int) bool {
		f, s := at(i)
		return f > first || f == first && s >= second
	})
	if i < n {
		if f, s := at(i); f == first && s == second {
			return rune(u32(t.compositions, 3*i+2)), true
		}
	}
	return 0, false
}

// Encode writes the tables.
func Encode(t *Built) ([]byte, error) {
	b := blob.NewWriter(Version)
	for _, set := range [][]Decomposition{t.Canonical, t.Compatibility} {
		records := map[string][]byte{}
		for _, d := range set {
			records[runeKey(d.Rune)] = []byte(d.To)
		}
		index, err := blob.BuildIndex(Version, records)
		if err != nil {
			return nil, err
		}
		b.String(string(index))
	}
	var classes, excluded, compositions []byte
	for _, c := range t.Classes {
		classes = binary.LittleEndian.AppendUint32(classes, uint32(c.Rune)<<8|uint32(c.Class))
	}
	for _, r := range t.Excluded {
		excluded = binary.LittleEndian.AppendUint32(excluded, uint32(r))
	}
	seconds := map[rune]bool{}
	for _, c := range t.Compositions {
		compositions = binary.LittleEndian.AppendUint32(compositions, uint32(c.First))
		compositions = binary.LittleEndian.AppendUint32(compositions, uint32(c.Second))
		compositions = binary.LittleEndian.AppendUint32(compositions, uint32(c.To))
		seconds[c.Second] = true
	}
	back := make([]rune, 0, len(seconds))
	for r := range seconds {
		back = append(back, r)
	}
	sort.Slice(back, func(i, j int) bool { return back[i] < back[j] })
	var combinesBack []byte
	for _, r := range back {
		combinesBack = binary.LittleEndian.AppendUint32(combinesBack, uint32(r))
	}
	for _, part := range [][]byte{classes, excluded, compositions, combinesBack} {
		b.String(string(part))
	}
	return b.Bytes(), nil
}

// Decode reads what Encode wrote, where it lies.
func Decode(data []byte) (*Tables, error) {
	r, err := blob.NewReader(data, Version)
	if err != nil {
		return nil, err
	}
	var t Tables
	for _, index := range []*runeIndex{&t.canonical, &t.compatibility} {
		x, err := blob.ReadIndex(r.Bytes(), Version)
		if err != nil {
			return nil, err
		}
		*index = newRuneIndex(x)
	}
	t.classes, t.excluded, t.compositions, t.combinesBack = r.Bytes(), r.Bytes(), r.Bytes(), r.Bytes()
	if err := r.Err(); err != nil {
		return nil, err
	}
	if len(t.classes)%4 != 0 || len(t.excluded)%4 != 0 || len(t.compositions)%12 != 0 || len(t.combinesBack)%4 != 0 {
		return nil, fmt.Errorf("normdata: tables of %d, %d, %d and %d bytes", len(t.classes),
			len(t.excluded), len(t.compositions), len(t.combinesBack))
	}
	return &t, nil
}
