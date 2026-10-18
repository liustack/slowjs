// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
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
// use of each must be one of the two. Every other use of bp must be a read
// or a write through (*T)(unsafe.Pointer(bp + K)) of a T whose bytes stay
// out of the cells, or one of cellsFrameUses, so that nothing else reads or
// writes the bytes of a cell. Any other use of a cell or of bp, a third
// cell or a different layout of JSValue stops fixlib, and so does a fixed
// file that still uses the bytes where the cells were. The frame keeps its
// size, so the TLS stack slots stay what they were. The 32-bit targets,
// where a JSValue is a NaN-boxed uint64, are not listed in cellFiles and
// keep their cells.

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

// cellsFrameUses are the uses of bp in _JS_CallInternal other than a read
// or a write through unsafe.Pointer(bp + K), and how many times each comes
// in every file of cellFiles. None of them may reach a cell.
var cellsFrameUses = map[string]int{
	"_find_own_property(bp+288)":                 1, // pr, a named local: the property found goes there
	"_find_own_property(bp+312)":                 1, // pr
	"_find_own_property(bp+336)":                 1, // pr
	"_find_own_property(bp+360)":                 1, // pr
	"_build_arg_list(bp+168)":                    1, // len, a named local: the length of the list goes there
	"_js_add_slow(bp+480+uintptr(2)*16)":         1, // the end of ops, a named [2]TJSValue: the two values before it are read and written
	"_js_unary_arith_slow(bp+736+uintptr(1)*16)": 1, // the end of op1, a named TJSValue: the value before it
	"_js_unary_arith_slow(bp+752+uintptr(1)*16)": 1, // the end of op1
	"libc.VaList(bp+1256)":                       2, // the arguments of an error message, 8 bytes each, past the named locals
	"local_buf = bp + uintptr(1280)":             1, // the room of the alloca, past the frame (see fixAlloca)
	"sf = bp + 96":                               2, // sf_s, the named stack frame
}

const (
	cellsInGo = "// Changed by internal/fixlib: the cells of JS_DupValue and JS_FreeValue (see fixCells).\n"

	// The layout of a JSValue fixCells relies on.
	cellsValue = "\ntype TJSValue = struct {\n\tFu   TJSValueUnion\n\tFtag Tint64_t\n}\n"
)

var (
	reNamedCell  = regexp.MustCompile(`/\* \w+ at bp\+(\d+) \*/`)
	reNamedType  = regexp.MustCompile(`\n\tvar _ /\* \w+ at bp\+(\d+) \*/ ([^\n]+)\n`)
	reCellsLine  = regexp.MustCompile(`\n\tvar cell(\d+), cell(\d+) TJSValue ` + regexp.QuoteMeta(cellsInGo))
	reCellsUnion = regexp.MustCompile(`\ntype TJSValueUnion = struct \{\n(?:\t\w+ +\[0\]\w+\n)*\tFuint641 +Tuint64_t\n\}\n`)
)

// cellUses are the uses of one unnamed cell: the stores
// *(*TJSValue)(unsafe.Pointer(bp + K)) = v (the left side) and the loads
// *(*uintptr)(unsafe.Pointer(bp + K)), and the first other use.
type cellUses struct {
	stores, loads []ast.Expr
	other         ast.Node
}

// bpUse is one use of bp in a function: a read or a write through
// (*T)(unsafe.Pointer(bp + K)), with the stack of nodes down to bp + K, or
// any other use, which key names.
type bpUse struct {
	node  ast.Expr
	k     int
	typ   ast.Expr
	stack []ast.Node
	key   string
}

func fixCells(b []byte, listed bool) ([]byte, error) {
	if !listed {
		return b, nil
	}
	// The union is the 8 bytes of Fuint641 and fields of no size.
	if !bytes.Contains(b, []byte(cellsValue)) || !reCellsUnion.Match(b) || !bytes.Contains(b, []byte("\ntype Tuint64_t = uint64\n")) {
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
	named := map[int]string{}
	for _, m := range reNamedType.FindAllSubmatch(body, -1) {
		k, _ := strconv.Atoi(string(m[1]))
		named[k] = string(m[2])
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

	direct, others, err := bpUses(f)
	if err != nil {
		return nil, err
	}

	// The cells are the unnamed offsets that a whole JSValue is stored at.
	cells := map[int]*cellUses{}
	for _, u := range direct {
		if kind, _ := dupFreeUse(u.stack); kind == "store" && !skip[u.k] {
			cells[u.k] = &cellUses{}
		}
	}
	var offsets []int
	for k := range cells {
		offsets = append(offsets, k)
	}
	sort.Ints(offsets)

	if m := reCellsLine.FindSubmatch(body); m != nil {
		// Fixed already: nothing may use the bytes where the cells were.
		if len(offsets) != 0 {
			return nil, fmt.Errorf("_JS_CallInternal: cells declared and others %v still in the frame", offsets)
		}
		var gone []int
		for _, c := range m[1:] {
			k, _ := strconv.Atoi(string(c))
			gone = append(gone, k)
		}
		for _, u := range direct {
			w, err := accessSize(u, named)
			if err != nil {
				return nil, err
			}
			for _, c := range gone {
				if u.k < c+16 && c < u.k+w {
					return nil, fmt.Errorf("_JS_CallInternal: bp+%d: the cell bp+%d is a Go variable, but the frame is still used there: %s", u.k, c, f.lineOf(u.node))
				}
			}
		}
		if err := checkFrameUses(f, others, gone); err != nil {
			return nil, err
		}
		return b, nil
	}

	// Every use of a cell is a store or a load, and nothing else reaches
	// into one.
	for _, u := range direct {
		if c := cells[u.k]; c != nil {
			switch kind, star := dupFreeUse(u.stack); kind {
			case "store":
				c.stores = append(c.stores, star)
			case "load":
				c.loads = append(c.loads, star)
			default:
				if c.other == nil {
					c.other = u.node
				}
			}
			continue
		}
		for _, c := range offsets {
			if c < u.k && u.k < c+16 {
				return nil, fmt.Errorf("_JS_CallInternal: bp+%d inside the cell bp+%d", u.k, c)
			}
		}
		w, err := accessSize(u, named)
		if err != nil {
			return nil, err
		}
		for _, c := range offsets {
			if u.k < c && c < u.k+w {
				return nil, fmt.Errorf("_JS_CallInternal: bp+%d reaches into the cell bp+%d: %s", u.k, c, f.lineOf(u.node))
			}
		}
	}
	if err := checkFrameUses(f, others, offsets); err != nil {
		return nil, err
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

// bpUses returns the uses of bp in f: the reads and writes through
// (*T)(unsafe.Pointer(bp + K)), and the others, each named by its call
// (f(bp+K)) or its assignment (x = bp + K). The definitions of bp by
// tls.Alloc are not uses. bp as the start of a longer sum, or bp + K not
// converted to a pointer, inside unsafe.Pointer(...) is an error: it may
// reach into a cell whatever K is.
func bpUses(f *parsedFunc) (direct, others []bpUse, err error) {
	var stack []ast.Node
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if err != nil {
			return false
		}
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		stack = append(stack, n)
		if id, ok := n.(*ast.Ident); !ok || id.Name != "bp" {
			return true
		}
		// The use is bp and the sums it starts: bp + K + ...
		i := len(stack) - 1
		for i > 0 {
			be, ok := stack[i-1].(*ast.BinaryExpr)
			if !ok || be.Op != token.ADD || be.X != stack[i] {
				break
			}
			i--
		}
		use := stack[i].(ast.Expr)
		switch p := stack[i-1].(type) {
		case *ast.AssignStmt:
			if len(p.Lhs) != 1 || len(p.Rhs) != 1 {
				break
			}
			if p.Lhs[0] == use {
				if _, ok := tlsCall(p.Rhs[0], "Alloc"); ok && isIdent(use, "bp") {
					return true
				}
				break
			}
			others = append(others, bpUse{node: use, key: f.text(p.Lhs[0]) + " = " + f.text(use)})
			return true
		case *ast.CallExpr:
			if isSelector(p.Fun, "unsafe", "Pointer") {
				k, ok := frameCell(use)
				var typ ast.Expr
				if conv, isCall := stack[i-2].(*ast.CallExpr); ok && isCall && len(conv.Args) == 1 {
					typ, ok = ptrType(conv.Fun, 1)
				} else {
					ok = false
				}
				if !ok {
					err = fmt.Errorf("_JS_CallInternal: %s: an address in the frame other than a pointer to bp + K, which may reach a cell: %s", f.text(use), f.lineOf(use))
					return false
				}
				direct = append(direct, bpUse{node: use, k: k, typ: typ, stack: append([]ast.Node{}, stack[:i+1]...)})
				return true
			}
			others = append(others, bpUse{node: use, key: f.text(p.Fun) + "(" + f.text(use) + ")"})
			return true
		}
		others = append(others, bpUse{node: use, key: f.lineOf(use)})
		return true
	})
	return direct, others, err
}

// checkFrameUses checks the uses of bp other than reads and writes through
// a pointer against cellsFrameUses: each must be listed, as many times as
// it comes, and none may start inside a cell.
func checkFrameUses(f *parsedFunc, others []bpUse, cells []int) error {
	found := map[string]int{}
	for _, u := range others {
		if k, ok := baseOffset(u.node); ok {
			for _, c := range cells {
				if c <= k && k < c+16 {
					return fmt.Errorf("_JS_CallInternal: %s: inside the cell bp+%d", u.key, c)
				}
			}
		}
		if found[u.key] == cellsFrameUses[u.key] {
			return fmt.Errorf("_JS_CallInternal: %s: a use of bp that cellsFrameUses does not list, or more of it: check that it cannot reach a cell: %s", u.key, f.lineOf(u.node))
		}
		found[u.key]++
	}
	var keys []string
	for key := range cellsFrameUses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if n := cellsFrameUses[key]; found[key] != n {
			return fmt.Errorf("_JS_CallInternal: %s: listed in cellsFrameUses %d times, found %d", key, n, found[key])
		}
	}
	return nil
}

// baseOffset returns K of the sum bp + K that a use starts with.
func baseOffset(e ast.Expr) (int, bool) {
	for {
		if k, ok := frameCell(e); ok {
			return k, true
		}
		be, ok := e.(*ast.BinaryExpr)
		if !ok || be.Op != token.ADD {
			return 0, false
		}
		e = be.X
	}
}

// valueSizes are the sizes in bytes of the types the interpreter reads and
// writes in its frame, on a 64-bit target.
var valueSizes = map[string]int{
	"int8": 1, "uint8": 1, "byte": 1, "Tint8_t": 1, "Tuint8_t": 1,
	"int16": 2, "uint16": 2, "Tint16_t": 2, "Tuint16_t": 2,
	"int32": 4, "uint32": 4, "float32": 4, "Tint32_t": 4, "Tuint32_t": 4,
	"int64": 8, "uint64": 8, "float64": 8, "uintptr": 8, "Tint64_t": 8, "Tuint64_t": 8, "Tsize_t": 8,
	"TJSValue": 16,
}

// accessSize returns how many bytes a read or a write through
// (*T)(unsafe.Pointer(bp + K)) may touch: the size of T, or, for a type of
// unknown size, none past the named local at K of that same type.
func accessSize(u bpUse, named map[int]string) (int, error) {
	if n, ok := typeSize(u.typ); ok {
		return n, nil
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), u.typ); err != nil {
		return 0, err
	}
	if named[u.k] == buf.String() {
		return 1, nil // the named local itself, which ccgo laid out apart from the cells
	}
	return 0, fmt.Errorf("_JS_CallInternal: bp+%d: a %s, whose size is not known: add it to valueSizes", u.k, buf.String())
}

func typeSize(e ast.Expr) (int, bool) {
	switch t := e.(type) {
	case *ast.Ident:
		n, ok := valueSizes[t.Name]
		return n, ok
	case *ast.ArrayType:
		n, ok := intLit(t.Len)
		if !ok {
			return 0, false
		}
		m, ok := typeSize(t.Elt)
		return n * m, ok
	}
	return 0, false
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
