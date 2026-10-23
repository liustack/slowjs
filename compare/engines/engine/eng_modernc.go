// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_modernc

package engine

import (
	"errors"
	"os"
	"path/filepath"

	quickjs "modernc.org/quickjs"
	"modernc.org/quickjs/intl"
)

func init() {
	register(&Engine{
		Name:           "modernc-quickjs",
		Module:         "modernc.org/quickjs",
		New:            newModernc,
		SharedPrograms: true,
		CompileShared:  compileModernc,
		Interrupt:      true,
		MemoryLimit:    true,
		Modules:        true,
		Notes: []string{
			"Scripts are compiled to bytecode with VM.Compile and run with EvalBytecode.",
			"Intl comes from package intl, registered only when a test asks for it.",
		},
	})
}

type moderncVM struct{ vm *quickjs.VM }

// compileVM is the VM that compiles shared programs. Bytecode is
// independent of the VM that made it.
var compileVM *quickjs.VM

func compileModernc(name, src string) (Program, error) {
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

func newModernc(o Options) (VM, error) {
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
	return &moderncVM{vm}, nil
}

func qjsErr(err error, compile bool) error {
	var e *quickjs.Error
	if errors.As(err, &e) {
		return &JSError{Name: e.Name, Message: e.Message, Compile: compile}
	}
	return Classify(err, compile)
}

func (v *moderncVM) Eval(name, src string) (string, error) {
	r, err := v.vm.Eval(src, quickjs.EvalGlobal)
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *moderncVM) Compile(name, src string) (Program, error) {
	b, err := v.vm.Compile(src, quickjs.EvalGlobal)
	if err != nil {
		return nil, qjsErr(err, true)
	}
	return b, nil
}

func (v *moderncVM) Run(p Program) (string, error) {
	r, err := v.vm.EvalBytecode(p.([]byte))
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *moderncVM) Call(fn string) (string, error) {
	r, err := v.vm.Call(fn)
	if err != nil {
		return "", qjsErr(err, false)
	}
	return Str(r), nil
}

func (v *moderncVM) RunJobs() error {
	_, err := v.vm.ExecutePendingJobs()
	if err != nil {
		return qjsErr(err, false)
	}
	return nil
}

func (v *moderncVM) EvalModule(name, src string) error {
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

func (v *moderncVM) Interrupt() { v.vm.Interrupt() }
func (v *moderncVM) Close()     { v.vm.Close() }
