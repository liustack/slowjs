/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The list of the functions that ran in the old loop with JS_SetRegvm(ctx,
   2), one line each (JS_RegvmFallbacks), for jsrun -regvm-why and for the
   programs of rvhook.h with RV_REGVM=2:

     fallback <TAB> reason <TAB> detail <TAB> name <TAB> file:line:col <TAB> runs

   reason is one of the engine (engine/regvm-entry.h): refused, out of
   memory, constructor, generator or async, other. detail says what the
   pass of a refused function stops at: an opcode of the old loop, or the
   rule that refuses the whole function. engine/test/regvm/fallbacks.sh sums
   such lines by reason. */
#ifndef RVFALLBACK_H
#define RVFALLBACK_H

#include <stdio.h>
#include <string.h>
#include "quickjs.h"

#ifndef SHORT_OPCODES
#define SHORT_OPCODES 1
#endif
static const char *const rvfb_opnames[] = {
#define FMT(f)
#define DEF(id, size, n_pop, n_push, f) #id,
#define def(id, size, n_pop, n_push, f)
#include "quickjs-opcode.h"
#undef def
#undef DEF
#undef FMT
};

static void rvfb_detail(const JSRegvmFallback *f, char *buf, size_t size)
{
    buf[0] = 0;
    if (f->reason[0] != 'r') /* not refused */
        return;
    if (f->op == -1)
        snprintf(buf, size, "%s%s%s%s",
                 f->pos & 1 ? "not a normal function " : "", f->pos & 2 ? "sloppy " : "",
                 f->pos & 8 ? "stack over 32 " : "",
                 f->pos == 0 ? "frame over 127 slots or code over 30000 bytes" : "");
    else if (f->op == -3)
        snprintf(buf, size, "code no compiler makes");
    else if (f->op == -99)
        snprintf(buf, size, "its jumps or its frame");
    else if (f->op >= 0 && f->op < (int)(sizeof(rvfb_opnames) / sizeof(rvfb_opnames[0])))
        snprintf(buf, size, "%s", rvfb_opnames[f->op]);
    else
        snprintf(buf, size, "op %d", f->op);
    {
        size_t n = strlen(buf);
        while (n > 0 && buf[n - 1] == ' ')
            buf[--n] = 0;
    }
}

static int rvfb_print(void *opaque, const JSRegvmFallback *f)
{
    char detail[96];

    rvfb_detail(f, detail, sizeof(detail));
    fprintf((FILE *)opaque, "fallback\t%s\t%s\t%s\t%s:%d:%d\t%lld\n", f->reason, detail,
            *f->name ? f->name : "<anonymous>", f->file, f->line, f->col, (long long)f->calls);
    return 0;
}

#endif
