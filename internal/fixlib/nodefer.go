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
	"sort"
	"strconv"
	"strings"
)

// A function with a TLS frame frees it with a defer: ccgo writes
//
//	bp := tls.Alloc(N)
//	defer tls.Free(N)
//
// at its entry, and fixAlloca writes a deferred closure that frees the
// frame and a large alloca's malloc in the three call functions that have a
// frame. Go does not inline a function with a defer, so the hundreds of
// small engine functions with a frame stay calls, and PGO cannot inline
// them either. fixNoDefer drops each of these defers and frees the frame
// before every return instead:
//
//	return            ->  tls.Free(N); return
//	return r          ->  tls.Free(N); return r
//	return X          ->  r = X; tls.Free(N); return
//
// X is evaluated before the frame is freed, as with the defer, and a void
// function that can fall off its end frees there too. The defer also ran
// when a Go panic went through the function, and the free does not, so the
// TLS stack keeps the frames of a panic that went through the engine. The
// Go API puts the stack back where it was when the panic leaves the engine
// (see tlsMark in the repository root).
//
// fixNoDefer is the last change fixEngine makes, and restoreDefer, the
// first, undoes it exactly, so every other change and check sees the frames
// as ccgo writes them. fixEngine checks that restoreDefer gives back what
// fixNoDefer was given.

// noDeferMark takes the place of the defer.
const noDeferMark = "// Changed by internal/fixlib: the frame is freed before each return, not by a defer (see fixNoDefer).\n"

// keptDefers are the defers fixNoDefer leaves, by function: the malloc of
// an alloca in a function without a TLS frame (see fixAlloca), which is not
// on the TLS stack, so running it in a panic leaves that stack as it is.
var keptDefers = map[string]string{
	"_js_c_function_data_call": "defer libc.Xfree(tls, arg_buf)",
	"_js_call_bound_function":  "defer libc.Xfree(tls, arg_buf)", // 32-bit targets
}

// allocaDefer is the deferred closure fixAlloca writes in a function with a
// frame, at the indent of the body.
var allocaDefer = fmt.Sprintf("defer func() {\n\t\tif %[2]s != 0 {\n\t\t\tlibc.Xfree(tls, %[2]s)\n\t\t}\n\t\ttls.Free(%[1]s)\n\t}()", allocaFrame, allocaMalloc)

// frameFree is how one function frees its frame.
type frameFree struct {
	name   string
	result string // the name of the result, or "" for a void function
	first  int    // the index of the first statement of the frame's head
	at     int    // the index of the defer, or of the statement after the mark
	defer_ string // the text of the defer, at the indent of the body
	free   []string
	alloca bool
}

// freeText is the free at the indent in.
func (fr *frameFree) freeText(in string) string {
	var b strings.Builder
	for i, s := range fr.free {
		if i > 0 {
			b.WriteString("\n" + in)
		}
		b.WriteString(strings.ReplaceAll(s, "\n", "\n"+in))
	}
	return b.String()
}

// frameOf returns how the function d frees its frame, or nil when it has
// none. marked selects the form: the defer as written (false), or the mark
// fixNoDefer leaves in its place (true). src is the text d was parsed from.
//
// The frame is allocated by the first statement of the body that is
// X := tls.Alloc(N), or the head fixAlloca writes. The statements before it
// (fixDefine puts a call there) may return, without a frame to free, but
// must not allocate one or have a label a later goto could go back to.
func frameOf(src []byte, fset *token.FileSet, d *ast.FuncDecl, marked bool) (*frameFree, error) {
	name := d.Name.Name
	list := d.Body.List
	text := func(n ast.Node) string {
		return string(src[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
	}
	fr := &frameFree{name: name}
	found := false
	for i := range list {
		switch {
		case isFrameAlloc(list[i]) != "":
			n := isFrameAlloc(list[i])
			fr.first, fr.at = i, i+1
			fr.free = []string{"tls.Free(" + n + ")"}
			fr.defer_ = "defer " + fr.free[0]
		case strings.HasPrefix(text(list[i]), allocaFrame+" := ") && i+2 < len(list) && text(list[i+1]) == "var "+allocaMalloc+" uintptr" && text(list[i+2]) == "bp := tls.Alloc("+allocaFrame+")":
			fr.first, fr.at = i, i+3
			fr.alloca = true
			fr.defer_ = allocaDefer
			fr.free = []string{fmt.Sprintf("if %[1]s != 0 {\n\tlibc.Xfree(tls, %[1]s)\n}", allocaMalloc), "tls.Free(" + allocaFrame + ")"}
		default:
			continue
		}
		found = true
		break
	}
	if !found {
		return nil, nil
	}
	for _, s := range list[:fr.first] {
		bad := ""
		ast.Inspect(s, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.LabeledStmt:
				bad = "a label"
			case *ast.CallExpr:
				if _, ok := tlsCall(x, "Alloc"); ok {
					bad = "a tls.Alloc"
				}
			case *ast.FuncLit:
				bad = "a function literal"
			}
			return bad == ""
		})
		if bad != "" {
			return nil, fmt.Errorf("%s: %s before the frame is allocated", name, bad)
		}
	}
	if res := d.Type.Results; res != nil {
		if len(res.List) != 1 || len(res.List[0].Names) != 1 {
			return nil, fmt.Errorf("%s: a result that is not one named value", name)
		}
		fr.result = res.List[0].Names[0].Name
	}
	// The mark is a comment, on the line after the head.
	markAt := fset.Position(list[fr.at-1].End()).Offset
	isMarked := bytes.HasPrefix(src[markAt:], []byte("\n\t"+noDeferMark))
	if marked {
		if !isMarked {
			return nil, nil
		}
		return fr, nil
	}
	if fr.at < len(list) {
		if d, ok := list[fr.at].(*ast.DeferStmt); ok && text(d) == fr.defer_ {
			return fr, nil
		}
	}
	return nil, fmt.Errorf("%s: a TLS frame that is not freed by %q right after it is allocated", name, fr.defer_)
}

// isFrameAlloc returns N for X := tls.Alloc(N) with a literal N, or "".
func isFrameAlloc(s ast.Stmt) string {
	a, ok := s.(*ast.AssignStmt)
	if !ok || a.Tok != token.DEFINE || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
		return ""
	}
	if _, ok := a.Lhs[0].(*ast.Ident); !ok {
		return ""
	}
	args, ok := tlsCall(a.Rhs[0], "Alloc")
	if !ok || len(args) != 1 {
		return ""
	}
	l, ok := args[0].(*ast.BasicLit)
	if !ok || l.Kind != token.INT {
		return ""
	}
	return l.Value
}

// stmtList is a statement in its list.
type stmtList struct {
	list []ast.Stmt
	i    int
}

// returns returns every return statement of list[from:] with the list it
// is in (a labeled return counts as the labeled statement), not looking
// into skip. It fails on a function literal, whose returns are not those of
// the function.
func returns(name string, list []ast.Stmt, from int, skip ast.Node) ([]stmtList, error) {
	var out []stmtList
	var err error
	total := 0
	visit := func(n ast.Node) bool {
		if err != nil || (skip != nil && n == skip) {
			return false
		}
		var inner []ast.Stmt
		switch x := n.(type) {
		case *ast.FuncLit:
			err = fmt.Errorf("%s: a function literal", name)
			return false
		case *ast.ReturnStmt:
			total++
		case *ast.BlockStmt:
			inner = x.List
		case *ast.CaseClause:
			inner = x.Body
		case *ast.CommClause:
			inner = x.Body
		}
		for i, s := range inner {
			if _, ok := unlabel(s).(*ast.ReturnStmt); ok {
				out = append(out, stmtList{inner, i})
			}
		}
		return true
	}
	for i := from; i < len(list); i++ {
		if _, ok := unlabel(list[i]).(*ast.ReturnStmt); ok {
			out = append(out, stmtList{list, i})
		}
		ast.Inspect(list[i], visit)
	}
	if err == nil && total != len(out) {
		err = fmt.Errorf("%s: %d returns, %d in a statement list", name, total, len(out))
	}
	return out, err
}

// unlabel returns the statement a label is on, or s.
func unlabel(s ast.Stmt) ast.Stmt {
	for {
		l, ok := s.(*ast.LabeledStmt)
		if !ok {
			return s
		}
		s = l.Stmt
	}
}

// declares reports whether list declares name, which would hide the
// result an assignment of fixNoDefer sets.
func declares(list []ast.Stmt, name string) bool {
	found := false
	for _, s := range list {
		ast.Inspect(s, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ValueSpec:
				for _, id := range x.Names {
					found = found || id.Name == name
				}
			case *ast.AssignStmt:
				if x.Tok == token.DEFINE {
					for _, l := range x.Lhs {
						found = found || isIdent(l, name)
					}
				}
			case *ast.RangeStmt:
				if x.Tok == token.DEFINE {
					found = found || isIdent(x.Key, name) || isIdent(x.Value, name)
				}
			}
			return !found
		})
	}
	return found
}

// fileFuncs parses b and returns its functions in order.
func fileFuncs(b []byte) (*token.FileSet, []*ast.FuncDecl, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", b, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	var out []*ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
			out = append(out, fd)
		}
	}
	return fset, out, nil
}

// applyEdits makes the edits of b, which must not overlap.
func applyEdits(b []byte, edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out bytes.Buffer
	last := 0
	for _, e := range edits {
		out.Write(b[last:e.start])
		out.WriteString(e.text)
		last = e.end
	}
	out.Write(b[last:])
	return out.Bytes()
}

// indentAt returns the tabs that start the line of off.
func indentAt(b []byte, off int) (string, error) {
	ls := bytes.LastIndexByte(b[:off], '\n') + 1
	in := string(b[ls:off])
	if strings.Trim(in, "\t") != "" {
		return "", fmt.Errorf("a statement that does not start its line")
	}
	return in, nil
}

// fixNoDefer frees the TLS frame of every function before each of its
// returns instead of in a defer. The input has every defer (restoreDefer
// gives them back). It fails on a frame freed some other way, a defer that
// is neither a frame's nor in keptDefers, a tls.Alloc that is not a frame's,
// and a function whose returns it could not rewrite safely: one with a
// function literal, a bare return with a result, or a local of the name of
// the result.
func fixNoDefer(b []byte) ([]byte, error) {
	if bytes.Contains(b, []byte(noDeferMark)) {
		return nil, fmt.Errorf("fixNoDefer: the file has its marks already, restoreDefer goes first")
	}
	fset, funcs, err := fileFuncs(b)
	if err != nil {
		return nil, err
	}
	pos := func(p token.Pos) int { return fset.Position(p).Offset }
	var edits []edit
	var problems []string
	for _, d := range funcs {
		name := d.Name.Name
		if n, ok := lazyHead(b[pos(d.Body.Lbrace):pos(d.Body.End())]); ok {
			// fixLazyFrames frees each frame right after its statement.
			if err := checkLazyFrames(b, fset, d, n); err != nil {
				problems = append(problems, err.Error())
			}
			continue
		}
		fr, err := frameOf(b, fset, d, false)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if err := checkDefers(b, fset, d, fr); err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if fr == nil {
			continue
		}
		list := d.Body.List
		rets, err := returns(name, list, fr.at, list[fr.at])
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if fr.result != "" && declares(list[fr.at:], fr.result) {
			problems = append(problems, fmt.Sprintf("%s: a local named %s, as the result", name, fr.result))
			continue
		}
		// The defer becomes the mark.
		ds, de := pos(list[fr.at].Pos()), pos(list[fr.at].End())
		edits = append(edits, edit{ds, de, strings.TrimSuffix(noDeferMark, "\n")})
		bad := ""
		for _, r := range rets {
			ret := unlabel(r.list[r.i]).(*ast.ReturnStmt)
			in, err := indentAt(b, pos(ret.Pos()))
			if err != nil {
				bad = fmt.Sprintf("%s: %v", name, err)
				break
			}
			free := fr.freeText(in)
			var text string
			switch {
			case fr.result == "" && len(ret.Results) == 0:
				text = free + "\n" + in + "return"
			case fr.result == "":
				bad = fmt.Sprintf("%s: a return with a value in a function without a result", name)
			case len(ret.Results) != 1:
				bad = fmt.Sprintf("%s: a return without its one value", name)
			case isIdent(ret.Results[0], fr.result):
				text = free + "\n" + in + "return " + fr.result
			default:
				x := b[pos(ret.Results[0].Pos()):pos(ret.Results[0].End())]
				text = fr.result + " = " + string(x) + "\n" + in + free + "\n" + in + "return"
			}
			if bad != "" {
				break
			}
			edits = append(edits, edit{pos(ret.Pos()), pos(ret.End()), text})
		}
		if bad != "" {
			problems = append(problems, bad)
			continue
		}
		if fr.result == "" && fallsOff(list) {
			end := pos(d.Body.Rbrace)
			edits = append(edits, edit{end, end, "\t" + fr.freeText("\t") + "\n"})
		}
	}
	if len(problems) != 0 {
		return nil, fmt.Errorf("fixNoDefer: read each function, then teach fixNoDefer or keptDefers:\n\t%s", strings.Join(problems, "\n\t"))
	}
	return applyEdits(b, edits), nil
}

// checkDefers checks that the defers of d are the one fr drops and those of
// keptDefers, and that each tls.Alloc of d is the frame of fr or one that
// fixAlloca makes again (bp = tls.Alloc(frame_size)).
func checkDefers(b []byte, fset *token.FileSet, d *ast.FuncDecl, fr *frameFree) error {
	name := d.Name.Name
	text := func(n ast.Node) string {
		return string(b[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
	}
	var problem string
	ast.Inspect(d.Body, func(n ast.Node) bool {
		if problem != "" {
			return false
		}
		switch x := n.(type) {
		case *ast.DeferStmt:
			t := text(x)
			if fr != nil && x == d.Body.List[fr.at] {
				return false
			}
			if keptDefers[name] != t {
				problem = fmt.Sprintf("%s: %s", name, firstLine(t))
			}
			return false
		case *ast.AssignStmt:
			if len(x.Rhs) == 1 {
				if _, ok := tlsCall(x.Rhs[0], "Alloc"); ok {
					t := text(x)
					head := fr != nil && x == d.Body.List[fr.at-1]
					again := fr != nil && fr.alloca && t == "bp = tls.Alloc("+allocaFrame+")"
					if !head && !again {
						problem = fmt.Sprintf("%s: %s is not the frame", name, t)
					}
				}
			}
		case *ast.CallExpr:
			if _, ok := tlsCall(x, "Alloc"); ok {
				// Only as the right side of an assignment, seen above.
				return false
			}
		}
		return true
	})
	if problem != "" {
		return fmt.Errorf("%s", problem)
	}
	// Every tls.Alloc is the right side of an assignment checked above,
	// and the only tls.Free outside the defer frees the frame that
	// fixAlloca allocates again, right before.
	allocs, assigned, again, frees := 0, 0, 0, 0
	ast.Inspect(d.Body, func(n ast.Node) bool {
		if fr != nil && n == d.Body.List[fr.at] {
			return false
		}
		switch x := n.(type) {
		case *ast.CallExpr:
			if _, ok := tlsCall(x, "Alloc"); ok {
				allocs++
			}
			if _, ok := tlsCall(x, "Free"); ok {
				frees++
				if !(fr != nil && fr.alloca && text(x) == "tls.Free("+allocaFrame+")") {
					problem = fmt.Sprintf("%s: %s outside the defer", name, text(x))
				}
			}
		case *ast.AssignStmt:
			if len(x.Rhs) == 1 {
				if _, ok := tlsCall(x.Rhs[0], "Alloc"); ok {
					assigned++
					if text(x) == "bp = tls.Alloc("+allocaFrame+")" {
						again++
					}
				}
			}
		}
		return true
	})
	switch {
	case problem != "":
		return fmt.Errorf("%s", problem)
	case allocs != assigned:
		return fmt.Errorf("%s: a tls.Alloc that is not assigned to bp", name)
	case frees != again:
		return fmt.Errorf("%s: %d tls.Free outside the defer, %d frames allocated again", name, frees, again)
	}
	return nil
}

// checkLazyFrames checks a function fixLazyFrames changed, whose TLS frame
// was n bytes: it has no defer, and each statement fixLazyFrames marked
// takes a frame of n bytes, as bp = tls.Alloc(n) or tls.Alloc(n), and frees
// it with tls.Free(n).
func checkLazyFrames(b []byte, fset *token.FileSet, d *ast.FuncDecl, n int) error {
	name := d.Name.Name
	pos := func(p token.Pos) int { return fset.Position(p).Offset }
	text := func(x ast.Node) string { return string(b[pos(x.Pos()):pos(x.End())]) }
	body := b[pos(d.Body.Lbrace):pos(d.Body.End())]
	size := strconv.Itoa(n)
	allocs, frees := 0, 0
	var problem string
	ast.Inspect(d.Body, func(x ast.Node) bool {
		if problem != "" {
			return false
		}
		switch x := x.(type) {
		case *ast.DeferStmt:
			problem = fmt.Sprintf("%s: %s", name, firstLine(text(x)))
		case *ast.AssignStmt:
			if len(x.Rhs) == 1 {
				if _, ok := tlsCall(x.Rhs[0], "Alloc"); ok {
					if x.Tok != token.ASSIGN || len(x.Lhs) != 1 || !isIdent(x.Lhs[0], "bp") {
						problem = fmt.Sprintf("%s: %s is not bp = tls.Alloc(%d)", name, text(x), n)
					}
				}
			}
		case *ast.CallExpr:
			for _, f := range []string{"Alloc", "Free"} {
				if args, ok := tlsCall(x, f); ok {
					if len(args) != 1 || text(args[0]) != size {
						problem = fmt.Sprintf("%s: %s in a function whose TLS frame is %d bytes", name, text(x), n)
					} else if f == "Alloc" {
						allocs++
					} else {
						frees++
					}
				}
			}
		}
		return true
	})
	if problem != "" {
		return fmt.Errorf("%s", problem)
	}
	marks := 0
	for _, m := range []string{lazyStmtMark, lazyReturnMark, lazyIfMark} {
		marks += bytes.Count(body, []byte(m))
	}
	if allocs != marks || frees != marks {
		return fmt.Errorf("%s: %d tls.Alloc and %d tls.Free for %d statements marked", name, allocs, frees, marks)
	}
	return nil
}

// fallsOff reports whether a void function whose body is list can reach
// its end: its last statement is not a return or a goto.
func fallsOff(list []ast.Stmt) bool {
	if len(list) == 0 {
		return true
	}
	switch s := unlabel(list[len(list)-1]).(type) {
	case *ast.ReturnStmt:
		return false
	case *ast.BranchStmt:
		return s.Tok != token.GOTO
	}
	return true
}

// restoreDefer undoes fixNoDefer: it puts back each defer and every return
// as it was. It fails on a function with the mark whose returns are not as
// fixNoDefer writes them. A file without the mark comes back as it is.
func restoreDefer(b []byte) ([]byte, error) {
	if !bytes.Contains(b, []byte(noDeferMark)) {
		return b, nil
	}
	fset, funcs, err := fileFuncs(b)
	if err != nil {
		return nil, err
	}
	pos := func(p token.Pos) int { return fset.Position(p).Offset }
	text := func(n ast.Node) string { return string(b[pos(n.Pos()):pos(n.End())]) }
	var edits []edit
	var problems []string
	marks := 0
	for _, d := range funcs {
		name := d.Name.Name
		fr, err := frameOf(b, fset, d, true)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if fr == nil {
			continue
		}
		marks++
		list := d.Body.List
		// The mark goes back to the defer.
		ms := pos(list[fr.at-1].End()) + 2
		edits = append(edits, edit{ms, ms + len(noDeferMark) - 1, fr.defer_})
		rets, err := returns(name, list, fr.at, nil)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		// isFree reports whether list[i:] starts with the free, at the
		// indent of list[i].
		isFree := func(list []ast.Stmt, i int) bool {
			if i < 0 || i+len(fr.free) > len(list) {
				return false
			}
			for k, s := range fr.free {
				st := unlabel(list[i+k])
				if k > 0 && st != list[i+k] {
					return false // a label inside the free
				}
				in, err := indentAt(b, pos(st.Pos()))
				if err != nil || text(st) != strings.ReplaceAll(s, "\n", "\n"+in) {
					return false
				}
			}
			return true
		}
		bad := ""
		for _, r := range rets {
			ret := unlabel(r.list[r.i]).(*ast.ReturnStmt)
			f0 := r.i - len(fr.free)
			if r.list[r.i] != ast.Stmt(ret) || !isFree(r.list, f0) {
				bad = fmt.Sprintf("%s: a return at offset %d without the free before it", name, pos(ret.Pos()))
				break
			}
			start := pos(unlabel(r.list[f0]).Pos())
			var back string
			switch {
			case fr.result == "" && len(ret.Results) == 0:
				back = "return"
			case fr.result == "":
				bad = fmt.Sprintf("%s: a return with a value in a function without a result", name)
			case len(ret.Results) == 1 && isIdent(ret.Results[0], fr.result):
				back = "return " + fr.result
			case len(ret.Results) == 0:
				var a *ast.AssignStmt
				ok := false
				if f0 >= 1 {
					a, ok = unlabel(r.list[f0-1]).(*ast.AssignStmt)
				}
				if !ok || a.Tok != token.ASSIGN || len(a.Lhs) != 1 || len(a.Rhs) != 1 || !isIdent(a.Lhs[0], fr.result) {
					bad = fmt.Sprintf("%s: a bare return without %s = X before the free", name, fr.result)
					break
				}
				start = pos(a.Pos())
				back = "return " + text(a.Rhs[0])
			default:
				bad = fmt.Sprintf("%s: a return fixNoDefer does not write", name)
			}
			if bad != "" {
				break
			}
			edits = append(edits, edit{start, pos(ret.End()), back})
		}
		if bad != "" {
			problems = append(problems, bad)
			continue
		}
		if fr.result == "" && len(list)-len(fr.free) >= fr.at && isFree(list, len(list)-len(fr.free)) {
			s := pos(unlabel(list[len(list)-len(fr.free)]).Pos())
			ls := bytes.LastIndexByte(b[:s], '\n') + 1
			end := pos(d.Body.Rbrace)
			edits = append(edits, edit{ls, end, ""})
		}
	}
	if len(problems) != 0 {
		return nil, fmt.Errorf("restoreDefer: these functions are not as fixNoDefer leaves them:\n\t%s", strings.Join(problems, "\n\t"))
	}
	if n := bytes.Count(b, []byte(noDeferMark)); n != marks {
		return nil, fmt.Errorf("restoreDefer: %d marks of fixNoDefer, %d where it puts them", n, marks)
	}
	return applyEdits(b, edits), nil
}

// dropDefers is the last change of fixEngine: fixNoDefer, checked against
// restoreDefer, which must give b back byte for byte.
func dropDefers(b []byte) ([]byte, error) {
	out, err := fixNoDefer(b)
	if err != nil {
		return nil, err
	}
	back, err := restoreDefer(out)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("restoreDefer does not undo fixNoDefer")
	}
	if n := bytes.Count(out, []byte("defer tls.Free(")); n != 0 {
		return nil, fmt.Errorf("fixNoDefer left %d defer tls.Free", n)
	}
	return out, nil
}

// readEngine reads a generated file of lib/ as every change but fixNoDefer
// leaves it, which is what the other changes take and check, and what the
// reports print.
func readEngine(fn string) ([]byte, error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	return restoreDefer(b)
}
