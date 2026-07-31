// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "modernc.org/libquickjs"

import (
	"modernc.org/libc"
)

func ___inline_isnan(tls *libc.TLS, x float64) int32 {
	return libc.Xisnan(tls, x)
}

func ___inline_isnanf(tls *libc.TLS, x float32) int32 {
	return libc.Xisnanf(tls, x)
}
