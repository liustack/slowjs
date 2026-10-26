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
    /* This one gives the memory of the heap back (engine/heap.h): a chunk that
       holds a single block (a dedicated chunk), as soon as its block is freed
       or has moved. It runs no user code and does not call back into the engine.
       Its arguments are words, a pointer or a size in the u.ptr of the value
       (HOST_WORD). The address and the size are exactly those of the chunk
       host_memory_grow gave. */
    HOST_CALL_MEMORY_RELEASE = -2, /* (address, size): unmap the chunk. It is already out of the heap, and the result is not read */
};
JSValue host_call(uint32_t vm, JSValueConst this_val, int argc, JSValueConst *argv, int magic);

/* A pointer-sized integer as an argument or a result of the kinds above. The
   tag is JS_TAG_INT and means nothing: the host reads the word. */
#define HOST_WORD(x) JS_MKPTR(JS_TAG_INT, (void *)(uintptr_t)(x))
#define HOST_WORD_VALUE(v) ((uintptr_t)JS_VALUE_GET_PTR(v))

#endif
