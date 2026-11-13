// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/liustack/quickjs"
)

// TestParallelVMs makes a VM on each of many goroutines, registers URL on
// every one, and has them parse different inputs at the same time, hosts that
// need NFC among them, and use the shared bytecode and the shared cache of
// bases. Each result is compared with the one a VM makes alone. It is for the
// race detector.
func TestParallelVMs(t *testing.T) {
	inputs := []string{
		"http://éxample.com/a?b#c",
		"http://한글.kr/",
		"https://ｅｘａｍｐｌｅ.com:8443/x",
		"http://å̈.example/",
		"http://example.com/é́?é#é",
		"/products/1?page=2",
		"../x/./y?z=1#h",
		"//other.example/p",
	}
	for i := 0; i < 20; i++ {
		inputs = append(inputs, fmt.Sprintf("http://h%d.é́%d.example/p%d?q=%d", i, i, i, i))
	}
	bases := []string{"http://localhost", "https://shop.example/a/b/c", "http://é.example/"}
	run := func(vm *quickjs.VM) string {
		src := "(function(){ var inputs = " + jsArray(inputs) + ", bases = " + jsArray(bases) + ", out = '';" + `
			for (var n = 0; n < 60; n++) {
				var s = inputs[n % inputs.length], b = bases[n % bases.length];
				try {
					var u = new URL(s, b);
					u.searchParams.append('k', 'v' + n);
					u.hash = 'h' + n;
					out += u.href + ' ' + URL.canParse(s) + '\n';
				} catch (e) { out += 'TypeError\n'; }
			}
			return out;
		})()`
		v, err := vm.Eval(src, quickjs.EvalGlobal)
		if err != nil {
			t.Error(err)
			return ""
		}
		return v.(string)
	}
	// What one VM answers alone.
	alone, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	if err := Register(alone); err != nil {
		t.Fatal(err)
	}
	want := run(alone)
	alone.Close()
	if want == "" {
		t.FailNow()
	}

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < 5; r++ {
				vm, err := quickjs.NewVM()
				if err != nil {
					t.Error(err)
					return
				}
				if err := Register(vm); err != nil {
					t.Error(err)
					vm.Close()
					return
				}
				if got := run(vm); got != want {
					t.Errorf("a VM answered\n%s\nwant\n%s", got, want)
				}
				vm.Close()
			}
		}()
	}
	wg.Wait()
}

func jsArray(a []string) string {
	s := "["
	for i, x := range a {
		if i > 0 {
			s += ","
		}
		s += strconv.Quote(x)
	}
	return s + "]"
}
