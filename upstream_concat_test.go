// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import "testing"

func TestUpstreamIteratorConcat(t *testing.T) {
	checkCompareFile(t, "iterator_concat")
}
