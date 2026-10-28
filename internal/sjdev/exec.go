// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package sjdev holds what the sjdev subcommands share: running a command with
// its output kept and timed.
package sjdev

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Cmd is a command to run.
type Cmd struct {
	Dir  string
	Env  []string // K=V added to the environment of this process
	Name string
	Args []string
}

// String returns the command as one line, for logs.
func (c Cmd) String() string {
	s := c.Name
	if len(c.Args) > 0 {
		s += " " + strings.Join(c.Args, " ")
	}
	return s
}

// Result is what a run left.
type Result struct {
	Out      string // stdout and stderr together
	Err      error  // nil on exit status 0
	Duration time.Duration
}

// LastLine returns the last non-empty line of the output, cut to n bytes.
func (r Result) LastLine(n int) string {
	lines := strings.Split(strings.TrimRight(r.Out, "\n"), "\n")
	l := strings.TrimSpace(lines[len(lines)-1])
	if len(l) > n {
		l = l[:n]
	}
	return l
}

// Runner runs commands. Tests give one that records them.
type Runner func(ctx context.Context, c Cmd, log io.Writer) Result

// Exec is the Runner that starts the process. The output goes to log (when not
// nil) as it comes, and is also returned.
func Exec(ctx context.Context, c Cmd, log io.Writer) Result {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	var buf bytes.Buffer
	var w io.Writer = &buf
	if log != nil {
		w = io.MultiWriter(&buf, log)
	}
	cmd.Stdout, cmd.Stderr = w, w
	t0 := time.Now()
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("%s: %w", c, err)
	}
	return Result{Out: buf.String(), Err: err, Duration: time.Since(t0)}
}
