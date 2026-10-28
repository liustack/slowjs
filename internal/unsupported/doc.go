// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package unsupported stops a build with the tag slowjs_cengine on a target
// the C engine does not run on: it runs on linux/amd64 and darwin/arm64 only.
// The root package imports it there only (engine_unsupported.go): the go
// command builds it first, and its error is the only one the build reports,
// before the files of the root package that need the engine fail too.
package unsupported
