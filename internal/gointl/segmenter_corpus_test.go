//go:build gointl_all

package intl_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/corpus"
)

// TestSegmenterMatchesICU replays the golden corpus's Segmenter cases: a
// field of every segment, joined as JavaScript joins an array.
func TestSegmenterMatchesICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}
	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "Segmenter" || c.Kind != corpus.Segment {
			continue
		}
		var opts intl.SegmenterOptions
		switch c.Options["granularity"] {
		case nil, "grapheme":
		case "word":
			opts.Granularity = intl.GranularityWord
		case "sentence":
			opts.Granularity = intl.GranularitySentence
		default:
			t.Errorf("%s: unknown granularity %v", c.Source, c.Options["granularity"])
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		text, ok := c.Args[0].(string)
		if !ok {
			t.Errorf("%s: no text", c.Source)
			continue
		}
		ran++
		s, err := intl.NewSegmenter(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		u := utf16.Encode([]rune(text))
		var parts []string
		for _, seg := range s.Segment(u).All() {
			switch c.Field {
			case "segment":
				parts = append(parts, string(utf16.Decode(u[seg.Index:seg.End])))
			case "index":
				parts = append(parts, strconv.Itoa(seg.Index))
			case "isWordLike":
				// Undefined outside word granularity, which joins as "".
				switch {
				case c.IfWordLike != "" || c.IfNot != "":
					if seg.IsWordLike {
						parts = append(parts, c.IfWordLike)
					} else {
						parts = append(parts, c.IfNot)
					}
				case opts.Granularity == intl.GranularityWord:
					parts = append(parts, strconv.FormatBool(seg.IsWordLike))
				default:
					parts = append(parts, "")
				}
			default:
				t.Fatalf("%s: unknown field %q", c.Source, c.Field)
			}
		}
		if got := strings.Join(parts, c.Join); got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q", c.Source, got, c.Want))
		}
	}
	for i, d := range differences {
		if i == 20 {
			break
		}
		t.Error(d)
	}
	if ran != 140 {
		t.Errorf("%d Segmenter cases, want 140", ran)
	}
	t.Logf("%d of %d match", matched, ran)
}
