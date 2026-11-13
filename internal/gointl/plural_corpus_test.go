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

func TestPluralRulesMatchICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "PluralRules" || c.Method != "select" {
			continue
		}
		var opts intl.PluralRulesOptions
		switch c.Options["type"] {
		case nil, "cardinal":
		case "ordinal":
			opts.Type = intl.Ordinal
		default:
			t.Errorf("%s: unknown type %v", c.Source, c.Options["type"])
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

		ran++
		p, err := intl.NewPluralRules(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		if got := string(p.Select(v)); got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q",
				c.Source, got, c.Want))
		}
	}

	if ran == 0 {
		t.Fatal("no PluralRules cases ran")
	}
	t.Logf("%d of %d PluralRules cases match ICU exactly (%.2f%%)",
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
