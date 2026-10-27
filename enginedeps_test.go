// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The package builds on one engine, and a program links only that one: the
// engine of lib, made by ccgo and running on modernc.org/libc, or with the tag
// slowjs_cengine the C engine of internal/cengine, which is generated for
// linux/amd64 only so far.
func TestEngineDeps(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}

	const (
		lib     = "github.com/liustack/slowjs/lib"
		libc    = "modernc.org/libc"
		cengine = "github.com/liustack/slowjs/internal/cengine"
		gohost  = "github.com/liustack/slowjs/internal/gohost"
		hostmem = "github.com/liustack/slowjs/internal/hostmem"
	)
	for _, c := range []struct {
		name      string
		args      []string
		env       []string
		want, not []string
	}{
		{"default", nil, nil, []string{lib, libc}, []string{cengine, gohost, hostmem}},
		{"slowjs_cengine", []string{"-tags", "slowjs_cengine"}, []string{"GOOS=linux", "GOARCH=amd64"},
			[]string{cengine, gohost, hostmem}, []string{lib, libc}},
	} {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(gobin, append(append([]string{"list", "-deps"}, c.args...), ".")...)
			// The flags of the run that started the test are not those of
			// the row.
			cmd.Env = append(append(os.Environ(), "GOFLAGS="), c.env...)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("go list: %v", err)
			}
			deps := strings.Fields(string(out))
			for _, p := range c.want {
				if !slices.Contains(deps, p) {
					t.Errorf("does not depend on %s", p)
				}
			}
			for _, p := range c.not {
				if slices.Contains(deps, p) {
					t.Errorf("depends on %s", p)
				}
			}
		})
	}
}
