// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Package unsupported stops a build on a target the C engine does not run
// on: it runs on linux, darwin and windows, on amd64 and arm64, only. android
// and ios, which Go builds with the files of linux and of darwin, are not among
// them, and every build line that names the targets says so.
// The root package imports it there only (engine_unsupported.go): the go
// command builds it first, and its error is the only one the build reports,
// before the files of the root package that need the engine fail too.
package unsupported
