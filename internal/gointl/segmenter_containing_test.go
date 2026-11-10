//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode/utf16"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestSegmenterContainingMatchesNode replays
// testdata/segmenter_containing_node.txt.gz, which segmenter_node.js
// writes: %Segments.prototype%.containing at every offset of a text, one
// before it and one past it, offsets inside a surrogate pair among them,
// in every granularity (ISSUES.md TE-1).
func TestSegmenterContainingMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/segmenter_containing_node.txt.gz")
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
	texts, offsets, bad := 0, 0, 0
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil || len(c) != 4 {
			t.Fatalf("a line of the wrong shape: %s", line)
		}
		var tag, granularity string
		var text []uint16
		var answers [][]*int
		json.Unmarshal(c[0], &tag)
		json.Unmarshal(c[1], &granularity)
		json.Unmarshal(c[2], &text)
		if err := json.Unmarshal(c[3], &answers); err != nil || len(answers) != len(text)+2 {
			t.Fatalf("a line of the wrong shape: %s", line)
		}
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
		texts++
		segs := seg.Segment(text)
		for i, want := range answers {
			n := i - 1
			offsets++
			got, ok := segs.Containing(n)
			switch {
			case want == nil && !ok:
				continue
			case want != nil && ok && got.Index == *want[0] && got.End == *want[1] &&
				(want[2] == nil && granularity != "word" || want[2] != nil && got.IsWordLike == (*want[2] == 1)):
				continue
			}
			bad++
			if bad <= 20 {
				t.Errorf("%s %s %q containing(%d): %+v %v, want %v", tag, granularity, string(utf16.Decode(text)), n, got, ok, deref(want))
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if texts < 1000 {
		t.Fatalf("only %d texts", texts)
	}
	t.Logf("%d texts, %d offsets", texts, offsets)
}

func deref(v []*int) []any {
	if v == nil {
		return nil
	}
	out := make([]any, len(v))
	for i, p := range v {
		if p != nil {
			out[i] = *p
		}
	}
	return out
}
