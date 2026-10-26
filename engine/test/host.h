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

/* A runtime with an interrupt that ends a test running over two minutes, and
   the call depth the engine starts with (JS_SetMaxCallDepth). The host maps
   the memory of the heap (host_memory_grow of heap.h) for it. */
JSRuntime *host_new_runtime(void);

/* Asks, from any thread, that the script running in rt stop: the thread
   that runs it sets the interrupt flag at its next host_poll, and the engine
   throws "interrupted". The thread that runs rt sets the flag itself. */
void host_interrupt(JSRuntime *rt);

/* The engine built for this host (JS_HOST_POLL, see poll_flag in cutils.h)
   calls this each time it reads a flag that stops it, and reads what it
   returns, *flag. When a stop of the runtime whose interrupt flag it is was
   asked, it sets the flag first. */
int host_poll(const volatile int *flag);

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

/* When a test sets it to n, the nth time from then that the engine asks how
   much stack is left in this thread (__shadow_remaining) it hears none, so
   that the count asking fails there as on a stack too short. It is 0 once
   that happened, and 0 asks nothing. */
extern _Thread_local long host_stack_fail_at;

/* The stack of a thread that runs the engine: enough for the deepest
   recursion JS_SetMaxCallDepth allows, under the sanitizers too. The pages
   are only reserved until a deep recursion touches them. */
#define HOST_THREAD_STACK ((size_t)512 << 20)

/* Runs fn(arg) in a thread with a stack of HOST_THREAD_STACK bytes and
   returns what it returns. A main thread has 8 MiB. */
int host_run_thread(int (*fn)(void *), void *arg);

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
