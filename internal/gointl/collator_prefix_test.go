//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Two strings that share a prefix: Standard compares the whole strings, as
// UCA does, and NodeICU compares from after the prefix as ICU's doCompare
// does, backed up only before a character in ICU's unsafe-backward set or a
// digit its tailoring-only, one-unit digit test sees (IdenticalPrefix). The
// NodeICU answers are Node's, which 7,065,792 pairs of strings sharing a
// prefix, over 29 locales and 8 option sets, all match (ISSUES.md RU-1).
func TestIdenticalPrefix(t *testing.T) {
	yes := true
	for _, c := range []struct {
		tag       string
		opts      intl.CollatorOptions
		a, b      string
		std, node int
	}{
		// Arabic tailors none of its digits, so ICU's digit test does not
		// see them, and skips the shared 1 of 15 and 100.
		{"ar", intl.CollatorOptions{Numeric: &yes}, "١٥", "١٠٠", -1, 1},
		{"cs", intl.CollatorOptions{Numeric: &yes}, "1٥", "1٠٠", -1, 1},
		// The root's table has them, so ICU reads the number whole.
		{"und", intl.CollatorOptions{Numeric: &yes}, "١٥", "١٠٠", -1, -1},
		{"en", intl.CollatorOptions{Numeric: &yes}, "15", "100", -1, -1},
		// A supplementary digit is two units, neither of them a digit.
		{"und", intl.CollatorOptions{Numeric: &yes}, "1é", "1\U0001D7CF", -1, 1},
		{"en", intl.CollatorOptions{Numeric: &yes}, "x1\U0001D7CF", "x1é", 1, -1},
		{"en", intl.CollatorOptions{Numeric: &yes}, "a\U0001D7CE5", "a\U0001D7CF", 1, 1},
		// French Canada's backward accents, read from the end of what
		// follows the prefix.
		{"fr-CA", intl.CollatorOptions{}, "a့ံ", "a့", -1, 1},
		{"fr-CA", intl.CollatorOptions{Sensitivity: intl.SensitivityAccent}, "a̴‍", "a̴‍̀", 1, -1},
		// A shifted hyphen in the prefix no longer makes the ignorable after
		// it ignorable.
		{"en", intl.CollatorOptions{IgnorePunctuation: &yes}, "-", "-ं", 0, -1},
		// A combining mark is unsafe: the comparison starts at the b.
		{"en", intl.CollatorOptions{}, "ab́", "ab̀", -1, -1},
		// Lone surrogates, as WTF-8 carries them.
		{"en", intl.CollatorOptions{}, "a\xed\xa0\x80", "a\xed\xb0\x80", -1, -1},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		for _, side := range []struct {
			compat intl.Compat
			want   int
		}{{intl.Standard, c.std}, {intl.NodeICU, c.node}} {
			opts := c.opts
			opts.Compat = side.compat
			coll, err := intl.NewCollator(loc, opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := coll.Compare(c.a, c.b); got != side.want {
				t.Errorf("%s %v %+q %+q: %d, want %d", c.tag, side.compat, c.a, c.b, got, side.want)
			}
			if got := coll.Compare(c.b, c.a); got != -side.want {
				t.Errorf("%s %v %+q %+q: %d, want %d", c.tag, side.compat, c.b, c.a, got, -side.want)
			}
		}
	}
}
