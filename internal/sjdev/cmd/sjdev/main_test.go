// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"testing"
)

// Every bench command defines its flags without a clash: a flag defined twice
// panics before -h prints anything.
func TestBenchCommandsParseHelp(t *testing.T) {
	stderr := os.Stderr
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	os.Stderr = null
	defer func() { os.Stderr = stderr }()
	for _, c := range []string{"build", "run", "judge", "report", "cg", "cgdiff", "addrs"} {
		err := cmdBench(context.Background(), []string{c, "-h"})
		if !errors.Is(err, flag.ErrHelp) {
			t.Errorf("bench %s -h: %v", c, err)
		}
	}
}
