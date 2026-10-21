// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl

import _ "embed"

// The packs, which internal/qjspack/gen writes (qjs-sync.sh pack).

//go:embed qjsdata/core.bin
var qjsCore string

//go:embed qjsdata/names.bin
var qjsNames string

//go:embed qjsdata/cjdict.bin
var qjsCJDict string
