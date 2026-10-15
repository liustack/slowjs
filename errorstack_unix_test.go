// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package quickjs // import "github.com/liustack/quickjs"

import (
	"io"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// TestErrorStackPrint checks the stack console.log prints with an error, which
// the engine reads without running any JS code. printed is called from Go, so
// that no top-level code is on the stack and the stack is not formatted yet.
func TestErrorStackPrint(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	if err := m.StdAddHelpers(); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Eval(`function printed() {
	var e = new Error("printed")
	console.log(e)
	console.log([e], { err: new TypeError("inner") })
	e.stack
	console.log(e)
}`, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	got := captureStdout(t, func() {
		if _, err := m.Call("printed"); err != nil {
			t.Error(err)
		}
	})
	want := `Error: printed
    at printed (<eval>:2:19)
[ Error: printed
    at printed (<eval>:2:19) ] { err: TypeError: inner
    at printed (<eval>:4:39) }
Error: printed
    at printed (<eval>:2:19)
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

	saved, err := unix.Dup(1)
	if err != nil {
		t.Fatal(err)
	}

	if err := unix.Dup2(int(w.Fd()), 1); err != nil {
		t.Fatal(err)
	}

	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	if err := unix.Dup2(saved, 1); err != nil {
		t.Fatal(err)
	}

	unix.Close(saved)
	w.Close()
	return <-done
}
