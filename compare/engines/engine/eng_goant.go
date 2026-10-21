// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_goant || eng_goantjit

package engine

import (
	"errors"

	"github.com/robomotionio/goant"
)

func init() {
	name, notes := "goant-interp", []string{"JIT off: goant.New() without WithJIT, the library default."}
	if goantJIT {
		name, notes = "goant-jit", []string{"JIT on: goant.New(goant.WithJIT(true)), the opt-in baseline JIT."}
	}
	register(&Engine{
		Name:        name,
		Module:      "github.com/robomotionio/goant",
		New:         newGoant,
		Interrupt:   true,
		MemoryLimit: true,
		Modules:     true,
		Notes: append(notes,
			"Scripts are compiled with Runtime.Compile and run with RunProgram. A Program belongs to the Runtime that compiled it, so cold start parses the source on every VM."),
	})
}

type goantVM struct{ rt *goant.Runtime }

func newGoant(o Options) (VM, error) {
	opts := []goant.Option{goant.WithJIT(goantJIT)}
	if o.MemoryLimit > 0 {
		opts = append(opts, goant.WithMemoryLimit(uint64(o.MemoryLimit)))
	}
	if o.ModuleDir != "" {
		opts = append(opts, goant.WithModuleDir(o.ModuleDir))
	}
	rt := goant.New(opts...)
	pr := o.Print
	if err := rt.Set("__jsbench_print", func(s string) {
		if pr != nil {
			pr(s)
		}
	}); err != nil {
		return nil, err
	}
	if _, err := rt.RunString(PrintShim); err != nil {
		return nil, err
	}
	return &goantVM{rt}, nil
}

func goantErr(err error, compile bool) error {
	var se *goant.SyntaxError
	if errors.As(err, &se) {
		return &JSError{Name: "SyntaxError", Message: se.Message, Compile: true}
	}
	var e *goant.Error
	if errors.As(err, &e) {
		return &JSError{Name: e.Name, Message: e.Message, Compile: compile}
	}
	return Classify(err, compile)
}

func (v *goantVM) Eval(name, src string) (string, error) {
	p, err := v.Compile(name, src)
	if err != nil {
		return "", err
	}
	return v.Run(p)
}

func (v *goantVM) Compile(name, src string) (Program, error) {
	p, err := v.rt.Compile(name, src)
	if err != nil {
		return nil, goantErr(err, true)
	}
	return p, nil
}

func (v *goantVM) Run(p Program) (string, error) {
	r, err := v.rt.RunProgram(p.(*goant.Program))
	if err != nil {
		return "", goantErr(err, false)
	}
	return r.String(), nil
}

func (v *goantVM) Call(fn string) (string, error) {
	f, err := v.rt.Get(fn)
	if err != nil {
		return "", goantErr(err, false)
	}
	fun := f.Function()
	if fun == nil {
		return "", errors.New(fn + " is not a function")
	}
	r, err := fun.Call()
	if err != nil {
		return "", goantErr(err, false)
	}
	return r.String(), nil
}

func (v *goantVM) RunJobs() error {
	if err := v.rt.RunJobs(); err != nil {
		return goantErr(err, false)
	}
	return nil
}

func (v *goantVM) EvalModule(name, src string) error {
	_, err := v.rt.RunModule(name, src)
	if err != nil {
		return goantErr(err, false)
	}
	return nil
}

func (v *goantVM) Interrupt() { v.rt.Interrupt() }
func (v *goantVM) Close()     { v.rt.Close() }
