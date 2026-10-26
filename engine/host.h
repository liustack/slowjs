/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_HOST_H
#define SLOWJS_HOST_H

#include <stdint.h>

#include "quickjs.h"

/* What the engine asks of its host. The engine has no other way out: no system
   calls, no libc beyond engine/libc. The native test host in engine/test
   implements these, and the Go host implements them for the generated
   assembly. */

/* The kinds of time host_now gives. */
enum {
    HOST_NOW_WALL = 0,      /* nanoseconds since 1970-01-01T00:00:00Z, on the clock set for the VM */
    HOST_NOW_MONOTONIC = 1, /* nanoseconds from any fixed start, never going back */
};

/* vm names the VM whose clock is read. It is 0 until JS_NewRuntime takes a VM. */
int64_t host_now(uint32_t vm, int kind);

/* Calls a function of the host: one it registered, a Builtin, or Intl and
   Temporal. magic says which, and is 0 or more for those. The result is the
   value of the call or JS_EXCEPTION. A magic below 0 is a request of the
   engine itself, and the host decides what to do with it. */
enum {
    HOST_CALL_ABORT = -1, /* the engine met a state it cannot go on from. It does not return */
};
JSValue host_call(uint32_t vm, JSValueConst this_val, int argc, JSValueConst *argv, int magic);

#endif
