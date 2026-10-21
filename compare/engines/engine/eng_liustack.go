// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_liustack

package engine

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	quickjs "github.com/liustack/quickjs"
	"github.com/liustack/quickjs/intl"
)

func init() {
	register(&Engine{
		Name:           "liustack-quickjs",
		Module:         "github.com/liustack/quickjs",
		New:            newLiustack,
		SharedPrograms: true,
		CompileShared:  compileLiustack,
		Interrupt:      true,
		MemoryLimit:    true,
		Modules:        true,
		Notes: []string{
			"Scripts are compiled to bytecode with VM.Compile and run with EvalBytecode.",
			"Intl comes from package intl, registered only when a test asks for it.",
		},
	})
}

type liustackVM struct{ vm *quickjs.VM }

// compileVM is the VM that compiles shared programs. Bytecode is
// independent of the VM that made it.
var compileVM *quickjs.VM

func compileLiustack(name, src string) (Program, error) {
	if compileVM == nil {
		var err error
		if compileVM, err = quickjs.NewVM(); err != nil {
			return nil, err
		}
	}
	b, err := compileVM.Compile(src, quickjs.EvalGlobal)
	if err != nil {
		return nil, qjsErr(err, true)
	}
	return b, nil
}

func newLiustack(o Options) (VM, error) {
	vm, err := quickjs.NewVM()
	if err != nil {
		return nil, err
	}
	if o.MemoryLimit > 0 {
		vm.SetMemoryLimit(uintptr(o.MemoryLimit))
	}
	pr := o.Print
	if err := vm.RegisterFunc("__jsbench_print", func(s string) {
		if pr != nil {
			pr(s)
		}
	}, false); err != nil {
		vm.Close()
		return nil, err
	}
	if _, err := vm.Eval(PrintShim, quickjs.EvalGlobal); err != nil {
		vm.Close()
		return nil, err
	}
	if o.Clock {
		start := time.Now()
		if err := vm.RegisterFunc("__jsbench_now", func() float64 {
			return float64(time.Since(start).Nanoseconds()) / 1e6
		}, false); err != nil {
			vm.Close()
			return nil, err
		}
	}
	if o.Intl {
		if err := intl.Register(vm); err != nil {
			vm.Close()
			return nil, err
		}
	}
	if o.ModuleDir != "" {
		dir := o.ModuleDir
		vm.SetModuleLoader(func(_ *quickjs.VM, name string) (string, error) {
			b, err := os.ReadFile(filepath.Join(dir, filepath.Base(name)))
			return string(b), err
		}, nil)
	}
	return &liustackVM{vm}, nil
}

func qjsErr(err error, compile bool) error {
	var e *quickjs.Error
	if errors.As(err, &e) {
		return &JSError{Name: e.Name, Message: e.Message, Compile: compile}
	}
	return Classify(err, compile)
}

func (v *liustackVM) Eval(name, src string) (string, error) {
	r, err := v.vm.Eval(src, quickjs.EvalGlobal)
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *liustackVM) Compile(name, src string) (Program, error) {
	b, err := v.vm.Compile(src, quickjs.EvalGlobal)
	if err != nil {
		return nil, qjsErr(err, true)
	}
	return b, nil
}

func (v *liustackVM) Run(p Program) (string, error) {
	r, err := v.vm.EvalBytecode(p.([]byte))
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *liustackVM) Call(fn string) (string, error) {
	r, err := v.vm.Call(fn)
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *liustackVM) RunJobs() error {
	_, err := v.vm.ExecutePendingJobs()
	if err != nil {
		return qjsErr(err, false)
	}
	return nil
}

func (v *liustackVM) EvalModule(name, src string) error {
	_, err := v.vm.Eval(src, quickjs.EvalModule)
	if err != nil {
		return qjsErr(err, false)
	}
	_, err = v.vm.ExecutePendingJobs()
	if err != nil {
		return qjsErr(err, false)
	}
	return nil
}

func (v *liustackVM) Interrupt() { v.vm.Interrupt() }
func (v *liustackVM) Close()     { v.vm.Close() }
