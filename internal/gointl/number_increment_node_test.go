//go:build gointl_all

package intl_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestRoundingIncrementMatchesNode holds rounding to an increment to Node,
// with every rounding mode. The expectations are written by
// testdata/number_increment_node.js. Rounding to the decimals first and to
// the increment after rounded twice: 1.25 to the nearest 0.2 was 1.4.
func TestRoundingIncrementMatchesNode(t *testing.T) {
	b, err := os.ReadFile("testdata/number_increment_node.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		ICU   string
		Cases [][5]any
	}
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	if file.ICU != "78.3" {
		t.Fatalf("the expectations come from ICU %s, not 78.3", file.ICU)
	}
	modes := map[string]intl.RoundingMode{"ceil": intl.Ceil, "floor": intl.Floor, "expand": intl.Expand,
		"trunc": intl.Trunc, "halfCeil": intl.HalfCeil, "halfFloor": intl.HalfFloor,
		"halfExpand": intl.HalfExpand, "halfTrunc": intl.HalfTrunc, "halfEven": intl.HalfEven}
	loc, _ := intl.ParseLocale("en-US")
	var differences []string
	for _, c := range file.Cases {
		inc, frac := int(c[0].(float64)), int(c[1].(float64))
		mode, x, want := c[2].(string), c[3].(float64), c[4].(string)
		f, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{RoundingIncrement: inc,
			MinimumFractionDigits: intl.Digits(frac), MaximumFractionDigits: intl.Digits(frac),
			RoundingMode: modes[mode], UseGrouping: intl.GroupingNever, Compat: intl.NodeICU})
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Format(x); got != want {
			differences = append(differences, fmt.Sprintf("%v at %d decimals by %d, %s: %q, want %q",
				x, frac, inc, mode, got, want))
		}
	}
	t.Logf("%d of %d match Node", len(file.Cases)-len(differences), len(file.Cases))
	if len(differences) > 0 {
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(differences[:min(25, len(differences))], "\n"))
	}
}

// Negative zero from a float is written with one minus sign, as Node writes
// (-0).toLocaleString(); a float's digits had kept their own.
func TestNegativeZeroFromFloat(t *testing.T) {
	loc, _ := intl.ParseLocale("en-US")
	f, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	negZero := math.Copysign(0, -1)
	for _, got := range []string{f.Format(negZero), f.FormatDecimal(intl.DecimalFromFloat(negZero)),
		f.FormatDecimal(intl.ParseDecimal("-0"))} {
		if got != "-0" {
			t.Errorf("negative zero is %q, want %q", got, "-0")
		}
	}
}
