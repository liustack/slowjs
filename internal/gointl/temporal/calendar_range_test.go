//go:build gointl_all

package temporal

import (
	"strings"
	"testing"
)

// Calendar.Date refuses a day outside ISO's date-time limits, which no
// Temporal value holds: the Hebrew calendar had looped for ever on the year
// 2^40, and the solar calendars answered garbage years (ISSUES.md API-5).
func TestCalendarDateOutOfRange(t *testing.T) {
	for _, id := range []string{"hebrew", "chinese", "dangi", "coptic", "ethiopic", "persian", "indian",
		"islamic-civil", "japanese", "gregory", "iso8601"} {
		c, err := NewCalendar(id)
		if err != nil {
			t.Fatal(err)
		}
		dates := []ISODate{{275761, 1, 1}, {-271822, 1, 1}}
		// Years past an int, which a 32-bit one cannot be given.
		for _, y := range []int64{1 << 40, -(1 << 40)} {
			if int64(int(y)) == y {
				dates = append(dates, ISODate{int(y), 6, 15})
			}
		}
		for _, d := range dates {
			if _, err := c.Date(d); err == nil || !strings.Contains(err.Error(), "not within ISO date time limits") {
				t.Errorf("%s %v: %v, want the limits' error", id, d, err)
			}
		}
		if got, err := c.Date(ISODate{2020, 6, 15}); err != nil || got.Day == 0 {
			t.Errorf("%s 2020-06-15: %+v %v", id, got, err)
		}
	}
}
