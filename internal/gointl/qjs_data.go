// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl

import _ "embed"

// The packs, which internal/qjspack/gen writes (qjs-sync.sh pack). The
// display names and the CJK dictionary are packs of their own, which the
// build tags quickjs_intl_nonames and quickjs_intl_nocjdict leave out.

//go:embed qjsdata/core.bin
var qjsCore string
