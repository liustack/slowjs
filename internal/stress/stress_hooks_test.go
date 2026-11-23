// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build testhooks

package main

import (
	"bytes"
	"io"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// An abort of the engine in a render leaves its VM good only for Close: the
// stress closes it, which must free it all, and the VMs beside it, on the
// same worker and on the others, go on giving the right HTML.
func TestStressAbort(t *testing.T) {
	var log bytes.Buffer
	c := config{
		duration: 4 * time.Second, workers: 2, churn: 1, kinds: []string{"store", "wb", "mui"},
		renderTimeout: time.Minute, measureEvery: 16,
		oom: 10, timeout: 10, interrupt: 10, kill: 5, abort: 20,
		tolerance: 0.05, slack: 1 << 20, maxLog: 50, log: io.MultiWriter(&log, t.Output()),
	}
	if err := run(c); err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`the engine aborted (\d+) times, and (\d+) renders after the first abort gave the right HTML`).FindSubmatch(log.Bytes())
	if m == nil {
		t.Fatal("the engine never aborted")
	}
	if n, _ := strconv.Atoi(string(m[2])); n == 0 {
		t.Fatalf("no render gave its HTML after the engine aborted %s times", m[1])
	}
}
