//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A time style for a locale whose -u-hc keyword hour12 or hourCycle
// overrode: the standard resolves the keyword away and writes the locale's
// own style, as test262's timedatestyle-en.js asks; V8 gives ICU the
// keyword, which makes the style afresh in the keyword's cycle and then
// turns it to twelve hours with its two digits (HourCycleStyles). Node's
// are its answers.
func TestHourCycleStyles(t *testing.T) {
	d := time.Date(1886, 5, 1, 14, 12, 47, 0, time.UTC)
	yes, no := true, false
	for _, c := range []struct {
		tag      string
		opts     intl.DateTimeFormatOptions
		standard string
		node     string
	}{
		{"en-US-u-hc-h23", intl.DateTimeFormatOptions{Hour12: &yes},
			"2:12:47 PM Coordinated Universal Time", "02:12:47 PM Coordinated Universal Time"},
		{"en-US-u-hc-h24", intl.DateTimeFormatOptions{HourCycle: intl.H23, Hour12: &yes},
			"2:12:47 PM Coordinated Universal Time", "02:12:47 PM Coordinated Universal Time"},
		// Where the keyword stays, both make the style afresh.
		{"en-US-u-hc-h23", intl.DateTimeFormatOptions{},
			"14:12:47 Coordinated Universal Time", "14:12:47 Coordinated Universal Time"},
		{"en-US-u-hc-h12", intl.DateTimeFormatOptions{Hour12: &no},
			"14:12:47 Coordinated Universal Time", "14:12:47 Coordinated Universal Time"},
		{"en-US", intl.DateTimeFormatOptions{Hour12: &yes},
			"2:12:47 PM Coordinated Universal Time", "2:12:47 PM Coordinated Universal Time"},
	} {
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.NodeICU, c.node}} {
			o := c.opts
			o.TimeStyle, o.TimeZone, o.Compat = intl.LengthFull, "UTC", side.compat
			if got := newDateTime(t, c.tag, o).Format(d); got != side.want {
				t.Errorf("%s %+v %v: %q, want %q", c.tag, c.opts, side.compat, got, side.want)
			}
		}
	}
}
