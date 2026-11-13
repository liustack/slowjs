// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build gointl_all

package intl

import (
	"fmt"
	"os"
	"testing"
)

// TestMain fails the tests of the gointl_all tag, which read go-intl's
// testdata and need packs of every language, when testdata is not there:
// they run in the copy internal/gointl/data.sh test makes, not in place.
func TestMain(m *testing.M) {
	if _, err := os.Stat("testdata"); err != nil {
		fmt.Fprintln(os.Stderr, "The gointl_all tests need go-intl's testdata, which this repository does not keep: run them with internal/gointl/data.sh test, or make gointl.")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
