// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strconv"
	"strings"
	"sync"
)

// pluralCategories is CLDR's order of the plural categories, the one
// resolvedOptions().pluralCategories reports them in.
var pluralCategories = [...]string{"zero", "one", "two", "few", "many", "other"}

// pluralRules is the compiled rule set of one language, cardinal or ordinal.
type pluralRules struct {
	rules []pluralRule // in pluralCategories order, without "other"
	cats  []string     // the categories the language uses, "other" included
}

type pluralRule struct {
	category string
	or       [][]pluralRelation // a disjunction of conjunctions
}

// pluralRelation is "operand [% mod] (=|!=) ranges".
type pluralRelation struct {
	operand byte
	mod     int64
	negate  bool
	ranges  [][2]int64
}

var pluralCache sync.Map // "pc/locale" or "po/locale" -> *pluralRules

// pluralLocale returns the locale whose plural rules t uses: t or the
// nearest truncation of it CLDR has rules for, "" for none.
func pluralLocale(t *langTag) string {
	d := cldr().supplemental()
	for l := t.base(); l != ""; l = truncate(l) {
		if _, ok := d.get("pc/" + l + "/other"); ok {
			return l
		}
	}
	return ""
}

// truncate drops the last subtag of a locale without extensions, "" when
// there is none left to drop.
func truncate(l string) string {
	i := strings.LastIndexByte(l, '-')
	if i < 0 {
		return ""
	}

	return l[:i]
}

// pluralRulesFor returns the cardinal or ordinal rules of locale, or of the
// nearest truncation of it that has rules of that kind, falling back to the
// root rules (everything is "other").
func pluralRulesFor(locale string, ordinal bool) *pluralRules {
	kind := "pc/"
	if ordinal {
		kind = "po/"
	}
	key := kind + locale
	if r, ok := pluralCache.Load(key); ok {
		return r.(*pluralRules)
	}

	d := cldr().supplemental()
	l := locale
	for ; l != ""; l = truncate(l) {
		if _, ok := d.get(kind + l + "/other"); ok {
			break
		}
	}
	r := &pluralRules{}
	for _, cat := range pluralCategories {
		src, ok := d.get(kind + l + "/" + cat)
		if !ok || l == "" {
			continue
		}

		r.cats = append(r.cats, cat)
		if cat != "other" {
			r.rules = append(r.rules, parsePluralRule(cat, src))
		}
	}
	if len(r.cats) == 0 {
		r.cats = []string{"other"}
	}
	v, _ := pluralCache.LoadOrStore(key, r)
	return v.(*pluralRules)
}

// parsePluralRule compiles CLDR's rule syntax, as in
// "v = 0 and i % 10 = 1 and i % 100 != 11 @integer 1, 21, ...". The samples
// after the first '@' are ignored. The rules are generated data, so a rule
// that does not parse is a generator bug: it panics.
func parsePluralRule(cat, src string) pluralRule {
	if i := strings.IndexByte(src, '@'); i >= 0 {
		src = src[:i]
	}
	r := pluralRule{category: cat}
	for _, or := range strings.Split(src, " or ") {
		var and []pluralRelation
		for _, rel := range strings.Split(or, " and ") {
			and = append(and, parsePluralRelation(strings.TrimSpace(rel)))
		}
		r.or = append(r.or, and)
	}
	return r
}

func parsePluralRelation(s string) (r pluralRelation) {
	op, rhs, ok := strings.Cut(s, "!=")
	if ok {
		r.negate = true
	} else if op, rhs, ok = strings.Cut(s, "="); !ok {
		panic("intl: bad plural rule: " + s)
	}

	op = strings.TrimSpace(op)
	if operand, mod, ok := strings.Cut(op, "%"); ok {
		op = strings.TrimSpace(operand)
		r.mod = mustInt(strings.TrimSpace(mod))
	}
	if len(op) != 1 {
		panic("intl: bad plural operand: " + s)
	}

	r.operand = op[0]
	for _, rng := range strings.Split(rhs, ",") {
		lo, hi, ok := strings.Cut(strings.TrimSpace(rng), "..")
		a := mustInt(lo)
		b := a
		if ok {
			b = mustInt(hi)
		}
		r.ranges = append(r.ranges, [2]int64{a, b})
	}
	return r
}

func mustInt(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic("intl: bad plural rule number: " + s)
	}

	return n
}

// pluralOperands are UTS 35's plural operands of a formatted number: its
// integer digits, its visible fraction digits and, for compact notation, the
// exponent the digits were scaled by.
type pluralOperands struct {
	i, f string
	e    int
}

// operandsOf returns the operands of the decimal ip.fp, the digits a number
// is shown with.
func operandsOf(ip, fp string, e int) pluralOperands {
	ip = strings.TrimLeft(ip, "0")
	if ip == "" {
		ip = "0"
	}
	return pluralOperands{i: ip, f: fp, e: e}
}

// value returns the operand c reduced modulo mod (when mod is not zero), and
// whether it is an integer. A value too large for an int64 reports
// 1<<62, larger than anything a rule compares with.
func (o pluralOperands) value(c byte, mod int64) (int64, bool) {
	digits := func(s string) int64 {
		if mod != 0 {
			var r int64
			for i := 0; i < len(s); i++ {
				r = (r*10 + int64(s[i]-'0')) % mod
			}
			return r
		}

		if len(s) > 18 {
			return 1 << 62
		}

		n, _ := strconv.ParseInt(s, 10, 64)
		return n
	}
	small := func(n int64) int64 {
		if mod != 0 {
			return n % mod
		}

		return n
	}
	t := strings.TrimRight(o.f, "0")
	switch c {
	case 'n':
		return digits(o.i), t == ""
	case 'i':
		// ICU reads i as an int64 and keeps the last 18 digits of an integer
		// part too long for one, so 1e21 has an i of 0.
		i := o.i
		if len(i) > 18 {
			i = i[len(i)-18:]
		}
		return digits(i), true
	case 'v':
		return small(int64(len(o.f))), true
	case 'w':
		return small(int64(len(t))), true
	case 'f':
		if o.f == "" {
			return 0, true
		}

		return digits(o.f), true
	case 't':
		if t == "" {
			return 0, true
		}

		return digits(t), true
	case 'c', 'e':
		return small(int64(o.e)), true
	}
	return 0, true
}

func (r *pluralRules) selectCategory(o pluralOperands) string {
	for _, rule := range r.rules {
		for _, and := range rule.or {
			ok := true
			for _, rel := range and {
				if !rel.match(o) {
					ok = false
					break
				}
			}
			if ok {
				return rule.category
			}
		}
	}
	return "other"
}

func (rel pluralRelation) match(o pluralOperands) bool {
	v, integer := o.value(rel.operand, rel.mod)
	in := false
	if integer {
		for _, rg := range rel.ranges {
			if v >= rg[0] && v <= rg[1] {
				in = true
				break
			}
		}
	}
	return in != rel.negate
}

// pluralRange returns the category of a range from start to end in lang,
// from CLDR's plural ranges. A pair the data does not list is "other", which
// is what ICU does.
func pluralRange(lang, start, end string) string {
	d := cldr().supplemental()
	if c, ok := d.get("pr/" + lang + "/start-" + start + "-end-" + end); ok {
		return c
	}

	return "other"
}

// pluralFormat is a resolved Intl.PluralRules. Like numberFormat, its json
// form is both the resolvedOptions() result, in the order a browser reports
// it, and the state the shim hands back on every call.
type pluralFormat struct {
	Locale                   string   `json:"locale"`
	Type                     string   `json:"type"`
	Notation                 string   `json:"notation"`
	MinimumIntegerDigits     int      `json:"minimumIntegerDigits"`
	MinimumFractionDigits    *int     `json:"minimumFractionDigits,omitempty"`
	MaximumFractionDigits    *int     `json:"maximumFractionDigits,omitempty"`
	MinimumSignificantDigits *int     `json:"minimumSignificantDigits,omitempty"`
	MaximumSignificantDigits *int     `json:"maximumSignificantDigits,omitempty"`
	PluralCategories         []string `json:"pluralCategories"`
	RoundingIncrement        int      `json:"roundingIncrement"`
	RoundingMode             string   `json:"roundingMode"`
	RoundingPriority         string   `json:"roundingPriority"`
	TrailingZeroDisplay      string   `json:"trailingZeroDisplay"`

	CompactRound bool   `json:"_compactRound,omitempty"`
	DataLocale   string `json:"_data"`
	PluralLocale string `json:"_plural"`

	nf    *numberFormat
	rules *pluralRules
}

func resolvePluralRules(req request) (*pluralFormat, error) {
	r, err := resolveLocale(req.Locales, pluralService)
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

	p := &pluralFormat{Locale: r.locale, DataLocale: r.data.name, PluralLocale: pluralLocale(r.tag)}
	if p.Type, err = optDefault(o, "type", "cardinal", "cardinal", "ordinal"); err != nil {
		return nil, err
	}

	if p.Notation, err = optDefault(o, "notation", "standard", "standard", "scientific", "engineering", "compact"); err != nil {
		return nil, err
	}

	nf := &numberFormat{Style: "decimal", Notation: p.Notation, RoundingIncrement: 1, RoundingPriority: "auto"}
	if nf.RoundingMode, err = optDefault(o, "roundingMode", "halfExpand", "ceil", "floor", "expand", "trunc",
		"halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven"); err != nil {
		return nil, err
	}

	if nf.TrailingZeroDisplay, err = optDefault(o, "trailingZeroDisplay", "auto", "auto", "stripIfInteger"); err != nil {
		return nil, err
	}

	if err := nf.digitOptions(o, 0); err != nil {
		return nil, err
	}

	p.MinimumIntegerDigits = nf.MinimumIntegerDigits
	p.MinimumFractionDigits, p.MaximumFractionDigits = nf.MinimumFractionDigits, nf.MaximumFractionDigits
	p.MinimumSignificantDigits, p.MaximumSignificantDigits = nf.MinimumSignificantDigits, nf.MaximumSignificantDigits
	p.RoundingIncrement, p.RoundingMode = nf.RoundingIncrement, nf.RoundingMode
	p.RoundingPriority, p.TrailingZeroDisplay, p.CompactRound = nf.RoundingPriority, nf.TrailingZeroDisplay, nf.CompactRound
	p.PluralCategories = pluralRulesFor(p.PluralLocale, p.Type == "ordinal").cats
	return p, nil
}

// init compiles the rules and the number formatter whose rounding select
// applies before picking a category.
func (p *pluralFormat) init() error {
	data := cldr().locale(p.DataLocale)
	if data == nil {
		return rangeError("internal error: no locale data for %s", p.DataLocale)
	}

	p.nf = &numberFormat{
		Style:                    "decimal",
		Notation:                 p.Notation,
		CompactDisplay:           "short",
		MinimumIntegerDigits:     p.MinimumIntegerDigits,
		MinimumFractionDigits:    p.MinimumFractionDigits,
		MaximumFractionDigits:    p.MaximumFractionDigits,
		MinimumSignificantDigits: p.MinimumSignificantDigits,
		MaximumSignificantDigits: p.MaximumSignificantDigits,
		RoundingMode:             p.RoundingMode,
		TrailingZeroDisplay:      p.TrailingZeroDisplay,
		CompactRound:             p.CompactRound,
	}
	if p.Notation == "compact" {
		p.nf.compact = numberSystemFor(data, "latn").compactPatterns(false, "short")
	}
	p.rules = pluralRulesFor(p.PluralLocale, p.Type == "ordinal")
	return nil
}

// selectNumber implements Intl.PluralRules.prototype.select for a value the
// shim has converted with Number().
func (p *pluralFormat) selectNumber(v any) string {
	d, special := numberValue(v)
	if special != "" {
		return "other"
	}

	return p.rules.selectCategory(p.nf.pluralOperands(d))
}

// selectRange implements Intl.PluralRules.prototype.selectRange.
func (p *pluralFormat) selectRange(x, y any) string {
	lang, _, _ := strings.Cut(p.Locale, "-")
	return pluralRange(lang, p.selectNumber(x), p.selectNumber(y))
}
