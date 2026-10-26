/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_TEST_HOST_H
#define SLOWJS_TEST_HOST_H

#include "../host.h"
#include "quickjs.h"

/* What run-test262.c takes from the host: three functions that QuickJS has in
   quickjs-libc.c, which the C tree leaves out (they are copies, see host.c),
   and the runtime of the test host. */
uint8_t *js_load_file(JSContext *ctx, size_t *pbuf_len, const char *filename);
int js_module_test_json(JSContext *ctx, JSValueConst attributes);
void js_std_dump_error(JSContext *ctx);

/* A runtime with the stack limit the host sets, and an interrupt that ends a
   test running over two minutes. The C tree starts runtimes with no stack
   limit, since the Go port counts its own stack slots. The host maps the
   memory of the heap (host_memory_grow of heap.h) for it. */
JSRuntime *host_new_runtime(void);

/* host_new_runtime with the seed of Math.random of your choice. The one above
   gives the seed that matches the fixed time, 1767225600000000. */
JSRuntime *host_new_runtime_seed(uint64_t seed);

/* How many times the engine asked for the wall clock in this thread. A test
   uses it to see that the engine asks for it only when a script does. */
extern _Thread_local long host_wall_reads;

/* The VM host_now was last asked about for the wall clock in this thread, and
   the VM of a runtime made by this host. */
extern _Thread_local uint32_t host_wall_vm;
uint32_t host_vm_id(JSRuntime *rt);

/* JS_FreeRuntime, then the memory of the runtime goes back to the system. A
   runtime that still has blocks in use after it freed everything is a leak:
   the host says so, and the process exits with status 87. */
void host_free_runtime(JSRuntime *rt);

/* What Go does for VM.ReleaseMemory: JS_ReleaseStart, unmap the chunks
   JS_ReleaseNextChunk gives, take the pages of the ranges JS_ReleaseNextRange
   gives. The sizes in bytes, before and after rounding to pages, are stored
   in *candidate and *released (either may be NULL). */
void host_release_memory(JSRuntime *rt, size_t keep, size_t *candidate, size_t *released);

/* What a test of the host counts: the VMs the host still holds and the bytes
   their chunks map. A runtime that was made and freed leaves neither. A
   thread that makes a runtime maps its chunks without the lock of the list,
   so call these when no other thread is making or freeing one. */
size_t host_live_vms(void);
size_t host_mapped_bytes(void);
#endif
