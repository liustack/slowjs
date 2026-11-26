// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package goversion

// TagArgs gives the build flags a test passes to a go command it starts, so
// that the child builds the engine under the same rule as the test itself:
// -tags=beastjs_anygo when the test was built with it, and nothing otherwise.
// A test that starts go build or go vet with GOFLAGS emptied drops the tag,
// and with a Go newer than the checked one its child then stops at the error
// of newer.go before it reaches what the test is about.
func TagArgs() []string {
	if AnyGo {
		return []string{"-tags=beastjs_anygo"}
	}
	return nil
}
