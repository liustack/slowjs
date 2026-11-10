//go:build gointl_all

package intl_test

import (
	"errors"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// ICU's Chinese astronomy (ChineseAstronomy) cannot reckon a date where
// the winter solstices it finds no longer fall either side of a date it
// needs: the date's own, or the start of its year or the next, which
// Calendar::computeWeekFields asks for. Node throws there, and Check
// reports it; go-intl had written month 0 with no name. The days are
// Node's, counted from 1970 (ISSUES.md DT-6).
func TestChineseAstronomyRange(t *testing.T) {
	for _, c := range []struct {
		tag  string
		days []int64 // days Node writes
		fail []int64 // days Node throws for
	}{
		{"en-u-ca-chinese", []int64{24992448, 24992783, 24993544, -37242661, 0},
			[]int64{24992449, 24992782, 24993545, 24994344, -37242662}},
		{"en-u-ca-dangi", []int64{-37242661, 0}, []int64{-37242662}},
	} {
		node := newDateTime(t, c.tag, intl.DateTimeFormatOptions{TimeZone: "UTC", Year: intl.WidthNumeric,
			Month: intl.WidthLong, Day: intl.WidthNumeric, Compat: intl.NodeICU})
		standard := newDateTime(t, c.tag, intl.DateTimeFormatOptions{TimeZone: "UTC", Year: intl.WidthNumeric,
			Month: intl.WidthLong, Day: intl.WidthNumeric})
		day := func(d int64) time.Time { return time.Unix(d*86400, 0).UTC() }
		for _, d := range c.days {
			if err := node.Check(day(d)); err != nil {
				t.Errorf("%s day %d: %v, want none", c.tag, d, err)
			}
		}
		for _, d := range c.fail {
			if err := node.Check(day(d)); !errors.Is(err, intl.ErrCalendarRange) {
				t.Errorf("%s day %d: %v, want ErrCalendarRange", c.tag, d, err)
			}
			// Temporal's reckoning, the standard's, has no such limit.
			if err := standard.Check(day(d)); err != nil {
				t.Errorf("%s day %d, standard: %v, want none", c.tag, d, err)
			}
		}
	}
}
