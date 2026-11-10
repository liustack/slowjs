//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A calendar's style patterns are the ones ICU finds for it: following the
// root's alias to the generic calendar up the locale's chain, where
// cldr-json had given every calendar the locale's Gregorian patterns. Danish
// writes a Chinese time with the root's colon, and British English an
// Indian time from English's generic patterns. Node's answers (ISSUES.md
// DT-3).
func TestCalendarStylePatterns(t *testing.T) {
	d := time.Date(2024, 7, 13, 7, 5, 9, 0, time.UTC)
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want string
	}{
		{"en-GB-u-ca-indian", intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, Minute: intl.Width2Digit}, "07:05"},
		{"da-u-ca-chinese", intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, Minute: intl.Width2Digit,
			Second: intl.Width2Digit}, "7:05:09"},
		{"zh-Hans-HK-u-ca-coptic", intl.DateTimeFormatOptions{DateStyle: intl.LengthShort}, "科普特历1740/11/6"},
		{"en-GB-u-ca-coptic", intl.DateTimeFormatOptions{TimeStyle: intl.LengthShort}, "07:05"},
		{"da-u-ca-buddhist", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium}, "07.05.09"},
		{"da", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium}, "07.05.09"},
	} {
		c.opts.TimeZone = "UTC"
		if got := newDateTime(t, c.tag, c.opts).Format(d); got != c.want {
			t.Errorf("%s %+v: %q, want %q", c.tag, c.opts, got, c.want)
		}
	}
}
