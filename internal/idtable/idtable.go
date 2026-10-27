// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package idtable gives things small ids, which the C engine passes back to
// the host on every callback, and finds a thing by its id with two atomic
// reads and no lock: callbacks of VMs on many goroutines do not wait for each
// other, and do not write to memory they share.
package idtable // import "github.com/liustack/slowjs/internal/idtable"

import (
	"fmt"
	"sync"
	"sync/atomic"
)

const (
	chunkBits = 10
	chunkSize = 1 << chunkBits
	chunks    = 1 << 10 // a million ids at once
)

// Max is one more than the largest id a Table hands out.
const Max = chunks * chunkSize

// A Table holds things by id. The zero Table is empty and ready to use. Add
// and Remove take a lock, Get takes none. A table hands out its own ids by Add
// and Remove, or holds things under the ids of another table by Put and
// Delete, never both.
type Table[T any] struct {
	mu     sync.Mutex
	free   []uint32
	next   uint32 // the ids handed out so far, 0 is never one
	put    bool   // the table took an id by Put
	chunks [chunks]atomic.Pointer[[chunkSize]atomic.Pointer[T]]
}

// Add gives x an id: never 0, and not the id of another thing in the table.
// An id that Remove took back is handed out again.
func (t *Table[T]) Add(x *T) (uint32, error) {
	if x == nil {
		panic("idtable: Add of nil")
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.put {
		panic("idtable: Add on a table that holds the ids of Put")
	}
	var id uint32
	switch n := len(t.free); {
	case n > 0:
		id = t.free[n-1]
		t.free = t.free[:n-1]
	case t.next+1 >= Max:
		return 0, fmt.Errorf("idtable: more than %d at once", Max-1)
	default:
		t.next++
		id = t.next
	}
	t.store(id, x)
	return id, nil
}

func (t *Table[T]) store(id uint32, x *T) {
	c := t.chunks[id>>chunkBits].Load()
	if c == nil {
		c = new([chunkSize]atomic.Pointer[T])
		t.chunks[id>>chunkBits].Store(c)
	}
	c[id&(chunkSize-1)].Store(x)
}

// Put holds x under id, which another table handed out: never 0, below Max
// and not holding anything. It panics otherwise.
func (t *Table[T]) Put(id uint32, x *T) {
	if x == nil || id == 0 || id >= Max {
		panic(fmt.Sprintf("idtable: Put of %p under %d", x, id))
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.next != 0 {
		panic("idtable: Put on a table that hands out ids by Add")
	}
	if t.Get(id) != nil {
		panic(fmt.Sprintf("idtable: Put under %d, which holds something", id))
	}
	t.put = true
	t.store(id, x)
}

// Delete takes out what Put put under id. It panics when id holds nothing.
func (t *Table[T]) Delete(id uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.put || t.Get(id) == nil {
		panic(fmt.Sprintf("idtable: Delete of %d, which Put did not fill", id))
	}
	t.chunks[id>>chunkBits].Load()[id&(chunkSize-1)].Store(nil)
}

// Remove takes id back. It panics when id holds nothing.
func (t *Table[T]) Remove(id uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Get(id) == nil {
		panic(fmt.Sprintf("idtable: Remove of %d, which holds nothing", id))
	}
	t.chunks[id>>chunkBits].Load()[id&(chunkSize-1)].Store(nil)
	t.free = append(t.free, id)
}

// Get returns what id holds, or nil.
func (t *Table[T]) Get(id uint32) *T {
	if id >= Max {
		return nil
	}
	c := t.chunks[id>>chunkBits].Load()
	if c == nil {
		return nil
	}
	return c[id&(chunkSize-1)].Load()
}
