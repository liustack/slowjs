// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && !(linux && amd64)

package slowjs // import "github.com/liustack/slowjs"

// The C engine is generated for linux/amd64 only so far: the tag
// slowjs_cengine on another target stops the build in this import, before
// the files that need the engine fail.
import _ "github.com/liustack/slowjs/internal/unsupported"
