// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && !(linux && amd64)

package slowjs // import "github.com/liustack/slowjs"

// The C engine is generated for linux/amd64 only so far: the tag
// slowjs_cengine on another target stops the build here.
var _ = the_C_engine_of_tag_slowjs_cengine_builds_only_on_linux_amd64_so_far
