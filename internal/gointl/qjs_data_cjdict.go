// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !quickjs_intl_nocjdict

package intl

import _ "embed"

//go:embed qjsdata/cjdict.bin
var qjsCJDict string

const qjsNoCJDict = false
