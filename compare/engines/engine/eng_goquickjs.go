// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_goquickjs || eng_goquickjsjit

package engine

import (
	"errors"
	"os"
	"path/filepath"

	quickjs "github.com/go-quickjs/go-quickjs"
)

func init() {
	name, notes := "go-quickjs-interp", []string{"JIT off: built without the quickjs_jit tag, the library default."}
	if goquickjsJIT {
		name, notes = "go-quickjs-jit", []string{"JIT on: built with -tags quickjs_jit and run with quickjs.WithJIT(), the experimental JIT."}
	}
	register(&Engine{
		Name:           name,
		Module:         "github.com/go-quickjs/go-quickjs",
		New:            newGoquickjs,
		SharedPrograms: true,
		CompileShared:  compileGoquickjs,
		Interrupt:      true,
		MemoryLimit:    true,
		Modules:        true,
		Notes: append(notes,
			"Scripts are compiled once with quickjs.Compile and run with RunProgram, as its docs recommend."),
	})
}

type goquickjsVM struct {
	rt   *quickjs.Runtime
	stop chan struct{}
}

func compileGoquickjs(name, src string) (Program, error) {
	p, err := quickjs.Compile(name, src)
	if err != nil {
		return nil, gqErr(err, true)
	}
	return p, nil
}

func newGoquickjs(o Options) (VM, error) {
	var opts []quickjs.Option
	if goquickjsJIT {
		opts = append(opts, quickjs.WithJIT())
	}
	if o.MemoryLimit > 0 {
		opts = append(opts, quickjs.WithMemoryLimit(o.MemoryLimit))
	}
	rt := quickjs.New(opts...)
	stop := make(chan struct{})
	if o.Interruptible {
		rt.AbortOn(stop)
	}
	pr := o.Print
	if err := rt.Set("__jsbench_print", func(s string) {
		if pr != nil {
			pr(s)
		}
	}); err != nil {
		return nil, err
	}
	if o.ModuleDir != "" {
		dir := o.ModuleDir
		rt.SetModuleLoader(func(spec, ref string) (string, string, error) {
			b, err := os.ReadFile(filepath.Join(dir, filepath.Base(spec)))
			return string(b), filepath.Base(spec), err
		})
	}
	v := &goquickjsVM{rt: rt, stop: stop}
	if _, err := rt.Eval(PrintShim); err != nil {
		return nil, err
	}
	return v, nil
}

func gqErr(err error, compile bool) error {
	var se *quickjs.SyntaxError
	if errors.As(err, &se) {
		return &JSError{Name: "SyntaxError", Message: se.Error(), Compile: true}
	}
	var e *quickjs.Error
	if errors.As(err, &e) {
		name := ""
		if c, err := e.Value().Get("constructor"); err == nil {
			if n, err := c.Get("name"); err == nil {
				name = n.String()
			}
		}
		return &JSError{Name: name, Message: e.Error(), Compile: compile}
	}
	return Classify(err, compile)
}

func (v *goquickjsVM) Eval(name, src string) (string, error) {
	r, err := v.rt.EvalFile(name, src)
	if err != nil {
		return "", gqErr(err, false)
	}
	return r.String(), nil
}

func (v *goquickjsVM) Compile(name, src string) (Program, error) { return compileGoquickjs(name, src) }

func (v *goquickjsVM) Run(p Program) (string, error) {
	r, err := v.rt.RunProgram(p.(*quickjs.Program))
	if err != nil {
		return "", gqErr(err, false)
	}
	return r.String(), nil
}

func (v *goquickjsVM) Call(fn string) (string, error) {
	f, err := v.rt.Get(fn)
	if err != nil {
		return "", gqErr(err, false)
	}
	r, err := f.Call()
	if err != nil {
		return "", gqErr(err, false)
	}
	return r.String(), nil
}

func (v *goquickjsVM) RunJobs() error {
	if err := v.rt.RunJobs(); err != nil {
		return gqErr(err, false)
	}
	return nil
}

func (v *goquickjsVM) EvalModule(name, src string) error {
	_, err := v.rt.EvalModule(name, src)
	if err != nil {
		return gqErr(err, false)
	}
	return v.RunJobs()
}

func (v *goquickjsVM) Interrupt() {
	select {
	case <-v.stop:
	default:
		close(v.stop)
	}
}

func (v *goquickjsVM) Close() { v.rt.Close() }
