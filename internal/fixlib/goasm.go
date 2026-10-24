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
	"strconv"
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

// goasmHelperRules gives a row to each file of fixlib that changes the Go
// copy of a helper the generated interpreter compiles in from the C (see
// goasmHelperFiles), keyed "helper in file". The kinds are those of
// goasmInterpreterRules.
var goasmHelperRules = map[string]goasmRule{
	"__JS_AtomIsTaggedInt in fieldchain.go": {goasmInC, "fingerprints it for fieldProto and leaves it as ccgo makes it"},
	"find_own_property in define.go":        {goasmInC, "fingerprints it for objlitDefine and leaves it as ccgo makes it"},
	"find_own_property in lazy.go":          {goasmGoOnly, "calls a clone that takes a Go pointer, so its address stays out of the TLS frame"},
	"find_own_property in slots.go":         {goasmInC, "auditSlots reads its callers for the pending stack, which quickjs.diff guards in C"},
	"get_shape_prop in forin.go":            {goasmInC, "fingerprints it for the for-in fast path and leaves it as ccgo makes it"},
	"get_shape_prop in ownkeys.go":          {goasmInC, "fingerprints it for lib/ownkeys.go and leaves it as ccgo makes it"},
	"js_poll_interrupts in poll.go":         {goasmInC, "fingerprints it for lib/poll.go, which repeats it, and leaves it as ccgo makes it"},
	"js_rc in atomkey.go":                   {goasmInC, "fingerprints it for the atom key path and leaves it as ccgo makes it"},
	"js_rc in regexcache.go":                {goasmInC, "fingerprints it for the regexp cache and leaves it as ccgo makes it"},
	"js_rc in shapecache.go":                {goasmInC, "fingerprints it for the shape cache and leaves it as ccgo makes it"},
	"set_value in frames.go":                {goasmGoOnly, "fixFrames makes its TLS frame a Go array"},
	"set_value in framesites.go":            {goasmGoOnly, "the frame site of fixFrames, see frames.go"},
	"set_value in inline.go":                {goasmGoOnly, "fixInline writes it out in the Go interpreter and leaves it as it is"},
	"set_value in main.go":                  {goasmGoOnly, "passes the frame sites to fixFrames, see frames.go"},
	"set_value in slots.go":                 {goasmInC, "auditSlots reads it for the pending stack, which quickjs.diff guards in C"},
	"set_value in slotsites.go":             {goasmInC, "the slot site of auditSlots, see slots.go"},
}

// goasmRuleFiles returns the files of the fixlib sources in dir that change
// _JS_CallInternal, with what shows it: code that names the function, a use
// of a package variable whose value names it, or a "(see fixX)" mark of a
// change the file defines in the interpreter of the generated file engine.
func goasmRuleFiles(dir, engine string) (map[string]string, error) {
	src, err := loadGoasmSources(dir)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(engine)
	if err != nil {
		return nil, err
	}
	body := goasmFunction.Find(b)
	if body == nil {
		return nil, fmt.Errorf("%s: no _JS_CallInternal", engine)
	}
	return src.changes(func(lit string) bool { return strings.Contains(lit, "_JS_CallInternal") }, "_JS_CallInternal", body)
}

// goasmHelperWhat is what goasmRuleProblems says a file of goasmHelperRules
// changes.
const goasmHelperWhat = "the Go copy of a helper the C interpreter compiles in"

// goasmHelperFiles returns, for each of helpers (C names, as the generator
// lists them in lib/goasm_helpers.txt) that has a Go function of its own in
// the generated file engine, the files of the fixlib sources in dir that
// change it, keyed "helper in file". A file changes it when its code names
// the Go function on its own, as a list of the functions a change rewrites,
// fingerprints or clones does, or holds its first line, as a hook does,
// uses a package variable with such a value, or defines a change that left
// a "(see fixX)" mark in the function. A call of the helper in the text of
// another function is not a change of it: a change inside a helper leaves
// its mark there. ccgo writes some helpers into their callers instead of a
// function of their own: those change with the interpreter, which
// goasmRuleFiles covers.
func goasmHelperFiles(dir, engine string, helpers []string) (map[string]string, error) {
	src, err := loadGoasmSources(dir)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(engine)
	if err != nil {
		return nil, err
	}
	found := map[string]string{}
	for _, h := range helpers {
		for _, goName := range []string{"_" + h, "X" + h, "x" + h} {
			body := regexp.MustCompile(`(?s)\nfunc ` + regexp.QuoteMeta(goName) + `\(.*?\n}\n`).Find(b)
			if body == nil {
				continue
			}
			head := "func " + goName + "("
			files, err := src.changes(func(lit string) bool {
				v, err := strconv.Unquote(lit)
				return err == nil && (v == goName || strings.Contains(v, head))
			}, goName, body)
			if err != nil {
				return nil, err
			}
			for name, why := range files {
				found[h+" in "+name] = why
			}
		}
	}
	return found, nil
}

// goasmSources are the parsed fixlib sources, without tests, by file name,
// and the file that defines each function.
type goasmSources struct {
	files   map[string]*ast.File
	defines map[string]string
}

func loadGoasmSources(dir string) (*goasmSources, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	src := &goasmSources{files: map[string]*ast.File{}, defines: map[string]string{}}
	fset := token.NewFileSet()
	for _, fn := range names {
		if strings.HasSuffix(fn, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, fn, nil, 0)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(fn)
		src.files[name] = f
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				src.defines[fd.Name.Name] = name
			}
		}
	}
	return src, nil
}

// changes returns the files that change the generated function goName,
// whose text is body: those with a string literal that names matches,
// those that use a package variable whose value has one, and those that
// define a change marked "(see fixX)" in body.
func (src *goasmSources) changes(names func(string) bool, goName string, body []byte) (map[string]string, error) {
	namesIt := func(n ast.Node) bool {
		found := false
		ast.Inspect(n, func(n ast.Node) bool {
			if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING && names(l.Value) {
				found = true
			}
			return !found
		})
		return found
	}
	vars := map[string]bool{}
	for _, f := range src.files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
				for _, s := range g.Specs {
					v := s.(*ast.ValueSpec)
					if namesIt(v) {
						for _, n := range v.Names {
							vars[n.Name] = true
						}
					}
				}
			}
		}
	}
	found := map[string]string{}
	for name, f := range src.files {
		if namesIt(f) {
			found[name] = "names " + goName
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && vars[id.Name] && id.Obj == nil && found[name] == "" {
				found[name] = "uses " + id.Name
			}
			return true
		})
	}
	for _, m := range regexp.MustCompile(`\(see (fix\w+)\)`).FindAllSubmatch(body, -1) {
		change := string(m[1])
		name, ok := src.defines[change]
		if !ok {
			return nil, fmt.Errorf("%s names %s, which fixlib does not define", goName, change)
		}
		if found[name] == "" {
			found[name] = "left (see " + change + ") in " + goName
		}
	}
	return found, nil
}

// goasmHelpers reads the helpers the generator lists in path, one C name a
// line after the comment lines that start with #.
func goasmHelpers(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r []string
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			r = append(r, line)
		}
	}
	return r, nil
}

// goasmRuleProblems compares the files that change what, with what shows
// it, to the rows of rules, the table named table, and returns one line per
// problem. cpatch is the text of internal/goasm/cpatch.py.
func goasmRuleProblems(found map[string]string, rules map[string]goasmRule, cpatch, table, what string) (r []string) {
	for name, why := range found {
		if _, ok := rules[name]; !ok {
			r = append(r, fmt.Sprintf("%s: changes %s (%s) but has no row in %s", name, what, why, table))
		}
	}
	for name, rule := range rules {
		if _, ok := found[name]; !ok {
			r = append(r, fmt.Sprintf("%s: has a row in %s but no longer changes %s", name, table, what))
		} else if rule.kind == goasmCPatch && !regexp.MustCompile(`\b`+regexp.QuoteMeta(rule.text)+`\b`).MatchString(cpatch) {
			r = append(r, fmt.Sprintf("%s: internal/goasm/cpatch.py does not name %s", name, rule.text))
		}
	}
	slices.Sort(r)
	return r
}
