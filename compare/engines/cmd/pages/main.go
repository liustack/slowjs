// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command pages makes the page tables and the archive of a run of
// tifa/run.sh.
//
//	go run ./cmd/pages <run dir> --output <results/run-pages>
//
// Without --output it writes the Markdown to the standard output. With it,
// it writes <stem>.md and <stem>.json, outside the raw run directory.
//
// Pages are compared after the router's update times (`u:` and 13 digits, the
// Date.now() of the render) are masked: they differ between any two renders.
// Times are per render: the mean of the CPU time of the timed renders where
// the engine reports it, else the mean wall time (the C QuickJS rows, timed in
// the script on a CPU of their own). Ratios are against slowjs on the same
// page, as a geometric mean over the pages with the range in brackets.
//
// slowjs-tifa is slowjs set up the way tifa sets it up (own Intl and Temporal,
// tifa's Go functions, no polyfill), so it has no pf runs, and so is
// slowjs-tifa-nogoasm, built with slowjs_nogoasm. Its pages are compared with
// the pages of slowjs on the polyfill, where the Intl output of the two may
// differ in U+202F (see tifaRowCheck).
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
)

var (
	goEngines = []string{"slowjs", "slowjs-tifa", "slowjs-tifa-nogoasm", "modernc-quickjs", "go-quickjs-interp", "go-quickjs-jit", "goant-interp", "goant-jit", "moejs"}
	refEngine = []string{"quickjs-2026-06-04", "quickjs-master", "quickjs-ng", "node-jit", "node-jitless"}
	ownIntl   = []string{"slowjs", "slowjs-tifa", "slowjs-tifa-nogoasm", "go-quickjs-interp", "go-quickjs-jit", "node-jit", "node-jitless"}
	label     = map[string]string{
		"quickjs-2026-06-04": "C QuickJS 2026-06-04 (reference)",
		"quickjs-master":     "C QuickJS master 7d1c13c5d8 (reference)",
		"quickjs-ng":         "QuickJS-NG v0.17.0 (reference)",
		"node-jit":           "Node v24.11.1 (reference)",
		"node-jitless":       "Node v24.11.1 --jitless (reference)",
	}
	apps = []string{"bench", "mui", "shop"}
)

// notRun says why a run is missing, for the runs left out of run.sh.
func notRun(engine, kind string) string {
	if (engine == "slowjs-tifa" || engine == "slowjs-tifa-nogoasm") && (kind == "pf" || kind == "mui-pf") {
		return "not run: this row has no polyfill"
	}
	return "not run"
}

// fatal is a failure to read the run: a file that is not what the tool
// expects. It ends the program with the message.
type fatal struct{ msg string }

func fail(format string, a ...any) { panic(fatal{fmt.Sprintf(format, a...)}) }

var updateTime = regexp.MustCompile(`\bu:\d{13}\b`)

func mask(b []byte) []byte { return updateTime.ReplaceAllLiteral(b, []byte("u:0")) }

// run is a directory of raw results.
type run struct{ dir string }

// get returns the value of key k of the object v, nil when it has none.
func get(v any, k string) any {
	o, ok := v.(*jobj)
	if !ok {
		fail("%v is not an object, looking for %q", v, k)
	}
	return o.m[k]
}

// need is get for a key that must be there.
func need(v any, k string) any {
	o, ok := v.(*jobj)
	if !ok {
		fail("%v is not an object, looking for %q", v, k)
	}
	x, ok := o.m[k]
	if !ok {
		fail("no %q in the result", k)
	}
	return x
}

// orObj is `v or {}`.
func orObj(v any) any {
	if !truthy(v) {
		return newObj()
	}
	return v
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case jint:
		return x != "0"
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case *jobj:
		return len(x.keys) > 0
	}
	return true
}

func num(v any) float64 {
	switch x := v.(type) {
	case jint:
		var f float64
		fmt.Sscan(string(x), &f)
		return f
	case float64:
		return x
	}
	fail("%v is not a number", v)
	return 0
}

// numOr is `v or 0` as a number.
func numOr(v any) float64 {
	if !truthy(v) {
		return 0
	}
	return num(v)
}

func runes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// load reads <name>.json: nil when there is none or it is empty, and an error
// object when the process did not finish writing it.
func (r *run) load(name string) any {
	p := filepath.Join(r.dir, name+".json")
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		fail("%v", err)
	}
	v, err := parseJSON(string(data))
	if err != nil {
		o := newObj()
		o.set("err", "no result (the process did not finish)")
		return o
	}
	return v
}

// pmap is the pages of a result in the order of the file, by name.
type pmap struct {
	keys []string
	m    map[string]*jobj
}

// pages is the pages of a result, by the name of their input.
func pages(r any) *pmap {
	out := &pmap{m: map[string]*jobj{}}
	if r == nil {
		return out
	}
	list, _ := get(r, "pages").([]any)
	for _, e := range list {
		pg, ok := e.(*jobj)
		if !ok {
			fail("a page is not an object")
		}
		in, ok := need(pg, "input").(string)
		if !ok {
			fail("the input of a page is not a string")
		}
		k := runes(in, len([]rune(in))-5)
		if _, dup := out.m[k]; !dup {
			out.keys = append(out.keys, k)
		}
		out.m[k] = pg
	}
	return out
}

func (p *pmap) sortedKeys() []string {
	k := slices.Clone(p.keys)
	sort.Strings(k)
	return k
}

// loadMerged is load, with the pages that failed taken from <name>-rest when
// there is one: a run without the pages that stopped the engine, in a process
// of its own (an engine that fails one page can leave its VM unusable).
func (r *run) loadMerged(name string) any {
	res := r.load(name)
	rest := r.load(name + "-rest")
	if !truthy(res) || !truthy(rest) {
		return res
	}
	got := pages(rest)
	merged := res.(*jobj).clone()
	var list []any
	if l, ok := merged.m["pages"].([]any); ok {
		list = l
	}
	var newPages []any
	for _, e := range list {
		pg := e.(*jobj)
		if truthy(get(pg, "ok")) {
			newPages = append(newPages, pg)
		} else if g, ok := got.m[inputName(pg)]; ok {
			newPages = append(newPages, g)
		} else {
			newPages = append(newPages, pg)
		}
	}
	merged.set("pages", orEmpty(newPages))
	// The pages left out of the rest run: the ones that stop the engine.
	failed := []any{}
	for _, e := range need(res, "pages").([]any) {
		pg := e.(*jobj)
		if !truthy(get(pg, "ok")) {
			if _, ok := got.m[inputName(pg)]; !ok {
				failed = append(failed, inputName(pg))
			}
		}
	}
	merged.set("failed", failed)
	return merged
}

func orEmpty(l []any) []any {
	if l == nil {
		return []any{}
	}
	return l
}

func inputName(pg *jobj) string {
	in := need(pg, "input").(string)
	return runes(in, len([]rune(in))-5)
}

// meta reads <name>.meta: a JSON object a line, one for each try.
func (r *run) meta(name string) []any {
	data, err := os.ReadFile(filepath.Join(r.dir, name+".meta"))
	if err != nil {
		return []any{}
	}
	out := []any{}
	for _, l := range pySplitLines(string(data)) {
		if strings.TrimFunc(l, pyIsSpace) == "" {
			continue
		}
		v, err := parseJSON(l)
		if err != nil {
			fail("%s.meta: %v", name, err)
		}
		out = append(out, v)
	}
	return out
}

// html is the page with its update times masked, if the file is there.
type html struct {
	b  []byte
	ok bool
}

func (r *run) html(engine, tag, page string) html {
	return r.read(filepath.Join("html", fmt.Sprintf("%s-%s-%s.html", engine, tag, page)))
}

func (r *run) read(rel string) html {
	b, err := os.ReadFile(filepath.Join(r.dir, rel))
	if err != nil {
		return html{}
	}
	return html{mask(b), true}
}

// equality is what the archive keeps of two pages: the digest of each and
// whether they are the same. A page that is not there has no digest and no
// verdict.
type equality struct {
	actual, ref html
}

func (e equality) sum(h html) any {
	if !h.ok {
		return nil
	}
	s := sha256.Sum256(h.b)
	return hex.EncodeToString(s[:])
}

// same is true, false or nil.
func (e equality) same() any {
	if !e.actual.ok || !e.ref.ok {
		return nil
	}
	return bytes.Equal(e.actual.b, e.ref.b)
}

func (e equality) object() map[string]any {
	return map[string]any{"normalized_sha256": e.sum(e.actual), "reference_sha256": e.sum(e.ref), "same_as_liustack": e.same()}
}

func isTrue(v any) bool { b, ok := v.(bool); return ok && b }

// kind says which run of an engine a table reads: the same for every engine,
// or by engine.
type kind struct {
	fixed string
	by    func(engine string) string
}

func (k kind) of(engine string) string {
	if k.by != nil {
		return k.by(engine)
	}
	return k.fixed
}

func fixed(s string) kind { return kind{fixed: s} }

func asDeployed(own, polyfill string) kind {
	return kind{by: func(e string) string {
		if slices.Contains(ownIntl, e) {
			return own
		}
		return polyfill
	}}
}

// comparisons compares the pages of every engine with those of slowjs, the
// kind of base (or the engine's own kind) being the run of slowjs.
func (r *run) comparisons(k kind, baseKind string) map[string]map[string]equality {
	result := map[string]map[string]equality{}
	for _, e := range append(slices.Clone(goEngines), refEngine...) {
		kk := k.of(e)
		base := baseKind
		if base == "" {
			base = kk
		}
		ps := pages(r.loadMerged(e + "-" + kk))
		refs := pages(r.loadMerged("slowjs-" + base))
		m := map[string]equality{}
		for _, pg := range union(ps, refs) {
			var eq equality
			if v, ok := ps.m[pg]; ok && truthy(get(v, "ok")) {
				eq.actual = r.html(e, kk, pg)
			}
			if v, ok := refs.m[pg]; ok && truthy(get(v, "ok")) {
				eq.ref = r.html("slowjs", base, pg)
			}
			m[pg] = eq
		}
		result[e] = m
	}
	return result
}

func union(a, b *pmap) []string {
	set := map[string]bool{}
	for k := range a.m {
		set[k] = true
	}
	for k := range b.m {
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type tifaKind struct{ app, tifa, polyfill string }

var tifaKinds = []tifaKind{{"bench", "bench", "bench"}, {"mui", "mui-native", "mui-pf"}, {"shop", "native", "pf"}}

var narrowSpace = []byte(" ")

// tifaRowEvidence compares the pages of slowjs-tifa with those of slowjs on
// the polyfill (the same JavaScript on another Intl). `same` is byte for byte
// after masking. `same_but_u202f` says the pages are the same once the narrow
// no-break space U+202F, which the FormatJS polyfill writes before AM and PM
// where Go's Intl and Node write a plain space, is a plain space.
func (r *run) tifaRowEvidence() (keys []string, result map[string]map[string]any) {
	result = map[string]map[string]any{}
	for _, tk := range tifaKinds {
		ps := pages(r.loadMerged("slowjs-tifa-" + tk.tifa))
		refs := pages(r.loadMerged("slowjs-" + tk.polyfill))
		for _, pg := range union(ps, refs) {
			var eq equality
			if v, ok := ps.m[pg]; ok && truthy(get(v, "ok")) {
				eq.actual = r.html("slowjs-tifa", tk.tifa, pg)
			}
			if v, ok := refs.m[pg]; ok && truthy(get(v, "ok")) {
				eq.ref = r.html("slowjs", tk.polyfill, pg)
			}
			e := eq.object()
			delete(e, "same_as_liustack")
			e["same"] = eq.same()
			e["same_but_u202f"] = nil
			if eq.actual.ok && eq.ref.ok {
				e["same_but_u202f"] = bytes.Equal(bytes.ReplaceAll(eq.actual.b, narrowSpace, []byte(" ")), bytes.ReplaceAll(eq.ref.b, narrowSpace, []byte(" ")))
			}
			e["u202f_in_polyfill_page"], e["u202f_in_tifa_page"] = nil, nil
			if eq.ref.ok {
				e["u202f_in_polyfill_page"] = bytes.Count(eq.ref.b, narrowSpace)
			}
			if eq.actual.ok {
				e["u202f_in_tifa_page"] = bytes.Count(eq.actual.b, narrowSpace)
			}
			key := tk.app + "/" + pg
			keys = append(keys, key)
			result[key] = e
		}
	}
	return keys, result
}

func (r *run) checkEquality(app, page string) equality {
	read := func(tag string) html {
		return r.read(filepath.Join("check", fmt.Sprintf("slowjs-%s-%s-%s.html", app, tag, page)))
	}
	return equality{read("stub"), read("normal")}
}

// archive keeps every process result, including rest runs, single-page probes
// and metadata for processes that did not produce JSON. It never replaces raw
// rows.
func (r *run) archive() map[string]any {
	names := map[string]bool{}
	err := filepath.WalkDir(r.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != r.dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".meta")) {
			rel, _ := filepath.Rel(r.dir, p)
			names[filepath.ToSlash(rel[:strings.LastIndex(rel, ".")])] = true
		}
		return nil
	})
	if err != nil {
		fail("%v", err)
	}
	result := map[string]any{}
	for name := range names {
		result[name] = map[string]any{"result": r.load(name), "tries": r.meta(name)}
	}
	cmps := map[string]any{}
	for _, c := range []struct {
		name string
		k    kind
		base string
	}{
		{"bench", fixed("bench"), ""},
		{"pf", fixed("pf"), ""},
		{"deployed", asDeployed("native", "pf"), "native"},
		{"mui_pf", fixed("mui-pf"), ""},
		{"mui_deployed", asDeployed("mui-native", "mui-pf"), "mui-native"},
	} {
		byEngine := map[string]any{}
		for e, m := range r.comparisons(c.k, c.base) {
			pm := map[string]any{}
			for pg, eq := range m {
				pm[pg] = eq.object()
			}
			byEngine[e] = pm
		}
		cmps[c.name] = byEngine
	}
	_, evidence := r.tifaRowEvidence()
	tifa := map[string]any{}
	for k, v := range evidence {
		tifa[k] = v
	}
	checks := map[string]any{}
	for _, app := range apps {
		m := map[string]any{}
		for _, pg := range pages(r.load("check/" + app + "-normal")).sortedKeys() {
			m[pg] = r.checkEquality(app, pg).object()
		}
		checks[app] = m
	}
	result["_html"] = map[string]any{
		"normalization":    map[string]any{"version": 1, "pattern": `\bu:\d{13}\b`, "replacement": "u:0", "digest": "sha256"},
		"comparisons":      cmps,
		"tifa_vs_polyfill": tifa,
		"checks":           checks,
	}
	return result
}

// perRender is the time of one render in milliseconds, if the page ran.
func perRender(p *jobj) (float64, bool) {
	if !truthy(get(p, "ok")) {
		return 0, false
	}
	if c := get(p, "cpu_ms_per_op"); truthy(c) {
		return num(c), true
	}
	return num(need(need(p, "render"), "mean_ns")) / 1e6, true
}

// times is the time of a render of each page, in the order of the file.
type times struct {
	keys []string
	v    map[string]float64
	ok   map[string]bool
}

func timesOf(pm *pmap) times {
	t := times{keys: pm.keys, v: map[string]float64{}, ok: map[string]bool{}}
	for _, k := range pm.keys {
		t.v[k], t.ok[k] = perRender(pm.m[k])
	}
	return t
}

// positive keeps the pages with a time other than zero.
func (t times) positive() times {
	out := times{v: map[string]float64{}, ok: map[string]bool{}}
	for _, k := range t.keys {
		if t.ok[k] && t.v[k] != 0 {
			out.keys = append(out.keys, k)
			out.v[k], out.ok[k] = t.v[k], true
		}
	}
	return out
}

func geoMean(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += math.Log(x)
	}
	return math.Exp(s / float64(len(xs)))
}

// f is a time for a table: three digits, or none after the point from 100.
func f(x float64) string {
	if x < 100 {
		return pyG(x, 3)
	}
	return fmt.Sprintf("%.0f", x)
}

func ratioCell(rows, base times) string {
	var rs []float64
	for _, k := range rows.keys {
		if base.ok[k] && base.v[k] != 0 && rows.v[k] != 0 {
			rs = append(rs, rows.v[k]/base.v[k])
		}
	}
	if len(rs) == 0 {
		return ""
	}
	return fmt.Sprintf("%.2f (%.2f–%.2f)", geoMean(rs), slices.Min(rs), slices.Max(rs))
}

func why(r any, ps *pmap) string {
	if r == nil {
		return "not run"
	}
	if e := get(r, "err"); truthy(e) {
		first, _, _ := strings.Cut(e.(string), "\n")
		return runes(first, 160)
	}
	var bad []string
	for _, k := range ps.keys {
		v := ps.m[k]
		if !truthy(get(v, "ok")) {
			msg, _ := get(v, "err").(string)
			bad = append(bad, k+": "+runes(msg, 120))
		}
	}
	if len(bad) > 3 {
		bad = bad[:3]
	}
	return strings.Join(bad, "; ")
}

func (r *run) disturbed(name string) bool {
	m := r.meta(name)
	return len(m) > 0 && truthy(get(m[len(m)-1], "disturbed"))
}

// report collects the lines of the Markdown.
type report struct{ strings.Builder }

func (w *report) println(s string)               { w.WriteString(s + "\n") }
func (w *report) printf(format string, a ...any) { w.println(fmt.Sprintf(format, a...)) }

func (r *run) check(w *report) {
	w.println("### Pages without Intl\n")
	w.println("| App | Page | Same page with Intl removed | Intl it reaches (first caller) |")
	w.println("|---|---|---|---|")
	for _, app := range apps {
		n := r.load("check/" + app + "-normal")
		s := orObj(r.load("check/" + app + "-stub"))
		for _, page := range pages(n).sortedKeys() {
			var same string
			if e := get(s, "err"); truthy(e) {
				same = "no: " + runes(e.(string), 80)
			} else {
				switch r.checkEquality(app, page).same() {
				case true:
					same = "yes"
				case false:
					same = "no"
				default:
					same = "not archived"
				}
			}
			pr := orObj(r.load("check/" + app + "-proxy-" + page))
			var reach []string
			if pg, ok := pages(pr).m[page]; ok {
				if probe := get(pg, "probe"); truthy(probe) {
					hits, err := parseJSON(probe.(string))
					if err != nil {
						fail("the probe of %s %s: %v", app, page, err)
					}
					o, ok := hits.(*jobj)
					if !ok {
						fail("the probe of %s %s is not an object", app, page)
					}
					for _, k := range o.keys {
						reach = append(reach, fmt.Sprintf("`%s` ×%s", k, pyStr(need(o.m[k], "n"))))
					}
				}
			}
			cell := strings.Join(reach, ", ")
			if cell == "" {
				cell = "nothing"
			}
			w.printf("| %s | %s | %s | %s |", app, page, same, cell)
		}
	}
	w.println("")
}

func (r *run) table(w *report, k kind, title string, engines []string, baseKind string) {
	bk := baseKind
	if bk == "" {
		bk = k.fixed
	}
	base := timesOf(pages(r.loadMerged("slowjs-" + bk)))
	evidence := r.comparisons(k, baseKind)
	w.printf("### %s\n", title)
	w.println("| Engine | Load ms | Pages that render | Same page as slowjs | Time per render vs slowjs |")
	w.println("|---|---|---|---|---|")
	for _, e := range engines {
		kk := k.of(e)
		res := r.loadMerged(e + "-" + kk)
		name := e
		if l, ok := label[e]; ok {
			name = l
		}
		if k.by != nil {
			if strings.HasSuffix(kk, "native") {
				name += " (own Intl)"
			} else {
				name += " (polyfill)"
			}
		}
		if res == nil {
			w.printf("| %s | %s | | | |", name, notRun(e, kk))
			continue
		}
		ps := pages(res)
		rows := timesOf(ps).positive()
		if len(rows.keys) == 0 {
			w.printf("| %s | %s | 0 of %d | | |", name, why(res, ps), len(base.keys))
			continue
		}
		same, missing := 0, 0
		for _, pg := range rows.keys {
			switch evidence[e][pg].same() {
			case true:
				same++
			case nil:
				missing++
			}
		}
		loadMs := numOr(get(res, "load_ms"))
		if l, ok := get(res, "prelude_ms").([]any); ok {
			for _, x := range l {
				loadMs += num(x)
			}
		}
		note := ""
		if r.disturbed(e + "-" + kk) {
			note = " (disturbed)"
		}
		if missing > 0 {
			note += fmt.Sprintf(". HTML unavailable for %d pages", missing)
		}
		if fl := get(res, "failed"); truthy(fl) {
			var fails []string
			for _, x := range fl.([]any) {
				fails = append(fails, x.(string))
			}
			var which string
			if float64(len(fails)) > float64(len(base.keys))/2 {
				var rest []string
				for _, p := range base.keys {
					if !slices.Contains(fails, p) {
						rest = append(rest, p)
					}
				}
				sort.Strings(rest)
				which = "every page but " + strings.Join(rest, ", ") + " fails"
			} else {
				which = strings.Join(fails, ", ") + " fail"
			}
			note += fmt.Sprintf(". %s, a failure leaves the runtime closed, the other pages measured in a run without the failing ones", which)
		}
		ok := 0
		for _, pg := range ps.keys {
			if truthy(get(ps.m[pg], "ok")) {
				ok++
			}
		}
		w.printf("| %s | %.0f | %d of %d | %d of %d | %s%s |", name, loadMs, ok, len(base.keys), same, len(rows.keys), ratioCell(rows, base), note)
	}
	w.println("")
}

func (r *run) perPage(w *report, k kind, engines []string, baseKind string) {
	bk := baseKind
	if bk == "" {
		bk = k.fixed
	}
	base := timesOf(pages(r.load("slowjs-" + bk)))
	w.println("| Page | " + strings.Join(engines, " | ") + " |")
	w.println("|---|" + strings.Repeat("---|", len(engines)))
	rs := map[string]times{}
	for _, e := range engines {
		rs[e] = timesOf(pages(r.loadMerged(e + "-" + k.of(e))))
	}
	keys := slices.Clone(base.keys)
	sort.Strings(keys)
	for _, pg := range keys {
		var cells []string
		for _, e := range engines {
			if !rs[e].ok[pg] {
				cells = append(cells, "")
				continue
			}
			x := rs[e].v[pg]
			if !base.ok[pg] || base.v[pg] == 0 {
				fail("page %s: the slowjs time is missing, cannot give a ratio", pg)
			}
			cells = append(cells, fmt.Sprintf("%s (%.2f)", f(x), x/base.v[pg]))
		}
		w.println("| " + pg + " | " + strings.Join(cells, " | ") + " |")
	}
	w.println("")
}

func (r *run) tifaRowCheck(w *report) {
	w.println("### slowjs-tifa against the same JavaScript on the polyfill\n")
	w.println("| Bundle | Pages | Same HTML | Same but for U+202F | Other difference | Not compared |")
	w.println("|---|---|---|---|---|---|")
	keys, ev := r.tifaRowEvidence()
	for _, tk := range tifaKinds {
		n, same, u202f, other, missing := 0, 0, 0, 0, 0
		for _, key := range keys {
			if !strings.HasPrefix(key, tk.app+"/") {
				continue
			}
			v := ev[key]
			n++
			switch s := v["same"]; {
			case isTrue(s):
				same++
			case s == nil:
				missing++
			case isTrue(v["same_but_u202f"]):
				u202f++
			default:
				other++
			}
		}
		w.printf("| %s | %d | %d | %d | %d | %d |", tk.app, n, same, u202f, other, missing)
	}
	w.println("")
}

// vsC gives, per bundle, every row as a ratio to C QuickJS 2026-06-04 on the
// same pages.
func (r *run) vsC(w *report) {
	pf := map[string]string{"bench": "bench", "mui": "mui-pf", "shop": "pf"}
	own := map[string]string{"bench": "bench", "mui": "mui-native", "shop": "native"}
	rows := []struct {
		label, engine string
		kinds         map[string]string
	}{
		{"slowjs, same JavaScript (polyfill)", "slowjs", pf},
		{"slowjs, own Intl only", "slowjs", own},
		{"slowjs-tifa, as tifa runs it", "slowjs-tifa", own},
		{"slowjs-tifa-nogoasm, as tifa runs it with build.goasm off", "slowjs-tifa-nogoasm", own},
		{"C QuickJS 2026-06-04", "quickjs-2026-06-04", pf},
		{"Node v24.11.1", "node-jit", pf},
		{"Node v24.11.1 --jitless", "node-jitless", pf},
	}
	w.println("### Per bundle, against C QuickJS 2026-06-04\n")
	w.println("Geometric mean over the pages of a bundle, range in brackets. Below 1 is faster than C. Bench has no Intl, MUI and the shop need it: every row but slowjs-tifa and the \"own Intl only\" row runs them on the polyfill.\n")
	w.println("| Row | bench | MUI | shop |")
	w.println("|---|---|---|---|")
	for _, row := range rows {
		var cells []string
		for _, app := range apps {
			base := timesOf(pages(r.loadMerged("quickjs-2026-06-04-" + pf[app])))
			mine := timesOf(pages(r.loadMerged(row.engine + "-" + row.kinds[app]))).positive()
			cell := ""
			if len(base.keys) > 0 && len(mine.keys) > 0 {
				cell = ratioCell(mine, base)
			}
			cells = append(cells, cell)
		}
		w.println("| " + row.label + " | " + strings.Join(cells, " | ") + " |")
	}
	w.println("")
}

func (r *run) polyfillCost(w *report) {
	w.println("### What the polyfill costs slowjs\n")
	pf, nat := r.load("slowjs-pf"), r.load("slowjs-native")
	prelude := 0.0
	for _, x := range need(pf, "prelude_ms").([]any) {
		prelude += num(x)
	}
	w.printf("- Loading it: %.0f ms per VM, before the bundle (%.0f ms after it, %.0f ms without it).", prelude, num(need(pf, "load_ms")), num(need(nat, "load_ms")))
	for _, k := range []string{"pf", "native"} {
		m := orObj(get(orObj(r.load("slowjs-mem-"+k)), "mem"))
		g := orObj(get(orObj(r.load("go-quickjs-interp-mem-"+k)), "mem"))
		what := "own Intl"
		if k == "pf" {
			what = "polyfill"
		}
		w.printf("- Memory per VM, %s: slowjs %.1f MiB RSS, go-quickjs-interp %.1f MiB RSS (%.1f MiB Go heap)", what,
			numOr(get(m, "rss_per_vm_bytes"))/(1<<20), numOr(get(g, "rss_per_vm_bytes"))/(1<<20), numOr(get(g, "go_heap_per_vm_bytes"))/(1<<20))
	}
	for _, t := range []struct{ k, what string }{{"timer-pf", "the polyfill"}, {"timer-native", "its own Intl"}} {
		res := r.load("slowjs-" + t.k)
		if !truthy(res) {
			continue
		}
		tot, ren, calls := 0.0, 0.0, 0.0
		for _, e := range need(res, "pages").([]any) {
			if !truthy(get(e, "ok")) {
				continue
			}
			probe, err := parseJSON(need(e, "probe_end").(string))
			if err != nil {
				fail("probe_end: %v", err)
			}
			n := num(need(need(e, "render"), "n")) + 20 // the probe covers the warm renders too
			tot += num(need(probe, "ms")) / n
			calls += num(need(probe, "calls")) / n
			ren += num(need(e, "cpu_ms_per_op"))
		}
		if ren == 0 {
			fail("no render time in slowjs-%s", t.k)
		}
		w.printf("- Time inside %s: %.1f ms of %.1f ms per render summed over the pages (%.0f%%), %.0f calls.", t.what, tot, ren, 100*tot/ren, calls)
	}
	w.println("")
}

// markdown makes the tables of the run.
func (r *run) markdown() string {
	w := &report{}
	r.check(w)
	engines := append(slices.Clone(goEngines), refEngine...)
	deployed := asDeployed("native", "pf")
	muiDeployed := asDeployed("mui-native", "mui-pf")
	r.table(w, fixed("bench"), "Pages without Intl (bench)", engines, "")
	r.perPage(w, fixed("bench"), engines, "")
	r.table(w, fixed("pf"), "Shop on the FormatJS polyfill, every engine", engines, "")
	r.perPage(w, fixed("pf"), engines, "")
	r.table(w, deployed, "Shop as deployed: own Intl where the engine has one", engines, "native")
	r.perPage(w, deployed, engines, "native")
	r.table(w, fixed("mui-pf"), "MUI pages on the FormatJS polyfill, every engine", engines, "")
	r.perPage(w, fixed("mui-pf"), engines, "")
	r.table(w, muiDeployed, "MUI pages as deployed: own Intl where the engine has one", engines, "mui-native")
	r.perPage(w, muiDeployed, engines, "mui-native")
	r.tifaRowCheck(w)
	r.vsC(w)
	r.polyfillCost(w)
	lines := pySplitLines(w.String())
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(l, pyIsSpace)
	}
	return strings.TrimRightFunc(strings.Join(lines, "\n"), pyIsSpace) + "\n"
}

// pyIsSpace is Python's str.isspace for one character.
func pyIsSpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }

// pySplitLines is Python's str.splitlines().
func pySplitLines(s string) []string {
	var out []string
	rs := []rune(s)
	start := 0
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\x85', ' ', ' ':
		case '\r':
			out = append(out, string(rs[start:i]))
			if i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
			start = i + 1
			continue
		default:
			continue
		}
		out = append(out, string(rs[start:i]))
		start = i + 1
	}
	if start < len(rs) {
		out = append(out, string(rs[start:]))
	}
	return out
}

func main() {
	var dir, output string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--output" || a == "-output":
			if i+1 >= len(args) {
				usage("--output needs a value")
			}
			i++
			output = args[i]
		case strings.HasPrefix(a, "--output="):
			output = strings.TrimPrefix(a, "--output=")
		case strings.HasPrefix(a, "-") && a != "-":
			usage("unknown option " + a)
		case dir == "":
			dir = a
		default:
			usage("unexpected argument " + a)
		}
	}
	if dir == "" {
		usage("the run directory is missing")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		usage(err.Error())
	}
	if output != "" {
		o, err := filepath.Abs(output)
		if err != nil {
			usage(err.Error())
		}
		if o == abs || strings.HasPrefix(o, abs+string(filepath.Separator)) {
			usage("--output must be outside the raw run directory")
		}
	}
	defer func() {
		if e := recover(); e != nil {
			if f, ok := e.(fatal); ok {
				fmt.Fprintln(os.Stderr, "pages:", f.msg)
				os.Exit(1)
			}
			panic(e)
		}
	}()
	r := &run{dir: abs}
	md := r.markdown()
	if output == "" {
		fmt.Print(md)
		return
	}
	var b strings.Builder
	writeJSON(&b, r.archive(), 0)
	b.WriteString("\n")
	if err := os.WriteFile(output+".json", []byte(b.String()), 0o644); err != nil {
		fail("%v", err)
	}
	if err := os.WriteFile(output+".md", []byte(md), 0o644); err != nil {
		fail("%v", err)
	}
}

func usage(msg string) {
	fmt.Fprintln(os.Stderr, "usage: pages <run dir> [--output <results/run-pages>]")
	fmt.Fprintln(os.Stderr, "pages:", msg)
	os.Exit(2)
}
