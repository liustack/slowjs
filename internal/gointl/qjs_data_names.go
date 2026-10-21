// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !quickjs_intl_nonames

package intl

import _ "embed"

//go:embed qjsdata/names.bin
var qjsNames string

const qjsNoNames = false
