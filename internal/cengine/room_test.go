// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package cengine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestCallbacksMakeRoom: every callback of the host that the generated code
// calls and that returns calls makeRoom as its last call, right before its
// only return, which returns names only. On windows the stack may shrink
// while a callback runs, and the generated code checks the room for the
// state of the CPU at a fault only where a function starts: the callback
// makes it again for the code it returns to. host_trap never returns.
func TestCallbacksMakeRoom(t *testing.T) {
	called := map[string]bool{}
	files, err := filepath.Glob("*_amd64.s")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`CALL ·(host_[A-Za-z0-9_]+)\(SB\)`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range call.FindAllSubmatch(b, -1) {
			called[string(m[1])] = true
		}
	}
	if !called["host_poll"] || !called["host_call"] {
		t.Fatalf("the generated code calls %v: no host_poll or host_call", called)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "host.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	defined := map[string]*ast.FuncDecl{}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
			defined[fd.Name.Name] = fd
		}
	}
	for name := range called {
		fd := defined[name]
		if fd == nil {
			t.Errorf("the generated code calls %s, which host.go does not define", name)
			continue
		}
		if name == "host_trap" {
			continue
		}
		if err := makesRoomLast(fd); err != "" {
			t.Errorf("%s: %s", name, err)
		}
	}
}

// makesRoomLast says what is wrong with fd, or "" when it ends with
// makeRoom() and then at most one return of names.
func makesRoomLast(fd *ast.FuncDecl) string {
	returns := 0
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			returns++
		}
		return true
	})
	list := fd.Body.List
	if r, ok := list[len(list)-1].(*ast.ReturnStmt); ok {
		for _, e := range r.Results {
			if _, ok := e.(*ast.Ident); !ok {
				return "its return computes a value after makeRoom"
			}
		}
		list = list[:len(list)-1]
		returns--
	}
	if returns != 0 {
		return "it returns before it makes room"
	}
	if len(list) == 0 {
		return "it does not call makeRoom"
	}
	es, ok := list[len(list)-1].(*ast.ExprStmt)
	if !ok {
		return "its last statement before the return is not makeRoom()"
	}
	if c, ok := es.X.(*ast.CallExpr); ok && len(c.Args) == 0 {
		if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "makeRoom" {
			return ""
		}
	}
	return "its last statement before the return is not makeRoom()"
}
