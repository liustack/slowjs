/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/* What musl's src/include/features.h gives its own sources. The build puts
   it in front of every musl file with -include. */
#ifndef SLOWJS_LIBC_FEATURES_H
#define SLOWJS_LIBC_FEATURES_H

#define weak __attribute__((__weak__))
#define hidden __attribute__((__visibility__("hidden")))
#ifdef __APPLE__
/* Mach-O has no alias attribute. The musl files that make a second name for a
   function (strchrnul for __strchrnul, memrchr for __memrchr) only declare
   it here, and nothing in the C tree calls the second name. */
#define weak_alias(old, new) extern __typeof(old) new
#else
/* The name is expanded before it is put in a string, so that a test can build
   the library with its functions renamed. */
#define SLOWJS_STR_(x) #x
#define SLOWJS_STR(x) SLOWJS_STR_(x)
#define weak_alias(old, new) \
	extern __typeof(old) new __attribute__((__weak__, __alias__(SLOWJS_STR(old))))
#endif

#endif
