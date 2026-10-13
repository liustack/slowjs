// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build libc.memgrind

package quickjs // import "github.com/liustack/quickjs"

import (
	"os"
	"path/filepath"
	"testing"

	util "modernc.org/fileutil/ccgo"
	"modernc.org/libc"
)

const runArewefastyet = `
BenchmarkSuite.RunSuites({
	NotifyError: function(name, error) {
		throw new Error("FAIL "+name+": "+error);
	}
});
`

var arewefastyetJS = []string{
	"base.js",
	"richards.js",
	"deltablue.js",
	"crypto.js",
	"raytrace.js",
	"earley-boyer.js",
	"regexp.js",
	"splay.js",
	"navier-stokes.js",
}

func init() {
	memgrind = true
}

func TestMemgrind(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	util.InDir(filepath.Join("internal", "arewefastyet", "v8-v7"), func() error {
		var src []string
		for _, fn := range arewefastyetJS {
			s, err := os.ReadFile(fn)
			if err != nil {
				t.Fatal(fn, err)
			}

			src = append(src, string(s))
		}
		if testing.Short() {
			src = src[:2]
		}
		src = append(src, runArewefastyet)

		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}

		defer m.Close()

		for _, v := range src {
			if _, err := m.Eval(v, EvalGlobal); err != nil {
				t.Fatal(err)
			}
		}

		return nil
	})

	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}

func TestMemgrind2(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()

	t.Run("eval1", testEval1)
	t.Run("eval2", testEval2)
	t.Run("eval3", testEval3)
	t.Run("evalError", testEvalError)
	t.Run("call1", testCall1)
	t.Run("call2", testCall2)
	t.Run("call3", testCall3)
	t.Run("call4", testCall4)
	t.Run("fnFail", testRegisterGoFuncMustFail)
	t.Run("fnOK", testRegisterGoFuncOK)
	t.Run("evalValue", TestEvalValue)
	t.Run("atom", TestAtom)
	t.Run("setPropValue", TestSetPropertyValue)
	t.Run("setProp", TestSetProperty)
	t.Run("getPropValue", TestGetPropertyValue)
	t.Run("getProp", TestGetProperty)
	t.Run("prop", TestValueProperty)
	t.Run("valueCall", TestValueCall)
	t.Run("valueCallErrors", TestValueCallErrors)
	t.Run("valueCallPromise", TestValueCallPromise)
	t.Run("nullArg", TestRegisterFuncNullArg)
	t.Run("nullArgHost", TestRegisterHostFuncNullArg)
	t.Run("mapResult", TestRegisterFuncMapResult)
	t.Run("mapValueResult", TestRegisterFuncMapValueResult)
	t.Run("then", TestThenLeak)
	t.Run("nul", TestStringNUL)
	t.Run("customLoader", TestCustomModuleLoader)
	t.Run("closeLoader", TestCloseReleasesModuleLoader)

	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
