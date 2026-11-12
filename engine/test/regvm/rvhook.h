/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Forced on a program of engine/test (-include rvhook.h, see build.sh of this
   folder), it leaves the program as it is unless RV_REGVM_STATS=1 is in the
   environment: then it adds up the counts of
   JS_RegvmStat of every runtime (read when the first context of a runtime
   is freed) and prints them on stderr at exit: functions converted and
   refused, and, in a build with -DRV_STATS, the dispatches. */
#ifndef RVHOOK_H
#define RVHOOK_H

#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include "quickjs.h"

static int rvhook_init, rvhook_stats;
static pthread_mutex_t rvhook_lock = PTHREAD_MUTEX_INITIALIZER;
static long long rvhook_sum[3]; /* converted, refused, dispatches */
static struct { JSRuntime *rt; JSContext *ctx; } rvhook_first[256];

static void rvhook_report(void)
{
    fprintf(stderr, "rvhook: converted %lld refused %lld, dispatches %lld\n",
            rvhook_sum[0], rvhook_sum[1], rvhook_sum[2]);
}

static inline JSContext *rvhook_new_context(JSRuntime *rt)
{
    JSContext *ctx = JS_NewContext(rt);
    int i, seen = 0, free_slot = -1;

    pthread_mutex_lock(&rvhook_lock);
    if (!rvhook_init) {
        const char *s = getenv("RV_REGVM_STATS");
        rvhook_init = 1;
        rvhook_stats = s && *s == '1';
        if (rvhook_stats)
            atexit(rvhook_report);
    }
    if (ctx && rvhook_stats) {
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
    return ctx;
}

static inline void rvhook_free_context(JSContext *ctx)
{
    int i;

    if (rvhook_stats) {
        pthread_mutex_lock(&rvhook_lock);
        for (i = 0; i < (int)(sizeof(rvhook_first) / sizeof(rvhook_first[0])); i++) {
            if (rvhook_first[i].ctx == ctx) {
                rvhook_sum[0] += JS_RegvmStat(ctx, 0);
                rvhook_sum[1] += JS_RegvmStat(ctx, 1);
                rvhook_sum[2] += JS_RegvmStat(ctx, 10);
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
