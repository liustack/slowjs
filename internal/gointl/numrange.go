package intl

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Ranges of numbers: formatRange and formatRangeToParts.
//
// V8 writes a range with ICU's NumberRangeFormatter (numrange_impl.cpp) at
// its defaults: layers the two ends share are written once when they are
// more than a single character, and ends written alike are one number marked
// approximate. The layers are the ones numberLayers holds. A unit or
// currency name is always shared, in the plural the range as a whole takes:
// "1–2 kilometers". The pattern's affixes are shared when they match and
// run to two characters or more: German "5,00–10,00 €", but English
// "$5.00 – $10.00", whose dollar sign is one. Anything written twice puts
// spaces round the range's dash.

// icuSameEnds is whether ICU's Formattable::operator== holds a range's two
// ends equal, as V8 hands them over: a double -- a Number, or a string
// that is an infinity -- as itself, and a decimal -- a BigInt, or another
// numeric string -- as the integer it is where it fits in an int64, else as
// the double nearest it. An integer never equals a double.
func icuSameEnds(start, end Decimal) bool {
	si, sInt := icuInteger(start)
	ei, eInt := icuInteger(end)
	if sInt || eInt {
		return sInt && eInt && si == ei
	}
	return icuDouble(start) == icuDouble(end)
}

// icuInteger is a decimal that fits in an int64 as the integer ICU's
// Formattable holds it as.
func icuInteger(d Decimal) (int64, bool) {
	if d.kind != decimalFinite || d.fromFloat || d.m.fraction != "" {
		return 0, false
	}
	n, err := strconv.ParseInt(d.m.integer, 10, 64)
	if err != nil {
		// Past an int64; -2^63 alone fits only as a negative number.
		if d.neg && d.m.integer == "9223372036854775808" {
			return math.MinInt64, true
		}
		return 0, false
	}
	if d.neg {
		n = -n
	}
	return n, true
}

// icuDouble is the double ICU's Formattable holds an end as.
func icuDouble(d Decimal) float64 {
	if d.fromFloat {
		return d.float
	}
	return d.Float()
}

// FormatRange writes a range of two numbers.
func (f *NumberFormat) FormatRange(start, end float64) (string, error) {
	return f.FormatDecimalRange(DecimalFromFloat(start), DecimalFromFloat(end))
}

// FormatRangeToParts writes a range of two numbers as the pieces it is made
// of, each marked with the end it belongs to.
func (f *NumberFormat) FormatRangeToParts(start, end float64) ([]RangePart, error) {
	return f.FormatDecimalRangeToParts(DecimalFromFloat(start), DecimalFromFloat(end))
}

// FormatDecimalRange writes a range of two numbers given exactly.
func (f *NumberFormat) FormatDecimalRange(start, end Decimal) (string, error) {
	parts, err := f.FormatDecimalRangeToParts(start, end)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Value)
	}
	return b.String(), nil
}

// FormatDecimalRangeToParts writes a range of two numbers given exactly, as
// pieces. Either end being NaN is an error, as ECMA-402 throws.
func (f *NumberFormat) FormatDecimalRangeToParts(start, end Decimal) ([]RangePart, error) {
	if start.IsNaN() || end.IsNaN() {
		return nil, fmt.Errorf("intl: a number range with an end that is not a number")
	}
	// Ends ICU holds equal before rounding are written as one number, with
	// Node's quirks, though they round apart (DoubleRangeIdentity).
	sameBefore := f.opts.Compat.Has(DoubleRangeIdentity) && icuSameEnds(start, end)
	// A range's ends are rounded from the double's own decimal with Node's
	// quirks too. V8 hands ICU a double for a range as for format, but
	// ICU's range formatter reads each end through a Formattable, whose
	// populateDecimalQuantity corrects the fast reading of a double
	// (roundToInfinity), where formatDouble keeps it: ApproximateIncrement
	// is never made of a range's ends.
	start.fromFloat, end.fromFloat = false, false
	a, b := f.layers(start, false), f.layers(end, false)
	innerSame := sameParts(a.inner, b.inner)
	middleSame := sameParts(a.pre, b.pre) && sameParts(a.post, b.post)
	outerSame := a.outer == b.outer

	// NumberRangeFormatterImpl::format: ends written alike are one number,
	// marked approximate, written from the start as given.
	if innerSame && middleSame && outerSame && (a.rounded == b.rounded || sameBefore) {
		return f.rangeParts(f.assemble(f.layers(start, true))), nil
	}

	// formatRange: which layers are written once. The middle one is shared
	// only when it is more than one character, ICU's heuristic since 63;
	// the exponent never is.
	collapseOuter := outerSame
	middleRunes := runeCount(a.pre) + runeCount(a.post)
	collapseMiddle := collapseOuter && middleSame && middleRunes > 1
	repeated := runeCount(a.inner) > 0 || !collapseMiddle && middleRunes > 0 || !collapseOuter && a.outer

	pattern := f.data.RangePattern
	if pattern == "" {
		pattern = "{0}–{1}"
	}
	var prefix, infix, suffix strings.Builder
	arg := -1
	glueSegments(pattern, func(i int) { arg = i }, func(text string) {
		switch arg {
		case -1:
			prefix.WriteString(text)
		case 0:
			infix.WriteString(text)
		default:
			suffix.WriteString(text)
		}
	})
	middle := infix.String()
	if repeated {
		// Spaces round the dash, where it has none of its own.
		if r, _ := utf8.DecodeRuneInString(middle); !isPatternWhiteSpace(r) {
			middle = " " + middle
		}
		if r, _ := utf8.DecodeLastRuneInString(middle); !isPatternWhiteSpace(r) {
			middle += " "
		}
	}

	end1, end2 := f.rangeEnd(a, collapseMiddle, collapseOuter), f.rangeEnd(b, collapseMiddle, collapseOuter)
	var out []Part
	literal := func(s string) {
		if s != "" {
			out = append(out, Part{PartLiteral, s})
		}
	}
	literal(prefix.String())
	out = append(out, end1...)
	literal(middle)
	out = append(out, end2...)
	literal(suffix.String())

	// The ends' spans, as ICU records them: in UTF-16 units, from the lengths
	// of what it wrote, and counting a shared prefix by its pattern's length
	// -- not the currency spacing applied with it, which therefore shifts the
	// spans a character off what they cover. V8 reports what the spans say.
	start1 := utf16Len(prefix.String())
	len1 := partsLen(end1)
	start2 := start1 + len1 + utf16Len(middle)
	len2 := partsLen(end2)
	if collapseMiddle {
		start1 += partsLen(a.pre)
		start2 += partsLen(a.pre)
		whole := append(append([]Part(nil), a.pre...), out...)
		out = append(whole, a.post...)
		if !a.outer {
			out = f.spaceCurrency(out)
		}
	}
	if collapseOuter && a.outer {
		count := string(PluralOther)
		if f.plurals != nil {
			count = f.plurals.resolveRange(a.count, b.count)
		}
		const hole PartKind = "\x00range"
		var wrapped []Part
		for _, p := range f.wrapOuter([]Part{{hole, ""}}, count) {
			if p.Kind == hole {
				start1 += partsLen(wrapped)
				start2 += partsLen(wrapped)
				wrapped = append(wrapped, out...)
				continue
			}
			wrapped = append(wrapped, p)
		}
		out = wrapped
	}
	spans := [2][2]int{{start1, start1 + len1}, {start2, start2 + len2}}
	return sourced(mergeParts(trimParts(f.props, out)), spans), nil
}

// sourced marks each part with the span that holds it, as V8 does: a part
// belongs to an end only if all of it lies within that end's span.
func sourced(parts []Part, spans [2][2]int) []RangePart {
	out := make([]RangePart, len(parts))
	at := 0
	for i, p := range parts {
		end := at + utf16Len(p.Value)
		source := SourceShared
		switch {
		case spans[0][0] <= at && end <= spans[0][1]:
			source = SourceStartRange
		case spans[1][0] <= at && end <= spans[1][1]:
			source = SourceEndRange
		}
		out[i] = RangePart{p.Kind, p.Value, source}
		at = end
	}
	return out
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func partsLen(parts []Part) int {
	n := 0
	for _, p := range parts {
		n += utf16Len(p.Value)
	}
	return n
}

// rangeEnd is one end of a range with the layers it does not share.
func (f *NumberFormat) rangeEnd(l numberLayers, collapseMiddle, collapseOuter bool) []Part {
	parts := append(append([]Part(nil), l.body...), l.inner...)
	if !collapseMiddle {
		parts = append(append(append([]Part(nil), l.pre...), parts...), l.post...)
		if !l.outer {
			parts = f.spaceCurrency(parts)
		}
	}
	if !collapseOuter && l.outer {
		parts = f.wrapOuter(parts, l.count)
	}
	return parts
}

// rangeParts marks every part of a single number as shared.
func (f *NumberFormat) rangeParts(parts []Part) []RangePart {
	return tag(mergeParts(trimParts(f.props, parts)), SourceShared)
}

// trimParts is FormattedValueStringBuilderImpl's trimming: a field's
// leading and trailing ignorables -- spaces, bidirectional marks -- are not
// part of it, and V8 writes them as literals. Arabic's minus sign is a
// left-to-right mark and a hyphen, of which only the hyphen is the sign. The
// grouping separator is left whole.
func trimParts(props *unicodeProps, parts []Part) []Part {
	var out []Part
	for _, p := range parts {
		if p.Kind == PartLiteral || p.Kind == PartGroup {
			out = append(out, p)
			continue
		}
		core := strings.TrimLeftFunc(p.Value, props.ignorable)
		trimmed := strings.TrimRightFunc(core, props.ignorable)
		if trimmed == "" {
			out = append(out, Part{PartLiteral, p.Value})
			continue
		}
		if lead := p.Value[:len(p.Value)-len(core)]; lead != "" {
			out = append(out, Part{PartLiteral, lead})
		}
		out = append(out, Part{p.Kind, trimmed})
		if trail := core[len(trimmed):]; trail != "" {
			out = append(out, Part{PartLiteral, trail})
		}
	}
	return out
}

// mergeParts joins neighbouring literals, as V8 writes the text between two
// fields as one literal.
func mergeParts(parts []Part) []Part {
	var out []Part
	for _, p := range parts {
		if p.Value == "" {
			continue
		}
		if n := len(out); n > 0 && p.Kind == PartLiteral && out[n-1].Kind == PartLiteral {
			out[n-1].Value += p.Value
			continue
		}
		out = append(out, p)
	}
	return out
}

func tag(parts []Part, source RangeSource) []RangePart {
	out := make([]RangePart, len(parts))
	for i, p := range parts {
		out[i] = RangePart{p.Kind, p.Value, source}
	}
	return out
}

func sameParts(a, b []Part) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func runeCount(parts []Part) int {
	n := 0
	for _, p := range parts {
		n += utf8.RuneCountInString(p.Value)
	}
	return n
}

// isPatternWhiteSpace is Unicode's Pattern_White_Space.
func isPatternWhiteSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x85, 0x200e, 0x200f, 0x2028, 0x2029:
		return true
	}
	return false
}

// resolveRange is StandardPluralRanges::resolve: the category of a range
// from its ends'.
func (p *PluralRules) resolveRange(first, second string) string {
	for _, r := range p.ranges {
		if r.Start == first && r.End == second {
			return r.Result
		}
	}
	return string(PluralOther)
}
