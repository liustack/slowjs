// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

var (
	reSlotFind   = regexp.MustCompile(`\b_find_own_property1?\(`)
	reFuncName   = regexp.MustCompile(`^func (\w+)`)
	reDescLookup = regexp.MustCompile(`\b(?:_JS_GetOwnPropertyInternal|XJS_GetOwnProperty|_js_backtrace_own_flags)\(`)
)

// slotSite is the expected use of property slots by one function and why
// that use is safe for a pending stack.
type slotSite struct {
	uses, why string
}

// auditSlots checks b against slotSites: every function that uses a
// property slot, or looks up a property descriptor, must be listed with
// exactly the uses it has.
func auditSlots(b []byte) error {
	got, err := auditSlotUses(b)
	if err != nil {
		return err
	}
	var problems []string
	for name, uses := range got {
		want, ok := slotSites[name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s uses property slots and is not listed: %s", name, uses))
		case want.uses != uses:
			problems = append(problems, fmt.Sprintf("%s: uses changed\n\t\tnow:  %s\n\t\tlist: %s", name, uses, want.uses))
		}
	}
	for name := range slotSites {
		if _, ok := got[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s is listed but uses no property slots", name))
		}
	}
	if len(problems) != 0 {
		sort.Strings(problems)
		return fmt.Errorf("property slot uses changed, review each for the pending stack and update slotSites (go run ./internal/fixlib -slots):\n\t%s", strings.Join(problems, "\n\t"))
	}
	return nil
}

// printSlots prints the slot uses of the generated file fn, with the fork's
// changes made, as slotSites entries, keeping the reasons already listed.
func printSlots(fn string) error {
	b, err := readEngine(fn)
	if err != nil {
		return err
	}
	if b, err = fixToCString(b); err != nil {
		return err
	}
	if b, err = fixBacktrace(b); err != nil {
		return err
	}
	got, err := auditSlotUses(b)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(got))
	for name := range got {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		why := slotSites[name].why
		if why == "" {
			why = "REVIEW"
		}
		fmt.Printf("\t%q: {%q, %q},\n", name, got[name], why)
	}
	return nil
}

// The slot audit follows pointers to property slots (JSProperty) through
// the generated engine and lists, for every function they reach, each way
// the function uses one. A pending stack is a marker object in the value of
// a slot, so every place that reads a slot's value must be one that formats
// the stack first, and a new or changed use is something to review.
//
// A pointer is a slot pointer when it comes from the prop array of an
// object, from the slot find_own_property stores through its out
// parameter, from a function that returns one, or from a parameter that a
// caller passes one in. It is followed through local variables, stack
// cells (bp+N), pointer arithmetic and call arguments, whatever the names
// are. A slot pointer stored into anything else, or used in a way the audit
// does not know, shows up as a use of its own, so it changes the list too.

// slotKind is what a tracked pointer points to.
type slotKind int

const (
	kindNone slotKind = iota
	kindSlot          // a JSProperty
	kindCell          // a cell that holds a JSProperty pointer
)

// slotFunc is one generated function.
type slotFunc struct {
	name   string
	text   []byte
	decl   *ast.FuncDecl
	fset   *token.FileSet
	params []string
	in     map[int]slotKind // parameter index (after tls) -> kind
	ret    bool             // returns a slot pointer
	uses   map[string]int
}

// slotFuncs splits b into its functions.
func slotFuncs(b []byte) map[string]*slotFunc {
	fns := map[string]*slotFunc{}
	parts := bytes.Split(b, []byte("\nfunc "))
	for _, part := range parts[1:] {
		// The separator took the newline that ends the previous part.
		text := append(append([]byte("func "), part...), '\n')
		if end := bytes.Index(text, []byte("\n}\n")); end >= 0 {
			text = text[:end+2]
		}
		m := reFuncName.FindSubmatch(text)
		if m == nil {
			continue
		}
		fns[string(m[1])] = &slotFunc{name: string(m[1]), text: text, in: map[int]slotKind{}}
	}
	return fns
}

func (f *slotFunc) parse() error {
	if f.decl != nil {
		return nil
	}
	f.fset = token.NewFileSet()
	file, err := parser.ParseFile(f.fset, "", append([]byte("package p\n"), f.text...), parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("%s: %v", f.name, err)
	}
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			f.decl = fd
		}
	}
	if f.decl == nil {
		return fmt.Errorf("%s: no function", f.name)
	}
	for _, field := range f.decl.Type.Params.List {
		for _, n := range field.Names {
			f.params = append(f.params, n.Name)
		}
	}
	return nil
}

// slotSources are the functions that search the prop array. They are not
// audited as users. find_own_property stores the slot it finds through its
// first argument after tls, find_own_property1 returns only the shape
// entry.
var slotSources = map[string]bool{
	"_find_own_property":  true,
	"_find_own_property1": true,
}

// slotBlocks are the allocator functions that get the prop array as a
// block of memory to free or resize. A pointer passed to one is a use of
// its own, but the audit does not follow it inside.
var slotBlocks = map[string]bool{
	"Xjs_free":       true,
	"Xjs_free_rt":    true,
	"Xjs_realloc":    true,
	"Xjs_realloc_rt": true,
	"Xjs_realloc2":   true,
}

// auditSlotUses follows slot pointers through b and returns the uses of
// every function they reach, as "use=count" lists keyed by function name.
func auditSlotUses(b []byte) (map[string]string, error) {
	fns := slotFuncs(b)
	queue := []string{}
	queued := map[string]bool{}
	push := func(name string) {
		if !queued[name] {
			queued[name] = true
			queue = append(queue, name)
		}
	}
	for name, f := range fns {
		if slotSources[name] {
			continue
		}
		if bytes.Contains(f.text, []byte(".Fprop")) || reSlotFind.Match(f.text) {
			push(name)
		}
	}
	for len(queue) != 0 {
		name := queue[0]
		queue = queue[1:]
		queued[name] = false
		f := fns[name]
		if err := f.parse(); err != nil {
			return nil, err
		}
		a := &slotWalk{f: f, fns: fns}
		if err := a.run(); err != nil {
			return nil, err
		}
		f.uses = a.uses
		if a.ret && !f.ret && !slotBlocks[name] {
			// Every caller now gets a slot pointer from a call of name.
			f.ret = true
			call := []byte(name + "(tls")
			for cname, c := range fns {
				if !slotSources[cname] && bytes.Contains(c.text, call) {
					push(cname)
				}
			}
		}
		for callee, args := range a.calls {
			g := fns[callee]
			if g == nil || slotSources[callee] {
				continue
			}
			changed := false
			for i, k := range args {
				if g.in[i] < k {
					g.in[i] = k
					changed = true
				}
			}
			if changed || g.uses == nil {
				push(callee)
			}
		}
	}
	out := map[string]string{}
	for name, f := range fns {
		if n := len(findAll(reDescLookup, f.text)); n != 0 {
			if f.uses == nil {
				f.uses = map[string]int{}
			}
			f.uses["descriptor lookup"] = n
		}
		if len(f.uses) == 0 || strings.HasPrefix(name, "_js_backtrace_") {
			continue
		}
		keys := make([]string, 0, len(f.uses))
		for k := range f.uses {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var s []string
		for _, k := range keys {
			s = append(s, fmt.Sprintf("%s=%d", k, f.uses[k]))
		}
		out[name] = strings.Join(s, " ")
	}
	return out, nil
}

// slotWalk analyses one function.
type slotWalk struct {
	f     *slotFunc
	fns   map[string]*slotFunc
	vars  map[string]slotKind // local variables and parameters
	cells map[string]bool     // stack cells (by expression text) holding a slot pointer
	uses  map[string]int
	calls map[string]map[int]slotKind
	ret   bool
	guard token.Pos // the first call of a _js_backtrace_ guard
}

func (a *slotWalk) run() error {
	a.vars = map[string]slotKind{}
	a.cells = map[string]bool{}
	for i, k := range a.f.in {
		if i+1 < len(a.f.params) {
			a.vars[a.f.params[i+1]] = k
		}
	}
	// Propagate kinds to a fixed point: the analysis does not follow the
	// order of statements, so a variable is a slot pointer if any
	// assignment makes it one.
	for {
		before := a.state()
		ast.Inspect(a.f.decl.Body, a.propagate)
		if a.state() == before {
			break
		}
	}
	a.guard = token.NoPos
	ast.Inspect(a.f.decl.Body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && a.guard == token.NoPos {
			if id, ok := c.Fun.(*ast.Ident); ok && strings.HasPrefix(id.Name, "_js_backtrace_") {
				for _, arg := range c.Args {
					if a.kind(arg) != kindNone {
						a.guard = c.Pos()
					}
				}
			}
		}
		return true
	})
	a.uses = map[string]int{}
	a.calls = map[string]map[int]slotKind{}
	var stack []ast.Node
	ast.Inspect(a.f.decl.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		if e, ok := n.(ast.Expr); ok {
			if k := a.kind(e); k != kindNone && !a.propagates(stack) {
				a.use(e, k, stack)
				// Inspect does not call f(nil) for a node whose children
				// it skips.
				stack = stack[:len(stack)-1]
				return false
			}
		}
		return true
	})
	return nil
}

func (a *slotWalk) state() string {
	var s []string
	for k, v := range a.vars {
		s = append(s, fmt.Sprintf("v%s%d", k, v))
	}
	for k := range a.cells {
		s = append(s, "c"+k)
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

func (a *slotWalk) text(e ast.Node) string {
	var buf bytes.Buffer
	printer.Fprint(&buf, a.f.fset, e)
	return strings.Join(strings.Fields(buf.String()), "")
}

// propagate records the variables and cells that receive a slot pointer.
func (a *slotWalk) propagate(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.AssignStmt:
		a.bind(n.Lhs, n.Rhs)
	case *ast.ValueSpec:
		// var x = v and var x T = v bind like x := v.
		lhs := make([]ast.Expr, len(n.Names))
		for i, id := range n.Names {
			lhs[i] = id
		}
		a.bind(lhs, n.Values)
	case *ast.CallExpr:
		if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "_find_own_property" && len(n.Args) > 1 {
			a.cells[a.text(n.Args[1])] = true
		}
	}
	return true
}

// bind records the variables and cells that lhs = rhs makes slot or cell
// pointers. An assignment of several results of one call binds nothing:
// use reports a slot pointer there.
func (a *slotWalk) bind(lhs, rhs []ast.Expr) {
	if len(lhs) != len(rhs) {
		return
	}
	for i, r := range rhs {
		k := a.kind(r)
		if k == kindNone {
			continue
		}
		switch l := lhs[i].(type) {
		case *ast.Ident:
			if a.vars[l.Name] < k {
				a.vars[l.Name] = k
			}
		default:
			if addr, typ, ok := a.load(l); ok && typ == "uintptr" && k == kindSlot && a.kind(addr) == kindNone {
				a.cells[a.text(addr)] = true
			}
		}
	}
}

// kind tells whether e is a slot pointer or a cell pointer.
func (a *slotWalk) kind(e ast.Expr) slotKind {
	switch e := e.(type) {
	case *ast.Ident:
		if k := a.vars[e.Name]; k != kindNone {
			return k
		}
	case *ast.ParenExpr:
		return a.kind(e.X)
	case *ast.BinaryExpr:
		// Arithmetic on a slot pointer moves along the prop array. A cell
		// is one stack cell, so bp+8 next to a cell at bp is not one.
		if e.Op == token.ADD || e.Op == token.SUB {
			if a.kind(e.X) == kindSlot {
				return kindSlot
			}
			if e.Op == token.ADD && a.kind(e.Y) == kindSlot {
				return kindSlot
			}
		}
	case *ast.SelectorExpr:
		if e.Sel.Name == "Fprop" && a.objectField(e.X) {
			return kindSlot
		}
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok {
			if id.Name == "uintptr" && len(e.Args) == 1 {
				return a.kind(e.Args[0])
			}
			if g := a.fns[id.Name]; g != nil && g.ret && !slotBlocks[id.Name] {
				return kindSlot
			}
		}
	}
	if addr, typ, ok := a.load(e); ok && typ == "uintptr" && a.kind(addr) == kindCell {
		return kindSlot
	}
	if a.cells[a.text(e)] {
		return kindCell
	}
	return kindNone
}

// objectField reports whether x is (*TJSObject)(unsafe.Pointer(...)).
func (a *slotWalk) objectField(x ast.Expr) bool {
	_, typ, ok := a.convert(x)
	return ok && typ == "TJSObject"
}

// convert matches (*T)(unsafe.Pointer(p)) and *(**T)(__ccgo_up(p)), which
// are p seen as a *T, and returns p and T.
func (a *slotWalk) convert(e ast.Expr) (ast.Expr, string, bool) {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = p.X
	}
	if s, ok := e.(*ast.StarExpr); ok {
		// *(**T)(__ccgo_up(p))
		c, ok := s.X.(*ast.CallExpr)
		if !ok || len(c.Args) != 1 {
			return nil, "", false
		}
		typ, ok := ptrType(c.Fun, 2)
		if !ok {
			return nil, "", false
		}
		inner, ok := c.Args[0].(*ast.CallExpr)
		if !ok || len(inner.Args) != 1 || !isIdent(inner.Fun, "__ccgo_up") {
			return nil, "", false
		}
		return inner.Args[0], a.text(typ), true
	}
	c, ok := e.(*ast.CallExpr)
	if !ok || len(c.Args) != 1 {
		return nil, "", false
	}
	typ, ok := ptrType(c.Fun, 1)
	if !ok {
		return nil, "", false
	}
	inner, ok := c.Args[0].(*ast.CallExpr)
	if !ok || len(inner.Args) != 1 || !isSelector(inner.Fun, "unsafe", "Pointer") {
		return nil, "", false
	}
	return inner.Args[0], a.text(typ), true
}

// load matches *(*T)(unsafe.Pointer(p)) and **(**T)(__ccgo_up(p)), a load
// of a T at p, and returns p and T.
func (a *slotWalk) load(e ast.Expr) (ast.Expr, string, bool) {
	s, ok := e.(*ast.StarExpr)
	if !ok {
		return nil, "", false
	}
	return a.convert(s.X)
}

// ptrType matches (*T) with n stars and returns T.
func ptrType(e ast.Expr, n int) (ast.Expr, bool) {
	p, ok := e.(*ast.ParenExpr)
	if !ok {
		return nil, false
	}
	e = p.X
	for i := 0; i < n; i++ {
		s, ok := e.(*ast.StarExpr)
		if !ok {
			return nil, false
		}
		e = s.X
	}
	return e, true
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isSelector(e ast.Expr, x, sel string) bool {
	s, ok := e.(*ast.SelectorExpr)
	return ok && isIdent(s.X, x) && s.Sel.Name == sel
}

// propagates reports whether the parent of the top of stack is itself a
// slot or cell expression made from it (arithmetic, parentheses, a
// conversion), so that only the outermost one counts as a use.
func (a *slotWalk) propagates(stack []ast.Node) bool {
	if len(stack) < 2 {
		return false
	}
	if p, ok := stack[len(stack)-2].(ast.Expr); ok {
		return a.kind(p) != kindNone
	}
	return false
}

// use records how the slot or cell pointer e is used.
func (a *slotWalk) use(e ast.Expr, k slotKind, stack []ast.Node) {
	name := "slot"
	if k == kindCell {
		name = "cell"
	}
	if k == kindSlot && a.guard != token.NoPos && e.Pos() < a.guard {
		name = "early " + name
	}
	parent := stack[len(stack)-2]
	switch p := parent.(type) {
	case *ast.CallExpr:
		if isSelector(p.Fun, "unsafe", "Pointer") || isIdent(p.Fun, "__ccgo_up") {
			a.access(e, name, stack)
			return
		}
		fn := a.text(p.Fun)
		if id, ok := p.Fun.(*ast.Ident); ok {
			if strings.HasPrefix(id.Name, "_js_backtrace_") {
				a.uses[fmt.Sprintf("%s guard %s", name, id.Name)]++
				return
			}
			if slotSources[id.Name] {
				a.uses[fmt.Sprintf("%s find", name)]++
				return
			}
			if g := a.fns[id.Name]; g != nil && !slotBlocks[id.Name] {
				for i, arg := range p.Args {
					if arg == e {
						if a.calls[id.Name] == nil {
							a.calls[id.Name] = map[int]slotKind{}
						}
						if a.calls[id.Name][i-1] < k {
							a.calls[id.Name][i-1] = k
						}
					}
				}
			}
		}
		a.uses[fmt.Sprintf("%s passed to %s", name, fn)]++
	case *ast.BinaryExpr:
		if k == kindCell && (p.Op == token.ADD || p.Op == token.SUB) {
			// bp+8 next to a cell at bp is another stack cell. Any other
			// arithmetic on a cell is reported.
			if l, ok := p.Y.(*ast.BasicLit); ok && p.X == e && l.Kind == token.INT && l.Value != "0" {
				return
			}
			a.uses[fmt.Sprintf("%s offset", name)]++
			return
		}
		a.uses[fmt.Sprintf("%s compared", name)]++
	case *ast.AssignStmt:
		for i, rhs := range p.Rhs {
			if rhs == e && len(p.Lhs) != len(p.Rhs) {
				a.uses[fmt.Sprintf("%s in an assignment of several results", name)]++
				return
			}
			if rhs == e {
				if _, ok := p.Lhs[i].(*ast.Ident); ok {
					return // followed by propagate
				}
				if addr, typ, ok := a.load(p.Lhs[i]); ok && typ == "uintptr" && k == kindSlot && a.kind(addr) == kindCell {
					return // stored in a tracked cell
				}
				a.uses[fmt.Sprintf("%s stored in %s", name, a.text(p.Lhs[i]))]++
				return
			}
		}
		// e is assigned to: not a use.
	case *ast.ValueSpec:
		for _, v := range p.Values {
			if v == e {
				if len(p.Names) != len(p.Values) {
					a.uses[fmt.Sprintf("%s in a declaration of several results", name)]++
				}
				return // followed by propagate
			}
		}
		// e is declared: not a use.
	case *ast.IncDecStmt:
		// pointer arithmetic on a variable: not a use.
	case *ast.ReturnStmt:
		if k == kindSlot {
			a.ret = true
		}
		a.uses[fmt.Sprintf("%s returned", name)]++
	case *ast.IfStmt, *ast.ExprStmt:
		a.uses[fmt.Sprintf("%s tested", name)]++
	default:
		a.uses[fmt.Sprintf("%s in %T", name, parent)]++
	}
}

// access records a load or store through the slot or cell pointer e,
// which stack shows inside unsafe.Pointer(e) or __ccgo_up(e).
func (a *slotWalk) access(e ast.Expr, name string, stack []ast.Node) {
	// Climb to the conversion, then through the derefs and fields that
	// make up the access.
	i := len(stack) - 2 // unsafe.Pointer(e) or __ccgo_up(e)
	conv := stack[i-1]
	_, typ, ok := a.convert(conv.(ast.Expr))
	j := i - 1
	if !ok {
		// __ccgo_up(e) under a single star is *(**T)(...)
		if j >= 1 {
			if _, typ2, ok2 := a.convert(stack[j-1].(ast.Expr)); ok2 {
				typ, ok = typ2, true
				j--
			}
		}
	}
	if !ok {
		a.uses[fmt.Sprintf("%s converted by %s", name, a.text(conv))]++
		return
	}
	path := typ
	top := stack[j]
	for j-1 >= 0 {
		up := stack[j-1]
		if pe, ok := up.(*ast.ParenExpr); ok {
			top = pe
			j--
			continue
		}
		if s, ok := up.(*ast.StarExpr); ok && s.X == top {
			path = "*" + path
			top = s
			j--
			continue
		}
		if s, ok := up.(*ast.SelectorExpr); ok && s.X == top {
			path += "." + s.Sel.Name
			top = s
			j--
			continue
		}
		break
	}
	op := "read"
	if j-1 >= 0 {
		switch p := stack[j-1].(type) {
		case *ast.AssignStmt:
			for _, l := range p.Lhs {
				if l == top {
					op = "write"
					if p.Tok != token.ASSIGN {
						op = "update"
					}
				}
			}
		case *ast.IncDecStmt:
			op = "update"
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				op = "address"
			}
		}
	}
	a.uses[fmt.Sprintf("%s %s %s", name, op, valuePath(path))]++
}

// valuePath names every access to the value of a slot, or to a part of it
// (its tag, or the 64 bits of a NaN-boxed value on 32-bit targets), the
// same way, so that the uses are the same on every target.
func valuePath(path string) string {
	switch {
	case strings.HasPrefix(path, "*TJSValue"), strings.HasPrefix(path, "TJSValue."), path == "*Tuint64_t", path == "*float64":
		return "value"
	}
	return path
}
