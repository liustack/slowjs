// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build libc.memgrind

package quickjs // import "github.com/liustack/quickjs"

import (
	"os"
	"path/filepath"
	"strings"
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
	t.Run("errorStack", testErrorStack)
	t.Run("errorStackOOM", testErrorStackOutOfMemory)
	t.Run("errorStackProxy", testErrorStackProxy)
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
	t.Run("registerExisting", testRegisterExistingName)
	t.Run("mapResult", TestRegisterFuncMapResult)
	t.Run("mapValueResult", TestRegisterFuncMapValueResult)
	t.Run("then", TestThenLeak)
	t.Run("nul", TestStringNUL)
	t.Run("goStringOOM", TestGoStringOutOfMemory)
	t.Run("objectJSONOOM", TestObjectJSONOutOfMemory)
	t.Run("hostFuncArgOOM", TestHostFuncArgumentOutOfMemory)
	t.Run("errorToStringThrows", TestErrorFromValueToStringThrows)
	t.Run("bigIntToStringThrows", TestBigIntToStringThrows)
	t.Run("errorGetterThrows", TestErrorFromValueGetterThrows)
	t.Run("customLoader", TestCustomModuleLoader)
	t.Run("closeLoader", TestCloseReleasesModuleLoader)
	t.Run("interruptUncatchable", TestInterruptUncatchable)
	t.Run("outOfMemoryUncatchable", TestOutOfMemoryUncatchable)
	t.Run("uncatchableNull", TestUncatchableNullIsOutOfMemory)
	t.Run("regexpOOM", TestRegexpOutOfMemory)
	t.Run("thrownNotUncatchable", TestThrownIsNotUncatchable)
	t.Run("uncatchableRunsNoPageCode", TestUncatchableRunsNoPageCode)
	t.Run("compileOOM", TestCompileOutOfMemory)
	t.Run("compileOOMFine", TestCompileOutOfMemoryFine)
	t.Run("compileOneFailure", TestCompileOneFailure)
	t.Run("bufferAfterFailure", TestBufferAfterFailure)
	t.Run("writeBytecodeOOM", TestWriteBytecodeOutOfMemory)
	t.Run("hostUncatchable", TestUncatchableThroughHostFunc)
	t.Run("moduleLoaderUncatchable", TestUncatchableThroughModuleLoader)
	t.Run("hostPanic", TestHostPanic)
	t.Run("hostPanicNested", TestHostPanicNested)
	t.Run("moduleLoaderPanic", TestModuleLoaderPanic)
	t.Run("defaultStackLimit", TestDefaultStackLimit)
	t.Run("newVMContextFailure", TestNewVMContextFailure)
	t.Run("enginePanicCall", testEnginePanicCall)

	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}

// testEnginePanicCall panics in the engine right below Call, where no
// javascript frame holds a value: Close frees everything, the frames the
// panic left on the TLS stack too.
func testEnginePanicCall(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	addRawPanic(t, m)
	runEnginePanic(t, m, enginePanicCases[0].run)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}

// After a panic deeper in the engine, Close frees the TLS stack and the VM
// (libc's memgrind TLS.Close panics on a stack it did not free). What it
// leaks are values that the javascript frames the panic went through held,
// which the engine frees on return only, with a defer as without: 69 blocks
// in the deep case either way.
func TestMemgrindEnginePanic(t *testing.T) {
	tls := libc.NewTLS()
	tls.Close()
	for _, c := range enginePanicCases {
		t.Run(c.name, func(t *testing.T) {
			libc.MemAuditStart()
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			addRawPanic(t, m)
			runEnginePanic(t, m, c.run)
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			err = libc.MemAuditReport()
			if err == nil {
				return
			}
			n := 0
			for _, line := range strings.Split(err.Error(), "\n")[1:] {
				switch {
				case strings.TrimSpace(line) == "":
				case strings.Contains(line, "_js_def_malloc: ") || strings.Contains(line, "_js_def_realloc: "):
					n++
				default:
					t.Errorf("a leak other than an engine value: %s", line)
				}
			}
			if c.name == "call" {
				t.Errorf("%d blocks leaked", n)
			}
			t.Logf("%d engine blocks leaked", n)
		})
	}
}
