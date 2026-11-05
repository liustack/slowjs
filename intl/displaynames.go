// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strings"

	gointl "github.com/liustack/quickjs/internal/gointl"
)

// displayNames is a resolved Intl.DisplayNames.
type displayNames struct {
	Locale          string `json:"locale"`
	Style           string `json:"style"`
	Type            string `json:"type"`
	Fallback        string `json:"fallback"`
	LanguageDisplay string `json:"languageDisplay,omitempty"`

	// d is nil in a build without the display names
	// (quickjs_intl_nonames), where every code falls back.
	d *gointl.DisplayNames
	// long names a region code ICU has replaced, in any style, as ICU
	// does: it looks the short name up by the code asked for, finds none,
	// and takes the long one of the code that replaced it. "BU" is
	// "Myanmar (Burma)" where "MM" is "Myanmar".
	long *gointl.DisplayNames
}

// resolveDisplayNames is the Intl.DisplayNames constructor but for reading
// the options, which intl.js does.
func resolveDisplayNames(req request) (*displayNames, error) {
	o := req.Options
	opts := gointl.DisplayNamesOptions{Compat: goCompat}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	loc, err := goResolve(gointl.ServiceDisplayNames, req.Locales)
	if err != nil {
		return nil, err
	}

	r := &displayNames{Style: "long", Fallback: "code"}
	switch s, err := optString(o, "style", "narrow", "short", "long"); {
	case err != nil:
		return nil, err
	case s == "short":
		opts.Style, r.Style = gointl.DisplayShort, s
	case s == "narrow":
		opts.Style, r.Style = gointl.DisplayNarrow, s
	}
	typ, err := optString(o, "type", "language", "region", "script", "currency", "calendar", "dateTimeField")
	if err != nil {
		return nil, err
	}

	if typ == "" {
		return nil, typeError("Required option 'type' is missing")
	}

	r.Type = typ
	opts.Kind, _ = gointl.ParseDisplayKind(typ)
	switch s, err := optString(o, "fallback", "code", "none"); {
	case err != nil:
		return nil, err
	case s == "none":
		opts.Fallback, r.Fallback = gointl.FallbackNone, s
	}
	switch s, err := optString(o, "languageDisplay", "dialect", "standard"); {
	case err != nil:
		return nil, err
	case typ != "language":
	case s == "standard":
		opts.LanguageDisplay, r.LanguageDisplay = gointl.LanguageStandard, s
	default:
		r.LanguageDisplay = "dialect"
	}
	if gointl.QJSNoNames() {
		r.Locale = gointl.QJSDisplayNamesLocale(loc)
		return r, nil
	}
	if r.d, err = gointl.NewDisplayNames(loc, opts); err != nil {
		return nil, err
	}

	r.long = r.d
	if typ == "region" && opts.Style != gointl.DisplayLong {
		opts.Style = gointl.DisplayLong
		if r.long, err = gointl.NewDisplayNames(loc, opts); err != nil {
			return nil, err
		}
	}
	r.Locale = r.d.ResolvedOptions().Locale
	return r, nil
}

// init has nothing to do: go-intl made it while resolving it.
func (d *displayNames) init() error { return nil }

// of is Intl.DisplayNames.prototype.of for code: its name, or with
// fallback code the code as ECMA-402 writes it, and false for none. A code
// that is not one of the type is a RangeError. Without the display names,
// every code falls back.
func (d *displayNames) of(code string) (string, bool, error) {
	asked := code
	code, err := d.canonical(code)
	if err != nil {
		return "", false, err
	}

	names := d.d
	if d.Type == "region" && code != asked {
		names = d.long
	}
	if names != nil {
		if name, ok := names.Of(code); ok {
			return name, true, nil
		}
	}

	if d.Fallback == "none" {
		return "", false, nil
	}

	return code, true, nil
}

// canonical checks that code is written as a code of the type is, and
// writes it as V8 passes it to ICU, which is not ECMA-402's
// CanonicalCodeForDisplayNames: a region, a script and a calendar keep
// their case, so that "us" has no name and falls back to "us", and a
// region code ICU has replaced is the code that replaced it.
func (d *displayNames) canonical(code string) (string, error) {
	bad := rangeError("invalid %s code: %s", d.Type, code)
	switch d.Type {
	case "language":
		// A unicode_language_id: no extension or private use, which
		// start with a subtag of one character.
		for _, sub := range strings.Split(code, "-") {
			if len(sub) == 1 {
				return "", bad
			}
		}
		if strings.EqualFold(code, "root") {
			return "", bad
		}

		c, err := goCanonicalizer()
		if err != nil {
			return "", err
		}

		l, err := c.Canonicalize(code)
		if err != nil {
			return "", bad
		}

		return l.String(), nil
	case "region":
		switch {
		case len(code) == 2 && isAlpha(code):
			if to, ok := replacedRegions[code]; ok {
				return to, nil
			}
			return code, nil
		case len(code) == 3 && isDigit(code):
			return code, nil
		}
		return "", bad
	case "script":
		if len(code) != 4 || !isAlpha(code) {
			return "", bad
		}

		return code, nil
	case "currency":
		if len(code) != 3 || !isAlpha(code) {
			return "", bad
		}

		return strings.ToUpper(code), nil
	case "calendar":
		if !isTypeSequence(code) {
			return "", bad
		}

		return code, nil
	case "dateTimeField":
		switch code {
		case "era", "year", "quarter", "month", "weekOfYear", "weekday", "day", "dayPeriod", "hour", "minute", "second", "timeZoneName":
			return code, nil
		}
		return "", bad
	}
	return code, nil
}

// replacedRegions are the region codes ICU's Locale replaces when it is
// made, as it reads them, in upper case only (uloc.cpp's
// DEPRECATED_COUNTRIES).
var replacedRegions = map[string]string{
	"AN": "CW", "BU": "MM", "CS": "RS", "DD": "DE", "DY": "BJ", "FX": "FR", "HV": "BF", "NH": "VU", "RH": "ZW",
	"SU": "RU", "TP": "TL", "UK": "GB", "VD": "VN", "YD": "YE", "YU": "RS", "ZR": "CD",
}
