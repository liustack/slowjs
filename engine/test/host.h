/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_TEST_HOST_H
#define SLOWJS_TEST_HOST_H

#include <sys/time.h>

#include "quickjs.h"

/* What run-test262.c takes from the host: three functions that QuickJS has in
   quickjs-libc.c, which the C tree leaves out (they are copies, see host.c),
   and the runtime of the test host. */
uint8_t *js_load_file(JSContext *ctx, size_t *pbuf_len, const char *filename);
int js_module_test_json(JSContext *ctx, JSValueConst attributes);
void js_std_dump_error(JSContext *ctx);

/* A runtime with the stack limit the host sets, and an interrupt that ends a
   test running over two minutes. The C tree starts runtimes with no stack
   limit, since the Go port counts its own stack slots. */
JSRuntime *host_new_runtime(void);

/* The wall clock of the engine objects, which build.sh compiles with
   gettimeofday renamed to this function: always the same time, so Date and
   the seed of Math.random give the same values in every run. With HOST_CLOCK=
   running in the environment the time starts the same and passes. */
int host_gettimeofday(struct timeval *tv, void *tz);

#endif
