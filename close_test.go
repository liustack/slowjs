// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !slowjs_cengine

package slowjs // import "github.com/liustack/slowjs"

import (
	"testing"

	lib "github.com/liustack/slowjs/lib"
)

// TestNewVMContextFailure pins that a VM whose context cannot be made
// frees what newVM allocated before: the argv of toString and the
// interrupt flag. The leak shows under the memgrind allocator
// (TestMemgrind2).
func TestNewVMContextFailure(t *testing.T) {
	rt, err := newRuntime()
	if err != nil {
		t.Fatal(err)
	}
	defer rt.close()
	// No allocation of the runtime succeeds from here on.
	lib.XJS_SetMemoryLimit(rt.tls, rt.cRuntime, 1)
	if _, err := rt.newVM(); err == nil {
		t.Fatal("newVM made a context with no memory to make it in")
	}
}
