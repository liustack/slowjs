/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_STDATOMIC_H
#define SLOWJS_LIBC_STDATOMIC_H

/* The operations Atomics.* uses, on the compiler's own C11 builtins. */
#define atomic_load(p) __c11_atomic_load(p, __ATOMIC_SEQ_CST)
#define atomic_store(p, v) __c11_atomic_store(p, v, __ATOMIC_SEQ_CST)
#define atomic_exchange(p, v) __c11_atomic_exchange(p, v, __ATOMIC_SEQ_CST)
#define atomic_fetch_add(p, v) __c11_atomic_fetch_add(p, v, __ATOMIC_SEQ_CST)
#define atomic_fetch_sub(p, v) __c11_atomic_fetch_sub(p, v, __ATOMIC_SEQ_CST)
#define atomic_fetch_and(p, v) __c11_atomic_fetch_and(p, v, __ATOMIC_SEQ_CST)
#define atomic_fetch_or(p, v) __c11_atomic_fetch_or(p, v, __ATOMIC_SEQ_CST)
#define atomic_fetch_xor(p, v) __c11_atomic_fetch_xor(p, v, __ATOMIC_SEQ_CST)
#define atomic_compare_exchange_strong(p, expected, desired) \
    __c11_atomic_compare_exchange_strong(p, expected, desired, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST)

#endif
