//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestSegmenterMatchesNode replays testdata/segmenter_node.js: sentences in
// many scripts and thousands of random strings, broken in every
// granularity in locales whose rules differ, and each segment's
// word-likeness.
func TestSegmenterMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/segmenter_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(nil, 1<<20)
	segmenters := map[string]*intl.Segmenter{}
	ran, bad := 0, 0
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil || len(c) != 5 {
			t.Fatalf("a line of the wrong shape: %s", line)
		}
		var tag, granularity string
		var text []uint16
		var bounds, wordLike []int
		json.Unmarshal(c[0], &tag)
		json.Unmarshal(c[1], &granularity)
		json.Unmarshal(c[2], &text)
		json.Unmarshal(c[3], &bounds)
		json.Unmarshal(c[4], &wordLike)
		key := tag + " " + granularity
		seg := segmenters[key]
		if seg == nil {
			loc, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}
			g := map[string]intl.Granularity{"grapheme": intl.GranularityGrapheme,
				"word": intl.GranularityWord, "sentence": intl.GranularitySentence}[granularity]
			if seg, err = intl.NewSegmenter(loc, intl.SegmenterOptions{Granularity: g}); err != nil {
				t.Fatal(err)
			}
			segmenters[key] = seg
		}
		ran++
		segs := seg.Segment(text)
		var gotBounds, gotWordLike []int
		for _, x := range segs.All() {
			gotBounds = append(gotBounds, x.Index)
			if granularity == "word" {
				w := 0
				if x.IsWordLike {
					w = 1
				}
				gotWordLike = append(gotWordLike, w)
			}
		}
		gotBounds = append(gotBounds, len(text))
		if fmt.Sprint(gotBounds) != fmt.Sprint(bounds) || fmt.Sprint(gotWordLike) != fmt.Sprint(wordLike) {
			bad++
			if bad <= 30 {
				t.Errorf("%s %s %q:\n got  %v %v\n want %v %v", tag, granularity, string(utf16ToRunes(text)),
					gotBounds, gotWordLike, bounds, wordLike)
			}
			continue
		}
		// containing(n) is the segment about n, an offset inside a surrogate
		// pair taken as the pair's start.
		for n := 0; n < len(text); n++ {
			m := n
			if m > 0 && text[m] >= 0xdc00 && text[m] <= 0xdfff && text[m-1] >= 0xd800 && text[m-1] <= 0xdbff {
				m--
			}
			i := 0
			for i+1 < len(bounds) && bounds[i+1] <= m {
				i++
			}
			got, ok := segs.Containing(n)
			if !ok || got.Index != bounds[i] || got.End != bounds[i+1] {
				bad++
				t.Errorf("%s %s %q: containing(%d) = %+v, want %d..%d", tag, granularity,
					string(utf16ToRunes(text)), n, got, bounds[i], bounds[i+1])
				break
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if ran < 9000 {
		t.Fatalf("%d cases: the recording is short", ran)
	}
	t.Logf("%d cases, %d differ", ran, bad)
}

func utf16ToRunes(u []uint16) []rune {
	var out []rune
	for i := 0; i < len(u); i++ {
		c := rune(u[i])
		if c >= 0xd800 && c <= 0xdbff && i+1 < len(u) && u[i+1] >= 0xdc00 && u[i+1] <= 0xdfff {
			c = 0x10000 + (c-0xd800)<<10 + rune(u[i+1]) - 0xdc00
			i++
		}
		out = append(out, c)
	}
	return out
}
