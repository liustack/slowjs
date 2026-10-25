/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/*
 * mathdump SCRIPT NAME...: runs mathcmp.js, which defines runFn(), and prints
 * what runFn(NAME) returns for each NAME. math.sh compares this output with
 * the one of the Go engine.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "host.h"

int main(int argc, char **argv)
{
    JSRuntime *rt;
    JSContext *ctx;
    JSValue v;
    size_t len;
    uint8_t *src;
    int i;

    if (argc < 3) {
        fprintf(stderr, "usage: mathdump SCRIPT NAME...\n");
        return 2;
    }
    src = js_load_file(NULL, &len, argv[1]);
    if (!src) {
        fprintf(stderr, "mathdump: cannot read %s\n", argv[1]);
        return 2;
    }
    rt = host_new_runtime();
    ctx = JS_NewContext(rt);
    v = JS_Eval(ctx, (const char *)src, len, argv[1], JS_EVAL_TYPE_GLOBAL);
    free(src);
    if (JS_IsException(v)) {
        js_std_dump_error(ctx);
        return 1;
    }
    JS_FreeValue(ctx, v);
    for (i = 2; i < argc; i++) {
        char code[128];
        const char *s;
        size_t n;

        snprintf(code, sizeof code, "runFn('%s')", argv[i]);
        v = JS_Eval(ctx, code, strlen(code), "<mathdump>", JS_EVAL_TYPE_GLOBAL);
        if (JS_IsException(v)) {
            js_std_dump_error(ctx);
            return 1;
        }
        s = JS_ToCStringLen(ctx, &n, v);
        if (!s) {
            js_std_dump_error(ctx);
            return 1;
        }
        fwrite(s, 1, n, stdout);
        fputc('\n', stdout);
        JS_FreeCString(ctx, s);
        JS_FreeValue(ctx, v);
    }
    JS_FreeContext(ctx);
    JS_FreeRuntime(rt);
    return 0;
}
