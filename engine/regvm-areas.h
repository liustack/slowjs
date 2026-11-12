/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The areas of the register loop, in order. regvm.h, regvm-pass.h and
   regvm-run.h read this list once for each part (RV_PART_LIST to
   RV_PART_RUN, in regvm.h), so a new op is written in one area file only:
   its name in RV_PART_LIST, its allow-list case in RV_PART_INFO, its pass
   case in RV_PART_PASS and its handler in RV_PART_RUN. No include guard:
   the files are read several times. */
#include "regvm-ops-basic.h"
#include "regvm-ops-cmp.h"
#include "regvm-ops-object.h"
#include "regvm-ops-arith.h"
#include "regvm-ops-closure.h"
#include "regvm-ops-except.h"
#include "regvm-ops-window.h"
#include "regvm-ops-ctor.h"
#include "regvm-ops-gen.h"
#include "regvm-ops-eval.h"
#include "regvm-ops-args.h"
#include "regvm-ops-old.h"
#include "regvm-ops-wide.h"
