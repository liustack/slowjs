/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Forced on a program of engine/test (-include rvhook.h, see build.sh of this
   folder), it makes every context the program makes turn the register loop
   on (JS_SetRegvm) when RV_REGVM=1 is in the environment, and leaves the
   program as it is otherwise. So run-test262, unit and unit-oom run the same
   tests with each loop, without a change to their source.

   With RV_REGVM_STATS=1 too, it adds up the counts of JS_RegvmStat of every
   runtime (read when the first context of a runtime is freed) and prints
   them on stderr at exit: functions converted and refused, and, in a build
   with -DRV_STATS, the dispatches of each loop.

   RV_REGVM=2 is 1 with the record of the functions that run in the old
   loop all the same (JS_SetRegvm 2): when the first context of a runtime is
   freed, its lines "fallback ..." (rvfallback.h) are appended to the file
   RV_FALLBACK_LOG, or written on stderr without it, and
   engine/test/regvm/fallbacks.sh sums them. RV_REGVM=3 makes each such run
   an error (JS_SetRegvm 3). */
#ifndef RVHOOK_H
#define RVHOOK_H

#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include "quickjs.h"
#include "regvm/rvfallback.h"

static int rvhook_on = -1, rvhook_stats;
static FILE *rvhook_fblog;
static pthread_mutex_t rvhook_lock = PTHREAD_MUTEX_INITIALIZER;
static long long rvhook_sum[4]; /* converted, refused, dispatches new, old */
static struct { JSRuntime *rt; JSContext *ctx; } rvhook_first[256];

static void rvhook_report(void)
{
    fprintf(stderr, "rvhook: converted %lld refused %lld, dispatches new %lld old %lld (%.2f%% in the register loop)\n",
            rvhook_sum[0], rvhook_sum[1], rvhook_sum[2], rvhook_sum[3],
            rvhook_sum[2] + rvhook_sum[3] ? 100.0 * rvhook_sum[2] / (rvhook_sum[2] + rvhook_sum[3]) : 0.0);
}

static inline JSContext *rvhook_new_context(JSRuntime *rt)
{
    JSContext *ctx = JS_NewContext(rt);
    int i, seen = 0, free_slot = -1;

    pthread_mutex_lock(&rvhook_lock);
    if (rvhook_on < 0) {
        const char *e = getenv("RV_REGVM"), *s = getenv("RV_REGVM_STATS");
        rvhook_on = e && *e >= '1' && *e <= '3' ? *e - '0' : 0;
        if (rvhook_on == 2) {
            const char *l = getenv("RV_FALLBACK_LOG");
            rvhook_fblog = l ? fopen(l, "a") : NULL;
            if (rvhook_fblog == NULL)
                rvhook_fblog = stderr;
        }
        rvhook_stats = rvhook_on && s && *s == '1';
        if (rvhook_stats)
            atexit(rvhook_report);
    }
    if (ctx && (rvhook_stats || rvhook_on == 2)) {
        for (i = 0; i < (int)(sizeof(rvhook_first) / sizeof(rvhook_first[0])); i++) {
            if (rvhook_first[i].rt == rt)
                seen = 1;
            else if (!rvhook_first[i].rt && free_slot < 0)
                free_slot = i;
        }
        if (!seen && free_slot >= 0) {
            rvhook_first[free_slot].rt = rt;
            rvhook_first[free_slot].ctx = ctx;
        }
    }
    pthread_mutex_unlock(&rvhook_lock);
    if (ctx && rvhook_on)
        JS_SetRegvm(ctx, rvhook_on);
    return ctx;
}

static inline void rvhook_free_context(JSContext *ctx)
{
    int i;

    if (rvhook_stats || rvhook_on == 2) {
        pthread_mutex_lock(&rvhook_lock);
        for (i = 0; i < (int)(sizeof(rvhook_first) / sizeof(rvhook_first[0])); i++) {
            if (rvhook_first[i].ctx == ctx) {
                if (rvhook_on == 2) {
                    JS_RegvmFallbacks(JS_GetRuntime(ctx), rvfb_print, rvhook_fblog);
                    fflush(rvhook_fblog);
                }
                rvhook_sum[0] += JS_RegvmStat(ctx, 0);
                rvhook_sum[1] += JS_RegvmStat(ctx, 1);
                rvhook_sum[2] += JS_RegvmStat(ctx, 10);
                rvhook_sum[3] += JS_RegvmStat(ctx, 11);
                rvhook_first[i].rt = NULL;
                rvhook_first[i].ctx = NULL;
            }
        }
        pthread_mutex_unlock(&rvhook_lock);
    }
    JS_FreeContext(ctx);
}

#define JS_NewContext rvhook_new_context
#define JS_FreeContext rvhook_free_context

#endif
