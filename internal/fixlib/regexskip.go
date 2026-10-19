// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"regexp"
)

// fixRegexSkip makes lre_exec call lreSkip of lib/regexskip.go first, which
// skips the positions of the string where the regexp cannot start to match,
// and run the bytecode from the op lreSkip gives, or return what lreSkip
// gives when there is nothing to run.
//
// lreSkip reads the bytecode lre_compile writes and does what lre_exec and
// lre_exec_backtrack do for a few ops. So the generated text of those three
// functions is pinned in regexSites: when a release changes one of them,
// fixRegexSkip stops, and the new code must be read against lreSkip before
// the fingerprints are updated (go run ./internal/fixlib -regex prints
// them). The fingerprints are of the text without the change, after every
// other change.
func fixRegexSkip(b []byte, file string) ([]byte, error) {
	got, err := regexFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := regexSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/regexskip.go, then update regexSites (go run ./internal/fixlib -regex)", file, "lre_compile, lre_exec or lre_exec_backtrack", got, want)
	}

	body, err := funcBody(b, reLreExecHead, "Xlre_exec")
	if err != nil {
		return nil, err
	}
	start := bytes.Index(b, body)
	fixed, err := insertAt("Xlre_exec", body, regexSkipCall, func(body []byte) (int, error) {
		return reLreExecHead.FindIndex(body)[1], nil
	})
	if err != nil {
		return nil, err
	}
	switch old, new := bytes.Count(fixed, []byte(regexRunOld)), bytes.Count(fixed, []byte(regexRunNew)); {
	case old == 1 && new == 0:
		fixed = bytes.Replace(fixed, []byte(regexRunOld), []byte(regexRunNew), 1)
	case old == 0 && new == 1:
	default:
		return nil, fmt.Errorf("Xlre_exec: want one call of lre_exec_backtrack from the start of the bytecode, found %d, and %d from where lreSkip says", old, new)
	}
	return append(append(append([]byte{}, b[:start]...), fixed...), b[start+len(body):]...), nil
}

var (
	reLreExecHead = regexp.MustCompile(`\nfunc Xlre_exec\(tls \*libc\.TLS, capture uintptr, bc_buf uintptr, cbuf uintptr, cindex int32, clen int32, cbuf_type int32, opaque uintptr\) \(r int32\) \{\n`)
	regexFuncs    = []string{"Xlre_compile", "Xlre_exec", "_lre_exec_backtrack"}
)

const (
	regexSkipCall = "\t// Changed by internal/fixlib: start where a match can start (see fixRegexSkip).\n" +
		"\tcindex, skipPC := lreSkip(tls, capture, bc_buf, cbuf, cindex, clen, cbuf_type, opaque)\n" +
		"\tif skipPC == 0 {\n\t\treturn cindex\n\t}\n"
	regexRunOld = "_lre_exec_backtrack(tls, s, capture, bc_buf+uintptr(MRE_HEADER_LEN), cptr)"
	regexRunNew = "_lre_exec_backtrack(tls, s, capture, bc_buf+skipPC, cptr)"
)

// regexFingerprint hashes regexFuncs, without the change of fixRegexSkip,
// with each C string written out in place of its offset.
func regexFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, regexFuncs, func(text []byte) []byte {
		text = bytes.Replace(text, []byte(regexSkipCall), nil, 1)
		return bytes.Replace(text, []byte(regexRunNew), []byte(regexRunOld), 1)
	})
}
