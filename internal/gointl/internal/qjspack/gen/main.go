// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command gen writes internal/gointl/qjsdata, the packs of go-intl's data that
// quickjs embeds in place of go-intl's data.pack, from go-intl's data
// directory. Run it from the root of the module, with the build tag that lets
// it see which pool parts a locale reads (qjs-sync.sh pack does this):
//
//	go run -tags gointl_walk ./internal/gointl/internal/qjspack/gen -data <go-intl>/data
//
// The locales are Chromium's, as internal/gointl/qjsdata/chromium.json
// (-chromium) records them: which locale Chromium resolves each of go-intl's
// available locales to, for every constructor. Each service's list of
// available locales keeps the tags Chromium resolves to themselves, and no
// other. A language's data is kept when Chromium has the bare language for
// a service, along with any language a kept locale inherits from in
// go-intl's tables, and its plural rules when Chromium has the bare
// language for PluralRules. Chromium has some locales of a language whose
// bare tag it does not have, such as ha-GH without ha: those are shells
// over the root's data in its ICU, and here too. Every other language's
// files go, as do its entries in the tables of ICU's trees. -locales all
// keeps every language and every available locale.
//
// go-intl keeps most of a data set's strings in one pool the locales share
// (datesshared.bin and the like). A pool is cut down to the parts the kept
// locales reach: the tool reads all of each kept locale's data with the data
// package's Walk while internal/blob records the parts read, and empties the
// rest. The parts keep their numbers, so the locales' files are go-intl's,
// byte for byte. Before that it walks every locale and fails if a part is
// left that none reaches: a data package that reads something Walk does not.
//
// The files go into four packs (see internal/qjspack): names.bin, the
// display names, and cjdict.bin, the Chinese and Japanese dictionary of the
// segmenter, which a build tag can leave out, tz.bin, the time zones and
// calendar tables the temporal package reads, which a binary that uses only
// Temporal links alone, and core.bin, the rest. Within a
// pack a file of 16 KB or more is a chunk of its own and the smaller files of
// a directory share one. Every chunk is compressed except the files -store
// names, which are served from the binary's read-only data.
//
// By default the files kept uncompressed are those a formatter reads only a
// little of: the shared pools, from which it reads its locale's strings, and
// the collation root and the segmenter's dictionaries, in which it looks up
// what the text holds. Inflated, a pool goes into the heap whole, for the
// life of the process, the first time any locale reads from it: using every
// service once on linux/amd64 took 11.6 MiB of heap and 79 ms with
// everything compressed, and takes 1.6 MiB and 12 ms with these stored, for
// 5.0 MiB more binary.
// Served in place, only the pages read come into memory, and those are the
// binary's, which the kernel can drop and read again. The locales' own files
// are read whole and compress four to eight times, and stay compressed.
//
// -check writes nothing and fails if qjsdata is not what the data gives.
// -tags prints the tags of go-intl's available locales, which
// qjs-chromium.cjs asks Chromium about. -cut-corpus takes the languages left
// out of go-intl's test corpora, for qjs-sync.sh test-chromium.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/blob"
	"github.com/liustack/quickjs/internal/gointl/internal/datedata"
	"github.com/liustack/quickjs/internal/gointl/internal/layout"
	"github.com/liustack/quickjs/internal/gointl/internal/namedata"
	"github.com/liustack/quickjs/internal/gointl/internal/numdata"
	"github.com/liustack/quickjs/internal/gointl/internal/qjspack"
	"github.com/liustack/quickjs/internal/gointl/internal/reltimedata"
	"github.com/liustack/quickjs/internal/gointl/internal/unitdata"
	"github.com/liustack/quickjs/internal/gointl/internal/zonedata"
)

var (
	dataDir  = flag.String("data", "", "go-intl's data directory")
	outDir   = flag.String("out", "internal/gointl/qjsdata", "where the packs go")
	chromium = flag.String("chromium", "internal/gointl/qjsdata/chromium.json", "what a Chromium build resolves each available locale to")
	locales  = flag.String("locales", "chromium", `the languages kept: "chromium" or "all"`)
	store    = flag.String("store", strings.Join(defaultStore, ","), "comma-separated files kept uncompressed")
	check    = flag.Bool("check", false, "write nothing; fail if the packs are not what the data gives")
	listTags = flag.Bool("tags", false, "write nothing; print the tags of go-intl's available locales, for qjs-chromium.cjs")
	corpus   = flag.String("cut-corpus", "", "take the cases of the languages left out of go-intl's test corpora in this directory")
)

// bigFile is the size from which a file is a chunk of its own.
const bigFile = 16 << 10

// A walkFunc reads all of one locale's data in a pooled data set.
type walkFunc func(data []byte, pool blob.Shared) error

// pooled are the data sets whose locales share a pool.
//
// orphans is how many parts of the pool no locale's data reaches, which the
// generators leave there: a locale with a variant, such as CLDR's el-POLYTON,
// is encoded into the pool and then not written (datawrite.AddLocales). Any
// other number is a Walk that has fallen behind its data package, or a new
// release whose orphans are to be looked at and counted here.
var pooled = []struct {
	dir, pool string
	version   byte
	walk      walkFunc
	orphans   int
}{
	{"dates", "datesshared.bin", datedata.Version, datedata.Walk, 15},
	{"names", "namesshared.bin", namedata.Version, namedata.Walk, 136},
	{"numbers", "numbersshared.bin", numdata.Version, numdata.Walk, 208},
	{"reltime", "reltimeshared.bin", reltimedata.Version, reltimedata.Walk, 4},
	{"units", "unitsshared.bin", unitdata.Version, unitdata.Walk, 5},
	{"zonenames", "zonenamesshared.bin", zonedata.Version, zonedata.Walk, 0},
}

// packOf says which pack a file goes into.
func packOf(name string) string {
	switch {
	case name == "namesshared.bin" || strings.HasPrefix(name, "names/"):
		return "names"
	case name == "brkitr/cjdict.bin":
		return "cjdict"
	case strings.HasPrefix(name, "tz/") || name == "temporalzones.bin" || name == "temporalcalendars.bin":
		return "tz"
	}
	return "core"
}

var packs = []string{"core", "names", "cjdict", "tz"}

// defaultStore are the files kept uncompressed unless -store says otherwise.
var defaultStore = []string{
	"datesshared.bin", "namesshared.bin", "numbersshared.bin", "reltimeshared.bin",
	"unitsshared.bin", "zonenamesshared.bin", "collationroot.bin",
	"brkitr/burmesedict.bin", "brkitr/cjdict.bin", "brkitr/khmerdict.bin",
	"brkitr/laodict.bin", "brkitr/thaidict.bin",
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	if !blob.Walking {
		return errors.New("build with -tags gointl_walk, which records the pool parts a locale reads")
	}
	if *dataDir == "" {
		return errors.New("-data is required")
	}
	files, err := readDir(*dataDir)
	if err != nil {
		return err
	}
	if *listTags {
		tags, err := availableTags(files["available.bin"])
		if err != nil {
			return err
		}
		for _, t := range tags {
			fmt.Println(t)
		}
		return nil
	}
	var langs *languages
	switch *locales {
	case "chromium":
		if langs, err = chromiumLanguages(*chromium, files["available.bin"]); err != nil {
			return err
		}
	case "all":
	default:
		return fmt.Errorf("-locales %q: want chromium or all", *locales)
	}
	out, dropped, pluralOnly, err := cut(files, langs)
	if err != nil {
		return err
	}
	if *corpus != "" {
		unlisted, err := unlistedTags(files["available.bin"], out["available.bin"])
		if err != nil {
			return err
		}
		if err := cutCorpus(*corpus, dropped, pluralOnly, unlisted); err != nil {
			return err
		}
	}

	stored := map[string]bool{}
	for _, s := range strings.Split(*store, ",") {
		if s != "" {
			if _, ok := out[s]; !ok {
				return fmt.Errorf("-store: no file %s", s)
			}
			stored[s] = true
		}
	}
	byPack := map[string]map[string][]byte{}
	for name, b := range out {
		p := packOf(name)
		if byPack[p] == nil {
			byPack[p] = map[string][]byte{}
		}
		byPack[p][name] = b
	}
	var bad []string
	for _, p := range packs {
		b, err := qjspack.Build(chunkFiles(byPack[p], stored))
		if err != nil {
			return err
		}
		file := filepath.Join(*outDir, p+".bin")
		raw := 0
		for _, f := range byPack[p] {
			raw += len(f)
		}
		fmt.Printf("%s: %d files, %d bytes, packed into %d\n", file, len(byPack[p]), raw, len(b))
		if *check {
			if old, err := os.ReadFile(file); err != nil || !bytes.Equal(old, b) {
				bad = append(bad, file)
			}
			continue
		}
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(file, b, 0o644); err != nil {
			return err
		}
	}
	if langs != nil && len(langs.added) > 0 {
		sort.Strings(langs.added)
		fmt.Printf("available as Chromium has them, not go-intl (%d): %s\n", len(langs.added), strings.Join(langs.added, ", "))
	}
	if len(dropped) > 0 {
		fmt.Printf("languages dropped (%d): %s\n", len(dropped), strings.Join(dropped, " "))
	}
	if len(pluralOnly) > 0 {
		fmt.Printf("languages with plural rules only (%d): %s\n", len(pluralOnly), strings.Join(pluralOnly, " "))
	}
	if len(bad) > 0 {
		return fmt.Errorf("not what the data gives: %s", strings.Join(bad, ", "))
	}
	return nil
}

func readDir(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(os.DirFS(dir), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		files[name] = b
		return err
	})
	if err == nil && len(files) == 0 {
		err = fmt.Errorf("%s holds no files", dir)
	}
	return files, err
}

// chromiumData is what qjs-chromium.cjs records.
type chromiumData struct {
	Browser string   `json:"browser"`
	Tags    []string `json:"tags"`
	// Canonical is each tag's canonical form where it is not the tag.
	Canonical map[string]string `json:"canonical"`
	// Services is, for every constructor, the locale each tag resolves to
	// where it is not the tag's canonical form.
	Services map[string]map[string]string `json:"services"`
}

// listService is the constructor each list of data/available.bin is for.
// The other constructors share a list (see availableList in negotiate.go),
// and Chromium has to resolve them as the list's own.
var listService = map[string]string{
	"all": "Segmenter", "collator": "Collator", "date": "DateTimeFormat", "list": "ListFormat",
	"number": "NumberFormat", "plural": "PluralRules",
}

var sharedLists = map[string]string{
	"DisplayNames": "DateTimeFormat", "RelativeTimeFormat": "DateTimeFormat", "DurationFormat": "NumberFormat",
}

// chromiumLanguages reads what a Chromium build resolves go-intl's available
// locales to.
func chromiumLanguages(file string, available []byte) (*languages, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var c chromiumData
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %v", file, err)
	}
	for _, service := range listService {
		if c.Services[service] == nil {
			return nil, fmt.Errorf("%s: no answers for %s", file, service)
		}
	}
	for service, as := range sharedLists {
		if !maps.Equal(c.Services[service], c.Services[as]) {
			return nil, fmt.Errorf("%s: %s resolves as %s does not, and go-intl gives them one list", file, service, as)
		}
	}
	tags, err := availableTags(available)
	if err != nil {
		return nil, err
	}
	index, err := blob.ReadIndex(available, layout.Available)
	if err != nil {
		return nil, fmt.Errorf("available.bin: %v", err)
	}
	if !slices.Equal(tags, c.Tags) {
		return nil, fmt.Errorf("%s was recorded for other tags than available.bin has: run qjs-sync.sh chromium", file)
	}
	langs := &languages{
		data: map[string]bool{}, plural: map[string]bool{},
		listed: func(list, tag string) bool {
			service, ok := listService[list]
			if !ok {
				return false
			}
			_, differs := c.Services[service][tag]
			if canonical, ok := c.Canonical[tag]; ok && strings.HasPrefix(canonical, "ERR:") {
				return false
			}
			return !differs
		},
	}
	for list, service := range listService {
		for _, tag := range tags {
			if !langs.listed(list, tag) {
				continue
			}
			// An alias is never asked for, only its canonical form.
			_, alias := c.Canonical[tag]
			if _, ok := index.Find(list + " " + tag); !ok && !alias {
				// Chromium's ICU has a few locales Node's has not, which
				// go-intl's lists follow. A segmenter reads no data of the
				// locale's own, and is given them. Another service would
				// work in data go-intl does not have.
				if list != "all" {
					return nil, fmt.Errorf("%s: %s has %s, which go-intl does not list", file, service, tag)
				}
				langs.added = append(langs.added, list+" "+tag)
			}
			if alias || strings.Contains(tag, "-") {
				continue
			}
			switch service {
			case "PluralRules":
				langs.plural[tag] = true
			case "Segmenter":
				// Segmenter reads no data of a language's own.
			default:
				langs.data[tag] = true
			}
		}
	}
	return langs, nil
}

// availableTags are the tags of every list of available locales, sorted,
// each once.
func availableTags(available []byte) ([]string, error) {
	x, err := blob.ReadIndex(available, layout.Available)
	if err != nil {
		return nil, fmt.Errorf("available.bin: %v", err)
	}
	seen := map[string]bool{}
	var tags []string
	for i := 0; i < x.Len(); i++ {
		k, _ := x.At(i)
		_, tag, _ := strings.Cut(string(k), " ")
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	sort.Strings(tags)
	return tags, nil
}

// languageOf is the language of a locale written as go-intl's files or ICU's
// trees write it, "und" for the root.
func languageOf(name string) string {
	lang, _, _ := strings.Cut(strings.ReplaceAll(name, "_", "-"), "-")
	if lang == "root" {
		return "und"
	}
	return lang
}

// languages are the languages whose data is kept: plural for the plural
// rules, data for everything else. A Chromium build has plural rules for
// more languages than it has other data for. listed says whether a list of
// available.bin keeps a tag, nil for all of them.
type languages struct {
	data, plural map[string]bool
	listed       func(list, tag string) bool
	// added are the keys available.bin is to have that go-intl's does not.
	added []string
}

// cut keeps the files of the languages in langs, or of all of them if langs is
// nil, and the parts of the pools those reach. It returns the languages it
// dropped altogether and those it keeps the plural rules of only.
func cut(files map[string][]byte, langs *languages) (out map[string][]byte, dropped, pluralOnly []string, err error) {
	// A directory with a root, und.bin, holds a file per locale.
	localeDirs := map[string]bool{}
	for name := range files {
		if dir, ok := strings.CutSuffix(name, "/und.bin"); ok && !strings.Contains(dir, "/") {
			localeDirs[dir] = true
		}
	}
	pools := map[string]bool{}
	for _, p := range pooled {
		pools[p.pool] = true
		if !localeDirs[p.dir] {
			return nil, nil, nil, fmt.Errorf("the data has no %s/und.bin", p.dir)
		}
	}
	for name := range files {
		dir, _, ok := strings.Cut(name, "/")
		switch {
		case !ok:
			if strings.HasSuffix(name, "shared.bin") && !pools[name] {
				return nil, nil, nil, fmt.Errorf("%s is a pool the tool does not know: add its data set to pooled", name)
			}
		case localeDirs[dir], dir == "tz", dir == "brkitr":
		default:
			return nil, nil, nil, fmt.Errorf("%s: a directory the tool does not know", name)
		}
	}
	all, withData, err := knownLanguages(files, localeDirs)
	if err != nil {
		return nil, nil, nil, err
	}
	if langs == nil {
		langs = &languages{data: all, plural: all}
	}
	data := map[string]bool{"und": true}
	for l := range langs.data {
		data[l] = true
	}
	if err := closeOver(files, data); err != nil {
		return nil, nil, nil, err
	}
	plural := map[string]bool{}
	for l := range data {
		plural[l] = true
	}
	for l := range langs.plural {
		plural[l] = true
	}
	for l := range withData {
		switch {
		case !plural[l]:
			dropped = append(dropped, l)
		case !data[l]:
			pluralOnly = append(pluralOnly, l)
		}
	}
	sort.Strings(dropped)
	sort.Strings(pluralOnly)
	keptIn := func(dir string) func(loc string) bool {
		set := data
		if dir == "plurals" {
			set = plural
		}
		return func(loc string) bool { return set[languageOf(loc)] }
	}

	out = map[string][]byte{}
	for name, b := range files {
		dir, file, ok := strings.Cut(name, "/")
		if !ok || !localeDirs[dir] {
			out[name] = b
			continue
		}
		kept := keptIn(dir)
		loc := strings.TrimSuffix(file, ".bin")
		if loc == "same" {
			// A kept locale whose data is byte for byte another's is
			// served from that one's file, which stays whatever its
			// language.
			pairs, err := readPairs(b)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("%s: %v", name, err)
			}
			for _, p := range pairs {
				if kept(p[0].String()) {
					to := dir + "/" + p[1].String() + ".bin"
					if files[to] == nil {
						return nil, nil, nil, fmt.Errorf("%s: %s has no file %s", name, p[0], to)
					}
					out[to] = files[to]
				}
			}
			out[name] = writePairs(pairs, func(from intl.DataLocale) bool { return kept(from.String()) })
			continue
		}
		if kept(loc) {
			out[name] = b
		}
	}

	// The tables that list locales.
	if out["available.bin"], err = filterIndex(files["available.bin"], layout.Available, func(key string) bool {
		list, tag, _ := strings.Cut(key, " ")
		return langs.listed == nil || langs.listed(list, tag)
	}, langs.added...); err != nil {
		return nil, nil, nil, fmt.Errorf("available.bin: %v", err)
	}
	keyed := func(key string) bool {
		_, loc, _ := strings.Cut(key, " ")
		return data[languageOf(loc)]
	}
	if out["icufallback.bin"], err = filterIndex(files["icufallback.bin"], layout.ICUFallback, keyed); err != nil {
		return nil, nil, nil, fmt.Errorf("icufallback.bin: %v", err)
	}
	for name := range files {
		if strings.HasPrefix(name, "icutree-") {
			if out[name], err = filterIndex(files[name], layout.ICUTree, keyed); err != nil {
				return nil, nil, nil, fmt.Errorf("%s: %v", name, err)
			}
		}
	}

	for _, p := range pooled {
		var keepTables, allTables [][]byte
		for name, b := range files {
			dir, file, ok := strings.Cut(name, "/")
			if !ok || dir != p.dir || file == "same.bin" {
				continue
			}
			allTables = append(allTables, b)
			if _, ok := out[name]; ok {
				keepTables = append(keepTables, b)
			}
		}
		pool, err := prunePool(files[p.pool], p.version, p.walk, keepTables, allTables, p.orphans)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %v", p.pool, err)
		}
		out[p.pool] = pool
	}
	return out, dropped, pluralOnly, nil
}

// knownLanguages are the languages the data names anywhere: in the names of
// the files of a locale's data, in the tables of locales that share a file,
// in the lists of available locales and in ICU's trees. withData are those
// of them a service is available in.
func knownLanguages(files map[string][]byte, localeDirs map[string]bool) (all, withData map[string]bool, err error) {
	all = map[string]bool{"und": true}
	for name, b := range files {
		dir, file, ok := strings.Cut(name, "/")
		if !ok || !localeDirs[dir] {
			continue
		}
		if file != "same.bin" {
			all[languageOf(strings.TrimSuffix(file, ".bin"))] = true
			continue
		}
		pairs, err := readPairs(b)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %v", name, err)
		}
		for _, p := range pairs {
			all[p[0].Language.String()] = true
		}
	}
	indexed := func(name string, version byte, into map[string]bool) error {
		x, err := blob.ReadIndex(files[name], version)
		if err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
		for i := 0; i < x.Len(); i++ {
			k, _ := x.At(i)
			_, loc, _ := strings.Cut(string(k), " ")
			into[languageOf(loc)] = true
		}
		return nil
	}
	withData = map[string]bool{}
	if err := indexed("available.bin", layout.Available, withData); err != nil {
		return nil, nil, err
	}
	for l := range withData {
		all[l] = true
	}
	if err := indexed("icufallback.bin", layout.ICUFallback, all); err != nil {
		return nil, nil, err
	}
	for name := range files {
		if strings.HasPrefix(name, "icutree-") {
			if err := indexed(name, layout.ICUTree, all); err != nil {
				return nil, nil, err
			}
		}
	}
	return all, withData, nil
}

// closeOver adds to keep the languages a kept locale inherits from: parents
// in CLDR's table and in ICU's trees, and aliases in ICU's trees.
func closeOver(files map[string][]byte, keep map[string]bool) error {
	var edges [][2]string
	pairs, err := readPairs(files["parentlocales.bin"])
	if err != nil {
		return fmt.Errorf("parentlocales.bin: %v", err)
	}
	for _, p := range pairs {
		edges = append(edges, [2]string{p[0].String(), p[1].String()})
	}
	addIndex := func(name string, version byte, kinds ...string) error {
		x, err := blob.ReadIndex(files[name], version)
		if err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
		for i := 0; i < x.Len(); i++ {
			k, v := x.At(i)
			kind, loc, _ := strings.Cut(string(k), " ")
			for _, want := range kinds {
				if kind == want {
					edges = append(edges, [2]string{loc, string(v)})
				}
			}
		}
		return nil
	}
	if err := addIndex("icufallback.bin", layout.ICUFallback, "icuparent"); err != nil {
		return err
	}
	for name := range files {
		if strings.HasPrefix(name, "icutree-") {
			if err := addIndex(name, layout.ICUTree, "parent", "alias"); err != nil {
				return err
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, e := range edges {
			from, to := languageOf(e[0]), languageOf(e[1])
			if keep[from] && !keep[to] {
				keep[to] = true
				changed = true
			}
		}
	}
	return nil
}

// readPairs reads a table of data locale pairs (layout.Pairs).
func readPairs(b []byte) ([][2]intl.DataLocale, error) {
	const size = 2 * intl.DataLocaleSize
	if len(b) == 0 || b[0] != layout.Pairs || (len(b)-1)%size != 0 {
		return nil, errors.New("not a table of locale pairs")
	}
	var pairs [][2]intl.DataLocale
	for b = b[1:]; len(b) > 0; b = b[size:] {
		var p [2]intl.DataLocale
		if err := p[0].UnmarshalBinary(b[:intl.DataLocaleSize]); err != nil {
			return nil, err
		}
		if err := p[1].UnmarshalBinary(b[intl.DataLocaleSize:size]); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

// writePairs writes the pairs whose first locale keep keeps, in their order.
func writePairs(pairs [][2]intl.DataLocale, keep func(from intl.DataLocale) bool) []byte {
	out := []byte{layout.Pairs}
	for _, p := range pairs {
		if keep(p[0]) {
			out, _ = p[0].AppendBinary(out)
			out, _ = p[1].AppendBinary(out)
		}
	}
	return out
}

// filterIndex writes an index (blob.BuildIndex) again with the records keep
// keeps.
func filterIndex(b []byte, version byte, keep func(key string) bool, add ...string) ([]byte, error) {
	x, err := blob.ReadIndex(b, version)
	if err != nil {
		return nil, err
	}
	records := map[string][]byte{}
	for i := 0; i < x.Len(); i++ {
		k, v := x.At(i)
		if keep(string(k)) {
			records[string(k)] = v
		}
	}
	for _, k := range add {
		records[k] = nil
	}
	return blob.BuildIndex(version, records)
}

// prunePool empties the parts of a pool the tables in keep do not reach. It
// fails unless the tables in all leave exactly orphans parts unreached: more
// means walk does not read all that a table refers to.
func prunePool(b []byte, version byte, walk walkFunc, keep, all [][]byte, orphans int) ([]byte, error) {
	pool, err := blob.ReadShared(b, version)
	if err != nil {
		return nil, err
	}
	n := partCount(b)
	reached := func(tables [][]byte) (map[int]bool, error) {
		blob.StartWalk()
		for _, t := range tables {
			if err := walk(t, pool); err != nil {
				return nil, err
			}
		}
		seen := map[int]bool{}
		for i := range blob.Walked(b) {
			seen[i] = true
		}
		return seen, nil
	}
	everything, err := reached(all)
	if err != nil {
		return nil, err
	}
	if n-len(everything) != orphans {
		var text []string
		for i := 0; i < n && len(text) < 20; i++ {
			if !everything[i] {
				text = append(text, fmt.Sprintf("%d %q", i, partText(b, n, i)))
			}
		}
		return nil, fmt.Errorf("%d of %d parts no locale reaches, not %d: a Walk does not read all its data package reads, or the orphans in the pool changed: %s",
			n-len(everything), n, orphans, strings.Join(text, ", "))
	}
	used, err := reached(keep)
	if err != nil {
		return nil, err
	}

	// The layout of blob.Pool.Bytes, with the parts not used left empty.
	out := append([]byte(nil), b[:5]...)
	var body []byte
	for i := 0; i < n; i++ {
		if used[i] {
			body = append(body, partText(b, n, i)...)
		}
		out = binary.LittleEndian.AppendUint32(out, uint32(len(body)))
	}
	out = append(out, body...)

	// What the kept tables read is what they read before.
	pruned, err := blob.ReadShared(out, version)
	if err != nil {
		return nil, err
	}
	blob.StartWalk()
	for _, t := range keep {
		if err := walk(t, pruned); err != nil {
			return nil, fmt.Errorf("a kept locale does not read from the pruned pool: %v", err)
		}
	}
	for i := range blob.Walked(out) {
		if !used[i] {
			return nil, fmt.Errorf("part %d is read from the pruned pool only", i)
		}
	}
	return out, nil
}

// partCount is the number of parts in a pool blob.ReadShared accepted.
func partCount(b []byte) int { return int(binary.LittleEndian.Uint32(b[1:])) }

// partText is part i of a pool of n parts.
func partText(b []byte, n, i int) []byte {
	end := func(i int) int { return int(binary.LittleEndian.Uint32(b[5+4*i:])) }
	start := 0
	if i > 0 {
		start = end(i - 1)
	}
	data := b[5+4*n:]
	return data[start:end(i)]
}

// chunkFiles groups files into chunks: a file of bigFile bytes or more is a
// chunk of its own, and the smaller files of a top-level directory share one.
// The files in stored are not compressed.
func chunkFiles(files map[string][]byte, stored map[string]bool) []qjspack.Chunk {
	dirs := map[string][]string{}
	for name := range files {
		dir, _, ok := strings.Cut(name, "/")
		if !ok {
			dir = ""
		}
		dirs[dir] = append(dirs[dir], name)
	}
	var order []string
	for dir := range dirs {
		order = append(order, dir)
	}
	sort.Strings(order)
	var chunks []qjspack.Chunk
	for _, dir := range order {
		names := dirs[dir]
		sort.Strings(names)
		var small qjspack.Chunk
		small.Deflate = true
		for _, name := range names {
			f := qjspack.File{Name: name, Data: files[name]}
			if len(f.Data) >= bigFile || stored[name] {
				chunks = append(chunks, qjspack.Chunk{Deflate: !stored[name], Files: []qjspack.File{f}})
				continue
			}
			small.Files = append(small.Files, f)
		}
		if len(small.Files) > 0 {
			chunks = append(chunks, small)
		}
	}
	return chunks
}
