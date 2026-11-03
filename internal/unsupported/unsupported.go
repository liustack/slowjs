// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !((linux && (amd64 || arm64)) || (darwin && arm64))

package unsupported

// The C engine runs on linux/amd64, linux/arm64 and darwin/arm64 only. Any
// other target stops here, with this name in the error.
var _ = the_C_engine_runs_only_on_linux_amd64_linux_arm64_and_darwin_arm64_this_target_is_not_supported
