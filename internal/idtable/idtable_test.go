// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package idtable

import (
	"sync"
	"testing"
)

func mustAdd(t *testing.T, tb *Table[int], x *int) uint32 {
	t.Helper()
	id, err := tb.Add(x)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", what)
		}
	}()
	f()
}

func TestTable(t *testing.T) {
	var tb Table[int]
	a, b := new(int), new(int)
	ia, ib := mustAdd(t, &tb, a), mustAdd(t, &tb, b)
	if ia == 0 || ib == 0 || ia == ib {
		t.Fatalf("ids %d and %d: not 0 and not the same", ia, ib)
	}
	if tb.Get(ia) != a || tb.Get(ib) != b {
		t.Fatal("Get does not find what Add put")
	}
	for _, id := range []uint32{0, ib + 1, chunkSize, Max - 1, Max, ^uint32(0)} {
		if tb.Get(id) != nil {
			t.Fatalf("Get(%d) found something", id)
		}
	}
	tb.Remove(ia)
	if tb.Get(ia) != nil {
		t.Fatal("Get finds what Remove took out")
	}
	mustPanic(t, "Remove of an empty id", func() { tb.Remove(ia) })
	mustPanic(t, "Remove of 0", func() { tb.Remove(0) })
	mustPanic(t, "Add of nil", func() { tb.Add(nil) })
	c := new(int)
	if ic := mustAdd(t, &tb, c); ic != ia || tb.Get(ic) != c {
		t.Fatalf("the id Remove took back was not handed out again: %d, want %d", ic, ia)
	}
}

// TestTableFull: the table hands out Max-1 ids, then refuses, and takes more
// once one is back.
func TestTableFull(t *testing.T) {
	if testing.Short() {
		t.Skip("a million ids")
	}
	var tb Table[int]
	x := new(int)
	for range Max - 1 {
		mustAdd(t, &tb, x)
	}
	if _, err := tb.Add(x); err == nil {
		t.Fatal("a full table took one more")
	}
	tb.Remove(Max - 1)
	if id := mustAdd(t, &tb, x); id != Max-1 {
		t.Fatalf("got id %d, want %d", id, Max-1)
	}
}

// TestTableConcurrent: Get on many goroutines while others add and remove
// finds each thing under its own id, and nothing else (run it with -race).
func TestTableConcurrent(t *testing.T) {
	var tb Table[int]
	const goroutines, rounds = 8, 2000
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rounds {
				x := new(int)
				*x = g*rounds + i
				id, err := tb.Add(x)
				if err != nil {
					t.Error(err)
					return
				}
				if got := tb.Get(id); got != x || *got != g*rounds+i {
					t.Errorf("Get(%d) = %v, want what goroutine %d added", id, got, g)
					return
				}
				tb.Remove(id)
			}
		}()
	}
	wg.Wait()
}
