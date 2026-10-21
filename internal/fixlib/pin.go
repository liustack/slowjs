// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// JS_CallInternal is one function of a few thousand blocks, and every
// opcode handler ends with a jump back to the dispatch. Where those jumps
// meet, the register allocator of Go makes each of the about 200 handlers
// put back the registers of the values that live across the loop, mostly
// as loads from the stack frame, whether or not the handler used them. On
// amd64, with few registers, that is about three and a half loads per
// opcode.
//
// fixPin takes the address of the values in pinVars, which the loop keeps
// for the whole call: the compiler then leaves each in its stack slot and
// reads it where it is used, and the handlers have less to put back. The
// address goes nowhere, so nothing escapes and nothing else changes. It
// is done in the files of pinFiles only: the amd64 targets and windows,
// whose file serves windows/arm64 too, where it costs nothing. Elsewhere
// it was not measured. A name of pinVars that the function no longer has
// stops fixlib.

// pinFiles are the generated files fixPin changes.
var pinFiles = map[string]bool{
	"ccgo_darwin_amd64.go":  true,
	"ccgo_freebsd_amd64.go": true,
	"ccgo_linux_amd64.go":   true,
	"ccgo_windows.go":       true,
}

// pinVars are the parameters and locals of JS_CallInternal that fixPin
// keeps in the stack frame.
var pinVars = []string{
	"this_obj1", "new_target", "argc", "argv", "caller_ctx", "flags", // parameters
	"rt", "ctx10", "b1", "sf", // the runtime, the context, the bytecode and the frame
	"var_buf", "arg_buf", "var_refs", "local_buf", "stack_buf", // the frame's buffers
}

const pinInGo = " // Changed by internal/fixlib: kept in the stack frame (see fixPin).\n"

func fixPin(b []byte, listed bool) ([]byte, error) {
	if !listed {
		return b, nil
	}
	body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
	if err != nil {
		return nil, err
	}
	pin := "\t" + strings.Repeat("_, ", len(pinVars)-1) + "_ = &" + strings.Join(pinVars, ", &") + pinInGo
	if bytes.Contains(body, []byte(pin)) {
		return b, nil
	}
	if bytes.Contains(body, []byte(pinInGo)) {
		return nil, fmt.Errorf("_JS_CallInternal: other values kept in the stack frame than pinVars")
	}
	f, err := parseFunc("_JS_CallInternal", body)
	if err != nil {
		return nil, err
	}
	declared := map[string]bool{}
	for _, field := range f.decl.Type.Params.List {
		for _, n := range field.Names {
			declared[n.Name] = true
		}
	}
	for _, s := range f.decl.Body.List {
		d, ok := s.(*ast.DeclStmt)
		if !ok {
			continue
		}
		g, ok := d.Decl.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			continue
		}
		for _, spec := range g.Specs {
			for _, n := range spec.(*ast.ValueSpec).Names {
				declared[n.Name] = true
			}
		}
	}
	for _, v := range pinVars {
		if !declared[v] {
			return nil, fmt.Errorf("_JS_CallInternal: no %s to keep in the stack frame", v)
		}
	}
	// Right after the statement that uses every local, before any label.
	at, err := blankUse(f)
	if err != nil {
		return nil, err
	}
	at += bytes.IndexByte(f.src[at:], '\n') + 1
	return bytes.Replace(b, body, f.apply([]edit{{at, at, pin}}), 1), nil
}
