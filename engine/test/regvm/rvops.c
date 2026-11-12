/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* rvops checks the table of opcodes of the register loop
   (engine/test/regvm/opcodes.txt) against the opcodes of the engine
   (engine/quickjs-opcode.h) and against the pass:

     rvops engine/test/regvm/opcodes.txt

   Each opcode of quickjs-opcode.h has one line in the table, so a new
   opcode cannot be forgotten. Its path is:

     reg     an op of the register loop of its own
     pass    the abstract stack of the pass, a move or a free at most
     window  a window op (regvm-ops-window.h, or W_GEN of regvm-ops-gen.h)
     old     OLD (regvm-ops-old.h)
     never   in the code of no function that runs: a temporary opcode of
             the compiler, or OP_invalid

   The pass must take (JS_RegvmStat 2000 + op) every opcode of the first
   four: it refuses none of final code.

   A line that starts with ">" is a form: a script whose value is a
   function, which rvops calls once with the register loop on (the call
   converts it) and which must then have register code. A form is written
   under the opcode it takes in a way the pass once refused, or under no
   opcode for a function the pass once refused whole. A line '!"ERROR"
   SOURCE' is a limit of the parser: a script that must throw an error
   whose text starts with ERROR.

   It prints each line that is wrong and exits with 1, 0 when none is, 2
   when it cannot run. */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "quickjs.h"
#include "../host.h"

/* the opcodes in the order of quickjs-opcode.h, as quickjs.c numbers them:
   the DEF ones from 0 (the short ones too: quickjs.c has SHORT_OPCODES),
   the temporary ones (def) apart */
typedef struct Op {
    const char *name;
    int temp;
} Op;

static const Op ops[] = {
#define SHORT_OPCODES 1
#define FMT(f)
#define DEF(id, size, n_pop, n_push, f) { #id, 0 },
#define def(id, size, n_pop, n_push, f) { #id, 1 },
#include "quickjs-opcode.h"
#undef def
#undef DEF
#undef FMT
};

#define NOPS ((int)(sizeof(ops) / sizeof(ops[0])))

static int seen[NOPS];
static int nforms;

/* the limit of the parser at src (after its '!"ERROR" '): 0 when the
   script throws an error whose text starts with err, else 1 with what is
   wrong printed */
static int check_limit(JSContext *ctx, const char *path, int lineno,
                       const char *src, const char *err)
{
    JSValue r, e;
    const char *t;
    int bad;

    nforms++;
    r = JS_Eval(ctx, src, strlen(src), "<limit>", JS_EVAL_TYPE_GLOBAL);
    if (!JS_IsException(r)) {
        printf("%s:%d: the script does not throw, want %s\n", path, lineno, err);
        JS_FreeValue(ctx, r);
        return 1;
    }
    e = JS_GetException(ctx);
    t = JS_ToCString(ctx, e);
    bad = !t || strncmp(t, err, strlen(err)) != 0;
    if (bad)
        printf("%s:%d: the script throws %s, want %s\n", path, lineno, t ? t : "(no text)", err);
    JS_FreeCString(ctx, t);
    JS_FreeValue(ctx, e);
    return bad;
}

/* the form at src (after its ">"): 0 when it converts, else 1 with what is
   wrong printed */
static int check_form(JSContext *ctx, const char *path, int lineno,
                      const char *src)
{
    JSValue f, r;
    int st, bad = 0;

    nforms++;
    f = JS_Eval(ctx, src, strlen(src), "<form>", JS_EVAL_TYPE_GLOBAL);
    if (!JS_IsFunction(ctx, f)) {
        printf("%s:%d: the form is no function\n", path, lineno);
        JS_FreeValue(ctx, JS_GetException(ctx));
        JS_FreeValue(ctx, f);
        return 1;
    }
    r = JS_Call(ctx, f, JS_UNDEFINED, 0, NULL);
    if (JS_IsException(r))
        JS_FreeValue(ctx, JS_GetException(ctx));
    JS_FreeValue(ctx, r);
    st = JS_RegvmFuncStat(ctx, f, 0);
    if (st != 1) {
        printf("%s:%d: the form is refused (opcode %d at %d)\n", path, lineno,
               JS_RegvmFuncStat(ctx, f, 2), JS_RegvmFuncStat(ctx, f, 3));
        bad = 1;
    }
    JS_FreeValue(ctx, f);
    return bad;
}

static int check(void *p)
{
    const char *path = p;
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    FILE *f = fopen(path, "r");
    static char line[8192];
    char name[64], path_kind[16], rest[16], *src;
    int i, op, nop = 0, bad = 0, lineno = 0, taken, n;

    if (!f) {
        fprintf(stderr, "rvops: cannot read %s\n", path);
        return 2;
    }
    /* the number of each DEF opcode is its rank among them */
    for (i = 0; i < NOPS; i++)
        nop += !ops[i].temp;
    if (JS_RegvmStat(ctx, 2000 + nop - 1) == -1 || JS_RegvmStat(ctx, 2000 + nop) != -1) {
        fprintf(stderr, "rvops: the engine has not %d opcodes: quickjs-opcode.h read otherwise\n", nop);
        return 2;
    }
    while (fgets(line, sizeof(line), f)) {
        lineno++;
        if (line[0] == '#' || line[0] == '\n')
            continue;
        if (line[0] == '>') {
            line[strcspn(line, "\n")] = '\0';
            bad += check_form(ctx, path, lineno, line + 1);
            continue;
        }
        if (line[0] == '!') {
            line[strcspn(line, "\n")] = '\0';
            src = line[1] == '"' ? strchr(line + 2, '"') : NULL;
            if (!src || src[1] != ' ') {
                printf("%s:%d: not !\"ERROR\" SOURCE\n", path, lineno);
                bad++;
                continue;
            }
            *src = '\0';
            bad += check_limit(ctx, path, lineno, src + 2, line + 2);
            continue;
        }
        rest[0] = '\0';
        n = sscanf(line, "%63s %15s %15s", name, path_kind, rest);
        if (n < 2 || (rest[0] && rest[0] != '#')) {
            printf("%s:%d: not NAME PATH [# note]\n", path, lineno);
            bad++;
            continue;
        }
        op = 0;
        for (i = 0; i < NOPS; i++) {
            if (!strcmp(ops[i].name, name))
                break;
            op += !ops[i].temp;
        }
        if (i == NOPS) {
            printf("%s:%d: %s is no opcode of quickjs-opcode.h\n", path, lineno, name);
            bad++;
            continue;
        }
        if (seen[i]++) {
            printf("%s:%d: %s again\n", path, lineno, name);
            bad++;
            continue;
        }
        if (!strcmp(path_kind, "never")) {
            if (!ops[i].temp && strcmp(name, "invalid")) {
                printf("%s:%d: %s is in finished code: it needs a path\n", path, lineno, name);
                bad++;
            }
            continue;
        }
        if (ops[i].temp) {
            printf("%s:%d: %s is a temporary opcode of the compiler: never\n", path, lineno, name);
            bad++;
            continue;
        }
        taken = JS_RegvmStat(ctx, 2000 + op);
        if (!strcmp(path_kind, "reg") || !strcmp(path_kind, "pass") ||
            !strcmp(path_kind, "window") || !strcmp(path_kind, "old")) {
            if (taken != 1) {
                printf("%s:%d: %s has no path into the register loop\n", path, lineno, name);
                bad++;
            }
        } else {
            printf("%s:%d: %s: no path %s\n", path, lineno, name, path_kind);
            bad++;
        }
    }
    fclose(f);
    for (i = 0; i < NOPS; i++) {
        if (!seen[i]) {
            printf("%s: %s is not in the table\n", path, ops[i].name);
            bad++;
        }
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    printf("rvops: %d opcodes, %d forms, %d wrong\n", NOPS, nforms, bad);
    return bad != 0;
}

int main(int argc, char **argv)
{
    if (argc != 2) {
        fprintf(stderr, "usage: rvops TABLE\n");
        return 2;
    }
    return host_run_thread(check, argv[1]);
}
