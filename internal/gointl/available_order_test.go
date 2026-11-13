//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The pattern generator takes a locale's available formats in ICU's order,
// the locale's own bundle first, which decides between two skeletons equally
// near the one asked for. go-intl had taken them sorted by skeleton, so
// Spanish in Latin America wrote a day and a long month as Spanish does, "13
// de julio", where its own "d-MMMM" comes first. Node's answers (ISSUES.md
// DT-4).
func TestAvailableFormatsOrder(t *testing.T) {
	d := time.Date(2024, 7, 13, 7, 5, 9, 0, time.UTC)
	monthDay := intl.DateTimeFormatOptions{Month: intl.WidthNumeric, Day: intl.Width2Digit}
	longMonthDay := intl.DateTimeFormatOptions{Month: intl.WidthLong, Day: intl.Width2Digit}
	for _, c := range []struct {
		tag  string
		opts intl.DateTimeFormatOptions
		want string
	}{
		{"en-ZA", monthDay, "07/13"},
		{"es-PA", monthDay, "07/13"},
		{"es-PR", monthDay, "07/13"},
		{"es-CL", monthDay, "13-07"},
		{"es-419", longMonthDay, "13-julio"},
		{"es-MX", longMonthDay, "13-julio"},
		{"es", longMonthDay, "13 de julio"},
		{"en", monthDay, "7/13"},
	} {
		c.opts.TimeZone = "UTC"
		if got := newDateTime(t, c.tag, c.opts).Format(d); got != c.want {
			t.Errorf("%s %+v: %q, want %q", c.tag, c.opts, got, c.want)
		}
	}
}
