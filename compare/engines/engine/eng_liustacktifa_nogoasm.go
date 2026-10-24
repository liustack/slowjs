// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_liustacktifa && slowjs_nogoasm

package engine

// tifaName is the name of the row built with slowjs_nogoasm: slowjs as tifa
// runs it with build.goasm off, so with the interpreter in Go on linux/amd64.
const tifaName = "slowjs-tifa-nogoasm"
