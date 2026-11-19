// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs

import "testing"

func TestUpstreamIteratorReduce(t *testing.T) {
	checkCompareFile(t, "iterator_reduce")
}
