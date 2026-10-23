// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build libc.memgrind

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"testing"

	"modernc.org/libc"
)

// Under libc's allocation tracker (make leak), the tests of the regexp cache
// leave no C memory behind: JS_FreeRuntime lets go of every entry, and an
// entry dropped or let go frees its strings.
func TestMemgrind2RegexCache(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	t.Run("shares", TestRegexCacheShares)
	t.Run("keeps", TestRegexCacheKeeps)
	t.Run("lru", TestRegexCacheLRU)
	t.Run("buckets", TestRegexCacheBuckets)
	t.Run("evicted", TestRegexCacheEvicted)
	t.Run("memory", TestRegexCacheMemory)
	t.Run("outOfMemory", TestRegexCacheOutOfMemory)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
