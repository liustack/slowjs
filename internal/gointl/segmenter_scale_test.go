//go:build gointl_all

package intl_test

import (
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestSegmentingScales pins that breaking a long run of Thai or Japanese into
// words, and asking which segment each offset is in, take time in proportion
// to the text: the dictionary's breaks were walked from the first for each
// break, and the boundaries from the first for each offset.
func TestSegmentingScales(t *testing.T) {
	for _, c := range []struct{ locale, unit string }{
		{"th", "สวัสดีครับ"}, {"ja", "日本語の文章です"},
	} {
		loc, err := intl.ParseLocale(c.locale)
		if err != nil {
			t.Fatal(err)
		}
		s, err := intl.NewSegmenter(loc, intl.SegmenterOptions{Granularity: intl.GranularityWord})
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Repeat(c.unit, 50000)
		start := time.Now()
		segs := s.SegmentString(text)
		n := len([]rune(text))
		for i := 0; i < n; i += 3 {
			if _, ok := segs.Containing(i); !ok {
				t.Fatalf("%s: nothing contains %d", c.locale, i)
			}
		}
		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("%s: %d characters took %v", c.locale, n, d)
		}
		if got := len(segs.All()); got < 50000 {
			t.Errorf("%s: %d segments", c.locale, got)
		}
	}
}
