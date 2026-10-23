// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

// quickjs-libc.c is the C file of the qjs shell: the std, os and Worker
// modules, the event loop and a few helpers. The Go API cannot register the
// modules, which only js_init_module_std and js_init_module_os do, and it
// never calls them. Their functions stay in a program all the same, as the
// tables of the modules are package variables that hold their function
// pointers, and the linker keeps what a package variable refers to. With them
// stay the parts of libc they call: fork, exec, setenv, popen, files and the
// terminal.
//
// fixQuickJSLibc drops the functions and variables of quickjs-libc.c that
// the functions of quickjsLibcRoots do not reach, with the init functions
// that fill the dropped variables. ccgo writes the functions of each C file
// in the order of the file, and the files one after the other, quickjs-libc.c
// last: its part of a generated file starts after Xrqsort, the last function
// of cutils.c, and ends at __ccgo_fp, which ccgo writes after the last file.
// The constants and types of that part stay: they cost nothing in a program.
//
// The names of the functions and variables of that part as upstream writes
// them, and the text of what stays, are checked against quickjsLibcSites, so
// that a function upstream adds to quickjs-libc.c, or a change to one that
// stays, stops fixlib until someone has read it. A dropped name that the rest
// of the file uses stops fixlib too.

// quickjsLibcRoots are the functions of quickjs-libc.c that stay, with what
// they reach.
var quickjsLibcRoots = []string{
	"Xjs_std_add_helpers",        // VM.StdAddHelpers
	"Xjs_module_set_import_meta", // VM.EvalBytecode of a module
	"Xjs_module_loader",          // VM.SetDefaultModuleLoader
	"Xjs_load_file",              // the test262 runner
	"Xjs_module_test_json",       // the test262 runner
	"Xjs_std_dump_error",         // the test262 runner
}

const (
	// quickjsLibcAfter is the last function of cutils.c.
	quickjsLibcAfter = "Xrqsort"
	// quickjsLibcEnd starts what ccgo writes after the last C file.
	quickjsLibcEnd = "\nfunc __ccgo_fp("
	// quickjsLibcMark starts the part of quickjs-libc.c, before its empty
	// line, once fixQuickJSLibc has dropped what the Go API does not reach.
	quickjsLibcMark = "\n// Changed by internal/fixlib: the functions and tables of quickjs-libc.c that only its std, os and Worker modules reach are dropped (see fixQuickJSLibc).\n"
)

// libcSite is what quickjsLibcSites holds for a generated file: the
// fingerprint of the names of the functions and variables of quickjs-libc.c,
// as upstream writes them, and that of the text of those that stay.
type libcSite struct {
	names, kept string
}

// libcDecl is a function or a variable of quickjs-libc.c in a generated
// file. An init function is named init(v), after the variable v it fills.
type libcDecl struct {
	name       string
	start, end int // its text, with its doc comment
	next       int // where what follows it starts
	refs       []string
}

// fixQuickJSLibc drops what the Go API does not reach of quickjs-libc.c from
// the generated file name.
func fixQuickJSLibc(b []byte, name string) ([]byte, error) {
	want, ok := quickjsLibcSites[name]
	if !ok {
		return nil, fmt.Errorf("no quickjsLibcSites for %s", name)
	}
	out, got, _, _, err := trimQuickJSLibc(b)
	if err != nil {
		return nil, err
	}
	if got.names != "" && got.names != want.names {
		return nil, fmt.Errorf("the functions and variables of quickjs-libc.c changed (fingerprint %s, want %s): read the new ones, add those the Go API or the test262 runner calls to quickjsLibcRoots, then update quickjsLibcSites (go run ./internal/fixlib -qjslibc, on the files as taken from upstream), see docs/fork-changes/trimmed.md", got.names, want.names)
	}
	if got.kept != want.kept {
		return nil, fmt.Errorf("what stays of quickjs-libc.c changed (fingerprint %s, want %s): read it against quickjsLibcRoots, then update quickjsLibcSites (go run ./internal/fixlib -qjslibc, on the files as taken from upstream), see docs/fork-changes/trimmed.md", got.kept, want.kept)
	}
	return out, nil
}

// trimQuickJSLibc drops from b the functions and variables of quickjs-libc.c
// that quickjsLibcRoots do not reach, and returns the fingerprints of what it
// found, with the names it keeps and drops. A file it trimmed before has
// nothing more to drop, and the fingerprint of its names is "".
func trimQuickJSLibc(b []byte) (out []byte, got libcSite, kept, dropped []string, err error) {
	start, end, err := quickjsLibcPart(b)
	if err != nil {
		return nil, got, nil, nil, err
	}
	part := b[start:end]
	trimmed := bytes.HasPrefix(part, []byte(quickjsLibcMark))
	if trimmed {
		part = part[len(quickjsLibcMark):]
	}
	if bytes.Contains(part, []byte(strings.TrimPrefix(quickjsLibcMark, "\n"))) {
		return nil, got, nil, nil, fmt.Errorf("quickjs-libc.c: the mark of fixQuickJSLibc is not at its start")
	}
	decls, err := quickjsLibcDecls(part)
	if err != nil {
		return nil, got, nil, nil, err
	}

	byName := map[string]*libcDecl{}
	for i := range decls {
		byName[decls[i].name] = &decls[i]
	}
	keep := map[string]bool{}
	var walk func(string)
	walk = func(n string) {
		if keep[n] {
			return
		}
		keep[n] = true
		for _, r := range byName[n].refs {
			walk(r)
		}
		if _, ok := byName["init("+n+")"]; ok {
			walk("init(" + n + ")")
		}
	}
	for _, root := range quickjsLibcRoots {
		if byName[root] == nil {
			return nil, got, nil, nil, fmt.Errorf("quickjs-libc.c has no function %s of quickjsLibcRoots: see docs/fork-changes/trimmed.md", root)
		}
		walk(root)
	}

	table, err := cStrings(b)
	if err != nil {
		return nil, got, nil, nil, err
	}
	names, keptText := sha256.New(), sha256.New()
	var text bytes.Buffer
	text.WriteString(quickjsLibcMark)
	last := 0
	for _, d := range decls {
		names.Write([]byte(d.name + "\n"))
		if keep[d.name] {
			kept = append(kept, d.name)
			keptText.Write(withStrings(part[d.start:d.end], table))
			continue
		}
		dropped = append(dropped, d.name)
		text.Write(part[last:d.start])
		last = d.next
	}
	text.Write(part[last:])
	if trimmed != (len(dropped) == 0) {
		if trimmed {
			return nil, got, nil, nil, fmt.Errorf("quickjs-libc.c has the mark of fixQuickJSLibc and functions or variables to drop: %s", strings.Join(dropped, ", "))
		}
		return nil, got, nil, nil, fmt.Errorf("quickjs-libc.c has nothing to drop but no mark of fixQuickJSLibc: upstream may have dropped it itself, see docs/fork-changes/trimmed.md")
	}
	if !trimmed {
		got.names = fmt.Sprintf("%x", names.Sum(nil))[:16]
	}
	got.kept = fmt.Sprintf("%x", keptText.Sum(nil))[:16]

	out = make([]byte, 0, start+text.Len()+len(b)-end)
	out = append(append(append(out, b[:start]...), text.Bytes()...), b[end:]...)
	if len(dropped) != 0 {
		if err := checkDroppedUses(out, dropped); err != nil {
			return nil, got, nil, nil, err
		}
	}
	return out, got, kept, dropped, nil
}

// quickjsLibcPart returns where the part of quickjs-libc.c starts and ends in
// the generated file b: from the line after quickjsLibcAfter to __ccgo_fp.
// It starts with the empty line after quickjsLibcAfter, and ends with the
// one before __ccgo_fp.
func quickjsLibcPart(b []byte) (start, end int, err error) {
	loc := findAll(funcHead(quickjsLibcAfter), b)
	if len(loc) != 1 {
		return 0, 0, fmt.Errorf("want 1 function %s, the last of cutils.c, found %d", quickjsLibcAfter, len(loc))
	}
	n := bytes.Index(b[loc[0][0]:], []byte("\n}\n"))
	if n < 0 {
		return 0, 0, fmt.Errorf("end of %s not found", quickjsLibcAfter)
	}
	start = loc[0][0] + n + 3
	if c := bytes.Count(b, []byte(quickjsLibcEnd)); c != 1 {
		return 0, 0, fmt.Errorf("want 1 function __ccgo_fp, found %d", c)
	}
	end = bytes.Index(b, []byte(quickjsLibcEnd)) + 1
	if end < start {
		return 0, 0, fmt.Errorf("__ccgo_fp comes before %s", quickjsLibcAfter)
	}
	return start, end, nil
}

// quickjsLibcDecls lists the functions and variables of part, the part of
// quickjs-libc.c of a generated file, with the functions and variables of
// part each of them uses. The constants and types are left out.
func quickjsLibcDecls(part []byte) ([]libcDecl, error) {
	fset := token.NewFileSet()
	src := append([]byte(parsePrefix), part...)
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("quickjs-libc.c: %v", err)
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset - len(parsePrefix) }

	// Where each thing at the top level starts: a declaration with its doc
	// comment, or a comment of its own.
	var starts []int
	for _, d := range f.Decls {
		starts = append(starts, off(declStart(d)))
	}
	for _, c := range f.Comments {
		starts = append(starts, off(c.Pos()))
	}
	sort.Ints(starts)
	nextAfter := func(end int) int {
		i := sort.SearchInts(starts, end)
		if i == len(starts) {
			return len(part)
		}
		return starts[i]
	}

	var decls []libcDecl
	var nodes []ast.Decl
	for _, d := range f.Decls {
		var name string
		switch d := d.(type) {
		case *ast.FuncDecl:
			name = d.Name.Name
			if d.Recv != nil {
				return nil, fmt.Errorf("quickjs-libc.c: a method %s", name)
			}
			if name == "init" {
				if name, err = initTarget(d); err != nil {
					return nil, err
				}
				name = "init(" + name + ")"
			}
		case *ast.GenDecl:
			if d.Tok != token.VAR {
				continue
			}
			if len(d.Specs) != 1 || len(d.Specs[0].(*ast.ValueSpec).Names) != 1 {
				return nil, fmt.Errorf("quickjs-libc.c: a var declaration of more than one variable at %d", off(d.Pos()))
			}
			name = d.Specs[0].(*ast.ValueSpec).Names[0].Name
		default:
			return nil, fmt.Errorf("quickjs-libc.c: an unexpected declaration at %d", off(d.Pos()))
		}
		end := off(d.End())
		decls = append(decls, libcDecl{name: name, start: off(declStart(d)), end: end, next: nextAfter(end)})
		nodes = append(nodes, d)
	}

	known, vars := map[string]bool{}, map[string]bool{}
	for i, d := range decls {
		if known[d.name] {
			return nil, fmt.Errorf("quickjs-libc.c: %s twice", d.name)
		}
		known[d.name] = true
		if g, ok := nodes[i].(*ast.GenDecl); ok && g.Tok == token.VAR {
			vars[d.name] = true
		}
	}
	// The closure reaches an init function through the variable it fills,
	// so one filling anything else would be dropped while what it fills
	// stays.
	for _, d := range decls {
		if v, ok := strings.CutPrefix(d.name, "init("); ok && !vars[strings.TrimSuffix(v, ")")] {
			return nil, fmt.Errorf("quickjs-libc.c: an init function fills %s, which is not a variable of quickjs-libc.c: see docs/fork-changes/trimmed.md", strings.TrimSuffix(v, ")"))
		}
	}
	for i, n := range nodes {
		seen := map[string]bool{decls[i].name: true}
		ast.Inspect(n, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && known[id.Name] && !seen[id.Name] {
				seen[id.Name] = true
				decls[i].refs = append(decls[i].refs, id.Name)
			}
			return true
		})
	}
	return decls, nil
}

// declStart is where d starts, with its doc comment.
func declStart(d ast.Decl) token.Pos {
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Doc != nil {
			return d.Doc.Pos()
		}
	case *ast.GenDecl:
		if d.Doc != nil {
			return d.Doc.Pos()
		}
	}
	return d.Pos()
}

// initTarget returns the variable an init function of ccgo fills, which its
// first statement takes the address of: p := unsafe.Pointer(&v).
func initTarget(d *ast.FuncDecl) (string, error) {
	if len(d.Body.List) != 0 {
		if a, ok := d.Body.List[0].(*ast.AssignStmt); ok && len(a.Lhs) == 1 && isIdent(a.Lhs[0], "p") && len(a.Rhs) == 1 {
			if c, ok := a.Rhs[0].(*ast.CallExpr); ok && isSelector(c.Fun, "unsafe", "Pointer") && len(c.Args) == 1 {
				if u, ok := c.Args[0].(*ast.UnaryExpr); ok && u.Op == token.AND {
					if id, ok := u.X.(*ast.Ident); ok {
						return id.Name, nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("quickjs-libc.c: an init function that does not start with p := unsafe.Pointer(&v)")
}

// checkDroppedUses fails when b still uses a name of dropped.
func checkDroppedUses(b []byte, dropped []string) error {
	gone := map[string]bool{}
	for _, n := range dropped {
		if !strings.HasPrefix(n, "init(") {
			gone[n] = true
		}
	}
	fset := token.NewFileSet()
	var s scanner.Scanner
	s.Init(fset.AddFile("", -1, len(b)), b, nil, 0)
	used := map[string]bool{}
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok == token.IDENT && gone[lit] {
			used[lit] = true
		}
	}
	if len(used) != 0 {
		var names []string
		for n := range used {
			names = append(names, n)
		}
		sort.Strings(names)
		return fmt.Errorf("quickjs-libc.c: %s used outside what stays: add what the Go API or the test262 runner calls to quickjsLibcRoots, see docs/fork-changes/trimmed.md", strings.Join(names, ", "))
	}
	return nil
}

// quickjsLibcReport prints quickjsLibcSites for the generated files files,
// which must be as taken from upstream, with what stays and what goes of
// each.
func quickjsLibcReport(files []string, hw []*ast.File) error {
	sort.Strings(files)
	var lists strings.Builder
	fmt.Println("var quickjsLibcSites = map[string]libcSite{")
	for _, fn := range files {
		name := filepath.Base(fn)
		b, err := readEngine(fn)
		if err == nil {
			b, err = fixBeforeUp(b, hw, name)
		}
		if err == nil {
			b, err = fixUp(b)
		}
		if err != nil {
			return fmt.Errorf("%s: %v", fn, err)
		}
		_, got, kept, dropped, err := trimQuickJSLibc(b)
		if err != nil {
			return fmt.Errorf("%s: %v", fn, err)
		}
		if got.names == "" {
			return fmt.Errorf("%s: fixlib has trimmed quickjs-libc.c already: run -qjslibc on the files as taken from upstream", fn)
		}
		fmt.Printf("\t%q: {names: %q, kept: %q},\n", name, got.names, got.kept)
		fmt.Fprintf(&lists, "// %s keeps %d: %s\n// %s drops %d: %s\n", name, len(kept), strings.Join(kept, " "), name, len(dropped), strings.Join(dropped, " "))
	}
	fmt.Println("}")
	fmt.Print(lists.String())
	return nil
}
