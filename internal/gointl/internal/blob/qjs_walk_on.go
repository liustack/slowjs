// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build gointl_walk

package blob

import (
	"sync"
	"unsafe"
)

// Walking reports whether the pool parts read are recorded, which only the
// gointl_walk build tag does, for slowjs's pack tool.
const Walking = true

var walk struct {
	sync.Mutex
	// seen holds the parts read of each pool, by where its ends start.
	seen map[*byte]map[int]bool
}

func walked(s Shared, i int) {
	walk.Lock()
	defer walk.Unlock()
	if walk.seen == nil {
		walk.seen = map[*byte]map[int]bool{}
	}
	key := unsafe.SliceData(s.ends)
	if walk.seen[key] == nil {
		walk.seen[key] = map[int]bool{}
	}
	walk.seen[key][i] = true
}

// Walked is the parts of pool, as ReadShared was given it, read since
// StartWalk.
func Walked(pool []byte) map[int]bool {
	walk.Lock()
	defer walk.Unlock()
	if len(pool) < 5 {
		return nil
	}
	return walk.seen[unsafe.SliceData(pool[5:])]
}

// StartWalk forgets the parts read so far.
func StartWalk() {
	walk.Lock()
	defer walk.Unlock()
	walk.seen = nil
}
