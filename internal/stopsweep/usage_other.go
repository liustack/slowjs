// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !unix && !windows

package stopsweep

// usageNow is unknown here: the OS is not asked.
func usageNow() usage { return usage{faults: -1, cycles: -1} }
