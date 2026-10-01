// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "modernc.org/quickjs/intl"

import (
	"math"
	"strconv"
	"strings"
)

// dec is a decimal number in sign-magnitude form. Its value is
//
//	(-1)^neg × 0.<digits> × 10^exp
//
// where digits carries neither a leading nor a trailing zero. Zero is the
// empty digits with exp 0.
//
// Keeping the mantissa as decimal digits rather than as a float64 is what
// makes ECMA-402 rounding reproducible. The spec rounds the "mathematical
// value" half away from zero, which implementations approximate by rounding
// the shortest decimal representation of the double - the same string
// strconv.FormatFloat(v, 'e', -1, 64) produces. Rounding a binary
// approximation instead visibly disagrees: 1.005 rounded to two fraction
// digits is 1.01 here and in a browser, but 1.00 if the exact binary value
// (1.00499999999999989...) is rounded.
type dec struct {
	digits []byte
	exp    int
	neg    bool
}

// decFromFloat returns f as a dec. f must be finite.
func decFromFloat(f float64) dec {
	neg := math.Signbit(f)
	if f == 0 {
		return dec{neg: neg}
	}

	s := strconv.FormatFloat(math.Abs(f), 'e', -1, 64)
	i := strings.IndexByte(s, 'e')
	if i < 0 { // Cannot happen for the 'e' format.
		return dec{neg: neg}
	}

	exp, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return dec{neg: neg}
	}

	mantissa := s[:i]
	digits := make([]byte, 0, len(mantissa))
	for j := 0; j < len(mantissa); j++ {
		if c := mantissa[j]; c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}
	// The mantissa is d.ddd, ie. 0.dddd × 10^(exp+1).
	return normDec(digits, exp+1, neg)
}

// decFromInt returns n as a dec. Integer arguments reach the formatters as Go
// ints whenever QuickJS tagged them as such, so they never take the float
// detour and format exactly.
func decFromInt(n int64) dec {
	if n == 0 {
		return dec{}
	}

	neg := n < 0
	s := strconv.FormatInt(n, 10)
	if neg {
		s = s[1:]
	}
	return normDec([]byte(s), len(s), neg)
}

// decFromDecimalString parses a decimal literal: an optional sign, digits, an
// optional fraction and an optional exponent. The empty string is zero, the
// value javascript's Number("") has.
func decFromDecimalString(s string) (dec, bool) {
	if s == "" {
		return dec{}, true
	}

	var neg bool
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg, s = true, s[1:]
	}
	exp := 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil {
			return dec{}, false
		}

		s, exp = s[:i], n
	}
	ip, fp, _ := strings.Cut(s, ".")
	if (ip == "" && fp == "") || !isDigit(ip) || !isDigit(fp) {
		return dec{}, false
	}

	digits := ip + fp
	exp += len(ip)
	i := 0
	for ; i < len(digits) && digits[i] == '0'; i++ {
		exp--
	}
	return normDec([]byte(digits[i:]), exp, neg), true
}

// normDec trims the trailing zeros of digits and returns the resulting dec.
func normDec(digits []byte, exp int, neg bool) dec {
	for len(digits) != 0 && digits[len(digits)-1] == '0' {
		digits = digits[:len(digits)-1]
	}
	if len(digits) == 0 {
		return dec{neg: neg}
	}

	return dec{digits: digits, exp: exp, neg: neg}
}

// isZero reports whether d is zero.
func (d dec) isZero() bool { return len(d.digits) == 0 }

// scale returns d × 10^n.
func (d dec) scale(n int) dec {
	if d.isZero() {
		return d
	}

	d.exp += n
	return d
}

// magnitude returns the decimal exponent of d's most significant digit, ie. 0
// for [1, 10), 1 for [10, 100) and -1 for [0.1, 1). The result for zero is 0.
func (d dec) magnitude() int {
	if d.isZero() {
		return 0
	}

	return d.exp - 1
}

// roundUp reports whether dropping every digit from k on rounds the magnitude
// up, under one of ECMA-402's nine rounding modes. The digits carry no trailing
// zero, so a lone '5' at k is an exact tie and anything after it is not.
func (d dec) roundUp(k int, mode string) bool {
	if k < 0 {
		// Every digit is dropped, so what is left is below half of the last
		// kept place and only the modes that always round the magnitude up
		// have anything to say.
		switch mode {
		case "expand":
			return true
		case "ceil":
			return !d.neg
		case "floor":
			return d.neg
		}
		return false
	}

	first := d.digits[k]
	tie := first == '5' && k == len(d.digits)-1
	switch mode {
	case "ceil":
		return !d.neg
	case "floor":
		return d.neg
	case "expand":
		return true
	case "trunc":
		return false
	case "halfCeil":
		return first > '5' || first == '5' && (!tie || !d.neg)
	case "halfFloor":
		return first > '5' || first == '5' && (!tie || d.neg)
	case "halfTrunc":
		return first > '5' || first == '5' && !tie
	case "halfEven":
		if first == '5' && tie {
			return k > 0 && (d.digits[k-1]-'0')%2 == 1
		}

		return first >= '5'
	}
	return first >= '5' // halfExpand
}

// roundTo returns d rounded to k significant digits. The default mode is
// ECMA-402's "halfExpand": ties round away from zero.
func (d dec) roundTo(k int, mode string) dec {
	if d.isZero() || k >= len(d.digits) {
		return d
	}

	if k < 0 {
		if !d.roundUp(k, mode) {
			return dec{neg: d.neg}
		}

		// One unit in the place the last kept digit would have had.
		return dec{digits: []byte{'1'}, exp: d.exp - k + 1, neg: d.neg}
	}

	digits := make([]byte, k, k+1)
	copy(digits, d.digits[:k])
	exp := d.exp
	if d.roundUp(k, mode) {
		i := k - 1
		for ; i >= 0; i-- {
			if digits[i] != '9' {
				digits[i]++
				break
			}

			digits[i] = '0'
		}
		if i < 0 { // 999... carried out into a new leading digit.
			digits = append([]byte{'1'}, digits...)
			exp++
		}
	}
	return normDec(digits, exp, d.neg)
}

// roundFrac returns d rounded to at most maxFrac digits after the decimal
// point.
func (d dec) roundFrac(maxFrac int, mode string) dec { return d.roundTo(d.exp+maxFrac, mode) }

// hasFraction reports whether d needs any digit after the decimal point.
func (d dec) hasFraction() bool { return !d.isZero() && d.exp < len(d.digits) }

// intDigits returns the number of digits d renders before the decimal point.
func (d dec) intDigits() int {
	if d.exp < 1 {
		return 1
	}

	return d.exp
}

// render returns d's integer and fraction digits, the integer part padded to
// at least minInt digits and the fraction part to at least minFrac digits.
// Neither carries a sign.
func (d dec) render(minInt, minFrac int) (ip, fp string) {
	switch {
	case d.exp <= 0:
		ip = "0"
	case d.exp >= len(d.digits):
		ip = string(d.digits) + strings.Repeat("0", d.exp-len(d.digits))
	default:
		ip = string(d.digits[:d.exp])
	}
	if n := minInt - len(ip); n > 0 {
		ip = strings.Repeat("0", n) + ip
	}

	switch {
	case d.isZero():
		// nop
	case d.exp < 0:
		fp = strings.Repeat("0", -d.exp) + string(d.digits)
	case d.exp < len(d.digits):
		fp = string(d.digits[d.exp:])
	}
	if n := minFrac - len(fp); n > 0 {
		fp += strings.Repeat("0", n)
	}
	return ip, fp
}
