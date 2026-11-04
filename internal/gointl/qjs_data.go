// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import _ "embed"

// The packs, which internal/qjspack/gen writes (qjs-sync.sh pack). The
// display names and the CJK dictionary are packs of their own, which the
// build tags quickjs_intl_nonames and quickjs_intl_nocjdict leave out. The
// time zones and calendar tables the temporal package reads are one too, so
// that a binary that reads only those (EmbeddedTemporalSource) links only
// tz.bin.

//go:embed qjsdata/core.bin
var qjsCore string

//go:embed qjsdata/tz.bin
var qjsTZ string
