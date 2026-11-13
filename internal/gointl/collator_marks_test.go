//go:build gointl_all

package intl

import (
	"strings"
	"testing"
	"time"
)

// A long run of marks that each begin a contraction is linear: each
// U+0F71, which begins Tibetan's contractions with U+0F72, U+0F74 and
// U+0F80, had looked through every mark after it for one, though those
// of its own class are blocked, and 40,000 had taken 54 seconds (ISSUES.md
// PE-1). The order is unchanged: Node's for the strings here.
func TestLongRunOfContractingMarks(t *testing.T) {
	texts := []string{"ཀཱི", "ཀཱི", "ཀཱཱུ", "ཀཱཱྀ",
		"ཀཱཱཱི", "ཀཱི", "ཀཱུ", "ཀཱིུ"}
	node := []int{0, -1, -1, -1, 0, -1, -1, -1, -1, -1, 0, -1, -1, 1, 1, 1, 1, 1, 1, 1, -1, 1, 1, -1, -1, -1, -1, 1}
	loc, err := ParseLocale("und")
	if err != nil {
		t.Fatal(err)
	}
	for _, compat := range []Compat{Standard, NodeICU} {
		c, err := NewCollator(loc, CollatorOptions{Compat: compat})
		if err != nil {
			t.Fatal(err)
		}
		k := 0
		for i := range texts {
			for j := i + 1; j < len(texts); j++ {
				if got := c.Compare(texts[i], texts[j]); got != node[k] {
					t.Errorf("%v %+q %+q: %d, want %d", compat, texts[i], texts[j], got, node[k])
				}
				k++
			}
		}

		long := "a" + strings.Repeat("ཱ", 40000)
		start := time.Now()
		if got := c.Compare(long, long+"x"); got != -1 {
			t.Errorf("%v: a run of U+0F71 against it and x: %d", compat, got)
		}
		if got := c.Compare(long+"ི", "b"+long); got != -1 {
			t.Errorf("%v: a run of U+0F71 and U+0F72 against b: %d", compat, got)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%v: 40,000 U+0F71 took %v", compat, d)
		}
	}
}
