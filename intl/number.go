// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// numberFormat is a resolved Intl.NumberFormat. Its json form is both what
// resolvedOptions() reports and the state handed back to the host on every
// format call, so the field order is the order ECMA-402's resolvedOptions table
// lists them in, which is also the order a browser reports them in. Keys
// starting with an underscore are internal and the shim strips them before
// handing the object to javascript.
type numberFormat struct {
	Locale                   string `json:"locale"`
	NumberingSystem          string `json:"numberingSystem"`
	Style                    string `json:"style"`
	Currency                 string `json:"currency,omitempty"`
	CurrencyDisplay          string `json:"currencyDisplay,omitempty"`
	CurrencySign             string `json:"currencySign,omitempty"`
	Unit                     string `json:"unit,omitempty"`
	UnitDisplay              string `json:"unitDisplay,omitempty"`
	MinimumIntegerDigits     int    `json:"minimumIntegerDigits"`
	MinimumFractionDigits    *int   `json:"minimumFractionDigits,omitempty"`
	MaximumFractionDigits    *int   `json:"maximumFractionDigits,omitempty"`
	MinimumSignificantDigits *int   `json:"minimumSignificantDigits,omitempty"`
	MaximumSignificantDigits *int   `json:"maximumSignificantDigits,omitempty"`
	UseGrouping              any    `json:"useGrouping"`
	Notation                 string `json:"notation"`
	CompactDisplay           string `json:"compactDisplay,omitempty"`
	SignDisplay              string `json:"signDisplay"`
	RoundingIncrement        int    `json:"roundingIncrement"`
	RoundingMode             string `json:"roundingMode"`
	RoundingPriority         string `json:"roundingPriority"`
	TrailingZeroDisplay      string `json:"trailingZeroDisplay"`

	// CompactRound records that no digit option was given for a compact
	// formatter, which selects compact notation's own rounding: an integer,
	// but never fewer than two significant digits, so 1234 is 1.2K and 12345
	// is 12K.
	CompactRound bool `json:"_compactRound,omitempty"`
	// DataLocale is the CLDR locale the formatter takes its symbols and
	// patterns from, and PluralLocale the one of its plural rules.
	DataLocale   string `json:"_data"`
	PluralLocale string `json:"_plural"`
	// LocaleCurrency is the currency of the region the locale names, which
	// can set the pattern and separators other currencies format with (see
	// init).
	LocaleCurrency string `json:"_localeCurrency,omitempty"`

	// The compiled form of the above, built by init.
	sys      *numberSystem
	main     *numPattern
	decimal  string // the decimal separator
	group    string // the grouping separator
	compact  *compactData
	plurals  *pluralRules
	currency string            // what ¤ stands for
	outer    map[string]string // plural category -> the unit or currency name pattern around the number, "{0} km"
	g1, g2   int               // grouping sizes, 0 for none
	minGroup int
}

func resolveNumberFormat(req request) (*numberFormat, error) {
	r, err := resolveLocale(req.Locales, numberService)
	if err != nil {
		return nil, err
	}

	o := req.Options
	if o == nil {
		o = map[string]any{}
	}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	nuOption, err := numberingSystemOption(o)
	if err != nil {
		return nil, err
	}

	f := &numberFormat{DataLocale: r.data.name, PluralLocale: pluralLocale(r.tag)}
	nu, inLocale := r.unicodeKeyword("nu", nuOption, supportedNumberingSystem)
	var keywords [][2]string
	if inLocale {
		keywords = append(keywords, [2]string{"nu", nu})
	}
	if nu == "" {
		nu = r.data.str("n/defaultNumberingSystem")
	}
	f.Locale, f.NumberingSystem = r.localeString(keywords), nu
	if f.Style, err = optDefault(o, "style", "decimal", "decimal", "percent", "currency", "unit"); err != nil {
		return nil, err
	}

	if f.Notation, err = optDefault(o, "notation", "standard", "standard", "scientific", "engineering", "compact"); err != nil {
		return nil, err
	}

	if f.SignDisplay, err = optDefault(o, "signDisplay", "auto", "auto", "never", "always", "exceptZero", "negative"); err != nil {
		return nil, err
	}

	if f.RoundingMode, err = optDefault(o, "roundingMode", "halfExpand", "ceil", "floor", "expand", "trunc",
		"halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven"); err != nil {
		return nil, err
	}

	if f.TrailingZeroDisplay, err = optDefault(o, "trailingZeroDisplay", "auto", "auto", "stripIfInteger"); err != nil {
		return nil, err
	}

	if err := roundingOptions(o); err != nil {
		return nil, err
	}

	// roundingIncrement and roundingPriority are reported at their defaults,
	// which is what this implementation does. A request for anything else
	// valid is ignored rather than rejected - see the package documentation.
	f.RoundingIncrement, f.RoundingPriority = 1, "auto"

	compactDisplay, err := optDefault(o, "compactDisplay", "short", "short", "long")
	if err != nil {
		return nil, err
	}

	if f.Notation == "compact" {
		f.CompactDisplay = compactDisplay
	}

	currencyDigits := 2
	switch f.Style {
	case "currency":
		s, ok := o["currency"].(string)
		if !ok {
			return nil, typeError("Currency code is required with currency style.")
		}

		if len(s) != 3 || !isAlpha(s) {
			return nil, rangeError("Invalid currency code : %s", s)
		}

		f.Currency = strings.ToUpper(s)
		if r.tag.region != "" {
			// Only a region the locale names: ICU infers none here, so tr
			// has no currency of its own where tr-TR has the lira.
			f.LocaleCurrency = cldr().supplemental().str("rc/" + r.tag.region)
		}
		currencyDigits = currencyFractionDigits(f.Currency)
		if f.CurrencyDisplay, err = optDefault(o, "currencyDisplay", "symbol", "code", "symbol", "narrowSymbol", "name"); err != nil {
			return nil, err
		}

		if f.CurrencySign, err = optDefault(o, "currencySign", "standard", "standard", "accounting"); err != nil {
			return nil, err
		}
	case "unit":
		s, ok := o["unit"].(string)
		if !ok {
			return nil, typeError("Unit is required with unit style.")
		}

		if !sanctionedUnit(s) {
			return nil, rangeError("Invalid unit argument for Intl.NumberFormat() '%s'", s)
		}

		f.Unit = s
		if f.UnitDisplay, err = optDefault(o, "unitDisplay", "short", "short", "narrow", "long"); err != nil {
			return nil, err
		}
	}

	if err := f.digitOptions(o, currencyDigits); err != nil {
		return nil, err
	}

	if f.UseGrouping, err = groupingOption(o, f.Notation); err != nil {
		return nil, err
	}

	return f, nil
}

// roundingIncrements are the values ECMA-402 allows roundingIncrement.
var roundingIncrements = []int{1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000}

// roundingOptions checks roundingIncrement and roundingPriority as
// SetNumberFormatDigitOptions does: a value out of range is a RangeError.
func roundingOptions(o map[string]any) error {
	n, ok, err := optInt(o, "roundingIncrement", 1, 5000)
	if err != nil {
		return err
	}

	if ok && !slices.Contains(roundingIncrements, n) {
		return rangeError("value %v out of range for Intl options property roundingIncrement", o["roundingIncrement"])
	}

	_, err = optString(o, "roundingPriority", "auto", "morePrecision", "lessPrecision")
	return err
}

// numberingSystemOption reads the numberingSystem option, which must be a
// well formed Unicode type even when it names a system nothing supports.
func numberingSystemOption(o map[string]any) (string, error) {
	v, ok := o["numberingSystem"]
	if !ok {
		return "", nil
	}

	s, _ := v.(string)
	for _, p := range strings.Split(s, "-") {
		if len(p) < 3 || len(p) > 8 || !isAlnum(p) {
			return "", rangeError("Invalid numberingSystem : %s", s)
		}
	}
	return strings.ToLower(s), nil
}

// supportedNumberingSystem reports whether nu is a decimal numbering system
// CLDR has the digits of.
func supportedNumberingSystem(nu string) bool {
	_, ok := supp().digits[nu]
	return ok
}

// sanctionedUnits are ECMA-402's simple units (IsSanctionedSingleUnitIdentifier).
var sanctionedUnits = strings.Fields(`acre bit byte celsius centimeter day degree fahrenheit fluid-ounce foot
	gallon gigabit gigabyte gram hectare hour inch kilobit kilobyte kilogram kilometer liter megabit
	megabyte meter microsecond mile mile-scandinavian milliliter millimeter millisecond minute month
	nanosecond ounce percent petabyte pound second stone terabit terabyte week yard year`)

// sanctionedUnit is ECMA-402's IsWellFormedUnitIdentifier: a simple unit or
// one simple unit per another.
func sanctionedUnit(u string) bool {
	if slices.Contains(sanctionedUnits, u) {
		return true
	}

	a, b, ok := strings.Cut(u, "-per-")
	return ok && slices.Contains(sanctionedUnits, a) && slices.Contains(sanctionedUnits, b)
}

// currencyFractionDigits returns the number of fraction digits CLDR writes a
// currency with, which is what a browser uses, and which is not always what
// ISO 4217 says: CLDR writes IQD with none where ISO gives it three.
func currencyFractionDigits(code string) int {
	s := cldr().supplemental()
	v, ok := s.get("cd/" + code)
	if !ok {
		v = s.str("cd/DEFAULT")
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 2
	}

	return n
}

// digitOptions implements ECMA-402's SetNumberFormatDigitOptions.
func (f *numberFormat) digitOptions(o map[string]any, currencyDigits int) error {
	mnid, ok, err := optInt(o, "minimumIntegerDigits", 1, 21)
	if err != nil {
		return err
	}

	f.MinimumIntegerDigits = 1
	if ok {
		f.MinimumIntegerDigits = mnid
	}

	// The fraction digits are checked only where they are used, which they
	// are not next to significant digits.
	_, hasMnfd := o["minimumFractionDigits"]
	_, hasMxfd := o["maximumFractionDigits"]

	mnsd, hasMnsd, err := optInt(o, "minimumSignificantDigits", 1, 21)
	if err != nil {
		return err
	}

	mxsd, hasMxsd, err := optInt(o, "maximumSignificantDigits", 1, 21)
	if err != nil {
		return err
	}

	switch {
	case hasMnsd || hasMxsd:
		if !hasMnsd {
			mnsd = 1
		}
		if !hasMxsd {
			mxsd = 21
		}
		if mnsd > mxsd {
			return rangeError("minimumSignificantDigits is greater than maximumSignificantDigits")
		}

		f.MinimumSignificantDigits, f.MaximumSignificantDigits = &mnsd, &mxsd
	case hasMnfd || hasMxfd || f.Notation != "compact":
		mnfd, _, err := optInt(o, "minimumFractionDigits", 0, 100)
		if err != nil {
			return err
		}
		mxfd, _, err := optInt(o, "maximumFractionDigits", 0, 100)
		if err != nil {
			return err
		}

		mnfdDefault, mxfdDefault := 0, 3
		switch {
		case f.Style == "currency" && f.Notation == "standard":
			mnfdDefault, mxfdDefault = currencyDigits, currencyDigits
		case f.Style == "percent":
			mxfdDefault = 0
		}
		// A given bound pulls the default of the other one to itself, and
		// only two given bounds can cross.
		switch {
		case !hasMnfd && !hasMxfd:
			mnfd, mxfd = mnfdDefault, mxfdDefault
		case !hasMnfd:
			mnfd = min(mnfdDefault, mxfd)
		case !hasMxfd:
			mxfd = max(mxfdDefault, mnfd)
		case mnfd > mxfd:
			return rangeError("minimumFractionDigits is greater than maximumFractionDigits")
		}

		f.MinimumFractionDigits, f.MaximumFractionDigits = &mnfd, &mxfd
	default:
		// Compact notation's own rounder, which ICU reports as one to two
		// significant digits and applies as "an integer, but never fewer than
		// two significant digits" - see compactRound.
		zero, one, two := 0, 1, 2
		f.MinimumFractionDigits, f.MaximumFractionDigits = &zero, &zero
		f.MinimumSignificantDigits, f.MaximumSignificantDigits = &one, &two
		f.RoundingPriority, f.CompactRound = "morePrecision", true
	}
	return nil
}

// groupingOption implements ECMA-402's GetBooleanOrStringNumberFormatOption for
// useGrouping, including its quirk of reading the strings "true" and "false" as
// the boolean false.
func groupingOption(o map[string]any, notation string) (any, error) {
	def := "auto"
	if notation == "compact" {
		def = "min2"
	}
	v, ok := o["useGrouping"]
	if !ok {
		return def, nil
	}

	switch x := v.(type) {
	case bool:
		if x {
			return "always", nil
		}

		return false, nil
	case string:
		switch x {
		case "min2", "auto", "always":
			return x, nil
		case "", "true", "false":
			return false, nil
		}
	case float64:
		if x == 0 {
			return false, nil
		}
	}
	return nil, rangeError("value %v out of range for Intl.NumberFormat options property useGrouping", v)
}

// optDefault is optString with a default for the absent option.
func optDefault(o map[string]any, key, def string, allowed ...string) (string, error) {
	s, err := optString(o, key, allowed...)
	if err != nil || s == "" {
		return def, err
	}

	return s, nil
}

// percentUnit reports whether the formatter writes a percent sign from the
// locale's percent pattern: style percent, and unit percent unless spelled
// out.
func (f *numberFormat) percentUnit() bool {
	return f.Style == "percent" || f.Style == "unit" && f.Unit == "percent" && f.UnitDisplay != "long" && f.Notation != "compact"
}

// init compiles the formatter's locale data, the patterns and names the
// format calls need. It runs once per distinct formatter in the process; the
// result is shared, read only, by every VM.
func (f *numberFormat) init() error {
	data := cldr().locale(f.DataLocale)
	if data == nil {
		return rangeError("internal error: no locale data for %s", f.DataLocale)
	}

	f.sys = numberSystemFor(data, f.NumberingSystem)
	f.plurals = pluralRulesFor(f.PluralLocale, false)
	switch {
	case f.percentUnit():
		f.main = f.sys.pattern("percentFormats", "standard")
	case f.Style == "currency" && f.CurrencyDisplay != "name" && f.CurrencySign == "accounting":
		f.main = f.sys.pattern("currencyFormats", "accounting")
	case f.Style == "currency" && f.CurrencyDisplay != "name":
		f.main = f.sys.pattern("currencyFormats", "standard")
	default:
		f.main = f.sys.pattern("decimalFormats", "standard")
	}
	f.decimal, f.group = f.sys.decimal, f.sys.group
	if f.Style == "currency" {
		f.decimal, f.group = f.sys.currencyDecimal, f.sys.currencyGroup
		// As in ICU, the pattern and separators in a currency's entry of the
		// locale data apply in every style and display. Those of the
		// locale's currency apply to the other currencies too, unless they
		// have their own: en-DE writes "-€1,234.50" and "-US$1,234.50"
		// where de writes "-1.234,50 €".
		for _, code := range []string{f.Currency, f.LocaleCurrency} {
			if p, ok := data.get("c/" + code + "/pattern"); ok {
				f.main = parseNumPattern(p)
				f.decimal, f.group = data.str("c/"+code+"/decimal"), data.str("c/"+code+"/group")
				break
			}
		}
	}
	switch f.UseGrouping {
	case false:
		// nop
	case "always":
		f.g1, f.g2, f.minGroup = f.main.g1, f.main.g2, 1
		if f.g1 == 0 {
			f.g1, f.g2 = 3, 3
		}
	case "min2":
		f.g1, f.g2, f.minGroup = f.main.g1, f.main.g2, max(2, f.sys.minGrouping)
	default:
		f.g1, f.g2, f.minGroup = f.main.g1, f.main.g2, f.sys.minGrouping
	}
	if f.Notation == "compact" {
		f.compact = f.sys.compactPatterns(f.Style == "currency" && f.CurrencyDisplay != "name", f.CompactDisplay)
	}
	if f.Style == "currency" {
		f.currency = currencySymbol(data, f.Currency, f.CurrencyDisplay)
		if f.CurrencyDisplay == "name" {
			f.outer = currencyNamePatterns(f.sys, data, f.Currency, f.plurals)
		}
	}
	if f.Style == "unit" && !f.percentUnit() {
		f.outer = unitPatterns(data, f.Unit, f.UnitDisplay, f.plurals)
	}
	return nil
}

// currencySymbol returns what a single ¤ stands for.
func currencySymbol(d *cldrData, code, display string) string {
	switch display {
	case "code":
		return code
	case "narrowSymbol":
		if s, ok := d.get("c/" + code + "/symbol-alt-narrow"); ok {
			return s
		}
	}
	if s, ok := d.get("c/" + code + "/symbol"); ok {
		return s
	}

	return code
}

// currencyNamePatterns returns the patterns writing an amount with the
// currency's name, "{0} euros", by plural category.
func currencyNamePatterns(sys *numberSystem, d *cldrData, code string, rules *pluralRules) map[string]string {
	r := map[string]string{}
	for _, cat := range rules.cats {
		p := sys.lookup("currencyFormats", "unitPattern-count-"+cat)
		if p == "" {
			p = sys.lookup("currencyFormats", "unitPattern-count-other")
		}
		name, ok := d.get("c/" + code + "/displayName-count-" + cat)
		if !ok {
			name, ok = d.get("c/" + code + "/displayName-count-other")
		}
		if !ok {
			name, ok = d.get("c/" + code + "/displayName")
		}
		if !ok {
			name = code
		}
		r[cat] = strings.Replace(p, "{1}", name, 1)
	}
	return r
}

// unitPatterns returns the patterns writing an amount of unit, "{0} km", by
// plural category, following ICU's LongNameHandler: a unit CLDR has data for
// uses it, and a compound one without data is built from its parts.
func unitPatterns(d *cldrData, unit, width string, rules *pluralRules) map[string]string {
	if r := unitData(d, unit, width, rules); r != nil {
		return r
	}

	num, den, _ := strings.Cut(unit, "-per-")
	lead := unitData(d, num, width, rules)
	trail := ""
	for _, w := range widthFallback(width) {
		if p, ok := d.get("u/" + w + "/" + den + "/perUnitPattern"); ok {
			trail = p
			break
		}
	}
	if trail == "" {
		per := ""
		for _, w := range widthFallback(width) {
			if p, ok := d.get("u/" + w + "/per"); ok {
				per = p
				break
			}
		}
		denOne := unitData(d, den, width, rules)["one"]
		if denOne == "" {
			denOne = unitData(d, den, width, rules)["other"]
		}
		denName := strings.TrimFunc(strings.Replace(denOne, "{0}", "", 1), ignorable)
		trail = strings.Replace(per, "{1}", denName, 1)
	}
	r := map[string]string{}
	for cat, p := range lead {
		r[cat] = strings.Replace(trail, "{0}", p, 1)
	}
	return r
}

// widthFallback is the order unit widths are looked up in.
func widthFallback(width string) []string {
	switch width {
	case "narrow":
		return []string{"narrow", "short", "long"}
	case "long":
		return []string{"long", "short"}
	}
	return []string{"short", "long"}
}

// unitData returns the unit patterns CLDR has for unit at width by plural
// category, or nil if it has none.
func unitData(d *cldrData, unit, width string, rules *pluralRules) map[string]string {
	for _, w := range widthFallback(width) {
		prefix := "u/" + w + "/" + unit + "/unitPattern-count-"
		other, ok := d.get(prefix + "other")
		if !ok {
			continue
		}

		r := map[string]string{}
		for _, cat := range rules.cats {
			if p, ok := d.get(prefix + cat); ok {
				r[cat] = p
			} else {
				r[cat] = other
			}
		}
		r["other"] = other
		return r
	}
	return nil
}

// format renders v, which is whatever the shim's Number() coercion produced, as
// ECMA-402 parts. Joining their values yields the format() result.
func (f *numberFormat) format(v any) ([]part, error) {
	d, special := numberValue(v)
	if special == "" && f.Style == "percent" {
		d = d.scale(2)
	}

	var (
		num      []span
		r        dec
		minFrac  int
		zero     bool
		neg      = d.neg || special == "-Infinity"
		pattern  = f.main
		field    = ""
		outerCat = "other"
	)
	switch special {
	case "NaN":
		num, zero, neg = []span{{"nan", f.sys.nan}}, true, false
	case "Infinity", "-Infinity":
		num = []span{{"infinity", f.sys.infinity}}
	default:
		exponent := 0
		switch f.Notation {
		case "scientific", "engineering":
			r, minFrac, exponent = f.roundScientific(d)
		case "compact":
			var cp *numPattern
			r, minFrac, outerCat, cp = f.roundCompact(d)
			if cp != nil {
				pattern, field = cp, "compact"
			}
		default:
			r, minFrac = f.round(d)
			outerCat = f.pluralOf(r, minFrac)
		}
		zero = r.isZero()
		if !pattern.noDigits {
			num = f.digits(r, minFrac)
		}
		if f.Notation == "scientific" || f.Notation == "engineering" {
			num = append(num, span{"exponentSeparator", f.sys.exponential})
			if exponent < 0 {
				num = append(num, span{"exponentMinusSign", f.sys.minus})
				exponent = -exponent
			}
			num = append(num, span{"exponentInteger", f.sys.transliterate(strconv.Itoa(exponent))})
		}
	}

	signType := signPos
	switch f.SignDisplay {
	case "auto":
		if neg {
			signType = signNeg
		}
	case "always":
		if signType = signPosSign; neg {
			signType = signNeg
		}
	case "exceptZero":
		switch {
		case zero:
			// nop
		case neg:
			signType = signNeg
		default:
			signType = signPosSign
		}
	case "negative":
		if neg && !zero {
			signType = signNeg
		}
	}

	prefix := f.affix(pattern.affixFor(false, signType), field)
	suffix := f.affix(pattern.affixFor(true, signType), field)
	// Currency spacing: a no-break space between a currency symbol ending or
	// starting with a letter and the digits next to it.
	if n := len(prefix); n != 0 && len(num) != 0 && prefix[n-1].Type == "currency" {
		last, _ := utf8.DecodeLastRuneInString(prefix[n-1].Value)
		first, _ := utf8.DecodeRuneInString(num[0].Value)
		if currencySpaced(last, first) {
			prefix = append(prefix, span{"", " "})
		}
	}
	if len(suffix) != 0 && len(num) != 0 && suffix[0].Type == "currency" {
		first, _ := utf8.DecodeRuneInString(suffix[0].Value)
		last, _ := utf8.DecodeLastRuneInString(num[len(num)-1].Value)
		if currencySpaced(first, last) {
			suffix = append([]span{{"", " "}}, suffix...)
		}
	}
	out := append(append(prefix, num...), suffix...)
	if f.outer != nil {
		p, ok := f.outer[outerCat]
		if !ok {
			p = f.outer["other"]
		}
		typ := "unit"
		if f.Style == "currency" {
			typ = "currency"
		}
		if before, after, ok := strings.Cut(p, "{0}"); ok {
			out = append(append([]span{{typ, before}}, out...), span{typ, after})
		} else {
			// Some languages spell out the amount, as Arabic does for one or
			// two days: the pattern is the whole text.
			out = []span{{typ, p}}
		}
	}
	return toParts(out), nil
}

// affix renders pattern tokens; literal text belongs to field.
func (f *numberFormat) affix(toks []affixTok, field string) []span {
	var r []span
	for _, t := range toks {
		switch t.kind {
		case 0:
			r = append(r, span{field, t.text})
		case '-':
			r = append(r, span{"minusSign", f.sys.minus})
		case '+':
			r = append(r, span{"plusSign", f.sys.plus})
		case '%':
			typ := "percentSign"
			if f.Style == "unit" {
				typ = "unit"
			}
			r = append(r, span{typ, f.sys.percent})
		case 'm':
			r = append(r, span{"perMille", "‰"})
		case 'c':
			switch t.n {
			case 1:
				r = append(r, span{"currency", f.currency})
			case 2:
				r = append(r, span{"currency", f.Currency})
			default:
				r = append(r, span{"currency", "�"})
			}
		}
	}
	return r
}

// round applies the formatter's rounding to d and returns the result with
// the number of fraction digits to show at least.
func (f *numberFormat) round(d dec) (dec, int) {
	minFrac := 0
	switch {
	case f.CompactRound:
		d = compactRound(d, f.RoundingMode)
	case f.MaximumSignificantDigits != nil:
		d = d.roundTo(*f.MaximumSignificantDigits, f.RoundingMode)
		// Zero has no significant digit of its own, but the "0" written for it
		// counts as the first one, so minimumSignificantDigits 3 is "0.00".
		digits := d.exp
		if d.isZero() {
			digits = 1
		}
		if minFrac = *f.MinimumSignificantDigits - digits; minFrac < 0 {
			minFrac = 0
		}
	default:
		d = d.roundFrac(*f.MaximumFractionDigits, f.RoundingMode)
		minFrac = *f.MinimumFractionDigits
	}
	if f.TrailingZeroDisplay == "stripIfInteger" && !d.hasFraction() {
		minFrac = 0
	}
	return d, minFrac
}

// roundScientific scales d to its mantissa and rounds it.
func (f *numberFormat) roundScientific(d dec) (dec, int, int) {
	exponent := d.magnitude()
	if f.Notation == "engineering" {
		exponent -= ((exponent % 3) + 3) % 3
	}
	d, minFrac := f.round(d.scale(-exponent))
	// Rounding can carry the mantissa out of its notation's range: 999999 in
	// scientific notation rounds to 10E5, which is 1E6.
	switch mantissa := d.intDigits(); {
	case f.Notation == "scientific" && mantissa > 1:
		d, exponent = d.scale(1-mantissa), exponent+mantissa-1
	case f.Notation == "engineering" && mantissa > 3:
		d, exponent = d.scale(-3), exponent+3
	}
	return d, minFrac, exponent
}

// roundCompact scales d by the power of ten its compact pattern divides by and
// rounds it, as ICU's CompactHandler does. It returns the plural category of
// a unit or currency name, which ICU picks before scaling, and the compact
// pattern, nil for the standard one.
func (f *numberFormat) roundCompact(d dec) (dec, int, string, *numPattern) {
	unscaled, unscaledFrac := f.round(d)
	outerCat := f.pluralOf(unscaled, unscaledFrac)
	r, minFrac, mult := f.compactScale(d)
	magnitude := -mult
	if !r.isZero() {
		magnitude += r.magnitude()
	}
	return r, minFrac, outerCat, f.compactPattern(magnitude, r, minFrac)
}

// compactScale returns d scaled for compact notation and rounded, and the
// power of ten it was scaled by, zero or negative.
func (f *numberFormat) compactScale(d dec) (dec, int, int) {
	if d.isZero() {
		r, minFrac := f.round(d)
		return r, minFrac, 0
	}

	m := d.magnitude()
	mult := f.compact.multiplier(m)
	r, minFrac := f.round(d.scale(mult))
	if !r.isZero() && r.magnitude() != m+mult {
		if mult2 := f.compact.multiplier(m + 1); mult2 != mult {
			r, minFrac = f.round(r.scale(mult2 - mult))
			mult = mult2
		}
	}
	return r, minFrac, mult
}

// pluralOperands returns the plural operands of d as the formatter would
// show it, sign aside. In compact and scientific notation they are those of
// the whole number the digits shown stand for, as ICU has them: "1,5 M" has
// an i of 1500000, a v of 0 and an e of 6, "1.5E-4" an i of 0, an f of 15, a
// v of 5 and an e of -4.
func (f *numberFormat) pluralOperands(d dec) pluralOperands {
	d.neg = false
	var (
		r       dec
		minFrac int
		e       int
	)
	switch f.Notation {
	case "scientific", "engineering":
		r, minFrac, e = f.roundScientific(d)
	case "compact":
		var mult int
		r, minFrac, mult = f.compactScale(d)
		e = -mult
	default:
		r, minFrac = f.round(d)
	}
	ip, fp := r.render(1, minFrac)
	ip, fp = shiftPoint(ip, fp, e)
	return operandsOf(ip, fp, e)
}

// shiftPoint moves the decimal point of ip.fp e places to the right, or to
// the left for a negative e, keeping every digit: 1.50 by 6 is 1500000, 1.5 by
// -4 is 0.00015.
func shiftPoint(ip, fp string, e int) (string, string) {
	switch {
	case e > 0:
		n := min(e, len(fp))
		ip, fp = ip+fp[:n], fp[n:]
		ip += strings.Repeat("0", e-n)
	case e < 0:
		n := min(-e, len(ip))
		ip, fp = ip[:len(ip)-n], ip[len(ip)-n:]+fp
		fp = strings.Repeat("0", -e-n) + fp
	}
	return ip, fp
}

func (f *numberFormat) compactPattern(magnitude int, r dec, minFrac int) *numPattern {
	// ICU matches the exact patterns against the signed integer value, so
	// -1 takes its plural category's pattern and -0 the one for 0.
	integer := int64(-1)
	if !r.hasFraction() && r.intDigits() == 1 {
		ip, _ := r.render(1, 0)
		if integer = int64(ip[0] - '0'); r.neg && integer != 0 {
			integer = -1
		}
	}
	return f.compact.pattern(magnitude, integer, f.pluralOf(r, minFrac))
}

// pluralOf returns the plural category of the rounded r shown with at least
// minFrac fraction digits.
func (f *numberFormat) pluralOf(r dec, minFrac int) string {
	ip, fp := r.render(1, minFrac)
	return f.plurals.selectCategory(operandsOf(ip, fp, 0))
}

// digits renders the rounded r as integer, group, decimal and fraction spans.
func (f *numberFormat) digits(r dec, minFrac int) []span {
	ip, fp := r.render(f.MinimumIntegerDigits, minFrac)
	group, decimal := f.group, f.decimal
	var s []span
	for i, g := range groupDigits(ip, f.g1, f.g2, f.minGroup) {
		if i != 0 {
			s = append(s, span{"group", group})
		}
		s = append(s, span{"integer", f.sys.transliterate(g)})
	}
	if fp != "" {
		s = append(s, span{"decimal", decimal}, span{"fraction", f.sys.transliterate(fp)})
	}
	return s
}

// groupDigits splits the integer digits ip into groups, the last one g1
// digits long and the others g2, as ICU's Grouper does: there are no groups
// unless at least minGroup digits precede the last one.
func groupDigits(ip string, g1, g2, minGroup int) []string {
	n := len(ip)
	if g1 <= 0 || n-g1 < minGroup {
		return []string{ip}
	}

	var r []string
	end := n - g1
	r = append(r, ip[end:])
	for end > 0 {
		start := max(end-g2, 0)
		r = append(r, ip[start:end])
		end = start
	}
	slices.Reverse(r)
	return r
}

// numberValue converts a value arriving from javascript to a dec. QuickJS tags
// integers that fit as such, so those never take the float64 detour.
func numberValue(v any) (dec, string) {
	var f float64
	switch x := v.(type) {
	case int:
		return decFromInt(int64(x)), ""
	case int64:
		return decFromInt(x), ""
	case float64:
		f = x
	case string:
		// A string arrives from a BigInt or from format("123.45"). Parsing its
		// digits keeps it exact, which a detour through a float64 would not do
		// for a BigInt of more than 17 of them.
		d, ok := decFromDecimalString(strings.TrimSpace(x))
		if !ok {
			return dec{}, "NaN"
		}

		return d, ""
	case bool:
		if x {
			return decFromInt(1), ""
		}

		return dec{}, ""
	case nil:
		return dec{}, ""
	default:
		return dec{}, "NaN"
	}
	switch {
	case math.IsNaN(f):
		return dec{}, "NaN"
	case math.IsInf(f, 1):
		return dec{}, "Infinity"
	case math.IsInf(f, -1):
		return dec{}, "-Infinity"
	}
	return decFromFloat(f), ""
}

// compactRound rounds to an integer but keeps two significant digits, which is
// what compact notation does when no digit option overrides it.
func compactRound(d dec, mode string) dec {
	return d.roundTo(max(d.exp, 2), mode)
}
