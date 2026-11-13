//go:build gointl_all

package temporal

import (
	"testing"
	"time"
)

// until in months over thousands of Chinese or Korean years is linear:
// each month it tried balanced from the first year again, walking the
// years, and 48,000 years took 17 seconds (ISSUES.md PE-2). Node's
// answers, which Node takes 21 seconds over.
func TestUntilInMonthsOverManyYears(t *testing.T) {
	for _, id := range []string{"chinese", "dangi"} {
		cal, err := NewCalendar(id)
		if err != nil {
			t.Fatal(err)
		}
		date := func(year int, code string, day int) PlainDate {
			d, err := PlainDateFromFields(CalendarFields{Year: Int(year), MonthCode: String(code), Day: Int(day)}, cal, Reject)
			if err != nil {
				t.Fatal(err)
			}
			return d
		}
		a := date(2000, "M01", 30)
		for _, c := range []struct {
			years             int
			months, backwards string
			inYears           string
		}{
			{1000, "P12372M15D", "-P12372M15D", "P1000Y4M15D"},
			{48000, "P593681M15D", "-P593681M15D", "P48000Y4M15D"},
		} {
			b := date(2000+c.years, "M06", 15)
			start := time.Now()
			for _, w := range []struct {
				from, to PlainDate
				largest  Unit
				want     string
			}{{a, b, Month, c.months}, {b, a, Month, c.backwards}, {a, b, Year, c.inYears}} {
				d, err := w.from.Until(w.to, DifferenceSettings{LargestUnit: w.largest})
				if err != nil {
					t.Fatal(err)
				}
				if s, _ := d.String(DefaultToStringOptions()); s != w.want {
					t.Errorf("%s %d years in %v: %s, want %s", id, c.years, w.largest, s, w.want)
				}
			}
			if took := time.Since(start); took > 5*time.Second {
				t.Errorf("%s %d years: %v", id, c.years, took)
			}
		}
	}
}
