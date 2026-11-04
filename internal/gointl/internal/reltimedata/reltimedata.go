// Package reltimedata is the model layer for relative times: how a locale says
// that something happened a while ago or will happen in a while.
//
// A language says this two ways. It has patterns that take a number -- "in {0}
// days", "{0} days ago" -- one per plural category, and it has wordings for
// the offsets it has a name for: yesterday, today, tomorrow. Both are stored,
// and which is used is the caller's choice.
package reltimedata

import (
	"github.com/liustack/quickjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 2

// The units, in the order they are stored. These are the ones ECMA-402 allows.
const (
	Year = iota
	Quarter
	Month
	Week
	Day
	Hour
	Minute
	Second
	Units
)

// Names are the units as ECMA-402 writes them, in storage order.
var Names = [Units]string{
	"year", "quarter", "month", "week", "day", "hour", "minute", "second",
}

// The widths, in the order they are stored.
const (
	Long = iota
	Short
	Narrow
	Widths
)

// WidthNames are CLDR's suffixes for the widths, in storage order. The long
// one has no suffix.
var WidthNames = [Widths]string{"", "-short", "-narrow"}

// A CountedText is a wording and the plural category it belongs to.
type CountedText struct {
	Count string
	Text  string
}

// A Named is a wording for one offset: -1 is "yesterday".
type Named struct {
	Offset int
	Text   string
}

// A Field is everything a locale says about one unit at one width.
type Field struct {
	// Named are the offsets the locale has a word for, sorted by offset. Most
	// units have -1, 0 and 1; a second usually has only 0, "now".
	Named []Named
	// Future and Past place a count, "in {0} days" and "{0} days ago".
	Future []CountedText
	Past   []CountedText
}

// Word returns the locale's wording for an offset, if it has one.
func (f *Field) Word(offset int) (string, bool) {
	for _, n := range f.Named {
		if n.Offset == offset {
			return n.Text, n.Text != ""
		}
	}
	return "", false
}

// Pattern returns the counted pattern for a direction and plural category.
func (f *Field) Pattern(future bool, count string) string {
	set := f.Past
	if future {
		set = f.Future
	}
	var other string
	for _, p := range set {
		if p.Count == count {
			return p.Text
		}
		if p.Count == "other" {
			other = p.Text
		}
	}
	return other
}

// Built is every unit at every width, as a generator builds it.
type Built struct {
	Fields [Units * Widths]Field
}

// Locale is a locale's relative times read where they lie: a formatter
// writes one unit at a time, so a field is read from the pool when it is
// asked for rather than all of them when the formatter is built.
type Locale struct {
	fields [Units * Widths]int // part numbers in the pool
	pool   blob.Shared
}

// Field returns one unit at one width, falling back to the wider forms where a
// locale gives none, which is what CLDR's own inheritance does.
func (l *Locale) Field(unit, width int) Field {
	if unit < 0 || unit >= Units {
		unit = Year
	}
	if width < Long || width >= Widths {
		width = Long
	}
	for w := width; w > Long; w-- {
		if f := l.field(unit*Widths + w); len(f.Future) > 0 || len(f.Named) > 0 {
			return f
		}
	}
	return l.field(unit*Widths + Long)
}

// field reads one field from the pool. Decode found every field's part, so
// a field that does not read is malformed data, and reads as empty.
func (l *Locale) field(i int) Field {
	var f Field
	if err := l.pool.Read(l.fields[i], func(r *blob.Reader) { decodeField(r, &f) }); err != nil {
		return Field{}
	}
	return f
}

// Encode writes a locale's relative-time data, with what it shares with
// other locales -- every string and each field -- in pool, which the
// generator writes beside the locales and Decode is given.
func Encode(l *Built, pool *blob.Pool) []byte {
	b := blob.NewPooledWriter(Version, pool)
	for _, f := range l.Fields {
		b.Shared(func(b *blob.Writer) {
			b.Uint(len(f.Named))
			for _, n := range f.Named {
				// The offsets run from -1 upwards in practice, and the
				// writer takes no negative numbers, so they are shifted.
				b.Uint(n.Offset + 8)
				b.SharedString(n.Text)
			}
			for _, set := range [][]CountedText{f.Future, f.Past} {
				b.Uint(len(set))
				for _, p := range set {
					b.SharedString(p.Count)
					b.SharedString(p.Text)
				}
			}
		})
	}
	return b.Bytes()
}

func decodeField(r *blob.Reader, f *Field) {
	n := r.Uint()
	if n < 0 || n > r.Left() {
		return
	}
	f.Named = make([]Named, n)
	for j := range f.Named {
		offset := r.Uint() - 8
		f.Named[j] = Named{offset, r.SharedString()}
	}
	for _, set := range []*[]CountedText{&f.Future, &f.Past} {
		m := r.Uint()
		if m < 0 || m > r.Left() {
			return
		}
		out := make([]CountedText, m)
		for k := range out {
			out[k] = CountedText{Count: r.SharedString(), Text: r.SharedString()}
		}
		*set = out
	}
}

// Decode reads what Encode wrote, with the pool it wrote into, finding each
// field's part and leaving it there.
func Decode(data []byte, pool blob.Shared) (*Locale, error) {
	r, err := blob.NewPooledReader(data, Version, pool)
	if err != nil {
		return nil, err
	}
	l := Locale{pool: pool}
	for i := range l.fields {
		l.fields[i] = r.SharedNumber()
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
