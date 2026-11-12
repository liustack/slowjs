// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !regvmenv

package cengine

// regvmFromEnv does nothing in the default build: see regvm_env_on.go.
func regvmFromEnv(*VM) {}
