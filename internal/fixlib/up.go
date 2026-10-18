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
	"sort"
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
// C does not fix that order either, and no statement in lib depends on it.
// The definition of __ccgo_up stays, unused. Any other use of __ccgo_up, or
// a definition that does something else, stops fixlib. The changes before
// it accept both forms, and the ones after it only the new one.

const upDefinition = "func __ccgo_up(n uintptr) unsafe.Pointer {\n\treturn unsafe.Pointer(&n)\n}"

// upConv is one *(**T)(__ccgo_up(x)) in the source: the offsets of the
// whole expression, of T and of x.
type upConv struct {
	start, end       int
	typStart, typEnd int
	argStart, argEnd int
}

func fixUp(b []byte) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", b, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
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
				return nil, fmt.Errorf("__ccgo_up changed: %s", b[off(fd.Pos()):off(fd.End())])
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
			})
			return true
		})
	}
	if defs != 1 {
		return nil, fmt.Errorf("want 1 definition of __ccgo_up, found %d", defs)
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
		return nil, bad
	}
	if len(convs) == 0 {
		return b, nil
	}

	// ast.Inspect visits an expression before the ones inside it, so convs
	// is sorted by start, and a conversion inside another comes after it.
	sort.SliceStable(convs, func(i, j int) bool { return convs[i].start < convs[j].start })
	var out bytes.Buffer
	var write func(start, end int, i int) int
	// write copies b[start:end] with the conversions from convs[i] on that
	// lie inside it rewritten, and returns the index of the first
	// conversion after it.
	write = func(start, end int, i int) int {
		pos := start
		for i < len(convs) && convs[i].start < end {
			c := convs[i]
			out.Write(b[pos:c.start])
			out.WriteString("(*")
			out.Write(b[c.typStart:c.typEnd])
			out.WriteString(")(unsafe.Pointer(")
			i = write(c.argStart, c.argEnd, i+1)
			out.WriteString("))")
			pos = c.end
		}
		out.Write(b[pos:end])
		return i
	}
	write(0, len(b), 0)
	return out.Bytes(), nil
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
