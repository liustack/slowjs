//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestPluralRangeMatchesNode holds SelectRange to what Node's selectRange
// answers, in every locale it supports. The expectations are written by
// testdata/plural_range_node.js.
func TestPluralRangeMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/plural_range_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	rules := map[string]*intl.PluralRules{}
	var ran, matched int
	var differences []string
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 5 {
			t.Fatalf("a line that is not five fields: %s", line)
		}
		var tag, want string
		var opts map[string]any
		var from, to float64
		for i, dst := range []any{&tag, &opts, &from, &to, &want} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: field %d: %v", line, i, err)
			}
		}
		ran++
		key := tag + string(fields[1])
		p, ok := rules[key]
		if !ok {
			var o intl.PluralRulesOptions
			if opts["type"] == "ordinal" {
				o.Type = intl.Ordinal
			}
			if v, ok := opts["minimumFractionDigits"].(float64); ok {
				n := int(v)
				o.MinimumFractionDigits = &n
			}
			if v, ok := opts["maximumFractionDigits"].(float64); ok {
				n := int(v)
				o.MaximumFractionDigits = &n
			}
			loc, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}
			if p, err = intl.NewPluralRules(loc, o); err != nil {
				t.Fatalf("%s: %v", key, err)
			}
			rules[key] = p
		}
		got, err := p.SelectRange(from, to)
		if err != nil {
			t.Fatalf("%s %v %v: %v", key, from, to, err)
		}
		if string(got) == want {
			matched++
			continue
		}
		differences = append(differences, fmt.Sprintf("%s %v–%v: got %s, want %s", key, from, to, got, want))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node exactly", matched, ran)
	if len(differences) > 0 {
		sort.Strings(differences)
		if len(differences) > 25 {
			differences = append(differences[:25], "...")
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(differences, "\n"))
	}
}

// selectRange with an end that is not a number throws in ECMA-402.
func TestPluralRangeNaN(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	p, err := intl.NewPluralRules(loc, intl.PluralRulesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.SelectRange(math.NaN(), 1); err == nil {
		t.Error("a NaN start is accepted")
	}
	if got, err := p.SelectRange(1, 2); err != nil || got != intl.PluralOther {
		t.Errorf("en 1–2 = %v, %v; want other", got, err)
	}
}
