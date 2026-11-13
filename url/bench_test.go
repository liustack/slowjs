// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"testing"

	"github.com/liustack/quickjs"
)

// BenchmarkRegister is what Register adds to a new VM, once the bytecode of
// the script is compiled.
func BenchmarkRegister(b *testing.B) {
	if _, err := bytecode(); err != nil {
		b.Fatal(err)
	}
	b.Run("NewVM", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			vm, err := quickjs.NewVM()
			if err != nil {
				b.Fatal(err)
			}
			vm.Close()
		}
	})
	b.Run("NewVM+Register", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			vm, err := quickjs.NewVM()
			if err != nil {
				b.Fatal(err)
			}
			if err := Register(vm); err != nil {
				b.Fatal(err)
			}
			vm.Close()
		}
	})
}

func TestRegisterMemory(t *testing.T) {
	a, _ := quickjs.NewVM()
	defer a.Close()
	b := newVM(t)
	ma, mb := a.MemoryUsage(), b.MemoryUsage()
	t.Logf("a VM: %+v", ma)
	t.Logf("with url: %+v", mb)
}

// BenchmarkShop is what a page of a shop does for each link, in a VM: a
// relative path against the origin, and the parts a router reads.
func BenchmarkShop(b *testing.B) {
	vm, err := quickjs.NewVM()
	if err != nil {
		b.Fatal(err)
	}
	defer vm.Close()
	if err := Register(vm); err != nil {
		b.Fatal(err)
	}
	f, err := vm.EvalValue(`(function (path) {
		const u = new URL(path, 'http://localhost');
		u.pathname = '/zh-CN' + u.pathname;
		return u.protocol.length + u.username.length + u.password.length + u.origin.length + u.pathname.length + u.search.length + u.hash.length;
	})`, quickjs.EvalGlobal)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Free()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := f.Call(quickjs.UndefinedValue, "/products/some-product-name?page=2&sort=price"); err != nil {
			b.Fatal(err)
		}
	}
}
