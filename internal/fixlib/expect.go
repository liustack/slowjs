// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
)

// ccgo writes the likely and unlikely of QuickJS, __builtin_expect(!!(x),
// c), which returns a long, as libc.BoolInt64(!!(x)) != 0, or as
// libc.BoolInt32(!!(x)) != 0 where a long has 32 bits. Go then makes the
// int of the condition and tests it, when the condition has && or ||. This
// change writes such a test as expect64(!!(x)) or expect32(!!(x)), of
// lib/expect.go, which return their argument and which Go inlines, so that
// Go branches on the condition itself. The value of the test is the same
// for every x: BoolInt64(b) != 0 is b.
//
// Other changes match the text of the generated files, these tests in it,
// so fixExpect is a final change (see finalChanges). It checks that
// restoreExpect, its exact inverse, gives its input back, and the number of
// tests it changes in each file against expectSites.

var expectF = flag.Bool("expect", false, "print the number of tests of the form of __builtin_expect in each generated file, in the form of expectSites, to update it after a review")

// expectForms are the calls of libc that fixExpect changes, and the names of
// the functions of lib/expect.go it changes them to.
var expectForms = map[string]string{
	"BoolInt64": "expect64",
	"BoolInt32": "expect32",
}

// outerEdits returns the edits of edits that no other edit holds.
func outerEdits(edits []edit) []edit {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out []edit
	end := 0
	for _, e := range edits {
		if e.start < end {
			continue
		}
		out = append(out, e)
		end = e.end
	}
	return out
}

// expectEdits returns the edits that change each test of b of the forms of
// expectForms, outermost first when one holds another.
func expectEdits(b []byte) ([]edit, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", b, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	var edits []edit
	ast.Inspect(f, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok || be.Op != token.NEQ {
			return true
		}
		if lit, ok := be.Y.(*ast.BasicLit); !ok || lit.Kind != token.INT || lit.Value != "0" {
			return true
		}
		call, ok := be.X.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "libc" {
			return true
		}
		fn, ok := expectForms[sel.Sel.Name]
		if !ok {
			return true
		}
		// !!(x): the form of __builtin_expect.
		u, ok := call.Args[0].(*ast.UnaryExpr)
		if !ok || u.Op != token.NOT {
			return true
		}
		if u2, ok := u.X.(*ast.UnaryExpr); !ok || u2.Op != token.NOT {
			return true
		}
		arg := b[off(call.Args[0].Pos()):off(call.Args[0].End())]
		edits = append(edits, edit{off(be.Pos()), off(be.End()), fn + "(" + string(arg) + ")"})
		return true
	})
	return edits, nil
}

// expectRewrite changes every test of b of the forms of expectForms, and
// returns how many it changed.
func expectRewrite(b []byte) ([]byte, int, error) {
	out := b
	for {
		edits, err := expectEdits(out)
		if err != nil {
			return nil, 0, err
		}
		if len(edits) == 0 {
			break
		}
		// The edits another holds are made in the next round, on the text
		// the outer one leaves.
		out = applyEdits(out, outerEdits(edits))
	}
	n := 0
	for _, fn := range expectForms {
		n += bytes.Count(out, []byte(fn+"("))
	}
	return out, n, nil
}

// fixExpect makes the change in b, the generated file name.
func fixExpect(b []byte, name string) ([]byte, error) {
	for _, fn := range expectForms {
		if bytes.Contains(b, []byte(fn+"(")) {
			return nil, fmt.Errorf("fixExpect: %s has a call of %s already, restoreExpect goes first", name, fn)
		}
	}
	out, n, err := expectRewrite(b)
	if err != nil {
		return nil, fmt.Errorf("fixExpect: %s: %v", name, err)
	}
	if want, ok := expectSites[name]; (!ok || n != want) && !*expectF {
		return nil, fmt.Errorf("fixExpect: %s has %d tests of the form of __builtin_expect, expectSites has %d: read the new ones, each must be libc.BoolInt64(!!(x)) != 0 or libc.BoolInt32(!!(x)) != 0, then update expectSites (go run ./internal/fixlib -expect prints it)", name, n, want)
	}
	back, err := restoreExpect(out, name)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixExpect: restoreExpect does not give %s back", name)
	}
	return out, nil
}

// expectReport prints expectSites as the generated files give it.
func expectReport(read func(fn string) ([]byte, error)) error {
	files, err := filepath.Glob(filepath.Join("lib", "ccgo_*.go"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	fmt.Println("var expectSites = map[string]int{")
	for _, fn := range files {
		b, err := read(fn)
		if err != nil {
			return err
		}
		name := filepath.Base(fn)
		_, n, err := expectRewrite(b)
		if err != nil {
			return fmt.Errorf("%s: %v", name, err)
		}
		fmt.Printf("\t%q: %d,\n", name, n)
	}
	fmt.Println("}")
	return nil
}

// restoreExpect undoes fixExpect in b, the generated file name.
func restoreExpect(b []byte, name string) ([]byte, error) {
	back := map[string]string{}
	for form, fn := range expectForms {
		back[fn] = form
	}
	out := b
	for {
		has := false
		for fn := range back {
			if bytes.Contains(out, []byte(fn+"(")) {
				has = true
			}
		}
		if !has {
			return out, nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "", out, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("restoreExpect: %s: %v", name, err)
		}
		off := func(p token.Pos) int { return fset.Position(p).Offset }
		var edits []edit
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			form, ok := back[id.Name]
			if !ok {
				return true
			}
			arg := out[off(call.Lparen)+1 : off(call.Rparen)]
			edits = append(edits, edit{off(call.Pos()), off(call.End()), "libc." + form + "(" + string(arg) + ") != 0"})
			return true
		})
		if len(edits) == 0 {
			return nil, fmt.Errorf("restoreExpect: %s names a function of lib/expect.go other than in a call", name)
		}
		out = applyEdits(out, outerEdits(edits))
	}
}
