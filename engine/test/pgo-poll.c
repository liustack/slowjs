/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The polls of the generator (poll_count in cutils.h) for the build of
   build.sh pgo, which compiles the engine as the generator does: the
   generator's poll pass turns each call into a count, here it does nothing. */
void goasm_poll_count(int n);
void goasm_poll_entry(int n);

void goasm_poll_count(int n) { (void)n; }
void goasm_poll_entry(int n) { (void)n; }
