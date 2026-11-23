/*
 * Copyright (c) 2026 The liustack/beastjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE at the
 * root of this repository.
 */
#ifndef BEASTJS_LIBC_ENDIAN_H
#define BEASTJS_LIBC_ENDIAN_H

#define __LITTLE_ENDIAN 1234
#define __BIG_ENDIAN 4321
#define __BYTE_ORDER (__BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__ ? __LITTLE_ENDIAN : __BIG_ENDIAN)

#define LITTLE_ENDIAN __LITTLE_ENDIAN
#define BIG_ENDIAN __BIG_ENDIAN
#define BYTE_ORDER __BYTE_ORDER

#endif
