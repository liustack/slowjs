// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compare // import "modernc.org/quickjs/compare"

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"runtime/debug"
	"slices"
	"testing"

	"github.com/dop251/goja"
	"github.com/fastschema/qjs"
	"modernc.org/quickjs"
)

var (
	//go:embed testdata/*.js
	embedFS embed.FS
	files   []fs.DirEntry
)

func TestMain(m *testing.M) {
	var err error
	if files, err = embedFS.ReadDir("testdata"); err != nil {
		os.Exit(1)
	}

	// --- FAIL: BenchmarkGOJA/date_parse.js
	//             err=TypeError: Object has no member 'toGMTString' at date_parse (<eval>:40:38(52)) out=<nil>
	// --- FAIL: BenchmarkQJS/global_destruct_strict.js
	//             err=ReferenceError: global_v1 is not defined
	//             at global_destruct_strict (global_destruct_strict.js:35:9)
	//             at <eval> (global_destruct_strict.js:41:1)
	// --- FAIL: BenchmarkQJS/global_write_strict.js
	//             err=ReferenceError: global_var0 is not defined
	//             at global_write_strict (global_write_strict.js:32:9)
	//             at <eval> (global_write_strict.js:40:1)
	// --- FAIL: BenchmarkQJS/map_delete.js
	//             err=ReferenceError: len is not defined
	//             at map_delete (map_delete.js:32:5)
	//             at <eval> (map_delete.js:45:1)
	//          out=<nil>
	// --- FAIL: BenchmarkQJS/weak_map_delete.js
	//             err=ReferenceError: len is not defined
	//             at weak_map_delete (weak_map_delete.js:32:5)
	//             at <eval> (weak_map_delete.js:49:1)
	//          out=<nil>
	// --- FAIL: BenchmarkQJS/weak_map_set.js
	//             err=ReferenceError: len is not defined
	//             at weak_map_set (weak_map_set.js:32:5)
	//             at <eval> (weak_map_set.js:46:1)
	//          out=<nil>
	files = slices.DeleteFunc(files, func(e fs.DirEntry) bool {
		switch e.Name() {
		case
			"date_parse.js",
			"global_destruct_strict.js",
			"global_write_strict.js",
			"map_delete.js",
			"weak_map_delete.js",
			"weak_map_set.js":

			return true
		default:
			return false
		}
	})

	os.Exit(m.Run())
}

func getTest(b *testing.B, fn string, bn int) (r string) {
	buf, err := embedFS.ReadFile(path.Join("testdata", fn))
	if err != nil {
		b.Fatal(err)
	}

	r = string(bytes.Replace(buf, []byte("$N"), []byte(fmt.Sprint(bn)), 1))
	debug.FreeOSMemory()
	return r
}

func BenchmarkCCGO(b *testing.B) {
	for _, v := range files {
		fn := v.Name()
		b.Run(fn, func(b *testing.B) {
			js := getTest(b, fn, b.N)
			vm, err := quickjs.NewVM()
			if err != nil {
				b.Fatal(fn, err)
			}

			defer vm.Close()

			b.ReportAllocs()
			b.ResetTimer()
			if out, err := vm.Eval(js, quickjs.EvalGlobal); err != nil {
				b.Fatalf("err=%v out=%v", err, out)
			}
		})
	}
}

func BenchmarkQJS(b *testing.B) {
	for _, v := range files {
		fn := v.Name()
		b.Run(fn, func(b *testing.B) {
			js := getTest(b, fn, b.N)
			vm, err := qjs.New()
			if err != nil {
				b.Fatal(fn, err)
			}

			defer vm.Close()

			b.ReportAllocs()
			b.ResetTimer()
			if out, err := vm.Eval(fn, qjs.Code(js)); err != nil {
				b.Fatalf("err=%v out=%v", err, out)
			}
		})
	}
}

func BenchmarkGOJA(b *testing.B) {
	for _, v := range files {
		fn := v.Name()
		b.Run(fn, func(b *testing.B) {
			js := getTest(b, fn, b.N)
			vm := goja.New()
			b.ReportAllocs()
			b.ResetTimer()
			if out, err := vm.RunString(js); err != nil {
				b.Fatalf("err=%v out=%v", err, out)
			}
		})
	}
}
