//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The Persian calendar reckons as ICU 78 does: the 30th of Esfand exists in a
// leap year and not otherwise, and the year turns at Nowruz. The expectations
// are Node's.
func TestPersianCalendar(t *testing.T) {
	f := newDateTime(t, "en-u-ca-persian", intl.DateTimeFormatOptions{
		TimeZone: "UTC", Year: intl.WidthNumeric, Month: intl.WidthNumeric, Day: intl.WidthNumeric,
	})
	for _, c := range []struct {
		year  int
		month time.Month
		day   int
		want  string
	}{
		{2025, time.March, 20, "12/30/1403 AP"},
		{2025, time.March, 21, "1/1/1404 AP"},
		{2024, time.March, 20, "1/1/1403 AP"},
		{2024, time.March, 19, "12/29/1402 AP"},
		{1979, time.February, 11, "11/22/1357 AP"},
		{2100, time.March, 21, "1/1/1479 AP"},
		{622, time.March, 19, "12/28/0 AP"},
	} {
		if got := f.Format(time.Date(c.year, c.month, c.day, 0, 0, 0, 0, time.UTC)); got != c.want {
			t.Errorf("%d-%02d-%02d = %q, want %q", c.year, c.month, c.day, got, c.want)
		}
	}
}

// A pattern's "Y" is the week-based year, which ICU reckons from the
// calendar's extended year. The Buddhist calendar relabels the Gregorian
// years without changing its extended year, so Burmese, whose Buddhist
// short date is "GGGGG dd/MM/Y", writes 2024 where "y" would be 2567.
func TestWeekYearIsTheExtendedYears(t *testing.T) {
	f := newDateTime(t, "my-u-ca-buddhist", intl.DateTimeFormatOptions{
		TimeZone: "UTC", Year: intl.WidthNumeric, Month: intl.WidthNumeric, Day: intl.WidthNumeric,
	})
	if got, want := f.Format(time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)), "BE \u1040\u1045/\u1040\u1041/\u1042\u1040\u1042\u1044"; got != want {
		t.Errorf("got %+q, want %+q", got, want)
	}
}
