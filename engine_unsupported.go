// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !((linux || darwin) && (amd64 || arm64))

package slowjs // import "github.com/liustack/slowjs"

// The engine runs on linux and darwin, on amd64 and arm64, only: another
// target stops the build in this import, before the files that need the
// engine fail.
import _ "github.com/liustack/slowjs/internal/unsupported"
