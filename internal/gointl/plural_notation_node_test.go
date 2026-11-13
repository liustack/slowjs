//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestPluralNotationMatchesNode holds Select to Node in every notation, where
// compact and scientific numbers carry a power of ten the rules count, and
// under directional rounding of negative numbers. The expectations are
// written by testdata/plural_notation_node.js.
func TestPluralNotationMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/plural_notation_node.txt.gz")
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
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 4 {
			t.Fatalf("a line that is not four fields: %s", line)
		}
		var tag, want string
		var opts map[string]any
		var n float64
		for i, dst := range []any{&tag, &opts, &n, &want} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: field %d: %v", line, i, err)
			}
		}
		ran++
		key := tag + string(fields[1])
		p, ok := rules[key]
		if !ok {
			o, err := pluralOptions(opts)
			if err != nil {
				t.Fatalf("%s: %v", key, err)
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
		if got := p.Select(n); string(got) == want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s %v: got %s, want %s", key, n, got, want))
		}
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

// pluralOptions reads the options the plural sweeps use.
func pluralOptions(opts map[string]any) (intl.PluralRulesOptions, error) {
	var o intl.PluralRulesOptions
	for k, v := range opts {
		switch k {
		case "type":
			if v == "ordinal" {
				o.Type = intl.Ordinal
			}
		case "notation":
			switch v {
			case "compact":
				o.Notation = intl.NotationCompact
			case "scientific":
				o.Notation = intl.NotationScientific
			case "engineering":
				o.Notation = intl.NotationEngineering
			}
		case "compactDisplay":
			if v == "long" {
				o.CompactDisplay = intl.CompactLong
			}
		case "maximumFractionDigits":
			n := int(v.(float64))
			o.MaximumFractionDigits = &n
		case "roundingMode":
			switch v {
			case "floor":
				o.RoundingMode = intl.Floor
			case "ceil":
				o.RoundingMode = intl.Ceil
			default:
				return o, fmt.Errorf("a rounding mode %v", v)
			}
		default:
			return o, fmt.Errorf("an option %q", k)
		}
	}
	return o, nil
}

// Compact and scientific numbers are selected as ICU's number formatter
// writes them, with the power of ten written apart: "1.5M" is many in French
// though 1500000 is other. ICU tries a language's rules in its own order,
// which makes 5E-1 many rather than one. A negative number is rounded with
// its sign, as a directional rounding mode needs.
func TestPluralSelectAsWritten(t *testing.T) {
	floor := 0
	for _, c := range []struct {
		tag  string
		opts intl.PluralRulesOptions
		n    float64
		want intl.PluralCategory
	}{
		{"fr", intl.PluralRulesOptions{}, 1500000, intl.PluralOther},
		{"fr", intl.PluralRulesOptions{Notation: intl.NotationCompact}, 1500000, intl.PluralMany},
		{"fr", intl.PluralRulesOptions{Notation: intl.NotationScientific}, 0.5, intl.PluralMany},
		{"af", intl.PluralRulesOptions{Notation: intl.NotationCompact}, 1000, intl.PluralOther},
		{"en", intl.PluralRulesOptions{MaximumFractionDigits: &floor, RoundingMode: intl.Floor}, -1.5, intl.PluralOther},
		{"en", intl.PluralRulesOptions{MaximumFractionDigits: &floor, RoundingMode: intl.Floor}, 1.5, intl.PluralOne},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		p, err := intl.NewPluralRules(loc, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Select(c.n); got != c.want {
			t.Errorf("%s %+v %v = %s, want %s", c.tag, c.opts, c.n, got, c.want)
		}
	}
}
