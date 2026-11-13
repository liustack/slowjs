// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command gocalls writes internal/cengine/gocalls.txt, the engine functions
// the Go code of the module names (gen.GoCalls). Run it from the root of the
// module after a change to the Go code that calls the engine:
//
//	go run ./internal/goasm/gen/cmd/gocalls
//
// The generator reads the list (-go-calls): a function off it does not write
// its result to the Go stack. TestGoCallsList fails when the list is stale.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	gen "github.com/liustack/quickjs/internal/goasm/gen"
)

func main() {
	pkg := filepath.Join("internal", "cengine")
	names, err := gen.GoCalls(".", pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gocalls:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(pkg, gen.GoCallsFile), gen.SortGoCalls(names), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gocalls:", err)
		os.Exit(1)
	}
}
