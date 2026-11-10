//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A locale's long compact patterns are only its own, as ICU's
// CompactData::populate reads them: a magnitude with none is written in
// full, and the short patterns stand in only where there are no long ones
// at all. cldr-json had filled the gaps from the root's short patterns:
// Pashto 1234 was "۱٫۲K". Node's answers (ISSUES.md NF-11).
func TestCompactLongOwnPatterns(t *testing.T) {
	for _, c := range []struct {
		tag  string
		v    float64
		want string
	}{
		{"ps", 1234, "۱۲۳۴"},
		{"ps", 1234567, "۱٬۲۳۴٬۵۶۷"},
		{"ps", 1234567890, "۱٫۲G"},
		{"ps-PK", 1234, "۱۲۳۴"},
		{"ast", 1234, "1,2 millares"},
		{"ast", 12345678901, "12.346 millones"},
		{"wo", 1234, "1,2 thousand"},
		{"wo", 1234567, "1,2M"},
		{"en", 1234, "1.2 thousand"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		f := newNumber(t, loc, intl.NumberFormatOptions{Notation: intl.NotationCompact,
			CompactDisplay: intl.CompactLong})
		if got := f.Format(c.v); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.tag, c.v, got, c.want)
		}
	}
}
