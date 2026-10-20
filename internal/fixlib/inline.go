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
	"strings"
)

// fixInline writes out the calls of set_value, the callee of
// inlineCallees, in JS_CallInternal. Go inlines into a function as big as
// the interpreter only callees that cost 20 or less, with or without PGO,
// so each of these calls is a real call there, made for every put_loc,
// set_loc, put_arg and put_var_ref of a script. A call, as a statement:
//
//	_set_value(tls, ctx10, var_buf+uintptr(idx3)*16, v121)
//
// becomes a block that gives each argument to a variable of the
// parameter's type, in order, then runs the body of the callee with its
// parameters renamed to those variables and its labels renamed to labels
// of their own. An argument that is a variable of the interpreter is read
// where the parameter is instead, and the first line names it, when
// nothing can set the variable while the copy runs (see inlineVars) and
// the callee neither sets the parameter nor takes its address nor has a
// local of that name:
//
//	{
//		// Changed by internal/fixlib: _set_value written out (see fixInline), with ctx10 as inl1, v121 as inl3.
//		var inl2 uintptr = var_buf + uintptr(idx3)*16
//		BODY OF set_value
//	}
//
// A copy of each such variable would be one more value for the compiler
// to keep in a register or spill, and on amd64 it undoes fixPin, which
// keeps ctx10 and var_buf in their stack slots: the frame of the
// interpreter there grew by 288 bytes with the copies.
//
// The locals of the callee are declared in the block, after the
// arguments, so they cannot change what an argument reads. The TLS frame
// that fixFrames gives set_value holds one JSValue, and becomes a Go
// JSValue in each copy (see inlineFrameUse). JS_FreeAtom and
// is_strict_mode were written out too, and gained nothing that could be
// told from noise, so they stay calls.
//
// fixInline stops when a callee has a result, returns early or takes a
// TLS frame, when a name the callee uses is a local of JS_CallInternal (it
// would read the local), when a call is not a statement of its own, or
// when the number of calls is not that of inlineCallees. The text of the
// callees is pinned in inlineSites, so a change upstream stops fixlib
// until it is read again. Other changes match JS_CallInternal as ccgo writes it, so
// fixInline comes last, before fixNoDefer, and restoreInline, its exact
// inverse, comes first. fixInline checks that restoreInline gives its
// input back.
func fixInline(b []byte, name string) ([]byte, error) {
	got, err := inlineFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := inlineSites[name]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against fixInline, then update inlineSites (go run ./internal/fixlib -inline)", name, strings.Join(inlineNames(), ", "), got, want)
	}
	out, err := inlineCalls(b, inlineCallees)
	if err != nil {
		return nil, err
	}
	back, err := restoreInline(out)
	if err != nil {
		return nil, fmt.Errorf("fixInline: %v", err)
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixInline: restoreInline does not give the input back")
	}
	return out, nil
}

// inlineFunc is the function whose calls fixInline writes out.
const inlineFunc = "_JS_CallInternal"

// inlineCallees are the callees fixInline writes out in inlineFunc, with
// the number of their calls there.
var inlineCallees = map[string]int{
	"_set_value": 46,
}

// inlineMark is the first line of the block a call of name becomes. with
// names the variables read in place of parameters, as "ctx10 as inl1".
func inlineMark(name string, with ...string) string {
	if len(with) == 0 {
		return "// Changed by internal/fixlib: " + name + " written out (see fixInline).\n"
	}
	return "// Changed by internal/fixlib: " + name + " written out (see fixInline), with " + strings.Join(with, ", ") + ".\n"
}

var (
	reInlineBlock = regexp.MustCompile(`^\{\n\t+// Changed by internal/fixlib: (\w+) written out \(see fixInline\)(?:, with (\w+ as inl\d+(?:, \w+ as inl\d+)*))?\.\n`)
	reInlineWith  = regexp.MustCompile(`(\w+) as inl(\d+)`)
	reInlineName  = regexp.MustCompile(`^inl\d`)
)

func inlineNames() []string {
	names := make([]string, 0, len(inlineCallees))
	for name := range inlineCallees {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// inlineFingerprint hashes the callees of inlineCallees.
func inlineFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, inlineNames(), func(text []byte) []byte { return text })
}

// inlineCallee is a callee of inlineCallees, ready to be written out.
type inlineCallee struct {
	name   string
	params []string // the names after tls
	types  []string // their types
	free   map[string]bool
	// The names the callee declares, and whether it sets each parameter
	// or takes its address.
	locals  map[string]bool
	written []bool
	// Its body, from the line after its opening brace to the line before
	// its closing one, and the offsets of its parameters and labels there.
	body   string
	idents []inlineIdent
}

// inlineIdent is a parameter (param >= 0) or a label of a callee, or
// text that its copies have in place of the callee's (text != "").
type inlineIdent struct {
	start, end int
	param      int
	label      string
	text       string
}

// inlineCell is the Go variable that the TLS frame of a callee becomes in
// its copies, when the frame holds one JSValue (see inlineFrameUse).
const inlineCell = "cell"

// newInlineCallee reads the callee name, whose text is body. cells says
// whether a JSValue is 8 bytes of a union then a tag of 8 bytes.
func newInlineCallee(name string, body []byte, cells bool) (*inlineCallee, error) {
	f, err := parseFunc(name, body)
	if err != nil {
		return nil, err
	}
	c := &inlineCallee{name: name, free: map[string]bool{}}
	ps := f.decl.Type.Params.List
	if len(ps) == 0 || len(ps[0].Names) != 1 || ps[0].Names[0].Name != "tls" || f.text(ps[0].Type) != "*libc.TLS" {
		return nil, fmt.Errorf("%s: the first parameter is not tls *libc.TLS", name)
	}
	param := map[string]int{}
	for _, p := range ps[1:] {
		for _, id := range p.Names {
			param[id.Name] = len(c.params)
			c.params = append(c.params, id.Name)
			c.types = append(c.types, f.text(p.Type))
		}
	}
	locals := map[string]bool{}
	labels := map[string]bool{}
	c.locals = locals
	c.written = make([]bool, len(c.params))
	set := func(e ast.Expr) {
		if id, ok := ast.Unparen(e).(*ast.Ident); ok {
			if i, ok := param[id.Name]; ok {
				c.written[i] = true
			}
		}
	}
	var problem string
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.UnaryExpr:
			if x.Op == token.AND {
				set(x.X)
			}
		case *ast.IncDecStmt:
			set(x.X)
		case *ast.ValueSpec:
			for _, id := range x.Names {
				locals[id.Name] = true
			}
		case *ast.AssignStmt:
			for _, l := range x.Lhs {
				if x.Tok == token.DEFINE {
					locals[l.(*ast.Ident).Name] = true
				}
				set(l)
			}
		case *ast.LabeledStmt:
			labels[x.Label.Name] = true
		case *ast.DeferStmt, *ast.GoStmt, *ast.FuncLit:
			problem = fmt.Sprintf("%s: a %T", name, x)
		case *ast.CallExpr:
			if _, ok := tlsCall(x, "Alloc"); ok {
				problem = name + ": a TLS frame"
			}
		}
		return problem == ""
	})
	if problem != "" {
		return nil, fmt.Errorf("%s", problem)
	}
	if res := f.decl.Type.Results; res != nil && len(res.List) != 0 {
		return nil, fmt.Errorf("%s: a result", name)
	}
	// The identifiers of the body: parameters and labels to rename, and
	// the names it takes from the package.
	start, end := f.off(f.decl.Body.Lbrace)+2, f.off(f.decl.Body.Rbrace)
	c.body = string(f.src[start:end])
	var stack []ast.Node
	frame := false
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch x := n.(type) {
		case *ast.ReturnStmt:
			problem = name + ": a return"
		case *ast.Ident:
			if reInlineName.MatchString(x.Name) {
				problem = fmt.Sprintf("%s: a name %s", name, x.Name)
			}
			if x.Name == localFrame && !inlineField(stack) {
				at, ok := inlineFrameUse(f, stack)
				if !ok {
					problem = fmt.Sprintf("%s: %s used other than as one JSValue: %s", name, localFrame, f.lineOf(x))
				} else if !cells {
					problem = fmt.Sprintf("%s: %s holds a JSValue, and JSValue is not a union of 8 bytes and a tag of 8 bytes", name, localFrame)
				} else if locals[inlineCell] {
					problem = fmt.Sprintf("%s: a local named %s", name, inlineCell)
				}
				at.start -= start
				at.end -= start
				c.idents = append(c.idents, at)
				frame = true
				return problem == ""
			}
			at := inlineIdent{start: f.off(x.Pos()) - start, end: f.off(x.End()) - start, param: -1}
			switch parent := stack[len(stack)-2].(type) {
			case *ast.LabeledStmt:
				at.label = x.Name
			case *ast.BranchStmt:
				if parent.Label == x {
					at.label = x.Name
				}
			}
			if i, ok := param[x.Name]; ok && at.label == "" {
				at.param = i
			}
			if at.label != "" || at.param >= 0 {
				c.idents = append(c.idents, at)
			} else if !locals[x.Name] && !inlineField(stack) && x.Name != "_" {
				c.free[x.Name] = true
			}
		}
		return problem == ""
	})
	if problem != "" {
		return nil, fmt.Errorf("%s", problem)
	}
	if c.free[inlineCell] {
		return nil, fmt.Errorf("%s: a name %s", name, inlineCell)
	}
	if frame {
		locals[inlineCell] = true // no variable of that name in place of a parameter
	}
	sort.Slice(c.idents, func(i, j int) bool { return c.idents[i].start < c.idents[j].start })
	return c, nil
}

// inlineFrameUse reads a use of localbp, the TLS frame fixFrames gives a
// callee, at the top of stack. Where the frame is one JSValue cell, as in
// set_value:
//
//	var localbp [2]uint64
//	*(*TJSValue)(unsafe.Pointer(&localbp)) = v1
//	v2 = *(*uintptr)(unsafe.Pointer(&localbp)) - ...
//
// a copy has a Go JSValue in its place, which the compiler can keep in
// registers, and reads the pointer from its first 8 bytes as fixCells
// does:
//
//	var cell TJSValue
//	cell = v1
//	v2 = uintptr(cell.Fu.Fuint641) - ...
//
// inlineFrameUse returns the text of the copy for the declaration, the store
// or the load, and false for any other use.
func inlineFrameUse(f *parsedFunc, stack []ast.Node) (inlineIdent, bool) {
	n := len(stack)
	id := stack[n-1]
	if spec, ok := stack[n-2].(*ast.ValueSpec); ok {
		if len(spec.Names) != 1 || spec.Values != nil || f.text(spec.Type) != "[2]uint64" {
			return inlineIdent{}, false
		}
		return inlineIdent{start: f.off(id.Pos()), end: f.off(spec.Type.End()), param: -1, text: inlineCell + " TJSValue"}, true
	}
	// &localbp in unsafe.Pointer(...) in (*T)(...) under a *.
	if n < 7 {
		return inlineIdent{}, false
	}
	addr, ok := stack[n-2].(*ast.UnaryExpr)
	if !ok || addr.Op != token.AND {
		return inlineIdent{}, false
	}
	ptr, ok := stack[n-3].(*ast.CallExpr)
	if !ok || !isSelector(ptr.Fun, "unsafe", "Pointer") || len(ptr.Args) != 1 {
		return inlineIdent{}, false
	}
	conv, ok := stack[n-4].(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 {
		return inlineIdent{}, false
	}
	typ, ok := conv.Fun.(*ast.ParenExpr)
	if !ok {
		return inlineIdent{}, false
	}
	deref, ok := stack[n-5].(*ast.StarExpr)
	if !ok {
		return inlineIdent{}, false
	}
	at := inlineIdent{start: f.off(deref.Pos()), end: f.off(deref.End()), param: -1}
	assign, _ := stack[n-6].(*ast.AssignStmt)
	stored := assign != nil && len(assign.Lhs) == 1 && assign.Lhs[0] == deref
	switch f.text(typ.X) {
	case "*TJSValue":
		if !stored || assign.Tok != token.ASSIGN || len(assign.Rhs) != 1 {
			return inlineIdent{}, false
		}
		at.text = inlineCell
	case "*uintptr":
		if stored {
			return inlineIdent{}, false
		}
		if u, ok := stack[n-6].(*ast.UnaryExpr); ok && u.Op == token.AND {
			return inlineIdent{}, false
		}
		at.text = "uintptr(" + inlineCell + ".Fu.Fuint641)"
	default:
		return inlineIdent{}, false
	}
	return at, true
}

// inlineField reports whether the identifier at the top of stack names a
// field: a selector or the key of a struct literal.
func inlineField(stack []ast.Node) bool {
	id := stack[len(stack)-1]
	switch p := stack[len(stack)-2].(type) {
	case *ast.SelectorExpr:
		return p.Sel == id
	case *ast.KeyValueExpr:
		return p.Key == id
	}
	return false
}

// block writes out a call with the arguments args, as the k-th call
// written out, at the indent in. An argument with direct set is a
// variable that the body reads in place of the parameter.
func (c *inlineCallee) block(in string, args []string, direct []bool, k int) string {
	var with []string
	for i, a := range args {
		if direct[i] {
			with = append(with, fmt.Sprintf("%s as inl%d", a, i+1))
		}
	}
	var s strings.Builder
	s.WriteString("{\n" + in + "\t" + inlineMark(c.name, with...))
	for i, a := range args {
		if !direct[i] {
			fmt.Fprintf(&s, "%s\tvar inl%d %s = %s\n", in, i+1, c.types[i], a)
		}
	}
	var body strings.Builder
	last := 0
	for _, id := range c.idents {
		body.WriteString(c.body[last:id.start])
		if id.text != "" {
			body.WriteString(id.text)
		} else if id.label != "" {
			fmt.Fprintf(&body, "inl%d_%s", k, id.label)
		} else if direct[id.param] {
			body.WriteString(args[id.param])
		} else {
			fmt.Fprintf(&body, "inl%d", id.param+1)
		}
		last = id.end
	}
	body.WriteString(c.body[last:])
	for _, line := range strings.SplitAfter(body.String(), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, "\t"), "// Changed by internal/fixlib:") {
			continue // the marks of other changes stay with the callee
		}
		if line != "" && line != "\n" {
			s.WriteString(in)
		}
		s.WriteString(line)
	}
	s.WriteString(in + "}")
	return s.String()
}

// inlineCalls writes out the calls of callees, with the number of calls
// of each, in inlineFunc of b.
func inlineCalls(b []byte, callees map[string]int) ([]byte, error) {
	body, err := funcBody(b, funcHead(inlineFunc), inlineFunc)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte("fixInline")) {
		return nil, fmt.Errorf("%s: changed by fixInline already", inlineFunc)
	}
	f, err := parseFunc(inlineFunc, body)
	if err != nil {
		return nil, err
	}
	var problem string
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && problem == "" && reInlineName.MatchString(id.Name) {
			problem = fmt.Sprintf("%s already has a name %s", inlineFunc, id.Name)
		}
		return problem == ""
	})
	if problem != "" {
		return nil, fmt.Errorf("%s", problem)
	}
	locals := localNames(f.decl)
	vars := inlineVars(f)
	cells := bytes.Contains(b, []byte(cellsValue)) && reCellsUnion.Match(b) && bytes.Contains(b, []byte("\ntype Tuint64_t = uint64\n"))
	names := make([]string, 0, len(callees))
	for name := range callees {
		names = append(names, name)
	}
	sort.Strings(names)
	cs := map[string]*inlineCallee{}
	for _, name := range names {
		cb, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return nil, err
		}
		c, err := newInlineCallee(name, cb, cells)
		if err != nil {
			return nil, err
		}
		free := make([]string, 0, len(c.free))
		for id := range c.free {
			free = append(free, id)
		}
		sort.Strings(free)
		for _, id := range free {
			if id != "tls" && locals[id] {
				return nil, fmt.Errorf("%s, which %s uses, is a local of %s", id, name, inlineFunc)
			}
		}
		cs[name] = c
	}

	counts := map[string]int{}
	var edits []edit
	var problems []string
	k := 0
	var stack []ast.Node
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		call, ok := n.(*ast.CallExpr)
		var c *inlineCallee
		if ok {
			if id, ok := call.Fun.(*ast.Ident); ok {
				c = cs[id.Name]
			}
		}
		if c == nil {
			stack = append(stack, n)
			return true
		}
		// The arguments are left as they are: a call of a callee in
		// them is not counted, and the counts stop it.
		counts[c.name]++
		k++
		args := make([]string, len(call.Args))
		for i, a := range call.Args {
			args[i] = f.text(a)
		}
		want := c.name + "(" + strings.Join(args, ", ") + ")"
		switch {
		case len(args) != len(c.params)+1 || args[0] != "tls":
			problems = append(problems, fmt.Sprintf("%s: %s, not with tls and %d arguments", inlineFunc, firstLine(f.text(call)), len(c.params)))
			return false
		case f.text(call) != want:
			problems = append(problems, fmt.Sprintf("%s: %s not written as %s", inlineFunc, firstLine(f.text(call)), firstLine(want)))
			return false
		}
		s, ok := stack[len(stack)-1].(*ast.ExprStmt)
		var in ast.Node = stack[len(stack)-1]
		if ok {
			in = stack[len(stack)-2]
			switch in.(type) {
			case *ast.BlockStmt, *ast.CaseClause, *ast.LabeledStmt:
				in = nil
			}
		}
		if in != nil {
			problems = append(problems, fmt.Sprintf("%s: %s called in a %T", inlineFunc, c.name, in))
			return false
		}
		indent := indentOf(f, s)
		if ls, _ := f.line(s); string(f.src[ls:f.off(s.Pos())]) != indent || strings.Trim(indent, "\t") != "" {
			problems = append(problems, fmt.Sprintf("%s: %s is not on a line of its own", inlineFunc, firstLine(want)))
			return false
		}
		direct := make([]bool, len(c.params))
		for i, a := range call.Args[1:] {
			id, ok := a.(*ast.Ident)
			direct[i] = ok && vars[id.Name] == c.types[i] && !c.written[i] && !c.locals[id.Name]
		}
		edits = append(edits, edit{f.off(s.Pos()), f.off(s.End()), c.block(indent, args[1:], direct, k)})
		return false
	})
	for _, name := range names {
		if counts[name] != callees[name] {
			problems = append(problems, fmt.Sprintf("%s: %d calls of %s, inlineCallees has %d", inlineFunc, counts[name], name, callees[name]))
		}
	}
	if len(problems) != 0 {
		return nil, fmt.Errorf("calls written out by fixInline changed, review them and update inlineCallees:\n\t%s", strings.Join(problems, "\n\t"))
	}
	out := f.apply(edits)
	if _, err := parseFunc(inlineFunc, out); err != nil {
		return nil, err
	}
	return bytes.Replace(b, body, out, 1), nil
}

// inlineVars returns the variables of the function f that a copy may read
// in place of a parameter of the same type, with their types: those
// declared once, as a parameter or by a var with a type, whose address
// is not taken and which no function literal uses. Nothing else can set
// such a variable while a copy runs. An address given to _ only, as
// fixPin gives the variables it keeps in their stack slots, goes nowhere:
// a copy then reads the variable in its slot, as the call did.
func inlineVars(f *parsedFunc) map[string]string {
	vars := map[string]string{}
	count := map[string]int{}
	add := func(names []*ast.Ident, typ ast.Expr) {
		for _, id := range names {
			count[id.Name]++
			if typ != nil {
				vars[id.Name] = f.text(typ)
			}
		}
	}
	for _, p := range f.decl.Type.Params.List {
		add(p.Names, p.Type)
	}
	if res := f.decl.Type.Results; res != nil {
		for _, p := range res.List {
			add(p.Names, nil)
		}
	}
	not := map[string]bool{}
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ValueSpec:
			add(x.Names, x.Type)
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, l := range x.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						add([]*ast.Ident{id}, nil)
					}
				}
			}
			blank := true
			for _, l := range x.Lhs {
				blank = blank && isIdent(l, "_")
			}
			if blank {
				for _, r := range x.Rhs {
					if u, ok := r.(*ast.UnaryExpr); ok && u.Op == token.AND {
						if _, ok := u.X.(*ast.Ident); ok {
							continue
						}
					}
					ast.Inspect(r, visit)
				}
				return false
			}
		case *ast.UnaryExpr:
			if id, ok := ast.Unparen(x.X).(*ast.Ident); ok && x.Op == token.AND {
				not[id.Name] = true
			}
		case *ast.FuncLit:
			ast.Inspect(x, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					not[id.Name] = true
				}
				return true
			})
			return false
		}
		return true
	}
	ast.Inspect(f.decl.Body, visit)
	for name := range vars {
		if count[name] != 1 || not[name] {
			delete(vars, name)
		}
	}
	return vars
}

// restoreInline undoes fixInline: it writes each call it wrote out back
// as the call. It fails on a change that is not in the form fixInline
// writes.
func restoreInline(b []byte) ([]byte, error) {
	if !bytes.Contains(b, []byte("(see fixInline)")) {
		return b, nil
	}
	body, err := funcBody(b, funcHead(inlineFunc), inlineFunc)
	if err != nil {
		return nil, err
	}
	if n, m := bytes.Count(b, []byte("(see fixInline)")), bytes.Count(body, []byte("(see fixInline)")); n != m {
		return nil, fmt.Errorf("%d changes of fixInline outside %s", n-m, inlineFunc)
	}
	// The blocks, from their first statements.
	f, err := parseFunc(inlineFunc, body)
	if err != nil {
		return nil, err
	}
	var edits []edit
	var problems []string
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		blk, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		m := reInlineBlock.FindSubmatch(f.src[f.off(blk.Lbrace):])
		if m == nil {
			return true
		}
		name := string(m[1])
		// The variables of the mark, then the declarations of the others
		// in order.
		given := map[int]string{}
		for _, w := range reInlineWith.FindAllSubmatch(m[2], -1) {
			i, _ := strconv.Atoi(string(w[2]))
			given[i] = string(w[1])
		}
		var args []string
		next := func() {
			for given[len(args)+1] != "" {
				args = append(args, given[len(args)+1])
			}
		}
		next()
		for _, s := range blk.List {
			d, ok := s.(*ast.DeclStmt)
			if !ok {
				break
			}
			g := d.Decl.(*ast.GenDecl)
			if g.Tok != token.VAR || len(g.Specs) != 1 {
				break
			}
			v := g.Specs[0].(*ast.ValueSpec)
			if len(v.Names) != 1 || v.Names[0].Name != fmt.Sprintf("inl%d", len(args)+1) || len(v.Values) != 1 {
				break
			}
			args = append(args, f.text(v.Values[0]))
			next()
		}
		if len(args) == 0 {
			problems = append(problems, fmt.Sprintf("%s: a block of %s without its arguments", inlineFunc, name))
			return false
		}
		for i := range given {
			if i > len(args) {
				problems = append(problems, fmt.Sprintf("%s: a block of %s without its argument inl%d", inlineFunc, name, len(args)+1))
				return false
			}
		}
		edits = append(edits, edit{f.off(blk.Pos()), f.off(blk.End()), name + "(tls, " + strings.Join(args, ", ") + ")"})
		return false
	})
	if len(problems) != 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "\n\t"))
	}
	out := f.apply(edits)
	if bytes.Contains(out, []byte("fixInline")) {
		return nil, fmt.Errorf("%s: a change of fixInline left after undoing it", inlineFunc)
	}
	return bytes.Replace(b, body, out, 1), nil
}
