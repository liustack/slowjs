//go:build gointl_all

package intl

import "testing"

// islamcal.cpp's civilLeapYear takes C's remainder, which keeps the sign of
// the dividend, so every year before 0 is a leap year: -7 is, though its
// remainder counted from zero up would be 27.
func TestIslamicCivilLeapYears(t *testing.T) {
	for _, c := range []struct{ year, length int }{
		{2, 355}, {3, 354}, {1445, 355}, {1446, 354}, {0, 354}, {-1, 355}, {-2, 355}, {-7, 355},
	} {
		if got := islamicTabularYearLength(c.year); got != c.length {
			t.Errorf("islamicTabularYearLength(%d) = %d, want %d", c.year, got, c.length)
		}
	}
}

// The Umm al-Qura table's years are the sums of its months, and outside it
// the civil calendar's; the lengths are Node's.
func TestUmmAlQuraYearLength(t *testing.T) {
	u, err := loadUmmAlQura(Embedded)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ year, length int }{
		{1299, 354}, {1300, 354}, {1302, 355}, {1445, 354}, {1599, 355}, {1600, 354}, {1603, 355},
	} {
		if got := u.yearLength(c.year); got != c.length {
			t.Errorf("yearLength(%d) = %d, want %d", c.year, got, c.length)
		}
	}
}
