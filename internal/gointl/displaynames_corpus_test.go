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

func TestDisplayNamesMatchICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "DisplayNames" || c.Method != "of" {
			continue
		}
		name, _ := c.Options["type"].(string)
		kind, ok := intl.ParseDisplayKind(name)
		if !ok {
			t.Errorf("%s: unknown type %q", c.Source, name)
			continue
		}
		opts := intl.DisplayNamesOptions{Kind: kind}
		switch c.Options["style"] {
		case nil, "long":
		case "short":
			opts.Style = intl.DisplayShort
		case "narrow":
			opts.Style = intl.DisplayNarrow
		default:
			t.Errorf("%s: unknown style %v", c.Source, c.Options["style"])
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		var code string
		for _, a := range c.Args {
			if s, ok := a.(string); ok {
				code = s
			}
		}

		ran++
		dn, err := intl.NewDisplayNames(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		got, _ := dn.Of(code)
		if got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q",
				c.Source, got, c.Want))
		}
	}

	if ran == 0 {
		t.Fatal("no DisplayNames cases ran")
	}
	t.Logf("%d of %d DisplayNames cases match ICU exactly (%.2f%%)",
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
