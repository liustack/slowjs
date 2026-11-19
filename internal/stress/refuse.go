// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !testhooks

package main

import "errors"

// refuse needs the tag testhooks: this build cannot make the OS refuse.
func refuse(perMille int, seed uint64, refused func()) (undo func(), err error) {
	return nil, errors.New("stress: -refuse needs a build with -tags testhooks")
}
