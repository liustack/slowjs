// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"fmt"
	"testing"
)

// Setup is excluded. Each sample collects a new graph, not an already empty VM.
func BenchmarkEphemeronGC(b *testing.B) {
	for _, count := range []int{10000, 100000} {
		for _, kind := range []string{"plain", "dead", "live-chain", "live-keys", "many-maps"} {
			b.Run(fmt.Sprintf("%s/%d", kind, count), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					vm, err := NewVM()
					if err != nil {
						b.Fatal(err)
					}
					vm.SetGCThreshold(^uintptr(0))
					body := `globalThis.wm=new WeakMap(); (()=>{for(let i=0;i<N;i++){let k={};wm.set(k,{k})}})();`
					if kind == "plain" {
						body = `(()=>{for(let i=0;i<N;i++){let k={};k.self=k}})();`
					}
					if kind == "live-keys" {
						body = `globalThis.wm=new WeakMap();globalThis.keys=Array.from({length:N},()=>({}));for(const k of keys)wm.set(k,{});`
					}
					if kind == "many-maps" {
						body = `globalThis.maps=Array.from({length:N/10},()=>new WeakMap());(()=>{let keys=Array.from({length:N+1},()=>({}));globalThis.key=keys[0];for(let i=N-1;i>=0;i--)maps[i%maps.length].set(keys[i],keys[i+1]);})();`
					}
					if kind == "live-chain" {
						body = `globalThis.wm=new WeakMap();(()=>{let keys=Array.from({length:N+1},()=>({}));globalThis.key=keys[0];for(let i=N-1;i>=0;i--)wm.set(keys[i],keys[i+1]);})();`
					}
					if _, err = vm.Eval(fmt.Sprintf("const N=%d;", count)+body+`void 0`, EvalGlobal); err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
					vm.RunGC()
					b.StopTimer()
					if err = vm.Close(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
