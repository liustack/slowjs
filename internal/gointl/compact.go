package intl

import (
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/numdata"
)

// Compact notation: 1234 as "1.2K", or "1.2 thousand".
//
// Two things make this more than dividing and appending a letter.
//
// The pattern is chosen by the plural category of the divided amount, because
// a language may write one million differently from three. That is why this
// waits on plural rules rather than arriving with the rest of NumberFormat.
//
// The rounding is ECMA-402's "more precision", which is not a rounding mode so
// much as a choice between two. A compact number is rounded both to no
// decimals and to two significant digits, and whichever keeps more is used:
// 1.2345 thousand is "1.2K" because two significant digits keep more than no
// decimals would, and 123.456 billion is "123B" because no decimals keeps more
// than two significant digits would.

// compactForm is the pattern chosen for one magnitude and plural category.
type compactForm struct {
	// exponent is the power of ten the number is divided by before it is
	// written, which is the "c" operand a plural rule may test.
	exponent int
	prefix   string
	suffix   string
	// zeros is how many digits the pattern asks for, which sets the minimum
	// integer digits of the divided amount.
	zeros int
	// noBody is a pattern with no digits, French "mille" for exactly a
	// thousand: ICU writes its text in place of the number.
	noBody bool
	// negPrefix and negSuffix are the pattern's negative form, where it has
	// one, which places the sign: Swahili's "elfu 0;elfu -0".
	negPrefix, negSuffix string
	hasNeg               bool
	// whole is a currency's compact pattern, which stands in place of the
	// currency pattern, its sign and all, rather than inside it.
	whole bool
}

// chooseCompact picks the pattern for a magnitude. It returns false when the
// number is too small to compact, which is every number below a thousand.
func chooseCompact(patterns []numdata.CompactPattern, want int) (int, bool) {
	if want < 0 {
		return 0, false
	}
	// The largest power of ten the patterns cover that is no larger than the
	// number itself.
	best, found := 0, false
	for _, p := range patterns {
		if p.Exponent <= want && (!found || p.Exponent > best) {
			best, found = p.Exponent, true
		}
	}
	// CompactData::getPattern: a magnitude whose pattern is "0" is written
	// out in full (USE_FALLBACK), not with a smaller magnitude's pattern.
	for _, p := range patterns {
		if found && p.Exponent == best && p.Pattern != "0" {
			return best, true
		}
	}
	return 0, false
}

// compactFor builds the form for one exponent and plural category, falling
// back to the "other" category as CLDR's own lookup does.
func compactFor(patterns []numdata.CompactPattern, exponent int, count string) (compactForm, bool) {
	var chosen string
	for _, p := range patterns {
		if p.Exponent != exponent || p.Pattern == "0" {
			continue
		}
		if p.Count == count {
			chosen = p.Pattern
			break
		}
		if p.Count == "other" && chosen == "" {
			chosen = p.Pattern
		}
	}
	if chosen == "" {
		return compactForm{}, false
	}

	negative := ""
	if cut := strings.IndexByte(chosen, ';'); cut >= 0 {
		chosen, negative = chosen[:cut], chosen[cut+1:]
	}

	// The zeros in the pattern say how many digits of the magnitude stay: "0K"
	// at a thousand divides by a thousand, "00K" at ten thousand also divides
	// by a thousand and writes two digits.
	start := strings.IndexByte(chosen, '0')
	if start < 0 {
		// A pattern with no digits divides as the magnitude's others do.
		if count == "other" {
			return compactForm{}, false
		}
		other, ok := compactFor(patterns, exponent, "other")
		if !ok {
			return compactForm{}, false
		}
		return compactForm{exponent: other.exponent, prefix: unquote(chosen), zeros: other.zeros, noBody: true}, true
	}
	end := start
	for end < len(chosen) && chosen[end] == '0' {
		end++
	}
	zeros := end - start
	form := compactForm{
		exponent: exponent - (zeros - 1),
		prefix:   unquote(chosen[:start]),
		suffix:   unquote(chosen[end:]),
		zeros:    zeros,
	}
	if first := strings.IndexByte(negative, '0'); first >= 0 {
		last := first
		for last < len(negative) && negative[last] == '0' {
			last++
		}
		form.negPrefix, form.negSuffix = unquote(negative[:first]), unquote(negative[last:])
		form.hasNeg = true
	}
	return form, true
}

// hasCompactCount reports whether a magnitude has a pattern for a count of
// its own, rather than by falling back to "other".
func hasCompactCount(patterns []numdata.CompactPattern, exponent int, count string) bool {
	for _, p := range patterns {
		if p.Exponent == exponent && p.Count == count {
			return true
		}
	}
	return false
}

// compactAffix writes a compact pattern's text as a "compact" part, with
// the spaces around it as literals, as ICU trims the whitespace off a
// field's ends.
func compactAffix(props *unicodeProps, parts []Part, text string) []Part {
	return affixAs(props, parts, text, PartCompact)
}

// affixAs writes a pattern's text as one part of a kind, with the spaces
// around it as literals, as ICU trims the whitespace off a field's ends.
func affixAs(props *unicodeProps, parts []Part, text string, kind PartKind) []Part {
	core := strings.TrimLeftFunc(text, props.ignorable)
	if lead := text[:len(text)-len(core)]; lead != "" {
		parts = append(parts, Part{PartLiteral, lead})
	}
	trimmed := strings.TrimRightFunc(core, props.ignorable)
	if trimmed != "" {
		parts = append(parts, Part{kind, trimmed})
	}
	if trail := core[len(trimmed):]; trail != "" {
		parts = append(parts, Part{PartLiteral, trail})
	}
	return parts
}

// compactPieces writes a number in compact notation: the pattern's text
// before the digits, the digits, and its text after them. With useNegative
// the pattern's negative form is written, its minus replaced by the sign
// symbols.
func (f *NumberFormat) compactPieces(form compactForm, magnitude mag, negative bool,
	useNegative bool, symbols []Part) (pre, body, post []Part) {
	if form.noBody {
		return f.compactText(nil, form.prefix), nil, nil
	}

	// The digit counts are settled once, when the formatter is built, so a
	// compact number rounds the same way as any other: by whichever counting
	// the options chose, which for a compact number with nothing asked for is
	// two significant digits or no decimals, whichever keeps more.
	value := magnitude.shift(-form.exponent)
	integer, fraction := f.round(value, negative)
	integer = padInteger(integer, max(f.minInt, 1))

	body = f.groupedInteger(integer)
	if fraction != "" {
		body = append(body, Part{PartDecimal, f.decimalSep}, Part{PartFraction, f.digits(fraction)})
	}
	if useNegative {
		return f.compactSigned(form.negPrefix, symbols), body, f.compactSigned(form.negSuffix, symbols)
	}
	return f.compactText(nil, form.prefix), body, f.compactText(nil, form.suffix)
}

// compactSigned writes a compact affix whose minus is a sign placeholder.
func (f *NumberFormat) compactSigned(affix string, symbols []Part) []Part {
	var parts []Part
	for i, piece := range strings.Split(affix, "-") {
		if i > 0 {
			parts = append(parts, symbols...)
		}
		parts = f.compactText(parts, piece)
	}
	return parts
}

// compactText writes a compact pattern's text: a currency's sign where the
// pattern has one, "¤0K", and the rest as compactAffix writes it.
func (f *NumberFormat) compactText(parts []Part, text string) []Part {
	for i, piece := range strings.Split(text, "¤") {
		if i > 0 {
			parts = append(parts, Part{PartCurrency, f.currencyText})
		}
		parts = compactAffix(f.props, parts, piece)
	}
	return parts
}

// compactPatterns are the patterns compact notation writes with: a
// currency's own where it is written with a symbol or a code, as ICU's
// TYPE_CURRENCY, whose long form CLDR leaves to the short; the decimal ones
// otherwise, a spelled-out currency name among them.
func (f *NumberFormat) compactPatterns() ([]numdata.CompactPattern, bool) {
	if f.opts.Style == StyleCurrency && f.opts.CurrencyDisplay != CurrencyName {
		return f.data.CurrencyCompact, true
	}
	if f.opts.CompactDisplay == CompactLong {
		return f.data.CompactLong, false
	}
	return f.data.CompactShort, false
}

// compactForm chooses the compact pattern a magnitude is written with.
func (f *NumberFormat) compactForm(magnitude mag, negative bool) compactForm {
	patterns, currency := f.compactPatterns()

	form := compactForm{zeros: 1}
	if magnitude.isZero() {
		return form
	}
	// CompactHandler::processQuantity: the pattern is the one for the power
	// of ten of the number as rounded. Rounding can carry it into the next
	// power, and the pattern is that power's even where it divides by the
	// same amount: Arabic writes 9999.9 as ten thousand, "10 ألف", not as
	// ten thousands, "10 آلاف". Where the next power divides by another
	// amount the number is rounded again by it (chooseMultiplierAndApply):
	// 999,999.5 is "1M", not "1000K".
	want := magnitude.exponent()
	divisor := func(want int) int {
		if exponent, ok := chooseCompact(patterns, want); ok {
			if first, ok := compactFor(patterns, exponent, "other"); ok {
				return first.exponent
			}
		}
		return 0
	}
	by := divisor(want)
	integer, fraction := f.round(magnitude.shift(-by), negative)
	if rounded := makeMag(integer, fraction); !rounded.isZero() && rounded.exponent()+by != want {
		want++
	}
	exponent, ok := chooseCompact(patterns, want)
	if ok {
		// The plural category is that of the divided amount, so the amount has
		// to be divided before the pattern can be chosen -- and the pattern is
		// what says by how much. CLDR's patterns for one magnitude all divide
		// by the same amount, so a first pass with the "other" category settles
		// the divisor and a second picks the wording. ICU asks the plural
		// rules about the divided amount alone, before it records the power
		// of ten it divided by.
		if first, ok := compactFor(patterns, exponent, "other"); ok {
			divided := magnitude.shift(-first.exponent)
			integer, fraction := f.round(divided, negative)
			// CompactData::getPattern: an amount that is exactly 0 or 1
			// takes the pattern CLDR gives for that number, where it gives
			// one -- French writes a thousand as "mille". The amount is
			// signed: minus a thousand is "-1 millier".
			if strings.Trim(fraction, "0") == "" {
				exact := ""
				switch strings.TrimLeft(integer, "0") {
				case "":
					exact = "0"
				case "1":
					if !negative {
						exact = "1"
					}
				}
				if exact != "" && hasCompactCount(patterns, exponent, exact) {
					if form, ok := compactFor(patterns, exponent, exact); ok {
						form.whole = currency
						return form
					}
				}
			}
			category := string(PluralOther)
			if f.plurals != nil {
				o := operandsFor(integer, fraction, 0)
				category = string(f.plurals.selectOperands(&o))
			}
			if chosen, ok := compactFor(patterns, exponent, category); ok {
				form = chosen
			} else {
				form = first
			}
			form.whole = currency
		}
	}
	return form
}

// selectOperands is Select for operands that have already been worked out,
// which the compact path needs because it has the digits in hand.
func (p *PluralRules) selectOperands(o *operands) PluralCategory {
	for _, r := range p.tried {
		if r.rule.matches(o) {
			return r.category
		}
	}
	return PluralOther
}
