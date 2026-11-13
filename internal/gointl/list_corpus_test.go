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

func TestListFormatMatchesICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "ListFormat" || c.Method != "format" {
			continue
		}
		var opts intl.ListFormatOptions
		switch c.Options["type"] {
		case nil, "conjunction":
		case "disjunction":
			opts.Type = intl.Disjunction
		case "unit":
			opts.Type = intl.UnitList
		default:
			t.Errorf("%s: unknown type %v", c.Source, c.Options["type"])
			continue
		}
		switch c.Options["style"] {
		case nil, "long":
		case "short":
			opts.Style = intl.ListShort
		case "narrow":
			opts.Style = intl.ListNarrow
		default:
			t.Errorf("%s: unknown style %v", c.Source, c.Options["style"])
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		items, ok := c.Strings()
		if !ok {
			t.Errorf("%s: no list", c.Source)
			continue
		}

		ran++
		lf, err := intl.NewListFormat(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		if got := lf.Format(items); got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q",
				c.Source, got, c.Want))
		}
	}

	if ran == 0 {
		t.Fatal("no ListFormat cases ran")
	}
	t.Logf("%d of %d ListFormat cases match ICU exactly (%.2f%%)",
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
