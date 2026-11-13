//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// names maps each JavaScript spelling of an option type's values to the
// value, from String: the first n values of the type.
func names[T interface {
	~int
	String() string
}](n int) map[string]T {
	m := make(map[string]T, n)
	for i := 0; i < n; i++ {
		m[T(i).String()] = T(i)
	}
	return m
}

var (
	styleNames        = names[intl.Style](4)
	currencyDisplays  = names[intl.CurrencyDisplay](4)
	currencySigns     = names[intl.CurrencySign](2)
	unitDisplays      = names[intl.UnitDisplay](3)
	notationNames     = names[intl.Notation](4)
	compactDisplays   = names[intl.CompactDisplay](2)
	signDisplays      = names[intl.SignDisplay](5)
	groupingNames     = names[intl.Grouping](5)
	roundingModes     = names[intl.RoundingMode](9)
	roundingPriority  = names[intl.RoundingPriority](3)
	trailingZeroNames = names[intl.TrailingZeroDisplay](2)
	pluralTypes       = names[intl.PluralType](2)
)

// jsDigits reads the digit and rounding options of a JavaScript option
// bag.
func jsDigits(o map[string]any) (minInt int, minFrac, maxFrac, minSig, maxSig *int,
	priority intl.RoundingPriority, mode intl.RoundingMode, increment int, zero intl.TrailingZeroDisplay) {
	num := func(k string) *int {
		if v, ok := o[k].(float64); ok {
			return intl.Digits(int(v))
		}
		return nil
	}
	if v := num("minimumIntegerDigits"); v != nil {
		minInt = *v
	}
	if v := num("roundingIncrement"); v != nil {
		increment = *v
	}
	s := func(k string) string { v, _ := o[k].(string); return v }
	return minInt, num("minimumFractionDigits"), num("maximumFractionDigits"),
		num("minimumSignificantDigits"), num("maximumSignificantDigits"),
		roundingPriority[s("roundingPriority")], roundingModes[s("roundingMode")], increment,
		trailingZeroNames[s("trailingZeroDisplay")]
}

// jsResolvedDigits writes the digit and rounding options as resolvedOptions
// does, leaving out what is nil.
func jsResolvedDigits(out map[string]any, d intl.ResolvedDigits) {
	out["minimumIntegerDigits"] = float64(d.MinimumIntegerDigits)
	for k, v := range map[string]*int{
		"minimumFractionDigits": d.MinimumFractionDigits, "maximumFractionDigits": d.MaximumFractionDigits,
		"minimumSignificantDigits": d.MinimumSignificantDigits, "maximumSignificantDigits": d.MaximumSignificantDigits,
	} {
		if v != nil {
			out[k] = float64(*v)
		}
	}
	out["roundingIncrement"] = float64(d.RoundingIncrement)
	out["roundingMode"] = d.RoundingMode.String()
	out["roundingPriority"] = d.RoundingPriority.String()
	out["trailingZeroDisplay"] = d.TrailingZeroDisplay.String()
}

// TestResolvedNumberOptionsMatchNode holds NumberFormat's and PluralRules'
// resolved options to what Node's resolvedOptions reports, for the option
// bags testdata/number_resolved_node.js tries; PluralRules under NodeICU,
// for PluralRulesDigits.
func TestResolvedNumberOptionsMatchNode(t *testing.T) {
	f, err := os.Open("testdata/number_resolved_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(z)
	cases, bad := 0, 0
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "#") {
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil || len(c) != 4 {
			t.Fatalf("%s: %v", sc.Text(), err)
		}
		var service, tag string
		var o, want map[string]any
		json.Unmarshal(c[0], &service)
		json.Unmarshal(c[1], &tag)
		json.Unmarshal(c[2], &o)
		json.Unmarshal(c[3], &want)
		cases++
		loc, err := intl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		s := func(k string) string { v, _ := o[k].(string); return v }
		minInt, minFrac, maxFrac, minSig, maxSig, priority, mode, increment, zero := jsDigits(o)

		got := map[string]any{}
		switch service {
		case "NumberFormat":
			opts := intl.NumberFormatOptions{
				Style: styleNames[s("style")], Currency: s("currency"),
				CurrencyDisplay: currencyDisplays[s("currencyDisplay")],
				CurrencySign:    currencySigns[s("currencySign")],
				Unit:            s("unit"), UnitDisplay: unitDisplays[s("unitDisplay")],
				Notation:             notationNames[s("notation")],
				CompactDisplay:       compactDisplays[s("compactDisplay")],
				SignDisplay:          signDisplays[s("signDisplay")],
				UseGrouping:          groupingNames[s("useGrouping")],
				MinimumIntegerDigits: minInt, MinimumFractionDigits: minFrac,
				MaximumFractionDigits: maxFrac, MinimumSignificantDigits: minSig,
				MaximumSignificantDigits: maxSig, RoundingPriority: priority,
				RoundingMode: mode, RoundingIncrement: increment, TrailingZeroDisplay: zero,
			}
			if g, ok := o["useGrouping"].(bool); ok && !g {
				opts.UseGrouping = intl.GroupingNever
			}
			nf, err := intl.NewNumberFormat(loc, opts)
			if err != nil {
				got["error"] = "RangeError"
				break
			}
			r := nf.ResolvedOptions()
			got["locale"], got["numberingSystem"], got["style"] = r.Locale, r.NumberingSystem, r.Style.String()
			if r.Style == intl.StyleCurrency {
				got["currency"], got["currencyDisplay"] = r.Currency, r.CurrencyDisplay.String()
				got["currencySign"] = r.CurrencySign.String()
			}
			if r.Style == intl.StyleUnit {
				got["unit"], got["unitDisplay"] = r.Unit, r.UnitDisplay.String()
			}
			jsResolvedDigits(got, r.ResolvedDigits)
			if r.UseGrouping == intl.GroupingNever {
				got["useGrouping"] = false
			} else {
				got["useGrouping"] = r.UseGrouping.String()
			}
			got["notation"] = r.Notation.String()
			if r.Notation == intl.NotationCompact {
				got["compactDisplay"] = r.CompactDisplay.String()
			}
			got["signDisplay"] = r.SignDisplay.String()
		case "PluralRules":
			opts := intl.PluralRulesOptions{
				Type: pluralTypes[s("type")], Notation: notationNames[s("notation")],
				CompactDisplay:       compactDisplays[s("compactDisplay")],
				MinimumIntegerDigits: minInt, MinimumFractionDigits: minFrac,
				MaximumFractionDigits: maxFrac, MinimumSignificantDigits: minSig,
				MaximumSignificantDigits: maxSig, RoundingPriority: priority,
				RoundingMode: mode, RoundingIncrement: increment, TrailingZeroDisplay: zero,
				Compat: intl.NodeICU,
			}
			pr, err := intl.NewPluralRules(loc, opts)
			if err != nil {
				got["error"] = "RangeError"
				break
			}
			r := pr.ResolvedOptions()
			got["locale"], got["type"], got["notation"] = r.Locale, r.Type.String(), r.Notation.String()
			if r.Notation == intl.NotationCompact {
				got["compactDisplay"] = r.CompactDisplay.String()
			}
			jsResolvedDigits(got, r.ResolvedDigits)
		}
		if !reflect.DeepEqual(got, want) {
			if bad++; bad <= 20 {
				g, _ := json.Marshal(got)
				t.Errorf("%s %s %s\n got  %s\n want %s", service, tag, c[2], g, c[3])
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if bad > 0 {
		t.Errorf("%d of %d differ", bad, cases)
	}
	t.Logf("%d cases", cases)
}

// Where a roundingPriority is given, or compact notation asks for no digits,
// both kinds of digits are settled on. The standard's resolvedOptions
// reports both; V8's PluralRules reports the significant digits alone
// (PluralRulesDigits).
func TestPluralRulesDigits(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	type span struct{ min, max int }
	for _, c := range []struct {
		opts intl.PluralRulesOptions
		// frac and sig are what the standard reports, nil for not at all;
		// under PluralRulesDigits, frac is left out wherever sig is there.
		frac, sig *span
	}{
		{intl.PluralRulesOptions{RoundingPriority: intl.MorePrecision}, &span{0, 3}, &span{1, 21}},
		{intl.PluralRulesOptions{Notation: intl.NotationCompact}, &span{0, 0}, &span{1, 2}},
		{intl.PluralRulesOptions{MaximumFractionDigits: intl.Digits(1)}, &span{0, 1}, nil},
		{intl.PluralRulesOptions{MaximumSignificantDigits: intl.Digits(3)}, nil, &span{1, 3}},
	} {
		for _, compat := range []intl.Compat{intl.Standard, intl.PluralRulesDigits} {
			o := c.opts
			o.Compat = compat
			pr, err := intl.NewPluralRules(loc, o)
			if err != nil {
				t.Fatal(err)
			}
			r := pr.ResolvedOptions()
			frac := c.frac
			if compat == intl.PluralRulesDigits && c.sig != nil {
				frac = nil
			}
			check := func(what string, want *span, min, max *int) {
				switch {
				case want == nil && min != nil:
					t.Errorf("%+v %v: %s %d to %d, want none", c.opts, compat, what, *min, *max)
				case want != nil && min == nil:
					t.Errorf("%+v %v: no %s, want %v", c.opts, compat, what, *want)
				case want != nil && (*min != want.min || *max != want.max):
					t.Errorf("%+v %v: %s %d to %d, want %v", c.opts, compat, what, *min, *max, *want)
				}
			}
			check("decimals", frac, r.MinimumFractionDigits, r.MaximumFractionDigits)
			check("significant digits", c.sig, r.MinimumSignificantDigits, r.MaximumSignificantDigits)
		}
	}
}

// A currency's own number of decimals is the default only in standard
// notation (InitializeNumberFormat), and a maximum given alone brings the
// minimum down to it (SetNumberFormatDigitOptions). Node's answers; go-intl
// had written "$1.23K" and "$1.23E3", and refused the last.
func TestCurrencyDigitDefaults(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		opts intl.NumberFormatOptions
		want string
	}{
		{intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact,
			MinimumFractionDigits: intl.Digits(1)}, "$1.235K"},
		{intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationScientific},
			"$1.235E3"},
		{intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "JPY", Notation: intl.NotationCompact},
			"¥1.2K"},
		{intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD",
			MaximumFractionDigits: intl.Digits(0)}, "$1,235"},
	} {
		nf, err := intl.NewNumberFormat(loc, c.opts)
		if err != nil {
			t.Errorf("%+v: %v", c.opts, err)
			continue
		}
		if got := nf.Format(1234.5678); got != c.want {
			t.Errorf("%+v: %q, want %q", c.opts, got, c.want)
		}
	}
}
