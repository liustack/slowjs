//go:build gointl_all

package intl_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestRoundingPriorityMatchesNode holds significant and fraction digits
// given together with a rounding priority to Node. The expectations are
// written by testdata/number_priority_node.js. The rounding the priority
// chose was padded with the significant digits' minimum whichever it was, so
// a fraction rounding lost the decimals minimumFractionDigits asked for:
// lessPrecision with two of each wrote 1 as "1.0", where ICU writes "1.00".
func TestRoundingPriorityMatchesNode(t *testing.T) {
	b, err := os.ReadFile("testdata/number_priority_node.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		ICU   string
		Cases [][3]json.RawMessage
	}
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	if file.ICU != "78.3" {
		t.Fatalf("the expectations come from ICU %s, not 78.3", file.ICU)
	}
	loc, _ := intl.ParseLocale("en-US")
	var differences []string
	for _, c := range file.Cases {
		var in map[string]any
		var x float64
		var want string
		for i, dst := range []any{&in, &x, &want} {
			if err := json.Unmarshal(c[i], dst); err != nil {
				t.Fatal(err)
			}
		}
		opts := intl.NumberFormatOptions{UseGrouping: intl.GroupingNever, Compat: intl.NodeICU}
		if in["roundingPriority"] == "lessPrecision" {
			opts.RoundingPriority = intl.LessPrecision
		} else {
			opts.RoundingPriority = intl.MorePrecision
		}
		count := func(key string) *int {
			if v, ok := in[key].(float64); ok {
				return intl.Digits(int(v))
			}
			return nil
		}
		opts.MinimumSignificantDigits = count("minimumSignificantDigits")
		opts.MaximumSignificantDigits = count("maximumSignificantDigits")
		opts.MinimumFractionDigits = count("minimumFractionDigits")
		opts.MaximumFractionDigits = count("maximumFractionDigits")
		f, err := intl.NewNumberFormat(loc, opts)
		if err != nil {
			differences = append(differences, fmt.Sprintf("%s: %v", c[0], err))
			continue
		}
		if got := f.Format(x); got != want {
			differences = append(differences, fmt.Sprintf("%s %v: %q, want %q", c[0], x, got, want))
		}
	}
	t.Logf("%d of %d match Node", len(file.Cases)-len(differences), len(file.Cases))
	if len(differences) > 0 {
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(differences[:min(25, len(differences))], "\n"))
	}
}
