// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && !((linux && (amd64 || arm64)) || (darwin && arm64))

package unsupported

// The C engine is generated for linux/amd64, linux/arm64 and darwin/arm64
// only so far.
var _ = the_C_engine_of_tag_slowjs_cengine_builds_only_on_linux_amd64_linux_arm64_and_darwin_arm64_so_far
