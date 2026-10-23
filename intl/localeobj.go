// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/liustack/slowjs"
	gointl "github.com/liustack/slowjs/internal/gointl"
)

// Intl.Locale is not a formatter: an object holds its tag and what its
// getters answer, worked out once by newLocale, and its methods go to
// go-intl's LocaleInfo with the tag.

// goLocaleInfo answers over Chromium's data, as V8 does but for the hour
// cycles of a "-u-sd-" subdivision, which Chromium's ICU, 78, takes the
// region of, as ECMA-402 has it, where Node's ICU passes over it.
var goLocaleInfo = sync.OnceValues(func() (*gointl.LocaleInfo, error) {
	return gointl.NewLocaleInfo(chromiumData{}, gointl.LocaleInfoOptions{Compat: goCompat &^ gointl.SubdivisionHourCycles})
})

// localeRecord is the internal slots of an Intl.Locale, as javascript keeps
// them. A field left out is undefined.
type localeRecord struct {
	Kind            string  `json:"kind"`
	Locale          string  `json:"locale"`
	BaseName        string  `json:"baseName"`
	Language        string  `json:"language"`
	Script          *string `json:"script,omitempty"`
	Region          *string `json:"region,omitempty"`
	Variants        *string `json:"variants,omitempty"`
	Calendar        *string `json:"calendar,omitempty"`
	CaseFirst       *string `json:"caseFirst,omitempty"`
	Collation       *string `json:"collation,omitempty"`
	FirstDayOfWeek  *string `json:"firstDayOfWeek,omitempty"`
	HourCycle       *string `json:"hourCycle,omitempty"`
	Numeric         bool    `json:"numeric"`
	NumberingSystem *string `json:"numberingSystem,omitempty"`
}

// The options of the constructor, in the order it reads them and newLocale
// takes them.
const (
	locLanguage = iota
	locScript
	locRegion
	locVariants
	locCalendar
	locCollation
	locFirstDayOfWeek
	locHourCycle
	locCaseFirst
	locNumeric
	locNumberingSystem
	locOptions
)

// localeKeys are the Unicode extension keys of the options from
// locCalendar on.
var localeKeys = [...]string{"ca", "co", "fw", "hc", "kf", "kn", "nu"}

// weekdays are the values of firstDayOfWeek, from Sunday as 0 and 7.
var weekdays = [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// checkLocale throws the RangeError of the constructor for s, the tag or one
// of the options V8 checks as it reads each: language, script, region and
// variants, and calendar, collation and firstDayOfWeek as ICU's
// uloc_toLegacyType takes a type. Their types are checked in full once all
// are read.
func checkLocale(kind, s string) error {
	ok := false
	switch kind {
	case "tag":
		c, err := goCanonicalizer()
		if err != nil {
			return err
		}

		_, err = c.Canonicalize(s)
		ok = err == nil
	case "language":
		ok = (len(s) >= 2 && len(s) <= 3 || len(s) >= 5 && len(s) <= 8) && isAlpha(s)
	case "script":
		ok = len(s) == 4 && isAlpha(s)
	case "region":
		ok = len(s) == 2 && isAlpha(s) || len(s) == 3 && isDigit(s)
	case "variants":
		ok = s != ""
		seen := map[string]bool{}
		for _, v := range strings.Split(strings.ToLower(s), "-") {
			if !isVariant(v) || seen[v] {
				ok = false
				break
			}

			seen[v] = true
		}
	case "calendar", "collation", "firstDayOfWeek":
		ok = isLegacyType(s)
	}
	if !ok {
		if kind == "tag" {
			return rangeError("Incorrect locale information provided")
		}

		return rangeError("Incorrect locale information provided: %s %s", kind, s)
	}

	return nil
}

// isLegacyType reports whether s is a type to ICU's uloc_toLegacyType
// (isWellFormedLegacyType in uloc_keytype.cpp): runs of ASCII letters and
// digits split by '-', '_' or '/'.
func isLegacyType(s string) bool {
	run := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '-' || c == '_' || c == '/':
			if run == 0 {
				return false
			}
			run = 0
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9':
			run++
		default:
			return false
		}
	}
	return run != 0
}

// isVariant reports whether v is a variant subtag: 5 to 8 letters and
// digits, or 4 starting with a digit.
func isVariant(v string) bool {
	switch {
	case len(v) >= 5 && len(v) <= 8:
	case len(v) == 4 && v[0] >= '0' && v[0] <= '9':
	default:
		return false
	}
	return isAlnum(v)
}

// newLocale is the Intl.Locale constructor after it has read its options,
// each a string or nil where it is undefined, and checked those checkLocale
// and javascript check: tag with the options applied and canonicalized, and
// its slots.
func newLocale(tag string, opts *[locOptions]*string) (*localeRecord, error) {
	c, err := goCanonicalizer()
	if err != nil {
		return nil, err
	}

	l, err := c.Canonicalize(tag)
	if err != nil {
		return nil, rangeError("Incorrect locale information provided")
	}

	if s := opts[locLanguage]; s != nil {
		if l.Language, err = gointl.ParseLanguage(*s); err != nil {
			return nil, err
		}
	}
	if s := opts[locScript]; s != nil {
		if l.Script, err = gointl.ParseScript(*s); err != nil {
			return nil, err
		}
	}
	if s := opts[locRegion]; s != nil {
		if l.Region, err = gointl.ParseRegion(*s); err != nil {
			return nil, err
		}
	}
	if s := opts[locVariants]; s != nil {
		l.Variants = l.Variants[:0:0]
		for _, v := range strings.Split(*s, "-") {
			p, err := gointl.ParseVariant(v)
			if err != nil {
				return nil, err
			}

			l.Variants = append(l.Variants, p)
		}
	}
	for i, key := range localeKeys {
		s := opts[locCalendar+i]
		if s == nil {
			continue
		}

		v := strings.ToLower(*s)
		if i+locCalendar == locFirstDayOfWeek && len(v) == 1 && v[0] >= '0' && v[0] <= '7' {
			v = weekdays[v[0]-'0']
		}
		switch i + locCalendar {
		case locHourCycle, locCaseFirst, locNumeric:
			// Javascript has checked them.
		default:
			if !isTypeSequence(v) {
				return nil, rangeError("Incorrect locale information provided: %s %s", key, *s)
			}
		}
		l = withKeyword(l, key, v)
	}
	// The options can make an alias, "sh" or a deprecated region, which
	// canonicalizing again replaces.
	if l, err = c.Canonicalize(l.String()); err != nil {
		return nil, rangeError("Incorrect locale information provided")
	}

	return localeRecordOf(l), nil
}

// withKeyword is l with the Unicode extension keyword key set to value.
func withKeyword(l gointl.Locale, key, value string) gointl.Locale {
	kw := make([]gointl.Keyword, 0, len(l.Keywords)+1)
	for _, k := range l.Keywords {
		if k.Key != key {
			kw = append(kw, k)
		}
	}
	l.Keywords = append(kw, gointl.Keyword{Key: key, Value: value})
	return l
}

// localeRecordOf is the slots of the Intl.Locale of l. A keyword without a
// value, which stands for "true", is "true" as V8 has it, but for caseFirst,
// which V8 answers as the empty string.
func localeRecordOf(l gointl.Locale) *localeRecord {
	str := func(s string) *string { return &s }
	r := &localeRecord{Kind: "Locale", Locale: l.String(), Language: l.Language.String()}
	base := gointl.Locale{Language: l.Language, Script: l.Script, Region: l.Region, Variants: l.Variants}
	r.BaseName = base.String()
	if !l.Script.IsZero() {
		r.Script = str(l.Script.String())
	}
	if !l.Region.IsZero() {
		r.Region = str(l.Region.String())
	}
	if len(l.Variants) != 0 {
		r.Variants = str(strings.TrimPrefix(gointl.Locale{Variants: l.Variants}.String(), "und-"))
	}
	for _, k := range l.Keywords {
		v := k.Value
		if v == "" && k.Key != "kf" {
			v = "true"
		}
		switch k.Key {
		case "ca":
			r.Calendar = str(v)
		case "co":
			r.Collation = str(v)
		case "fw":
			r.FirstDayOfWeek = str(v)
		case "hc":
			r.HourCycle = str(v)
		case "kf":
			r.CaseFirst = str(v)
		case "kn":
			r.Numeric = v == "true"
		case "nu":
			r.NumberingSystem = str(v)
		}
	}
	return r
}

// localeJSON is newLocale's record as json.
func localeJSON(r *localeRecord) (string, error) {
	b, err := json.Marshal(r)
	return string(b), err
}

// localeInfo is the method what of the Intl.Locale of tag, a tag newLocale
// made.
func localeInfo(vm *slowjs.VM, tag, what string) (slowjs.Value, error) {
	info, err := goLocaleInfo()
	if err != nil {
		return slowjs.Value{}, err
	}

	l, err := gointl.ParseLocale(tag)
	if err != nil {
		return slowjs.Value{}, err
	}

	var list []string
	switch what {
	case "maximize":
		return vm.NewString(info.Maximize(l).String())
	case "minimize":
		return vm.NewString(info.Minimize(l).String())
	case "calendars":
		list = info.Calendars(l)
	case "collations":
		var all []string
		all, err = info.Collations(l)
		for _, co := range all {
			if chromiumHasCollation(l.Language.String(), co) {
				list = append(list, co)
			}
		}
	case "hourCycles":
		list, err = info.HourCycles(l)
	case "numberingSystems":
		list, err = info.NumberingSystems(l)
	case "timeZones":
		var ok bool
		if list, ok, err = info.TimeZones(l); err == nil && !ok {
			return slowjs.UndefinedValue, nil
		}
	case "direction":
		d, err := info.Direction(l)
		if err != nil || d == "" {
			return slowjs.UndefinedValue, err
		}

		return vm.NewString(d)
	case "firstDay":
		first, _, err := info.WeekInfo(l)
		if err != nil {
			return slowjs.Value{}, err
		}

		// ICU's ISO 8601 calendar starts the week on Monday, where no
		// "-u-fw-" keyword says otherwise.
		if ca, _ := l.Keyword("ca"); ca == "iso8601" {
			if _, ok := l.Keyword("fw"); !ok {
				first = 1
			}
		}

		return vm.NewInt(first), nil
	case "weekend":
		_, weekend, err := info.WeekInfo(l)
		if err != nil {
			return slowjs.Value{}, err
		}

		return intArray(vm, weekend)
	default:
		return slowjs.Value{}, errors.New("intl: internal error: no Intl.Locale method " + what)
	}
	if err != nil {
		return slowjs.Value{}, err
	}

	return stringArray(vm, list)
}
