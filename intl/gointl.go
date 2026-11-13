// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"maps"
	"sync"

	"github.com/liustack/quickjs"
	gointl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/intlhook"
	"github.com/liustack/quickjs/internal/vmhook"
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
// RangeError. It is for a Builtin of vm, which a long list stops at the
// eval timeout or Interrupt (see intlhook.Check).
func goLocales(vm *quickjs.VM, tags []string) ([]gointl.Locale, error) {
	c, err := goCanonicalizer()
	if err != nil {
		return nil, err
	}

	r := make([]gointl.Locale, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for i, tag := range tags {
		checkTurn(vm, i)
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
func goResolve(vm *quickjs.VM, s gointl.Service, tags []string) (gointl.Locale, error) {
	l, err := goLocales(vm, tags)
	if err != nil {
		return gointl.Locale{}, err
	}

	m, err := goMatcher(s)
	if err != nil {
		return gointl.Locale{}, err
	}

	// A long list goes checkEvery locales at a time, with a check between.
	// Supported finds none that s is available in before the part where it
	// finds one, so resolving from the start of that part resolves the list.
	for len(l) > checkEvery && len(m.Supported(l[:checkEvery], gointl.BestFit)) == 0 {
		intlhook.Check(vm)
		l = l[checkEvery:]
	}
	return m.Resolve(l, gointl.BestFit, goDefaultLocale), nil
}

// goSupported is SupportedLocales for s: those of tags s is available in, in
// canonical form.
func goSupported(vm *quickjs.VM, s gointl.Service, tags []string) ([]string, error) {
	l, err := goLocales(vm, tags)
	if err != nil {
		return nil, err
	}

	m, err := goMatcher(s)
	if err != nil {
		return nil, err
	}

	// Supported takes each locale on its own: a long list goes checkEvery
	// locales at a time, with a check between.
	r := []string{}
	for i := 0; i < len(l); i += checkEvery {
		if i > 0 {
			intlhook.Check(vm)
		}
		for _, x := range m.Supported(l[i:min(i+checkEvery, len(l))], gointl.BestFit) {
			r = append(r, x.String())
		}
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

// chromiumData is go-intl's data (gointl.Embedded) as Chromium's ICU data
// has it. Chromium has the locales of some languages but leaves some of
// their data out, and ICU then takes other data in its place: root's where
// no bundle along the locale's chain has it, and the default locale's,
// English, where a tree has no bundle for the language at all. go-intl has
// the locale's own. chromiumData serves those data sets as Chromium has
// them, for the locales Chromium has no data for: the language alone, its
// default script, as "az-Latn", and regions of it, as "kk-KZ", where
// "kk-Arab" and "az-Cyrl" have data of their own in Chromium.
// TestChromiumData derives the locales from the summary of Chromium's ICU
// data where it can, and the golden test formats in them.
type chromiumData struct{}

var goData = gointl.CacheSource(chromiumData{})

// chromiumMissing are, by data set, the locales Chromium has other data
// for, and the data it has.
var chromiumMissing = map[gointl.Marker]map[string]gointl.DataLocale{
	gointl.MarkerRelativeTime: missingAs(gointl.DataLocale{}, chromiumMissingFields...),
	gointl.MarkerLists:        missingAs(gointl.DataLocale{}, chromiumMissingFields...),
	gointl.MarkerUnits:        chromiumMissingUnits(),
}

// chromiumMissingFields are the locales Chromium has no relative times and
// no lists for.
var chromiumMissingFields = []string{"az", "az-Cyrl", "az-Latn", "bs", "bs-Latn", "kk", "kk-Cyrl", "kk-KZ", "kok",
	"kok-Deva", "pa", "pa-Guru", "sd", "sd-Arab", "uz", "uz-Arab", "uz-Latn"}

// chromiumMissingUnits are the locales Chromium has no units for: those of
// a language whose units it leaves out take English's, and those whose
// parent is root, as "az-Cyrl", root's.
func chromiumMissingUnits() map[string]gointl.DataLocale {
	m := missingAs(english, "ak", "az", "az-Latn", "bs", "bs-Latn", "kk", "kk-Cyrl", "kk-KZ", "kok", "kok-Deva", "nn", "pa",
		"pa-Guru", "sd", "sd-Arab", "uz", "uz-Latn")
	maps.Copy(m, missingAs(gointl.DataLocale{}, "az-Cyrl", "bs-Cyrl", "kk-Arab", "kok-Latn", "pa-Arab", "sd-Deva", "uz-Arab",
		"uz-Cyrl"))
	return m
}

func missingAs(as gointl.DataLocale, tags ...string) map[string]gointl.DataLocale {
	m := make(map[string]gointl.DataLocale, len(tags))
	for _, tag := range tags {
		m[tag] = as
	}
	return m
}

// chromiumMissingLanguages are the languages of chromiumMissing's locales,
// which Open looks for before it writes a locale out.
var chromiumMissingLanguages = func() map[gointl.Language]bool {
	r := map[gointl.Language]bool{}
	for _, m := range chromiumMissing {
		for tag := range m {
			l, err := gointl.ParseLocale(tag)
			if err != nil {
				panic(err)
			}

			r[l.Language] = true
		}
	}
	return r
}()

// english is the data of en-US, the default locale, which ICU falls back
// along to en.
var english = gointl.DataLocale{Language: goDefaultLocale.Language}

func (chromiumData) Open(m gointl.Marker, d gointl.DataLocale) ([]byte, error) {
	if missing, ok := chromiumMissing[m]; ok && chromiumMissingLanguages[d.Language] {
		if as, found := missing[d.String()]; found {
			d = as
		}
	}
	return gointl.Embedded.Open(m, d)
}

// checkEvery is how many turns a loop of a Builtin over a list a script
// passed takes between two calls of intlhook.Check: about a millisecond of
// work or less, for a canonicalization of a few microseconds.
const checkEvery = 256

// checkTurn calls intlhook.Check on turn i of such a loop, once every
// checkEvery turns.
func checkTurn(vm *quickjs.VM, i int) {
	if i%checkEvery == checkEvery-1 {
		intlhook.Check(vm)
	}
}

// limitsOf are the checks of a call on vm, for the work of gointl that is
// one piece no Builtin can cut (gointl.Limits).
func limitsOf(vm *quickjs.VM) *gointl.Limits {
	return &gointl.Limits{
		Check: func() { intlhook.Check(vm) },
		Room:  func(n int64) { intlhook.Room(vm, n) },
	}
}

// stopTurn is checkTurn for a loop that has made javascript values, which
// it frees before it returns when stopTurn is true: then the call is to
// end, with its error pending, and the Builtin returns at once.
func stopTurn(vm *quickjs.VM, i int) bool {
	return i%checkEvery == checkEvery-1 && vmhook.Stop(vm)
}
