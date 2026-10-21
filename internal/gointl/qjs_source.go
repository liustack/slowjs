// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl

import "io/fs"

// qjsFiles is the data quickjs builds into the package in place of go-intl's
// data.pack. There is none yet.
type qjsFiles struct{}

func (qjsFiles) readFile(name string) ([]byte, error) { return nil, fs.ErrNotExist }
