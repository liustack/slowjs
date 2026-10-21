package intl

import (
	"strconv"
	"strings"
)

// Turning a number into digits, and laying those digits out in groups.
//
// Two decisions here are worth stating, because both are places where the
// obvious thing is wrong.
//
// The first is which decimal a float stands for. A float64 is a binary
// fraction and almost never exactly the decimal that was written, so 0.615 is
// really 0.614999999999999991118... Rounding the exact value gives 0.61;
// rounding the shortest decimal that reads back as the same float gives 0.62.
// ECMA-402 describes the first and ICU does the second, because ICU builds its
// digits from the shortest representation. This follows ICU, since ICU is what
// the corpus holds this to.
//
// The second is which way a half goes. Go's strconv rounds a tie to even, so
// 0.5 at no decimals is "0" and 1234.5 is "1234". ECMA-402 rounds a half away
// from zero, so they are "1" and "1235". The rounding here is written out
// rather than left to strconv for that reason.

// digitsOf renders a non-negative number as an integer part and a fraction
// part, rounded to at most maxFrac places with a half going away from zero.
func digitsOf(v float64, maxFrac int) (integer, fraction string) {
	// 'f' with a precision of -1 is the shortest form that reads back as the
	// same float, which is the one ICU works from.
	s := strconv.FormatFloat(v, 'f', -1, 64)
	integer, fraction, _ = strings.Cut(s, ".")
	if len(fraction) <= maxFrac {
		return integer, fraction
	}

	roundUp := fraction[maxFrac] >= '5'
	fraction = fraction[:maxFrac]
	if roundUp {
		integer, fraction = increment(integer, fraction)
	}
	return integer, fraction
}

// increment adds one to the last place of a number held as two digit strings.
func increment(integer, fraction string) (string, string) {
	f := []byte(fraction)
	for i := len(f) - 1; i >= 0; i-- {
		if f[i] < '9' {
			f[i]++
			return integer, string(f)
		}
		f[i] = '0'
	}
	n := []byte(integer)
	for i := len(n) - 1; i >= 0; i-- {
		if n[i] < '9' {
			n[i]++
			return string(n), string(f)
		}
		n[i] = '0'
	}
	// Every digit was a nine, so the number gained one: 999 became 1000.
	return "1" + string(n), string(f)
}

// trimTrailingZeros removes the decimals that are not asked for. A fraction is
// written to at most maxFrac places but only to minFrac if the rest are zero,
// which is why 1.50 is "1.5" and 1.00 is "1".
func trimTrailingZeros(fraction string, minFrac int) string {
	end := len(fraction)
	for end > minFrac && fraction[end-1] == '0' {
		end--
	}
	return fraction[:end]
}

// padInteger writes the leading zeros a minimum integer count asks for.
func padInteger(integer string, minInt int) string {
	if len(integer) >= minInt {
		return integer
	}
	return strings.Repeat("0", minInt-len(integer)) + integer
}

// groupPositions returns the offsets in an integer string where a separator
// goes, counted from the left, in increasing order.
//
// The groups are measured from the point outwards: the first holds primary
// digits and every one above it secondary, which is how 1234567 is 1,234,567
// in most places and 12,34,567 in India.
//
// minGrouping is how many digits have to stand before the first separator for
// there to be one at all. It is two in Spanish and Polish, where 1234 is
// written without a separator and 12345 with one.
func groupPositions(digits int, primary, secondary, minGrouping int) []int {
	if primary <= 0 || digits <= primary {
		return nil
	}
	if digits-primary < minGrouping {
		return nil
	}
	if secondary <= 0 {
		secondary = primary
	}
	var at []int
	for pos := digits - primary; pos > 0; pos -= secondary {
		at = append(at, pos)
	}
	// Built from the right, wanted from the left.
	for i, j := 0, len(at)-1; i < j; i, j = i+1, j-1 {
		at[i], at[j] = at[j], at[i]
	}
	return at
}

// mapDigits writes ASCII digits in another numbering system's digits. An empty
// table means the ASCII ones, which is most locales.
func mapDigits(s string, digits string) string {
	if digits == "" || s == "" {
		return s
	}
	table := []rune(digits)
	if len(table) != 10 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) * 2)
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= '0' && c <= '9' {
			b.WriteRune(table[c-'0'])
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}
