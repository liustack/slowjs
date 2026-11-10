//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// ICU puts no zone in a metazone before 1970 or after 9999-12-31 23:59, the
// bounds zonemeta.cpp gives a use metaZones.txt dates no further, so a
// specific name there falls back to the offset and a generic one to the
// place. Node's answers.
func TestMetazoneBounds(t *testing.T) {
	styles := []intl.ZoneStyle{intl.ZoneShort, intl.ZoneLong, intl.ZoneShortGeneric, intl.ZoneLongGeneric}
	for _, c := range []struct {
		zone string
		ms   int64
		want [4]string
	}{
		{"Europe/Paris", -2208988800000,
			[4]string{"1900, GMT+0:09:21", "1900, GMT+00:09:21", "1900, France Time", "1900, France Time"}},
		{"America/New_York", -2840140800000,
			[4]string{"1879, GMT-4:56:02", "1879, GMT-04:56:02", "1879, New York Time", "1879, New York Time"}},
		{"Asia/Calcutta", -2208988800000,
			[4]string{"1900, GMT+5:21:10", "1900, GMT+05:21:10", "1900, India Time", "1900, India Time"}},
		{"Europe/Paris", -1800000,
			[4]string{"1970, GMT+1", "1970, GMT+01:00", "1970, France Time", "1970, France Time"}},
		{"Europe/Paris", 0,
			[4]string{"1970, GMT+1", "1970, Central European Standard Time", "1970, France Time",
				"1970, Central European Standard Time"}},
		{"America/New_York", 253402257600000,
			[4]string{"9999, EST", "9999, Eastern Standard Time", "9999, ET", "9999, Eastern Time"}},
		{"America/New_York", 253402300800000,
			[4]string{"9999, GMT-5", "9999, GMT-05:00", "9999, New York Time", "9999, New York Time"}},
	} {
		for i, style := range styles {
			f := newDateTime(t, "en", intl.DateTimeFormatOptions{
				TimeZone: c.zone, Year: intl.WidthNumeric, TimeZoneName: style,
			})
			if got := f.Format(time.UnixMilli(c.ms)); got != c.want[i] {
				t.Errorf("%s at %d, style %d: %q, want %q", c.zone, c.ms, style, got, c.want[i])
			}
		}
	}
}
