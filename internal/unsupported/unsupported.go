// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !(((linux || darwin) && (amd64 || arm64)) || (windows && amd64))

package unsupported

// The C engine runs on linux and darwin, on amd64 and arm64, and on
// windows/amd64, only. Any other target stops here, with this name in the
// error.
var _ = the_C_engine_runs_only_on_linux_and_darwin_on_amd64_and_arm64_and_on_windows_amd64_this_target_is_not_supported
