// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"strings"
	"testing"
)

// testFormatter is a formatter for the tests of the registry alone.
type testFormatter struct {
	N int `json:"n"`
}

func (*testFormatter) init() error { return nil }

// The registry keeps no more than registrySlots formatters and
// registryRequests requests. An id it dropped finds nothing, and the same
// formatter made again gets a new id.
func TestRegistryDropsOldest(t *testing.T) {
	var r registry
	first, err := r.add([]byte("T0"), &testFormatter{N: 0})
	if err != nil {
		t.Fatal(err)
	}

	if r.entry(float64(first.id)) != first || r.lookup([]byte("T0")) != first {
		t.Fatal("the registry does not find what it was given")
	}

	for i := 1; i <= registryRequests+10; i++ {
		if _, err := r.add([]byte(fmt.Sprintf("T%d", i)), &testFormatter{N: i}); err != nil {
			t.Fatal(err)
		}
	}
	if entries, requests := r.count(); entries > registrySlots || requests > registryRequests {
		t.Fatalf("the registry has %d formatters and %d requests", entries, requests)
	}
	if r.entry(float64(first.id)) != nil || r.lookup([]byte("T0")) != nil {
		t.Error("the registry still finds a formatter it dropped")
	}

	again, err := r.add([]byte("T0"), &testFormatter{N: 0})
	if err != nil {
		t.Fatal(err)
	}

	if again.id == first.id || r.entry(float64(again.id)) != again {
		t.Errorf("made again, the formatter has id %d, and %d before", again.id, first.id)
	}
}

// A request key longer than maxRequestKey is not remembered: the registry
// finds its formatter by what it resolves to, and its size is bounded by
// that.
func TestRegistryLongRequest(t *testing.T) {
	var r registry
	short, long := []byte("Tshort"), []byte("T"+strings.Repeat("x", maxRequestKey))
	e, err := r.add(short, &testFormatter{N: 1})
	if err != nil {
		t.Fatal(err)
	}

	if r.lookup(short) != e {
		t.Fatal("a short request is not remembered")
	}

	e2, err := r.add(long, &testFormatter{N: 1})
	if err != nil {
		t.Fatal(err)
	}

	if e2 != e {
		t.Error("a long request that resolves alike does not share the formatter")
	}
	if r.lookup(long) != nil {
		t.Error("a long request is remembered")
	}
	if _, requests := r.count(); requests != 1 {
		t.Errorf("the registry remembers %d requests, want 1", requests)
	}
	if n := r.longestRequest(); n != len(short) {
		t.Errorf("the longest request remembered has %d bytes", n)
	}
}
