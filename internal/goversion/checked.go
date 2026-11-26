// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !beastjs_anygo

package goversion

// AnyGo says whether this build was made with -tags beastjs_anygo, which
// lets a Go newer than the checked one build the engine.
const AnyGo = false
