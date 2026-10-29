// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !((linux && amd64) || (darwin && arm64))

package slowjs // import "github.com/liustack/slowjs"

// The engine runs on linux/amd64 and darwin/arm64 only: another target stops
// the build in this import, before the files that need the engine fail. (linux/arm64 is generated but no machine of
// its own has run it yet.)
import _ "github.com/liustack/slowjs/internal/unsupported"
