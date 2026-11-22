// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !((((linux && !android) || (darwin && !ios)) && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package hostmem

// The engine runs on linux, darwin and windows, on amd64 and arm64, only:
// another target stops the build in this import, before the files that need
// the OS of those fail (Take is used beside the engine).
import _ "github.com/liustack/slowjs/internal/unsupported"
