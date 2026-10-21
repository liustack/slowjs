package intl

import (
	"math"
	"strconv"
	"strings"
)

// Scientific and engineering notation: 1234.5 as "1.235E3", or "1.2345E3" as
// engineering would write 123456789012, "123.457E9".
//
// The two differ only in which exponents are allowed. Scientific puts exactly
// one digit before the point, so the exponent is whatever it has to be.
// Engineering restricts the exponent to multiples of three, which is how
// magnitudes line up with the words for them -- thousand, million, billion --
// so the mantissa has one, two or three digits before the point instead.

// exponentFor returns the power of ten a number is written against, and the
// mantissa that goes with it.
func (f *NumberFormat) exponentFor(magnitude mag) int {
	if magnitude.isZero() {
		return 0
	}
	return f.exponentAt(magnitude.exponent())
}

// exponentAt is the power of ten a number whose first digit stands at the
// power e is written against: e itself, or in engineering notation the
// multiple of three at or below it, a floor and not a truncation, so -4 goes
// to -6 and not to -3.
func (f *NumberFormat) exponentAt(e int) int {
	if f.opts.Notation == NotationEngineering {
		e = int(math.Floor(float64(e)/3)) * 3
	}
	return e
}

// scientificPieces writes a number against a power of ten: the mantissa,
// and the exponent after it.
func (f *NumberFormat) scientificPieces(magnitude mag, negative bool) (body, inner []Part) {
	exponent, integer, fraction := f.scientificDigits(magnitude, negative)

	body = f.groupedInteger(integer)
	if fraction != "" {
		body = append(body, Part{PartDecimal, f.decimalSep}, Part{PartFraction, f.digits(fraction)})
	}

	inner = []Part{{PartExponentSeparator, f.data.Symbols.Exponential}}
	if exponent < 0 {
		inner = append(inner, Part{PartExponentMinusSign, f.data.Symbols.MinusSign})
		exponent = -exponent
	}
	inner = append(inner, Part{PartExponentInteger, f.digits(itoa(exponent))})
	return body, inner
}

// scientificDigits is the exponent a number is written against and the
// rounded digits of its mantissa.
func (f *NumberFormat) scientificDigits(magnitude mag, negative bool) (int, string, string) {
	exponent := f.exponentFor(magnitude)
	integer, fraction := f.round(magnitude.shift(-exponent), negative)
	// RoundingImpl::chooseMultiplierAndApply: rounding can carry the
	// mantissa to the next power, 9.9995 to 10.000. The number is then
	// written against the exponent of one power higher, rounded again from
	// the start where that exponent is another: one power up in scientific
	// notation, but in engineering only where the next power begins another
	// group of three, so 999999.5 is 1E6 and 99999.5 stays 100E3.
	if !magnitude.isZero() {
		if trimmed := strings.TrimLeft(integer, "0"); trimmed != "" && exponent+len(trimmed)-1 > magnitude.exponent() {
			if next := f.exponentAt(magnitude.exponent() + 1); next != exponent {
				exponent = next
				integer, fraction = f.round(magnitude.shift(-exponent), negative)
			}
		}
	}
	return exponent, padInteger(integer, f.minInt), fraction
}

// shiftDigits moves the decimal point of written digits by a power of ten:
// ICU's plural operands for "1.5M" are those of 1500000, with the exponent
// beside them, so that "1.5M" has no visible fraction.
func shiftDigits(integer, fraction string, exponent int) (string, string) {
	digits := integer + fraction
	point := len(integer) + exponent
	switch {
	case point <= 0:
		integer, fraction = "0", strings.Repeat("0", -point)+digits
	case point >= len(digits):
		integer, fraction = digits+strings.Repeat("0", point-len(digits)), ""
	default:
		integer, fraction = digits[:point], digits[point:]
	}
	if trimmed := strings.TrimLeft(integer, "0"); trimmed != "" {
		integer = trimmed
	} else {
		integer = "0"
	}
	return integer, fraction
}

// pluralOperands are what plural rules see of a number as this formatter
// writes it: the digits after rounding, and in compact or scientific
// notation the power of ten written apart, which French and others count.
func (f *NumberFormat) pluralOperands(v float64) operands {
	return f.operandsOf(magOf(v), math.Signbit(v))
}

// operandsOf are what plural rules see of a number's magnitude and sign as
// this formatter writes them, as pluralOperands says.
func (f *NumberFormat) operandsOf(magnitude mag, negative bool) operands {
	switch f.opts.Notation {
	case NotationCompact:
		form := f.compactForm(magnitude, negative)
		integer, fraction := f.round(magnitude.shift(-form.exponent), negative)
		integer, fraction = shiftDigits(integer, fraction, form.exponent)
		return operandsFor(integer, fraction, form.exponent)
	case NotationScientific, NotationEngineering:
		exponent, integer, fraction := f.scientificDigits(magnitude, negative)
		integer, fraction = shiftDigits(integer, fraction, exponent)
		return operandsFor(integer, fraction, exponent)
	}
	integer, fraction := f.round(magnitude, negative)
	return operandsFor(padInteger(integer, f.minInt), fraction, 0)
}

// itoa writes a non-negative number in ASCII digits: an exponent, which a
// number given as a string can make as long as nine of them.
func itoa(v int) string { return strconv.Itoa(v) }
