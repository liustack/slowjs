//go:build gointl_all

package intl

import "testing"

// TestLoneSurrogatesCollate pins that a lone surrogate, which a JavaScript
// string may hold and which arrives in WTF-8, weighs as ICU weighs one: as a
// character no table names, by its code point, after every letter. The
// normalizer made each of its bytes U+FFFD, so every one compared equal.
func TestLoneSurrogatesCollate(t *testing.T) {
	loc, err := ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCollator(loc, CollatorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	hi, lo, hi2 := "\xed\xa0\x80", "\xed\xb0\x80", "\xed\xa0\x81"
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{hi, lo, -1}, {hi, hi2, -1}, {lo, hi, 1}, {"a", hi, -1}, {hi, hi, 0},
		{"a" + hi + "b", "a" + lo + "b", -1}, {"e\u0301" + hi, "\u00e9" + hi, 0},
	} {
		if got := c.Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
