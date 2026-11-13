//go:build gointl_all

package temporal

import (
	"math"
	"strings"
	"testing"
)

// A non-ISO calendar refuses a duration field of math.MinInt64, as it does
// one past 2^32, where abs64 had left it negative and the month loop run for
// ever. temporal_rs refuses it so (ISSUES.md API-3).
func TestDateAddMinInt64(t *testing.T) {
	date := ISODate{2020, 1, 1}
	for _, id := range []string{"hebrew", "chinese", "japanese", "islamic-civil"} {
		c, err := NewCalendar(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, dur := range []DateDuration{
			{Months: math.MinInt64}, {Years: math.MinInt64}, {Weeks: math.MinInt64}, {Days: math.MinInt64},
			{Months: 1 << 32},
		} {
			_, err := c.DateAdd(date, dur, Constrain)
			if err == nil || !strings.Contains(err.Error(), "Duration was not valid.") {
				t.Errorf("%s %+v: %v, want Duration was not valid.", id, dur, err)
			}
		}
	}
}
