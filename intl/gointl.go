// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"sync"

	gointl "github.com/liustack/quickjs/internal/gointl"
)

// The constructors built on go-intl (internal/gointl) resolve their locales
// with it too: its canonicalizer, its matcher over each service's available
// locales, which are Chromium's (TestQJSResolveMatchesChromium), and en-US
// when none matches.

// goCompat is how go-intl answers where ECMA-402 and V8 over ICU differ: as
// V8 does, which is what Chromium does.
const goCompat = gointl.NodeICU

var goCanonicalizer = sync.OnceValues(func() (*gointl.Canonicalizer, error) {
	return gointl.NewCanonicalizer(gointl.Embedded, gointl.CanonicalizeOptions{Compat: goCompat})
})

var goDefaultLocale = func() gointl.Locale {
	l, err := gointl.ParseLocale(defaultLocale)
	if err != nil {
		panic(err)
	}

	return l
}()

// goMatchers holds a *gointl.LocaleMatcher per gointl.Service, made on first
// use.
var goMatchers sync.Map

func goMatcher(s gointl.Service) (*gointl.LocaleMatcher, error) {
	if m, ok := goMatchers.Load(s); ok {
		return m.(*gointl.LocaleMatcher), nil
	}

	m, err := gointl.NewLocaleMatcher(gointl.Embedded, s)
	if err != nil {
		return nil, err
	}

	v, _ := goMatchers.LoadOrStore(s, m)
	return v.(*gointl.LocaleMatcher), nil
}

// goLocales is CanonicalizeLocaleList for tags, which javascript has already
// made strings: each in canonical form, once. A tag that is not one is a
// RangeError.
func goLocales(tags []string) ([]gointl.Locale, error) {
	c, err := goCanonicalizer()
	if err != nil {
		return nil, err
	}

	r := make([]gointl.Locale, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		l, err := c.Canonicalize(tag)
		if err != nil {
			return nil, rangeError("Incorrect locale information provided")
		}

		if s := l.String(); !seen[s] {
			seen[s] = true
			r = append(r, l)
		}
	}
	return r, nil
}

// goResolve is ResolveLocale for s as far as the locale: the first of tags
// s is available in, with its Unicode extension, or else en-US. Both
// matchers match alike, in V8 as in go-intl.
func goResolve(s gointl.Service, tags []string) (gointl.Locale, error) {
	l, err := goLocales(tags)
	if err != nil {
		return gointl.Locale{}, err
	}

	m, err := goMatcher(s)
	if err != nil {
		return gointl.Locale{}, err
	}

	return m.Resolve(l, gointl.BestFit, goDefaultLocale), nil
}

// goSupported is SupportedLocales for s: those of tags s is available in, in
// canonical form.
func goSupported(s gointl.Service, tags []string) ([]string, error) {
	l, err := goLocales(tags)
	if err != nil {
		return nil, err
	}

	m, err := goMatcher(s)
	if err != nil {
		return nil, err
	}

	r := []string{}
	for _, l := range m.Supported(l, gointl.BestFit) {
		r = append(r, l.String())
	}
	return r, nil
}

// isTypeSequence reports whether s is a type of a Unicode extension keyword:
// subtags of 3 to 8 letters and digits, as a collation or a numbering system
// option has to be.
func isTypeSequence(s string) bool {
	if s == "" {
		return false
	}

	n := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '-' {
			if n < 3 || n > 8 {
				return false
			}

			n = 0
			continue
		}

		if !isAlnum(s[i : i+1]) {
			return false
		}

		n++
	}
	return true
}

// chromiumLacks is, by language, the one collation type go-intl's data has
// that Chromium's ICU data leaves out: Chromium has no zh unihan, and
// resolves "zh-u-co-unihan" as it resolves "zh".
var chromiumLacks = map[string]string{"zh": "unihan"}

// chromiumHasCollation reports whether Chromium has collation type co for
// language lang, where go-intl has it.
func chromiumHasCollation(lang, co string) bool {
	lacked, ok := chromiumLacks[lang]
	return !ok || lacked != co
}

// withoutLackedCollation is l without a -u-co keyword Chromium does not
// have for l's language.
func withoutLackedCollation(l gointl.Locale) gointl.Locale {
	co, ok := l.Keyword("co")
	if !ok || chromiumHasCollation(l.Language.String(), co) {
		return l
	}

	kept := make([]gointl.Keyword, 0, len(l.Keywords)-1)
	for _, k := range l.Keywords {
		if k.Key != "co" {
			kept = append(kept, k)
		}
	}
	l.Keywords = kept
	return l
}
