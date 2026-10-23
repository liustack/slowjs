// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	intl "github.com/liustack/slowjs/internal/gointl"
	"github.com/liustack/slowjs/internal/gointl/internal/blob"
	"github.com/liustack/slowjs/internal/gointl/internal/layout"
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
	RBNF            []string                       `json:"rbnf"`
	CalendarStrings map[string]map[string][]string `json:"calendarStrings"`
	ZoneStrings     map[string]map[string]string   `json:"zoneStrings"`
	ICU             string                         `json:"icu"`        // the data package name, like icudt78l
	Currencies      []string                       `json:"currencies"` // the currencies some locale has names for
	Paths           []string                       `json:"paths"`      // the resources looked for
	Bundles         map[string]icuBundle           `json:"bundles"`    // by tree/name; the main tree has no prefix

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
	rbnf            []string
	calendarStrings map[string]map[string][]string
	zoneStrings     map[string]map[string]string
	currencies      map[string]bool
	numbers         map[string]map[string]string // bundle -> numbering system resources
	bundles         map[string]icuBundle
	has             map[string]map[string]bool   // bundle key -> resource paths
	links           map[string]map[string]string // bundle key -> alias -> the resource it stands for
	fallback        blob.Index
	parents         map[string]string   // ICU's parent locale table
	chains          map[string][]string // tree/locale -> chain
}

func loadChromiumICU(path string, files map[string][]byte) *chromiumICU {
	f, err := os.Open(path)
	if err != nil {
		fail(fmt.Errorf("%v (docs/upgrading.md says how to record Chromium's data)", err))
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

	c := &chromiumICU{rbnf: s.RBNF, calendarStrings: s.CalendarStrings, zoneStrings: s.ZoneStrings, currencies: map[string]bool{}, numbers: s.Numbers, bundles: s.Bundles, has: map[string]map[string]bool{}, links: map[string]map[string]string{}, parents: map[string]string{}, chains: map[string][]string{}}
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
	c.fallback, err = blob.ReadIndex(files["icufallback.bin"], layout.ICUFallback)
	if err != nil {
		fail(err)
	}
	for i := 0; i < c.fallback.Len(); i++ {
		key, _ := c.fallback.At(i)
		k := string(key)
		if name, ok := strings.CutPrefix(k, "icuparent "); ok {
			b, _ := c.fallback.Find(k)
			c.parents[name] = string(b)
		}
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
		if c.defaultScript(lang, region) == script {
			return lang + "_" + region, true
		}

		return lang + "_" + script, true
	case region != "":
		if _, s, _ := splitTag(strings.ReplaceAll(orig, "_", "-")); s != "" {
			return lang + "_" + s, true
		}

		return lang + "_" + c.defaultScript(lang, ""), true
	case script != "":
		if c.defaultScript(lang, "") == script {
			return lang, true
		}

		// A script other than the language's likely one falls back to
		// root.
		return "root", true
	}
	return "", false
}

func (c *chromiumICU) defaultScript(lang, region string) string {
	if region != "" {
		if b, ok := c.fallback.Find("defaultscript " + lang + "_" + region); ok {
			return string(b)
		}
	}
	if b, ok := c.fallback.Find("defaultscript " + lang); ok {
		return string(b)
	}
	// ICU omits the default Latin script from this table.
	return "Latn"
}
func splitTag(tag string) (string, string, string) {
	l, err := intl.ParseLocale(tag)
	if err != nil {
		fail(err)
	}
	return l.Language.String(), l.Script.String(), l.Region.String()
}
func fail(err error) { panic(err) }

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

// calendarValues follows resource aliases before reading recorded string arrays.
func (c *chromiumICU) calendarValues(id, path string) []string {
	for range 8 {
		b, a := c.source("", id, path)
		if a == "" {
			return c.calendarStrings[b][path]
		}
		path = a
	}
	fail(fmt.Errorf("calendar alias loop at %s %s", id, path))
	return nil
}
