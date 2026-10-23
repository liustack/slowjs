// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_moejs

package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Calcium-Ion/moejs"
	mengine "github.com/Calcium-Ion/moejs/engine"
)

func init() {
	register(&Engine{
		Name:           "moejs",
		Module:         "github.com/Calcium-Ion/moejs",
		New:            newMoejs,
		SharedPrograms: true,
		CompileShared:  compileMoejs,
		Interrupt:      true,
		MemoryLimit:    true,
		Modules:        true,
		Notes: []string{
			"Scripts are compiled once with moejs.CompileScript and run with Runtime.RunScript. A Script runs on any runtime.",
			"Runtimes are made with Options.MutableIntrinsics: test262, the Intl polyfills and the tifa bundles write to the builtins, which are frozen and shared by default (a new runtime costs about 25 times as much this way, by its guide).",
			"Promise jobs run before RunScript and CallFunction return, so RunJobs has nothing to do.",
			"Options.MemoryLimit counts what a runtime allocates between resets, not what is live. The call depth is fixed at 512.",
		},
	})
}

type moejsVM struct {
	rt      *moejs.Runtime
	dir     string
	mu      sync.Mutex
	modules map[string]*moejs.Module
}

var errMoejsInterrupted = errors.New("interrupted")

func compileMoejs(name, src string) (Program, error) {
	s, err := moejs.CompileScript(name, src)
	if err != nil {
		return nil, moejsErr(err, true)
	}
	return s, nil
}

func newMoejs(o Options) (VM, error) {
	v := &moejsVM{dir: o.ModuleDir, modules: map[string]*moejs.Module{}}
	opts := moejs.Options{MutableIntrinsics: true, MemoryLimit: o.MemoryLimit}
	if o.ModuleDir != "" {
		opts.Importer = &moejs.Importer{Resolve: v.resolve}
	}
	v.rt = moejs.NewRuntime(opts)
	pr := o.Print
	print := v.rt.Function("__jsbench_print", 1, func(r *mengine.Realm, this mengine.Value, args []mengine.Value) (mengine.Value, error) {
		s, err := r.ToString(mengine.Arg(args, 0))
		if err != nil {
			return mengine.Undefined(), err
		}
		if pr != nil {
			pr(s.GoString())
		}
		return mengine.Undefined(), nil
	})
	if err := v.rt.SetGlobal("__jsbench_print", print); err != nil {
		return nil, err
	}
	if _, err := v.Eval("print.js", PrintShim); err != nil {
		return nil, err
	}
	return v, nil
}

// resolve maps a specifier to the file next to the importing module, or in
// ModuleDir, compiled once per VM: moejs reads no files itself.
func (v *moejsVM) resolve(ref moejs.Referrer, spec string) (*moejs.Module, error) {
	base := v.dir
	if ref != nil && filepath.IsAbs(ref.Name()) {
		base = filepath.Dir(ref.Name())
	}
	path := filepath.Join(base, spec)
	v.mu.Lock()
	defer v.mu.Unlock()
	if m, ok := v.modules[path]; ok {
		return m, nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := moejs.Compile(path, string(src))
	if err != nil {
		return nil, err
	}
	v.modules[path] = m
	return m, nil
}

func moejsErr(err error, compile bool) error {
	if err == nil {
		return nil
	}
	var se *moejs.SyntaxError
	if errors.As(err, &se) {
		return &JSError{Name: "SyntaxError", Message: se.Message, Compile: true}
	}
	var mle *moejs.MemoryLimitError
	if errors.As(err, &mle) {
		return fmt.Errorf("memory limit: %d bytes allocated, limit %d", mle.Allocated, mle.Limit)
	}
	var ie *moejs.InterruptedError
	if errors.As(err, &ie) {
		return errMoejsInterrupted
	}
	var ex *moejs.Exception
	if errors.As(err, &ex) {
		return &JSError{Name: ex.Name(), Message: ex.Message(), Compile: compile}
	}
	return Classify(err, compile)
}

// str is String(v), falling back to the engine's own description for values
// String() throws on (symbols).
func (v *moejsVM) str(x mengine.Value) string {
	s, err := v.rt.Realm().ToString(x)
	if err != nil {
		return x.String()
	}
	return s.GoString()
}

// done clears an interrupt that stopped the call, so the VM can be used
// again, as the guide says a host must.
func (v *moejsVM) done(err error, compile bool) error {
	err = moejsErr(err, compile)
	if err != nil {
		v.rt.ClearInterrupt()
	}
	return err
}

func (v *moejsVM) Eval(name, src string) (string, error) {
	p, err := v.Compile(name, src)
	if err != nil {
		return "", err
	}
	return v.Run(p)
}

func (v *moejsVM) Compile(name, src string) (Program, error) { return compileMoejs(name, src) }

func (v *moejsVM) Run(p Program) (string, error) {
	r, err := v.rt.RunScript(p.(*moejs.Script))
	if err != nil {
		return "", v.done(err, false)
	}
	return v.str(r), nil
}

func (v *moejsVM) Call(fn string) (string, error) {
	f, err := v.rt.Get(mengine.ObjectValue(v.rt.Realm().Global), fn)
	if err != nil {
		return "", v.done(err, false)
	}
	r, err := v.rt.CallFunction(f, mengine.Undefined())
	if err != nil {
		return "", v.done(err, false)
	}
	return v.str(r), nil
}

func (v *moejsVM) RunJobs() error { return nil }

func (v *moejsVM) EvalModule(name, src string) error {
	m, err := moejs.Compile(name, src)
	if err != nil {
		return moejsErr(err, true)
	}
	if m, err = moejs.Link(m, v.resolve); err != nil {
		return moejsErr(err, true)
	}
	return v.done(v.rt.Load(m), false)
}

func (v *moejsVM) Interrupt() { v.rt.Interrupt(errMoejsInterrupted) }
func (v *moejsVM) Close()     {}
