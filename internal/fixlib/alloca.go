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
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// allocaFuncs are the functions of the engine that call alloca, one call
// each: the four ways of calling a function.
var allocaFuncs = []string{
	"_JS_CallInternal",         // the room for the arguments, locals and stack of a javascript function
	"_js_call_c_function",      // arguments a native function declares and the call does not pass
	"_js_call_bound_function",  // the bound arguments followed by those of the call
	"_js_c_function_data_call", // as js_call_c_function, for a native function with data
}

const (
	// allocaFrame names the variable that holds the size of a frame that
	// takes the room of an alloca.
	allocaFrame = "frame_size"
	// allocaMalloc names the variable that holds a large alloca's malloc,
	// which the deferred function that frees the frame frees too, so that
	// the function keeps a single defer.
	allocaMalloc = "frame_malloc"
	// allocaMax is the largest alloca, in bytes, that goes into the TLS
	// frame. A larger one is malloc'd and freed on return, as a function
	// without a frame does. Linux's libc keeps the TLS block of every call
	// depth until the TLS goes, grown to the largest frame used at that
	// depth, and its allocator rounds a block up to a power of two: the
	// frame of JS_CallInternal on 64-bit targets, 1280 bytes, takes a
	// 2 KB block, and with at most 768 bytes more it still does, so a deep
	// recursion holds no more TLS memory than before, whatever its
	// functions. 768 bytes are 48 values, the arguments, locals and stack
	// of 96% of the calls of a server render of tifa's example shop. The
	// next sizes those calls take are between 1.5 and 2 KB (3% more), which
	// would double the block.
	allocaMax = 768
	// The markers of a function fixAlloca changed, one per function.
	allocaInFrame  = "// Changed by internal/fixlib: the room of a small alloca below is the end of this frame (see fixAlloca).\n"
	allocaInMalloc = "// Changed by internal/fixlib: alloca is malloc, freed on return (see fixAlloca).\n"
)

// allocaTest ends the tests of the size against allocaMax.
var allocaTest = fmt.Sprintf(" <= %d {\n", allocaMax)

var reAllocaRegister = regexp.MustCompile(`tls\.(FreeAlloca\(\)\(\)|AllocaEntry\(\)|AllocaExit\(\))`)

// fixAlloca takes the alloca calls out of the engine. ccgo turns alloca
// into a malloc that libc records in a list, and every function that may
// call alloca registers itself with that list on entry, whether it calls
// alloca or not: on darwin, freebsd and windows the registration returns a
// new closure and the list starts empty, two Go allocations per call of a
// javascript function and one per call of a native function. On linux the
// list keeps its capacity, but every call still mallocs and frees.
//
// In C, alloca makes the frame of the function longer, so that is what the
// rewrite does. A function with a TLS frame (bp := tls.Alloc(N)) frees it
// and allocates N plus the room of the alloca just before its stack check,
// copies its parameters into the new frame again and derives its pointers
// into the frame again, and the alloca becomes bp + N. When the alloca is
// conditional, so is the new frame, and so it is when the alloca is larger
// than allocaMax: then the alloca is a malloc freed on return. Nothing else is on the TLS stack at
// that point, and the frame holds nothing yet but the parameter copies, so
// each call still takes one TLS stack slot at every point of its run, and
// the stack check, which counts slots, sees what it saw before, also in an
// interrupt handler and in the paths that call another function without
// an alloca. A function without a TLS frame (js_c_function_data_call, and
// js_call_bound_function on 32-bit targets) mallocs the room and frees it
// on return, which adds no slot either.
//
// Each generated file must have exactly the four calls and their four
// registrations before, and none after. A file fixAlloca changed already
// has none and a marker in each of the four functions, and a function
// with a frame tests the size against allocaMax twice.
func fixAlloca(b []byte) ([]byte, error) {
	allocas := bytes.Count(b, []byte("libc.X__builtin_alloca("))
	regs := len(findAll(reAllocaRegister, b))
	if allocas == 0 && regs == 0 {
		for _, name := range allocaFuncs {
			body, err := funcBody(b, funcHead(name), name)
			if err != nil {
				return nil, err
			}
			if n := bytes.Count(body, []byte(allocaInFrame)) + bytes.Count(body, []byte(allocaInMalloc)); n != 1 {
				return nil, fmt.Errorf("%s: no alloca and %d markers of fixAlloca, want 1", name, n)
			}
			if bytes.Contains(body, []byte(allocaInFrame)) {
				if n := bytes.Count(body, []byte(allocaTest)); n != 2 {
					return nil, fmt.Errorf("%s: %d tests of the size of the alloca against %d, want 2", name, n, allocaMax)
				}
			}
		}
		return b, nil
	}
	if allocas != len(allocaFuncs) {
		return nil, fmt.Errorf("%d calls of alloca, want %d, one in each of %s", allocas, len(allocaFuncs), strings.Join(allocaFuncs, ", "))
	}
	for _, name := range allocaFuncs {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return nil, err
		}
		fixed, err := allocaFunc(name, body)
		if err != nil {
			return nil, err
		}
		b = bytes.Replace(b, body, fixed, 1)
	}
	if n := bytes.Count(b, []byte("libc.X__builtin_alloca(")); n != 0 {
		return nil, fmt.Errorf("%d calls of alloca outside %s", n, strings.Join(allocaFuncs, ", "))
	}
	if m := reAllocaRegister.Find(b); m != nil {
		return nil, fmt.Errorf("%s outside %s", m, strings.Join(allocaFuncs, ", "))
	}
	return b, nil
}

func funcHead(name string) *regexp.Regexp {
	return regexp.MustCompile(`\nfunc ` + regexp.QuoteMeta(name) + `\(tls \*libc\.TLS[,)][^\n]*\{\n`)
}

// parsedFunc is one function parsed on its own.
type parsedFunc struct {
	name string
	src  []byte // "package p\n" and the function
	fset *token.FileSet
	decl *ast.FuncDecl
}

const parsePrefix = "package p\n"

// parseFunc parses body, the text of one function as funcBody returns it.
func parseFunc(name string, body []byte) (*parsedFunc, error) {
	src := append([]byte(parsePrefix), bytes.TrimPrefix(body, []byte("\n"))...)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, err)
	}
	if len(file.Decls) != 1 {
		return nil, fmt.Errorf("%s: %d declarations", name, len(file.Decls))
	}
	fd, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok {
		return nil, fmt.Errorf("%s: not a function", name)
	}
	return &parsedFunc{name: name, src: src, fset: fset, decl: fd}, nil
}

func (f *parsedFunc) off(p token.Pos) int { return f.fset.Position(p).Offset }

func (f *parsedFunc) text(n ast.Node) string { return string(f.src[f.off(n.Pos()):f.off(n.End())]) }

// line returns the start and the end (after the newline) of the lines
// that n spans.
func (f *parsedFunc) line(n ast.Node) (int, int) {
	start := bytes.LastIndexByte(f.src[:f.off(n.Pos())], '\n') + 1
	end := bytes.IndexByte(f.src[f.off(n.End()):], '\n')
	return start, f.off(n.End()) + end + 1
}

// edit is a replacement of src[start:end].
type edit struct {
	start, end int
	text       string
}

// apply makes the edits, which must not overlap, and returns the function
// text in the form funcBody gives it.
func (f *parsedFunc) apply(edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out bytes.Buffer
	out.WriteByte('\n')
	last := len(parsePrefix)
	for _, e := range edits {
		out.Write(f.src[last:e.start])
		out.WriteString(e.text)
		last = e.end
	}
	out.Write(f.src[last:])
	return out.Bytes()
}

// tlsCall matches tls.Name(args) and returns the arguments.
func tlsCall(e ast.Expr, name string) ([]ast.Expr, bool) {
	c, ok := e.(*ast.CallExpr)
	if !ok || !isSelector(c.Fun, "tls", name) {
		return nil, false
	}
	return c.Args, true
}

// intLit returns the value of an integer literal.
func intLit(e ast.Expr) (int, bool) {
	l, ok := e.(*ast.BasicLit)
	if !ok || l.Kind != token.INT {
		return 0, false
	}
	n, err := strconv.Atoi(l.Value)
	return n, err == nil
}

// frameHead matches the first two statements of a function with a TLS
// frame, bp := tls.Alloc(N) and defer tls.Free(N), and returns N.
func frameHead(list []ast.Stmt) (int, bool) {
	if len(list) < 2 {
		return 0, false
	}
	a, ok := list[0].(*ast.AssignStmt)
	if !ok || a.Tok != token.DEFINE || len(a.Lhs) != 1 || len(a.Rhs) != 1 || !isIdent(a.Lhs[0], "bp") {
		return 0, false
	}
	args, ok := tlsCall(a.Rhs[0], "Alloc")
	if !ok || len(args) != 1 {
		return 0, false
	}
	n, ok := intLit(args[0])
	if !ok {
		return 0, false
	}
	d, ok := list[1].(*ast.DeferStmt)
	if !ok {
		return 0, false
	}
	args, ok = tlsCall(d.Call, "Free")
	if !ok || len(args) != 1 {
		return 0, false
	}
	if m, ok := intLit(args[0]); !ok || m != n {
		return 0, false
	}
	return n, true
}

// frameCell matches bp and bp + K and returns K.
func frameCell(e ast.Expr) (int, bool) {
	if isIdent(e, "bp") {
		return 0, true
	}
	be, ok := e.(*ast.BinaryExpr)
	if !ok || be.Op != token.ADD || !isIdent(be.X, "bp") {
		return 0, false
	}
	return intLit(be.Y)
}

// cellLoad matches *(*T)(unsafe.Pointer(C)) and **(**T)(__ccgo_up(C)), a
// load or store of a T in a frame cell, and returns C and T.
func cellLoad(e ast.Expr) (cell ast.Expr, typ ast.Expr, ok bool) {
	s, ok := e.(*ast.StarExpr)
	if !ok {
		return nil, nil, false
	}
	if s2, ok := s.X.(*ast.StarExpr); ok {
		// **(**T)(__ccgo_up(C))
		c, ok := s2.X.(*ast.CallExpr)
		if !ok || len(c.Args) != 1 {
			return nil, nil, false
		}
		typ, ok := ptrType(c.Fun, 2)
		if !ok {
			return nil, nil, false
		}
		inner, ok := c.Args[0].(*ast.CallExpr)
		if !ok || len(inner.Args) != 1 || !isIdent(inner.Fun, "__ccgo_up") {
			return nil, nil, false
		}
		return inner.Args[0], typ, true
	}
	c, ok := s.X.(*ast.CallExpr)
	if !ok || len(c.Args) != 1 {
		return nil, nil, false
	}
	typ, ok = ptrType(c.Fun, 1)
	if !ok {
		return nil, nil, false
	}
	inner, ok := c.Args[0].(*ast.CallExpr)
	if !ok || len(inner.Args) != 1 || !isSelector(inner.Fun, "unsafe", "Pointer") {
		return nil, nil, false
	}
	return inner.Args[0], typ, true
}

// paramCopy matches *(*T)(unsafe.Pointer(bp + K)) = param, the copy of a
// parameter into the frame, and returns K and the parameter.
func paramCopy(s ast.Stmt) (int, string, bool) {
	a, ok := s.(*ast.AssignStmt)
	if !ok || a.Tok != token.ASSIGN || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
		return 0, "", false
	}
	id, ok := a.Rhs[0].(*ast.Ident)
	if !ok {
		return 0, "", false
	}
	cell, _, ok := cellLoad(a.Lhs[0])
	if !ok {
		return 0, "", false
	}
	k, ok := frameCell(cell)
	return k, id.Name, ok
}

// isStackCheck reports whether s is if _js_check_stack_overflow(...) != 0 {.
func isStackCheck(s ast.Stmt) bool {
	is, ok := s.(*ast.IfStmt)
	if !ok || is.Init != nil {
		return false
	}
	be, ok := is.Cond.(*ast.BinaryExpr)
	if !ok || be.Op != token.NEQ {
		return false
	}
	c, ok := be.X.(*ast.CallExpr)
	return ok && isIdent(c.Fun, "_js_check_stack_overflow")
}

// allocaSite is the call of alloca in a function.
type allocaSite struct {
	stmt  *ast.AssignStmt // dst = libc.X__builtin_alloca(tls, size)
	dst   string
	size  ast.Expr
	top   int         // index of the top-level statement that holds it
	cond  *ast.IfStmt // the top-level if whose body holds it, or nil
	depth int         // how many blocks hold it
}

// findAlloca returns the one call of alloca in f, which must be a
// statement of the function body or of the body of an if in it.
func findAlloca(f *parsedFunc) (*allocaSite, error) {
	var site *allocaSite
	var err error
	list := f.decl.Body.List
	for i, s := range list {
		var stack []ast.Node
		ast.Inspect(s, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			c, ok := n.(*ast.CallExpr)
			if !ok || !isSelector(c.Fun, "libc", "X__builtin_alloca") {
				return true
			}
			defer func() { stack = stack[:len(stack)-1] }() // no f(nil) for a skipped node
			if site != nil {
				err = fmt.Errorf("%s: more than one call of alloca", f.name)
				return false
			}
			var a *ast.AssignStmt
			if len(stack) >= 2 {
				a, _ = stack[len(stack)-2].(*ast.AssignStmt)
			}
			if a == nil || a.Tok != token.ASSIGN || len(a.Lhs) != 1 || len(a.Rhs) != 1 || a.Rhs[0] != c || len(c.Args) != 2 || !isIdent(c.Args[0], "tls") {
				err = fmt.Errorf("%s: alloca is not assigned to a variable", f.name)
				return false
			}
			dst, ok := a.Lhs[0].(*ast.Ident)
			if !ok {
				err = fmt.Errorf("%s: alloca is not assigned to a variable", f.name)
				return false
			}
			site = &allocaSite{stmt: a, dst: dst.Name, size: c.Args[1], top: i}
			for _, p := range stack[:len(stack)-2] {
				switch p := p.(type) {
				case *ast.BlockStmt:
					site.depth++
				case *ast.IfStmt:
					if p == s && site.cond == nil {
						site.cond = p
					}
				case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.FuncLit:
					err = fmt.Errorf("%s: alloca inside a %T", f.name, p)
					return false
				}
			}
			return false
		})
		if err != nil {
			return nil, err
		}
	}
	if site == nil {
		return nil, fmt.Errorf("%s: no call of alloca", f.name)
	}
	switch {
	case site.depth == 0 && site.cond == nil:
	case site.depth == 1 && site.cond != nil && site.cond.Init == nil && site.cond.Body.List != nil && containsNode(site.cond.Body, site.stmt):
	default:
		return nil, fmt.Errorf("%s: alloca is neither a statement of the function nor of the body of an if in it", f.name)
	}
	return site, nil
}

func containsNode(block *ast.BlockStmt, n ast.Node) bool {
	for _, s := range block.List {
		if s == n {
			return true
		}
	}
	return false
}

// pureExpr reports whether e only reads local variables and constants, so
// that it has the same value wherever no variable of it changes, and adds
// the variables to vars.
func pureExpr(e ast.Expr, vars map[string]bool) bool {
	switch e := e.(type) {
	case *ast.Ident:
		vars[e.Name] = true
		return true
	case *ast.BasicLit:
		return true
	case *ast.ParenExpr:
		return pureExpr(e.X, vars)
	case *ast.BinaryExpr:
		return pureExpr(e.X, vars) && pureExpr(e.Y, vars)
	case *ast.UnaryExpr:
		return e.Op != token.AND && pureExpr(e.X, vars)
	case *ast.CallExpr:
		// Conversions to a basic type or a type of the engine, and the
		// helpers of libc that are conversions; no other call.
		switch fn := e.Fun.(type) {
		case *ast.Ident:
			if !reConvType.MatchString(fn.Name) {
				return false
			}
		case *ast.SelectorExpr:
			if !isIdent(fn.X, "libc") || !reConvHelper.MatchString(fn.Sel.Name) {
				return false
			}
		default:
			return false
		}
		for _, a := range e.Args {
			if !pureExpr(a, vars) {
				return false
			}
		}
		return true
	}
	return false
}

var (
	reConvType   = regexp.MustCompile(`^(u?int(8|16|32|64)?|uintptr|float(32|64)|bool|T[A-Za-z_]\w*)$`)
	reConvHelper = regexp.MustCompile(`^(Bool(Int|Uint)(8|16|32|64)|(Int|Uint)(8|16|32|64)From(Int|Uint|Float)(8|16|32|64))$`)
)

// assigned returns the variables that the statements between from and to
// (positions) assign or take the address of.
func assigned(body *ast.BlockStmt, from, to token.Pos) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil || n.End() <= from || n.Pos() >= to {
			return false
		}
		if n.Pos() < from {
			return true // holds the range
		}
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, l := range n.Lhs {
				if id, ok := l.(*ast.Ident); ok {
					out[id.Name] = true
				}
			}
		case *ast.IncDecStmt:
			if id, ok := n.X.(*ast.Ident); ok {
				out[id.Name] = true
			}
		case *ast.UnaryExpr:
			if id, ok := n.X.(*ast.Ident); ok && n.Op == token.AND {
				out[id.Name] = true
			}
		case *ast.ValueSpec:
			for _, id := range n.Names {
				out[id.Name] = true
			}
		}
		return true
	})
	return out
}

// labels returns the labels of the statements in [from, to) and the
// gotos outside it, by target.
func labels(body *ast.BlockStmt, from, to token.Pos) (inside map[string]bool, gotos map[string][]token.Pos) {
	inside, gotos = map[string]bool{}, map[string][]token.Pos{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.LabeledStmt:
			if n.Pos() >= from && n.Pos() < to {
				inside[n.Label.Name] = true
			}
		case *ast.BranchStmt:
			if n.Tok == token.GOTO && (n.Pos() < from || n.Pos() >= to) {
				gotos[n.Label.Name] = append(gotos[n.Label.Name], n.Pos())
			}
		}
		return true
	})
	return inside, gotos
}

func identUsed(body *ast.BlockStmt, name string) bool {
	used := false
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			used = true
		}
		return !used
	})
	return used
}

func allocaFunc(name string, body []byte) ([]byte, error) {
	f, err := parseFunc(name, body)
	if err != nil {
		return nil, err
	}
	list := f.decl.Body.List
	site, err := findAlloca(f)
	if err != nil {
		return nil, err
	}
	for _, v := range []string{allocaFrame, allocaMalloc} {
		if identUsed(f.decl.Body, v) {
			return nil, fmt.Errorf("%s: already has a variable %s", name, v)
		}
	}

	// The registration: defer tls.FreeAlloca()(), or tls.AllocaEntry()
	// followed by defer tls.AllocaExit(), among the statements of the
	// function before the alloca.
	var edits []edit
	regs := 0
	for i, s := range list[:site.top] {
		switch s := s.(type) {
		case *ast.DeferStmt:
			if c, ok := s.Call.Fun.(*ast.CallExpr); ok {
				if _, ok := tlsCall(c, "FreeAlloca"); ok && len(s.Call.Args) == 0 {
					start, end := f.line(s)
					edits = append(edits, edit{start, end, ""})
					regs++
				}
			}
		case *ast.ExprStmt:
			if _, ok := tlsCall(s.X, "AllocaEntry"); ok {
				d, ok := list[i+1].(*ast.DeferStmt)
				if ok {
					_, ok = tlsCall(d.Call, "AllocaExit")
				}
				if !ok {
					return nil, fmt.Errorf("%s: AllocaEntry not followed by defer AllocaExit", name)
				}
				start, _ := f.line(s)
				_, end := f.line(d)
				edits = append(edits, edit{start, end, ""})
				regs++
			}
		}
	}
	if regs != 1 {
		return nil, fmt.Errorf("%s: %d registrations of alloca before it, want 1", name, regs)
	}
	if n := len(findAll(reAllocaRegister, body)); n != 1 && n != 2 {
		return nil, fmt.Errorf("%s: registrations of alloca after it", name)
	}

	size := f.text(site.size)
	astart, aend := f.line(site.stmt)
	indent := string(f.src[astart:f.off(site.stmt.Pos())])

	n, framed := frameHead(list)
	if !framed {
		// No TLS frame: malloc the room and free it on return. No label
		// comes before, so the defer runs once.
		if in, _ := labels(f.decl.Body, f.decl.Body.Pos(), site.stmt.Pos()); len(in) != 0 {
			return nil, fmt.Errorf("%s: a label before the alloca", name)
		}
		edits = append(edits, edit{astart, aend, fmt.Sprintf("%s%s%s%s = libc.Xmalloc(tls, %s)\n%sdefer libc.Xfree(tls, %s)\n", indent, allocaInMalloc, indent, site.dst, size, indent, site.dst)})
		return f.apply(edits), nil
	}
	if n%8 != 0 {
		return nil, fmt.Errorf("%s: a frame of %d bytes, not a multiple of 8", name, n)
	}
	sizeVars := map[string]bool{}
	if !pureExpr(site.size, sizeVars) {
		return nil, fmt.Errorf("%s: the size of the alloca, %s, reads more than variables", name, size)
	}

	// The parameter copies right after the frame.
	params := map[int]string{}
	var copies []string
	for _, s := range list[2:] {
		k, p, ok := paramCopy(s)
		if !ok {
			break
		}
		params[k] = p
		copies = append(copies, f.text(s))
	}

	// The anchor: the stack check, the last statement of the function
	// before the alloca that is one.
	anchor := -1
	for i := 0; i < site.top; i++ {
		if isStackCheck(list[i]) {
			if anchor >= 0 {
				return nil, fmt.Errorf("%s: more than one stack check before the alloca", name)
			}
			anchor = i
		}
	}
	if anchor < 0 {
		return nil, fmt.Errorf("%s: no stack check before the alloca", name)
	}
	at := list[anchor]

	// From the anchor to the alloca: one way in, and nothing that changes
	// the size or the condition.
	var cond string
	condVars := map[string]bool{}
	if site.cond != nil {
		if !pureExpr(site.cond.Cond, condVars) {
			return nil, fmt.Errorf("%s: the condition of the alloca, %s, reads more than variables", name, f.text(site.cond.Cond))
		}
		cond = f.text(site.cond.Cond)
	}
	// No label up to the alloca either, so that a large one is malloc'd
	// once, into the one variable the deferred function frees.
	if in, _ := labels(f.decl.Body, at.Pos(), site.stmt.Pos()); len(in) != 0 {
		return nil, fmt.Errorf("%s: a label between the stack check and the alloca", name)
	}
	changed := assigned(f.decl.Body, at.Pos(), site.stmt.Pos())
	for v := range sizeVars {
		if changed[v] {
			return nil, fmt.Errorf("%s: %s changes between the stack check and the alloca", name, v)
		}
	}
	for v := range condVars {
		if changed[v] {
			return nil, fmt.Errorf("%s: %s changes between the stack check and the alloca", name, v)
		}
	}
	pre, gotos := labels(f.decl.Body, f.decl.Body.Pos(), at.Pos())
	for l, pos := range gotos {
		for _, p := range pos {
			if pre[l] && p > at.Pos() {
				return nil, fmt.Errorf("%s: a goto %s after the stack check goes back before it", name, l)
			}
		}
	}

	// The parameter copies are made again from the parameters, which must
	// still hold what they held at the entry.
	before := assigned(f.decl.Body, list[1].End(), at.Pos())
	for _, p := range params {
		if before[p] {
			return nil, fmt.Errorf("%s: parameter %s changes before the stack check", name, p)
		}
	}

	// Before the anchor, the frame holds the parameter copies only, and
	// the pointers into it that the function derives are derived again.
	w := &preWalk{f: f, params: params, before: pre}
	derived, err := w.top(list[2+len(copies) : anchor])
	if err != nil {
		return nil, err
	}

	var grow bytes.Buffer
	in := "\t\t"
	if cond != "" {
		if be, ok := site.cond.Cond.(*ast.BinaryExpr); ok && be.Op == token.LOR {
			cond = "(" + cond + ")"
		}
		fmt.Fprintf(&grow, "\tif %s && %s%s", cond, size, allocaTest)
	} else {
		fmt.Fprintf(&grow, "\tif %s%s", size, allocaTest)
	}
	fmt.Fprintf(&grow, "%stls.Free(%s)\n", in, allocaFrame)
	fmt.Fprintf(&grow, "%s%s = %d + int(%s)\n", in, allocaFrame, n, size)
	fmt.Fprintf(&grow, "%sbp = tls.Alloc(%s)\n", in, allocaFrame)
	for _, c := range copies {
		fmt.Fprintf(&grow, "%s%s\n", in, c)
	}
	for _, d := range derived {
		fmt.Fprintf(&grow, "%s%s\n", in, d)
	}
	grow.WriteString("\t}\n")
	hstart, _ := f.line(list[0])
	_, hend := f.line(list[1])
	ls, _ := f.line(at)
	edits = append(edits,
		edit{hstart, hend, fmt.Sprintf("\t%[1]s := %[3]d\n\tvar %[2]s uintptr\n\tbp := tls.Alloc(%[1]s)\n\tdefer func() {\n\t\tif %[2]s != 0 {\n\t\t\tlibc.Xfree(tls, %[2]s)\n\t\t}\n\t\ttls.Free(%[1]s)\n\t}()\n", allocaFrame, allocaMalloc, n)},
		edit{ls, ls, "\t" + allocaInFrame + grow.String()},
		edit{astart, aend, fmt.Sprintf("%[1]sif %[2]s%[3]s%[1]s\t%[4]s = bp + uintptr(%[5]d)\n%[1]s} else {\n%[1]s\t%[4]s = libc.Xmalloc(tls, %[2]s)\n%[1]s\t%[6]s = %[4]s\n%[1]s}\n",
			indent, size, allocaTest, site.dst, n, allocaMalloc)},
	)
	return f.apply(edits), nil
}

// preWalk checks the statements of a function before its stack check: the
// frame is allocated again there, so nothing may be stored in it and no
// pointer into it may live across that point, other than the parameter
// copies and the pointers derived at the top level, which are made again.
type preWalk struct {
	f       *parsedFunc
	params  map[int]string  // offset -> parameter of the cells that hold a parameter copy
	derived []string        // the statements that derive a pointer into the frame
	before  map[string]bool // the labels before the stack check
}

// top checks the top-level statements list and returns the statements
// that derive a pointer into the frame, in order.
func (w *preWalk) top(list []ast.Stmt) ([]string, error) {
	live := map[string]bool{}
	for _, s := range list {
		if l, ok := s.(*ast.LabeledStmt); ok {
			s = l.Stmt
		}
		if a, ok := s.(*ast.AssignStmt); ok && a.Tok == token.ASSIGN && len(a.Lhs) == 1 && len(a.Rhs) == 1 {
			if id, ok := a.Lhs[0].(*ast.Ident); ok {
				if _, ok := frameCell(a.Rhs[0]); ok {
					live[id.Name] = true
					w.derived = append(w.derived, w.f.text(a))
					continue
				}
				if live[id.Name] {
					return nil, fmt.Errorf("%s: %s, a pointer into the frame, changes before the stack check", w.f.name, id.Name)
				}
			}
		}
		if err := w.stmt(s, live); err != nil {
			return nil, err
		}
	}
	return w.derived, nil
}

// stmt checks a statement nested below the top level, where an
// assignment that does not derive from the frame ends a derived pointer
// for the rest of its block. Such a block must end in a return or in a
// goto past the stack check: the pointer is derived again before the
// stack check, which would undo the assignment on a way that leaves the
// block at its end or goes to a label before the check.
func (w *preWalk) stmt(s ast.Stmt, live map[string]bool) error {
	switch s := s.(type) {
	case *ast.BlockStmt:
		inner := map[string]bool{}
		for k, v := range live {
			inner[k] = v
		}
		if killed := w.kills(s, live); killed != "" && !w.leaves(s) {
			return fmt.Errorf("%s: %s, a pointer into the frame, changes in a block that does not end in a return or a goto past the stack check", w.f.name, killed)
		}
		for _, s := range s.List {
			if a, ok := s.(*ast.AssignStmt); ok && a.Tok == token.ASSIGN && len(a.Lhs) == 1 && len(a.Rhs) == 1 {
				if id, ok := a.Lhs[0].(*ast.Ident); ok && inner[id.Name] {
					if err := w.expr(a.Rhs[0], inner); err != nil {
						return err
					}
					if _, ok := frameCell(a.Rhs[0]); ok {
						return fmt.Errorf("%s: %s derived from the frame in a block before the stack check", w.f.name, id.Name)
					}
					delete(inner, id.Name)
					continue
				}
			}
			if err := w.stmt(s, inner); err != nil {
				return err
			}
		}
		return nil
	case *ast.IfStmt:
		if s.Init != nil {
			if err := w.stmt(s.Init, live); err != nil {
				return err
			}
		}
		if err := w.expr(s.Cond, live); err != nil {
			return err
		}
		if err := w.stmt(s.Body, live); err != nil {
			return err
		}
		if s.Else != nil {
			return w.stmt(s.Else, live)
		}
		return nil
	case *ast.LabeledStmt:
		return w.stmt(s.Stmt, live)
	}
	return w.expr(s, live)
}

// kills returns a live pointer into the frame that a statement of block
// assigns, or "".
func (w *preWalk) kills(block *ast.BlockStmt, live map[string]bool) string {
	for _, s := range block.List {
		if a, ok := s.(*ast.AssignStmt); ok && a.Tok == token.ASSIGN && len(a.Lhs) == 1 && len(a.Rhs) == 1 {
			if id, ok := a.Lhs[0].(*ast.Ident); ok && live[id.Name] {
				return id.Name
			}
		}
	}
	return ""
}

// leaves reports whether block ends in a return or a goto to a label
// after the stack check, or in an if and else that both do.
func (w *preWalk) leaves(block *ast.BlockStmt) bool {
	if len(block.List) == 0 {
		return false
	}
	return w.terminates(block.List[len(block.List)-1])
}

func (w *preWalk) terminates(s ast.Stmt) bool {
	switch s := s.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return s.Tok == token.GOTO && s.Label != nil && !w.before[s.Label.Name]
	case *ast.BlockStmt:
		return w.leaves(s)
	case *ast.IfStmt:
		return s.Else != nil && w.leaves(s.Body) && w.terminates(s.Else)
	case *ast.LabeledStmt:
		return w.terminates(s.Stmt)
	}
	return false
}

// expr checks that n mentions the frame only to load a parameter copy and
// no live pointer into it.
func (w *preWalk) expr(n ast.Node, live map[string]bool) error {
	var err error
	var stack []ast.Node
	ast.Inspect(n, func(n ast.Node) bool {
		if err != nil {
			return false
		}
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch x := n.(type) {
		case *ast.StarExpr:
			cell, _, ok := cellLoad(x)
			if !ok {
				return true
			}
			if k, ok := frameCell(cell); ok {
				if _, ok := w.params[k]; !ok {
					err = fmt.Errorf("%s: frame cell bp+%d used before the stack check", w.f.name, k)
					return false
				}
				if len(stack) >= 2 {
					switch p := stack[len(stack)-2].(type) {
					case *ast.AssignStmt:
						for _, l := range p.Lhs {
							if l == x {
								err = fmt.Errorf("%s: frame cell bp+%d written before the stack check", w.f.name, k)
							}
						}
					case *ast.UnaryExpr:
						if p.Op == token.AND {
							err = fmt.Errorf("%s: the address of frame cell bp+%d taken before the stack check", w.f.name, k)
						}
					}
				}
				stack = stack[:len(stack)-1]
				return false
			}
		case *ast.Ident:
			if x.Name == "bp" {
				err = fmt.Errorf("%s: the frame used before the stack check: %s", w.f.name, w.f.text(stack[0]))
			} else if live[x.Name] {
				err = fmt.Errorf("%s: %s, a pointer into the frame, used before the stack check", w.f.name, x.Name)
			}
		}
		return true
	})
	return err
}
