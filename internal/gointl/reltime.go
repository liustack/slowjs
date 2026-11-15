package intl

import (
	"fmt"
	"math"
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/reltimedata"
)

// Intl.RelativeTimeFormat.
//
// A language says "in three days" two ways, and which one is wanted is the
// caller's business rather than the language's. Counted out, every offset has
// the same shape: "in 1 day", "1 day ago". Worded, the ones a language has a
// name for take it instead: "tomorrow", "yesterday", "now". The first is
// clearer in a table and the second reads better in a sentence.

// RelativeTimeUnit is what is being counted.
type RelativeTimeUnit int

const (
	RelativeYear RelativeTimeUnit = iota
	RelativeQuarter
	RelativeMonth
	RelativeWeek
	RelativeDay
	RelativeHour
	RelativeMinute
	RelativeSecond
)

// ParseRelativeTimeUnit reads a unit as ECMA-402 names it, in either the
// singular or the plural: both "day" and "days" are allowed.
func ParseRelativeTimeUnit(name string) (RelativeTimeUnit, bool) {
	name = strings.TrimSuffix(name, "s")
	for i, n := range reltimedata.Names {
		if n == name {
			return RelativeTimeUnit(i), true
		}
	}
	return 0, false
}

// RelativeTimeNumeric is whether a wording may stand in for the count.
type RelativeTimeNumeric int

const (
	// RelativeAlways always counts: "in 1 day". It is ECMA-402's default.
	RelativeAlways RelativeTimeNumeric = iota
	// RelativeAuto prefers a wording where the language has one: "tomorrow".
	RelativeAuto
)

// RelativeTimeStyle is how much room the wording takes.
type RelativeTimeStyle int

const (
	RelativeLong RelativeTimeStyle = iota
	RelativeShort
	RelativeNarrow
)

// RelativeTimeFormatOptions mirrors the option bag of
// Intl.RelativeTimeFormat. Its zero value is ECMA-402's default in every
// field.
type RelativeTimeFormatOptions struct {
	Numeric RelativeTimeNumeric
	Style   RelativeTimeStyle
	// NumberingSystem names the digits to write, as for NumberFormat.
	NumberingSystem string

	// Compat chooses between the standard and Node's observable behavior.
	Compat Compat
}

// A RelativeTimeFormat writes relative times in one locale. It never changes
// after it is built and is safe for any number of goroutines to share.
type RelativeTimeFormat struct {
	locale  Locale
	opts    RelativeTimeFormatOptions
	data    *reltimedata.Locale
	width   int
	numbers *NumberFormat
	plurals *PluralRules
}

// NewRelativeTimeFormat builds a formatter from the data built into the
// package.
//
// It works in loc as it is given. Resolve a requested locale among the
// service's available locales first, with a LocaleMatcher, to answer as
// ECMA-402 and Node do: "az-Arab" resolves to "az".
func NewRelativeTimeFormat(loc Locale, opts RelativeTimeFormatOptions) (*RelativeTimeFormat, error) {
	return NewRelativeTimeFormatFrom(Embedded, loc, opts)
}

// NewRelativeTimeFormatFrom builds a formatter from a source of the caller's
// own.
func NewRelativeTimeFormatFrom(src Source, loc Locale, opts RelativeTimeFormatOptions) (*RelativeTimeFormat, error) {
	data, err := loadRelativeTime(src, loc)
	if err != nil {
		return nil, err
	}
	f := &RelativeTimeFormat{locale: loc, opts: opts, data: data}
	switch opts.Style {
	case RelativeShort:
		f.width = reltimedata.Short
	case RelativeNarrow:
		f.width = reltimedata.Narrow
	default:
		f.width = reltimedata.Long
	}
	if f.numbers, err = NewNumberFormatFrom(src, loc, NumberFormatOptions{
		NumberingSystem: opts.NumberingSystem,
	}); err != nil {
		return nil, err
	}
	// Like NumberFormat, it uses only the numbering system of the extension.
	nu, _ := f.numbers.locale.keywordValue("nu")
	f.locale = loc.onlyKeywords().withKeyword("nu", nu)
	if f.plurals, err = NewPluralRulesFrom(src, loc, PluralRulesOptions{}); err != nil {
		return nil, err
	}
	return f, nil
}

func loadRelativeTime(src Source, loc Locale) (*reltimedata.Locale, error) {
	chain := loc.Fallback()
	if fb, err := NewFallbacker(src); err == nil {
		chain = fb.ChainIn(treeLocales, loc.Data())
	}
	pool, err := openShared(src, MarkerRelativeTimeShared, reltimedata.Version)
	if err != nil {
		return nil, err
	}
	for _, d := range chain {
		b, err := src.Open(MarkerRelativeTime, d)
		if err != nil {
			continue
		}
		l, err := reltimedata.Decode(b, pool)
		if err != nil {
			return nil, fmt.Errorf("intl: the relative times for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no relative times for %s: %w", loc, ErrNotFound)
}

// RelativePartKind says what one piece of a relative time is.
type RelativePartKind string

const (
	// RelativeLiteral is text from the pattern, and RelativeInteger the count
	// itself. The pieces of the number keep the kinds a number has.
	RelativeLiteral RelativePartKind = "literal"
	RelativeInteger RelativePartKind = "integer"
)

// A RelativePart is one piece of a formatted relative time.
type RelativePart struct {
	Kind PartKind
	// Unit names what the piece counts, and is empty for the text around it.
	Unit  string
	Value string
}

// wordOffset is the offset a word may be found for: a whole number, or with
// RelativeEpsilon, as ICU's formatRelativeImpl rounds it, a hundred times
// the offset rounded half away from zero, for an offset within 0.005 of a
// whole one from -2 to 2.
func (f *RelativeTimeFormat) wordOffset(v float64) (int, bool) {
	if !f.opts.Compat.Has(RelativeEpsilon) {
		return int(v), v == math.Trunc(v) && !math.IsInf(v, 0)
	}
	if !(v > -2.1 && v < 2.1) {
		return 0, false
	}
	x := float64(v * 100)
	var hundredths int32
	if x < 0 {
		hundredths = int32(float64(x - 0.5))
	} else {
		hundredths = int32(float64(x + 0.5))
	}
	return int(hundredths / 100), hundredths%100 == 0
}

// Format writes a relative time.
func (f *RelativeTimeFormat) Format(v float64, unit RelativeTimeUnit) string {
	var b strings.Builder
	for _, p := range f.FormatToParts(v, unit) {
		b.WriteString(p.Value)
	}
	return b.String()
}

// FormatToParts writes a relative time as the pieces it is made of, naming
// which of them counts.
func (f *RelativeTimeFormat) FormatToParts(v float64, unit RelativeTimeUnit) []RelativePart {
	field := f.data.Field(int(unit), f.width)

	// A wording stands in for the count only when one was asked for, the
	// offset is a whole number, and the language has a word for it.
	if f.opts.Numeric == RelativeAuto {
		if offset, ok := f.wordOffset(v); ok {
			if word, ok := field.Word(offset); ok {
				return []RelativePart{{Kind: PartLiteral, Value: word}}
			}
		}
	}

	magnitude := math.Abs(v)
	// Zero looks to the future: "in 0 seconds" rather than "0 seconds ago".
	future := !math.Signbit(v)

	integer, fraction := f.numbers.round(magOf(magnitude), false)
	o := operandsFor(padInteger(integer, f.numbers.minInt), fraction, 0)
	count := string(f.plurals.selectOperands(&o))

	pattern := field.Pattern(future, count)
	if pattern == "" {
		return []RelativePart{{Kind: PartLiteral, Value: f.numbers.Format(magnitude)}}
	}

	number := f.numbers.FormatToParts(magnitude)
	var out []RelativePart
	literal := func(s string) {
		if s != "" {
			out = append(out, RelativePart{Kind: PartLiteral, Value: s})
		}
	}
	rest := pattern
	for {
		at := strings.IndexByte(rest, '{')
		if at < 0 || at+2 >= len(rest) || rest[at+2] != '}' {
			break
		}
		literal(rest[:at])
		if rest[at+1] == '0' {
			for _, p := range number {
				out = append(out, RelativePart{
					Kind: p.Kind, Unit: unit.String(), Value: p.Value,
				})
			}
		} else {
			literal(rest[at : at+3])
		}
		rest = rest[at+3:]
	}
	literal(rest)
	return out
}

// ResolvedRelativeTimeFormat is what a formatter settled on.
type ResolvedRelativeTimeFormat struct {
	Locale          string
	Style           RelativeTimeStyle
	Numeric         RelativeTimeNumeric
	NumberingSystem string
}

// ResolvedOptions returns what the formatter settled on.
func (f *RelativeTimeFormat) ResolvedOptions() ResolvedRelativeTimeFormat {
	return ResolvedRelativeTimeFormat{
		Locale:          f.locale.String(),
		Style:           f.opts.Style,
		Numeric:         f.opts.Numeric,
		NumberingSystem: f.numbers.ResolvedOptions().NumberingSystem,
	}
}
