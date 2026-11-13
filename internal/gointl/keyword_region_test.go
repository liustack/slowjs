//go:build gointl_all

package intl_test

import (
	"reflect"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A "-u-rg-" or "-u-sd-" keyword names a region only where ICU's
// GetRegionFromKey takes it: three to six characters starting with two
// letters that RegionValidateMap has as a region. go-intl had taken any
// first two letters, so "en-u-rg-xxzzzz" had its week begin on Monday.
// A keyword with no value is ICU's "yes", Yemen, on Node's side
// (YesValues). Node's answers (ISSUES.md LO-4).
func TestKeywordRegionValid(t *testing.T) {
	for _, c := range []struct {
		tag      string
		compat   intl.Compat
		firstDay int
		weekend  []int
		cycles   []string
	}{
		{"en-u-rg-xxzzzz", intl.NodeICU, 7, []int{6, 7}, []string{"h12"}},
		{"en-u-rg-abcdefgh", intl.NodeICU, 7, []int{6, 7}, []string{"h12"}},
		{"en-u-rg-gbzzzz", intl.NodeICU, 1, []int{6, 7}, []string{"h23"}},
		{"en-US-u-rg-dezzzz", intl.NodeICU, 1, []int{6, 7}, []string{"h23"}},
		{"en-u-sd-gbsct", intl.NodeICU, 1, []int{6, 7}, []string{"h12"}},
		{"en-u-sd-xxabc", intl.NodeICU, 7, []int{6, 7}, []string{"h12"}},
		{"en-GB-u-rg-uszzzz", intl.NodeICU, 7, []int{6, 7}, []string{"h12"}},
		{"en-u-rg-gb", intl.NodeICU, 7, []int{5, 6}, []string{"h12"}},
		{"en-u-rg-xxzzzz", intl.Standard, 7, []int{6, 7}, []string{"h12"}},
		{"en-u-rg-gb", intl.Standard, 7, []int{6, 7}, []string{"h12"}},
	} {
		info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		l, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		firstDay, weekend, err := info.WeekInfo(l)
		if err != nil {
			t.Fatal(err)
		}
		if firstDay != c.firstDay || !reflect.DeepEqual(weekend, c.weekend) {
			t.Errorf("%s %v: week %d %v, want %d %v", c.tag, c.compat, firstDay, weekend, c.firstDay, c.weekend)
		}
		cycles, err := info.HourCycles(l)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(cycles, c.cycles) {
			t.Errorf("%s %v: hour cycles %v, want %v", c.tag, c.compat, cycles, c.cycles)
		}
	}
}
