// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import "strings"

// A number given as a string can ask for a long output however short it is:
// "1e-1073741000" to three significant digits is a thousand million zeros.
// Output tells a caller how many digits f writes d with before f writes
// it, so that the caller can refuse a number whose written form is too
// long. Only zeros after the point can be that many: a string past the
// largest float is an infinity, and a BigInt is written with its digits.

// Written is how many characters of digits f writes d with before it is
// rounded, the zeros it writes out included. Scientific and engineering
// notation write none of the zeros, only the exponent they make, and a
// number rounded to its decimals at most one more of them than those, as
// does one under lessPrecision whose first digit is past its decimals
// (digitPlan.round).
func (f *NumberFormat) Written(d Decimal) int64 {
	zeros := int64(d.m.zeros)
	switch {
	case f.opts.Notation == NotationScientific || f.opts.Notation == NotationEngineering:
		zeros = 0
	case f.decimalsDecide(d) && f.opts.Notation != NotationCompact:
		zeros = min(zeros, int64(f.maxFrac)+1)
	}
	return int64(len(d.m.integer)) + int64(len(d.m.fraction)) + zeros + int64(f.opts.MinimumIntegerDigits)
}

// Output is a bound on how many characters of digits f writes d with once
// it is rounded: Written, but for the digits of d past those the rounding
// keeps, which f reads and does not write. The rounding keeps at most
// maxFrac decimals or maxSig significant digits after the zeros that lead
// the fraction, or the more of the two under a priority, then pads with
// zeros to minFrac or minSig at most, and a carry writes one more digit.
// A percent moves two digits of the fraction into the integer, and an
// increment rounds to a multiple of it, which may write its digits where
// d has fewer. Scientific and engineering notation write at most three
// integer digits and an exponent of at most expDigits digits.
func (f *NumberFormat) Output(d Decimal) int64 {
	if d.kind != decimalFinite {
		return 0
	}
	integer, fraction := int64(len(d.m.integer)), int64(len(d.m.fraction))
	var keep int64
	switch {
	case f.decimalsDecide(d):
		keep = int64(f.maxFrac)
	case f.rounding == roundSignificantDigits:
		keep = int64(f.maxSig)
	default:
		keep = int64(max(f.maxFrac, f.maxSig))
	}
	pad := int64(max(f.minFrac, f.minSig)) + 1
	if f.opts.Style == StylePercent {
		pad += 2
	}
	if f.opts.RoundingIncrement > 1 {
		pad += incrementDigits
	}
	added := f.Written(d) - integer - fraction
	if f.opts.Notation == NotationScientific || f.opts.Notation == NotationEngineering {
		return added + 3 + min(integer+fraction, keep) + pad + expDigits
	}
	lead := fraction - int64(len(strings.TrimLeft(d.m.fraction, "0")))
	return added + integer + min(fraction, lead+keep) + pad
}

// decimalsDecide reports whether f rounds d to its decimals alone, and so
// writes no more of them than those.
func (f *NumberFormat) decimalsDecide(d Decimal) bool {
	return f.rounding == roundFractionDigits || f.rounding == roundLessPrecision && d.m.zeros > f.maxFrac
}

// expDigits is the most digits of the exponent of a number:
// maxDecimalExponent has fewer.
const expDigits = 11

// incrementDigits is the most digits of a rounding increment: ECMA-402
// takes 5000 at most.
const incrementDigits = 4
