// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux

package libquickjs // import "modernc.org/libquickjs"

import (
	"modernc.org/libc/sys/types"
)

type Size_t = types.Size_t
