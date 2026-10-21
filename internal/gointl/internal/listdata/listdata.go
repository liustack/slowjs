// Package listdata is the model layer for list formatting: the patterns a
// locale joins a list with.
//
// A locale gives four patterns for each kind of list -- how a list of exactly
// two is written, and how the first, middle and last joins are written for a
// longer one. What is stored is those patterns. No joined list is stored.
package listdata

import (
	"github.com/liustack/quickjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 1

// The kinds of list, in the order they are stored.
const (
	// Standard is "a, b, and c", the kind ECMA-402 calls a conjunction.
	Standard = iota
	// Or is "a, b, or c", a disjunction.
	Or
	// Unit is "a, b, c", a list of measurements.
	Unit
	kinds
)

// The widths, in the order they are stored.
const (
	Long = iota
	Short
	Narrow
	widths
)

// Count is how many pattern sets a locale holds.
const Count = kinds * widths

// Patterns is one locale's way of joining one kind of list.
type Patterns struct {
	// Two joins a list of exactly two. Start, Middle and End join the first,
	// the middle and the last pair of a longer one.
	Two    string
	Start  string
	Middle string
	End    string
}

// Empty reports whether a set was not given, in which case a wider one stands
// in for it: CLDR leaves out a narrow set that is the same as the short one.
func (p Patterns) Empty() bool { return p.Two == "" && p.Start == "" }

// Locale holds every set a locale gives.
type Locale struct {
	Sets [Count]Patterns
}

// Get returns one set, falling back from narrow to short to long, which is
// what CLDR's own inheritance does for the widths it leaves out.
func (l *Locale) Get(kind, width int) Patterns {
	if kind < 0 || kind >= kinds {
		kind = Standard
	}
	for w := width; w >= Long; w-- {
		if p := l.Sets[kind*widths+w]; !p.Empty() {
			return p
		}
	}
	return l.Sets[kind*widths+Long]
}

// Encode writes a locale's patterns.
func Encode(l *Locale) []byte {
	w := blob.NewWriter(Version)
	for _, p := range l.Sets {
		w.String(p.Two)
		w.String(p.Start)
		w.String(p.Middle)
		w.String(p.End)
	}
	return w.Bytes()
}

// Decode reads what Encode wrote.
func Decode(b []byte) (*Locale, error) {
	r, err := blob.NewReader(b, Version)
	if err != nil {
		return nil, err
	}
	var l Locale
	for i := range l.Sets {
		l.Sets[i] = Patterns{
			Two:    r.String(),
			Start:  r.String(),
			Middle: r.String(),
			End:    r.String(),
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
