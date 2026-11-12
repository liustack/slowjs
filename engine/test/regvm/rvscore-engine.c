/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The engine, quickjs.c itself, built with the counting of
   engine/regvm-count.h (-DJS_REGVM_COUNT) and of RV_STATS, and one function
   added at its end for score.sh of this folder: rvscore_dump, which render.c
   calls in place of JS_RegvmDump once its timed renders ran with the register
   loop on.

   The counting build counts each instruction the old loop runs, by function.
   With the register loop on, what the old loop still runs is what the
   register loop does not, so rvscore_dump asks, for each function that ran
   there, why: the pass refused it (and then for what: the whole function, or
   which instructions it does not take), it was never converted, or it is
   converted and was called by a path that does not enter the register loop.
   It weighs each answer by the dispatches the function ran in the old loop,
   a render, and writes the ranking (through js_rv_emit of render.c, to the
   file RV_OUT names).

   It reads the pass's own tables: rv_build, rv_old_info and the refusal of
   rt->rv_stat[3] and [4] (engine/regvm.h). When those change, this file
   follows. Built as the engine is, freestanding, like engine/test/white.c. */

#include "../../quickjs.c"

static const char *const rvscore_names[] = {
#define FMT(f)
#define DEF(id, size, n_pop, n_push, f) #id,
#define def(id, size, n_pop, n_push, f)
#include "quickjs-opcode.h"
#undef def
#undef DEF
#undef FMT
};

static const char *rvscore_op(int op)
{
    static char buf[16];
    if (op >= 0 && op < (int)countof(rvscore_names))
        return rvscore_names[op];
    snprintf(buf, sizeof(buf), "op%d", op);
    return buf;
}

/* a reason a function runs in the old loop, and what it weighs */
typedef struct RVScoreRow {
    char key[64];
    int funcs;
    uint64_t disp;  /* old-loop dispatches, all renders */
    uint64_t sole;  /* of those, in functions where this is the only reason */
    int sole_funcs;
} RVScoreRow;

typedef struct RVScoreTab {
    RVScoreRow row[512];
    int n;
} RVScoreTab;

static RVScoreRow *rvscore_row(RVScoreTab *t, const char *key)
{
    int i;
    for (i = 0; i < t->n; i++)
        if (!strcmp(t->row[i].key, key))
            return &t->row[i];
    if (t->n == (int)countof(t->row))
        return &t->row[t->n - 1];
    memset(&t->row[t->n], 0, sizeof(t->row[0]));
    snprintf(t->row[t->n].key, sizeof(t->row[0].key), "%s", key);
    return &t->row[t->n++];
}

static int rvscore_cmp(const void *a, const void *b)
{
    const RVScoreRow *x = a, *y = b;
    if (x->disp != y->disp)
        return x->disp < y->disp ? 1 : -1;
    return strcmp(x->key, y->key);
}

static void rvscore_print(const char *title, RVScoreTab *t, uint64_t total, uint64_t n, int with_sole)
{
    int i;
    qsort(t->row, t->n, sizeof(t->row[0]), rvscore_cmp);
    rv_printf("\n%s\n", title);
    if (with_sole)
        rv_printf("  %-34s %6s %14s %8s %14s %6s\n", "", "funcs", "dispatches", "share", "only this", "funcs");
    else
        rv_printf("  %-34s %6s %14s %8s\n", "", "funcs", "dispatches", "share");
    for (i = 0; i < t->n; i++) {
        RVScoreRow *r = &t->row[i];
        /* the engine's printf has no floating point: shares in hundredths of
           a percent */
        uint64_t bp = total ? r->disp * 10000 / total : 0;
        if (with_sole)
            rv_printf("  %-34s %6d %14llu %4llu.%02llu%% %14llu %6d\n", r->key, r->funcs,
                      (unsigned long long)(r->disp / n), (unsigned long long)(bp / 100),
                      (unsigned long long)(bp % 100), (unsigned long long)(r->sole / n), r->sole_funcs);
        else
            rv_printf("  %-34s %6d %14llu %4llu.%02llu%%\n", r->key, r->funcs,
                      (unsigned long long)(r->disp / n), (unsigned long long)(bp / 100),
                      (unsigned long long)(bp % 100));
    }
}

/* the reasons of rv_build for a whole function that the bytecode tells
   (rt->rv_stat[4] when rv_stat[3] is -1). Every kind of function folds a
   stack too deep for the temps (regvm-fold.h): when that fails too,
   rv_build names it (bit 64, rvscore_blockers). */
static int rvscore_whole(JSFunctionBytecode *b, const char **why, int max)
{
    int n = 0;
    if (b->arg_count + 2 + b->var_count > RV_FAR_MAX && n < max)
        why[n++] = "function: over 32 K locals and arguments";
    return n;
}

/* the renders the counts are of (rvscore-host.c, from RVSCORE_RENDERS) */
int rvscore_renders(void);

/* a refused function: its dispatches and its blockers (rows of the table of
   blockers) */
typedef struct RVScoreFunc {
    uint64_t d;
    JSFunctionBytecode *fb;
    char key[64];
    int nb;
    int16_t b[64];
} RVScoreFunc;

static int rvscore_func_cmp(const void *a, const void *b)
{
    const RVScoreFunc *x = a, *y = b;
    return x->d < y->d ? 1 : x->d > y->d ? -1 : 0;
}

/* the blockers of a refused function b into r, as rows of ops: the reasons
   of the whole function, every op rv_old_info does not take, and the op
   rv_build stopped at when rv_old_info takes it (refused in the form it has
   there). key gets what rv_build stopped at. */
static void rvscore_blockers(JSContext *ctx, JSFunctionBytecode *b, RVScoreTab *ops,
                             RVScoreFunc *r, char *key, size_t key_size)
{
    JSRuntime *rt = ctx->rt;
    const char *why[8];
    int nwhy = rvscore_whole(b, why, 8), k, pos, size;
    RVFunc *rf;
    char name[48];

    rt->rv_stat[3] = -99;
    rt->rv_stat[4] = 0;
    rf = rv_build(rt, b, NULL);
    if (!rf && rt->rv_stat[3] == -1 && (rt->rv_stat[4] & 64) && nwhy < 8)
        why[nwhy++] = "function: stack too deep to fold";
    if (rf) {
        /* it converts now: it was refused for a failed allocation */
        rv_free_func(rt, rf);
        snprintf(key, key_size, "refused, converts now");
    } else if (rt->rv_stat[3] == -1) {
        snprintf(key, key_size, "refused: %s", nwhy ? why[0] : "function: other");
    } else if (rt->rv_stat[3] == -3) {
        snprintf(key, key_size, "refused: code no compiler makes");
    } else if (rt->rv_stat[3] == -99) {
        snprintf(key, key_size, "refused: jumps, frame or memory");
    } else {
        snprintf(key, key_size, "refused: op %s", rvscore_op(rt->rv_stat[3]));
    }
    r->nb = 0;
#define RVSCORE_ADD(s) do {                                             \
        int id_ = (int)(rvscore_row(ops, (s)) - ops->row), j_;          \
        for (j_ = 0; j_ < r->nb && r->b[j_] != id_; j_++)              \
            ;                                                           \
        if (j_ == r->nb && r->nb < 64)                                  \
            r->b[r->nb++] = id_;                                        \
    } while (0)
    for (k = 0; k < nwhy; k++)
        RVSCORE_ADD(why[k]);
    for (pos = 0; pos < (int)b->byte_code_len; pos += size) {
        int npop, npush, term, target, op = b->byte_code_buf[pos];
        size = short_opcode_info(op).size;
        if (size <= 0)
            break;
        if (rv_old_info(b->byte_code_buf, pos, &npop, &npush, &term, &target) < 0)
            RVSCORE_ADD(rvscore_op(op));
    }
    if (rt->rv_stat[3] >= 0) {
        int j;
        const char *op = rvscore_op(rt->rv_stat[3]);
        for (j = 0; j < r->nb && strcmp(ops->row[r->b[j]].key, op); j++)
            ;
        if (j == r->nb) {
            snprintf(name, sizeof(name), "%s (form)", op);
            RVSCORE_ADD(name);
        }
    }
    if (r->nb == 0 && rt->rv_stat[3] < 0)
        RVSCORE_ADD(key);
#undef RVSCORE_ADD
}

void rvscore_dump(JSRuntime *rt)
{
    uint64_t n = rvscore_renders();
    JSContext *ctx = list_entry(rt->context_list.next, JSContext, link);
    RVScoreTab *state = js_rv_alloc(sizeof(RVScoreTab));
    RVScoreTab *ops = js_rv_alloc(sizeof(RVScoreTab));
    RVCountFunc *f;
    RVScoreFunc *rf;
    uint64_t total = 0, freed_all = 0;
    int nfuncs = 0, nrf = 0, cap = 0, i, k, step;
    uint8_t *chosen, *freed;

    for (f = rv_all; f; f = f->all_next)
        cap++;
    rf = js_rv_alloc(sizeof(RVScoreFunc) * (cap + 1));
    for (f = rv_all; f; f = f->all_next) {
        JSFunctionBytecode *b = f->b;
        uint64_t d = 0;
        uint32_t j;
        char key[64];
        RVScoreRow *r;

        if (!b)
            continue; /* freed during the renders: its counts went out then */
        for (j = 0; j < f->len; j++)
            d += f->n[j];
        if (!d)
            continue;
        total += d;
        nfuncs++;
        if (b->rv == RV_OFF) {
            snprintf(key, sizeof(key), "made with the loop off");
        } else if (b->rv == RV_UNCONVERTED) {
            snprintf(key, sizeof(key), "never converted");
        } else if (b->rv->arg_count >= 0) {
            snprintf(key, sizeof(key), "converted, ran in the old loop");
        } else {
            rf[nrf].d = d;
            rf[nrf].fb = b;
            rvscore_blockers(ctx, b, ops, &rf[nrf], key, sizeof(key));
            snprintf(rf[nrf].key, sizeof(rf[nrf].key), "%s", key);
            for (k = 0; k < rf[nrf].nb; k++) {
                r = &ops->row[rf[nrf].b[k]];
                r->funcs++;
                r->disp += d;
                if (rf[nrf].nb == 1) {
                    r->sole += d;
                    r->sole_funcs++;
                }
            }
            nrf++;
        }
        r = rvscore_row(state, key);
        r->funcs++;
        r->disp += d;
    }
    rv_printf("old loop: %llu dispatches a render in %d functions (counted over %llu renders)\n",
              (unsigned long long)(total / n), nfuncs, (unsigned long long)n);
    rvscore_print("Why the functions that run in the old loop are there (refused: what the pass stopped at first):",
                  state, total, n, 0);

    /* the order to take the blockers in: at each step the one that frees
       the most dispatches with those taken before, or when none frees any
       alone, the one in the most blocked dispatches */
    chosen = js_rv_alloc(ops->n + 1);
    freed = js_rv_alloc(nrf + 1);
    rv_printf("\nThe blockers in the order that frees the most dispatches (each step with the ones above it taken; "
              "\"in\" when no single one frees any, then the one in the most blocked dispatches is taken):\n");
    rv_printf("  %-4s %-34s %14s %14s %8s\n", "step", "blocker", "frees", "all freed", "of old");
    for (step = 1; step <= 60 && freed_all < total; step++) {
        int best = -1, how = 0;
        uint64_t best_gain = 0, best_in = 0;
        for (k = 0; k < ops->n; k++) {
            uint64_t gain = 0, in = 0;
            if (chosen[k])
                continue;
            for (i = 0; i < nrf; i++) {
                int j, left = 0, has = 0;
                if (freed[i])
                    continue;
                for (j = 0; j < rf[i].nb; j++) {
                    if (rf[i].b[j] == k)
                        has = 1;
                    else if (!chosen[rf[i].b[j]])
                        left++;
                }
                if (has) {
                    in += rf[i].d;
                    if (!left)
                        gain += rf[i].d;
                }
            }
            if (gain > best_gain || (best_gain == 0 && gain == 0 && in > best_in)) {
                best = k;
                best_gain = gain;
                best_in = in;
            }
        }
        if (best < 0)
            break;
        how = best_gain > 0;
        chosen[best] = 1;
        for (i = 0; i < nrf; i++) {
            int j;
            if (freed[i])
                continue;
            for (j = 0; j < rf[i].nb && chosen[rf[i].b[j]]; j++)
                ;
            if (j == rf[i].nb) {
                freed[i] = 1;
                freed_all += rf[i].d;
            }
        }
        {
            uint64_t bp = total ? freed_all * 10000 / total : 0;
            rv_printf("  %-4d %-34s %14llu %14llu %4llu.%02llu%%%s\n", step, ops->row[best].key,
                      (unsigned long long)(best_gain / n), (unsigned long long)(freed_all / n),
                      (unsigned long long)(bp / 100), (unsigned long long)(bp % 100),
                      how ? "" : "  in");
        }
    }
    /* the refused functions that run the most, with their blockers */
    qsort(rf, nrf, sizeof(rf[0]), rvscore_func_cmp);
    rv_printf("\nThe refused functions that run the most (name, first line, dispatches, blockers):\n");
    for (i = 0; i < nrf && i < 30; i++) {
        JSFunctionBytecode *b = rf[i].fb;
        char nb[ATOM_GET_STR_BUF_SIZE];
        int line = 0;
        if (b->has_debug && b->debug.pc2line_buf) {
            uint32_t v;
            if (get_leb128(&v, b->debug.pc2line_buf, b->debug.pc2line_buf + b->debug.pc2line_len) > 0)
                line = v + 1;
        }
        rv_printf("  %-24s %6d %12llu ", JS_AtomGetStrRT(rt, nb, sizeof(nb), b->func_name), line,
                  (unsigned long long)(rf[i].d / n));
        for (k = 0; k < rf[i].nb; k++)
            rv_printf("%s%s", k ? ", " : "", ops->row[rf[i].b[k]].key);
        rv_printf("\n");
    }
    /* last: it sorts the rows of ops, which the lists above refer to */
    rvscore_print("Each blocker, with the dispatches of the refused functions it is in (a function counts under "
                  "each of its blockers), and of those where it is the only one:", ops, total, n, 1);
}
