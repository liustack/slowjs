// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package goversion

import (
	"fmt"
	"go/build"
	"os"
	"regexp"
	"strconv"
	"testing"
)

// checked reads the version newer.go names in its error, and checks that its
// build line stops the next one.
func checked(t *testing.T) int {
	b, err := os.ReadFile("newer.go")
	if err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`(?m)^//go:build go1\.(\d+) && !slowjs_anygo$`).FindSubmatch(b)
	name := regexp.MustCompile(`(?m)^var _ = slowjs_is_checked_up_to_go1_(\d+)_see_`).FindSubmatch(b)
	if line == nil || name == nil {
		t.Fatalf("newer.go has no build line or no name of the form the test reads:\n%s", b)
	}
	stop, _ := strconv.Atoi(string(line[1]))
	minor, _ := strconv.Atoi(string(name[1]))
	if stop != minor+1 {
		t.Fatalf("newer.go stops go1.%d, but its name says go1.%d was checked", stop, minor)
	}
	return minor
}

func TestNewerGoStops(t *testing.T) {
	minor := checked(t)
	for _, c := range []struct {
		minor int
		tags  []string
		stops bool
	}{
		{minor, nil, false},
		{minor + 1, nil, true},
		{minor + 2, nil, true},
		{minor + 1, []string{"slowjs_anygo"}, false},
	} {
		ctx := build.Default
		ctx.ReleaseTags = nil
		for i := 1; i <= c.minor; i++ {
			ctx.ReleaseTags = append(ctx.ReleaseTags, fmt.Sprintf("go1.%d", i))
		}
		ctx.BuildTags = c.tags
		ok, err := ctx.MatchFile(".", "newer.go")
		if err != nil {
			t.Fatal(err)
		}
		if ok != c.stops {
			t.Errorf("go1.%d with tags %v: newer.go in the build is %v, want %v", c.minor, c.tags, ok, c.stops)
		}
	}
}
