// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_intl_nonames

package intl

// Without the display names, Intl.DisplayNames has no data in any locale.
var qjsNames string

const qjsNoNames = true
