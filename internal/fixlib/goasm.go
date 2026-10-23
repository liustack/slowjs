// Copyright (c) 2026 The slowjs Authors.
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
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var goasmFunction = regexp.MustCompile(`(?s)func _JS_CallInternal\([^\n]+\{\n.*?\n}`)

const goasmMark = "\t// Changed by internal/fixlib: optional LLVM interpreter (see fixGoasm).\n\tif goasmEnabled {\n\t\treturn goasmCall(tls, caller_ctx, _func_obj, this_obj1, new_target, argc, argv, flags)\n\t}\n"

func checkGoasm(b []byte) error {
	f := goasmFunction.Find(b)
	if fmt.Sprintf("%x", sha256.Sum256(f)) != "b0b1d873b39201387090a1deb800ddcb4795d567b357498b39772f232d62ded2" {
		return fmt.Errorf("fixGoasm: interpreter fingerprint changed")
	}
	return nil
}
func fixGoasm(b []byte, name string) ([]byte, error) {
	if name != "ccgo_linux_amd64.go" {
		return b, nil
	}
	if err := checkGoasm(b); err != nil {
		return nil, err
	}
	i := goasmFunction.FindIndex(b)[0]
	i += bytes.IndexByte(b[i:], '\n') + 1
	out := append([]byte{}, b[:i]...)
	out = append(out, goasmMark...)
	out = append(out, b[i:]...)
	restored, err := restoreGoasm(out, name)
	if err != nil || !bytes.Equal(restored, b) {
		return nil, fmt.Errorf("fixGoasm: inverse mismatch: %v", err)
	}
	return out, nil
}
func restoreGoasm(b []byte, name string) ([]byte, error) {
	if name != "ccgo_linux_amd64.go" || !bytes.Contains(b, []byte(goasmMark)) {
		return b, nil
	}
	if bytes.Count(b, []byte(goasmMark)) != 1 {
		return nil, fmt.Errorf("restoreGoasm: marker count")
	}
	out := bytes.Replace(b, []byte(goasmMark), nil, 1)
	if err := checkGoasm(out); err != nil {
		return nil, err
	}
	return out, nil
}

// The generated interpreter of the goasm tag replaces the whole Go
// _JS_CallInternal, so the changes fixlib makes in that function do not reach
// it. Each file of fixlib that changes the interpreter says here what the
// generated one does instead, and TestGoasmCoversInterpreterRules fails when
// a new file has no row, so that the two interpreters cannot drift apart
// unnoticed (see docs/fork-changes/go-assembly.md).
type goasmKind int

const (
	// goasmCPatch: internal/goasm/cpatch.py writes the same fast path in C.
	// The row's text is the change's name, which cpatch.py must mention.
	goasmCPatch goasmKind = iota
	// goasmInC: the C the generator compiles already behaves this way.
	goasmInC
	// goasmGoOnly: the change works around Go's compiler or the TLS frames
	// of ccgo, and has no meaning for LLVM.
	goasmGoOnly
)

type goasmRule struct {
	kind goasmKind
	text string
}

var goasmInterpreterRules = map[string]goasmRule{
	"bitwise.go":     {goasmCPatch, "fixBitwise"},
	"eq.go":          {goasmCPatch, "fixEq"},
	"fieldchain.go":  {goasmCPatch, "fixFieldChain"},
	"main.go":        {goasmInC, "fixInterpreter: quickjs.diff guards the inline property reads against a pending stack in C"},
	"poll.go":        {goasmInC, "fixPoll makes Go count down like js_poll_interrupts, which the C inlines"},
	"slots.go":       {goasmInC, "auditSlots checks the Go code for the pending stack rules that quickjs.diff writes in C"},
	"slotsites.go":   {goasmInC, "the sites of auditSlots, see slots.go"},
	"uncatchable.go": {goasmInC, "quickjs.diff checks the uncatchable flag before a catch in C"},
	"alloca.go":      {goasmGoOnly, "the alloca room of the Go frame, allocas.cpp places the C alloca in the TLS slot"},
	"cells.go":       {goasmGoOnly, "keeps frame cells in Go locals"},
	"fastframes.go":  {goasmGoOnly, "fast TLS frames, which skip the interpreter"},
	"goasm.go":       {goasmGoOnly, "the switch to the generated interpreter itself"},
	"inline.go":      {goasmGoOnly, "inlines small callees, which LLVM does on its own"},
	"nodefer.go":     {goasmGoOnly, "drops the defers that free TLS frames"},
	"oneframe.go":    {goasmGoOnly, "one TLS frame for the whole Go interpreter"},
	"pin.go":         {goasmGoOnly, "keeps hot locals out of the Go frame"},
}

// goasmRuleFiles returns the files of the fixlib sources in dir that change
// _JS_CallInternal, with what shows it: code that names the function, a use
// of a package variable whose value names it, or a "(see fixX)" mark of a
// change the file defines in the interpreter of the generated file engine.
func goasmRuleFiles(dir, engine string) (map[string]string, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, fn := range names {
		if strings.HasSuffix(fn, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, fn, nil, 0)
		if err != nil {
			return nil, err
		}
		files[filepath.Base(fn)] = f
	}
	namesCall := func(n ast.Node) bool {
		found := false
		ast.Inspect(n, func(n ast.Node) bool {
			if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING && strings.Contains(l.Value, "_JS_CallInternal") {
				found = true
			}
			return !found
		})
		return found
	}
	vars := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				for _, s := range g.Specs {
					v := s.(*ast.ValueSpec)
					if namesCall(v) {
						for _, n := range v.Names {
							vars[n.Name] = true
						}
					}
				}
			}
		}
	}
	found := map[string]string{}
	defines := map[string]string{}
	for name, f := range files {
		if namesCall(f) {
			found[name] = "names _JS_CallInternal"
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && vars[id.Name] && id.Obj == nil && found[name] == "" {
				found[name] = "uses " + id.Name
			}
			return true
		})
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				defines[fd.Name.Name] = name
			}
		}
	}
	b, err := os.ReadFile(engine)
	if err != nil {
		return nil, err
	}
	body := goasmFunction.Find(b)
	if body == nil {
		return nil, fmt.Errorf("%s: no _JS_CallInternal", engine)
	}
	for _, m := range regexp.MustCompile(`\(see (fix\w+)\)`).FindAllSubmatch(body, -1) {
		change := string(m[1])
		name, ok := defines[change]
		if !ok {
			return nil, fmt.Errorf("the interpreter names %s, which fixlib does not define", change)
		}
		if found[name] == "" {
			found[name] = "left (see " + change + ") in the interpreter"
		}
	}
	return found, nil
}

// goasmRuleProblems compares the files that change the interpreter, with
// what shows it, to the rows of rules, and returns one line per problem.
// cpatch is the text of internal/goasm/cpatch.py.
func goasmRuleProblems(found map[string]string, rules map[string]goasmRule, cpatch string) (r []string) {
	for name, why := range found {
		if _, ok := rules[name]; !ok {
			r = append(r, fmt.Sprintf("%s: changes _JS_CallInternal (%s) but has no row in goasmInterpreterRules", name, why))
		}
	}
	for name, rule := range rules {
		if _, ok := found[name]; !ok {
			r = append(r, name+": has a row in goasmInterpreterRules but no longer changes _JS_CallInternal")
		} else if rule.kind == goasmCPatch && !regexp.MustCompile(`\b`+regexp.QuoteMeta(rule.text)+`\b`).MatchString(cpatch) {
			r = append(r, fmt.Sprintf("%s: internal/goasm/cpatch.py does not name %s", name, rule.text))
		}
	}
	slices.Sort(r)
	return r
}
