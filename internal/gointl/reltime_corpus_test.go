//go:build gointl_all

package intl_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/corpus"
)

func TestRelativeTimeFormatMatchesICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "RelativeTimeFormat" || c.Method != "format" {
			continue
		}
		var opts intl.RelativeTimeFormatOptions
		switch c.Options["numeric"] {
		case nil, "always":
		case "auto":
			opts.Numeric = intl.RelativeAuto
		default:
			t.Errorf("%s: unknown numeric %v", c.Source, c.Options["numeric"])
			continue
		}
		switch c.Options["style"] {
		case nil, "long":
		case "short":
			opts.Style = intl.RelativeShort
		case "narrow":
			opts.Style = intl.RelativeNarrow
		default:
			t.Errorf("%s: unknown style %v", c.Source, c.Options["style"])
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		v, ok := c.Number()
		if !ok {
			t.Errorf("%s: no number", c.Source)
			continue
		}
		var name string
		for _, a := range c.Args {
			if s, ok := a.(string); ok {
				name = s
			}
		}
		unit, ok := intl.ParseRelativeTimeUnit(name)
		if !ok {
			t.Errorf("%s: unknown unit %q", c.Source, name)
			continue
		}

		ran++
		rtf, err := intl.NewRelativeTimeFormat(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		if got := rtf.Format(v, unit); got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q",
				c.Source, got, c.Want))
		}
	}

	if ran == 0 {
		t.Fatal("no RelativeTimeFormat cases ran")
	}
	t.Logf("%d of %d RelativeTimeFormat cases match ICU exactly (%.2f%%)",
		matched, ran, 100*float64(matched)/float64(ran))
	if matched != ran {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 25 {
			shown = shown[:25]
		}
		t.Errorf("%d of %d cases differ:\n%s", ran-matched, ran, strings.Join(shown, "\n"))
	}
}
