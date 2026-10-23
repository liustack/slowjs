// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !(((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package libquickjs // import "github.com/liustack/slowjs/lib"

// The engine is generated for darwin and linux on amd64 and arm64, and for
// windows on amd64. Building for any other target stops here: the import
// below does not exist, and the go command reports it, with the message in its
// name, before it compiles anything. Without this file such a build would
// print a page of undefined names.
import _ "this-target-is-not-supported/use-darwin-or-linux-on-amd64-or-arm64-or-windows-on-amd64"
