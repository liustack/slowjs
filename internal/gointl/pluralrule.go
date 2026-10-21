package intl

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// CLDR's plural rule language.
//
// A rule is a condition over the operands UTS #35 defines, and it reads close
// to English: "i = 1 and v = 0" is the English "one", "v = 0 and i % 10 = 2..4
// and i % 100 != 12..14" is the Polish "few". The grammar is small:
//
//	condition     = and_condition ('or' and_condition)*
//	and_condition = relation ('and' relation)*
//	relation      = expr ('=' | '!=') range_list
//	expr          = operand (('mod' | '%') value)?
//	range_list    = (range | value) (',' range_list)*
//	range         = value '..' value
//
// The rules are parsed when a PluralRules is built, not when a number is
// selected, so selecting walks a small tree rather than a string.

// operands are the values a rule can test, as UTS #35 names them. They are
// taken from the number *as it would be written*, not from the number alone:
// 1.0 and 1 are the same value but not the same plural in every language,
// because v differs.
type operands struct {
	n float64 // the absolute value
	i int64   // the integer digits
	v int     // how many fraction digits are written, trailing zeros included
	w int     // how many are written with trailing zeros removed
	f int64   // those fraction digits as a number, trailing zeros included
	t int64   // the same with trailing zeros removed
	c int     // the exponent of a compact or scientific form
}

// value returns one operand by its letter.
func (o *operands) value(name byte) (float64, bool) {
	switch name {
	case 'n':
		return o.n, true
	case 'i':
		return float64(o.i), true
	case 'v':
		return float64(o.v), true
	case 'w':
		return float64(o.w), true
	case 'f':
		return float64(o.f), true
	case 't':
		return float64(o.t), true
	case 'c', 'e':
		return float64(o.c), true
	}
	return 0, false
}

// operandsFor builds the operands for a number written with a given fraction.
// The integer and fraction come from the formatter, so that "1.0" and "1" are
// told apart, which is the whole reason v and w exist.
func operandsFor(integer, fraction string, exponent int) operands {
	o := operands{v: len(fraction), c: exponent}

	digits := integer
	if fraction != "" {
		digits = integer + "." + fraction
	}
	o.n, _ = strconv.ParseFloat(digits, 64)
	// ICU's i keeps the lowest eighteen digits of a number too long for
	// them all (DecimalQuantity::toLong), so 10^21 has an i of zero.
	if len(integer) > 18 {
		integer = integer[len(integer)-18:]
	}
	o.i, _ = strconv.ParseInt(integer, 10, 64)

	if fraction != "" {
		o.f, _ = strconv.ParseInt(fraction, 10, 64)
		trimmed := strings.TrimRight(fraction, "0")
		o.w = len(trimmed)
		if trimmed != "" {
			o.t, _ = strconv.ParseInt(trimmed, 10, 64)
		}
	}
	return o
}

// A pluralRule is a parsed condition. An empty one always holds, which is how
// "other" is written.
type pluralRule struct {
	// ors hold the alternatives; the rule holds if any of them does.
	ors []andCondition
}

type andCondition struct {
	relations []relation
}

type relation struct {
	operand byte
	mod     int64 // zero when the relation has no modulus
	negated bool
	ranges  []numRange
}

type numRange struct{ lo, hi float64 }

// parsePluralRule reads one CLDR condition.
func parsePluralRule(src string) (*pluralRule, error) {
	out := &pluralRule{}
	src = strings.TrimSpace(src)
	if src == "" {
		return out, nil
	}
	for _, alternative := range splitKeyword(src, "or") {
		var and andCondition
		for _, part := range splitKeyword(alternative, "and") {
			rel, err := parseRelation(part)
			if err != nil {
				return nil, fmt.Errorf("%q: %w", src, err)
			}
			and.relations = append(and.relations, rel)
		}
		if len(and.relations) == 0 {
			return nil, fmt.Errorf("%q has an empty alternative", src)
		}
		out.ors = append(out.ors, and)
	}
	return out, nil
}

// splitKeyword splits on a whole word, so that a rule is not cut at the "or"
// inside some other token.
func splitKeyword(src, word string) []string {
	var out []string
	fields := strings.Fields(src)
	current := make([]string, 0, len(fields))
	for _, f := range fields {
		if f == word {
			out = append(out, strings.Join(current, " "))
			current = current[:0]
			continue
		}
		current = append(current, f)
	}
	return append(out, strings.Join(current, " "))
}

func parseRelation(src string) (relation, error) {
	var rel relation
	src = strings.TrimSpace(src)

	left, right, ok := strings.Cut(src, "!=")
	if ok {
		rel.negated = true
	} else if left, right, ok = strings.Cut(src, "="); !ok {
		return rel, fmt.Errorf("the relation %q has no comparison", src)
	}

	left = strings.TrimSpace(left)
	// An expression may take a modulus: "i % 100" or "i mod 100".
	name, modulus, hasMod := strings.Cut(left, "%")
	if !hasMod {
		name, modulus, hasMod = cutWord(left, "mod")
	}
	name = strings.TrimSpace(name)
	if len(name) != 1 {
		return rel, fmt.Errorf("%q is not an operand", name)
	}
	rel.operand = name[0]
	if _, ok := (&operands{}).value(rel.operand); !ok {
		return rel, fmt.Errorf("%q is not an operand", name)
	}
	if hasMod {
		m, err := strconv.ParseInt(strings.TrimSpace(modulus), 10, 64)
		if err != nil || m == 0 {
			return rel, fmt.Errorf("%q is not a modulus", modulus)
		}
		rel.mod = m
	}

	for _, item := range strings.Split(right, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return rel, fmt.Errorf("the relation %q has an empty value", src)
		}
		lo, hi, isRange := strings.Cut(item, "..")
		low, err := strconv.ParseFloat(strings.TrimSpace(lo), 64)
		if err != nil {
			return rel, fmt.Errorf("%q is not a number", lo)
		}
		high := low
		if isRange {
			if high, err = strconv.ParseFloat(strings.TrimSpace(hi), 64); err != nil {
				return rel, fmt.Errorf("%q is not a number", hi)
			}
		}
		rel.ranges = append(rel.ranges, numRange{low, high})
	}
	if len(rel.ranges) == 0 {
		return rel, fmt.Errorf("the relation %q compares with nothing", src)
	}
	return rel, nil
}

// cutWord splits on a whole word surrounded by spaces.
func cutWord(src, word string) (before, after string, found bool) {
	at := strings.Index(src, " "+word+" ")
	if at < 0 {
		return src, "", false
	}
	return src[:at], src[at+len(word)+2:], true
}

// matches reports whether a rule holds for a number.
func (r *pluralRule) matches(o *operands) bool {
	if len(r.ors) == 0 {
		return true
	}
	for _, and := range r.ors {
		if and.matches(o) {
			return true
		}
	}
	return false
}

func (a andCondition) matches(o *operands) bool {
	for _, rel := range a.relations {
		if !rel.matches(o) {
			return false
		}
	}
	return true
}

func (r relation) matches(o *operands) bool {
	v, ok := o.value(r.operand)
	if !ok {
		return false
	}
	if r.mod != 0 {
		// The remainder keeps the fraction: UTS #35 has 1.5 mod 10 as 1.5, not
		// 1. It matters for the English ordinal rule "n % 10 = 1", which holds
		// for 1 and 21 but not for 1.5 -- there is no 1.5th of anything.
		v = math.Mod(v, float64(r.mod))
	}
	in := false
	for _, rng := range r.ranges {
		// A range holds only for whole numbers in it: "n = 2..4" does not hold
		// for 2.5, which is what UTS #35 says and what tells "few" from
		// "other" in Polish.
		if v < rng.lo || v > rng.hi {
			continue
		}
		if rng.lo != rng.hi && v != float64(int64(v)) {
			continue
		}
		in = true
		break
	}
	return in != r.negated
}
