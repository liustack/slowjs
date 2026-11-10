//go:build gointl_all

package intl_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/temporal"
)

// TestEastAsianDaysMatchTemporal holds the standard's Chinese calendar and
// Dangi to Temporal's, day by day, as test262's
// compare-to-temporal-lunisolar asks: the year, the month and whether it is
// the leap one, and the day, from 1600 to 2400.
func TestEastAsianDaysMatchTemporal(t *testing.T) {
	iso, err := temporal.NewCalendar("iso8601")
	if err != nil {
		t.Fatal(err)
	}
	for _, calendar := range []string{"chinese", "dangi"} {
		cal, err := temporal.NewCalendar(calendar)
		if err != nil {
			t.Fatal(err)
		}
		loc, _ := intl.ParseLocale("en-u-nu-latn")
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
			Calendar: calendar, TimeZone: "UTC",
			Year: intl.WidthNumeric, Month: intl.WidthNumeric, Day: intl.WidthNumeric,
		})
		if err != nil {
			t.Fatal(err)
		}
		days, wrong := 0, 0
		for at := time.Date(1600, 1, 1, 0, 0, 0, 0, time.UTC); at.Year() <= 2400; at = at.AddDate(0, 0, 1) {
			days++
			d, err := temporal.NewPlainDate(at.Year(), int(at.Month()), at.Day(), iso, temporal.Reject)
			if err != nil {
				t.Fatal(err)
			}
			d = d.WithCalendar(cal)
			want := fmt.Sprintf("%d %s %d", d.Year(), d.MonthCode(), d.Day())
			if got := eastAsianFields(f.FormatToParts(at)); got != want {
				wrong++
				if wrong <= 5 {
					t.Errorf("%s %s: %s, want %s", calendar, at.Format("2006-01-02"), got, want)
				}
			}
		}
		if wrong > 0 {
			t.Errorf("%s: %d of %d days differ", calendar, wrong, days)
		}
	}
}

// eastAsianFields is a written date as Temporal's year, month code and day:
// the related year, and a month whose number is followed by a leap mark.
func eastAsianFields(parts []intl.Part) string {
	var year, month, day string
	for _, p := range parts {
		switch p.Kind {
		case intl.PartRelatedYear:
			year = p.Value
		case intl.PartMonth:
			month = p.Value
		case intl.PartDay:
			day = p.Value
		}
	}
	digits := strings.TrimRightFunc(month, func(r rune) bool { return r < '0' || r > '9' })
	n, _ := strconv.Atoi(digits)
	code := fmt.Sprintf("M%02d", n)
	if digits != month {
		code += "L"
	}
	return year + " " + code + " " + day
}
