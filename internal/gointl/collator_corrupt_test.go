//go:build gointl_all

package intl

import (
	"fmt"
	"math/rand"
	"testing"
)

// corruptSource answers as base does, but for one data set, which it
// answers with the bytes given.
type corruptSource struct {
	base   Source
	marker Marker
	locale DataLocale
	bytes  []byte
}

func (s corruptSource) Open(m Marker, d DataLocale) ([]byte, error) {
	if m == s.marker && d == s.locale {
		return s.bytes, nil
	}
	return s.base.Open(m, d)
}

// A collation table a Source corrupted is refused when the collator is
// built, or reads as some order, but never panics: the trie's index and
// the element arrays had been read without bounds checks, so that a value
// pointing past them panicked in Compare (ISSUES.md DA-2).
func TestCorruptCollationTables(t *testing.T) {
	words := []string{"", "a", "ab", "ch", "cz", "ä", "ä", "å", "aa", "각",
		"가", "カー", "เก", "12", "١٥", "\U0001D7CF", "ß", "ss",
		"İ", "Ǆ", "x̣́", "�", "\U00020000", "-", "a b", "്‍"}
	yes := true
	options := []CollatorOptions{{}, {Numeric: &yes, IgnorePunctuation: &yes}, {Usage: UsageSearch}}
	type target struct {
		marker Marker
		tag    string
	}
	targets := []target{{MarkerCollationRoot, ""}, {MarkerCollation, "und"}, {MarkerCollation, "de"},
		{MarkerCollation, "sv"}, {MarkerCollation, "ja"}, {MarkerCollation, "ko"}, {MarkerCollation, "th"},
		{MarkerCollation, "zh"}, {MarkerCollation, "ar"}, {MarkerCollation, "lt"}}
	built, refused := 0, 0
	for _, tg := range targets {
		var d DataLocale
		loc := mustParse(t, "und")
		if tg.tag != "" {
			loc = mustParse(t, tg.tag)
			d = loc.Data()
		}
		orig, err := Embedded.Open(tg.marker, d)
		if err != nil {
			t.Fatalf("%s %s: %v", tg.marker, tg.tag, err)
		}
		rng := rand.New(rand.NewSource(int64(len(orig))))
		for iter := 0; iter < 300; iter++ {
			b := append([]byte(nil), orig...)
			for flips := 1 + rng.Intn(8); flips > 0; flips-- {
				// The arrays are most of a table: flip there, where decoding
				// does not notice.
				b[len(b)/8+rng.Intn(len(b)-len(b)/8)] ^= byte(1 + rng.Intn(255))
			}
			src := corruptSource{Embedded, tg.marker, d, b}
			opts := options[iter%len(options)]
			name := fmt.Sprintf("%s %s, change %d", tg.marker, tg.tag, iter)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s: panicked: %v", name, r)
					}
				}()
				c, err := NewCollatorFrom(src, loc, opts)
				if err != nil {
					refused++
					return
				}
				built++
				for _, a := range words {
					for _, b := range words {
						c.Compare(a, b)
					}
				}
			}()
		}
	}
	if built == 0 {
		t.Errorf("every corrupt table was refused (%d): the test reached no comparison", refused)
	}
	t.Logf("%d corrupt tables built and compared with, %d refused", built, refused)
}

func mustParse(t *testing.T, tag string) Locale {
	t.Helper()
	l, err := ParseLocale(tag)
	if err != nil {
		t.Fatal(err)
	}
	return l
}
