//go:build gointl_all

package intl

import "testing"

// The rule interpreter against numerals whose spelling is not in doubt, and
// against Node for the Hebrew and Japanese ones: ICU's rules, run here, give
// ICU's answers.
func TestRBNF(t *testing.T) {
	for _, c := range []struct {
		system string
		n      int64
		want   string
	}{
		{"romanlow", 1, "i"},
		{"romanlow", 4, "iv"},
		{"romanlow", 12, "xii"},
		{"romanlow", 1999, "mcmxcix"},
		{"roman", 2024, "MMXXIV"},
		{"hebr", 24, "כ״ד"},
		{"hebr", 10, "י׳"},
		{"hebr", 15, "ט״ו"},
		{"hebr", 16, "ט״ז"},
		{"hebr", 784, "תשפ״ד"},
		{"jpanyear", 1, "元"},
		{"jpanyear", 6, "6"},
	} {
		rules, set, err := loadRBNF(Embedded, c.system)
		if err != nil {
			t.Fatal(err)
		}
		if got := rules.format(set, c.n); got != c.want {
			t.Errorf("%s %d = %q, want %q", c.system, c.n, got, c.want)
		}
	}
}
