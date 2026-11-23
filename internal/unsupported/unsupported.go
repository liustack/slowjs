// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !((((linux && !android) || (darwin && !ios)) && (amd64 || arm64)) || (windows && (amd64 || arm64)))

package unsupported

// The C engine runs on linux, darwin and windows, on amd64 and arm64, only.
// Any other target stops here, with this name in the
// error.
var _ = the_C_engine_runs_only_on_linux_darwin_and_windows_on_amd64_and_arm64_this_target_is_not_supported
