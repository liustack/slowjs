// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_liustackpolyfill

package engine

import "github.com/liustack/slowjs"

const liustackName = "liustack-slowjs-polyfill"

var liustackNotes = []string{
	"The bare engine: none of the packages intl, temporal and url (or any other package that adds a global) is linked or registered, so the run gets the polyfills an engine without the features gets (tifa/run.sh).",
}

// registerGoPackages registers nothing: this build is the engine alone.
func registerGoPackages(*slowjs.VM) error { return nil }
