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
// as the difference from its parent's resolved data.
package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
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

	av := loadAvailability()
	g := &gen{keys: map[string]int{}}
	g.locales = selectLocales(av)
	if *fetch {
		fetchAll(g.locales)
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
	all := readJSON(filepath.Join("cldr-core", "availableLocales.json"), true)
	full := map[string]bool{}
	for _, v := range dig(all, "availableLocales", "full").([]any) {
		full[v.(string)] = true
	}

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

// firstData returns the most specific cldr-json locale holding data for tag.
func firstData(tag string, full map[string]bool, likely map[string]string) string {
	if full[tag] {
		return tag
	}

	parts := strings.Split(tag, "-")
	lang, script, region := parts[0], "", ""
	for _, p := range parts[1:] {
		switch {
		case len(p) == 4 && script == "" && region == "":
			script = p
		case (len(p) == 2 || len(p) == 3) && region == "":
			region = p
		}
	}
	if script == "" {
		if m, ok := likely[lang+"-"+region]; ok && region != "" {
			script = strings.Split(m, "-")[1]
		} else if m, ok := likely[lang]; ok {
			script = strings.Split(m, "-")[1]
		}
	}
	var cands []string
	defScript := ""
	if m, ok := likely[lang]; ok {
		defScript = strings.Split(m, "-")[1]
	}
	if script != "" && script != defScript {
		if region != "" {
			cands = append(cands, lang+"-"+script+"-"+region)
		}
		cands = append(cands, lang+"-"+script)
	} else {
		if region != "" {
			cands = append(cands, lang+"-"+region)
		}
		cands = append(cands, lang)
	}
	for _, c := range cands {
		if full[c] {
			return c
		}
	}
	return ""
}

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
	{"cldr-numbers-full", "numbers"},
	{"cldr-numbers-full", "currencies"},
	{"cldr-units-full", "units"},
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
	"cldr-core/supplemental/numberingSystems.json",
	"cldr-core/supplemental/aliases.json",
	"cldr-bcp47/bcp47/timezone.json",
}

func fetchAll(locales []string) {
	var files []string
	files = append(files, supplementalFiles...)
	for _, l := range locales {
		for _, f := range localeFiles {
			files = append(files, f[0]+"/main/"+l+"/"+f[1]+".json")
		}
	}
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

func readJSON(rel string, required bool) map[string]any {
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
		chain := dataChain(l, full, parents, nil)
		if len(chain) > 1 {
			c.parent = index[chain[1]]
		}
	}
	for i := range g.chunks {
		c := &g.chunks[i]
		c.diff = c.kv
		if c.parent >= 0 {
			c.diff = map[string]string{}
			for k, v := range c.kv {
				if pv, ok := g.resolved(c.parent, k); !ok || pv != v {
					c.diff[k] = v
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

// resolved returns the value of k in chunk i or the closest ancestor having it.
func (g *gen) resolved(i int, k string) (string, bool) {
	for ; i >= 0; i = g.chunks[i].parent {
		if v, ok := g.chunks[i].kv[k]; ok {
			return v, true
		}
	}
	return "", false
}

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

// localeData returns the data of one cldr-json locale as flat keys.
func localeData(l string) map[string]string {
	kv := map[string]string{}
	main := func(pkg, file string, path ...string) any {
		v := readJSON(filepath.Join(pkg, "main", l, file+".json"), true)
		return dig(v, append([]string{"main", l}, path...)...)
	}

	greg := main("cldr-dates-full", "ca-gregorian", "dates", "calendars", "gregorian")
	flatten(kv, "g", greg, func(p string) bool {
		switch {
		case strings.HasPrefix(p, "g/quarters"),
			strings.HasPrefix(p, "g/dateSkeletons"),
			strings.HasPrefix(p, "g/timeSkeletons"),
			strings.HasPrefix(p, "g/dateTimeFormats-relative"),
			strings.HasPrefix(p, "g/dateTimeFormats/intervalFormats"),
			strings.HasSuffix(p, "-alt-ascii"),
			strings.Contains(p, "/availableFormats/") && strings.Contains(p, "-count-"):
			return true
		}
		return false
	})

	tz := main("cldr-dates-full", "timeZoneNames", "dates", "timeZoneNames")
	flatten(kv, "z", tz, func(p string) bool {
		return strings.HasSuffix(p, "/exemplarCity") || strings.HasSuffix(p, "/_type")
	})

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
		for k, s := range v.(map[string]any) {
			switch {
			case k == "symbol", k == "symbol-alt-narrow", k == "displayName", strings.HasPrefix(k, "displayName-count-"):
				kv["c/"+code+"/"+k] = s.(string)
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
				uses = append(uses, u["_mzone"].(string)+"|"+from+"|"+to)
			}
			kv["mz/"+prefix] = strings.Join(uses, ";")
		}
	}
	walk("", dig(mz, "metazoneInfo", "timezone"))
	return kv
}
