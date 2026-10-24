// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
)

type pkg struct {
	Dir        string
	GoFiles    []string
	ImportPath string
}
type signature struct {
	Package string
	Params  []string
	Results []string
}

func main() {
	d := json.NewDecoder(os.Stdin)
	all := map[string]signature{}
	fs := token.NewFileSet()
	for {
		var p pkg
		if err := d.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
		for _, name := range p.GoFiles {
			f, err := parser.ParseFile(fs, filepath.Join(p.Dir, name), nil, 0)
			if err != nil {
				panic(err)
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil {
					continue
				}
				fields := func(fl *ast.FieldList) []string {
					var out []string
					if fl == nil {
						return out
					}
					for _, v := range fl.List {
						var b bytes.Buffer
						format.Node(&b, fs, v.Type)
						n := len(v.Names)
						if n == 0 {
							n = 1
						}
						for i := 0; i < n; i++ {
							out = append(out, b.String())
						}
					}
					return out
				}
				all[p.ImportPath+":"+fn.Name.Name] = signature{p.ImportPath, fields(fn.Type.Params), fields(fn.Type.Results)}
			}
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(all); err != nil {
		panic(err)
	}
}
