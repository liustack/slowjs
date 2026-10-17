// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// reMacro matches the name ccgo gives a C macro constant.
var reMacro = regexp.MustCompile(`^M[A-Z][A-Z0-9_]*$`)

// reMacroDecl matches the declaration of a macro constant, alone or in a
// const block.
var reMacroDecl = regexp.MustCompile(`(?m)^(?:const |\t)(M[A-Z][A-Z0-9_]*) = `)

// macrosUsed returns the macro constants src refers to by their bare name.
func macrosUsed(t *testing.T, fn string, src []byte) []string {
	var s scanner.Scanner
	fset := token.NewFileSet()
	s.Init(fset.AddFile(fn, -1, len(src)), src, func(pos token.Position, msg string) { t.Fatalf("%s: %s", pos, msg) }, 0)
	seen := map[string]bool{}
	prev := token.ILLEGAL
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.IDENT && prev != token.PERIOD && reMacro.MatchString(lit) {
			seen[lit] = true
		}
		prev = tok
	}
	var names []string
	for name := range seen {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// ccgo writes into each generated file only the macros of that platform's C
// headers, so a macro constant one file declares can be missing from
// another: MTRUE is in the darwin and windows files and not in the linux
// ones. Every macro constant the generated code or the Go files next to it
// use is declared in each generated file of the package.
func TestMacrosAreDeclaredOnEveryPlatform(t *testing.T) {
	for _, dir := range []string{filepath.Join("..", "..", "lib"), filepath.Join("..", "..", "lib", "internal", "run-test262")} {
		generated, err := filepath.Glob(filepath.Join(dir, "ccgo_*.go"))
		if err != nil || len(generated) == 0 {
			t.Fatal(dir, generated, err)
		}
		all, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		var shared []string
		for _, fn := range all {
			if !slices.Contains(generated, fn) && !strings.HasSuffix(fn, "_test.go") {
				shared = append(shared, fn)
			}
		}
		used := map[string][]string{}
		for _, fn := range shared {
			b, err := os.ReadFile(fn)
			if err != nil {
				t.Fatal(err)
			}
			used[fn] = macrosUsed(t, fn, b)
		}
		for _, fn := range generated {
			b, err := os.ReadFile(fn)
			if err != nil {
				t.Fatal(err)
			}
			declared := map[string]bool{}
			for _, m := range reMacroDecl.FindAllSubmatch(b, -1) {
				declared[string(m[1])] = true
			}
			check := func(from string, names []string) {
				for _, name := range names {
					if !declared[name] {
						t.Errorf("%s uses %s, which %s does not declare", from, name, fn)
					}
				}
			}
			check(fn, macrosUsed(t, fn, b))
			for _, from := range shared {
				check(from, used[from])
			}
		}
	}
}
