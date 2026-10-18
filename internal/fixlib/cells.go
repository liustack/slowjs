// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"sort"
	"strconv"
)

// The interpreter inlines JS_DupValue and JS_FreeValue, and ccgo gives the
// JSValue each of them takes a cell of the TLS frame, because
// JS_VALUE_GET_PTR reads the value through its address. Each expansion
// stores the whole value into the cell and loads its first word back as
// the pointer: one write and one read of memory for every reference count
// the interpreter changes.
//
// fixCells makes the two cells Go variables of type TJSValue, named after
// their offsets (cell48, cell64): the store becomes an assignment and the
// load reads the union, uintptr(cellK.Fu.Fuint641). On a 64-bit target the
// union is the first 8 bytes of a JSValue and Fuint641 all of them, so the
// load gives the same bits as before, whatever the byte order. The cells
// are the constant offsets in the frame where a whole JSValue is stored
// that no named local (/* name at bp+K */) or parameter copy has, and every
// use of each must be one of the two. Any other use, an access inside a
// cell, a third cell or a different layout of JSValue stops fixlib. The frame keeps its size, so the TLS
// stack slots stay what they were. The 32-bit targets, where a JSValue is
// a NaN-boxed uint64, are not listed in cellFiles and keep their cells.

// cellFiles are the generated files of 64-bit targets.
var cellFiles = map[string]bool{
	"ccgo_darwin_amd64.go":  true,
	"ccgo_darwin_arm64.go":  true,
	"ccgo_freebsd_amd64.go": true,
	"ccgo_freebsd_arm64.go": true,
	"ccgo_linux_amd64.go":   true,
	"ccgo_linux_arm64.go":   true,
	"ccgo_linux_loong64.go": true,
	"ccgo_linux_ppc64le.go": true,
	"ccgo_linux_riscv64.go": true,
	"ccgo_linux_s390x.go":   true,
	"ccgo_windows.go":       true,
}

const (
	cellsInGo = "// Changed by internal/fixlib: the cells of JS_DupValue and JS_FreeValue (see fixCells).\n"

	// The layout of a JSValue fixCells relies on.
	cellsValue = "\ntype TJSValue = struct {\n\tFu   TJSValueUnion\n\tFtag Tint64_t\n}\n"
	cellsUnion = "\n\tFuint641       Tuint64_t\n}\n"
)

var (
	reNamedCell = regexp.MustCompile(`/\* \w+ at bp\+(\d+) \*/`)
	reCellsLine = regexp.MustCompile(`\n\tvar cell\d+, cell\d+ TJSValue ` + regexp.QuoteMeta(cellsInGo))
)

// cellUses are the uses of one unnamed cell: the stores
// *(*TJSValue)(unsafe.Pointer(bp + K)) = v (the left side) and the loads
// *(*uintptr)(unsafe.Pointer(bp + K)), and the first other use.
type cellUses struct {
	stores, loads []ast.Expr
	other         ast.Node
}

func fixCells(b []byte, listed bool) ([]byte, error) {
	if !listed {
		return b, nil
	}
	if !bytes.Contains(b, []byte(cellsValue)) || !regexp.MustCompile(`\ntype TJSValueUnion = struct \{\n[^}]*`+regexp.QuoteMeta(cellsUnion)).Match(b) {
		return nil, fmt.Errorf("JSValue is not a union of 8 bytes and a tag of 8 bytes")
	}
	body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
	if err != nil {
		return nil, err
	}
	f, err := parseFunc("_JS_CallInternal", body)
	if err != nil {
		return nil, err
	}

	// Offsets of named locals and of parameter copies are not cells.
	skip := map[int]bool{}
	for _, m := range reNamedCell.FindAllSubmatch(body, -1) {
		k, _ := strconv.Atoi(string(m[1]))
		skip[k] = true
	}
	params := map[string]bool{}
	for _, field := range f.decl.Type.Params.List {
		for _, n := range field.Names {
			params[n.Name] = true
		}
	}
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if s, ok := n.(ast.Stmt); ok {
			if k, p, ok := paramCopy(s); ok && params[p] {
				skip[k] = true
			}
		}
		return true
	})

	// Classify every other bp + K by what holds it: the stack ends with
	// the star, the conversion, unsafe.Pointer(...) and the sum, and what
	// comes before is the parent of the star.
	cells := map[int]*cellUses{}
	var stack []ast.Node
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		stack = append(stack, n)
		sum, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		k, ok := frameCell(sum)
		if !ok || skip[k] {
			return true
		}
		u := cells[k]
		if u == nil {
			u = &cellUses{}
			cells[k] = u
		}
		switch kind, star := dupFreeUse(stack); kind {
		case "store":
			u.stores = append(u.stores, star)
		case "load":
			u.loads = append(u.loads, star)
		default:
			if u.other == nil {
				u.other = n
			}
		}
		return true
	})

	// The cells are the offsets that a JSValue is stored at. Others, such
	// as the room for the arguments of libc.VaList, stay, but none may lie
	// inside a cell.
	var offsets []int
	for k, u := range cells {
		if len(u.stores) > 0 {
			offsets = append(offsets, k)
		}
	}
	sort.Ints(offsets)
	if reCellsLine.Match(body) {
		if len(offsets) == 0 {
			return b, nil // fixed already
		}
		return nil, fmt.Errorf("_JS_CallInternal: cells declared and others %v still in the frame", offsets)
	}
	for k := range cells {
		for _, c := range offsets {
			if c < k && k < c+16 {
				return nil, fmt.Errorf("_JS_CallInternal: bp+%d inside the cell bp+%d", k, c)
			}
		}
	}
	if len(offsets) != 2 {
		return nil, fmt.Errorf("_JS_CallInternal: %d cells %v in the frame, want the 2 of JS_DupValue and JS_FreeValue", len(offsets), offsets)
	}
	var edits []edit
	for _, k := range offsets {
		u := cells[k]
		if u.other != nil {
			return nil, fmt.Errorf("_JS_CallInternal: bp+%d: a use other than a store of a JSValue or a load of a pointer: %s", k, f.lineOf(u.other))
		}
		if len(u.stores) == 0 || len(u.loads) == 0 {
			return nil, fmt.Errorf("_JS_CallInternal: bp+%d: %d stores and %d loads", k, len(u.stores), len(u.loads))
		}
		name := "cell" + strconv.Itoa(k)
		if regexp.MustCompile(`\b` + name + `\b`).Match(body) {
			return nil, fmt.Errorf("_JS_CallInternal: %s already used", name)
		}
		for _, s := range u.stores {
			edits = append(edits, edit{f.off(s.Pos()), f.off(s.End()), name})
		}
		for _, l := range u.loads {
			edits = append(edits, edit{f.off(l.Pos()), f.off(l.End()), "uintptr(" + name + ".Fu.Fuint641)"})
		}
	}
	// Declare the cells with the other locals, before the statement that
	// uses them all, so that no goto jumps over the declaration.
	at, err := blankUse(f)
	if err != nil {
		return nil, err
	}
	edits = append(edits, edit{at, at, fmt.Sprintf("\tvar cell%d, cell%d TJSValue %s", offsets[0], offsets[1], cellsInGo)})
	return bytes.Replace(b, body, f.apply(edits), 1), nil
}

// dupFreeUse tells what the sum bp + K at the end of stack is used for: a
// "store" of a whole JSValue, *(*TJSValue)(unsafe.Pointer(bp + K)) = v
// where v does not read the cell, a "load" of the first word as a value,
// *(*uintptr)(unsafe.Pointer(bp + K)), or "" for anything else. It returns
// the star expression of a store or a load.
func dupFreeUse(stack []ast.Node) (string, ast.Expr) {
	if len(stack) < 5 {
		return "", nil
	}
	star, ok := stack[len(stack)-4].(*ast.StarExpr)
	if !ok {
		return "", nil
	}
	cell, typ, ok := cellLoad(star)
	if !ok || cell != stack[len(stack)-1] {
		return "", nil
	}
	switch parent := stack[len(stack)-5].(type) {
	case *ast.AssignStmt:
		if parent.Tok != token.ASSIGN || len(parent.Lhs) != 1 || len(parent.Rhs) != 1 {
			return "", nil
		}
		if parent.Lhs[0] == ast.Expr(star) {
			k, _ := frameCell(cell)
			if isIdent(typ, "TJSValue") && !usesCell(parent.Rhs[0], k) {
				return "store", star
			}
			return "", nil
		}
		if isIdent(typ, "uintptr") {
			return "load", star
		}
	case *ast.BinaryExpr, *ast.CallExpr, *ast.ReturnStmt, *ast.ParenExpr:
		if isIdent(typ, "uintptr") {
			return "load", star
		}
	}
	return "", nil
}

// usesCell reports whether e uses bp + k.
func usesCell(e ast.Expr, k int) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if sum, ok := n.(*ast.BinaryExpr); ok {
			if c, ok := frameCell(sum); ok && c == k {
				found = true
			}
		}
		return !found
	})
	return found
}

// blankUse returns the offset of the line of the statement that assigns
// every local to _, which ccgo writes right after the declarations.
func blankUse(f *parsedFunc) (int, error) {
	for _, s := range f.decl.Body.List {
		a, ok := s.(*ast.AssignStmt)
		if !ok || a.Tok != token.ASSIGN || len(a.Lhs) < 2 {
			continue
		}
		all := true
		for _, l := range a.Lhs {
			all = all && isIdent(l, "_")
		}
		if all {
			start, _ := f.line(a)
			return start, nil
		}
	}
	return 0, fmt.Errorf("_JS_CallInternal: cannot find where the locals are used")
}

// lineOf returns the line of n, without its indentation.
func (f *parsedFunc) lineOf(n ast.Node) string {
	start, end := f.line(n)
	return string(bytes.TrimSpace(f.src[start:end]))
}
