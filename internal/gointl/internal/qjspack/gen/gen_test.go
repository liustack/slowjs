// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build gointl_walk

package main

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/blob"
	"github.com/liustack/quickjs/internal/gointl/internal/layout"
	"github.com/liustack/quickjs/internal/gointl/internal/namedata"
)

// names builds a display names data set of two locales in one pool, as
// namegen writes them.
func names(t *testing.T) (pool, de, gsw []byte) {
	t.Helper()
	p := blob.NewPool(namedata.Version)
	enc := func(langs map[string]string) []byte {
		var b namedata.Built
		b.Pattern, b.Separator = "{0} ({1})", "{0}, {1}"
		for code, name := range langs {
			b.Sets[namedata.Language*namedata.Widths+namedata.Long].Entries = append(
				b.Sets[namedata.Language*namedata.Widths+namedata.Long].Entries, namedata.Entry{Code: code, Name: name})
		}
		return namedata.Encode(&b, p)
	}
	de = enc(map[string]string{"en": "Englisch", "fr": "Französisch"})
	gsw = enc(map[string]string{"en": "Änglisch", "fr": "Französisch"})
	return p.Bytes(), de, gsw
}

func TestPrunePoolKeepsWhatKeptLocalesRead(t *testing.T) {
	pool, de, gsw := names(t)
	pruned, err := prunePool(pool, namedata.Version, namedata.Walk, [][]byte{de}, [][]byte{de, gsw}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned) >= len(pool) {
		t.Errorf("pruned pool of %d bytes, the whole one %d", len(pruned), len(pool))
	}
	if bytes.Contains(pruned, []byte("Änglisch")) {
		t.Error("the pruned pool keeps what only the dropped locale says")
	}
	shared, err := blob.ReadShared(pruned, namedata.Version)
	if err != nil {
		t.Fatal(err)
	}
	l, err := namedata.Decode(de, shared)
	if err != nil {
		t.Fatal(err)
	}
	for code, want := range map[string]string{"en": "Englisch", "fr": "Französisch"} {
		if got, _ := l.Lookup(namedata.Language, namedata.Long, code); got != want {
			t.Errorf("Lookup(%s) = %q, want %q", code, got, want)
		}
	}
	if l.Pattern != "{0} ({1})" || l.Separator != "{0}, {1}" {
		t.Errorf("pattern %q, separator %q", l.Pattern, l.Separator)
	}
}

// A walk that does not read everything a locale reaches leaves parts of
// the pool no locale reaches, which the tool takes for a walk that has
// fallen behind its data package.
func TestPrunePoolNoticesAShortWalk(t *testing.T) {
	pool, de, gsw := names(t)
	decodeOnly := func(data []byte, pool blob.Shared) error {
		_, err := namedata.Decode(data, pool)
		return err
	}
	_, err := prunePool(pool, namedata.Version, decodeOnly, [][]byte{de}, [][]byte{de, gsw}, 0)
	if err == nil || !strings.Contains(err.Error(), "no locale reaches") {
		t.Fatalf("err = %v, want one about parts no locale reaches", err)
	}
}

func TestFilterIndex(t *testing.T) {
	in, err := blob.BuildIndex(layout.Available, map[string][]byte{
		"number de": nil, "number de-AT": nil, "number gsw": nil, "all gsw-CH": nil, "plural de": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := filterIndex(in, layout.Available, func(key string) bool { return !strings.Contains(key, "gsw") })
	if err != nil {
		t.Fatal(err)
	}
	x, err := blob.ReadIndex(out, layout.Available)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for i := 0; i < x.Len(); i++ {
		k, _ := x.At(i)
		keys = append(keys, string(k))
	}
	if got, want := strings.Join(keys, ","), "number de,number de-AT,plural de"; got != want {
		t.Errorf("keys %s, want %s", got, want)
	}
}

func dl(t *testing.T, s string) intl.DataLocale {
	t.Helper()
	l, err := intl.ParseLocale(s)
	if err != nil {
		t.Fatal(err)
	}
	return l.Data()
}

func TestFilterPairs(t *testing.T) {
	in := []byte{layout.Pairs}
	for _, p := range [][2]string{{"de-AT", "de"}, {"gsw-LI", "gsw"}, {"sr-ME", "sr-Latn"}} {
		in, _ = dl(t, p[0]).AppendBinary(in)
		in, _ = dl(t, p[1]).AppendBinary(in)
	}
	pairs, err := readPairs(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 3 || pairs[2][1].String() != "sr-Latn" {
		t.Fatalf("pairs %v", pairs)
	}
	out := writePairs(pairs, func(from intl.DataLocale) bool { return from.Language.String() != "gsw" })
	got, err := readPairs(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0][0].String() != "de-AT" || got[1][0].String() != "sr-ME" {
		t.Errorf("pairs %v", got)
	}
}

func TestLanguageOf(t *testing.T) {
	for in, want := range map[string]string{
		"de": "de", "de-AT": "de", "sr_Latn_ME": "sr", "und": "und", "root": "und", "ca-ES-valencia": "ca",
	} {
		if got := languageOf(in); got != want {
			t.Errorf("languageOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// A file of 16 KB or more is a chunk of its own, and the smaller files of a
// directory share one.
func TestChunkFiles(t *testing.T) {
	files := map[string][]byte{
		"collation/zh.bin":  make([]byte, 20000),
		"collation/de.bin":  make([]byte, 100),
		"collation/fr.bin":  make([]byte, 100),
		"tz/europe/paris":   make([]byte, 100),
		"tz/asia/tokyo":     make([]byte, 100),
		"likelysubtags.bin": make([]byte, 30000),
		"weekdata.bin":      make([]byte, 100),
		"timedata.bin":      make([]byte, 100),
	}
	chunks := chunkFiles(files, map[string]bool{"likelysubtags.bin": true})
	var got []string
	for _, c := range chunks {
		var names []string
		for _, f := range c.Files {
			names = append(names, f.Name)
		}
		mark := "z"
		if !c.Deflate {
			mark = "s"
		}
		got = append(got, mark+":"+strings.Join(names, "+"))
	}
	want := "s:likelysubtags.bin z:timedata.bin+weekdata.bin z:collation/zh.bin z:collation/de.bin+collation/fr.bin z:tz/asia/tokyo+tz/europe/paris"
	if strings.Join(got, " ") != want {
		t.Errorf("chunks\n%s\nwant\n%s", strings.Join(got, " "), want)
	}
}

func TestChromiumLanguages(t *testing.T) {
	available, err := blob.BuildIndex(layout.Available, map[string][]byte{
		"number de": nil, "number de-AT": nil, "number ha": nil, "number ha-GH": nil, "number nn": nil,
		"date de": nil, "date ha-GH": nil, "list de": nil, "collator de": nil, "collator in": nil,
		"plural de": nil, "plural cy": nil, "plural ha": nil, "all de": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	write := func(services map[string]map[string]string) string {
		tags := []string{"cy", "de", "de-AT", "ha", "ha-GH", "in", "nn"}
		all := map[string]map[string]string{}
		for _, s := range []string{"Collator", "DateTimeFormat", "DisplayNames", "DurationFormat", "ListFormat",
			"NumberFormat", "PluralRules", "RelativeTimeFormat", "Segmenter"} {
			all[s] = services[s]
		}
		for s, as := range sharedLists {
			all[s] = all[as]
		}
		b, err := json.Marshal(chromiumData{Browser: "Chromium 1", Tags: tags, Canonical: map[string]string{"in": "id"}, Services: all})
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(t.TempDir(), "chromium.json")
		if err := os.WriteFile(file, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return file
	}
	// What resolves elsewhere than to itself. The rest is available.
	no := func(tags ...string) map[string]string {
		m := map[string]string{}
		for _, tag := range tags {
			m[tag] = "en-US"
		}
		return m
	}
	services := map[string]map[string]string{
		"NumberFormat":   no("cy", "ha", "nn"),
		"DateTimeFormat": no("cy", "de-AT", "ha", "nn"),
		"ListFormat":     no("cy", "de-AT", "ha", "ha-GH", "nn"),
		"Collator":       no("cy", "de-AT", "ha", "ha-GH", "nn"),
		"PluralRules":    no("de-AT", "ha-GH", "nn"),
		"Segmenter":      no("de-AT", "ha", "ha-GH", "nn"),
	}
	langs, err := chromiumLanguages(write(services), available)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		list, tag string
		want      bool
	}{
		{"number", "de-AT", true}, {"number", "ha-GH", true}, {"number", "ha", false}, {"number", "nn", false},
		{"date", "ha-GH", true}, {"collator", "in", true}, {"plural", "ha", true},
	} {
		if got := langs.listed(c.list, c.tag); got != c.want {
			t.Errorf("listed(%s, %s) = %v, want %v", c.list, c.tag, got, c.want)
		}
	}
	// ha has only a shell in Chromium, ha-GH, and no data here.
	if got := strings.Join(slices.Sorted(maps.Keys(langs.data)), " "); got != "de" {
		t.Errorf("data languages %s, want de", got)
	}
	if got := strings.Join(slices.Sorted(maps.Keys(langs.plural)), " "); got != "cy de ha" {
		t.Errorf("plural languages %s, want cy de ha", got)
	}
	// Chromium segments cy, which go-intl does not list.
	if got := strings.Join(langs.added, " "); got != "all cy" {
		t.Errorf("added %q, want all cy", got)
	}

	services["NumberFormat"] = no("ha", "nn")
	if _, err := chromiumLanguages(write(services), available); err == nil || !strings.Contains(err.Error(), "does not list") {
		t.Errorf("Chromium with NumberFormat for cy: %v, want an error", err)
	}
}

func TestCorpusLocale(t *testing.T) {
	for line, want := range map[string]string{
		"case\tde-AT\t1234":   "de-AT",
		`["sr_Latn", "x", 1]`: "sr_Latn",
		`[1, "ja"]`:           "ja",
		`new Intl.NumberFormat("fr-CA").format(1)`: "fr-CA",
		`(5).toLocaleString("ar", {})`:             "ar",
		"zh-Hant-HK":                               "zh-Hant-HK",
		"# de":                                     "",
		`["not a tag at all", 2]`:                  "",
		"plain words":                              "",
		`["b","a"].sort(new Intl.Collator("is").compare)`: "is",
	} {
		if got, _ := corpusLocale(line); got != want {
			t.Errorf("corpusLocale(%q) = %q, want %q", line, got, want)
		}
	}
}
