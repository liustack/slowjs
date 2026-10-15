// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Chromium does not ship all of its ICU's locale data: some languages have
// an empty bundle, so their dates and numbers come from root ("2026 M01 5,
// Mon" in az), and some have no currency, zone, unit or region names at all,
// so those come from the default locale, English. Which resources a locale
// takes from where is decided here the way ICU decides it, from a summary of
// the data file of the Chromium the golden test records with.

// icuSummary is what the generator knows of Chromium's ICU data. -icudtl
// writes it to -icu.
type icuSummary struct {
	ICU     string               `json:"icu"`     // the data package name, like icudt78l
	Paths   []string             `json:"paths"`   // the resources looked for (see icuPath)
	Bundles map[string]icuBundle `json:"bundles"` // by tree/name; the main tree has no prefix
}

type icuBundle struct {
	Alias  string `json:"alias,omitempty"`  // %%ALIAS: the bundle stands for this one
	Parent string `json:"parent,omitempty"` // %%Parent: the explicit parent
	Has    []int  `json:"has,omitempty"`    // indexes into Paths
	Links  []int  `json:"links,omitempty"`  // the ones of Has that are aliases of other resources
}

func icuKey(tree, name string) string {
	if tree == "" {
		return name
	}

	return tree + "/" + name
}

// writeICUSummary summarizes the ICU data file at src into dst.
func writeICUSummary(src, dst string) {
	b, err := os.ReadFile(src)
	if err != nil {
		fail(err)
	}

	s, err := summarizeICU(b)
	if err != nil {
		fail(fmt.Errorf("%s: %v", src, err))
	}

	f, err := os.Create(dst)
	if err != nil {
		fail(err)
	}

	z, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		fail(err)
	}

	z.ModTime = time.Time{}
	if err := json.NewEncoder(z).Encode(s); err != nil {
		fail(err)
	}

	if err := z.Close(); err != nil {
		fail(err)
	}

	if err := f.Close(); err != nil {
		fail(err)
	}

	fmt.Printf("%s: %d bundles, %d resources\n", s.ICU, len(s.Bundles), len(s.Paths))
}

// chromiumICU answers where ICU finds a resource for a locale.
type chromiumICU struct {
	bundles map[string]icuBundle
	has     map[string]map[string]bool // bundle key -> resource paths
	links   map[string]map[string]bool // bundle key -> resource paths that are aliases
	parents map[string]string          // ICU's parent locale table
	chains  map[string][]string        // tree/locale -> chain
}

func loadChromiumICU(path string) *chromiumICU {
	f, err := os.Open(path)
	if err != nil {
		fail(fmt.Errorf("%v (see the README on recording Chromium's data)", err))
	}

	defer f.Close()

	z, err := gzip.NewReader(f)
	if err != nil {
		fail(fmt.Errorf("%s: %v", path, err))
	}

	var s icuSummary
	if err := json.NewDecoder(z).Decode(&s); err != nil {
		fail(fmt.Errorf("%s: %v", path, err))
	}

	c := &chromiumICU{bundles: s.Bundles, has: map[string]map[string]bool{}, links: map[string]map[string]bool{}, parents: map[string]string{}, chains: map[string][]string{}}
	for k, b := range s.Bundles {
		c.has[k], c.links[k] = map[string]bool{}, map[string]bool{}
		for _, i := range b.Has {
			c.has[k][s.Paths[i]] = true
		}
		for _, i := range b.Links {
			c.links[k][s.Paths[i]] = true
		}
	}
	// ICU's parent locale table is CLDR's, with the rule that a language in
	// a script other than its likely one falls back to root spelled out.
	for k, v := range dig(readJSON("cldr-core/supplemental/parentLocales.json", true), "supplemental", "parentLocales", "parentLocale").(map[string]any) {
		c.parents[icuID(k)] = icuID(v.(string))
	}
	return c
}

// icuID returns a CLDR locale's ICU name: en_150 for en-150, root for und.
func icuID(l string) string {
	if l == "und" {
		return "root"
	}

	return strings.ReplaceAll(l, "-", "_")
}

// icuDefaultLocale is the locale ICU falls back to when a locale has no
// bundle of its own: the browser's, which the golden test sets to en-US.
const icuDefaultLocale = "en_US"

// chain returns the bundles ICU reads tree's resources from for locale id,
// nearest first and root last, as entryOpen in ICU's uresbund.cpp orders
// them.
func (c *chromiumICU) chain(tree, id string) []string {
	key := icuKey(tree, id)
	if r, ok := c.chains[key]; ok {
		return r
	}

	name, found := c.findFirst(tree, id)
	if !found && name != "root" {
		name, found = c.findFirst(tree, icuDefaultLocale)
	}
	if !found {
		name = "root"
	}
	var r []string
	for {
		b := c.bundles[icuKey(tree, name)]
		if b.Alias != "" {
			name = b.Alias
			continue
		}

		r = append(r, name)
		if name == "root" {
			break
		}

		switch i := strings.LastIndexByte(name, '_'); {
		case b.Parent != "":
			name = b.Parent
		case i < 0:
			name = "root"
		default:
			name = name[:i]
		}
	}
	c.chains[key] = r
	return r
}

// findFirst returns the first bundle of tree on id's fallback path that
// exists, following aliases. found is false when the path ends without one,
// at name.
func (c *chromiumICU) findFirst(tree, id string) (name string, found bool) {
	name = id
	for {
		if b, ok := c.bundles[icuKey(tree, name)]; ok {
			if b.Alias != "" {
				name = b.Alias
				continue
			}

			return name, true
		}

		next, ok := c.parentID(name, id)
		if !ok {
			return name, false
		}

		name = next
	}
}

// parentID is getParentLocaleID in ICU's uresbund.cpp: where ICU looks next
// for a locale whose bundle does not exist.
func (c *chromiumICU) parentID(name, orig string) (string, bool) {
	if p, ok := c.parents[name]; ok {
		return p, true
	}

	lang, script, region := splitTag(strings.ReplaceAll(name, "_", "-"))
	switch {
	case script != "" && region != "":
		if defaultScript(lang, region) == script {
			return lang + "_" + region, true
		}

		return lang + "_" + script, true
	case region != "":
		if _, s, _ := splitTag(strings.ReplaceAll(orig, "_", "-")); s != "" {
			return lang + "_" + s, true
		}

		return lang + "_" + defaultScript(lang, ""), true
	case script != "":
		if defaultScript(lang, "") == script {
			return lang, true
		}

		// A script other than the language's likely one falls back to
		// root.
		return "root", true
	}
	return "", false
}

func defaultScript(lang, region string) string {
	_, s, _ := maximize(lang, "", region, likely())
	return s
}

// source returns the bundle ICU takes resource path of tree from for locale
// id, "" if none has it. link reports that the resource there is an alias of
// another resource, which ICU looks up in the locale's own data.
func (c *chromiumICU) source(tree, id, path string) (bundle string, link bool) {
	for _, b := range c.chain(tree, id) {
		if k := icuKey(tree, b); c.has[k][path] {
			return b, c.links[k][path]
		}
	}
	return "", false
}

// icuPath returns the ICU resource a key of the locale data comes from, and
// the bundle tree it is in. ok is false for a key that is not looked up by
// resource, like the currency spacing.
func icuPath(key string) (tree, path string, ok bool) {
	cat, rest, _ := strings.Cut(key, "/")
	switch cat {
	case "g", "b", "p":
		cal := map[string]string{"g": "gregorian", "b": "buddhist", "p": "persian"}[cat]
		part, rest, _ := strings.Cut(rest, "/")
		var r string
		switch part {
		case "months":
			r = "monthNames"
		case "days":
			r = "dayNames"
		case "eras":
			r = "eras"
		case "dayPeriods":
			// format/abbreviated/am: am and pm are in AmPmMarkers, the
			// others in dayPeriod.
			ctx, rest, _ := strings.Cut(rest, "/")
			width, period, _ := strings.Cut(rest, "/")
			period, _, _ = strings.Cut(period, "-alt-")
			r = "dayPeriod"
			if ctx == "format" && (period == "am" || period == "pm") {
				r = map[string]string{"wide": "AmPmMarkers", "abbreviated": "AmPmMarkersAbbr", "narrow": "AmPmMarkersNarrow"}[width]
			}
		case "dateFormats", "timeFormats":
			r = "DateTimePatterns"
		case "dateTimeFormats-atTime":
			r = "DateTimePatterns%atTime"
		case "dateTimeFormats":
			switch sub, _, _ := strings.Cut(rest, "/"); sub {
			case "availableFormats":
				r = "availableFormats"
			case "appendItems":
				r = "appendItems"
			case "full", "long", "medium", "short":
				r = "DateTimePatterns"
			}
		}
		if r == "" {
			return "", "", false
		}

		return "", "calendar/" + cal + "/" + r, true
	case "f":
		return "", "fields", true
	case "n":
		return numberPath(rest)
	case "c":
		_, field, _ := strings.Cut(rest, "/")
		switch {
		case field == "symbol-alt-narrow":
			return "curr", "Currencies%narrow", true
		case strings.HasPrefix(field, "displayName-count-"):
			return "curr", "CurrencyPlurals", true
		default:
			return "curr", "Currencies", true
		}
	case "u":
		width, _, _ := strings.Cut(rest, "/")
		return "unit", map[string]string{"long": "units", "short": "unitsShort", "narrow": "unitsNarrow"}[width], true
	case "z":
		return "zone", "zoneStrings", true
	case "t":
		return "region", "Countries", true
	}
	return "", "", false
}

// numberPath is icuPath for the keys under n/.
func numberPath(rest string) (tree, path string, ok bool) {
	switch rest {
	case "defaultNumberingSystem":
		return "", "NumberElements/default", true
	case "otherNumberingSystems/native":
		return "", "NumberElements/native", true
	case "minimumGroupingDigits":
		return "", "NumberElements/minimumGroupingDigits", true
	}

	head, sub, _ := strings.Cut(rest, "/")
	kind, ns, ok := strings.Cut(head, "-numberSystem-")
	if !ok {
		return "", "", false
	}

	elements := "NumberElements/" + ns + "/"
	switch kind {
	case "symbols":
		name, _, _ := strings.Cut(sub, "-alt-")
		return "", elements + "symbols/" + name, true
	case "decimalFormats", "percentFormats", "scientificFormats":
		switch {
		case sub == "standard":
			return "", elements + "patterns/" + strings.TrimSuffix(kind, "s"), true
		case strings.HasPrefix(sub, "short/"):
			return "", elements + "patternsShort/decimalFormat", true
		case strings.HasPrefix(sub, "long/"):
			return "", elements + "patternsLong/decimalFormat", true
		}
	case "currencyFormats":
		switch {
		case strings.HasPrefix(sub, "short/"):
			return "", elements + "patternsShort/currencyFormat", true
		case strings.HasPrefix(sub, "unitPattern-count-"):
			return "curr", "CurrencyUnitPatterns", true
		}
		style, variant, _ := strings.Cut(sub, "-")
		name := map[string]string{"standard": "currencyFormat", "accounting": "accountingFormat"}[style]
		if name == "" {
			return "", "", false
		}

		if variant != "" {
			name += "%" + variant
		}
		return "", elements + "patterns/" + name, true
	}
	return "", "", false
}

// followChromium makes each locale take every resource from where ICU would
// in Chromium: from the CLDR locale whose bundle holds it. A locale whose
// bundle in Chromium lacks a resource its CLDR data has gets its parent's,
// root's or the default locale's instead.
func (g *gen) followChromium(c *chromiumICU) {
	type section struct{ tree, path string }
	// The sources are read again rather than taken from the chunks, which
	// this changes as it goes.
	sources := map[string]map[section]map[string]string{} // CLDR locale -> its keys by section
	sourceKV := func(l string) map[section]map[string]string {
		if s, ok := sources[l]; ok {
			return s
		}

		if *fetch {
			fetchFiles(localeFileList([]string{l}))
		}
		s := map[section]map[string]string{}
		for k, v := range localeData(l) {
			if tree, path, ok := icuPath(k); ok {
				sec := section{tree, path}
				if s[sec] == nil {
					s[sec] = map[string]string{}
				}
				s[sec][k] = v
			}
		}
		sources[l] = s
		return s
	}
	for i := range g.chunks {
		ch := &g.chunks[i]
		if ch.name == "@" || ch.name == "und" {
			continue
		}

		id := icuID(ch.name)
		own := map[section][]string{}
		for k := range ch.kv {
			if tree, path, ok := icuPath(k); ok {
				sec := section{tree, path}
				own[sec] = append(own[sec], k)
			}
		}
		for sec, keys := range own {
			src, link := c.source(sec.tree, id, sec.path)
			srcLocale := ""
			if src != "" {
				srcLocale = g.cldrLocale(src)
			}
			switch {
			case srcLocale == ch.name:
				continue
			case link:
				// Root's accounting pattern is the currency pattern of the
				// locale it is looked up for, its buddhist months the
				// Gregorian ones: CLDR resolves such aliases the same way.
				continue
			case srcLocale == "und" && sourceKV("und")[sec] == nil:
				// cldr-json leaves some of root out, like its arab
				// symbols. The locale has them resolved from root.
				continue
			}

			for _, k := range keys {
				delete(ch.kv, k)
			}
			if srcLocale == "" {
				continue
			}

			for k, v := range sourceKV(srcLocale)[sec] {
				ch.kv[k] = v
			}
		}
	}
}

// cldrLocale returns the CLDR locale holding the data of ICU bundle name.
func (g *gen) cldrLocale(name string) string {
	if name == "root" {
		return "und"
	}

	l := firstData(strings.ReplaceAll(name, "_", "-"), fullLocales(), likely())
	if l == "" {
		fail(fmt.Errorf("no CLDR locale has the data of ICU's %s", name))
	}

	return l
}
