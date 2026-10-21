package intl

import (
	"math"
	"strconv"
	"strings"
)

// A mag is the magnitude of a number as its decimal digits: the integer part
// with no leading zeros ("0" for none) and the fraction with no trailing
// ones. Everything a number formatter does to a number -- rounding it,
// dividing it into a compact or scientific form, making it a percent -- is
// done on the digits, so that a number given exactly, as a string or a
// BigInt, is written exactly, however many digits it has.
//
// A number far below one -- "1e-999999999", given as a string -- has more
// zeros after its point than are worth writing out, and most formats round
// them all away. Those zeros are counted rather than written: the fraction
// is zeros of them followed by fraction's digits, and they are written out
// only as far as a format keeps them.
type mag struct {
	integer, fraction string
	// zeros is the zeros between the point and fraction, which only a
	// number below one has.
	zeros int
}

// longZeros is how many zeros after the point are held as a count rather
// than written: more than any number given as a float has.
const longZeros = 400

// maxDecimalExponent bounds the power of ten a Decimal is held to. A number
// beyond it is an infinity or zero, as far as any formatter could write it:
// more digits than there is memory to write them.
const maxDecimalExponent = 1 << 30

// magOf is the digits of a float's magnitude: the shortest that read back as
// the same float, which is what ICU works from; see decimal.go.
func magOf(v float64) mag {
	// Abs rather than a comparison with zero, which negative zero passes
	// with its minus sign still on it.
	v = math.Abs(v)
	integer, fraction := splitFloat(v)
	return makeMag(integer, fraction)
}

// makeMag normalizes digits into a mag.
func makeMag(integer, fraction string) mag {
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	return mag{integer: integer, fraction: strings.TrimRight(fraction, "0")}
}

// isZero reports whether the magnitude is zero. The zero value of a mag, as
// in the zero value of a Decimal, is zero too.
func (m mag) isZero() bool { return (m.integer == "0" || m.integer == "") && m.fraction == "" }

// exponent is the power of ten of the first significant digit: the floor of
// the base-ten logarithm. Zero has none, and answers zero.
func (m mag) exponent() int {
	if m.integer != "0" && m.integer != "" {
		return len(m.integer) - 1
	}
	lead := len(m.fraction) - len(strings.TrimLeft(m.fraction, "0"))
	if lead == len(m.fraction) {
		return 0
	}
	return -m.zeros - lead - 1
}

// shift multiplies the magnitude by a power of ten.
func (m mag) shift(by int) mag {
	if by == 0 || m.isZero() {
		return m
	}
	if m.zeros > 0 {
		if by <= m.zeros {
			// Only the counted zeros move.
			return mag{integer: "0", fraction: m.fraction, zeros: m.zeros - by}
		}
		return mag{integer: "0", fraction: m.fraction}.shift(by - m.zeros)
	}
	if point := len(m.integer) + by; point < -longZeros {
		// Every digit goes below the point, and further than is worth
		// writing: the zeros before them are counted.
		digits := m.integer + m.fraction
		trimmed := strings.TrimLeft(digits, "0")
		return mag{integer: "0", fraction: strings.TrimRight(trimmed, "0"),
			zeros: -point + len(digits) - len(trimmed)}
	}
	integer, fraction := shiftDigits(m.integer, m.fraction, by)
	return makeMag(integer, fraction)
}

// written is the fraction with its counted zeros written out, as far as
// limit of them: rounding at a place no further than limit decimals comes
// out the same with the rest left off, since what is dropped is below half
// of that place and not zero either way.
func (m mag) written(limit int) string {
	if m.zeros == 0 {
		return m.fraction
	}
	return strings.Repeat("0", min(m.zeros, limit)) + m.fraction
}

// float is the magnitude as a float, for the few decisions that only need
// its size.
func (m mag) float() float64 {
	if m.zeros > longZeros {
		// Far below the smallest float.
		return 0
	}
	s := m.integer
	if m.fraction != "" {
		s += "." + m.written(m.zeros)
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// decimalKind says whether a Decimal is a number, NaN or an infinity.
type decimalKind uint8

const (
	decimalFinite decimalKind = iota
	decimalNaN
	decimalInfinite
)

// A Decimal is a number held exactly, as ECMA-402's formatters take one: a
// float, or a string or BigInt carrying more digits than a float can. Its
// zero value is zero.
type Decimal struct {
	kind decimalKind
	neg  bool
	m    mag
	// float is the double a Decimal was made from, where it was, which
	// ApproximateIncrement reads as ICU does.
	float     float64
	fromFloat bool
}

// DecimalFromFloat is a float as a Decimal: the shortest decimal that reads
// back as the float, which is the one ICU formats.
func DecimalFromFloat(v float64) Decimal {
	switch {
	case math.IsNaN(v):
		return Decimal{kind: decimalNaN}
	case math.IsInf(v, 0):
		return Decimal{kind: decimalInfinite, neg: v < 0}
	}
	neg := math.Signbit(v)
	return Decimal{neg: neg, m: magOf(v), float: v, fromFloat: true}
}

// icuDoubleMag is a double's magnitude as ICU's
// DecimalQuantity::_setToDoubleFast reads it: an integer that fits is
// exact, and anything else is scaled by a power of ten in floating point
// to an integer of about sixteen digits and rounded, which is not always
// the double's own decimal past them.
func icuDoubleMag(v float64) mag {
	v = math.Abs(v)
	exponent := int((math.Float64bits(v)>>52)&0x7ff) - 0x3ff
	if v == 0 || exponent <= 52 && float64(int64(v)) == v || exponent == -1023 || exponent == 1024 {
		// Exact, or ICU's slow path, which is the shortest decimal.
		return magOf(v)
	}
	// 3.3219... is log2(10). Each step is rounded on its own, as ICU's is.
	places := int(float64(52-exponent) / 3.32192809488736234787031942948939017586)
	n := v
	if places >= 0 {
		i := places
		for ; i >= 22; i -= 22 {
			n = float64(n * 1e22)
		}
		n = float64(n * math.Pow10(i))
	} else {
		i := places
		for ; i <= -22; i += 22 {
			n = float64(n / 1e22)
		}
		n = float64(n / math.Pow10(-i))
	}
	digits := strconv.FormatInt(int64(math.Floor(float64(n+0.5))), 10)
	if digits == "0" {
		return zeroMag
	}
	return makeMag(digits, "").shift(-places)
}

// ParseDecimal reads a string as ECMA-402's ToIntlMathematicalValue does: as
// a JavaScript numeric string -- surrounding whitespace, a sign, digits, a
// point, an exponent; or "Infinity"; or hexadecimal, octal or binary digits
// without a sign -- but exactly, rather than rounded to a float. An empty
// string is zero, and anything else that is not a number is NaN. "-0" is
// negative zero. A number too large for a float is an infinity, as ECMA-402
// has it; one too small for a float is kept exactly, as V8 keeps it, where
// ECMA-402 would make it zero.
func ParseDecimal(s string) Decimal { return parseDecimal(s, true) }

// ParseExactDecimal reads a string as ParseDecimal does, but keeps a number
// too large for a float as it is written: ToIntlMathematicalValue takes a
// BigInt as the integer it is, where it rounds a string to a float's range
// first. A formatter writes a BigInt past 1.8e308 in full, as V8 does.
//
// Only digits written out are kept so: a string with an exponent is read as
// ParseDecimal reads it, so that a short string cannot stand for a number
// too long to write, "1e1000000000" a gigabyte of zeros. A BigInt's decimal
// string has no exponent.
func ParseExactDecimal(s string) Decimal { return parseDecimal(s, false) }

// parseDecimal is ParseDecimal, rounding a number too large for a float to
// an infinity when round is set or the string has an exponent.
func parseDecimal(s string, round bool) Decimal {
	s = strings.TrimFunc(s, isJSWhitespace)
	if s == "" {
		return Decimal{m: zeroMag}
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			integer, ok := baseToDecimal(s[2:], base)
			if !ok {
				return Decimal{kind: decimalNaN}
			}
			if _, err := strconv.ParseFloat(integer, 64); err != nil {
				return Decimal{kind: decimalInfinite}
			}
			return Decimal{m: makeMag(integer, "")}
		}
	}
	neg := false
	body := s
	switch body[0] {
	case '-':
		neg, body = true, body[1:]
	case '+':
		body = body[1:]
	}
	if body == "Infinity" {
		return Decimal{kind: decimalInfinite, neg: neg}
	}
	mantissa, exp := body, 0
	if i := strings.IndexAny(body, "eE"); i >= 0 {
		round = true
		mantissa = body[:i]
		e := body[i+1:]
		if e == "" || e == "+" || e == "-" {
			return Decimal{kind: decimalNaN}
		}
		n, err := strconv.Atoi(e)
		if err != nil {
			if !allDigits(strings.TrimLeft(e, "+-")) {
				return Decimal{kind: decimalNaN}
			}
			// An exponent too large for an int: the number is an infinity
			// or zero, as far as any formatter could write it.
			if strings.HasPrefix(e, "-") {
				return Decimal{neg: neg, m: zeroMag}
			}
			return Decimal{kind: decimalInfinite, neg: neg}
		}
		exp = n
	}
	// And one too large to write out, likewise; which also keeps the
	// arithmetic on it from overflowing.
	switch {
	case exp < -maxDecimalExponent:
		return Decimal{neg: neg, m: zeroMag}
	case exp > maxDecimalExponent:
		return Decimal{kind: decimalInfinite, neg: neg}
	}
	integer, fraction, _ := strings.Cut(mantissa, ".")
	if integer == "" && fraction == "" || !allDigits(integer) || !allDigits(fraction) ||
		strings.Count(mantissa, ".") > 1 {
		return Decimal{kind: decimalNaN}
	}
	m := makeMag(integer, fraction)
	if round && !m.isZero() {
		// RoundMVResult: what the number would be as a float decides
		// whether it is an infinity.
		v, _ := strconv.ParseFloat(integer+"."+fraction+"e"+strconv.Itoa(exp), 64)
		if math.IsInf(v, 0) {
			return Decimal{kind: decimalInfinite, neg: neg}
		}
	}
	return Decimal{neg: neg, m: m.shift(exp)}
}

// zeroMag is zero's digits.
var zeroMag = mag{integer: "0"}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// baseToDecimal converts digits in base 2, 8 or 16 to decimal digits.
func baseToDecimal(s string, base int) (string, bool) {
	if s == "" {
		return "", false
	}
	out := []byte{0} // little-endian decimal digits
	for i := 0; i < len(s); i++ {
		d, err := strconv.ParseUint(s[i:i+1], base, 8)
		if err != nil {
			return "", false
		}
		carry := int(d)
		for j := range out {
			v := int(out[j])*base + carry
			out[j], carry = byte(v%10), v/10
		}
		for carry > 0 {
			out = append(out, byte(carry%10))
			carry /= 10
		}
	}
	b := make([]byte, len(out))
	for i, d := range out {
		b[len(out)-1-i] = '0' + d
	}
	return string(b), true
}

// isJSWhitespace is JavaScript's WhiteSpace and LineTerminator.
func isJSWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// IsNaN reports whether the Decimal is not a number.
func (d Decimal) IsNaN() bool { return d.kind == decimalNaN }

// IsInf reports whether the Decimal is an infinity.
func (d Decimal) IsInf() bool { return d.kind == decimalInfinite }

// Negative reports whether the Decimal has a minus sign, negative zero
// included.
func (d Decimal) Negative() bool { return d.neg }

// isZero reports whether the number is zero, of either sign.
func (d Decimal) isZero() bool { return d.kind == decimalFinite && d.m.isZero() }

// abs is the number without its sign.
func (d Decimal) abs() Decimal {
	d.neg = false
	return d
}

// Float is the nearest float to the Decimal.
func (d Decimal) Float() float64 {
	switch d.kind {
	case decimalNaN:
		return math.NaN()
	case decimalInfinite:
		if d.neg {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	v := d.m.float()
	if d.neg {
		v = -v
	}
	return v
}

// String writes the Decimal plainly, "-0.001".
func (d Decimal) String() string {
	switch d.kind {
	case decimalNaN:
		return "NaN"
	case decimalInfinite:
		if d.neg {
			return "-Infinity"
		}
		return "Infinity"
	}
	s := d.m.integer
	if d.m.fraction != "" {
		s += "." + d.m.written(d.m.zeros)
	}
	if d.neg {
		s = "-" + s
	}
	return s
}
