// Package plurdata is the model layer for plural rules: which categories a
// language has and the condition that selects each one.
//
// What is stored is CLDR's rule, as CLDR writes it -- "i = 1 and v = 0" -- and
// never which category a particular number falls in. The rule is the input; a
// category is an answer, and answers are not stored.
package plurdata

import (
	"github.com/liustack/slowjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 2

// A Rule is one category and the condition that chooses it.
type Rule struct {
	// Category is "zero", "one", "two", "few", "many" or "other".
	Category string
	// Condition is the rule as CLDR writes it, with its sample lists removed.
	// An empty condition always matches, which is how "other" is written.
	Condition string
}

// Locale holds a language's rules, in the order they are tried. The first
// whose condition holds wins, and "other" comes last and always holds.
type Locale struct {
	Cardinal []Rule
	Ordinal  []Rule
	// Ranges say the category of a range from the categories of its ends,
	// as CLDR gives them for the language. A pair not listed is "other".
	Ranges []Range
}

// A Range is the category of a range whose ends are Start and End.
type Range struct {
	Start, End, Result string
}

// Encode writes a locale's rules.
func Encode(l *Locale) []byte {
	w := blob.NewWriter(Version)
	for _, set := range [][]Rule{l.Cardinal, l.Ordinal} {
		w.Uint(len(set))
		for _, r := range set {
			w.String(r.Category)
			w.String(r.Condition)
		}
	}
	w.Uint(len(l.Ranges))
	for _, r := range l.Ranges {
		w.String(r.Start)
		w.String(r.End)
		w.String(r.Result)
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
	for _, set := range []*[]Rule{&l.Cardinal, &l.Ordinal} {
		n := r.Uint()
		if n < 0 || n > r.Left() {
			break
		}
		rules := make([]Rule, 0, n)
		for i := 0; i < n; i++ {
			category := r.String()
			condition := r.String()
			rules = append(rules, Rule{Category: category, Condition: condition})
		}
		*set = rules
	}
	if n := r.Uint(); n >= 0 && n <= r.Left() {
		l.Ranges = make([]Range, n)
		for i := range l.Ranges {
			l.Ranges[i] = Range{Start: r.String(), End: r.String(), Result: r.String()}
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
