//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A time style is made afresh by the pattern generator, from "jmmss" and its
// kin, where the locale has a -u-hc keyword or ICU opens a bundle of another
// language or region for it, as SimpleDateFormat::construct does. go-intl
// had kept the locale's pattern and changed its hours, so British English
// with -u-hc-h12 wrote "07:05:09 am" for "7:05:09 am", and Japanese with
// -u-hc-h23 its full style "7時05分09秒 協定世界時". Node's answers (ISSUES.md
// DT-1).
func TestTimeStyleAfresh(t *testing.T) {
	d := time.Date(2024, 7, 13, 7, 5, 9, 0, time.UTC)
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want string
	}{
		{"en-GB-u-hc-h12", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium}, "7:05:09 am"},
		{"en-GB-u-hc-h23", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium}, "07:05:09"},
		{"ja-u-hc-h23", intl.DateTimeFormatOptions{TimeStyle: intl.LengthFull}, "7:05:09 協定世界時"},
		{"en-GB-u-hc-h11", intl.DateTimeFormatOptions{DateStyle: intl.LengthShort, TimeStyle: intl.LengthShort},
			"13/07/2024, 7:05 am"},
		{"de-u-hc-h12", intl.DateTimeFormatOptions{TimeStyle: intl.LengthShort}, "7:05 AM"},
		{"en-US-u-hc-h23", intl.DateTimeFormatOptions{TimeStyle: intl.LengthLong}, "07:05:09 UTC"},
		{"ar-u-ca-chinese", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium}, "07:05:09 ص"},
	} {
		c.opts.TimeZone = "UTC"
		c.opts.Compat = intl.NodeICU
		if got := newDateTime(t, c.tag, c.opts).Format(d); got != c.want {
			t.Errorf("%s %+v: %q, want %q", c.tag, c.opts, got, c.want)
		}
	}

	// The bundle ICU opens is its own, which cldr-json leaves to the
	// language: ICU has "en_US", so American English keeps its time styles.
	// Taking "en" for it made a twelve-hour time "02:12:47 PM", which
	// test262's timedatestyle-en.js caught.
	afternoon := time.Date(2024, 7, 13, 14, 12, 47, 0, time.UTC)
	yes := true
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want string
	}{
		{"en-US", intl.DateTimeFormatOptions{TimeStyle: intl.LengthFull, Hour12: &yes},
			"2:12:47 PM Coordinated Universal Time"},
		{"en-US", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium}, "2:12:47 PM"},
		{"de-DE", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium, Hour12: &yes}, "02:12:47 PM"},
		{"es-ES", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium, Hour12: &yes}, "2:12:47 p. m."},
		{"fr-FR", intl.DateTimeFormatOptions{TimeStyle: intl.LengthShort}, "14:12"},
		{"ja-JP", intl.DateTimeFormatOptions{TimeStyle: intl.LengthMedium}, "14:12:47"},
	} {
		c.opts.TimeZone = "UTC"
		c.opts.Compat = intl.NodeICU
		if got := newDateTime(t, c.tag, c.opts).Format(afternoon); got != c.want {
			t.Errorf("%s %+v: %q, want %q", c.tag, c.opts, got, c.want)
		}
	}
}
