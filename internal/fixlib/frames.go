// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// A TLS frame (bp := tls.Alloc(N) at the entry of a generated function) is
// where ccgo puts the C locals whose address is taken, because the
// goroutine stack moves and a C pointer into it would not. In most small
// functions nothing takes such an address: ccgo stores a JSValue into a
// cell of the frame to read it back as a pointer (the expansions of
// JS_VALUE_GET_PTR in JS_FreeValue and JS_DupValue), or copies a JSValue
// parameter into one to do the same. Each cell is then only loaded or
// stored right where it is named, so a Go array on the goroutine stack
// holds the same bytes, and the function needs neither tls.Alloc,
// tls.Free nor their defer.
//
// fixFrames does that for the functions of frameSites. A function
// qualifies when:
//
//  1. every use of bp is a load or a store of a cell at a constant offset,
//     *(*T)(unsafe.Pointer(bp + K)) or **(**T)(__ccgo_up(bp + K)), or a
//     field of one, and nothing takes the address of a cell, passes bp on
//     or stores it;
//  2. it cannot reach a stack check (_js_check_stack_overflow), following
//     the direct calls of the generated file and of the hand-written files
//     of lib/. An indirect call counts as reaching one, except a call of a
//     class finalizer, a GC mark function or the allocator.
//
// The stack check counts TLS stack slots. A function that cannot reach it
// is never on the call chain when it runs, so dropping its slot changes no
// check, and the depth a script can recurse to stays the same.

// frameSite is a function of frameSites: its frame size and number of
// cells on 64-bit targets, and why it is worth the change.
type frameSite struct {
	frame, cells int
	why          string
}

// indirectOK are the fields an indirect call may go through without
// counting as reaching a stack check: class finalizers and GC mark
// functions, which the engine documents may not run javascript, and the
// allocator.
var indirectOK = map[string]bool{
	"Ffinalizer":             true,
	"Fgc_mark":               true,
	"Fjs_malloc":             true,
	"Fjs_free":               true,
	"Fjs_realloc":            true,
	"Fjs_calloc":             true,
	"Fjs_malloc_usable_size": true,
}

// callNode is one function of the call graph.
type callNode struct {
	callees  map[string]bool
	indirect []string // indirect calls that may reach anything
	checks   bool     // calls _js_check_stack_overflow
}

// handWritten parses the hand-written Go files of lib/, whose functions
// the generated code calls.
func handWritten(dir string) ([]*ast.File, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	var out []*ast.File
	for _, fn := range files {
		base := filepath.Base(fn)
		if strings.HasPrefix(base, "ccgo_") || strings.HasSuffix(base, "_test.go") {
			continue
		}
		b, err := os.ReadFile(fn)
		if err != nil {
			return nil, err
		}
		if bytes.Contains(b, []byte("//go:build ignore")) {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), fn, b, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// callGraph returns the call graph of the functions of files.
func callGraph(files []*ast.File) map[string]*callNode {
	g := map[string]*callNode{}
	imports := map[string]bool{}
	for _, f := range files {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			name := path[strings.LastIndex(path, "/")+1:]
			if imp.Name != nil {
				name = imp.Name.Name
			}
			imports[name] = true
		}
	}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			if fd.Recv != nil {
				continue // the methods are of the hand-written files, which the engine does not call
			}
			n := g[name]
			if n == nil {
				n = &callNode{callees: map[string]bool{}}
				g[name] = n
			}
			// The fields the local variables of function pointers are
			// loaded from, as free_object loads its finalizer.
			fields := map[string]map[string]bool{}
			ast.Inspect(fd.Body, func(x ast.Node) bool {
				a, ok := x.(*ast.AssignStmt)
				if !ok || len(a.Lhs) != len(a.Rhs) {
					return true
				}
				for i, l := range a.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						if fields[id.Name] == nil {
							fields[id.Name] = map[string]bool{}
						}
						field := ""
						if s, ok := a.Rhs[i].(*ast.SelectorExpr); ok {
							field = s.Sel.Name
						}
						fields[id.Name][field] = true
					}
				}
				return true
			})
			ast.Inspect(fd.Body, func(x ast.Node) bool {
				c, ok := x.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fn := c.Fun.(type) {
				case *ast.Ident:
					switch fn.Name {
					case "_js_check_stack_overflow":
						n.checks = true
					case "__ccgo_fp":
						// A function whose address is taken may be called
						// through it: count it as called here.
						if len(c.Args) == 1 {
							if id, ok := c.Args[0].(*ast.Ident); ok {
								n.callees[id.Name] = true
							}
						}
					default:
						n.callees[fn.Name] = true
					}
					return true
				case *ast.SelectorExpr:
					if id, ok := fn.X.(*ast.Ident); ok && (imports[id.Name] || id.Name == "tls") {
						return true // another package, or the TLS
					}
				case *ast.FuncLit:
					return true // its body is looked at as part of this function
				case *ast.ParenExpr, *ast.ArrayType, *ast.StarExpr, *ast.FuncType, *ast.InterfaceType, *ast.MapType, *ast.ChanType:
					if isConversion(c.Fun) {
						return true
					}
				}
				if indirectAllowed(indirectVia(c.Fun), fields) {
					return true
				}
				n.indirect = append(n.indirect, shortText(c.Fun))
				return true
			})
		}
	}
	return g
}

// isConversion reports whether a call of fun is a conversion: fun is a
// type, (*T), (*func(...) R) or []T, rather than a value.
func isConversion(fun ast.Expr) bool {
	for {
		switch e := fun.(type) {
		case *ast.ParenExpr:
			fun = e.X
		case *ast.StarExpr:
			fun = e.X
		case *ast.Ident, *ast.SelectorExpr, *ast.FuncType, *ast.ArrayType, *ast.StructType, *ast.MapType, *ast.ChanType, *ast.InterfaceType:
			return true
		default:
			return false
		}
	}
}

// indirectAllowed reports whether an indirect call through via (a field
// name, or a variable) may not count as reaching a stack check: via is a
// field of indirectOK, a variable that is only loaded from such fields, or
// the mark function JS_MarkValue gets from the GC.
func indirectAllowed(via string, fields map[string]map[string]bool) bool {
	if indirectOK[via] || via == "__ccgo_fp_mark_func" {
		return true
	}
	from := fields[via]
	if len(from) == 0 {
		return false
	}
	for f := range from {
		if !indirectOK[f] {
			return false
		}
	}
	return true
}

// indirectVia returns the last field name, or the variable, that holds the
// function pointer of an indirect call as ccgo writes it,
// (*(*func(...) R)(unsafe.Pointer(&struct{ uintptr }{X})))(...), or "".
func indirectVia(fun ast.Expr) string {
	var lit *ast.CompositeLit
	ast.Inspect(fun, func(n ast.Node) bool {
		if l, ok := n.(*ast.CompositeLit); ok && lit == nil {
			lit = l
		}
		return lit == nil
	})
	if lit == nil || len(lit.Elts) != 1 {
		return ""
	}
	switch e := lit.Elts[0].(type) {
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.Ident:
		return e.Name
	}
	return ""
}

func shortText(e ast.Node) string {
	var buf bytes.Buffer
	ast.Fprint(&buf, nil, e, nil)
	s := strings.Join(strings.Fields(buf.String()), " ")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

// reaching returns the functions of g that can reach a stack check, and
// for each the first step on the way. It goes through functions and
// callees in name order, so that the steps are the same in every run.
func reaching(g map[string]*callNode) map[string]string {
	names := make([]string, 0, len(g))
	for name := range g {
		names = append(names, name)
	}
	sort.Strings(names)
	callees := map[string][]string{}
	why := map[string]string{}
	for _, name := range names {
		n := g[name]
		switch {
		case n.checks:
			why[name] = "_js_check_stack_overflow"
		case len(n.indirect) != 0:
			why[name] = "an indirect call"
		}
		for c := range n.callees {
			callees[name] = append(callees[name], c)
		}
		sort.Strings(callees[name])
	}
	for changed := true; changed; {
		changed = false
		for _, name := range names {
			if why[name] != "" {
				continue
			}
			for _, c := range callees[name] {
				if why[c] != "" {
					why[name] = c
					changed = true
					break
				}
			}
		}
	}
	return why
}

// frameUse describes the TLS frame of one function.
type frameUse struct {
	frame, cells int
	bad          string // why bp is used in another way, or ""
}

// frameUses returns the frame of every generated function of b that has
// one.
func frameUses(b []byte) (map[string]frameUse, error) {
	out := map[string]frameUse{}
	heads := findAll(reAnyFunc, b)
	for i, h := range heads {
		end := len(b)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		text := b[h[0]:end]
		if !bytes.Contains(text, []byte("\n\tbp := tls.Alloc(")) {
			continue
		}
		name := string(b[h[2]:h[3]])
		body, err := funcBody(text, funcHead(name), name)
		if err != nil {
			continue // not a function of the engine (a method, ...)
		}
		f, err := parseFunc(name, body)
		if err != nil {
			return nil, err
		}
		n, ok := frameHead(f.decl.Body.List)
		if !ok {
			continue
		}
		use := frameUse{frame: n}
		cells, bad := frameCells(f)
		use.cells, use.bad = len(cells), bad
		out[name] = use
	}
	return out, nil
}

// cellUse is one use of a frame cell: the expression that reads or writes
// the cell, its offset, and the replacement of the expression that makes
// its address.
type cellUse struct {
	off        int
	start, end int // of the expression to replace, in f.src
	typ        string
	ccgoUp     bool // **(**T)(__ccgo_up(C)): replace *(**T)(__ccgo_up(C))
}

// frameCells returns the uses of the frame cells of f, skipping its first
// two statements (the Alloc and the defer), or why bp is used otherwise.
func frameCells(f *parsedFunc) ([]cellUse, string) {
	var uses []cellUse
	bad := ""
	var stack []ast.Node
	for _, s := range f.decl.Body.List[2:] {
		ast.Inspect(s, func(n ast.Node) bool {
			if bad != "" {
				return false
			}
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			id, ok := n.(*ast.Ident)
			if !ok || id.Name != "bp" {
				return true
			}
			u, why := cellAccess(f, stack)
			if why != "" {
				bad = why
				return false
			}
			uses = append(uses, u)
			return true
		})
		stack = stack[:0]
		if bad != "" {
			return nil, bad
		}
	}
	return uses, ""
}

// cellAccess checks the use of bp at the top of stack: it must be C in
// unsafe.Pointer(C) or __ccgo_up(C), where C is bp or bp + K, converted to
// a pointer that is dereferenced or whose field is taken, and nothing
// takes the address of what that gives.
func cellAccess(f *parsedFunc, stack []ast.Node) (cellUse, string) {
	at := func(i int) ast.Node {
		if i < 0 || i >= len(stack) {
			return nil
		}
		return stack[i]
	}
	i := len(stack) - 1
	cell := ast.Expr(stack[i].(*ast.Ident))
	if be, ok := at(i - 1).(*ast.BinaryExpr); ok && be.X == cell {
		cell = be
		i--
	}
	k, ok := frameCell(cell)
	if !ok {
		return cellUse{}, fmt.Sprintf("bp in %s", f.text(at(i-1)))
	}
	call, ok := at(i - 1).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return cellUse{}, fmt.Sprintf("bp+%d in %s", k, f.text(at(i-1)))
	}
	up := isIdent(call.Fun, "__ccgo_up")
	if !up && !isSelector(call.Fun, "unsafe", "Pointer") {
		return cellUse{}, fmt.Sprintf("bp+%d passed to %s", k, f.text(call.Fun))
	}
	conv, ok := at(i - 2).(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 || conv.Args[0] != call {
		return cellUse{}, fmt.Sprintf("bp+%d not converted to a pointer", k)
	}
	stars := 1
	if up {
		stars = 2
	}
	typ, ok := ptrType(conv.Fun, stars)
	if !ok {
		return cellUse{}, fmt.Sprintf("bp+%d converted by %s", k, f.text(conv.Fun))
	}
	j := i - 2
	repl := ast.Node(conv)
	if up {
		s, ok := at(j - 1).(*ast.StarExpr)
		if !ok {
			return cellUse{}, fmt.Sprintf("bp+%d: __ccgo_up not dereferenced", k)
		}
		repl = s
		j--
	}
	// repl is a *T. Above it: a dereference or a field, then anything
	// but taking an address.
	switch p := at(j - 1).(type) {
	case *ast.StarExpr:
	case *ast.SelectorExpr:
		if p.X != repl {
			return cellUse{}, fmt.Sprintf("bp+%d: a pointer in %s", k, f.text(p))
		}
	case *ast.ParenExpr:
		if s, ok := at(j - 2).(*ast.SelectorExpr); !ok || s.X != p {
			return cellUse{}, fmt.Sprintf("bp+%d: a pointer in %s", k, f.text(at(j-2)))
		}
		j--
	default:
		return cellUse{}, fmt.Sprintf("bp+%d: the pointer used in %s", k, f.text(p))
	}
	for j--; j >= 1; j-- {
		switch p := at(j - 1).(type) {
		case *ast.SelectorExpr, *ast.ParenExpr, *ast.StarExpr:
			continue
		case *ast.IndexExpr:
			if _, ok := intLit(p.Index); ok {
				continue
			}
			return cellUse{}, fmt.Sprintf("bp+%d: a variable index in %s", k, f.text(p))
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				return cellUse{}, fmt.Sprintf("bp+%d: its address taken in %s", k, f.text(p))
			}
		case *ast.SliceExpr:
			return cellUse{}, fmt.Sprintf("bp+%d: sliced in %s", k, f.text(p))
		}
		break
	}
	return cellUse{off: k, start: f.off(repl.Pos()), end: f.off(repl.End()), typ: f.text(typ), ccgoUp: up}, ""
}

// frameReport prints, for every function of the generated file fn with a
// TLS frame, its frame size, how many cell uses it has, and why it cannot
// drop the frame (bp used otherwise, or the way to a stack check), and for
// every function of frameSites, the same for the Go array it has instead.
// A line with no reason is a function fixFrames could take: add it to
// frameSites after reading it.
func frameReport(fn string) error {
	b, err := readEngine(fn)
	if err != nil {
		return err
	}
	uses, err := frameUses(b)
	if err != nil {
		return err
	}
	for name := range frameSites {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return err
		}
		if bytes.Contains(body, []byte(framesInGo)) {
			frame, cells, err := localStats(name, body)
			if err != nil {
				return err
			}
			uses[name] = frameUse{frame: frame, cells: cells}
		}
	}
	f, err := parser.ParseFile(token.NewFileSet(), fn, b, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	hw, err := handWritten(filepath.Dir(fn))
	if err != nil {
		return err
	}
	reach := reaching(callGraph(append(hw, f)))
	names := make([]string, 0, len(uses))
	for name := range uses {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		u := uses[name]
		state := ""
		if _, ok := frameSites[name]; ok {
			state = "\tin frameSites"
		}
		why := ""
		switch {
		case u.bad != "":
			why = "\tbp: " + strings.Join(strings.Fields(u.bad), " ")
		case reach[name] != "":
			why = "\treaches a stack check through " + reach[name]
		}
		fmt.Printf("%s\tframe=%d\tcells=%d%s%s\n", name, u.frame, u.cells, state, why)
	}
	return nil
}

const (
	localFrame  = "localbp"
	framesInGo  = "// Changed by internal/fixlib: the TLS frame is a Go array (see fixFrames).\n"
	localPrefix = "\tvar " + localFrame + " ["
)

// fixFrames replaces the TLS frame of each function of sites (frameSites)
// with a Go array, localbp, and every cell bp + K with an offset into it.
// hw are the hand-written files of lib/. pin is set for
// lib/ccgo_darwin_arm64.go, whose frame sizes and cell counts must be those
// sites lists. A function without a frame is left as it is on 32-bit
// targets, where a JSValue is a uint64 and needs no cell.
func fixFrames(b []byte, hw []*ast.File, sites map[string]frameSite, pin bool) ([]byte, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", b, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	reach := reaching(callGraph(append(hw[:len(hw):len(hw)], f)))
	wide := !bytes.Contains(b, []byte("\ntype TJSValue = uint64\n"))
	names := make([]string, 0, len(sites))
	for name := range sites {
		names = append(names, name)
	}
	sort.Strings(names)
	var problems []string
	for _, name := range names {
		want := sites[name]
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return nil, err
		}
		if r := reach[name]; r != "" {
			problems = append(problems, fmt.Sprintf("%s can reach a stack check, through %s", name, r))
			continue
		}
		var frame, cells int
		fixed := body
		switch {
		case bytes.Contains(body, []byte(framesInGo)):
			frame, cells, err = localStats(name, body)
		case bytes.Contains(body, []byte("\n\tbp := tls.Alloc(")):
			fixed, frame, cells, err = frameFunc(name, body)
		case !wide:
			continue // no frame on a 32-bit target
		default:
			err = fmt.Errorf("%s: no TLS frame", name)
		}
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if pin && (frame != want.frame || cells != want.cells) {
			problems = append(problems, fmt.Sprintf("%s: a frame of %d bytes with %d cell uses, frameSites has %d and %d", name, frame, cells, want.frame, want.cells))
			continue
		}
		b = bytes.Replace(b, body, fixed, 1)
	}
	if len(problems) != 0 {
		return nil, fmt.Errorf("TLS frames changed, review each and update frameSites (go run ./internal/fixlib -frames):\n\t%s", strings.Join(problems, "\n\t"))
	}
	return b, nil
}

// localStats returns the frame size and cell uses of a function fixFrames
// changed.
func localStats(name string, body []byte) (frame, cells int, err error) {
	i := bytes.Index(body, []byte(framesInGo+localPrefix))
	if i < 0 || bytes.Count(body, []byte(framesInGo)) != 1 {
		return 0, 0, fmt.Errorf("%s: the Go array of the frame is not where fixFrames puts it", name)
	}
	rest := body[i+len(framesInGo)+len(localPrefix):]
	j := bytes.Index(rest, []byte("]uint64\n"))
	if j < 0 {
		return 0, 0, fmt.Errorf("%s: cannot read the size of the Go array of the frame", name)
	}
	words, err := strconv.Atoi(string(rest[:j]))
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %v", name, err)
	}
	f, err := parseFunc(name, body)
	if err != nil {
		return 0, 0, err
	}
	if identUsed(f.decl.Body, "bp") {
		return 0, 0, fmt.Errorf("%s: bp is used beside the Go array of the frame", name)
	}
	return 8 * words, bytes.Count(body, []byte("unsafe.Pointer(&"+localFrame+")")), nil
}

// frameFunc rewrites one function whose frame qualifies, and returns its
// frame size and number of cell uses.
func frameFunc(name string, body []byte) ([]byte, int, int, error) {
	f, err := parseFunc(name, body)
	if err != nil {
		return nil, 0, 0, err
	}
	list := f.decl.Body.List
	n, ok := frameHead(list)
	if !ok {
		return nil, 0, 0, fmt.Errorf("%s: the frame is not allocated at the entry", name)
	}
	if identUsed(f.decl.Body, localFrame) {
		return nil, 0, 0, fmt.Errorf("%s: already has a variable %s", name, localFrame)
	}
	uses, bad := frameCells(f)
	if bad != "" {
		return nil, 0, 0, fmt.Errorf("%s: %s", name, bad)
	}
	hstart, _ := f.line(list[0])
	_, hend := f.line(list[1])
	edits := []edit{{hstart, hend, fmt.Sprintf("\t%s%s%d]uint64\n", framesInGo, localPrefix, (n+7)/8)}}
	for _, u := range uses {
		addr := "unsafe.Pointer(&" + localFrame + ")"
		if u.off != 0 {
			addr = fmt.Sprintf("unsafe.Add(%s, %d)", addr, u.off)
		}
		edits = append(edits, edit{u.start, u.end, fmt.Sprintf("(*%s)(%s)", u.typ, addr)})
	}
	out := f.apply(edits)
	g, err := parseFunc(name, out)
	if err != nil {
		return nil, 0, 0, err
	}
	if identUsed(g.decl.Body, "bp") {
		return nil, 0, 0, fmt.Errorf("%s: bp left after the change", name)
	}
	return out, n, len(uses), nil
}
