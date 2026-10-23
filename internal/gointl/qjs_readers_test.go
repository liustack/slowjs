// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/liustack/slowjs/internal/gointl/internal/qjspack"
)

// qjsReaderHooks are the readers' packs and the function that puts each in
// Embedded.
var qjsReaderHooks = []struct{ pack, data, hook string }{
	{"dates", qjsDates, "qjsUseDates"},
	{"zonenames", qjsZoneNames, "qjsUseZoneNames"},
	{"hostzones", qjsHostZones, "qjsUseHostZones"},
}

// Every function of go-intl that opens a data set of one of the readers'
// packs calls the function that puts that pack in Embedded, so that it never
// finds the data missing. A new release of go-intl that reads those data sets
// from somewhere else fails here: add the call to qjs.patch.
func TestQJSReadersUsePacks(t *testing.T) {
	// The data sets of each pack: the first part of its files' names.
	hookOf := map[string]string{}
	for _, r := range qjsReaderHooks {
		p, err := qjspack.Open(r.data)
		if err != nil {
			t.Fatalf("%s.bin: %v", r.pack, err)
		}
		if len(p.Names()) == 0 {
			t.Fatalf("%s.bin is empty", r.pack)
		}
		for _, name := range p.Names() {
			set, _, _ := strings.Cut(name, "/")
			hookOf[strings.TrimSuffix(set, ".bin")] = r.hook
		}
	}
	// No other pack has those data sets.
	for _, b := range qjsPackData() {
		p, err := qjspack.Open(b.data)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range p.Names() {
			set, _, _ := strings.Cut(name, "/")
			if hook, ok := hookOf[strings.TrimSuffix(set, ".bin")]; ok {
				t.Errorf("%s.bin has %s, which %s's pack has", b.name, name, hook)
			}
		}
	}

	hooks := map[string]bool{}
	for _, r := range qjsReaderHooks {
		hooks[r.hook] = true
	}

	// The files of go-intl and of its packages that read data, and the
	// Marker constants of go-intl.
	fset := token.NewFileSet()
	var parsed []*ast.File
	markers := map[string]string{} // Marker constant name to its data set
	for _, dir := range []string{".", "date", "temporal"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("no Go files in %s", dir)
		}
		for _, f := range files {
			base := filepath.Base(f)
			if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "qjs") {
				continue
			}
			file, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			parsed = append(parsed, file)
			if dir != "." {
				continue
			}
			for _, d := range file.Decls {
				g, ok := d.(*ast.GenDecl)
				if !ok || g.Tok != token.CONST {
					continue
				}
				for _, s := range g.Specs {
					v := s.(*ast.ValueSpec)
					for i, n := range v.Names {
						if i >= len(v.Values) {
							continue
						}
						if lit, ok := v.Values[i].(*ast.BasicLit); ok && strings.HasPrefix(n.Name, "Marker") {
							markers[n.Name], _ = strconv.Unquote(lit.Value)
						}
					}
				}
			}
		}
	}
	if len(markers) == 0 {
		t.Fatal("no Marker constants")
	}

	// setOf is the data set a string literal names, as in Marker("dates/"+x),
	// or "" for an expression that starts with none.
	setOf := func(e ast.Expr) string {
		for {
			b, ok := e.(*ast.BinaryExpr)
			if !ok {
				break
			}
			e = b.X
		}
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return ""
		}
		v, _ := strconv.Unquote(lit.Value)
		set, _, _ := strings.Cut(v, "/")
		return set
	}

	// Every reference to a data set of a reader's pack, in a function or a
	// function literal, comes after a call of its hook in that function or
	// one around it.
	type call struct {
		hook string
		pos  token.Pos
		in   *ast.BlockStmt // the innermost function body around it
	}
	type ref struct {
		hook, what string
		pos        token.Pos
		in         *ast.BlockStmt
	}
	checked := 0
	for _, file := range parsed {
		var calls []call
		var refs []ref
		names := map[*ast.BlockStmt]string{}
		var stack []*ast.BlockStmt
		var visit func(n ast.Node) bool
		visit = func(n ast.Node) bool {
			var body *ast.BlockStmt
			switch n := n.(type) {
			case *ast.GenDecl:
				// A constant reads nothing.
				return n.Tok != token.CONST
			case *ast.FuncDecl:
				body = n.Body
				names[body] = n.Name.Name
			case *ast.FuncLit:
				body = n.Body
				names[body] = "the function literal at " + fset.Position(n.Pos()).String()
			}
			if body != nil {
				stack = append(stack, body)
				ast.Inspect(body, func(m ast.Node) bool {
					if m == body {
						return true
					}
					return visit(m)
				})
				stack = stack[:len(stack)-1]
				return false
			}
			var in *ast.BlockStmt
			if len(stack) > 0 {
				in = stack[len(stack)-1]
			}
			add := func(set, what string, pos token.Pos) {
				if hook, ok := hookOf[set]; ok {
					refs = append(refs, ref{hook, what, pos, in})
				}
			}
			switch n := n.(type) {
			case *ast.Ident:
				if set, ok := markers[n.Name]; ok {
					add(set, n.Name, n.Pos())
				}
			case *ast.CallExpr:
				var name string
				switch f := n.Fun.(type) {
				case *ast.Ident:
					name = f.Name
				case *ast.SelectorExpr:
					name = f.Sel.Name
				}
				switch {
				case hooks[name]:
					calls = append(calls, call{name, n.Pos(), in})
				case (name == "Marker" || name == "Open") && len(n.Args) > 0:
					if set := setOf(n.Args[0]); set != "" {
						add(set, fmt.Sprintf("%s(%q)", name, set), n.Pos())
					}
				}
			}
			return true
		}
		ast.Inspect(file, visit)

		seen := map[*ast.BlockStmt]map[string]bool{}
		for _, r := range refs {
			if r.in == nil {
				t.Errorf("%s: %s is read outside a function", fset.Position(r.pos), r.what)
				continue
			}
			if seen[r.in] == nil {
				seen[r.in] = map[string]bool{}
			}
			if !seen[r.in][r.hook] {
				seen[r.in][r.hook] = true
				checked++
			}
			ok := false
			for _, c := range calls {
				if c.hook == r.hook && c.pos < r.pos && c.in.Lbrace <= r.pos && r.pos < c.in.Rbrace {
					ok = true
					break
				}
			}
			if !ok {
				t.Errorf("%s: %s reads %s without calling %s before", fset.Position(r.pos), names[r.in], r.what, r.hook)
			}
		}
	}
	if checked < 8 {
		t.Errorf("found %d readers of the readers' packs", checked)
	}
}

// A reader's pack serves the same bytes through Embedded once its function
// has run.
func TestQJSReaderPackServed(t *testing.T) {
	qjsUseDates()
	qjsUseZoneNames()
	qjsUseHostZones()
	for _, m := range []Marker{MarkerDatesShared, MarkerZoneNamesShared, MarkerMetazones, MarkerWindowsZones} {
		if b, err := Embedded.Open(m, DataLocale{}); err != nil || len(b) == 0 {
			t.Errorf("%s: %d bytes, %v", m, len(b), err)
		}
	}
	if _, err := Embedded.Open(MarkerDates, qjsLocale("de").Data()); err != nil {
		t.Errorf("the dates of de: %v", err)
	}
}

// A panic opening a reader's pack is kept: every later read panics with it
// too, where it would find no pack and no error.
func TestQJSReaderKeepsPanic(t *testing.T) {
	old := qjsReaders.list.Load()
	defer qjsReaders.list.Store(old)
	qjsReaders.list.Store(nil)
	var r qjsReader
	calls := 0
	r.use(func() (*qjspack.Pack, error) {
		calls++
		panic("broken pack")
	})
	for i := range 2 {
		func() {
			defer func() {
				if p := recover(); p != "broken pack" {
					t.Errorf("read %d: recovered %v", i, p)
				}
			}()
			b, err := qjsReadersFile("datesshared.bin")
			t.Errorf("read %d: no panic: %d bytes, %v", i, len(b), err)
		}()
	}
	if calls != 1 {
		t.Errorf("open ran %d times", calls)
	}
}

// qjsReaderLoaders are go-intl's loaders that read the readers' packs, each
// with what it finds in the embedded data.
var qjsReaderLoaders = []struct {
	name string
	load func() error
}{
	{"loadDates", func() error { _, err := loadDates(Embedded, qjsLocale("de")); return err }},
	{"patternCalendarOf", func() error {
		if c := patternCalendarOf(Embedded, qjsLocale("uz-AF")); c != "gregory" {
			return fmt.Errorf("uz-AF: %q", c)
		}
		return nil
	}},
	{"loadUmmAlQura", func() error { _, err := loadUmmAlQura(Embedded); return err }},
	{"loadJapaneseEras", func() error { _, err := loadJapaneseEras(Embedded); return err }},
	{"loadRBNF", func() error { _, _, err := loadRBNF(Embedded, "hans"); return err }},
	{"loadZoneNames", func() error { _, err := loadZoneNames(Embedded, qjsLocale("de")); return err }},
	{"windowsZone", func() error {
		z, err := windowsZone(Embedded, "W. Europe Standard Time", "DE")
		if err == nil && z != "Europe/Berlin" {
			err = fmt.Errorf("W. Europe Standard Time in DE: %q", z)
		}
		return err
	}},
	{"guessHostZone", func() error {
		if z := guessHostZone(Embedded, hostAbbreviation{std: "CHAST", dst: "CHADT", daylight: 2}, 45900); z != "Pacific/Chatham" {
			return fmt.Errorf("CHAST: %q", z)
		}
		return nil
	}},
}

// Each loader finds its data in a process where nothing has called a hook
// before it: the hook it calls itself is enough.
func TestQJSReaderLoadersAlone(t *testing.T) {
	if name := os.Getenv("QJS_READER_LOADER"); name != "" {
		if qjsReaders.list.Load() != nil {
			t.Fatal("a reader's pack is in use before the loader runs")
		}
		for _, l := range qjsReaderLoaders {
			if l.name == name {
				if err := l.load(); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		t.Fatalf("no loader %s", name)
	}
	for _, l := range qjsReaderLoaders {
		cmd := exec.Command(os.Args[0], "-test.run=^TestQJSReaderLoadersAlone$", "-test.count=1")
		cmd.Env = append(os.Environ(), "QJS_READER_LOADER="+l.name)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: %v\n%s", l.name, err, out)
		}
	}
}
