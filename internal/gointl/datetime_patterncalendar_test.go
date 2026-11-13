//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// uz-AF and uz-Arab-AF reckon in the Persian calendar. CLDR writes their
// dates with its patterns, "AP ۱۳۴۸-۱۰-۱۱"; ICU's pattern generator and
// interval formatter read the Gregorian ones unless a calendar is asked
// for, and Node writes "۱۳۴۸-۱۰-۱۱" (PatternCalendar). A style is not the
// generator's and keeps the era on both sides. Node's answers, and its
// answers with -u-ca-persian for the standard.
func TestPatternCalendar(t *testing.T) {
	from, to := time.UnixMilli(0), time.UnixMilli(86400000*45)
	for _, c := range []struct {
		tag       string
		opts      intl.DateTimeFormatOptions
		node, std [2]string
	}{
		{"uz-AF", intl.DateTimeFormatOptions{},
			[2]string{"۱۳۴۸-۱۰-۱۱", "۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"}},
		{"uz-Arab-AF", intl.DateTimeFormatOptions{Year: intl.WidthNumeric, Month: intl.WidthLong},
			[2]string{"۱۳۴۸ Dey", "۱۳۴۸ Dey–Bahman"},
			[2]string{"AP ۱۳۴۸ Dey", "AP ۱۳۴۸ Dey–Bahman"}},
		{"uz-Arab-AF-u-nu-latn", intl.DateTimeFormatOptions{},
			[2]string{"1348-10-11", "1348-10-11 – 1348-11-26"},
			[2]string{"AP 1348-10-11", "AP 1348-10-11 – 1348-11-26"}},
		{"uz-AF", intl.DateTimeFormatOptions{Calendar: "persian"},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"}},
		{"uz-Arab-AF-u-ca-persian", intl.DateTimeFormatOptions{},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"}},
		{"uz-AF", intl.DateTimeFormatOptions{DateStyle: intl.LengthShort},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"}},
		{"uz-Arab", intl.DateTimeFormatOptions{},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"},
			[2]string{"AP ۱۳۴۸-۱۰-۱۱", "AP ۱۳۴۸-۱۰-۱۱ – ۱۳۴۸-۱۱-۲۶"}},
	} {
		for _, side := range []struct {
			compat intl.Compat
			want   [2]string
		}{{intl.PatternCalendar, c.node}, {intl.Standard, c.std}} {
			o := c.opts
			o.TimeZone = "UTC"
			o.Compat = side.compat
			f := newDateTime(t, c.tag, o)
			if got := f.Format(from); got != side.want[0] {
				t.Errorf("%s %v: %q, want %q", c.tag, side.compat, got, side.want[0])
			}
			if got := f.FormatRange(from, to); got != side.want[1] {
				t.Errorf("%s %v range: %q, want %q", c.tag, side.compat, got, side.want[1])
			}
		}
	}
}
