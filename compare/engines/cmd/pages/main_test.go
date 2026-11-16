// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// result writes <name>.json: a run of pages, each with a CPU time.
func result(t *testing.T, dir, name string, ok bool, pageNames ...string) {
	t.Helper()
	var ps []string
	for _, p := range pageNames {
		ps = append(ps, `{"input": "`+p+`.json", "ok": `+map[bool]string{true: "true", false: "false"}[ok]+`, "cpu_ms_per_op": 1}`)
	}
	write(t, dir, name+".json", `{"pages": [`+strings.Join(ps, ",")+`]}`)
}

func getPath(t *testing.T, v any, path ...string) any {
	t.Helper()
	for _, k := range path {
		o, ok := v.(*jobj)
		if !ok {
			t.Fatalf("%v is not an object at %q", v, k)
		}
		v = o.m[k]
	}
	return v
}

// archiveOf runs archive and reads it back through the JSON writer.
func archiveOf(t *testing.T, dir string) *jobj {
	t.Helper()
	var b strings.Builder
	writeJSON(&b, (&run{dir: dir}).archive(), 0)
	v, err := parseJSON(b.String())
	if err != nil {
		t.Fatal(err)
	}
	return v.(*jobj)
}

func TestArchiveKeepsRawRunsAndHTMLEvidence(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"liustack-slowjs-pf", "liustack-slowjs-native", "hermes-pf"} {
		write(t, dir, name+".json", `{"pages": [{"input": "one.json", "ok": true, "cpu_ms_per_op": 1}, {"input": "missing.json", "ok": false}]}`)
	}
	for _, name := range []string{"go-quickjs-interp-native"} {
		write(t, dir, name+".json", `{"pages": [{"input": "one.json", "ok": false, "cpu_ms_per_op": 1}, {"input": "missing.json", "ok": false}]}`)
	}
	result(t, dir, "go-quickjs-interp-native-rest", true, "one")
	result(t, dir, "probe-one", true, "one")
	write(t, dir, "probe-one.meta", `{"try":0,"disturbed":1}`+"\n"+`{"try":1,"disturbed":0}`+"\n")
	for name, body := range map[string]string{
		"liustack-slowjs-pf":       "<p>u:1234567890123</p>",
		"liustack-slowjs-native":   "<p>u:1234567890124</p>",
		"go-quickjs-interp-native": "<p>u:1234567890125</p>",
		"hermes-pf":                "<p><!-- -->u:1234567890126</p>",
	} {
		write(t, dir, "html/"+name+"-one.html", body)
	}
	a := archiveOf(t, dir)
	if getPath(t, a, "go-quickjs-interp-native-rest") == nil {
		t.Error("the rest run is not archived")
	}
	if tries := getPath(t, a, "probe-one", "tries").([]any); len(tries) != 2 {
		t.Errorf("tries = %v, want 2", tries)
	}
	if ok := getPath(t, a, "go-quickjs-interp-native", "result", "pages").([]any)[0].(*jobj).m["ok"]; ok != false {
		t.Errorf("the raw page of the engine was replaced: ok = %v", ok)
	}
	cmp := func(kind, engine, page string) *jobj {
		return getPath(t, a, "_html", "comparisons", kind, engine, page).(*jobj)
	}
	same := cmp("shop", "go-quickjs-interp", "one")
	if same.m["same_as_liustack"] != true {
		t.Errorf("a page with a different timestamp is not the same: %v", same.m)
	}
	if h := same.m["normalized_sha256"].(string); len(h) != 64 || h != same.m["reference_sha256"] {
		t.Errorf("digests: %v", same.m)
	}
	if v := cmp("shop", "hermes", "one").m["same_as_liustack"]; v != false {
		t.Errorf("hermes page differs, got %v", v)
	}
	if v := cmp("shop", "hermes", "missing").m["same_as_liustack"]; v != nil {
		t.Errorf("a page that did not render has no verdict, got %v", v)
	}
	if v := cmp("shop", "liustack-slowjs", "one").m["same_as_liustack"]; v != true {
		t.Errorf("liustack-slowjs against itself: %v", v)
	}
}

func TestDifferencesShowTheFirstPlace(t *testing.T) {
	dir := t.TempDir()
	result(t, dir, "liustack-slowjs-native", true, "a")
	result(t, dir, "go-quickjs-interp-native", true, "a")
	write(t, dir, "html/liustack-slowjs-native-a.html", "<p>8:00 AM u:1234567890123</p>")
	write(t, dir, "html/go-quickjs-interp-native-a.html", "<p>8:00\u202fAM u:1234567890124</p>")
	w := &report{}
	(&run{dir: dir}).differences(w)
	if got := w.String(); !strings.Contains(got, "go-quickjs-interp, shop a: at byte 7 ") {
		t.Errorf("differences:\n%s", got)
	}
}

// A row of a table: the pages that render, the pages that are the same as
// liustack-slowjs's, the geometric mean of the ratios with its range, and the notes.
func TestTableRows(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "liustack-slowjs-bench.json", `{"load_ms": 7, "pages": [{"input": "a.json", "ok": true, "cpu_ms_per_op": 10}, {"input": "b.json", "ok": true, "cpu_ms_per_op": 20}]}`)
	write(t, dir, "go-quickjs-interp-bench.json", `{"load_ms": 4.6, "prelude_ms": [1.2], "pages": [{"input": "a.json", "ok": true, "cpu_ms_per_op": 20}, {"input": "b.json", "ok": true, "cpu_ms_per_op": 20}]}`)
	write(t, dir, "go-quickjs-interp-bench.meta", `{"try":0,"disturbed":1}`+"\n")
	write(t, dir, "moejs-bench.json", `{"err": "load: SyntaxError: no\nsecond line"}`)
	write(t, dir, "hermes-bench.json", ``)
	write(t, dir, "node-jitless-bench.json", `{"pages": [{"input": "a.json", "ok": false, "err": "boom"}, {"input": "b.json", "ok": false, "err": "bang"}]}`)
	for _, e := range []string{"liustack-slowjs", "go-quickjs-interp"} {
		write(t, dir, "html/"+e+"-bench-a.html", "<p>u:1234567890123</p>")
		write(t, dir, "html/"+e+"-bench-b.html", "<p>b</p>")
	}
	w := &report{}
	(&run{dir: dir}).table(w, fixed("bench"), "Pages", []string{"liustack-slowjs", "go-quickjs-interp", "moejs", "hermes", "node-jitless", "quickjs-2026-06-04"}, "")
	want := `### Pages

| Engine | Load ms | Pages that render | Same page as liustack-slowjs | Time per render vs liustack-slowjs |
|---|---|---|---|---|
| liustack-slowjs | 7 | 2 of 2 | 2 of 2 | 1.00 (1.00–1.00) |
| go-quickjs-interp | 6 | 2 of 2 | 2 of 2 | 1.41 (1.00–2.00) (disturbed) |
| moejs | load: SyntaxError: no | 0 of 2 | | |
| Hermes V1 260318099.0.7, -O bytecode (reference) | not run | | | |
| Node v26.13.1 --jitless (reference) | a: boom; b: bang | 0 of 2 | | |
| C QuickJS 2026-06-04 (reference) | not run | | | |

`
	if got := w.String(); got != want {
		t.Errorf("table:\n%s\nwant:\n%s", got, want)
	}
}

// Pages that stop an engine come from the run of the engine without them.
func TestRestRunReplacesFailedPages(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "liustack-slowjs-pf.json", `{"pages": [{"input": "a.json", "ok": true, "cpu_ms_per_op": 1}, {"input": "b.json", "ok": true, "cpu_ms_per_op": 1}, {"input": "c.json", "ok": true, "cpu_ms_per_op": 1}]}`)
	write(t, dir, "go-quickjs-interp-pf.json", `{"pages": [{"input": "a.json", "ok": true, "cpu_ms_per_op": 1}, {"input": "b.json", "ok": false}, {"input": "c.json", "ok": false}]}`)
	write(t, dir, "go-quickjs-interp-pf-rest.json", `{"pages": [{"input": "b.json", "ok": true, "cpu_ms_per_op": 3}, {"input": "a.json", "ok": true, "cpu_ms_per_op": 9}]}`)
	r := &run{dir: dir}
	merged := r.loadMerged("go-quickjs-interp-pf")
	ps := pages(merged)
	if v, _ := perRender(ps.m["b"]); v != 3 {
		t.Errorf("page b = %v, want the rest run's 3", v)
	}
	if v, _ := perRender(ps.m["a"]); v != 1 {
		t.Errorf("page a = %v, want its own 1: a page that ran is not replaced", v)
	}
	if _, ok := perRender(ps.m["c"]); ok {
		t.Error("page c is not in the rest run and stays failed")
	}
	if f := get(merged, "failed").([]any); len(f) != 1 || f[0] != "c" {
		t.Errorf("failed = %v, want [c]", f)
	}
}

func TestPyFloat(t *testing.T) {
	for _, c := range []struct {
		x    float64
		want string
	}{
		{0, "0.0"}, {1, "1.0"}, {-2.5, "-2.5"}, {0.1, "0.1"}, {123.456, "123.456"},
		{100000, "100000.0"}, {1e15, "1000000000000000.0"}, {1e16, "1e+16"}, {1.5e16, "1.5e+16"},
		{123456789012345680, "1.2345678901234568e+17"}, {0.0001, "0.0001"}, {0.00001, "1e-05"},
		{1.5e-7, "1.5e-07"}, {1e22, "1e+22"}, {5e-324, "5e-324"}, {1.7976931348623157e308, "1.7976931348623157e+308"},
		{15.860535, "15.860535"}, {2.5e-5, "2.5e-05"}, {9999999999999998, "9999999999999998.0"},
	} {
		if got := pyFloat(c.x); got != c.want {
			t.Errorf("pyFloat(%v) = %q, want %q", c.x, got, c.want)
		}
	}
}

func TestPyG(t *testing.T) {
	for _, c := range []struct {
		x    float64
		want string
	}{{5, "5"}, {0.5, "0.5"}, {12.345, "12.3"}, {99.99, "100"}, {0.000123456, "0.000123"}, {0.00001234, "1.23e-05"}, {1.5, "1.5"}, {0.07, "0.07"}} {
		if got := pyG(c.x, 3); got != c.want {
			t.Errorf("pyG(%v, 3) = %q, want %q", c.x, got, c.want)
		}
	}
}

// The archive is written as Python's json.dump(indent=2, sort_keys=True)
// wrote it, so an archive of an earlier run reads back and writes out the
// same.
func TestJSONRoundTrip(t *testing.T) {
	in := `{
  "b": [
    1,
    1.0,
    -0,
    12345678901234567890,
    1e-05,
    100000.0,
    null,
    true
  ],
  "a": {},
  "c": [],
  "s": "caf@u00e9 @ud83d@ude00 @u007f \" \\ \n\t@u0001 <>&"
}
`
	// keys come out sorted: a, b, c, s
	wantSorted := `{
  "a": {},
  "b": [
    1,
    1.0,
    0,
    12345678901234567890,
    1e-05,
    100000.0,
    null,
    true
  ],
  "c": [],
  "s": "caf@u00e9 @ud83d@ude00 @u007f \" \\ \n\t@u0001 <>&"
}
`
	// The texts hold @ where the escape has its backslash.
	in = strings.ReplaceAll(in, "@", "\\")
	wantSorted = strings.ReplaceAll(wantSorted, "@", "\\")
	v, err := parseJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	writeJSON(&b, v, 0)
	b.WriteString("\n")
	if got := b.String(); got != wantSorted {
		t.Errorf("round trip:\n%s\nwant:\n%s", got, wantSorted)
	}
}

func TestParseJSONRefuses(t *testing.T) {
	for _, in := range []string{"", " ", "{", `{"a":1,}`, "[1,]", `"a` + "\n" + `"`, "{} x", "\xef\xbb\xbf{}", "01", "1.", "tru", `{"a" 1}`, "\"\x01\"", "\xff"} {
		if v, err := parseJSON(in); err == nil {
			t.Errorf("parseJSON(%q) = %v, want an error", in, v)
		}
	}
	for _, in := range []string{"NaN", "Infinity", "-Infinity", " [ ] ", `{"a":{"b":[1,2]}}`, "-1.5E+3"} {
		if _, err := parseJSON(in); err != nil {
			t.Errorf("parseJSON(%q): %v", in, err)
		}
	}
}

// liustack-slowjs-polyfill runs the pf kinds of the shop and MUI pages, as the
// engines without an Intl of their own do, and liustack-slowjs its own.
func TestBareVariantRunsThePolyfillKinds(t *testing.T) {
	shop := asDeployed("native", "pf")
	for e, want := range map[string]string{"liustack-slowjs": "native", "liustack-slowjs-polyfill": "pf", "hermes": "pf"} {
		if got := shop.of(e); got != want {
			t.Errorf("shop kind of %s = %q, want %q", e, got, want)
		}
	}
	if got := asDeployed("mui-native", "mui-pf").of("liustack-slowjs-polyfill"); got != "mui-pf" {
		t.Errorf("mui kind = %q", got)
	}
}
