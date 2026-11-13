//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A two-digit year is its last two digits as ICU's zeroPaddingNumber(value,
// 2, 2) writes them, keeping the sign of a year below zero: the Persian year
// -121 is "-21". go-intl had written "79". Node's answers (ISSUES.md DT-5).
func TestTwoDigitYearSign(t *testing.T) {
	year := func(y int) time.Time { return time.Date(y, 6, 1, 0, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		tag  string
		at   time.Time
		opts intl.DateTimeFormatOptions
		want string
	}{
		{"en-u-ca-persian", year(500), intl.DateTimeFormatOptions{Year: intl.Width2Digit}, "-21 AP"},
		{"en-u-ca-persian", year(522), intl.DateTimeFormatOptions{Year: intl.Width2Digit}, "-99 AP"},
		{"en-u-ca-persian", year(612), intl.DateTimeFormatOptions{Year: intl.Width2Digit}, "-09 AP"},
		{"en-u-ca-persian", year(621), intl.DateTimeFormatOptions{Year: intl.Width2Digit}, "00 AP"},
		{"en-u-ca-persian", year(500), intl.DateTimeFormatOptions{Year: intl.Width2Digit,
			Month: intl.Width2Digit}, "03/-21 AP"},
		{"en-u-ca-japanese", year(500), intl.DateTimeFormatOptions{Year: intl.Width2Digit},
			"-44 Taika (645–650)"},
		{"en-u-ca-buddhist", year(-600), intl.DateTimeFormatOptions{Year: intl.Width2Digit}, "-57 BE"},
		{"en-u-ca-indian", year(30), intl.DateTimeFormatOptions{Year: intl.Width2Digit}, "-48 Śaka"},
		{"fa-u-ca-persian", year(500), intl.DateTimeFormatOptions{Year: intl.Width2Digit}, "‎−۲۱"},
		{"en", year(-50), intl.DateTimeFormatOptions{Year: intl.Width2Digit}, "51"},
	} {
		c.opts.TimeZone = "UTC"
		f := newDateTime(t, c.tag, c.opts)
		if got := f.Format(c.at); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.tag, c.at.Year(), got, c.want)
		}
	}
}
