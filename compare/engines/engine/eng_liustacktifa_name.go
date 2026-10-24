// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_liustacktifa && !slowjs_nogoasm

package engine

// tifaName is the name of the row: slowjs as tifa runs it, with the default
// interpreter of the platform (the Go assembly one on linux/amd64).
const tifaName = "liustack-tifa"
