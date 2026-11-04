// Package namedata is the model layer for display names: what a locale calls a
// language, a region, a script, a currency, a calendar or a part of a date.
//
// This is the largest data set in the library by a wide margin -- seven
// hundred languages and three hundred regions in each of seven hundred
// locales -- and the reason it is not a problem is the Source it arrives
// through. A program that never asks what French is called in Japanese never
// reads a byte of it.
package namedata

import (
	"github.com/liustack/quickjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 4

// The kinds of name, in the order they are stored.
const (
	Language = iota
	Region
	Script
	Calendar
	DateTimeField
	// Variant is a language's variant, keyed as ICU keys it: upper case,
	// "VALENCIA", and long only.
	Variant
	Kinds
)

// The widths, in the order they are stored.
const (
	Long = iota
	Short
	Narrow
	Widths
)

// An Entry is one code and what the locale calls it.
type Entry struct {
	Code string
	Name string
}

// A Set is every name of one kind at one width.
type Set struct {
	Entries []Entry
}

// Built is what a generator builds for a locale: every kind at every
// width, plus the patterns that join a language to the region or script it
// is qualified by. A width holds only the names that differ from the next
// wider one's, which is what CLDR's own inheritance does: a locale gives a
// short name only where it differs from the long one.
type Built struct {
	Sets [Kinds * Widths]Set

	// Pattern puts a qualifier beside a language, "{0} ({1})", and Separator
	// joins two qualifiers, "{0}, {1}".
	Pattern   string
	Separator string
}

// Locale is a locale's names read where they lie: a DisplayNames names one
// code at a time, so each kind at each width is a table it looks the code up
// in rather than a list read whole.
type Locale struct {
	sets [Kinds * Widths]blob.Table

	Pattern   string
	Separator string
}

// Lookup finds a name at a width, falling back through the narrower forms to
// the long one, entry by entry rather than set by set.
func (l *Locale) Lookup(kind, width int, code string) (string, bool) {
	if kind < 0 || kind >= Kinds || width < Long || width >= Widths {
		return "", false
	}
	for w := width; w >= Long; w-- {
		r, ok := l.sets[kind*Widths+w].Find(code)
		if !ok {
			continue
		}
		if name := r.SharedString(); name != "" && r.Err() == nil {
			return name, true
		}
	}
	return "", false
}

// Encode writes a locale's names, with what it shares with other locales --
// every string and each set -- in pool, which the generator writes beside
// the locales and Decode is given.
func Encode(l *Built, pool *blob.Pool) []byte {
	b := blob.NewPooledWriter(Version, pool)
	b.SharedString(l.Pattern)
	b.SharedString(l.Separator)
	for _, s := range l.Sets {
		names := map[string]string{}
		codes := make([]string, 0, len(s.Entries))
		for _, e := range s.Entries {
			names[e.Code] = e.Name
			codes = append(codes, e.Code)
		}
		b.SharedTable(codes, func(code string, b *blob.Writer) { b.SharedString(names[code]) })
	}
	return b.Bytes()
}

// Decode reads what Encode wrote, with the pool it wrote into.
func Decode(data []byte, pool blob.Shared) (*Locale, error) {
	r, err := blob.NewPooledReader(data, Version, pool)
	if err != nil {
		return nil, err
	}
	var l Locale
	l.Pattern = r.SharedString()
	l.Separator = r.SharedString()
	for i := range l.sets {
		l.sets[i] = r.SharedTable()
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
