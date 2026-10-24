// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build libc.memgrind

package slowjs

import (
	"testing"

	"modernc.org/libc"
)

// The async context and the reactions that keep it leak nothing, also when
// the VM closes with them.
func TestMemgrind2AsyncContext(t *testing.T) {
	tls := libc.NewTLS()
	tls.Close()
	libc.MemAuditStart()
	t.Run("jobs", TestAsyncContextFollowsPromiseJobs)
	t.Run("object", TestAsyncContextIsAnObject)
	t.Run("close", TestAsyncContextClose)
	t.Run("freed", TestAsyncContextIsFreed)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
