package intl

import (
	"fmt"
	"slices"
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/namedata"
	"github.com/liustack/slowjs/internal/gointl/internal/numdata"
)

// Intl.DisplayNames.
//
// What a locale calls a language, a region, a script, a currency, a calendar
// or a part of a date. This is the largest data set here by a wide margin, and
// the reason that is not a problem is the Source it arrives through: a program
// that never asks what French is called in Japanese never reads a byte of it.

// DisplayKind is what sort of thing is being named.
type DisplayKind int

const (
	DisplayLanguage DisplayKind = iota
	DisplayRegion
	DisplayScript
	DisplayCurrency
	DisplayCalendar
	DisplayDateTimeField
)

// ParseDisplayKind reads a kind as ECMA-402 names it.
func ParseDisplayKind(name string) (DisplayKind, bool) {
	switch name {
	case "language":
		return DisplayLanguage, true
	case "region":
		return DisplayRegion, true
	case "script":
		return DisplayScript, true
	case "currency":
		return DisplayCurrency, true
	case "calendar":
		return DisplayCalendar, true
	case "dateTimeField":
		return DisplayDateTimeField, true
	}
	return 0, false
}

// DisplayStyle is how much room a name takes.
type DisplayStyle int

const (
	DisplayLong DisplayStyle = iota
	DisplayShort
	DisplayNarrow
)

// DisplayFallback is what to answer when a code has no name.
type DisplayFallback int

const (
	// FallbackCode answers with the code itself, and is the default.
	FallbackCode DisplayFallback = iota
	// FallbackNone answers with nothing.
	FallbackNone
)

// LanguageDisplay is how a language with a region or script is named.
type LanguageDisplay int

const (
	// LanguageDialect uses the name the locale has for the whole thing where
	// it has one -- "British English" -- and is the default.
	LanguageDialect LanguageDisplay = iota
	// LanguageStandard always builds the name from its parts: "English
	// (United Kingdom)".
	LanguageStandard
)

// DisplayNamesOptions mirrors the option bag of Intl.DisplayNames. Its zero
// value is ECMA-402's default in every field except Kind, which has no
// default: ECMA-402 requires it.
type DisplayNamesOptions struct {
	Kind            DisplayKind
	Style           DisplayStyle
	Fallback        DisplayFallback
	LanguageDisplay LanguageDisplay

	Compat Compat
}

// A DisplayNames answers what things are called in one locale. It never
// changes after it is built and is safe for any number of goroutines to share.
type DisplayNames struct {
	locale   Locale
	opts     DisplayNamesOptions
	data     *namedata.Locale
	width    int
	currency *numdata.Locale
	// canon puts a language's code in canonical form before it is named.
	canon *Canonicalizer
	// available are the currencies Intl.supportedValuesOf lists, sorted,
	// which are the only ones named on the standard side (CurrencyNames).
	available []string
}

// NewDisplayNames builds one from the data built into the package.
//
// It works in loc as it is given. Resolve a requested locale among the
// service's available locales first, with a LocaleMatcher, to answer as
// ECMA-402 and Node do: "az-Arab" resolves to "az".
func NewDisplayNames(loc Locale, opts DisplayNamesOptions) (*DisplayNames, error) {
	return NewDisplayNamesFrom(Embedded, loc, opts)
}

// NewDisplayNamesFrom builds one from a source of the caller's own.
func NewDisplayNamesFrom(src Source, loc Locale, opts DisplayNamesOptions) (*DisplayNames, error) {
	// ICU keeps language, script and region names in trees of their own,
	// and the redirects differ between them only a little.
	tree := treeLang
	if opts.Kind == DisplayRegion {
		tree = treeRegion
	}
	data, err := loadNames(src, loc, tree)
	if err != nil {
		return nil, err
	}
	// DisplayNames uses nothing of the Unicode extension.
	d := &DisplayNames{locale: loc.onlyKeywords(), opts: opts, data: data}
	switch opts.Style {
	case DisplayShort:
		d.width = namedata.Short
	case DisplayNarrow:
		d.width = namedata.Narrow
	default:
		d.width = namedata.Long
	}
	if opts.Kind == DisplayLanguage {
		// ECMA-402 names a language by its canonical code: "mo" is Romanian.
		if d.canon, err = NewCanonicalizer(src, CanonicalizeOptions{Compat: opts.Compat}); err != nil {
			return nil, err
		}
	}
	if opts.Kind == DisplayCurrency {
		// A currency's name lives with the number data, since that is where it
		// is also used to write an amount out in words.
		if d.currency, err = loadNumbers(src, loc); err != nil {
			return nil, err
		}
		if !opts.Compat.Has(CurrencyNames) {
			if d.available, err = CurrenciesFrom(src); err != nil {
				return nil, err
			}
		}
	}
	return d, nil
}

func loadNames(src Source, loc Locale, tree string) (*namedata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.ChainIn(tree, loc.Data())
	}
	pool, err := openShared(src, MarkerNamesShared, namedata.Version)
	if err != nil {
		return nil, err
	}
	for _, d := range chain {
		b, err := src.Open(MarkerNames, d)
		if err != nil {
			continue
		}
		l, err := namedata.Decode(b, pool)
		if err != nil {
			return nil, fmt.Errorf("intl: the display names for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no display names for %s: %w", loc, ErrNotFound)
}

// Of returns what the locale calls a code.
//
// The second result says whether a name was found. When none was, the first is
// the code itself or empty, by the fallback option.
//
// A language is always named when the fallback is the code, as ICU names
// one: a part of it with no name of its own is written as its code, "English
// (XYZZY)". When the fallback is none, a part with no name leaves the whole
// without one.
func (d *DisplayNames) Of(code string) (string, bool) {
	name, ok := d.lookup(code)
	if ok {
		return name, true
	}
	if d.opts.Fallback == FallbackNone {
		return "", false
	}
	return code, false
}

func (d *DisplayNames) lookup(code string) (string, bool) {
	switch d.opts.Kind {
	case DisplayCurrency:
		if d.currency == nil {
			return "", false
		}
		code = strings.ToUpper(code)
		if !d.opts.Compat.Has(CurrencyNames) {
			// ECMA-402's AvailableCurrencies are the currencies DisplayNames
			// names, so a currency it does not list has no name.
			if _, found := slices.BinarySearch(d.available, code); !found {
				return "", false
			}
		}
		c, ok := d.currency.Currency(code)
		if !ok {
			return "", false
		}
		// The plain display name, not one of the counted wordings: "US Dollar"
		// rather than "US dollars", which is what goes beside an amount.
		if c.DisplayName != "" {
			return c.DisplayName, true
		}
		if name := c.Name("other"); name != "" {
			return name, true
		}
		return "", false
	case DisplayLanguage:
		return d.language(code)
	case DisplayRegion:
		return d.data.Lookup(namedata.Region, d.width, code)
	case DisplayScript:
		return d.data.Lookup(namedata.Script, d.width, code)
	case DisplayCalendar:
		return d.data.Lookup(namedata.Calendar, d.width, code)
	case DisplayDateTimeField:
		if key, ok := dateTimeFieldKeys[code]; ok {
			code = key
		}
		return d.data.Lookup(namedata.DateTimeField, d.width, code)
	}
	return "", false
}

// dateTimeFieldKeys is CLDR's key for each of ECMA-402's date-time fields
// that CLDR calls something else; the rest are CLDR's own.
var dateTimeFieldKeys = map[string]string{
	"weekOfYear": "week", "dayPeriod": "dayperiod", "timeZoneName": "zone",
}

// language names a language, which is the only kind with a shape of its own,
// as ICU's LocaleDisplayNamesImpl::localeDisplayName does: a language
// qualified by a script or a region may have a name for the whole thing, and
// what it does not name is put beside it, the script, the region and the
// variants, "Catalan (Spain, Valencian)".
func (d *DisplayNames) language(code string) (string, bool) {
	loc, err := d.canon.Canonicalize(code)
	if err != nil {
		return "", false
	}
	// A part with no name is written as its code, or leaves the whole
	// without a name: ICU's UDISPCTX_SUBSTITUTE, which V8 asks for by the
	// fallback.
	substitute := d.opts.Fallback == FallbackCode
	lookup := func(kind int, key string) (string, bool) {
		if name, ok := d.data.Lookup(kind, d.width, key); ok {
			return name, true
		}
		return key, substitute
	}

	// ICU's language is empty for "und", which it names as "root".
	lang := loc.Language.String()
	if lang == "" || lang == "und" {
		lang = "root"
	}
	script, region := loc.Script.String(), loc.Region.String()
	// ICU's own spelling of the variants, upper case and sorted, POSIX among
	// them however it was asked for: "sl-posix-rozaj" is "POSIX_ROZAJ".
	var variants []string
	for _, v := range loc.Variants {
		variants = append(variants, strings.ToUpper(v.String()))
	}
	if v, ok := loc.Keyword("va"); ok && v == "posix" {
		variants = append(variants, "POSIX")
	}
	slices.Sort(variants)
	// V8 hands ICU the tag as a string, and ICU's parser takes four letters
	// after the language for a script without asking what follows them, and
	// reads no further: "de-baku1926" is German in the script "Baku".
	if script == "" && region == "" && len(loc.Variants) > 0 {
		if v := loc.Variants[0].String(); len(v) > 4 && isLetters(v[:4]) && !isLetters(v[4:5]) {
			script, variants = strings.ToUpper(v[:1])+v[1:4], nil
		}
	}
	hasScript, hasRegion := script != "", region != ""
	name, named := "", false
	if d.opts.LanguageDisplay == LanguageDialect {
		// ICU tries the language with both a script and a region, then
		// with one of them; but it does not clear the key between tries, so
		// with both it finds only a name for all three: "en-Latn-GB" is
		// English in Latin in the United Kingdom, not British English.
		switch {
		case hasScript && hasRegion:
			if name, named = d.data.Lookup(namedata.Language, d.width, lang+"-"+script+"-"+region); named {
				hasScript, hasRegion = false, false
			}
		case hasScript:
			if name, named = d.data.Lookup(namedata.Language, d.width, lang+"-"+script); named {
				hasScript = false
			}
		case hasRegion:
			if name, named = d.data.Lookup(namedata.Language, d.width, lang+"-"+region); named {
				hasRegion = false
			}
		}
	}
	if !named {
		var ok bool
		if name, ok = lookup(namedata.Language, lang); !ok {
			return "", false
		}
	}

	var qualifiers []string
	qualify := func(kind int, key string) bool {
		q, ok := lookup(kind, key)
		qualifiers = append(qualifiers, q)
		return ok
	}
	if hasScript && !qualify(namedata.Script, script) {
		return "", false
	}
	if hasRegion && !qualify(namedata.Region, region) {
		return "", false
	}
	// ICU names the variants together: "BISKE_ROZAJ", which has no name, so
	// is written as it is.
	if len(variants) > 0 && !qualify(namedata.Variant, strings.Join(variants, "_")) {
		return "", false
	}
	if len(qualifiers) == 0 {
		return name, true
	}

	separator := d.data.Separator
	if separator == "" {
		separator = "{0}, {1}"
	}
	joined := qualifiers[0]
	for _, q := range qualifiers[1:] {
		joined = fill2(separator, joined, q)
	}
	pattern := d.data.Pattern
	if pattern == "" {
		pattern = "{0} ({1})"
	}
	// The parentheses in what goes inside the pattern's become brackets,
	// full width where the pattern's are: "English (Myanmar [Burma])".
	brackets := strings.NewReplacer("(", "[", ")", "]")
	if strings.ContainsRune(pattern, '\uFF08') {
		brackets = strings.NewReplacer("\uFF08", "\uFF3B", "\uFF09", "\uFF3D")
	}
	return fill2(pattern, name, brackets.Replace(joined)), true
}

// isLetters reports whether s is all ASCII letters.
func isLetters(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isAlpha(s[i]) {
			return false
		}
	}
	return true
}

// fill2 substitutes a two-placeholder pattern.
func fill2(pattern, first, second string) string {
	out := strings.ReplaceAll(pattern, "{0}", first)
	return strings.ReplaceAll(out, "{1}", second)
}

// ResolvedDisplayNames is what a DisplayNames settled on.
type ResolvedDisplayNames struct {
	Locale          string
	Kind            DisplayKind
	Style           DisplayStyle
	Fallback        DisplayFallback
	LanguageDisplay LanguageDisplay
}

// ResolvedOptions returns what it settled on.
func (d *DisplayNames) ResolvedOptions() ResolvedDisplayNames {
	return ResolvedDisplayNames{
		Locale:          d.locale.String(),
		Kind:            d.opts.Kind,
		Style:           d.opts.Style,
		Fallback:        d.opts.Fallback,
		LanguageDisplay: d.opts.LanguageDisplay,
	}
}
