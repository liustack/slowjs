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
	ICU        string               `json:"icu"`        // the data package name, like icudt78l
	Currencies []string             `json:"currencies"` // the currencies some locale has names for
	Paths      []string             `json:"paths"`      // the resources looked for (see icuPath)
	Bundles    map[string]icuBundle `json:"bundles"`    // by tree/name; the main tree has no prefix

	// Numbers are the main tree bundles' symbols and patterns of numbering
	// systems other than latn, by name (see numbers).
	Numbers map[string]map[string]string `json:"numbers"`
}

type icuBundle struct {
	Alias  string         `json:"alias,omitempty"`  // %%ALIAS: the bundle stands for this one
	Parent string         `json:"parent,omitempty"` // %%Parent: the explicit parent
	Has    []int          `json:"has,omitempty"`    // indexes into Paths
	Links  map[int]string `json:"links,omitempty"`  // the ones of Has that are aliases: the resource they stand for
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
	currencies map[string]bool
	numbers    map[string]map[string]string // bundle -> numbering system resources
	bundles    map[string]icuBundle
	has        map[string]map[string]bool   // bundle key -> resource paths
	links      map[string]map[string]string // bundle key -> alias -> the resource it stands for
	parents    map[string]string            // ICU's parent locale table
	chains     map[string][]string          // tree/locale -> chain
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

	c := &chromiumICU{currencies: map[string]bool{}, numbers: s.Numbers, bundles: s.Bundles, has: map[string]map[string]bool{}, links: map[string]map[string]string{}, parents: map[string]string{}, chains: map[string][]string{}}
	for _, code := range s.Currencies {
		c.currencies[code] = true
	}
	for k, b := range s.Bundles {
		c.has[k], c.links[k] = map[string]bool{}, map[string]string{}
		for _, i := range b.Has {
			c.has[k][s.Paths[i]] = true
		}
		for i, target := range b.Links {
			c.links[k][s.Paths[i]] = target
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
// id, "" if none has it. alias is set when the resource there is an alias:
// the resource it stands for, which ICU looks up in the locale's own data.
func (c *chromiumICU) source(tree, id, path string) (bundle, alias string) {
	if entry, ok := strings.CutSuffix(path, "/pattern"); ok && strings.HasPrefix(entry, "Currencies/") {
		// The pattern is part of the currency's entry: the nearest one
		// decides, whether it has a pattern or not.
		b, _ := c.source(tree, id, entry)
		if b == "" || !c.has[icuKey(tree, b)][path] {
			return "", ""
		}

		return b, ""
	}

	for _, b := range c.chain(tree, id) {
		k := icuKey(tree, b)
		// ICU walks the path key by key and follows an alias it meets on
		// the way: root's Persian day names stand for the Gregorian ones,
		// width by width.
		for p := path; ; {
			if target, ok := c.links[k][p]; ok {
				return b, target + path[len(p):]
			}

			i := strings.LastIndexByte(p, '/')
			if i < 0 {
				break
			}

			p = p[:i]
		}
		if c.has[k][path] {
			return b, ""
		}
	}
	// A width no bundle has is filled in by ICU's code from another width,
	// ko's wide day periods from the abbreviated ones, as cldr-json fills
	// it in: the locale takes it from where it takes the names.
	if parts := strings.Split(path, "/"); len(parts) > 3 && parts[0] == "calendar" {
		return c.source(tree, id, strings.Join(parts[:3], "/"))
	}
	return "", ""
}

// resolve is source with aliases followed: it returns the bundle ICU takes
// the value of resource path from.
func (c *chromiumICU) resolve(tree, id, path string) string {
	for range 8 {
		b, alias := c.source(tree, id, path)
		if alias == "" {
			return b
		}

		path = alias
	}
	fail(fmt.Errorf("%s %s: aliases loop at %s", tree, id, path))
	return ""
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
		// The names are looked up width by width: months/format/wide/1 is
		// in monthNames/format/wide.
		ctxWidth := func() string {
			ctx, rest, _ := strings.Cut(rest, "/")
			width, _, _ := strings.Cut(rest, "/")
			return ctx + "/" + width
		}
		var r string
		switch part {
		case "months":
			r = "monthNames/" + ctxWidth()
		case "days":
			r = "dayNames/" + ctxWidth()
		case "eras":
			width, _, _ := strings.Cut(rest, "/")
			r = "eras/" + map[string]string{"eraNames": "wide", "eraAbbr": "abbreviated", "eraNarrow": "narrow"}[width]
		case "dayPeriods":
			// format/abbreviated/am: am and pm are in AmPmMarkers, the
			// others in dayPeriod.
			ctx, rest, _ := strings.Cut(rest, "/")
			width, period, _ := strings.Cut(rest, "/")
			period, _, _ = strings.Cut(period, "-alt-")
			r = "dayPeriod/" + ctx + "/" + width
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
		code, field, _ := strings.Cut(rest, "/")
		switch {
		case field == "pattern", field == "decimal", field == "group":
			return "curr", "Currencies/" + code + "/pattern", true
		case field == "symbol-alt-narrow":
			return "curr", "Currencies%narrow", true
		case strings.HasPrefix(field, "displayName-count-"):
			return "curr", "CurrencyPlurals/" + code, true
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
	add := func(s map[section]map[string]string, kv map[string]string) {
		for k, v := range kv {
			if tree, path, ok := icuPath(k); ok {
				sec := section{tree, path}
				if s[sec] == nil {
					s[sec] = map[string]string{}
				}
				s[sec][k] = v
			}
		}
	}
	sourceKV := func(l string) map[section]map[string]string {
		if s, ok := sources[l]; ok {
			return s
		}

		if *fetch {
			fetchFiles(localeFileList([]string{l}))
		}
		s := map[section]map[string]string{}
		add(s, localeData(l))
		sources[l] = s
		return s
	}
	// sourceSection returns the keys of section sec in CLDR locale l, with
	// the data of a calendar l does not format with by default read for it.
	calendarsRead := map[string]bool{} // CLDR locale/calendar
	sourceSection := func(l string, sec section) map[string]string {
		s := sourceKV(l)
		if rest, ok := strings.CutPrefix(sec.path, "calendar/"); ok {
			cal, _, _ := strings.Cut(rest, "/")
			if calendars[cal] != "" && localeCalendar(l, likely()) != cal && !calendarsRead[l+"/"+cal] {
				if *fetch {
					fetchFiles([]string{calendarFile(l, cal)})
				}
				add(s, calendarData(l, cal))
				calendarsRead[l+"/"+cal] = true
			}
		}
		return s[sec]
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
		// A tree in which Chromium has no bundle of the locale's language
		// comes whole from root or the default locale, along with what
		// CLDR has nothing of for the locale: oc-ES takes the plural names
		// of currencies from English.
		lineage := map[string]bool{}
		for j := i; j >= 0; j = g.chunks[j].parent {
			lineage[g.chunks[j].name] = true
		}
		for _, tree := range icuTrees {
			if l := g.cldrLocale(c.chain(tree, id)[0]); !lineage[l] {
				for sec := range sourceKV(l) {
					if _, ok := own[sec]; !ok && sec.tree == tree {
						own[sec] = nil
					}
				}
			}
		}
		for sec, keys := range own {
			src, alias := c.source(sec.tree, id, sec.path)
			if alias != "" {
				// An alias, mostly root's, stands for another resource of
				// the locale: Persian day names for the Gregorian ones, the
				// accounting pattern for the currency pattern, long unit
				// names for the short ones. The locale's Chromium data can
				// have that from a bundle other than the one cldr-json
				// resolved it from: ckb-IR has root's Persian patterns
				// where cldr-json has ckb's. The value is the one of the
				// bundle that has the resource stood for, which cldr-json
				// resolves as ICU does.
				src = c.resolve(sec.tree, id, alias)
			}
			srcLocale := ""
			if src != "" {
				srcLocale = g.cldrLocale(src)
			}
			switch {
			case srcLocale == ch.name:
				continue
			case srcLocale == "und" && sourceSection("und", sec) == nil:
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

			for k, v := range sourceSection(srcLocale, sec) {
				ch.kv[k] = v
			}
		}
	}
	// Chromium leaves out the names of most currencies no longer in use,
	// and writes those as their codes.
	for _, ch := range g.chunks {
		for k := range ch.kv {
			if rest, ok := strings.CutPrefix(k, "c/"); ok {
				if code, _, _ := strings.Cut(rest, "/"); !c.currencies[code] {
					delete(ch.kv, k)
				}
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

// markGeneratedTimes marks the DateTimeFormat locales whose data ICU finds
// under another region or language, kok-IN in kok_Deva: ICU's
// SimpleDateFormat then generates their timeStyle patterns for the hour cycle
// of the region instead of taking the data's.
func (g *gen) markGeneratedTimes(c *chromiumICU, av *availability) {
	supp := g.chunks[0].kv
	for _, l := range av.DateTimeFormat {
		valid, _ := c.findFirst("", icuID(l))
		lang, _, region := splitTag(l)
		vlang, _, vregion := splitTag(strings.ReplaceAll(valid, "_", "-"))
		if region != "" && region != vregion || lang != vlang {
			supp["gt/"+l] = "1"
		}
	}
}

// icuNumberKeys maps the pattern names of numbers to the keys of the locale
// data, with %s for the numbering system.
var icuNumberKeys = map[string]string{
	"patterns/decimalFormat":    "n/decimalFormats-numberSystem-%s/standard",
	"patterns/percentFormat":    "n/percentFormats-numberSystem-%s/standard",
	"patterns/scientificFormat": "n/scientificFormats-numberSystem-%s/standard",
	"patterns/currencyFormat":   "n/currencyFormats-numberSystem-%s/standard",
	"patterns/accountingFormat": "n/currencyFormats-numberSystem-%s/accounting",
}

// numberKey returns the key of the locale data for <system>/<part>/<name>.
func numberKey(k string) string {
	ns, rest, _ := strings.Cut(k, "/")
	if name, ok := strings.CutPrefix(rest, "symbols/"); ok {
		return "n/symbols-numberSystem-" + ns + "/" + name
	}

	return fmt.Sprintf(icuNumberKeys[rest], ns)
}

// followChromiumNumbers gives each locale the symbols and patterns of
// numbering systems other than latn that ICU finds for it in Chromium.
// cldr-json has them only for the systems a locale uses, so sv has none for
// arab digits where Chromium's sv has a group separator and minus sign, and
// root has all of arab's. A resource that is an alias to latn data is left
// out, as the runtime then takes the locale's latn data, as ICU does.
func (g *gen) followChromiumNumbers(c *chromiumICU) {
	if len(c.numbers["root"]) == 0 {
		fail(fmt.Errorf("the summary of Chromium's ICU data has no numbering systems: run the generator with -icudtl"))
	}

	names := map[string]bool{}
	for _, m := range c.numbers {
		for k := range m {
			names[k] = true
		}
	}
	for i := range g.chunks {
		ch := &g.chunks[i]
		if ch.name == "@" {
			continue
		}

		chain := c.chain("", icuID(ch.name))
		// find returns what ICU finds for k along the chain, following the
		// aliases into the locale's own data.
		find := func(k string) (string, bool) {
			for range 8 {
				v, found := "", false
				for _, b := range chain {
					if v, found = c.numbers[b][k]; found {
						break
					}
				}
				rest, alias := strings.CutPrefix(v, "/LOCALE/NumberElements/")
				if !found || !alias {
					return v, found
				}

				if ns, _, _ := strings.Cut(k, "/"); !strings.HasPrefix(rest, ns+"/") {
					return "", false
				}

				k = rest
			}
			fail(fmt.Errorf("%s: aliases of %s go round", ch.name, k))
			return "", false
		}
		for k := range names {
			key := numberKey(k)
			if v, ok := find(k); ok {
				ch.kv[key] = v
			} else {
				delete(ch.kv, key)
			}
		}
	}
}
