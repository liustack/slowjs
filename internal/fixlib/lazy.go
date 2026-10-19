// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// The marks fixLazyFrames leaves (see fixLazyFrames).
const (
	lazyStmtMark   = "// Changed by internal/fixlib: a TLS frame for this statement (see fixLazyFrames).\n"
	lazyReturnMark = "// Changed by internal/fixlib: a TLS frame for this return (see fixLazyFrames).\n"
	lazyIfMark     = "// Changed by internal/fixlib: a TLS frame for the condition of this if (see fixLazyFrames).\n"
)

// lazyHeadMark is the mark at the head of a function fixLazyFrames changed,
// whose TLS frame was n bytes.
func lazyHeadMark(n int) string {
	return fmt.Sprintf("// Changed by internal/fixlib: the TLS frame of %d bytes is a Go array, and each statement that needs a TLS frame takes one of its own (see fixLazyFrames).\n", n)
}

var reLazyHead = regexp.MustCompile(`^\{\n\t// Changed by internal/fixlib: the TLS frame of (\d+) bytes is a Go array, and each statement that needs a TLS frame takes one of its own \(see fixLazyFrames\)\.\n`)

// lazyHead returns the frame size in the head mark of body, the text of a
// function body from its opening brace, or false when it has none.
func lazyHead(body []byte) (int, bool) {
	m := reLazyHead.FindSubmatch(body)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(string(m[1]))
	return n, err == nil
}
