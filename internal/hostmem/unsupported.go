// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !(((linux || darwin) && (amd64 || arm64)) || (windows && amd64))

package hostmem

// The engine runs on linux and darwin, on amd64 and arm64, and on
// windows/amd64, only: another target stops the build in this import, before
// the files that need the OS of those fail (Take is used beside the engine).
import _ "github.com/liustack/quickjs/internal/unsupported"
