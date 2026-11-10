//go:build gointl_all

package intl_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/corpus"
)

// The legacy methods format without a named formatter: Number's
// toLocaleString is a NumberFormat with no options, and Date's three are a
// DateTimeFormat with the required and default fields ECMA-402 gives each.
func TestToLocaleMatchesICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}
	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Kind != corpus.ToLocale {
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		v, ok := c.Number()
		if !ok {
			t.Errorf("%s: no value", c.Source)
			continue
		}
		ran++
		var got string
		if strings.HasPrefix(c.Source, "new Date(") {
			opts, err := dateTimeOptions(c.Options)
			if err != nil {
				t.Errorf("%s: %v", c.Source, err)
				continue
			}
			switch c.Method {
			case "toLocaleString":
				opts.Required, opts.Defaults = intl.ComponentsAny, intl.ComponentsAll
			case "toLocaleDateString":
				opts.Required, opts.Defaults = intl.ComponentsDate, intl.ComponentsDate
			case "toLocaleTimeString":
				opts.Required, opts.Defaults = intl.ComponentsTime, intl.ComponentsTime
			default:
				t.Errorf("%s: method %s", c.Source, c.Method)
				continue
			}
			dtf, err := intl.NewDateTimeFormat(loc, opts)
			if err != nil {
				differences = append(differences, c.Source+"\n   failed: "+err.Error())
				continue
			}
			got = dtf.Format(time.UnixMilli(int64(v)).UTC())
		} else {
			nf, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{})
			if err != nil {
				differences = append(differences, c.Source+"\n   failed: "+err.Error())
				continue
			}
			got = nf.Format(v)
		}
		if got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q", c.Source, got, c.Want))
		}
	}
	if ran == 0 {
		t.Fatal("no toLocale cases ran")
	}
	t.Logf("%d of %d toLocale cases match ICU exactly", matched, ran)
	if matched != ran {
		sort.Strings(differences)
		t.Errorf("%d of %d cases differ:\n%s", ran-matched, ran, strings.Join(differences, "\n"))
	}
}
