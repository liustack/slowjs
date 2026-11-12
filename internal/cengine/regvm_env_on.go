// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build regvmenv

package cengine

import "os"

// regvmEnv is QUICKJS_REGVM, read once: 1 turns the register loop of
// engine/regvm.h on for every VM New makes, so that whole test suites run
// on it (go test -tags regvmenv with QUICKJS_REGVM=1). A test build only:
// the default build has regvm_env_off.go.
var regvmEnv = os.Getenv("QUICKJS_REGVM") == "1"

func regvmFromEnv(v *VM) {
	if regvmEnv {
		v.enter(func() { JS_SetRegvm(v.sp, v.ctx, 1) })
	}
}
