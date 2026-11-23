// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !linux

package bench

import "testing"

func skipWherePagesStay(t *testing.T, dir string) {}

func testDropPagesLocked(t *testing.T) {}
