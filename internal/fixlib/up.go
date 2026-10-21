// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ccgo writes every use of an address held in a uintptr as
// *(**T)(__ccgo_up(x)), where __ccgo_up(n) returns &n: the address of a
// copy of x, read back as a *T. It is there only so that go vet does not
// report a conversion of a uintptr to unsafe.Pointer, and does not keep
// anything alive for the garbage collector: x points into C memory. Once
// inlined, each use stores x into an addressed stack slot and loads it
// back. On arm64 the compiler cannot forward that store to the load, and
// on every target the slots make the frames of the big functions much
// larger (JS_CallInternal on darwin/arm64: 7744 bytes, 944 without them).
//
// fixUp writes each of these as (*T)(unsafe.Pointer(x)), the same pointer,
// so **(**T)(__ccgo_up(x)) becomes *(*T)(unsafe.Pointer(x)). Without the
// call, Go no longer reads x before the other calls in the same statement.
// C does not fix that order either, and no statement in lib depends on it:
// each statement where the order goes away must be one of upSites, and a
// new one stops fixlib (see upOrderOf). The definition of __ccgo_up stays,
// unused. Any other use of __ccgo_up, or a definition that does something
// else, stops fixlib. The changes before it accept both forms, and the ones
// after it only the new one.

const upDefinition = "func __ccgo_up(n uintptr) unsafe.Pointer {\n\treturn unsafe.Pointer(&n)\n}"

// upConv is one *(**T)(__ccgo_up(x)) in the source: the offsets of the
// whole expression, of T and of x, and x.
type upConv struct {
	start, end       int
	typStart, typEnd int
	argStart, argEnd int
	arg              ast.Expr
}

func fixUp(b []byte) ([]byte, error) {
	fset, file, convs, err := upConvs(b)
	if err != nil {
		return nil, err
	}
	if len(convs) == 0 {
		return b, nil
	}
	// -up reports the statements instead (see upReport).
	if !*up {
		if err := checkUpOrder(upOrderOf(fset, file, b, convs)); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	upWrite(&out, b, convs, 0, len(b), 0)
	return out.Bytes(), nil
}

// upConvs parses b and returns its conversions through __ccgo_up, sorted
// by start, a conversion inside another after it. Any other use of
// __ccgo_up, or a definition that does something else, is an error.
func upConvs(b []byte) (*token.FileSet, *ast.File, []upConv, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", b, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, nil, err
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }

	var convs []upConv
	matched := map[*ast.Ident]bool{}
	defs := 0
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fd.Recv == nil && fd.Name.Name == "__ccgo_up" {
			defs++
			if string(b[off(fd.Pos()):off(fd.End())]) != upDefinition {
				return nil, nil, nil, fmt.Errorf("__ccgo_up changed: %s", b[off(fd.Pos()):off(fd.End())])
			}
			continue
		}
		ast.Inspect(fd, func(n ast.Node) bool {
			// *(**T)(__ccgo_up(x))
			s, ok := n.(*ast.StarExpr)
			if !ok {
				return true
			}
			c, ok := s.X.(*ast.CallExpr)
			if !ok || len(c.Args) != 1 {
				return true
			}
			typ, ok := ptrType(c.Fun, 2)
			if !ok {
				return true
			}
			inner, ok := c.Args[0].(*ast.CallExpr)
			if !ok || len(inner.Args) != 1 || !isIdent(inner.Fun, "__ccgo_up") {
				return true
			}
			matched[inner.Fun.(*ast.Ident)] = true
			convs = append(convs, upConv{
				off(s.Pos()), off(s.End()),
				off(typ.Pos()), off(typ.End()),
				off(inner.Args[0].Pos()), off(inner.Args[0].End()),
				inner.Args[0],
			})
			return true
		})
	}
	if defs != 1 {
		return nil, nil, nil, fmt.Errorf("want 1 definition of __ccgo_up, found %d", defs)
	}
	var bad error
	ast.Inspect(file, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if ok && id.Name == "__ccgo_up" && !matched[id] && bad == nil {
			if fd := funcAt(file, id.Pos()); fd == nil || fd.Name != id {
				bad = fmt.Errorf("%s: __ccgo_up used as something else than *(**T)(__ccgo_up(x))", fset.Position(id.Pos()))
			}
		}
		return bad == nil
	})
	if bad != nil {
		return nil, nil, nil, bad
	}
	// ast.Inspect visits an expression before the ones inside it, so convs
	// is sorted by start, and a conversion inside another comes after it.
	sort.SliceStable(convs, func(i, j int) bool { return convs[i].start < convs[j].start })
	return fset, file, convs, nil
}

// upWrite writes b[start:end] to out with the conversions from convs[i] on
// that lie inside it rewritten, and returns the index of the first
// conversion after it.
func upWrite(out *bytes.Buffer, b []byte, convs []upConv, start, end, i int) int {
	pos := start
	for i < len(convs) && convs[i].start < end {
		c := convs[i]
		out.Write(b[pos:c.start])
		out.WriteString("(*")
		out.Write(b[c.typStart:c.typEnd])
		out.WriteString(")(unsafe.Pointer(")
		i = upWrite(out, b, convs, c.argStart, c.argEnd, i+1)
		out.WriteString("))")
		pos = c.end
	}
	out.Write(b[pos:end])
	return i
}

// The order fixUp changes. Go evaluates the calls of a statement from left
// to right, and the argument x of __ccgo_up(x) before that call, so in
// *(**T)(__ccgo_up(x)) every read of memory in x comes before the calls to
// the right of it. Once written *(*T)(unsafe.Pointer(x)), the reads in x
// have no order with those calls. C gives none either, so code that relies
// on it is wrong in C too, but a statement such as s->buf[i] = f(s), where f
// moves buf, would then go from writing the old buf to writing either.
// upOrderOf finds each statement (or condition) where a conversion has a
// read in its address and a call to the right of that address, and names it
// by its function, its addresses with reads, in the form fixUp writes, and
// the functions it calls after them, with spaces and numbers made uniform,
// so that the 13 files give the same names. upSites lists them, reviewed:
// a new one stops fixlib.

// upPure matches the libc helpers that convert a number and touch no
// memory: a call of one cannot move what an address reads.
var upPure = regexp.MustCompile(`^(Bool(Int|Uint)\d+|(Int|Uint|Float|Uintptr)\d*From(Int|Uint|Float|Uintptr)\d*)$`)

var reUpNumber = regexp.MustCompile(`\b[0-9]+\b`)

// upNorm makes the spaces and the numbers of text uniform.
func upNorm(text string) string {
	return reUpNumber.ReplaceAllString(strings.Join(strings.Fields(text), " "), "N")
}

func upOrder(b []byte) (map[string]int, error) {
	fset, file, convs, err := upConvs(b)
	if err != nil {
		return nil, err
	}
	return upOrderOf(fset, file, b, convs), nil
}

func upOrderOf(fset *token.FileSet, file *ast.File, b []byte, convs []upConv) map[string]int {
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	text := func(n ast.Node) string { return string(b[off(n.Pos()):off(n.End())]) }
	sites := map[string]int{}
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		start, end := off(fd.Pos()), off(fd.End())
		first := sort.Search(len(convs), func(i int) bool { return convs[i].start >= start })
		if first == len(convs) || convs[first].start >= end {
			continue
		}
		locals := localNames(fd)
		for _, unit := range evalUnits(fd.Body) {
			us, ue := off(unit.Pos()), off(unit.End())
			var calls []*ast.CallExpr
			ast.Inspect(unit, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok && !isIdent(c.Fun, "__ccgo_up") && !upNotCall(c.Fun) {
					calls = append(calls, c)
				}
				return true
			})
			var reads []string
			after := map[string]bool{}
			for i := first; i < len(convs) && convs[i].start < end; i++ {
				c := convs[i]
				if c.start < us || c.end > ue || !upReads(c.arg, locals) {
					continue
				}
				hit := false
				for _, call := range calls {
					if off(call.Pos()) >= c.argEnd {
						hit = true
						after[text(call.Fun)] = true
					}
				}
				if hit {
					var x bytes.Buffer
					upWrite(&x, b, convs, c.argStart, c.argEnd, i+1)
					reads = append(reads, upNorm(x.String()))
				}
			}
			if len(reads) == 0 {
				continue
			}
			var names []string
			for name := range after {
				names = append(names, name)
			}
			sort.Strings(reads)
			sort.Strings(names)
			sites[fd.Name.Name+": "+strings.Join(reads, " ; ")+" -> "+strings.Join(names, " ")]++
		}
	}
	return sites
}

// checkUpOrder compares the statements upOrderOf found with upSites.
func checkUpOrder(sites map[string]int) error {
	var diffs []string
	for key, n := range sites {
		switch want, ok := upSites[key]; {
		case !ok:
			diffs = append(diffs, key+": not in upSites")
		case n != want:
			diffs = append(diffs, fmt.Sprintf("%s: %d times, upSites has %d", key, n, want))
		}
	}
	for key, want := range upSites {
		if _, ok := sites[key]; !ok {
			diffs = append(diffs, fmt.Sprintf("%s: 0 times, upSites has %d", key, want))
		}
	}
	if len(diffs) == 0 {
		return nil
	}
	sort.Strings(diffs)
	return fmt.Errorf("statements where fixUp drops the order of a read in an address and a later call differ from upSites: check that each call cannot change what the read reads, then update upSites (go run ./internal/fixlib -up):\n\t%s", strings.Join(diffs, "\n\t"))
}

// upNotCall reports whether fun, called, is a conversion or a libc helper
// that converts a number, which change no memory.
func upNotCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.ParenExpr, *ast.ArrayType, *ast.StarExpr, *ast.StructType:
		return true
	case *ast.Ident:
		switch f.Name {
		case "bool", "byte", "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "__ccgo_fp":
			return true
		}
		return strings.HasPrefix(f.Name, "T")
	case *ast.SelectorExpr:
		if isSelector(f, "unsafe", "Pointer") {
			return true
		}
		x, ok := f.X.(*ast.Ident)
		return ok && x.Name == "libc" && upPure.MatchString(f.Sel.Name)
	}
	return false
}

// upReads reports whether e reads memory: through a pointer, an index or a
// field, or a variable of the package, which is a C global.
func upReads(e ast.Expr, locals map[string]bool) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.StarExpr, *ast.IndexExpr:
			found = true
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); !ok || locals[x.Name] {
				found = true
			}
			return false
		case *ast.CallExpr:
			// The function of a call is not read: only its arguments.
			for _, a := range n.Args {
				found = found || upReads(a, locals)
			}
			return false
		case *ast.Ident:
			if !locals[n.Name] && (strings.HasPrefix(n.Name, "X") || strings.HasPrefix(n.Name, "_")) && n.Name != "_" && n.Name != "__ccgo_ts" {
				found = true
			}
		}
		return !found
	})
	return found
}

// localNames returns the parameters, results and local variables of fd.
func localNames(fd *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}
	add := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				names[n.Name] = true
			}
		}
	}
	add(fd.Type.Params)
	add(fd.Type.Results)
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ValueSpec:
			for _, id := range n.Names {
				names[id.Name] = true
			}
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				for _, l := range n.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						names[id.Name] = true
					}
				}
			}
		}
		return true
	})
	return names
}

// evalUnits returns the simple statements of body and the conditions and
// tags of its if, for and switch statements and case clauses: what Go
// evaluates as one expression, assignment or return.
func evalUnits(body *ast.BlockStmt) []ast.Node {
	var units []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.IfStmt:
			units = append(units, s.Cond)
		case *ast.ForStmt:
			if s.Cond != nil {
				units = append(units, s.Cond)
			}
		case *ast.SwitchStmt:
			if s.Tag != nil {
				units = append(units, s.Tag)
			}
		case *ast.CaseClause:
			for _, e := range s.List {
				units = append(units, e)
			}
		case *ast.BlockStmt, *ast.LabeledStmt, *ast.RangeStmt:
		case ast.Stmt:
			units = append(units, s)
			return false
		}
		return true
	})
	return units
}

// upReport prints upSites for the generated files as ccgo writes them,
// with the changes before fixUp made in memory. Files fixUp changed already
// have no conversion left to tell the statements by, so it needs the files
// as taken from upstream.
func upReport(files []string, hw []*ast.File) error {
	sort.Strings(files)
	var sites map[string]int
	for _, fn := range files {
		b, err := readEngine(fn)
		if err != nil {
			return err
		}
		if b, err = fixBeforeUp(b, hw, filepath.Base(fn)); err != nil {
			return fmt.Errorf("%s: %v", fn, err)
		}
		fset, file, convs, err := upConvs(b)
		if err != nil {
			return fmt.Errorf("%s: %v", fn, err)
		}
		if len(convs) == 0 {
			return fmt.Errorf("%s: no __ccgo_up left: -up needs the files as taken from upstream", fn)
		}
		got := upOrderOf(fset, file, b, convs)
		if sites == nil {
			sites = got
		} else if !maps.Equal(sites, got) {
			return fmt.Errorf("%s: other statements than in %s", fn, files[0])
		}
	}
	var keys []string
	for key := range sites {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Println("var upSites = map[string]int{")
	for _, key := range keys {
		fmt.Printf("\t%q: %d,\n", key, sites[key])
	}
	fmt.Println("}")
	return nil
}

// funcAt returns the function declaration whose name is at pos, or nil.
func funcAt(file *ast.File, pos token.Pos) *ast.FuncDecl {
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Pos() == pos {
			return fd
		}
	}
	return nil
}
