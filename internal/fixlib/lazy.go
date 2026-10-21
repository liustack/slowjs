// Copyright (c) 2026 The liustack/quickjs Authors.
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
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The marks fixLazyFrames leaves (see fixLazyFrames).
const (
	lazyStmtMark   = "// Changed by internal/fixlib: a TLS frame for this statement (see fixLazyFrames).\n"
	lazyReturnMark = "// Changed by internal/fixlib: a TLS frame for this return (see fixLazyFrames).\n"
	lazyIfMark     = "// Changed by internal/fixlib: a TLS frame for the condition of this if (see fixLazyFrames).\n"
)

// lazyHeadMark is the mark at the head of a function fixLazyFrames changed,
// whose TLS frame was n bytes.
func lazyHeadMark(n int) string {
	return fmt.Sprintf("// Changed by internal/fixlib: the TLS frame of %d bytes is a Go array, and each statement that needs a TLS frame takes one of its own (see fixLazyFrames).\n", n)
}

var reLazyHead = regexp.MustCompile(`^\{\n\t// Changed by internal/fixlib: the TLS frame of (\d+) bytes is a Go array, and each statement that needs a TLS frame takes one of its own \(see fixLazyFrames\)\.\n`)

// lazyHead returns the frame size in the head mark of body, the text of a
// function body from its opening brace, or false when it has none.
func lazyHead(body []byte) (int, bool) {
	m := reLazyHead.FindSubmatch(body)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(string(m[1]))
	return n, err == nil
}

// fixLazyFrames gives each function of lazySites a Go array for its TLS
// frame, as fixFrames does, though the function passes an address in the
// frame to a callee or reaches a stack check on some paths. Each statement
// that does either takes a TLS frame of the same size for itself:
//
//	bp = tls.Alloc(N)
//	*(*[N/8]uint64)(unsafe.Pointer(bp)) = localbp
//	STATEMENT, as ccgo wrote it, using bp
//	localbp = *(*[N/8]uint64)(unsafe.Pointer(bp))
//	tls.Free(N)
//
// The copies keep the cells of the Go array and of the TLS frame the same,
// so the statement reads and writes the cells as before. A stack check the
// statement reaches sees the TLS stack it saw before: one more slot, and
// the frame at the same place. A statement that does not use bp takes the
// frame without the copies. On its usual path the function then takes no
// TLS frame at all.
//
// A function qualifies when:
//
//  1. every use of bp outside these statements is a load or a store of a
//     cell (as in fixFrames), or the out parameter of a callee of
//     outParams, which gets a Go pointer into the array instead and goes to
//     a copy of the callee that takes one (outParams says why that is
//     safe);
//  2. every address in the frame a statement passes on goes to a callee of
//     addrCallees, which does not keep it after it returns, nor return it,
//     except libc.VaList, whose result must be an argument of such a callee
//     in the same call;
//  3. every call that can reach a stack check (see fixFrames) is in such a
//     statement;
//  4. each such statement is an expression, an assignment or a return of
//     one value in a list of statements, or the condition of an if that is
//     not an else if and has no init statement, which is evaluated into
//     lazyc first.
//
// Other changes match the text of these functions as ccgo writes it, so
// fixLazyFrames comes last, before fixNoDefer, and restoreLazy, its exact
// inverse, comes first. fixLazyFrames checks that restoreLazy gives its
// input back. The frame sizes, cell uses, wrapped statements and out
// parameters of lib/ccgo_darwin_arm64.go are pinned in lazySites. On
// 32-bit targets, where a JSValue is a uint64, the functions of
// lazyFrameless have no TLS frame and are left as they are, and the frames
// of the others are smaller.
func fixLazyFrames(b []byte, hw []*ast.File, name string) ([]byte, error) {
	got, err := lazyAddrFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := lazyAddrSites[name]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s or a get_own_property method changed (fingerprint %s, want %s): check that none keeps an address it is given after it returns, nor returns it, then update lazyAddrSites (go run ./internal/fixlib -lazyaddr)", name, strings.Join(lazyAddrFuncs, ", "), got, want)
	}
	out, err := lazyFrames(b, hw, lazySites, name == lazyPinFile)
	if err != nil {
		return nil, err
	}
	back, err := restoreLazy(out)
	if err != nil {
		return nil, fmt.Errorf("fixLazyFrames: %v", err)
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixLazyFrames: restoreLazy does not give the input back")
	}
	return out, nil
}

// lazyPinFile is the file whose counts must be those of lazySites.
const lazyPinFile = "ccgo_darwin_arm64.go"

// lazySite is a function of lazySites, as in lazyPinFile.
type lazySite struct {
	frame, cells, wraps, outs int
	why                       string
}

var lazySites = map[string]lazySite{
	"XJS_GetPropertyInternal":    {144, 31, 14, 1, "property reads: the found property (find_own_property) and the descriptor, getters, errors and exotic objects need the frame"},
	"_JS_GetOwnPropertyInternal": {32, 15, 6, 1, "the found property (find_own_property), auto init, errors and exotic objects need the frame"},
	"_JS_AtomIsArrayIndex":       {16, 2, 0, 1, "the index of a string atom (is_num_string)"},
	"_JS_NewAtomStr":             {16, 2, 0, 1, "the index of a numeric string (is_num_string)"},
	"_js_eq_slow":                {48, 56, 6, 0, "the operands, conversions of objects, strings to numbers and BigInts need the frame"},
	"_JS_ToStringInternal":       {80, 14, 13, 0, "the value, ropes, numbers, symbols and objects need the frame"},
	"_JS_ToObject":               {48, 12, 10, 0, "the value, making a wrapper object needs the frame"},
	"_js_array_finalizer":        {32, 4, 0, 0, "the elements, to read their pointers (JS_FreeValueRT)"},
	"_JS_ToInt32Free":            {32, 9, 1, 0, "the value and float conversions, turning a value that is not a number into one needs the frame"},
	"_JS_ToInt32SatFree":         {16, 5, 1, 0, "the value and float conversions, turning a value that is not a number into one needs the frame"},
	"XJS_GetPrototype":           {32, 8, 1, 0, "the value, the get_prototype method of an exotic object needs the frame"},
	"_JS_ToFloat64Free":          {16, 4, 1, 0, "the value, turning a value that is not a number into one needs the frame"},
	"_js_get_regexp":             {16, 3, 1, 0, "the object, the error for another class needs the frame"},
	"_js_regexp_get_lastIndex":   {32, 4, 1, 0, "the object, a lastIndex that is not an integer needs the frame"},
	"_JS_ToInt64SatFree":         {16, 5, 1, 0, "the value and float conversions, turning a value that is not a number into one needs the frame"},
	"_js_thisNumberValue":        {32, 9, 1, 0, "the value, the error for a value that is not a number needs the frame"},
	"_js_sub_string":             {16, 2, 3, 0, "the string, making the substring needs the frame"},
	"_js_object_has_name":        {48, 6, 0, 1, "the object and the found property (find_own_property)"},
	"_JS_ConcatString2":          {48, 15, 1, 0, "the strings, joining two strings that do not fit in place needs the frame"},
	"_add_fast_array_element":    {16, 4, 2, 0, "the array, growing it and the error for a read-only length need the frame"},
}

// outParam is a callee of outParams: its second parameter (after tls) is a
// pointer it only stores through or loads through, as *(*T)(unsafe.Pointer(p)),
// and the callee reaches no stack check. A copy of it, clone, takes a *T
// instead, which the caller points into its Go array: the address does not
// outlive the call, and Go sees it as a pointer.
type outParam struct {
	clone, elem string
}

var outParams = map[string]outParam{
	"_find_own_property": {"_find_own_property_lazy", "uintptr"},
	"_is_num_string":     {"_is_num_string_lazy", "Tuint32_t"},
}

// addrCallees may be given an address in the frame in a wrapped statement.
// What each does with it was read in lazyAddrFuncs and in the
// get_own_property methods of the engine's classes, whose fingerprints are
// pinned in lazyAddrSites.
var addrCallees = map[string]string{
	"libc.VaList":              "writes its arguments at the address and returns it, checked to be an argument of a callee below",
	"___JS_ThrowTypeErrorAtom": "formats the message from the va_list and returns",
	"XJS_ThrowTypeError":       "formats the message from the va_list and returns",
	"Xi32toa":                  "writes the digits and returns their count",
	"_js_new_string8_len":      "copies the digits into a new string",
	"(*TJSClassExoticMethods).Fget_own_property": "fills the property descriptor (the class API)",
}

// lazyAddrFuncs are the functions that get an address in the frame from
// the calls addrCallees allows, directly or passed on: the va_list goes
// from JS_ThrowTypeError through JS_ThrowError to vsnprintf in
// JS_ThrowError2, i32toa writes through u32toa, and the get_own_property
// method of a proxy without a trap passes the descriptor to
// JS_GetOwnPropertyInternal.
var lazyAddrFuncs = []string{
	"___JS_ThrowTypeErrorAtom",
	"XJS_ThrowTypeError",
	"_JS_ThrowError",
	"_JS_ThrowError2",
	"Xi32toa",
	"Xu32toa",
	"_js_new_string8_len",
	"_JS_GetOwnPropertyInternal",
}

var (
	reExoticType     = regexp.MustCompile(`\ntype TJSClassExoticMethods = struct \{\n\tFget_own_property +uintptr\n[^}]*\}\n`)
	reExoticTable    = regexp.MustCompile(`\nvar \w+ = TJSClassExoticMethods\{\}\n\nfunc init\(\) \{\n\tp := unsafe\.Pointer\(&\w+\)\n((?:\t\*\(\*uintptr\)\(unsafe\.Add\(p, \d+\)\) = __ccgo_fp\(\w+\)\n)+)\}\n`)
	reExoticGetOwn   = regexp.MustCompile(`\t\*\(\*uintptr\)\(unsafe\.Add\(p, 0\)\) = __ccgo_fp\((\w+)\)\n`)
	exoticTableCount = []byte("= TJSClassExoticMethods{}")
)

// lazyAddrFingerprint hashes the type of the exotic methods of a class,
// lazyAddrFuncs and the get_own_property methods the engine's classes set.
func lazyAddrFingerprint(b []byte) (string, error) {
	typ := reExoticType.Find(b)
	if typ == nil {
		return "", fmt.Errorf("TJSClassExoticMethods without get_own_property first")
	}
	tables := findAllSubmatch(reExoticTable, b)
	if n := bytes.Count(b, exoticTableCount); len(tables) != n {
		return "", fmt.Errorf("%d tables of exotic methods, %d set in an init function as fixLazyFrames reads them", n, len(tables))
	}
	names := append([]string{}, lazyAddrFuncs...)
	for _, t := range tables {
		if m := reExoticGetOwn.FindSubmatch(t[1]); m != nil {
			names = append(names, string(m[1]))
		}
	}
	fp, err := funcsFingerprint(b, names, func(text []byte) []byte { return text })
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(string(typ) + strings.Join(names, "\n") + "\n" + fp))
	return fmt.Sprintf("%x", h)[:16], nil
}

// lazyCloneMark is the mark above the copy of the callee orig of outParams.
func lazyCloneMark(orig string) string {
	return "// Changed by internal/fixlib: " + orig + " with its out parameter as a Go pointer, for the functions fixLazyFrames changes.\n"
}

var reLazyClone = regexp.MustCompile(`\n// Changed by internal/fixlib: (\w+) with its out parameter as a Go pointer, for the functions fixLazyFrames changes\.\nfunc (\w+)\(`)

// lazyFrameless are the functions of lazySites without a TLS frame on
// 32-bit targets, where a JSValue is a uint64 the functions keep in a Go
// local. lazyFrames leaves them as they are there.
var lazyFrameless = map[string]bool{
	"_JS_ToObject":             true,
	"_js_array_finalizer":      true,
	"XJS_GetPrototype":         true,
	"_JS_ConcatString2":        true,
	"_add_fast_array_element":  true,
	"_js_get_regexp":           true,
	"_js_regexp_get_lastIndex": true,
	"_js_sub_string":           true,
	"_js_thisNumberValue":      true,
}

// lazyFrames changes the functions of sites in b. pin checks the counts
// of sites. A function of lazyFrameless without a TLS frame is left as it
// is on 32-bit targets.
func lazyFrames(b []byte, hw []*ast.File, sites map[string]lazySite, pin bool) ([]byte, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "", b, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	wide := !bytes.Contains(b, []byte("\ntype TJSValue = uint64\n"))
	all := append(hw[:len(hw):len(hw)], file)
	reach := reaching(callGraph(all))
	imports := map[string]bool{"tls": true}
	for _, f := range all {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			name := path[strings.LastIndex(path, "/")+1:]
			if imp.Name != nil {
				name = imp.Name.Name
			}
			imports[name] = true
		}
	}
	names := make([]string, 0, len(sites))
	for name := range sites {
		names = append(names, name)
	}
	sort.Strings(names)
	var problems []string
	usedOut := map[string]bool{}
	for _, name := range names {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return nil, err
		}
		if !wide && !bytes.Contains(body, []byte("\n\tbp := tls.Alloc(")) {
			if !lazyFrameless[name] {
				problems = append(problems, fmt.Sprintf("%s: no TLS frame on a 32-bit target, and lazyFrameless does not have it", name))
			}
			continue
		}
		fixed, got, outs, err := lazyFunc(name, body, reach, imports)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if want := sites[name]; pin && (got.frame != want.frame || got.cells != want.cells || got.wraps != want.wraps || got.outs != want.outs) {
			problems = append(problems, fmt.Sprintf("%s: a frame of %d bytes, %d cell uses, %d statements with a frame of their own and %d out parameters, lazySites has %d, %d, %d and %d", name, got.frame, got.cells, got.wraps, got.outs, want.frame, want.cells, want.wraps, want.outs))
			continue
		}
		for o := range outs {
			usedOut[o] = true
		}
		b = bytes.Replace(b, body, fixed, 1)
	}
	if len(problems) != 0 {
		return nil, fmt.Errorf("TLS frames taken by statement changed, review each and update lazySites:\n\t%s", strings.Join(problems, "\n\t"))
	}
	outs := make([]string, 0, len(usedOut))
	for o := range usedOut {
		outs = append(outs, o)
	}
	sort.Strings(outs)
	for _, o := range outs {
		if r := reach[o]; r != "" {
			return nil, fmt.Errorf("%s can reach a stack check, through %s", o, r)
		}
		body, err := funcBody(b, funcHead(o), o)
		if err != nil {
			return nil, err
		}
		clone, err := outClone(o, body)
		if err != nil {
			return nil, err
		}
		i := bytes.Index(b, body) + len(body)
		text := "\n" + lazyCloneMark(o) + string(clone[1:])
		b = append(b[:i:i], append([]byte(text), b[i:]...)...)
	}
	return b, nil
}

// outClone copies the callee name of outParams into a function that takes
// its out parameter as a Go pointer. body and the copy are in the form
// funcBody gives.
func outClone(name string, body []byte) ([]byte, error) {
	op := outParams[name]
	f, err := parseFunc(name, body)
	if err != nil {
		return nil, err
	}
	params := f.decl.Type.Params.List
	if len(params) < 2 || len(params[1].Names) != 1 || !isIdent(params[1].Type, "uintptr") {
		return nil, fmt.Errorf("%s: the second parameter is not one uintptr", name)
	}
	p := params[1].Names[0].Name
	edits := []edit{{f.off(f.decl.Name.Pos()), f.off(f.decl.Name.End()), op.clone}, {f.off(params[1].Type.Pos()), f.off(params[1].Type.End()), "*" + op.elem}}
	uses := 0
	var stack []ast.Node
	bad := ""
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != p {
			return true
		}
		// *(*ELEM)(unsafe.Pointer(p))
		k := len(stack) - 1
		call, ok1 := stack[k-1].(*ast.CallExpr)
		conv, ok2 := stack[k-2].(*ast.CallExpr)
		star, ok3 := stack[k-3].(*ast.StarExpr)
		if !ok1 || !ok2 || !ok3 || !isSelector(call.Fun, "unsafe", "Pointer") || conv.Args[0] != call || star.X != conv {
			bad = fmt.Sprintf("%s: %s used in %s", name, p, f.text(stack[k-1]))
			return false
		}
		if typ, ok := ptrType(conv.Fun, 1); !ok || f.text(typ) != op.elem {
			bad = fmt.Sprintf("%s: %s converted by %s", name, p, f.text(conv.Fun))
			return false
		}
		for _, s := range stack[:k-3] {
			if u, ok := s.(*ast.UnaryExpr); ok && u.Op == token.AND {
				bad = fmt.Sprintf("%s: the address of *%s taken", name, p)
				return false
			}
		}
		edits = append(edits, edit{f.off(star.Pos()), f.off(star.End()), "*" + p})
		uses++
		return true
	})
	if bad != "" {
		return nil, fmt.Errorf("%s", bad)
	}
	if uses == 0 {
		return nil, fmt.Errorf("%s: %s not used", name, p)
	}
	return f.apply(edits), nil
}

// lazyWrap is a statement that takes a TLS frame of its own.
type lazyWrap struct {
	stmt   ast.Stmt
	usesBP bool
}

// lazyCell is the text fixLazyFrames writes for a pointer to the cell at
// off of the Go array.
func lazyCell(off int) string {
	if off == 0 {
		return "unsafe.Pointer(&" + localFrame + ")"
	}
	return fmt.Sprintf("unsafe.Add(unsafe.Pointer(&%s), %d)", localFrame, off)
}

// lazyFunc changes one function.
func lazyFunc(name string, body []byte, reach map[string]string, imports map[string]bool) ([]byte, lazySite, map[string]bool, error) {
	var site lazySite
	if bytes.Contains(body, []byte("fixLazyFrames")) {
		return nil, site, nil, fmt.Errorf("%s: changed by fixLazyFrames already", name)
	}
	f, err := parseFunc(name, body)
	if err != nil {
		return nil, site, nil, err
	}
	list := f.decl.Body.List
	n, ok := frameHead(list)
	if !ok {
		return nil, site, nil, fmt.Errorf("%s: the frame is not allocated at the entry", name)
	}
	site.frame = n
	hstart, _ := f.line(list[0])
	_, hend := f.line(list[1])
	if h := fmt.Sprintf("\tbp := tls.Alloc(%d)\n\tdefer tls.Free(%d)\n", n, n); string(f.src[hstart:hend]) != h {
		return nil, site, nil, fmt.Errorf("%s: the frame is not taken by %q", name, h)
	}
	for _, v := range []string{localFrame, "lazyc"} {
		if identUsed(f.decl.Body, v) {
			return nil, site, nil, fmt.Errorf("%s: already has a variable %s", name, v)
		}
	}
	// The fields the function pointers of local variables come from (see
	// callGraph).
	fields := map[string]map[string]bool{}
	ast.Inspect(f.decl.Body, func(x ast.Node) bool {
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
	// needs reports whether c can reach a stack check.
	needs := func(c *ast.CallExpr) bool {
		switch fn := c.Fun.(type) {
		case *ast.Ident:
			switch fn.Name {
			case "_js_check_stack_overflow":
				return true
			case "__ccgo_fp":
				if len(c.Args) == 1 {
					if id, ok := c.Args[0].(*ast.Ident); ok {
						return reach[id.Name] != ""
					}
				}
				return true
			}
			return reach[fn.Name] != ""
		case *ast.SelectorExpr:
			if id, ok := fn.X.(*ast.Ident); ok && imports[id.Name] {
				return false
			}
		case *ast.FuncLit:
			return false
		case *ast.ParenExpr, *ast.ArrayType, *ast.StarExpr, *ast.FuncType, *ast.InterfaceType, *ast.MapType, *ast.ChanType:
			if isConversion(c.Fun) {
				return false
			}
		}
		return !indirectAllowed(indirectVia(c.Fun), fields)
	}

	// Find the statements to wrap.
	wraps := map[ast.Stmt]*lazyWrap{}
	var order []*lazyWrap
	var problems []string
	wrapAt := func(stack []ast.Node, why string) {
		for i := len(stack) - 1; i >= 1; i-- {
			s, ok := stack[i].(ast.Stmt)
			if !ok {
				continue
			}
			switch x := s.(type) {
			case *ast.ExprStmt, *ast.AssignStmt, *ast.IncDecStmt:
			case *ast.ReturnStmt:
				if len(x.Results) != 1 {
					problems = append(problems, fmt.Sprintf("%s: %s in a return of %d values", name, why, len(x.Results)))
					return
				}
			case *ast.IfStmt:
				if i+1 >= len(stack) || stack[i+1] != x.Cond || x.Init != nil {
					problems = append(problems, fmt.Sprintf("%s: %s in an if, not in its condition", name, why))
					return
				}
			default:
				problems = append(problems, fmt.Sprintf("%s: %s in a %T", name, why, s))
				return
			}
			switch p := stack[i-1].(type) {
			case *ast.BlockStmt, *ast.CaseClause, *ast.LabeledStmt:
			case *ast.IfStmt:
				if p.Else == s {
					problems = append(problems, fmt.Sprintf("%s: %s in the condition of an else if", name, why))
				} else {
					problems = append(problems, fmt.Sprintf("%s: %s in a statement of an if", name, why))
				}
				return
			default:
				problems = append(problems, fmt.Sprintf("%s: %s in a statement inside a %T", name, why, p))
				return
			}
			if wraps[s] == nil {
				wraps[s] = &lazyWrap{stmt: s}
				order = append(order, wraps[s])
			}
			return
		}
		problems = append(problems, fmt.Sprintf("%s: %s outside a statement", name, why))
	}
	// outArg matches bp or bp+K, at the top of stack, as the out parameter
	// of a callee of outParams.
	outArg := func(stack []ast.Node) (*ast.CallExpr, ast.Expr, bool) {
		k := len(stack) - 1
		cell := ast.Expr(stack[k].(*ast.Ident))
		if be, ok := stack[k-1].(*ast.BinaryExpr); ok && be.X == cell {
			cell = be
			k--
		}
		if _, ok := frameCell(cell); !ok {
			return nil, nil, false
		}
		call, ok := stack[k-1].(*ast.CallExpr)
		if !ok || len(call.Args) < 2 || call.Args[1] != cell {
			return nil, nil, false
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok {
			return nil, nil, false
		}
		_, ok = outParams[id.Name]
		return call, cell, ok
	}
	stack := []ast.Node{f.decl.Body}
	for _, s := range list[2:] {
		ast.Inspect(s, func(nd ast.Node) bool {
			if nd == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, nd)
			switch x := nd.(type) {
			case *ast.CallExpr:
				if needs(x) {
					wrapAt(stack, "a call that can reach a stack check, "+calleeName(f, x))
				}
			case *ast.Ident:
				if x.Name != "bp" {
					break
				}
				if _, why := cellAccess(f, stack); why == "" {
					break
				}
				if call, _, ok := outArg(stack); ok && !needs(call) {
					break
				}
				wrapAt(stack, "the frame address in "+f.text(stack[len(stack)-2]))
			}
			return true
		})
		stack = stack[:1]
	}
	if len(problems) != 0 {
		return nil, site, nil, fmt.Errorf("%s", strings.Join(problems, "\n\t"))
	}
	for _, w := range order {
		ast.Inspect(w.stmt, func(nd ast.Node) bool {
			if id, ok := nd.(*ast.Ident); ok && id.Name == "bp" {
				w.usesBP = true
			}
			return !w.usesBP
		})
	}
	// The part of a wrapped statement that keeps the TLS frame: an if's
	// condition, or the whole statement.
	inWrap := func(p token.Pos) bool {
		for _, w := range order {
			if ifs, ok := w.stmt.(*ast.IfStmt); ok {
				if p >= ifs.Cond.Pos() && p < ifs.Cond.End() {
					return true
				}
			} else if p >= w.stmt.Pos() && p < w.stmt.End() {
				return true
			}
		}
		return false
	}
	// Check the addresses the wrapped statements pass on.
	for _, w := range order {
		var st []ast.Node
		root := ast.Node(w.stmt)
		if ifs, ok := w.stmt.(*ast.IfStmt); ok {
			root = ifs.Cond
		}
		ast.Inspect(root, func(nd ast.Node) bool {
			if nd == nil {
				st = st[:len(st)-1]
				return true
			}
			st = append(st, nd)
			id, ok := nd.(*ast.Ident)
			if !ok || id.Name != "bp" {
				return true
			}
			if _, why := cellAccess(f, st); why == "" {
				return true
			}
			if why := addrUse(f, st); why != "" {
				problems = append(problems, fmt.Sprintf("%s: %s", name, why))
			}
			return true
		})
	}
	if len(problems) != 0 {
		return nil, site, nil, fmt.Errorf("%s", strings.Join(problems, "\n\t"))
	}

	// Edits: the head, the cells and out parameters outside the wrapped
	// statements, and the wrapped statements. restoreLazy undoes each by
	// its text, so each must be in the one form it reads back.
	words := (n + 7) / 8
	head := "\t" + lazyHeadMark(n) + fmt.Sprintf("\tvar %s [%d]uint64\n", localFrame, words)
	usesBP, ifs := false, false
	for _, w := range order {
		usesBP = usesBP || w.usesBP
		if _, ok := w.stmt.(*ast.IfStmt); ok {
			ifs = true
		}
	}
	if usesBP {
		head += "\tvar bp uintptr\n"
	}
	if ifs {
		head += "\tvar lazyc bool\n"
	}
	edits := []edit{{hstart, hend, head}}
	outs := map[string]bool{}
	for _, s := range list[2:] {
		ast.Inspect(s, func(nd ast.Node) bool {
			if nd == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, nd)
			id, ok := nd.(*ast.Ident)
			if !ok || id.Name != "bp" || inWrap(id.Pos()) {
				return true
			}
			if u, why := cellAccess(f, stack); why == "" {
				was := fmt.Sprintf("(*%s)(unsafe.Pointer(bp + %d))", u.typ, u.off)
				if u.off == 0 {
					was = fmt.Sprintf("(*%s)(unsafe.Pointer(bp))", u.typ)
				}
				if got := string(f.src[u.start:u.end]); u.ccgoUp || got != was {
					problems = append(problems, fmt.Sprintf("%s: a cell written %s, not %s", name, got, was))
					return true
				}
				edits = append(edits, edit{u.start, u.end, fmt.Sprintf("(*%s)(%s)", u.typ, lazyCell(u.off))})
				site.cells++
				return true
			}
			call, cell, ok := outArg(stack)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: bp left outside a wrapped statement in %s", name, f.text(stack[len(stack)-2])))
				return true
			}
			k, _ := frameCell(cell)
			was := fmt.Sprintf("bp+%d", k)
			if k == 0 {
				was = "bp"
			}
			callee := call.Fun.(*ast.Ident).Name
			if got := f.text(call.Fun) + "(" + f.text(call.Args[0]) + ", " + f.text(cell); got != callee+"(tls, "+was {
				problems = append(problems, fmt.Sprintf("%s: the out parameter passed in %s, not %s(tls, %s", name, got, callee, was))
				return true
			}
			op := outParams[callee]
			edits = append(edits, edit{f.off(call.Fun.Pos()), f.off(call.Fun.End()), op.clone})
			edits = append(edits, edit{f.off(cell.Pos()), f.off(cell.End()), fmt.Sprintf("(*%s)(%s)", op.elem, lazyCell(k))})
			outs[callee] = true
			site.outs++
			return true
		})
		stack = stack[:1]
	}
	result := ""
	if res := f.decl.Type.Results; res != nil && len(res.List) == 1 && len(res.List[0].Names) == 1 {
		result = res.List[0].Names[0].Name
	}
	for _, w := range order {
		in := indentOf(f, w.stmt)
		start, end := f.off(w.stmt.Pos()), f.off(w.stmt.End())
		if ifs, ok := w.stmt.(*ast.IfStmt); ok {
			end = f.off(ifs.Body.Lbrace) + 1
		}
		// A comment after the statement stays with it, except after an if.
		eol := bytes.IndexByte(f.src[end:], '\n')
		comment := ""
		if eol > 0 {
			comment = string(f.src[end : end+eol])
		}
		_, isIf := w.stmt.(*ast.IfStmt)
		if ls, _ := f.line(w.stmt); string(f.src[ls:start]) != in || in == "" || strings.Trim(in, "\t") != "" || eol < 0 || comment != "" && (isIf || !lineComment(comment)) {
			problems = append(problems, fmt.Sprintf("%s: %s is not on lines of its own", name, firstLine(f.text(w.stmt))))
			continue
		}
		end += len(comment)
		alloc := fmt.Sprintf("%stls.Alloc(%d)\n", in, n)
		copyIn, copyOut := "", ""
		if w.usesBP {
			alloc = fmt.Sprintf("%sbp = tls.Alloc(%d)\n", in, n)
			copyIn = fmt.Sprintf("%s*(*[%d]uint64)(unsafe.Pointer(bp)) = %s\n", in, words, localFrame)
			copyOut = fmt.Sprintf("%s%s = *(*[%d]uint64)(unsafe.Pointer(bp))\n", in, localFrame, words)
		}
		free := fmt.Sprintf("%stls.Free(%d)", in, n)
		var text string
		switch x := w.stmt.(type) {
		case *ast.ReturnStmt:
			if result == "" {
				problems = append(problems, fmt.Sprintf("%s: a wrapped return in a function without one named result", name))
				continue
			}
			text = lazyReturnMark + alloc + copyIn + in + result + " = " + f.text(x.Results[0]) + comment + "\n" + free + "\n" + in + "return"
		case *ast.IfStmt:
			text = lazyIfMark + alloc + copyIn + in + "lazyc = " + f.text(x.Cond) + "\n" + copyOut + free + "\n" + in + "if lazyc {"
		default:
			text = lazyStmtMark + alloc + copyIn + in + f.text(x) + comment + "\n" + copyOut + free
		}
		edits = append(edits, edit{start, end, text})
		site.wraps++
	}
	if len(problems) != 0 {
		return nil, site, nil, fmt.Errorf("%s", strings.Join(problems, "\n\t"))
	}
	out := f.apply(edits)
	if _, err := parseFunc(name, out); err != nil {
		return nil, site, nil, err
	}
	return out, site, outs, nil
}

// lineComment reports whether s, the rest of a line after a statement, is
// one comment.
func lineComment(s string) bool {
	if strings.HasPrefix(s, " //") {
		return true
	}
	return strings.HasPrefix(s, " /*") && strings.Index(s, "*/") == len(s)-2
}

// addrUse checks a use of bp in a wrapped statement that is not a cell:
// bp or bp + K must be an argument of a callee of addrCallees, and the
// result of libc.VaList an argument of another.
func addrUse(f *parsedFunc, st []ast.Node) string {
	k := len(st) - 1
	cell := ast.Expr(st[k].(*ast.Ident))
	if be, ok := st[k-1].(*ast.BinaryExpr); ok && be.X == cell {
		cell = be
		k--
	}
	if _, ok := frameCell(cell); !ok {
		return "bp in " + f.text(st[k-1])
	}
	call, ok := st[k-1].(*ast.CallExpr)
	if !ok {
		return "the frame address in " + f.text(st[k-1])
	}
	name := calleeName(f, call)
	if _, ok := addrCallees[name]; !ok {
		return "the frame address passed to " + name
	}
	if name == "libc.VaList" {
		outer, ok := st[k-2].(*ast.CallExpr)
		if !ok {
			return "the result of libc.VaList not passed to a callee"
		}
		on := calleeName(f, outer)
		if _, ok := addrCallees[on]; !ok || on == "libc.VaList" {
			return "the result of libc.VaList passed to " + on
		}
	}
	return ""
}

// calleeName names the callee of c: a function, pkg.Func, or for an
// indirect call through a field of a struct pointer, (*T).F.
func calleeName(f *parsedFunc, c *ast.CallExpr) string {
	switch fn := c.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return f.text(fn)
	}
	var via *ast.SelectorExpr
	ast.Inspect(c.Fun, func(n ast.Node) bool {
		if l, ok := n.(*ast.CompositeLit); ok && via == nil && len(l.Elts) == 1 {
			if s, ok := l.Elts[0].(*ast.SelectorExpr); ok {
				via = s
			}
		}
		return via == nil
	})
	if via == nil {
		return f.text(c.Fun)
	}
	// (*T)(unsafe.Pointer(x)).F
	if p, ok := via.X.(*ast.CallExpr); ok {
		if t, ok := ptrType(p.Fun, 1); ok {
			return "(*" + f.text(t) + ")." + via.Sel.Name
		}
	}
	return f.text(via)
}

// indentOf returns the leading white space of the line of s.
func indentOf(f *parsedFunc, s ast.Node) string {
	start, _ := f.line(s)
	line := f.src[start:]
	i := 0
	for i < len(line) && (line[i] == '\t' || line[i] == ' ') {
		i++
	}
	return string(line[:i])
}

// restoreLazy undoes fixLazyFrames: it gives the functions it changed
// their TLS frame back, as ccgo writes it with its defer, and drops the
// copies of the callees of outParams. It fails on a change that is not in
// the form fixLazyFrames writes.
func restoreLazy(b []byte) ([]byte, error) {
	for {
		m := reLazyClone.FindSubmatchIndex(b)
		if m == nil {
			break
		}
		orig, clone := string(b[m[2]:m[3]]), string(b[m[4]:m[5]])
		if op, ok := outParams[orig]; !ok || op.clone != clone {
			return nil, fmt.Errorf("%s is not the copy of %s of outParams", clone, orig)
		}
		end := bytes.Index(b[m[0]:], []byte("\n}\n"))
		if end < 0 {
			return nil, fmt.Errorf("end of %s not found", clone)
		}
		b = append(b[:m[0]:m[0]], b[m[0]+end+3:]...)
	}
	var out []byte
	for {
		m := reLazyHeadAt.FindIndex(b)
		if m == nil {
			break
		}
		start := bytes.LastIndex(b[:m[0]], []byte("\nfunc "))
		end := bytes.Index(b[m[0]:], []byte("\n}\n"))
		if start < 0 || end < 0 {
			return nil, fmt.Errorf("a function changed by fixLazyFrames not found")
		}
		end += m[0] + 3
		body, err := unlazyFunc(b[start:end])
		if err != nil {
			return nil, err
		}
		out = append(append(out, b[:start]...), body...)
		b = b[end:]
	}
	out = append(out, b...)
	for _, op := range outParams {
		if bytes.Contains(out, []byte(op.clone+"(")) {
			return nil, fmt.Errorf("%s left", op.clone)
		}
	}
	return out, nil
}

var (
	reLazyHeadAt = regexp.MustCompile(`\{\n\t// Changed by internal/fixlib: the TLS frame of \d+ bytes is a Go array, and each statement that needs a TLS frame takes one of its own \(see fixLazyFrames\)\.\n`)
	reLazyName   = regexp.MustCompile(`^\nfunc (\w+)\(`)
	reLazyResult = regexp.MustCompile(`^(\w+) = `)
	reLazyCell   = regexp.MustCompile(`unsafe\.Add\(unsafe\.Pointer\(&localbp\), (\d+)\)`)
)

// unlazyFunc undoes fixLazyFrames in one function, in the form funcBody
// gives.
func unlazyFunc(body []byte) ([]byte, error) {
	m := reLazyName.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("a function changed by fixLazyFrames without a name")
	}
	name := string(m[1])
	i := reLazyHeadAt.FindIndex(body)
	n, _ := lazyHead(body[i[0]:])
	size := strconv.Itoa(n)
	// The head.
	rest := body[i[1]:]
	decl := fmt.Sprintf("\tvar %s [%d]uint64\n", localFrame, (n+7)/8)
	if !bytes.HasPrefix(rest, []byte(decl)) {
		return nil, fmt.Errorf("%s: the Go array of the frame is not %q", name, decl)
	}
	rest = rest[len(decl):]
	for _, v := range []string{"\tvar bp uintptr\n", "\tvar lazyc bool\n"} {
		rest = bytes.TrimPrefix(rest, []byte(v))
	}
	head := fmt.Sprintf("{\n\tbp := tls.Alloc(%d)\n\tdefer tls.Free(%d)\n", n, n)
	b := append(append(append([]byte{}, body[:i[0]]...), head...), rest...)

	// The wrapped statements.
	var out []byte
	for {
		p, mark := -1, ""
		for _, mk := range []string{lazyStmtMark, lazyReturnMark, lazyIfMark} {
			if j := bytes.Index(b, []byte(mk)); j >= 0 && (p < 0 || j < p) {
				p, mark = j, mk
			}
		}
		if p < 0 {
			break
		}
		ls := bytes.LastIndexByte(b[:p], '\n') + 1
		in := string(b[ls:p])
		if in == "" || strings.Trim(in, "\t") != "" {
			return nil, fmt.Errorf("%s: a mark of fixLazyFrames not on a line of its own", name)
		}
		what := map[string]string{lazyStmtMark: "a statement", lazyReturnMark: "a return", lazyIfMark: "an if"}[mark]
		rest := b[p+len(mark):]
		alloc := in + "tls.Alloc(" + size + ")\n"
		copyIn, copyOut := "", ""
		switch {
		case bytes.HasPrefix(rest, []byte(alloc)):
		case bytes.HasPrefix(rest, []byte(in+"bp = tls.Alloc("+size+")\n")):
			alloc = in + "bp = tls.Alloc(" + size + ")\n"
			words := strconv.Itoa((n + 7) / 8)
			copyIn = in + "*(*[" + words + "]uint64)(unsafe.Pointer(bp)) = " + localFrame + "\n"
			copyOut = in + localFrame + " = *(*[" + words + "]uint64)(unsafe.Pointer(bp))\n"
		default:
			return nil, fmt.Errorf("%s: %s marked by fixLazyFrames without its tls.Alloc(%d)", name, what, n)
		}
		rest = rest[len(alloc):]
		if !bytes.HasPrefix(rest, []byte(copyIn+in)) {
			return nil, fmt.Errorf("%s: %s marked by fixLazyFrames without its copy into the TLS frame", name, what)
		}
		rest = rest[len(copyIn)+len(in):]
		free := in + "tls.Free(" + size + ")"
		var tail, was string
		switch mark {
		case lazyStmtMark:
			tail = "\n" + copyOut + free
		case lazyReturnMark:
			r := reLazyResult.Find(rest)
			if r == nil {
				return nil, fmt.Errorf("%s: a return marked by fixLazyFrames without r = X", name)
			}
			rest = rest[len(r):]
			tail = "\n" + free + "\n" + in + "return"
			was = "return "
		case lazyIfMark:
			if !bytes.HasPrefix(rest, []byte("lazyc = ")) {
				return nil, fmt.Errorf("%s: an if marked by fixLazyFrames without lazyc = X", name)
			}
			rest = rest[len("lazyc = "):]
			tail = "\n" + copyOut + free + "\n" + in + "if lazyc {"
			was = "if "
		}
		j := bytes.Index(rest, []byte(tail+"\n"))
		if j < 0 {
			return nil, fmt.Errorf("%s: %s marked by fixLazyFrames without its tls.Free(%d)", name, what, n)
		}
		stmt := was + string(rest[:j])
		if mark == lazyIfMark {
			stmt += " {"
		}
		out = append(append(out, b[:p]...), stmt...)
		b = rest[j+len(tail):]
	}
	b = append(out, b...)

	// The out parameters, then the cells.
	for orig, op := range outParams {
		re := regexp.MustCompile(regexp.QuoteMeta(op.clone+"(tls, (*"+op.elem+")(") + `(?:unsafe\.Add\(unsafe\.Pointer\(&localbp\), (\d+)\)|unsafe\.Pointer\(&localbp\))\)`)
		b = re.ReplaceAllFunc(b, func(s []byte) []byte {
			k := re.FindSubmatch(s)[1]
			if k == nil {
				return []byte(orig + "(tls, bp")
			}
			return []byte(orig + "(tls, bp+" + string(k))
		})
	}
	b = reLazyCell.ReplaceAll(b, []byte("unsafe.Pointer(bp + $1)"))
	b = bytes.ReplaceAll(b, []byte("unsafe.Pointer(&"+localFrame+")"), []byte("unsafe.Pointer(bp)"))
	for _, left := range []string{localFrame, "lazyc", "fixLazyFrames"} {
		if bytes.Contains(b, []byte(left)) {
			return nil, fmt.Errorf("%s: %s left after undoing fixLazyFrames", name, left)
		}
	}
	return b, nil
}
