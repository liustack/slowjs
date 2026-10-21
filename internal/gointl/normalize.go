package intl

import (
	"fmt"
	"sort"
	"strings"

	"github.com/liustack/quickjs/internal/gointl/internal/normdata"
)

// Unicode normalization.
//
// Two strings can spell the same text in more than one way: an e with an acute
// accent is one character or two, and a run of accents can be written in more
// than one order. Normalizing settles both, which is what lets text from
// different places be compared at all.
//
// There are four forms. D takes characters apart and C puts them back
// together; the K forms additionally replace a character by what it stands for
// -- the ligature ﬁ by "fi", the superscript ² by "2" -- which loses a
// distinction rather than merely respelling it.
//
// The collator sorts on the decomposed form, so everything it does rests on
// this being right. The tables come from the Unicode Character Database at the
// version the anchor names.

// A NormalizationForm is one of Unicode's four.
type NormalizationForm int

const (
	// NFC composes: the shortest spelling.
	NFC NormalizationForm = iota
	// NFD decomposes: characters taken apart, accents in canonical order.
	NFD
	// NFKC and NFKD do the same after replacing a character by what it stands
	// for.
	NFKC
	NFKD
)

// Hangul syllables come apart by arithmetic rather than by table, which is
// smaller than the table would be. The names are Unicode's own.
const (
	hangulBase   = 0xAC00
	hangulLBase  = 0x1100
	hangulVBase  = 0x1161
	hangulTBase  = 0x11A7
	hangulLCount = 19
	hangulVCount = 21
	hangulTCount = 28
	hangulNCount = hangulVCount * hangulTCount
	hangulSCount = hangulLCount * hangulNCount
)

// A Normalizer puts text into one of Unicode's forms. It never changes after
// it is built and is safe for any number of goroutines to share.
type Normalizer struct {
	tables *normdata.Tables
}

// NewNormalizer builds one from the data built into the package.
func NewNormalizer() (*Normalizer, error) { return NewNormalizerFrom(Embedded) }

// NewNormalizerFrom builds one from a source of the caller's own.
func NewNormalizerFrom(src Source) (*Normalizer, error) {
	b, err := src.Open(MarkerNormalization, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the normalization tables: %w", err)
	}
	tables, err := normdata.Decode(b)
	if err != nil {
		return nil, fmt.Errorf("intl: the normalization tables: %w", err)
	}
	return &Normalizer{tables: tables}, nil
}

// Normalize puts a string into one of Unicode's forms, using the data built
// into the package.
func Normalize(s string, form NormalizationForm) (string, error) {
	// Building one reads the tables where they lie, which costs nothing
	// worth keeping it for.
	n, err := NewNormalizer()
	if err != nil {
		return "", err
	}
	return n.Normalize(s, form), nil
}

// Normalize puts a string into one of Unicode's forms.
func (n *Normalizer) Normalize(s string, form NormalizationForm) string {
	compatibility := form == NFKC || form == NFKD
	out := n.decompose(s, compatibility)
	if form == NFC || form == NFKC {
		out = n.compose(out)
	}
	return out
}

// CombiningClass is a character's canonical combining class, which decides
// the order accents are written in and which of them combine: 0 for a
// starter, 230 for most accents written above. Case mapping reads it too,
// for the accents Lithuanian and Turkish keep a dot through.
func (n *Normalizer) CombiningClass(r rune) int { return int(n.tables.Class(r)) }

// decompose takes every character apart and puts the accents in order.
func (n *Normalizer) decompose(s string, compatibility bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= hangulBase && r < hangulBase+hangulSCount:
			b.WriteString(decomposeHangul(r))
			continue
		}
		var to string
		var ok bool
		if compatibility {
			to, ok = n.tables.DecomposeCompatibility(r)
		} else {
			to, ok = n.tables.Decompose(r)
		}
		if ok {
			b.WriteString(to)
		} else {
			b.WriteRune(r)
		}
	}
	return n.order(b.String())
}

// order puts a run of accents into the order Unicode fixes, which is by
// combining class and otherwise unchanged. The sort has to be stable: two
// accents of the same class are written in the order they were given.
func (n *Normalizer) order(s string) string {
	runes := []rune(s)
	for i := 0; i < len(runes); {
		if n.tables.Class(runes[i]) == 0 {
			i++
			continue
		}
		j := i
		for j < len(runes) && n.tables.Class(runes[j]) != 0 {
			j++
		}
		run := runes[i:j]
		sort.SliceStable(run, func(a, b int) bool {
			return n.tables.Class(run[a]) < n.tables.Class(run[b])
		})
		i = j
	}
	return string(runes)
}

// compose puts the decomposed text back together, by Unicode's algorithm: each
// character that follows a starter is joined to it where the two make one,
// unless something with an equal or greater combining class stands between.
func (n *Normalizer) compose(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	out := make([]rune, 0, len(runes))
	starter := -1
	lastClass := -1

	for _, r := range runes {
		class := int(n.tables.Class(r))
		// A character is blocked from the starter when something between them
		// has an equal or greater combining class. Immediately after a starter
		// nothing stands between, which is what the negative class means.
		if starter >= 0 && lastClass < class {
			if joined, ok := n.join(out[starter], r); ok {
				out[starter] = joined
				continue
			}
		}
		if class == 0 {
			starter = len(out)
			lastClass = -1
		} else {
			lastClass = class
		}
		out = append(out, r)
	}
	return string(out)
}

// join returns what two characters make together, if they make anything.
func (n *Normalizer) join(a, b rune) (rune, bool) {
	// Hangul joins by arithmetic rather than by table.
	if r, ok := composeHangul(a, b); ok {
		return r, true
	}
	return n.tables.Compose(a, b)
}

func decomposeHangul(r rune) string {
	index := r - hangulBase
	lead := hangulLBase + index/hangulNCount
	vowel := hangulVBase + (index%hangulNCount)/hangulTCount
	trail := hangulTBase + index%hangulTCount
	if index%hangulTCount == 0 {
		return string([]rune{lead, vowel})
	}
	return string([]rune{lead, vowel, trail})
}

func composeHangul(a, b rune) (rune, bool) {
	if a >= hangulLBase && a < hangulLBase+hangulLCount &&
		b >= hangulVBase && b < hangulVBase+hangulVCount {
		return hangulBase + ((a-hangulLBase)*hangulVCount+(b-hangulVBase))*hangulTCount, true
	}
	if a >= hangulBase && a < hangulBase+hangulSCount && (a-hangulBase)%hangulTCount == 0 &&
		b > hangulTBase && b < hangulTBase+hangulTCount {
		return a + (b - hangulTBase), true
	}
	return 0, false
}
