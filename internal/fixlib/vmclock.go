// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
)

// Only these two entry points sample Date's current time. Check both the
// per-function counts and the whole file so a new caller cannot bypass it.
var vmClockCallers = []string{"_js_date_constructor", "_js_Date_now"}

func rewriteVMClock(b []byte, restore bool) ([]byte, error) {
	old := "_date_now(tls)"
	if restore {
		old = "dateNowContext("
	}
	if n := bytes.Count(b, []byte(old)); n != 2 {
		return nil, fmt.Errorf("VM clock: want 2 calls of %s, found %d", old, n)
	}
	for _, name := range vmClockCallers {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return nil, err
		}
		m := reClockParam.FindSubmatch(body)
		if m == nil {
			return nil, fmt.Errorf("VM clock: %s has no context parameter", name)
		}
		live, custom := []byte("_date_now(tls)"), []byte("dateNowContext("+string(m[1])+")")
		if restore {
			live, custom = custom, live
		}
		if n := bytes.Count(body, live); n != 1 {
			return nil, fmt.Errorf("VM clock: %s: want 1 %s, found %d", name, live, n)
		}
		b = bytes.Replace(b, body, bytes.Replace(body, live, custom, 1), 1)
	}
	return b, nil
}

func restoreVMClock(b []byte) ([]byte, error) {
	if !bytes.Contains(b, []byte("dateNowContext(")) {
		return b, nil
	}
	return rewriteVMClock(b, true)
}

func fixVMClock(b []byte, _ string) ([]byte, error) {
	bare, err := restoreVMClock(b)
	if err != nil {
		return nil, err
	}
	out, err := rewriteVMClock(bare, false)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("VM clock: partial rewrite")
	}
	back, err := restoreVMClock(out)
	if err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("VM clock: restore differs: %v", err)
	}
	return out, nil
}
