package intl

import (
	"errors"
	"fmt"
	"strings"
)

// ErrOption reports an option ECMA-402 refuses, out of its range or not one
// of its values, where JavaScript throws a RangeError.
var ErrOption = errors.New("intl: an option is out of range")

// named reports whether an option's value is one of its type's, which its
// String writes as a name rather than as "Style(9)".
func named(v fmt.Stringer) bool { return !strings.Contains(v.String(), "(") }

// isRoundingIncrement reports whether ECMA-402 allows an increment, 1
// being the default.
func isRoundingIncrement(n int) bool {
	switch n {
	case 1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000:
		return true
	}
	return false
}

// digitOptions are the options SetNumberFormatDigitOptions reads, which
// NumberFormat and PluralRules share.
type digitOptions struct {
	minInt                           int
	minFrac, maxFrac, minSig, maxSig *int
	increment                        int
	mode                             RoundingMode
	priority                         RoundingPriority
	trailing                         TrailingZeroDisplay
}

// check is SetNumberFormatDigitOptions's ranges: at least one and at most
// twenty-one integer digits (zero being the default), at most a hundred
// decimals, one to twenty-one significant digits, an increment from its
// list, and a named rounding mode, priority and trailing zero display.
func (d digitOptions) check() error {
	if d.minInt < 0 || d.minInt > 21 {
		return fmt.Errorf("%w: %d integer digits, not 1 to 21", ErrOption, d.minInt)
	}
	for _, n := range []*int{d.minFrac, d.maxFrac} {
		if n != nil && (*n < 0 || *n > 100) {
			return fmt.Errorf("%w: %d fraction digits, not 0 to 100", ErrOption, *n)
		}
	}
	for _, n := range []*int{d.minSig, d.maxSig} {
		if n != nil && (*n < 1 || *n > 21) {
			return fmt.Errorf("%w: %d significant digits, not 1 to 21", ErrOption, *n)
		}
	}
	if d.increment != 0 && !isRoundingIncrement(d.increment) {
		return fmt.Errorf("%w: %d is not a rounding increment", ErrOption, d.increment)
	}
	for _, v := range []fmt.Stringer{d.mode, d.priority, d.trailing} {
		if !named(v) {
			return fmt.Errorf("%w: %s", ErrOption, v)
		}
	}
	return nil
}

// isWellFormedCurrencyCode is ECMA-402's IsWellFormedCurrencyCode: three
// ASCII letters, in any case.
func isWellFormedCurrencyCode(code string) bool {
	return len(code) == 3 && isLetters(code)
}
