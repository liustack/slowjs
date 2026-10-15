// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command gen builds intl/cldr.bin, the locale data package intl formats
// with, from the JSON distribution of CLDR (github.com/unicode-org/cldr-json).
//
// Run it from the root of the module:
//
//	go run ./intl/internal/gen -fetch
//
// -fetch downloads the files it needs for the CLDR version in cldrTag into
// -cldr (a cache directory by default) and skips files already there. Without
// it the directory has to hold a cldr-json checkout or the same files.
//
// Which locales get data is decided by intl/testdata/chromium/availability.json, the
// locales a Chromium build reports as supported. It is written by the golden
// recorder (intl/testdata/chromium/record.cjs), so that a locale this package
// formats is one the browser formats too, and one the browser falls back from
// falls back here as well.
//
// The output is a single file. Its layout:
//
//	"QJI1"
//	uvarint count, then that many key strings (uvarint length + bytes)
//	uvarint count, then per chunk: name, parent+1 (0 for none), offset, size
//	the chunks, each a raw DEFLATE stream of (uvarint key index, string) pairs
//	sorted by key index
//
// Chunk "@" holds the supplemental data, every other chunk one locale, stored
// as the difference from its parent's data: the keys whose value differs, and
// "∅∅∅" for a key the parent has and the locale does not.
//
// The README of the module says how to move to a new CLDR or Chromium.
package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// cldrTag is the cldr-json release the data comes from. Changing it and
// running the generator again is how the data is upgraded; see the README.
const cldrTag = "48.2.3"

var (
	cldrDir = flag.String("cldr", "", "cldr-json directory (default: a per-user cache directory)")
	fetch   = flag.Bool("fetch", false, "download missing cldr-json files")
	out     = flag.String("o", "intl/cldr.bin", "output file")
	browser = flag.String("chromium", "intl/testdata/chromium/availability.json", "the Chromium locale availability file")
	icu     = flag.String("icu", "intl/testdata/chromium/icu.json.gz", "the summary of Chromium's ICU data")
	icudtl  = flag.String("icudtl", "", "summarize this ICU data file of Chromium's into -icu and exit")
)

func main() {
	flag.Parse()
	if *cldrDir == "" {
		d, err := os.UserCacheDir()
		if err != nil {
			fail(err)
		}

		*cldrDir = filepath.Join(d, "quickjs-intl", "cldr-json-"+cldrTag)
	}

	if *icudtl != "" {
		writeICUSummary(*icudtl, *icu)
		return
	}

	av := loadAvailability()
	g := &gen{keys: map[string]int{}}
	if *fetch {
		fetchFiles(supplementalFiles)
	}

	// Root is the parent of every language, as it is in ICU: Chromium has
	// languages that format with nothing but root's data.
	g.locales = append(selectLocales(av), "und")
	slices.Sort(g.locales)
	if *fetch {
		fetchFiles(localeFileList(g.locales))
	}

	g.build(av)
	if err := os.WriteFile(*out, g.encode(), 0o644); err != nil {
		fail(err)
	}

	g.report()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen:", err)
	os.Exit(1)
}

// availability is chromium.json.
type availability struct {
	Browser        string   `json:"browser"`
	NumberFormat   []string `json:"numberFormat"`
	DateTimeFormat []string `json:"dateTimeFormat"`
	PluralRules    []string `json:"pluralRules"`
}

func loadAvailability() *availability {
	b, err := os.ReadFile(*browser)
	if err != nil {
		fail(err)
	}

	var av availability
	if err := json.Unmarshal(b, &av); err != nil {
		fail(err)
	}

	return &av
}

// selectLocales returns the cldr-json locales whose data a supported locale can
// reach: the locale itself or one of its CLDR ancestors.
func selectLocales(av *availability) []string {
	full := fullLocales()

	parents := map[string]string{}
	for k, v := range dig(readJSON(filepath.Join("cldr-core", "supplemental", "parentLocales.json"), true), "supplemental", "parentLocales", "parentLocale").(map[string]any) {
		parents[k] = v.(string)
	}

	likely := likelySubtags()
	sel := map[string]bool{}
	for _, tag := range append(append([]string{}, av.NumberFormat...), av.DateTimeFormat...) {
		for _, l := range dataChain(tag, full, parents, likely) {
			sel[l] = true
		}
	}
	r := make([]string, 0, len(sel))
	for l := range sel {
		r = append(r, l)
	}
	sort.Strings(r)
	return r
}

// regionCurrency picks a region's currency from its list in cldr-json's
// currencyData as ICU's ucurr_forLocale does: the first one that is legal
// tender, else the first one.
func regionCurrency(list []any) string {
	first := ""
	for _, c := range list {
		for code, m := range c.(map[string]any) {
			if m.(map[string]any)["_tender"] != "false" {
				return code
			}
			if first == "" {
				first = code
			}
		}
	}
	return first
}

// fullLocales are the locales cldr-json has data for.
var fullLocales = sync.OnceValue(func() map[string]bool {
	full := map[string]bool{}
	for _, v := range dig(readJSON(filepath.Join("cldr-core", "availableLocales.json"), true), "availableLocales", "full").([]any) {
		full[v.(string)] = true
	}
	return full
})

// dataChain returns the cldr-json locales tag inherits from, the most specific
// one first. It mirrors what the runtime does (see locale.go).
func dataChain(tag string, full map[string]bool, parents map[string]string, likely map[string]string) (r []string) {
	l := firstData(tag, full, likely)
	for l != "" {
		r = append(r, l)
		p, ok := parents[l]
		switch {
		case ok && p == "und":
			return r
		case ok:
			l = p
		default:
			i := strings.LastIndexByte(l, '-')
			if i < 0 {
				return r
			}

			l = l[:i]
		}
		for l != "" && !full[l] {
			i := strings.LastIndexByte(l, '-')
			if i < 0 {
				l = ""
				break
			}

			l = l[:i]
		}
	}
	return r
}

// firstData returns the most specific cldr-json locale holding data for tag:
// the first of language-script-region, language-region, language-script and
// language that exists, with the missing subtags filled in from the likely
// subtags. zh-SG is zh-Hans-SG, zh-TW zh-Hant, en-GB en-GB and pt-BR pt.
func firstData(tag string, full map[string]bool, likely map[string]string) string {
	if full[tag] {
		return tag
	}

	lang, script, region := splitTag(tag)
	ml, ms, mr := maximize(lang, script, region, likely)
	if script == "" {
		script = ms
	}
	if region == "" {
		region = mr
	}
	lang = ml
	for _, c := range []string{lang + "-" + script + "-" + region, lang + "-" + region, lang + "-" + script, lang} {
		if full[c] {
			return c
		}
	}
	return ""
}

// splitTag returns the language, script and region of a canonical tag.
func splitTag(tag string) (lang, script, region string) {
	parts := strings.Split(tag, "-")
	lang = parts[0]
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4 && script == "" && region == "":
			script = p
		case (len(p) == 2 || len(p) == 3 && p[0] >= '0' && p[0] <= '9') && region == "":
			region = p
		}
	}
	return lang, script, region
}

// maximize is CLDR's add likely subtags.
func maximize(lang, script, region string, likely map[string]string) (string, string, string) {
	for _, k := range []string{lang + "-" + script + "-" + region, lang + "-" + region, lang + "-" + script, lang, "und-" + script + "-" + region, "und-" + region, "und-" + script} {
		if strings.Contains(k, "--") || strings.HasSuffix(k, "-") {
			continue
		}

		if m, ok := likely[k]; ok {
			ml, ms, mr := splitTag(m)
			if lang == "und" || lang == "" {
				lang = ml
			}
			if script == "" {
				script = ms
			}
			if region == "" {
				region = mr
			}
			return lang, script, region
		}
	}
	return lang, script, region
}

var likely = sync.OnceValue(likelySubtags)

func likelySubtags() map[string]string {
	r := map[string]string{}
	for k, v := range dig(readJSON(filepath.Join("cldr-core", "supplemental", "likelySubtags.json"), true), "supplemental", "likelySubtags").(map[string]any) {
		r[k] = v.(string)
	}
	return r
}

// localeFiles are the per locale cldr-json files the data comes from.
var localeFiles = [][2]string{
	{"cldr-dates-full", "ca-gregorian"},
	{"cldr-dates-full", "timeZoneNames"},
	{"cldr-dates-full", "dateFields"},
	{"cldr-localenames-full", "territories"},
	{"cldr-numbers-full", "numbers"},
	{"cldr-numbers-full", "currencies"},
	{"cldr-units-full", "units"},
}

// calendars are the calendars other than the gregorian one there is data
// for, with the key prefix their data goes under. A locale gets the data of
// one only when it is the default calendar of the locale's region, as the
// buddhist one is for th: those are the dates a page formats without asking
// for a calendar.
var calendars = map[string]string{
	"buddhist": "b",
	"persian":  "p",
}

// localeCalendar returns the calendar a locale formats dates with by default,
// from its region, explicit or likely.
func localeCalendar(l string, likely map[string]string) string {
	lang, script, region := splitTag(l)
	_, _, region = maximize(lang, script, region, likely)
	pref := dig(readJSON(filepath.Join("cldr-core", "supplemental", "calendarPreferenceData.json"), true), "supplemental", "calendarPreferenceData").(map[string]any)
	cals, ok := pref[region].([]any)
	if !ok {
		cals = pref["001"].([]any)
	}
	return cals[0].(string)
}

var supplementalFiles = []string{
	"cldr-core/availableLocales.json",
	"cldr-core/supplemental/likelySubtags.json",
	"cldr-core/supplemental/parentLocales.json",
	"cldr-core/supplemental/plurals.json",
	"cldr-core/supplemental/ordinals.json",
	"cldr-core/supplemental/pluralRanges.json",
	"cldr-core/supplemental/timeData.json",
	"cldr-core/supplemental/dayPeriods.json",
	"cldr-core/supplemental/currencyData.json",
	"cldr-core/supplemental/metaZones.json",
	"cldr-core/supplemental/primaryZones.json",
	"cldr-core/supplemental/calendarPreferenceData.json",
	"cldr-core/supplemental/numberingSystems.json",
	"cldr-core/supplemental/aliases.json",
	"cldr-bcp47/bcp47/timezone.json",
}

// localeFileList returns the per locale files of locales.
func localeFileList(locales []string) (files []string) {
	likely := likelySubtags()
	for _, l := range locales {
		for _, f := range localeFiles {
			if l == "und" && f[1] == "territories" {
				continue // root names no territories
			}

			files = append(files, f[0]+"/main/"+l+"/"+f[1]+".json")
		}
		if c := localeCalendar(l, likely); calendars[c] != "" {
			files = append(files, calendarFile(l, c))
		}
	}
	return files
}

func fetchFiles(files []string) {
	client := &http.Client{Timeout: time.Minute}
	for _, f := range files {
		dst := filepath.Join(*cldrDir, filepath.FromSlash(f))
		if _, err := os.Stat(dst); err == nil {
			continue
		}

		url := "https://raw.githubusercontent.com/unicode-org/cldr-json/" + cldrTag + "/cldr-json/" + f
		resp, err := client.Get(url)
		if err != nil {
			fail(err)
		}

		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fail(err)
		}

		if resp.StatusCode != http.StatusOK {
			fail(fmt.Errorf("%s: %s", url, resp.Status))
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			fail(err)
		}

		if err := os.WriteFile(dst, b, 0o644); err != nil {
			fail(err)
		}
	}
}

// supplementalJSON caches the supplemental files, which are read once per
// locale.
var supplementalJSON sync.Map

func readJSON(rel string, required bool) map[string]any {
	cache := strings.Contains(rel, "supplemental")
	if v, ok := supplementalJSON.Load(rel); ok && cache {
		return v.(map[string]any)
	}

	b, err := os.ReadFile(filepath.Join(*cldrDir, rel))
	if err != nil {
		if !required && os.IsNotExist(err) {
			return nil
		}

		fail(fmt.Errorf("%v (run with -fetch to download it)", err))
	}

	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		fail(fmt.Errorf("%s: %v", rel, err))
	}

	if cache {
		supplementalJSON.Store(rel, v)
	}
	return v
}

func dig(v any, path ...string) any {
	for _, p := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}

		v = m[p]
	}
	return v
}

type gen struct {
	locales []string
	keys    map[string]int
	keyList []string
	chunks  []chunk
	sizes   map[string][2]int // per category: raw bytes, entries
}

type chunk struct {
	name   string
	parent int // index into chunks, -1 for none
	kv     map[string]string
	diff   map[string]string
	data   []byte
}

func (g *gen) build(av *availability) {
	full := map[string]bool{}
	for _, l := range g.locales {
		full[l] = true
	}
	parents := map[string]string{}
	for k, v := range dig(readJSON(filepath.Join("cldr-core", "supplemental", "parentLocales.json"), true), "supplemental", "parentLocales", "parentLocale").(map[string]any) {
		parents[k] = v.(string)
	}

	g.chunks = append(g.chunks, chunk{name: "@", parent: -1, kv: g.supplemental(av)})
	index := map[string]int{}
	for _, l := range g.locales {
		index[l] = len(g.chunks)
		g.chunks = append(g.chunks, chunk{name: l, parent: -1, kv: localeData(l)})
	}
	for _, l := range g.locales {
		c := &g.chunks[index[l]]
		switch chain := dataChain(l, full, parents, nil); {
		case l == "und":
			// nop
		case len(chain) > 1:
			c.parent = index[chain[1]]
		default:
			c.parent = index["und"]
		}
	}
	chromium := loadChromiumICU(*icu)
	g.followChromium(chromium)
	g.markGeneratedTimes(chromium, av)
	g.applyBrowserOverrides(index)
	for i := range g.chunks {
		c := &g.chunks[i]
		c.diff = c.kv
		if c.parent >= 0 {
			// Every chunk's kv is already resolved, so the difference is from
			// the parent's kv alone. A key the parent has and the child does
			// not is marked absent: en-IN has no "EST", though en does.
			parent := g.chunks[c.parent].kv
			c.diff = map[string]string{}
			for k, v := range c.kv {
				if pv, ok := parent[k]; !ok || pv != v {
					c.diff[k] = v
				}
			}
			for k := range parent {
				if _, ok := c.kv[k]; !ok {
					c.diff[k] = noValue
				}
			}
		}
	}
	// Keys are numbered in sorted order so that the output does not depend on
	// map iteration.
	set := map[string]bool{}
	for _, c := range g.chunks {
		for k := range c.diff {
			set[k] = true
		}
	}
	for k := range set {
		g.keyList = append(g.keyList, k)
	}
	sort.Strings(g.keyList)
	for i, k := range g.keyList {
		g.keys[k] = i
	}
	g.sizes = map[string][2]int{}
	for i := range g.chunks {
		c := &g.chunks[i]
		idx := make([]int, 0, len(c.diff))
		for k, v := range c.diff {
			idx = append(idx, g.keys[k])
			cat := k
			if j := strings.IndexByte(k, '/'); j >= 0 {
				cat = k[:j]
			}
			s := g.sizes[cat]
			g.sizes[cat] = [2]int{s[0] + len(v), s[1] + 1}
		}
		slices.Sort(idx)
		var raw []byte
		for _, k := range idx {
			raw = binary.AppendUvarint(raw, uint64(k))
			v := c.diff[g.keyList[k]]
			raw = binary.AppendUvarint(raw, uint64(len(v)))
			raw = append(raw, v...)
		}
		var buf bytes.Buffer
		w, err := flate.NewWriter(&buf, flate.BestCompression)
		if err != nil {
			fail(err)
		}

		w.Write(raw)
		w.Close()
		c.data = buf.Bytes()
	}
}

// britishIsles are the regions whose zones Chromium never names after the
// GMT metazone.
var britishIsles = map[string]bool{"GB": true, "IE": true, "GG": true, "JE": true, "IM": true}

// noValue marks a key a locale does not have although its parent does. It
// is CLDR's own marker for a deliberately absent value.
const noValue = "∅∅∅"

func (g *gen) encode() []byte {
	var b []byte
	b = append(b, "QJI1"...)
	b = binary.AppendUvarint(b, uint64(len(g.keyList)))
	for _, k := range g.keyList {
		b = binary.AppendUvarint(b, uint64(len(k)))
		b = append(b, k...)
	}
	b = binary.AppendUvarint(b, uint64(len(g.chunks)))
	off := 0
	for _, c := range g.chunks {
		b = binary.AppendUvarint(b, uint64(len(c.name)))
		b = append(b, c.name...)
		b = binary.AppendUvarint(b, uint64(c.parent+1))
		b = binary.AppendUvarint(b, uint64(off))
		b = binary.AppendUvarint(b, uint64(len(c.data)))
		off += len(c.data)
	}
	for _, c := range g.chunks {
		b = append(b, c.data...)
	}
	return b
}

func (g *gen) report() {
	total := 0
	for _, c := range g.chunks {
		total += len(c.data)
	}
	fmt.Printf("CLDR %s: %d locales, %d keys, %d bytes compressed\n", cldrTag, len(g.chunks)-1, len(g.keyList), total)
	var cats []string
	for k := range g.sizes {
		cats = append(cats, k)
	}
	sort.Strings(cats)
	for _, k := range cats {
		fmt.Printf("  %-4s %8d bytes raw in %6d values\n", k, g.sizes[k][0], g.sizes[k][1])
	}
	var big []chunk
	big = append(big, g.chunks...)
	sort.Slice(big, func(i, j int) bool { return len(big[i].data) > len(big[j].data) })
	for _, c := range big[:min(8, len(big))] {
		fmt.Printf("  largest: %-10s %7d bytes\n", c.name, len(c.data))
	}
}

// flatten adds every string leaf under v to kv, keyed by prefix and the path
// to it joined with '/'. skip reports paths to leave out.
func flatten(kv map[string]string, prefix string, v any, skip func(path string) bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			p := prefix + "/" + k
			if skip != nil && skip(p) {
				continue
			}

			flatten(kv, p, e, skip)
		}
	case string:
		kv[prefix] = x
	}
}

// skipCalendar reports whether the calendar data at path p, below the
// calendar's key prefix, is left out.
func skipCalendar(p string) bool {
	_, p, _ = strings.Cut(p, "/")
	switch {
	case strings.HasPrefix(p, "quarters"),
		strings.HasPrefix(p, "dateSkeletons"),
		strings.HasPrefix(p, "timeSkeletons"),
		strings.HasPrefix(p, "dateTimeFormats-relative"),
		strings.HasPrefix(p, "dateTimeFormats/intervalFormats"),
		strings.Contains(p, "-alt-"),
		strings.Contains(p, "/availableFormats/") && strings.Contains(p, "-count-"):
		return true
	}
	return false
}

// calendarData returns the data of calendar c, one of calendars, in
// cldr-json locale l as flat keys.
func calendarData(l, c string) map[string]string {
	kv := map[string]string{}
	file := calendarFile(l, c)
	v := dig(readJSON(filepath.FromSlash(file), true), "main", l, "dates", "calendars", c)
	// ICU looks for the "at time" glue of another calendar in that
	// calendar's own data only, then takes the Gregorian one. cldr-json
	// resolves it through the generic calendar instead, so it is left out
	// and the lookup falls back to the Gregorian glue as ICU's does.
	flatten(kv, calendars[c], v, func(p string) bool {
		_, rest, _ := strings.Cut(p, "/")
		return skipCalendar(p) || strings.HasPrefix(rest, "dateTimeFormats-atTime")
	})
	return kv
}

// calendarFile returns the cldr-json file of calendar c in locale l.
func calendarFile(l, c string) string {
	return "cldr-cal-" + c + "-full/main/" + l + "/ca-" + c + ".json"
}

// localeData returns the data of one cldr-json locale as flat keys.
func localeData(l string) map[string]string {
	kv := map[string]string{}
	main := func(pkg, file string, path ...string) any {
		v := readJSON(filepath.Join(pkg, "main", l, file+".json"), true)
		return dig(v, append([]string{"main", l}, path...)...)
	}

	greg := main("cldr-dates-full", "ca-gregorian", "dates", "calendars", "gregorian")
	flatten(kv, "g", greg, skipCalendar)
	if c := localeCalendar(l, likely()); calendars[c] != "" {
		maps.Copy(kv, calendarData(l, c))
	}

	// Field names, which the date pattern generator writes when it has to
	// append a field no pattern has.
	fields := main("cldr-dates-full", "dateFields", "dates", "fields")
	for _, f := range []string{"era", "year", "quarter", "month", "week", "weekOfMonth", "weekday",
		"dayOfYear", "weekdayOfMonth", "day", "dayperiod", "hour", "minute", "second", "zone"} {
		if s, ok := dig(fields, f, "displayName").(string); ok {
			kv["f/"+f] = s
		}
	}

	tz := main("cldr-dates-full", "timeZoneNames", "dates", "timeZoneNames")
	// Chromium's ICU data has no exemplar cities: a zone's city is always
	// the one its id spells ("Sao Paulo"), which is what the code derives
	// when the data has none.
	flatten(kv, "z", tz, func(p string) bool {
		return strings.HasSuffix(p, "/_type") || strings.HasSuffix(p, "/exemplarCity")
	})

	// The names of the countries time zones are in, for names like
	// "Japan Time".
	regions := zoneRegions()
	territories := map[string]any{}
	if l != "und" { // root names no territories
		territories = main("cldr-localenames-full", "territories", "localeDisplayNames", "territories").(map[string]any)
	}
	for code, v := range territories {
		if regions[code] {
			kv["t/"+code] = v.(string)
		}
	}
	// Chromium's ICU data calls Hong Kong and Macao by their short names:
	// "Hong Kong Time", not "Hong Kong SAR China Time".
	for _, code := range []string{"HK", "MO"} {
		if v, ok := territories[code+"-alt-short"].(string); ok {
			kv["t/"+code] = v
		}
	}

	nums := main("cldr-numbers-full", "numbers", "numbers").(map[string]any)
	systems := map[string]bool{"latn": true}
	if s, ok := nums["defaultNumberingSystem"].(string); ok {
		systems[s] = true
	}
	if s, ok := dig(nums, "otherNumberingSystems", "native").(string); ok {
		systems[s] = true
	}
	flatten(kv, "n", nums, func(p string) bool {
		if i := strings.Index(p, "-numberSystem-"); i >= 0 {
			ns := p[i+len("-numberSystem-"):]
			if j := strings.IndexByte(ns, '/'); j >= 0 {
				ns = ns[:j]
			}
			if !systems[ns] {
				return true
			}
		}
		switch {
		case strings.HasPrefix(p, "n/minimalPairs"),
			strings.HasPrefix(p, "n/rationalFormats"),
			strings.HasPrefix(p, "n/miscPatterns"),
			strings.HasPrefix(p, "n/otherNumberingSystems/") && !strings.HasSuffix(p, "/native"),
			strings.HasSuffix(p, "/list"),
			strings.HasSuffix(p, "/perMille"),
			strings.HasSuffix(p, "/superscriptingExponent"),
			strings.HasSuffix(p, "/approximatelySign"):
			return true
		}
		return false
	})

	cur := main("cldr-numbers-full", "currencies", "numbers", "currencies").(map[string]any)
	for code, v := range cur {
		m := v.(map[string]any)
		for k, s := range m {
			switch {
			case k == "symbol", k == "symbol-alt-narrow", k == "displayName", strings.HasPrefix(k, "displayName-count-"):
				kv["c/"+code+"/"+k] = s.(string)
			}
		}
		// A currency with a pattern or separators of its own formats with
		// all three, the ones it lacks being the locale's, as ICU's data
		// spells them out.
		if m["pattern"] != nil || m["decimal"] != nil || m["group"] != nil {
			for k, own := range map[string]string{
				"pattern": kv["n/currencyFormats-numberSystem-latn/standard"],
				"decimal": kv["n/symbols-numberSystem-latn/decimal"],
				"group":   kv["n/symbols-numberSystem-latn/group"],
			} {
				if s, ok := m[k].(string); ok {
					own = s
				}
				kv["c/"+code+"/"+k] = own
			}
		}
	}

	units := main("cldr-units-full", "units", "units").(map[string]any)
	for _, width := range []string{"long", "short", "narrow"} {
		w, _ := units[width].(map[string]any)
		if p, ok := dig(w, "per", "compoundUnitPattern").(string); ok {
			kv["u/"+width+"/per"] = p
		}
		for id, v := range w {
			unit, ok := sanctionedUnit(id)
			if !ok {
				continue
			}

			for k, s := range v.(map[string]any) {
				switch {
				case k == "displayName", k == "perUnitPattern", strings.HasPrefix(k, "unitPattern-count-"):
					kv["u/"+width+"/"+unit+"/"+k] = s.(string)
				}
			}
		}
	}
	return kv
}

// sanctioned are ECMA-402's simple units (IsSanctionedSingleUnitIdentifier).
var sanctioned = map[string]bool{}

func init() {
	for _, u := range strings.Fields(`acre bit byte celsius centimeter day degree fahrenheit fluid-ounce foot
		gallon gigabit gigabyte gram hectare hour inch kilobit kilobyte kilogram kilometer liter megabit
		megabyte meter microsecond mile mile-scandinavian milliliter millimeter millisecond minute month
		nanosecond ounce percent petabyte pound second stone terabit terabyte week yard year`) {
		sanctioned[u] = true
	}
}

// sanctionedUnit maps a CLDR unit key such as "length-kilometer" or
// "speed-kilometer-per-hour" to the ECMA-402 identifier, if it is one.
func sanctionedUnit(id string) (string, bool) {
	i := strings.IndexByte(id, '-')
	if i < 0 {
		return "", false
	}

	u := id[i+1:]
	if sanctioned[u] {
		return u, true
	}

	if a, b, ok := strings.Cut(u, "-per-"); ok && sanctioned[a] && sanctioned[b] {
		return u, true
	}

	return "", false
}

// supplemental returns the locale independent data.
func (g *gen) supplemental(av *availability) map[string]string {
	kv := map[string]string{}
	kv["version"] = cldrTag
	kv["browser"] = av.Browser
	kv["av/nf"] = strings.Join(av.NumberFormat, " ")
	kv["av/dtf"] = strings.Join(av.DateTimeFormat, " ")
	kv["av/pr"] = strings.Join(av.PluralRules, " ")

	supp := func(file string, path ...string) map[string]any {
		v, _ := dig(readJSON(filepath.Join("cldr-core", "supplemental", file+".json"), true), append([]string{"supplemental"}, path...)...).(map[string]any)
		return v
	}

	// Likely subtags are needed for the languages there is any data for,
	// plus the region and script only ones.
	langs := map[string]bool{}
	for _, l := range g.locales {
		langs[strings.Split(l, "-")[0]] = true
	}
	for _, t := range av.PluralRules {
		langs[strings.Split(t, "-")[0]] = true
	}
	for k, v := range supp("likelySubtags", "likelySubtags") {
		lang := strings.Split(k, "-")[0]
		if langs[lang] || lang == "und" {
			kv["ls/"+k] = v.(string)
		}
	}
	for k, v := range supp("parentLocales", "parentLocale") {
		kv["pl/"+k] = v.(string)
	}
	for lang, v := range supp("plurals", "plurals-type-cardinal") {
		for k, r := range v.(map[string]any) {
			kv["pc/"+lang+"/"+strings.TrimPrefix(k, "pluralRule-count-")] = r.(string)
		}
	}
	for lang, v := range dig(readJSON(filepath.Join("cldr-core", "supplemental", "ordinals.json"), true), "supplemental", "plurals-type-ordinal").(map[string]any) {
		for k, r := range v.(map[string]any) {
			kv["po/"+lang+"/"+strings.TrimPrefix(k, "pluralRule-count-")] = r.(string)
		}
	}
	for lang, v := range supp("pluralRanges", "plurals") {
		for k, r := range v.(map[string]any) {
			kv["pr/"+lang+"/"+strings.TrimPrefix(k, "pluralRange-")] = r.(string)
		}
	}
	for region, v := range supp("timeData", "timeData") {
		m := v.(map[string]any)
		kv["td/"+region] = m["_preferred"].(string) + ";" + m["_allowed"].(string)
	}
	for lang, v := range supp("dayPeriods", "dayPeriodRuleSet") {
		var rules []string
		for name, r := range v.(map[string]any) {
			m := r.(map[string]any)
			if at, ok := m["_at"].(string); ok {
				rules = append(rules, name+"="+at)
				continue
			}

			rules = append(rules, name+"="+m["_from"].(string)+"-"+m["_before"].(string))
		}
		sort.Strings(rules)
		kv["dp/"+lang] = strings.Join(rules, " ")
	}
	for code, v := range supp("currencyData", "currencyData", "fractions") {
		kv["cd/"+code] = v.(map[string]any)["_digits"].(string)
	}
	// A region's currency, as ICU's ucurr_forLocale picks it: the first one
	// listed that is legal tender, else the first one.
	for region, v := range supp("currencyData", "currencyData", "region") {
		kv["rc/"+region] = regionCurrency(v.([]any))
	}
	for name, v := range dig(readJSON(filepath.Join("cldr-core", "supplemental", "numberingSystems.json"), true), "supplemental", "numberingSystems").(map[string]any) {
		m := v.(map[string]any)
		if m["_type"] == "numeric" {
			kv["ns/"+name] = m["_digits"].(string)
		}
	}
	alias := supp("aliases", "metadata", "alias")
	for k, v := range alias["languageAlias"].(map[string]any) {
		kv["la/"+k] = v.(map[string]any)["_replacement"].(string)
	}
	for k, v := range alias["territoryAlias"].(map[string]any) {
		kv["ta/"+k] = v.(map[string]any)["_replacement"].(string)
	}
	for k, v := range alias["scriptAlias"].(map[string]any) {
		kv["sa/"+k] = v.(map[string]any)["_replacement"].(string)
	}
	for k, v := range alias["variantAlias"].(map[string]any) {
		kv["va/"+k] = v.(map[string]any)["_replacement"].(string)
	}

	// Time zones: every IANA identifier CLDR knows, with the canonical one of
	// its group first, and the metazone each one belongs to over time.
	var zones []string
	for k, v := range dig(readJSON(filepath.Join("cldr-bcp47", "bcp47", "timezone.json"), true), "keyword", "u", "tz").(map[string]any) {
		if strings.HasPrefix(k, "_") {
			continue
		}

		if a, ok := v.(map[string]any)["_alias"].(string); ok {
			zones = append(zones, a)
		}
	}
	sort.Strings(zones)
	kv["tz"] = strings.Join(zones, "\n")
	for id, region := range zoneRegion() {
		kv["zr/"+id] = region
	}
	for region, id := range supp("primaryZones", "primaryZones") {
		kv["pz/"+region] = id.(string)
	}
	for region, v := range supp("calendarPreferenceData", "calendarPreferenceData") {
		if c := v.([]any)[0].(string); c != "gregorian" {
			kv["ca/"+region] = c
		}
	}
	mz := supp("metaZones", "metaZones")
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				p := k
				if prefix != "" {
					p = prefix + "/" + k
				}
				walk(p, e)
			}
		case []any:
			var uses []string
			for _, e := range x {
				u := dig(e, "usesMetazone").(map[string]any)
				from, _ := u["_from"].(string)
				to, _ := u["_to"].(string)
				if u["_mzone"] == "GMT" && britishIsles[zoneRegion()[prefix]] {
					// Chromium does not call the time of the British Isles
					// by the GMT metazone's names: London in winter is
					// "GMT+00:00", not "Greenwich Mean Time".
					continue
				}
				uses = append(uses, u["_mzone"].(string)+"|"+from+"|"+to)
			}
			if len(uses) != 0 {
				kv["mz/"+prefix] = strings.Join(uses, ";")
			}
		}
	}
	walk("", dig(mz, "metazoneInfo", "timezone"))
	// The zone a metazone stands for in each territory, "001" for the rest of
	// the world: Asia/Shanghai is China's, America/New_York the Americas'.
	golden := map[string][]string{}
	for _, v := range mz["metazones"].([]any) {
		m := dig(v, "mapZone").(map[string]any)
		mzone := m["_other"].(string)
		golden[mzone] = append(golden[mzone], m["_territory"].(string)+":"+m["_type"].(string))
	}
	for mzone, l := range golden {
		sort.Strings(l)
		kv["mzr/"+mzone] = strings.Join(l, " ")
	}
	return kv
}

// zoneRegion maps every canonical time zone that is in a country to the
// country. The country is the start of the zone's BCP 47 identifier ("usnyc"
// is America/New_York), which CLDR builds from UN/LOCODE, except for the few
// identifiers named otherwise.
var zoneRegion = sync.OnceValue(func() map[string]string {
	special := map[string]string{"jeruslm": "IL", "hebron": "PS", "gazastrp": "PS"}
	r := map[string]string{}
	for k, v := range dig(readJSON(filepath.Join("cldr-bcp47", "bcp47", "timezone.json"), true), "keyword", "u", "tz").(map[string]any) {
		m, ok := v.(map[string]any)
		if !ok || strings.HasPrefix(k, "_") || m["_deprecated"] == true {
			continue
		}

		alias, ok := m["_alias"].(string)
		if !ok {
			continue
		}

		region := special[k]
		if region == "" && len(k) >= 5 && strings.IndexFunc(k, func(c rune) bool { return c < 'a' || c > 'z' }) < 0 &&
			!strings.HasPrefix(k, "utc") && !strings.HasPrefix(k, "gmt") && k != "unk" {
			region = strings.ToUpper(k[:2])
		}
		if region != "" {
			r[strings.Fields(alias)[0]] = region
		}
	}
	return r
})

// zoneRegions is the set of countries some time zone is in.
var zoneRegions = sync.OnceValue(func() map[string]bool {
	r := map[string]bool{}
	for _, region := range zoneRegion() {
		r[region] = true
	}
	return r
})
