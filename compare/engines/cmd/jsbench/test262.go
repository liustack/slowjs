// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/liustack/quickjs/compare/engines/engine"
	"github.com/liustack/quickjs/compare/engines/t262"
)

type t262Result struct {
	Path string  `json:"t"`
	Mode string  `json:"m"`
	Pass bool    `json:"ok"`
	Err  string  `json:"e,omitempty"`
	MS   float64 `json:"ms"`
}

// runTest262 runs the tests listed in list, one fresh VM per test and mode,
// and prints one JSON line per run as it finishes.
func runTest262(e *engine.Engine, root, list string, timeout time.Duration) {
	cache := map[string]string{}
	harness := func(name string) string {
		if s, ok := cache[name]; ok {
			return s
		}
		b, err := os.ReadFile(filepath.Join(root, "harness", name))
		if err != nil {
			fatal(err)
		}
		cache[name] = string(b)
		return cache[name]
	}
	f, err := os.Open(list)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	w := bufio.NewWriter(os.Stdout)
	for sc.Scan() {
		mode, path, ok := strings.Cut(sc.Text(), " ")
		if !ok {
			continue
		}
		r := runOne262(e, root, path, mode, timeout, harness)
		b, _ := json.Marshal(r)
		w.Write(append(b, '\n'))
		w.Flush()
	}
}

func runOne262(e *engine.Engine, root, path, mode string, timeout time.Duration, harness func(string) string) (r t262Result) {
	r = t262Result{Path: path, Mode: mode}
	start := time.Now()
	defer func() {
		r.MS = float64(time.Since(start).Microseconds()) / 1000
		if p := recover(); p != nil {
			r.Pass, r.Err = false, "panic: "+errString(errors.New(strings.TrimSpace(sprint(p))))
		}
	}()
	src, meta, err := t262.Source(root, path, mode, harness)
	if err != nil {
		r.Err = err.Error()
		return
	}
	var printed strings.Builder
	vm, err := e.New(engine.Options{Interruptible: true, Intl: true, Print: func(s string) {
		printed.WriteString(s)
		printed.WriteByte('\n')
	}})
	if err != nil {
		r.Err = "new VM: " + errString(err)
		return
	}
	defer vm.Close()
	stop := engine.Watchdog(vm, timeout)
	_, err = vm.Eval(filepath.Base(path), src)
	if err == nil && meta.Flag("async") {
		err = vm.RunJobs()
	}
	stop()
	if meta.HasNegative {
		if err == nil {
			r.Err = "expected " + meta.NegType
			return
		}
		var je *engine.JSError
		if errors.As(engine.Classify(err, false), &je) && je.Name == meta.NegType {
			r.Pass = true
			return
		}
		r.Err = "expected " + meta.NegType + ", got " + errString(err)
		return
	}
	if err != nil {
		r.Err = errString(err)
		return
	}
	if meta.Flag("async") {
		out := printed.String()
		if strings.Contains(out, "Test262:AsyncTestFailure") || !strings.Contains(out, "Test262:AsyncTestComplete") {
			r.Err = "async: " + errString(errors.New(strings.TrimSpace(out)))
			return
		}
	}
	r.Pass = true
	return
}
