// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

// dstPeriod is a time of one zone with one offset and one daylight saving
// flag, from start (Unix seconds) to the start of the next one.
type dstPeriod struct {
	start  int64
	offset int
	dst    bool
}
