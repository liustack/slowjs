package intl

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// What Intl.Locale says of a locale beyond its subtags: its likely full and
// shortest forms, the calendars, collations, hour cycles, numbering systems
// and time zones in use there, which way its script runs, and its week.
// Each is answered as V8 answers it from ICU (js-locale.cc).

// LocaleInfo answers Intl.Locale's questions. It never changes after it is
// built and is safe for any number of goroutines to share.
type LocaleInfo struct {
	src    Source
	likely *Fallbacker
	opts   LocaleInfoOptions
}

// LocaleInfoOptions choose how a LocaleInfo answers.
type LocaleInfoOptions struct {
	Compat Compat
}

// NewLocaleInfo reads what it needs from a source.
func NewLocaleInfo(src Source, opts LocaleInfoOptions) (*LocaleInfo, error) {
	fb, err := NewFallbacker(src)
	if err != nil {
		return nil, err
	}
	return &LocaleInfo{src: src, likely: fb, opts: opts}, nil
}

// keyword is the value of a Unicode extension keyword a getter answers
// with, where the locale has one: its type, and for a key with none, which
// stands for "true", the empty string, as the locale's [[Calendar]] and the
// like hold it. V8 answers "true" (TrueKeywords), and ICU's own spelling of
// it, "yes" (YesValues).
func (i *LocaleInfo) keyword(l Locale, key string) ([]string, bool) {
	v, ok := l.Keyword(key)
	if !ok {
		return nil, false
	}
	if v == "" && i.opts.Compat.Has(TrueKeywords) {
		v = "true"
		if i.opts.Compat.Has(YesValues) {
			v = "yes"
		}
	}
	return []string{v}, true
}

// withBase is the locale with another language, script and region, its
// variants and extensions kept.
func withBase(l Locale, d DataLocale) Locale {
	l.Language, l.Script, l.Region = d.Language, d.Script, d.Region
	return cloned(l)
}

// cloned is the locale with slices of its own. Copying a Locale copies the
// headers of its four slices, so a caller that changed what Maximize or
// Minimize returned would be writing into the arrays of the locale it
// passed; what these return is the caller's to change, as what
// CanonicalizeLocale returns is. A locale with none of the four, which
// most are, is cloned without allocating.
func cloned(l Locale) Locale {
	l.Variants = slices.Clone(l.Variants)
	l.Attributes = slices.Clone(l.Attributes)
	l.Keywords = slices.Clone(l.Keywords)
	l.Extensions = slices.Clone(l.Extensions)
	return l
}

// Maximize is Intl.Locale.prototype.maximize: the locale with the likely
// script and region filled in, "en" as "en-Latn-US". The unknown script
// "Zzzz" and region "ZZ" count as left out, as UTS #35 has it; under
// UnknownSubtags a locale that names all three is kept as it is. What it
// returns shares no slice with the locale it is given.
func (i *LocaleInfo) Maximize(l Locale) Locale {
	d := l.Data()
	if i.opts.Compat.Has(UnknownSubtags) && !d.Language.IsUnd() && !d.Script.IsZero() && !d.Region.IsZero() {
		return cloned(l)
	}
	full, ok := i.likely.Maximize(withoutUnknown(d))
	if !ok {
		return cloned(l)
	}
	return withBase(l, full)
}

// Minimize is Intl.Locale.prototype.minimize: the shortest form that
// maximizes to the same locale, "zh-Hant-TW" as "zh-TW". "Zzzz" and "ZZ"
// count as left out, as they do in ICU as well. What it returns shares no
// slice with the locale it is given.
func (i *LocaleInfo) Minimize(l Locale) Locale {
	short, ok := i.likely.Minimize(withoutUnknown(l.Data()))
	if !ok {
		return cloned(l)
	}
	return withBase(l, short)
}

// withoutUnknown is a locale without the unknown script, "Zzzz", and the
// unknown region, "ZZ", which UTS #35's Add Likely Subtags removes before
// it looks anything up.
func withoutUnknown(d DataLocale) DataLocale {
	if d.Script == (Script{'Z', 'z', 'z', 'z'}) {
		d.Script = Script{}
	}
	if d.Region == (Region{'Z', 'Z'}) {
		d.Region = Region{}
	}
	return d
}

// supplementalRegion is ulocimp_getRegionForSupplementalData: the region of
// "-u-rg-", else the region subtag, else, when inferring, that of "-u-sd-"
// or the likely region.
func (i *LocaleInfo) supplementalRegion(l Locale, infer bool) string {
	if r := keywordRegion(i.src, l, "rg", i.opts.Compat.Has(YesValues)); r != "" {
		return r
	}
	if !l.Region.IsZero() || !infer {
		return l.Region.String()
	}
	if r := keywordRegion(i.src, l, "sd", i.opts.Compat.Has(YesValues)); r != "" {
		return r
	}
	if full, ok := i.likely.Maximize(l.Data()); ok {
		return full.Region.String()
	}
	return ""
}

// keywordRegion is the region of a "-u-rg-" or "-u-sd-" keyword's value,
// as ICU's GetRegionFromKey reads it: its first two letters, of a value of
// three to six that starts with them, where RegionValidateMap has them as a
// region; empty where it has none. "-u-rg-xxzzzz" names none, and the
// locale's own region stands.
//
// With yes, a keyword with no value reads as ICU spells it, "yes", which
// names Yemen (YesValues): "en-u-rg-gb" is the keywords "gb" and "rg".
func keywordRegion(src Source, l Locale, key string, yes bool) string {
	v, ok := l.Keyword(key)
	if ok && v == "" && yes {
		v = "yes"
	}
	if !ok || len(v) < 3 || len(v) > 6 || !isAlpha(v[0]) || !isAlpha(v[1]) {
		return ""
	}
	region := strings.ToUpper(v[:2])
	b, err := src.Open(MarkerValidRegions, DataLocale{})
	if err != nil {
		return ""
	}
	// Each line is two letters and a newline, sorted.
	n := len(b) / 3
	i := sort.Search(n, func(i int) bool { return string(b[3*i:3*i+2]) >= region })
	if i < n && string(b[3*i:3*i+2]) == region {
		return region
	}
	return ""
}

// Calendars is getCalendars: the "-u-ca-" keyword's calendar, else those
// the region reckons in, most preferred first (Calendar::
// getKeywordValuesForLocale, commonly used).
func (i *LocaleInfo) Calendars(l Locale) []string {
	if ca, ok := i.keyword(l, "ca"); ok {
		return ca
	}
	b, err := i.src.Open(MarkerCalendarPrefs, DataLocale{})
	if err != nil {
		return []string{string(Gregory)}
	}
	prefs := calendarPreferences(b)
	if list := prefs.all(i.supplementalRegion(l, true)); len(list) > 0 {
		return list
	}
	if list := prefs.all("001"); len(list) > 0 {
		return list
	}
	return []string{string(Gregory)}
}

// Collations is getCollations: the "-u-co-" keyword's collation, else every
// collation the collation tree has along the locale's chain, but "standard"
// and "search", sorted (Collator::getKeywordValuesForLocale).
func (i *LocaleInfo) Collations(l Locale) ([]string, error) {
	if co, ok := i.keyword(l, "co"); ok {
		return co, nil
	}
	chain, err := collationChain(i.src, l.Data())
	if err != nil {
		return nil, err
	}
	// The collation data names them as ICU's data does; the list is in
	// BCP 47's spelling, "phonebk" for "phonebook".
	bcp := map[string]string{}
	if b, err := i.src.Open(MarkerValues, DataLocale{}); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if f := strings.Fields(line); len(f) == 3 && f[0] == "cotype" {
				bcp[f[1]] = f[2]
			}
		}
	}
	set := map[string]bool{}
	for _, loc := range chain {
		for _, c := range loc.Collations {
			if namedCollation(c.Name) != "" && !strings.HasPrefix(c.Name, "private-") {
				name := c.Name
				if to, ok := bcp[name]; ok {
					name = to
				}
				set[name] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// HourCycles is getHourCycles: the "-u-hc-" keyword's cycle, else the
// pattern generator's default for the locale, whose region is the "-u-rg-"
// keyword's where there is one. A DateTimeFormat does not answer this: V8
// drops "-u-rg-" before building one, and Node's formats 12-hour time for
// "en-US-u-rg-dezzzz", whose Intl.Locale says h23.
//
// ECMA-402's RegionPreference takes a locale without a region subtag to
// be in its "-u-sd-" subdivision's before its likely region; ICU's pattern
// generator passes over the subdivision (SubdivisionHourCycles).
func (i *LocaleInfo) HourCycles(l Locale) ([]string, error) {
	if hc, ok := i.keyword(l, "hc"); ok {
		return hc, nil
	}
	if l.Region.IsZero() && !i.opts.Compat.Has(SubdivisionHourCycles) {
		if r, err := ParseRegion(keywordRegion(i.src, l, "sd", i.opts.Compat.Has(YesValues))); err == nil && !r.IsZero() {
			l.Region = r
		}
	}
	hourChar, _, err := allowedHourFormats(i.src, l)
	if err != nil {
		return nil, err
	}
	g := &dtpg{defaultHourChar: hourChar}
	name := map[HourCycle]string{H11: "h11", H12: "h12", H23: "h23", H24: "h24"}[g.defaultHourCycle()]
	return []string{name}, nil
}

// NumberingSystems is getNumberingSystems: the "-u-nu-" keyword's system,
// else the locale's default.
func (i *LocaleInfo) NumberingSystems(l Locale) ([]string, error) {
	if nu, ok := i.keyword(l, "nu"); ok {
		return nu, nil
	}
	data, err := loadNumbers(i.src, l)
	if err != nil {
		return nil, err
	}
	return []string{data.NumberingSystem}, nil
}

// TimeZones is getTimeZones: the canonical zones of the locale's region
// subtag, sorted, and false for a locale without one, where ECMA-402
// answers undefined.
func (i *LocaleInfo) TimeZones(l Locale) ([]string, bool, error) {
	if l.Region.IsZero() {
		return nil, false, nil
	}
	b, err := i.src.Open(MarkerValues, DataLocale{})
	if err != nil {
		return nil, false, fmt.Errorf("intl: the time zones: %w", err)
	}
	prefix := "zone " + l.Region.String() + " "
	out := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		if zone, ok := strings.CutPrefix(line, prefix); ok {
			out = append(out, zone)
		}
	}
	return out, true, nil
}

// langDirections is uloc_isRightToLeft's shortcut for common languages
// written without a script: "+" after one that runs right to left, "-"
// otherwise. It is ICU's string, and matched as ICU matches it, as a
// substring.
const langDirections = "root-en-es-pt-zh-ja-ko-de-fr-it-ar+he+fa+ru-nl-pl-th-tr-"

// scriptVariants maps the ISO 15924 codes of a script's variant forms,
// which SCRIPT_PROPS has no properties for, to the script: a variant is
// written in its script's direction.
var scriptVariants = map[string]string{
	"Aran": "Arab", "Syre": "Syrc", "Syrj": "Syrc", "Syrn": "Syrc",
	"Latf": "Latn", "Latg": "Latn", "Cyrs": "Cyrl", "Geok": "Geor", "Hrkt": "Kana",
}

// Direction is getTextInfo's direction (TextDirectionOfLocale): "rtl" or
// "ltr" as the locale's script, or else its likely script, is written, and
// the empty string where no one knows -- a script of no direction of its
// own, Common, Inherited, Braille or Unknown, one private use or
// unregistered, or a locale whose likely script cannot be found -- which
// ECMA-402 answers undefined. ICU answers "ltr" for all of those
// (LeftToRightDirection), as RightToLeft does.
func (i *LocaleInfo) Direction(l Locale) (string, error) {
	if i.opts.Compat.Has(LeftToRightDirection) {
		rtl, err := i.RightToLeft(l)
		if err != nil || !rtl {
			return "ltr", err
		}
		return "rtl", nil
	}
	script := l.Script
	if script.IsZero() {
		full, ok := i.likely.Maximize(l.Data())
		if !ok || full.Script.IsZero() {
			return "", nil
		}
		script = full.Script
	}
	name := script.String()
	if base, ok := scriptVariants[name]; ok {
		name = base
	}
	b, err := i.src.Open(MarkerValues, DataLocale{})
	if err != nil {
		return "", fmt.Errorf("intl: the script directions: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		switch line {
		case "rtl " + name:
			return "rtl", nil
		case "ltr " + name:
			return "ltr", nil
		}
	}
	return "", nil
}

// RightToLeft is getTextInfo's direction as ICU answers it: whether the
// locale's script, or its likely script, runs right to left
// (uloc_isRightToLeft). Direction answers as ECMA-402 does.
func (i *LocaleInfo) RightToLeft(l Locale) (bool, error) {
	script := l.Script
	if script.IsZero() {
		if lang := l.Language.String(); !l.Language.IsUnd() {
			if at := strings.Index(langDirections, lang); at >= 0 {
				switch langDirections[at+len(lang)] {
				case '-':
					return false, nil
				case '+':
					return true, nil
				}
			}
		}
		full, ok := i.likely.Maximize(l.Data())
		if !ok || full.Script.IsZero() {
			return false, nil
		}
		script = full.Script
	}
	b, err := i.src.Open(MarkerValues, DataLocale{})
	if err != nil {
		return false, fmt.Errorf("intl: the script directions: %w", err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "rtl "+script.String() {
			return true, nil
		}
	}
	return false, nil
}

// WeekInfo is getWeekInfo: the first day of the week and the weekend days,
// numbered from Monday as one to Sunday as seven. The "-u-fw-" keyword
// chooses the first day; the region's week data does the rest.
func (i *LocaleInfo) WeekInfo(l Locale) (firstDay int, weekend []int, err error) {
	b, err := i.src.Open(MarkerWeekData, DataLocale{})
	if err != nil {
		return 0, nil, fmt.Errorf("intl: the week data: %w", err)
	}
	values := parseWeekData(b)
	region := i.supplementalRegion(l, true)
	get := func(field string) int {
		if v, ok := values[field][region]; ok {
			return v
		}
		return values[field]["001"]
	}
	iso := func(sundayZero int) int {
		if sundayZero == 0 {
			return 7
		}
		return sundayZero
	}
	firstDay = iso(get("firstDay"))
	if fw, ok := l.Keyword("fw"); ok {
		for d, name := range [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"} {
			if fw == name {
				firstDay = iso(d)
			}
		}
	}
	start, end := get("weekendStart"), get("weekendEnd")
	for d := start; ; d = (d + 1) % 7 {
		weekend = append(weekend, iso(d))
		if d == end || len(weekend) == 7 {
			break
		}
	}
	sort.Ints(weekend)
	return firstDay, weekend, nil
}
