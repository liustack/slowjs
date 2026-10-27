// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

// Memory outside a VM on the engine of lib.

import (
	"modernc.org/libc"
)

// allocShared returns n bytes of C memory for a SharedBytecode, at least one,
// or 0 when there is no memory.
func allocShared(n int) uintptr {
	tls := libc.NewTLS()
	defer tls.Close()
	return libc.Xmalloc(tls, libc.Tsize_t(max(n, 1)))
}

// freeShared frees the memory of allocShared.
func freeShared(p uintptr) {
	tls := libc.NewTLS()
	libc.Xfree(tls, p)
	tls.Close()
}
