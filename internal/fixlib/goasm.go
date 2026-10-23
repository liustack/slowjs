// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
)

var goasmFunction = regexp.MustCompile(`(?s)func _JS_CallInternal\([^\n]+\{\n.*?\n}`)

const goasmMark = "\t// Changed by internal/fixlib: optional LLVM interpreter (see fixGoasm).\n\tif goasmEnabled {\n\t\treturn goasmCall(tls, caller_ctx, _func_obj, this_obj1, new_target, argc, argv, flags)\n\t}\n"

func checkGoasm(b []byte) error {
	f := goasmFunction.Find(b)
	if fmt.Sprintf("%x", sha256.Sum256(f)) != "b0b1d873b39201387090a1deb800ddcb4795d567b357498b39772f232d62ded2" {
		return fmt.Errorf("fixGoasm: interpreter fingerprint changed")
	}
	return nil
}
func fixGoasm(b []byte, name string) ([]byte, error) {
	if name != "ccgo_linux_amd64.go" {
		return b, nil
	}
	if err := checkGoasm(b); err != nil {
		return nil, err
	}
	i := goasmFunction.FindIndex(b)[0]
	i += bytes.IndexByte(b[i:], '\n') + 1
	out := append([]byte{}, b[:i]...)
	out = append(out, goasmMark...)
	out = append(out, b[i:]...)
	restored, err := restoreGoasm(out, name)
	if err != nil || !bytes.Equal(restored, b) {
		return nil, fmt.Errorf("fixGoasm: inverse mismatch: %v", err)
	}
	return out, nil
}
func restoreGoasm(b []byte, name string) ([]byte, error) {
	if name != "ccgo_linux_amd64.go" || !bytes.Contains(b, []byte(goasmMark)) {
		return b, nil
	}
	if bytes.Count(b, []byte(goasmMark)) != 1 {
		return nil, fmt.Errorf("restoreGoasm: marker count")
	}
	out := bytes.Replace(b, []byte(goasmMark), nil, 1)
	if err := checkGoasm(out); err != nil {
		return nil, err
	}
	return out, nil
}
