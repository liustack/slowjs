package intl

import (
	"fmt"
	"strings"
)

// Number patterns, as UTS #35 writes them.
//
// A pattern says how a number is laid out without saying what any particular
// number looks like: "#,##0.###" is three digits to a group, at least one
// before the point and at most three after it; "¤#,##0.00" puts the
// currency first and always writes two decimals. CLDR ships these and this
// takes them apart. Nothing here is generated, and no formatted number is
// stored anywhere -- that is the whole point of keeping the pattern.
//
// A pattern may give two forms separated by a semicolon, the second for
// negative numbers, which is how a locale that brackets a loss rather than
// signing it says so.

type pattern struct {
	posPrefix, posSuffix string
	negPrefix, negSuffix string
	// explicitNeg says the pattern gave a negative form of its own. Without
	// one, a negative number takes the positive form with a minus in front.
	explicitNeg bool

	minInt           int
	minFrac, maxFrac int
	// primaryGroup is how many digits are in the group nearest the point and
	// secondaryGroup how many are in each group above it. They differ in
	// India, where 1234567 is written 12,34,567. Zero means no grouping.
	primaryGroup, secondaryGroup int
}

func parsePattern(src string) (*pattern, error) {
	if src == "" {
		return nil, fmt.Errorf("the pattern is empty")
	}
	positive, negative, hasNeg := cutPattern(src)

	p := &pattern{}
	prefix, suffix, err := p.readSubpattern(positive)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", src, err)
	}
	p.posPrefix, p.posSuffix = prefix, suffix

	if hasNeg {
		// The negative form contributes only its affixes: the digits are the
		// positive form's, which is what UTS #35 says and what keeps a pattern
		// like "#,##0.00;(#,##0.00)" from being read as two different shapes.
		var ignored pattern
		negPrefix, negSuffix, err := ignored.readSubpattern(negative)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", src, err)
		}
		p.negPrefix, p.negSuffix, p.explicitNeg = negPrefix, negSuffix, true
	}
	if p.minInt == 0 && p.maxFrac == 0 && p.primaryGroup == 0 {
		return nil, fmt.Errorf("%q has no number in it", src)
	}
	return p, nil
}

// cutPattern splits a pattern on the semicolon that separates its two forms,
// ignoring one inside quotes.
func cutPattern(src string) (positive, negative string, found bool) {
	quoted := false
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\'':
			quoted = !quoted
		case ';':
			if !quoted {
				return src[:i], src[i+1:], true
			}
		}
	}
	return src, "", false
}

// readSubpattern takes one form apart into its prefix, its number, and its
// suffix, filling in the digit and grouping counts as it goes.
func (p *pattern) readSubpattern(src string) (prefix, suffix string, err error) {
	start := strings.IndexAny(src, "#0,.@")
	if start < 0 {
		return "", "", fmt.Errorf("%q has no number in it", src)
	}
	end := start
	for end < len(src) && strings.ContainsRune("#0,.@", rune(src[end])) {
		end++
	}
	if err := p.readNumber(src[start:end]); err != nil {
		return "", "", err
	}
	return unquote(src[:start]), unquote(src[end:]), nil
}

// readNumber reads the digits, the point and the grouping commas.
func (p *pattern) readNumber(src string) error {
	integer, fraction, _ := strings.Cut(src, ".")

	for i := 0; i < len(integer); i++ {
		if integer[i] == '0' {
			p.minInt++
		}
	}
	for i := 0; i < len(fraction); i++ {
		switch fraction[i] {
		case '0':
			p.minFrac++
			p.maxFrac++
		case '#':
			p.maxFrac++
		}
	}

	// The groups are measured from the point outwards: the digits after the
	// last comma are the first group, and those between the last two commas
	// every group above it. A pattern with one comma repeats that group.
	var groups []int
	count := 0
	for i := len(integer) - 1; i >= 0; i-- {
		switch integer[i] {
		case ',':
			groups = append(groups, count)
			count = 0
		default:
			count++
		}
	}
	switch len(groups) {
	case 0:
	case 1:
		p.primaryGroup, p.secondaryGroup = groups[0], groups[0]
	default:
		p.primaryGroup, p.secondaryGroup = groups[0], groups[1]
	}
	if p.primaryGroup == 0 {
		p.secondaryGroup = 0
	}
	return nil
}

// unquote reads the literal text of an affix. An apostrophe quotes what
// follows so that a pattern can write a percent sign that is not the percent
// symbol, and two of them are one apostrophe.
func unquote(src string) string {
	if !strings.Contains(src, "'") {
		return src
	}
	var b strings.Builder
	for i := 0; i < len(src); i++ {
		if src[i] != '\'' {
			b.WriteByte(src[i])
			continue
		}
		if i+1 < len(src) && src[i+1] == '\'' {
			b.WriteByte('\'')
			i++
			continue
		}
		// An opening quote: copy to the closing one.
		for i++; i < len(src) && src[i] != '\''; i++ {
			b.WriteByte(src[i])
		}
	}
	return b.String()
}

// prefixFor and suffixFor give the affixes for a number of a given sign.
func (p *pattern) prefixFor(neg bool) string {
	if neg && p.explicitNeg {
		return p.negPrefix
	}
	return p.posPrefix
}

func (p *pattern) suffixFor(neg bool) string {
	if neg && p.explicitNeg {
		return p.negSuffix
	}
	return p.posSuffix
}
