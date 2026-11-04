// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package goversion stops a build with a Go newer than the one the engine was
// checked with. The Go assembly of internal/cengine leans on how Go starts a
// function and handles a fault, which the Go 1 promise does not cover, so a
// newer Go builds it without checking it (see docs/upgrading.md).
// internal/cengine imports it: the go command builds it first, and its error
// is the only one the build reports. Build with -tags slowjs_anygo to build
// anyway.
package goversion
