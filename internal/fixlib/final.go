// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
)

// A final change is made by fixEngine after fixNoDefer, on the text every
// other change leaves, and is undone first, before any other change or check
// reads a generated file. Each checks that its restore gives its input back.
type finalChange struct {
	name    string
	fix     func(b []byte, name string) ([]byte, error)
	restore func(b []byte, name string) ([]byte, error)
	// report, when its flag is set, prints the sites of the change for a
	// review, and the run stops. It reads the generated files with read,
	// which runs the earlier changes through dropDefers in memory.
	flag   *bool
	report func(read func(fn string) ([]byte, error)) error
}

// finalChanges are the final changes, in the order fixEngine makes them.
var finalChanges = []finalChange{
	{"fixExpect", fixExpect, restoreExpect, expectF, expectReport},
	{"fixFastFrames", fixFastFrames, restoreFastFrames, fastFramesF, fastFrameReport},
	{"fixEphemeron", fixEphemeron, restoreEphemeron, ephemeronF, ephemeronReport},
}

// fixFinal makes the final changes in b, the generated file name.
func fixFinal(b []byte, name string) ([]byte, error) {
	var err error
	for _, c := range finalChanges {
		if b, err = c.fix(b, name); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// restoreFinal undoes the final changes in b, the generated file name, last
// first.
func restoreFinal(b []byte, name string) ([]byte, error) {
	var err error
	for i := len(finalChanges) - 1; i >= 0; i-- {
		if b, err = finalChanges[i].restore(b, name); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// finalReports runs the report of a final change whose flag is set, and
// says whether it ran one.
func finalReports() bool {
	for _, c := range finalChanges {
		if c.flag == nil || !*c.flag {
			continue
		}
		var hw []*ast.File
		read := func(fn string) ([]byte, error) {
			if hw == nil {
				var err error
				hw, err = handWritten("lib")
				if err != nil {
					return nil, err
				}
			}
			return readFinalInput(fn, hw)
		}
		if err := c.report(read); err != nil {
			fmt.Fprintln(os.Stderr, "fixlib:", err)
			os.Exit(1)
		}
		return true
	}
	return false
}

// readFinalInput prepares a file for a final report without writing it.
func readFinalInput(fn string, hw []*ast.File) ([]byte, error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}
	return fixBeforeFinal(b, hw, filepath.Base(fn))
}
