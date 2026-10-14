// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package quickjs // import "github.com/liustack/quickjs"

import (
	"io"
	"os"
	"syscall"
	"testing"
)

// TestErrorStackPrint checks the stack console.log prints with an error, which
// the engine reads without running any JS code.
func TestErrorStackPrint(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	if err := m.StdAddHelpers(); err != nil {
		t.Fatal(err)
	}

	got := captureStdout(t, func() {
		if _, err := m.Eval(`function printed() {
	var e = new Error("printed")
	console.log(e)
	console.log([e], { err: new TypeError("inner") })
	e.stack
	console.log(e)
}
printed()`, EvalGlobal); err != nil {
			t.Error(err)
		}
	})
	want := `Error: printed
    at printed (<eval>:2:19)
    at <eval> (<eval>:8:8)
[ Error: printed
    at printed (<eval>:2:19)
    at <eval> (<eval>:8:8) ] { err: TypeError: inner
    at printed (<eval>:4:39)
    at <eval> (<eval>:8:8) }
Error: printed
    at printed (<eval>:2:19)
    at <eval> (<eval>:8:8)
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// captureStdout returns what f writes to file descriptor 1.
func captureStdout(t *testing.T, f func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	saved, err := syscall.Dup(1)
	if err != nil {
		t.Fatal(err)
	}

	if err := syscall.Dup2(int(w.Fd()), 1); err != nil {
		t.Fatal(err)
	}

	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	if err := syscall.Dup2(saved, 1); err != nil {
		t.Fatal(err)
	}

	syscall.Close(saved)
	w.Close()
	return <-done
}
