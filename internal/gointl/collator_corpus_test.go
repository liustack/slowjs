//go:build gointl_all

package intl_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/corpus"
)

func TestCollatorMatchesICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "Collator" {
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		opts, err := collatorOptions(c.Options)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		ran++
		coll, err := intl.NewCollator(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		var got string
		switch c.Kind {
		case corpus.Compare:
			var args []string
			for _, a := range c.Args {
				if s, ok := a.(string); ok {
					args = append(args, s)
				}
			}
			if len(args) != 2 {
				t.Errorf("%s: %d strings to compare", c.Source, len(args))
				continue
			}
			got = strconv.Itoa(coll.Compare(args[0], args[1]))
		case corpus.Sort:
			list, ok := c.Strings()
			if !ok {
				t.Errorf("%s: nothing to sort", c.Source)
				continue
			}
			list = append([]string(nil), list...)
			sort.SliceStable(list, func(i, j int) bool { return coll.Compare(list[i], list[j]) < 0 })
			got = strings.Join(list, c.Join)
		default:
			t.Errorf("%s: a %v case", c.Source, c.Kind)
			continue
		}
		if got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q",
				c.Source, got, c.Want))
		}
	}

	if ran == 0 {
		t.Fatal("no Collator cases ran")
	}
	t.Logf("%d of %d Collator cases match ICU exactly (%.2f%%)",
		matched, ran, 100*float64(matched)/float64(ran))
	if matched != ran {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 30 {
			shown = shown[:30]
		}
		t.Errorf("%d of %d cases differ:\n%s", ran-matched, ran, strings.Join(shown, "\n"))
	}
}

func collatorOptions(in map[string]any) (intl.CollatorOptions, error) {
	var out intl.CollatorOptions
	for key, value := range in {
		switch key {
		case "sensitivity":
			switch value {
			case "base":
				out.Sensitivity = intl.SensitivityBase
			case "accent":
				out.Sensitivity = intl.SensitivityAccent
			case "case":
				out.Sensitivity = intl.SensitivityCase
			case "variant":
				out.Sensitivity = intl.SensitivityVariant
			default:
				return out, fmt.Errorf("sensitivity %v", value)
			}
		case "numeric":
			on, _ := value.(bool)
			out.Numeric = intl.Bool(on)
		case "caseFirst":
			switch value {
			case "upper":
				out.CaseFirst = intl.CaseFirstUpper
			case "lower":
				out.CaseFirst = intl.CaseFirstLower
			case "false":
				out.CaseFirst = intl.CaseFirstFalse
			default:
				return out, fmt.Errorf("caseFirst %v", value)
			}
		case "ignorePunctuation":
			on, _ := value.(bool)
			out.IgnorePunctuation = intl.Bool(on)
		case "usage":
			if value == "search" {
				out.Usage = intl.UsageSearch
			}
		case "collation":
			out.Collation, _ = value.(string)
		default:
			return out, fmt.Errorf("unknown option %q", key)
		}
	}
	return out, nil
}
