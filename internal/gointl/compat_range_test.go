//go:build gointl_all

package intl_test

import (
	"math"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A range whose two ends are the same double but not the same number:
// ECMA-402 writes both, and ICU, which holds a decimal past an int64 as the
// double nearest it as well and asks whether the ends were equal before
// rounding by those doubles, one marked approximate. An integer that fits in
// an int64 is compared as one, and never equals a double; ends whose signs,
// exponents or affixes differ are a range on both sides. The ends are made
// as go-quickjs hands them over: a Number by DecimalFromFloat, a numeric
// string by ParseDecimal and a BigInt by ParseExactDecimal. Node v26.10's
// are the answers.
func TestDoubleRangeIdentity(t *testing.T) {
	en, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	n, s, b := intl.DecimalFromFloat, intl.ParseDecimal, intl.ParseExactDecimal
	opts := func(set func(*intl.NumberFormatOptions)) intl.NumberFormatOptions {
		o := intl.NumberFormatOptions{MaximumFractionDigits: intl.Digits(20)}
		if set != nil {
			set(&o)
		}
		return o
	}
	plain := intl.NumberFormatOptions{}
	unit := opts(func(o *intl.NumberFormatOptions) {
		o.Style, o.Unit, o.UnitDisplay = intl.StyleUnit, "meter", intl.UnitLong
	})
	sci := opts(func(o *intl.NumberFormatOptions) { o.Notation = intl.NotationScientific })
	compact := opts(func(o *intl.NumberFormatOptions) { o.Notation = intl.NotationCompact })
	currency := opts(func(o *intl.NumberFormatOptions) { o.Style, o.Currency = intl.StyleCurrency, "USD" })
	always := opts(func(o *intl.NumberFormatOptions) { o.SignDisplay = intl.SignAlways })
	tenTo30 := "1" + strings.Repeat("0", 30)
	for _, c := range []struct {
		opts           intl.NumberFormatOptions
		start, end     intl.Decimal
		standard, node string
	}{
		{opts(nil), s("0.1"), s("0.10000000000000000001"), "0.1–0.10000000000000000001", "~0.1"},
		{opts(nil), s("0.10000000000000000001"), s("0.1"), "0.10000000000000000001–0.1", "~0.10000000000000000001"},
		{opts(nil), n(0.1), s("0.10000000000000000001"), "0.1–0.10000000000000000001", "~0.1"},
		{opts(nil), s("-0.1"), s("-0.10000000000000000001"), "-0.1 – -0.10000000000000000001", "~-0.1"},
		{always, s("0.1"), s("0.10000000000000000001"), "+0.1 – +0.10000000000000000001", "~+0.1"},
		{currency, s("0.1"), s("0.10000000000000000001"), "$0.10 – $0.10000000000000000001", "~$0.10"},
		{unit, n(1), s("1.0000000000000000001"), "1–1.0000000000000000001 meters", "~1 meter"},
		{unit, s("1.0000000000000000001"), n(1), "1.0000000000000000001–1 meters", "~1.0000000000000000001 meters"},
		{plain, b(tenTo30), b(tenTo30[:30] + "1"),
			"1,000,000,000,000,000,000,000,000,000,000–1,000,000,000,000,000,000,000,000,000,001",
			"~1,000,000,000,000,000,000,000,000,000,000"},
		{plain, b("9223372036854775808"), b("9223372036854775809"),
			"9,223,372,036,854,775,808–9,223,372,036,854,775,809", "~9,223,372,036,854,775,808"},
		{plain, b("-9223372036854775809"), b("-9223372036854775810"),
			"-9,223,372,036,854,775,809 – -9,223,372,036,854,775,810", "~-9,223,372,036,854,775,809"},
		// Integers that fit in an int64 are compared as integers, and one
		// that fits is never the same as one that does not, nor a double.
		{plain, b("9223372036854775807"), b("9223372036854775806"),
			"9,223,372,036,854,775,807–9,223,372,036,854,775,806", "9,223,372,036,854,775,807–9,223,372,036,854,775,806"},
		{plain, b("-9223372036854775808"), b("-9223372036854775809"),
			"-9,223,372,036,854,775,808 – -9,223,372,036,854,775,809", "-9,223,372,036,854,775,808 – -9,223,372,036,854,775,809"},
		{plain, n(9007199254740992), b("9007199254740993"),
			"9,007,199,254,740,992–9,007,199,254,740,993", "9,007,199,254,740,992–9,007,199,254,740,993"},
		// Ends whose sign, exponent or compact suffix differ are a range.
		{plain, n(math.Copysign(0, -1)), n(0), "-0 – 0", "-0 – 0"},
		{sci, s("9.99999999999999999999"), n(10), "9.99999999999999999999E0 – 1E1", "9.99999999999999999999E0 – 1E1"},
		{compact, s("999.99999999999999999"), n(1000), "999.99999999999999999–1K", "999.99999999999999999–1K"},
	} {
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.DoubleRangeIdentity, c.node}, {intl.NodeICU, c.node}} {
			o := c.opts
			o.Compat = side.compat
			f := newNumber(t, en, o)
			if got, err := f.FormatDecimalRange(c.start, c.end); err != nil || got != side.want {
				t.Errorf("%v %v–%v: %q, %v, want %q", side.compat, c.start, c.end, got, err, side.want)
			}
		}
	}
}
