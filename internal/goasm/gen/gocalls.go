// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// GoCallsFile is the list of the engine functions Go code names, in the
// package of the engine: GoCalls makes it from the Go sources
// (cmd/gocalls writes it, and TestGoCallsList fails when it is not what
// GoCalls finds), and the generator reads it (-go-calls).
const GoCallsFile = "gocalls.txt"

// GoCallsHeader starts GoCallsFile. Every other line is a Go name.
const GoCallsHeader = "# The engine functions Go code names (go run ./internal/goasm/gen/cmd/gocalls). Do not edit.\n"

var reAsmRef = regexp.MustCompile(`·([A-Za-z_][A-Za-z_0-9]*)(?:<[^>]*>)?(?:[+-]\d+)?\(SB\)`)

// GoCalls lists, sorted, the engine functions the Go code of the module at
// root names: each function declared in the engine's generated declarations
// (engine_decl_<arch>.go of pkgDir, for every architecture, the Go wrappers
// of packed functions too) that a Go file names, or a hand-written assembly
// file of pkgDir. A name counts wherever it is used, as a call or as a
// value: only Go code can read a result from the Go stack, and a function
// Go never names never returns to Go. A file of the package names a
// function by its bare name (unless the file imports a package by that
// name), any other by a selector of an import of the package. Generated
// files, testdata, vendor and node_modules folders and folders that start
// with a dot or an underscore are left out. The modules nested in root are
// read too: Go lets them import the package.
func GoCalls(root, pkgDir string) ([]string, error) {
	pkgPath, err := importPath(root, pkgDir)
	if err != nil {
		return nil, err
	}
	engine, err := engineFuncs(pkgDir)
	if err != nil {
		return nil, err
	}
	pkgAbs, err := filepath.Abs(pkgDir)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		dir, err := filepath.Abs(filepath.Dir(path))
		if err != nil {
			return err
		}
		inPkg := dir == pkgAbs
		switch {
		case strings.HasSuffix(name, ".go"):
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if generated(src) {
				return nil
			}
			f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			goNames(f, inPkg && !strings.HasSuffix(f.Name.Name, "_test"), pkgPath, engine, used)
		case strings.HasSuffix(name, ".s") && inPkg:
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if generated(src) {
				return nil
			}
			for _, m := range reAsmRef.FindAllSubmatch(src, -1) {
				if engine[string(m[1])] {
					used[string(m[1])] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return SortedKeys(used), nil
}

// ReadGoCalls reads a list GoCalls made (GoCallsFile).
func ReadGoCalls(b []byte) (map[string]bool, error) {
	out := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for n := 1; sc.Scan(); n++ {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if !token.IsIdentifier(l) {
			return nil, fmt.Errorf("line %d of the list of Go calls: %q is no Go name", n, l)
		}
		out[l] = true
	}
	return out, sc.Err()
}

// generated says a file is generated: it has the line Go's rule for
// generated files names (https://go.dev/s/generatedcode) before its package
// clause or first instruction.
func generated(src []byte) bool {
	for _, l := range strings.Split(string(src), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "// Code generated ") && strings.HasSuffix(l, " DO NOT EDIT.") {
			return true
		}
		if strings.HasPrefix(l, "package ") || strings.HasPrefix(l, "TEXT ") {
			return false
		}
	}
	return false
}

// engineFuncs are the functions the generated declarations of pkgDir
// declare, for every architecture.
func engineFuncs(pkgDir string) (map[string]bool, error) {
	files, err := filepath.Glob(filepath.Join(pkgDir, "engine_decl_*.go"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no engine_decl_<arch>.go in %s", pkgDir)
	}
	out := map[string]bool{}
	fset := token.NewFileSet()
	for _, p := range files {
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				out[fd.Name.Name] = true
			}
		}
	}
	return out, nil
}

// importPath is the import path of dir in the module at root.
func importPath(root, dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	mod := ""
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "module" {
			mod = f[1]
		}
	}
	if mod == "" {
		return "", fmt.Errorf("no module line in %s/go.mod", root)
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", err
	}
	return mod + "/" + filepath.ToSlash(rel), nil
}

// goNames adds to used the engine functions f names: in the engine's own
// package (inPkg) by an identifier the file does not take for something of
// its own, elsewhere by a selector of an import of pkgPath. It resolves the
// scopes of the file itself, so a local variable, parameter, field, label
// or import of the same name is not the function.
func goNames(f *ast.File, inPkg bool, pkgPath string, engine, used map[string]bool) {
	imports := map[string]bool{}
	var pkgNames []string
	for _, is := range f.Imports {
		p, _ := strconv.Unquote(is.Path.Value)
		name := p[strings.LastIndex(p, "/")+1:]
		if is.Name != nil {
			name = is.Name.Name
		}
		imports[name] = true
		if p == pkgPath && name != "_" && name != "." {
			pkgNames = append(pkgNames, name)
		}
	}
	isPkg := map[string]bool{}
	for _, n := range pkgNames {
		isPkg[n] = true
	}
	if inPkg {
		w := &scopeWalker{engine: engine, used: used, imports: imports}
		w.file(f)
	}
	if len(pkgNames) > 0 {
		ast.Inspect(f, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok {
				if x, ok := s.X.(*ast.Ident); ok && isPkg[x.Name] && engine[s.Sel.Name] {
					used[s.Sel.Name] = true
				}
			}
			return true
		})
	}
}

// scopeWalker finds the identifiers of a file of the engine's package that
// name a package-level function of the engine: an identifier in an
// expression that no scope of the file declares, which is not a selector's
// field, a field or key of a composite literal, or an import.
type scopeWalker struct {
	engine, used, imports map[string]bool
	scopes                []map[string]bool
}

func (w *scopeWalker) declared(name string) bool {
	for i := len(w.scopes) - 1; i >= 0; i-- {
		if w.scopes[i][name] {
			return true
		}
	}
	return false
}

func (w *scopeWalker) push() { w.scopes = append(w.scopes, map[string]bool{}) }
func (w *scopeWalker) pop()  { w.scopes = w.scopes[:len(w.scopes)-1] }
func (w *scopeWalker) declare(ids ...*ast.Ident) {
	for _, id := range ids {
		if id != nil && id.Name != "_" {
			w.scopes[len(w.scopes)-1][id.Name] = true
		}
	}
}

func (w *scopeWalker) ident(id *ast.Ident) {
	if w.engine[id.Name] && !w.imports[id.Name] && !w.declared(id.Name) {
		w.used[id.Name] = true
	}
}

func (w *scopeWalker) fields(fl *ast.FieldList, declare bool) {
	if fl == nil {
		return
	}
	for _, f := range fl.List {
		w.expr(f.Type)
		if declare {
			w.declare(f.Names...)
		}
	}
}

func (w *scopeWalker) file(f *ast.File) {
	w.push()
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			w.push()
			w.fields(d.Recv, true)
			w.fields(d.Type.TypeParams, true)
			w.fields(d.Type.Params, true)
			w.fields(d.Type.Results, true)
			if d.Body != nil {
				w.stmts(d.Body.List)
			}
			w.pop()
		case *ast.GenDecl:
			w.genDecl(d)
		}
	}
	w.pop()
}

func (w *scopeWalker) genDecl(d *ast.GenDecl) {
	for _, s := range d.Specs {
		switch s := s.(type) {
		case *ast.ValueSpec:
			w.expr(s.Type)
			for _, v := range s.Values {
				w.expr(v)
			}
			if len(w.scopes) > 1 {
				w.declare(s.Names...)
			}
		case *ast.TypeSpec:
			if len(w.scopes) > 1 {
				w.declare(s.Name)
			}
			w.push()
			w.fields(s.TypeParams, true)
			w.expr(s.Type)
			w.pop()
		}
	}
}

func (w *scopeWalker) stmts(list []ast.Stmt) {
	for _, s := range list {
		w.stmt(s)
	}
}

func (w *scopeWalker) stmt(s ast.Stmt) {
	switch s := s.(type) {
	case nil:
	case *ast.DeclStmt:
		w.genDecl(s.Decl.(*ast.GenDecl))
	case *ast.AssignStmt:
		for _, r := range s.Rhs {
			w.expr(r)
		}
		if s.Tok == token.DEFINE {
			for _, l := range s.Lhs {
				if id, ok := l.(*ast.Ident); ok {
					w.declare(id)
				}
			}
		} else {
			for _, l := range s.Lhs {
				w.expr(l)
			}
		}
	case *ast.BlockStmt:
		w.push()
		w.stmts(s.List)
		w.pop()
	case *ast.ExprStmt:
		w.expr(s.X)
	case *ast.GoStmt:
		w.expr(s.Call)
	case *ast.DeferStmt:
		w.expr(s.Call)
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			w.expr(r)
		}
	case *ast.IfStmt:
		w.push()
		w.stmt(s.Init)
		w.expr(s.Cond)
		w.stmt(s.Body)
		w.stmt(s.Else)
		w.pop()
	case *ast.ForStmt:
		w.push()
		w.stmt(s.Init)
		w.expr(s.Cond)
		w.stmt(s.Post)
		w.stmt(s.Body)
		w.pop()
	case *ast.RangeStmt:
		w.push()
		w.expr(s.X)
		if s.Tok == token.DEFINE {
			for _, e := range []ast.Expr{s.Key, s.Value} {
				if id, ok := e.(*ast.Ident); ok {
					w.declare(id)
				}
			}
		} else {
			w.expr(s.Key)
			w.expr(s.Value)
		}
		w.stmt(s.Body)
		w.pop()
	case *ast.SwitchStmt:
		w.push()
		w.stmt(s.Init)
		w.expr(s.Tag)
		w.clauses(s.Body, nil)
		w.pop()
	case *ast.TypeSwitchStmt:
		w.push()
		w.stmt(s.Init)
		var bound *ast.Ident
		if a, ok := s.Assign.(*ast.AssignStmt); ok {
			if id, ok := a.Lhs[0].(*ast.Ident); ok {
				bound = id
			}
			for _, r := range a.Rhs {
				w.expr(r)
			}
		} else {
			w.stmt(s.Assign)
		}
		w.clauses(s.Body, bound)
		w.pop()
	case *ast.SelectStmt:
		for _, c := range s.Body.List {
			cc := c.(*ast.CommClause)
			w.push()
			w.stmt(cc.Comm)
			w.stmts(cc.Body)
			w.pop()
		}
	case *ast.LabeledStmt:
		w.stmt(s.Stmt)
	case *ast.IncDecStmt:
		w.expr(s.X)
	case *ast.SendStmt:
		w.expr(s.Chan)
		w.expr(s.Value)
	case *ast.BranchStmt, *ast.EmptyStmt:
	default:
		ast.Inspect(s, func(n ast.Node) bool {
			if e, ok := n.(ast.Expr); ok {
				w.expr(e)
				return false
			}
			return true
		})
	}
}

func (w *scopeWalker) clauses(body *ast.BlockStmt, bound *ast.Ident) {
	for _, c := range body.List {
		cc := c.(*ast.CaseClause)
		for _, e := range cc.List {
			w.expr(e)
		}
		w.push()
		w.declare(bound)
		w.stmts(cc.Body)
		w.pop()
	}
}

func (w *scopeWalker) expr(e ast.Expr) {
	switch e := e.(type) {
	case nil:
	case *ast.Ident:
		w.ident(e)
	case *ast.SelectorExpr:
		w.expr(e.X) // e.Sel is a field, a method or a name of another package
	case *ast.FuncLit:
		w.push()
		w.fields(e.Type.Params, true)
		w.fields(e.Type.Results, true)
		w.stmts(e.Body.List)
		w.pop()
	case *ast.CompositeLit:
		w.expr(e.Type)
		for _, el := range e.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if _, isID := kv.Key.(*ast.Ident); !isID {
					w.expr(kv.Key)
				}
				w.expr(kv.Value)
			} else {
				w.expr(el)
			}
		}
	case *ast.StructType:
		w.fields(e.Fields, false)
	case *ast.InterfaceType:
		w.fields(e.Methods, false)
	case *ast.FuncType:
		w.push()
		w.fields(e.TypeParams, true)
		w.fields(e.Params, true)
		w.fields(e.Results, true)
		w.pop()
	default:
		ast.Inspect(e, func(n ast.Node) bool {
			if n == e {
				return true
			}
			if x, ok := n.(ast.Expr); ok {
				w.expr(x)
				return false
			}
			return true
		})
	}
}

// SortGoCalls writes the list GoCallsFile holds.
func SortGoCalls(names []string) []byte {
	s := append([]string(nil), names...)
	sort.Strings(s)
	return []byte(GoCallsHeader + strings.Join(s, "\n") + "\n")
}
