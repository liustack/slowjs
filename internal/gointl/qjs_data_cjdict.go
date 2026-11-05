// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !quickjs_intl_nocjdict

package intl

import _ "embed"

//go:embed qjsdata/cjdict.bin
var qjsCJDict string

const qjsNoCJDict = false
