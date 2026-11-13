//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A compact number takes the pattern of its power of ten as rounded, even
// where that power divides by the same amount: Arabic writes 9999.9 as ten
// thousand, "10 ألف", where it had written ten thousands, "10 آلاف". ICU's
// CompactHandler; Node's answers (ISSUES.md NF-3).
func TestCompactCarry(t *testing.T) {
	long := intl.NumberFormatOptions{Notation: intl.NotationCompact, CompactDisplay: intl.CompactLong}
	short := intl.NumberFormatOptions{Notation: intl.NotationCompact}
	for _, c := range []struct {
		tag  string
		opts intl.NumberFormatOptions
		v    float64
		want string
	}{
		{"ar", long, 9999.9, "10 ألف"},
		{"ar", long, 9999, "10 ألف"},
		{"ar", long, -9999.9, "‎-10 ألف"},
		{"ar", long, 999999.5, "1 مليون"},
		{"en", short, 999999.5, "1M"},
		{"en", short, 999.95, "1K"},
		{"de", long, 999999.5, "1 Million"},
		{"ru", long, 9999.9, "10 тысяч"},
		{"pl", long, 4999.9, "5 tysięcy"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := newNumber(t, loc, c.opts).Format(c.v); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.tag, c.v, got, c.want)
		}
	}
}
