// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs

import (
	"testing"
	"time"
)

func TestVMClockReset(t *testing.T) {
	tls, ctx := newClockContext(t)
	rt := XJS_GetRuntime(tls, ctx)
	at := time.Unix(-1, 999999999)
	SetClock(rt, func() time.Time { return at })
	if got := evalNumber(t, tls, ctx, "Date.now()"); got != -1 {
		t.Fatalf("Date.now = %v", got)
	}
	if got := Now(rt); !got.Equal(at) {
		t.Fatalf("Now = %v", got)
	}
	before := evalNumber(t, tls, ctx, "performance.now()")
	time.Sleep(time.Millisecond)
	if after := evalNumber(t, tls, ctx, "performance.now()"); after <= before {
		t.Fatal("performance.now stopped with fixed wall clock")
	}
	SetClock(rt, nil)
	if _, ok := clocks.Load(rt); ok {
		t.Fatal("reset retained clock")
	}
	if got := evalNumber(t, tls, ctx, "Date.now()"); got < float64(time.Now().Add(-time.Second).UnixMilli()) {
		t.Fatalf("reset Date.now = %v", got)
	}
}
