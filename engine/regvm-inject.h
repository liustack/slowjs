/*
 * Copyright (c) 2026 The liustack/beastjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE at the
 * root of this repository.
 */

/* The throw-injection test of the register loop, only in a build with
   -DRV_INJECT (engine/test/regvm/inject.sh runs it). Included by quickjs.c
   before JS_CallInternal; the side of the register loop is rv_inj_new in
   regvm-run.h and the hooks RV_INJ_HIT in the handlers and their slow sides.

   With the register loop on, the n-th place where an instruction of the
   register loop can throw, counted as it runs, throws an InternalError "rv
   inject". The instruction is not thrown at from outside: its own handler
   runs, takes its slow side whatever its operands are (RV_INJ_ARMED turns
   the fast paths off for that one instruction), and at the place where the
   slow side calls into the engine a hook (RV_INJ_HIT) fails in place of
   the call, with the operands the call would have taken freed as the call
   frees them when it fails. From there on everything is the code that runs
   for a real error: what the handler frees after a failed call, the pc it
   leaves, and the unwinding by the pcmap. So a handler that frees a temp
   its pcmap entry also names is a double free, and one that leaves a temp
   the entry does not name is a leak, under ASan and in the counts of the
   runtime, whatever the mask says: the test does not read the mask.

   An instruction has one place (an instruction that can throw by itself or
   reads the interrupt flag) or two (a compare and branch, CASES and GORET:
   the instruction itself, then the reading of the flag), told by the hooks
   field of its pcmap entry (regvm-func.h), and each is a number of its own.
   A compare and branch to throw at its second place takes its fast path
   where its operands have one (RV_INJ_SLOW), so that the reading of the
   flag on that path is thrown at too: its slow side reads it in another
   place (rv_jump_slow).
   THROW and THROWE throw at every run and CATCH never does: the test adds
   nothing to them.
   An instruction that ran to its end with no hook taken is a hole of the
   test: the result is "miss" with its name, and inject.sh fails.

   The hooks taken are those of the instruction that is to throw, in its
   own run of the register loop: JS it runs between its two places (a
   getter under WITH, a valueOf under a compare and branch) runs in a run
   of the loop of its own, which puts the injection aside while it runs
   (RV_INJ_ENTER and RV_INJ_LEAVE in JS_RegvmRun), so that a hook of that
   JS neither throws in its place nor counts one down, and a hook taken in
   another frame than the one the instruction was armed in throws nothing
   and gives the result "outside NAME", which fails inject.sh as a miss does.
   The result then says which hook threw, "hit NAME P" on a line of its
   own: the P-th place of the instruction NAME.

   The place is asked for by its number in the run, or by its site: the
   j-th place run at the s-th old instruction of a function, the sites
   numbered as the run first comes to them. A script that runs more places
   than a test has time for is sampled by site, so that an instruction run
   a few times is not lost among the runs of a hot loop.

   It records where that was, as "at FUNC POS K": the function (its number
   among the functions the run made, in the order they were made: the place
   where a function is defined does not tell two functions apart, every
   class field initializer is defined at 1:1), the old instruction of the
   register instruction, and how many times that old instruction ran so
   far. The key names the same place in every run of the script, and the
   goldens of inject.sh hold it. It is given when the register instruction
   is the first one of its old instruction (an old instruction may give
   more than one) and the place is the instruction itself, not the
   interrupt of a conditional jump, which QuickJS's interpreter read after
   it had jumped, and not a case of a switch on strings, which it ran in
   the dispatch of the case before it. For the others the key is "nokey". */

#ifdef RV_INJECT

/* an old instruction of a function (func is not 0 once the entry is used) */
typedef struct RVInjCount {
    uint32_t func;
    int pos;
    uint32_t n;      /* how many times it ran so far, in either loop */
    uint32_t site;   /* its number among the sites of the register loop */
    uint32_t places; /* the places to throw run at it so far */
} RVInjCount;

#define RV_INJ_TAB (1 << 20)
static RVInjCount rv_inj_tab[RV_INJ_TAB];
static int rv_inj_used;

static int rv_inj_any;          /* an injection is asked for */
static long rv_inj_n, rv_inj_seen; /* the register loop: the n-th, and so far */
static uint32_t rv_inj_site, rv_inj_sites; /* or at this site, and the sites so far */
static long rv_inj_site_j;      /* its j-th place */
static uint32_t rv_inj_funcs;   /* the functions made so far */
static int rv_inj_fired;
/* the register loop: the instruction being run is to throw at the hook it
   comes to after this many (1: the first one), 0 when none is */
static int rv_inj_armed;
static int rv_inj_place;        /* rv_inj_armed when it was set: the place */
static const JSStackFrame *rv_inj_frame; /* the frame of its run then */
static int rv_inj_just;         /* a hook threw in the instruction being run */
static int rv_inj_op;           /* the register op that is to throw */
static const uint8_t *rv_inj_pc;
static char rv_inj_result[256];
static char rv_inj_key[96];     /* the result once the hook has thrown */

/* The cover mode (JS_RegvmInjectCover): nothing throws. Each instruction
   with a place P (1 or 2, one run of the script for each) is armed as an
   injection at its place P arms it, and the hook the injection would throw
   at is written down in place of the throw, and the call into the engine
   after it runs: the hooks the runs of the script take are those its
   injections throw at. A hook is told by the op, the old opcode under it for OLD, WITH
   and W_GEN (rv_inj_sub), and the hook: its file and line and its
   __COUNTER__, which tells apart the hooks of one macro of a handler.
   inject-cover.sh lists the hooks no script of a set comes to. */
static int rv_inj_cover;        /* the cover mode is on: the place P */
static long rv_inj_cover_each;  /* 0, or the places it arms at each site */
/* and the places it arms besides, sorted: N, and S << 32 | J, the J-th
   place of the site S (the places a golden of inject.sh names) */
static const long long *rv_inj_cover_n, *rv_inj_cover_sj;
static int rv_inj_cover_nn, rv_inj_cover_nsj;
static int rv_inj_sub;         /* the old opcode under the op, or -1 */
typedef struct RVInjHook {
    const char *file;           /* NULL: the entry is free */
    int line, id, op, sub, kind; /* id: __COUNTER__ at the hook. kind: 0 a hook,
                                   1 outside, 2 miss */
} RVInjHook;
#define RV_INJ_HOOKS 4096
static RVInjHook rv_inj_hooks[RV_INJ_HOOKS];
static int rv_inj_hooks_used, rv_inj_hooks_lost;
/* what an outer run of the loop was arming, while an inner one runs */
typedef struct RVInjOuter {
    const JSStackFrame *frame;
    int op, sub;
} RVInjOuter;
#define RV_INJ_DEPTH 65536
static RVInjOuter rv_inj_outer[RV_INJ_DEPTH];
static int rv_inj_depth;

static void rv_inj_cover_add(const char *file, int line, int id, int kind)
{
    uint32_t h = (uint32_t)id * 131 + (uint32_t)rv_inj_op * 7 + (uint32_t)(rv_inj_sub + 1) * 3 + kind;
    const char *s;
    RVInjHook *e;

    if (file) {
        for (s = file; *s; s++)
            if (*s == '/')
                file = s + 1;
    } else {
        file = "-";
    }
    for (;;) {
        h &= RV_INJ_HOOKS - 1;
        e = &rv_inj_hooks[h];
        if (!e->file) {
            if (++rv_inj_hooks_used > RV_INJ_HOOKS / 2) {
                rv_inj_hooks_lost = 1;
                return;
            }
            e->file = file;
            e->line = line;
            e->id = id;
            e->op = rv_inj_op;
            e->sub = rv_inj_sub;
            e->kind = kind;
            return;
        }
        if (e->id == id && e->op == rv_inj_op && e->sub == rv_inj_sub &&
            e->kind == kind)
            return;
        h++;
    }
}

/* the cover mode: a run of the loop starts, the instruction of the run it
   was entered from waits */
static no_inline void rv_inj_cover_enter(void)
{
    if (rv_inj_depth < RV_INJ_DEPTH) {
        rv_inj_outer[rv_inj_depth].frame = rv_inj_frame;
        rv_inj_outer[rv_inj_depth].op = rv_inj_op;
        rv_inj_outer[rv_inj_depth].sub = rv_inj_sub;
    } else {
        rv_inj_hooks_lost = 1;
    }
    rv_inj_depth++;
}

/* the instruction that was to throw threw an error of its own before its
   hook, or has no place to fail with these operands (iterator_close of an
   iterator that is done): the run goes on with nothing thrown by the test */
static no_inline void rv_inj_none(const char *what)
{
    if (rv_inj_cover) {
        rv_inj_armed = 0;
        return;
    }
    rv_inj_armed = 0;
    rv_inj_fired = 1;
    snprintf(rv_inj_result, sizeof(rv_inj_result), "%s", what);
}

/* a hook of the instruction that is to throw: 1 with the error thrown when
   it is the one, in place of the call into the engine after it */
static no_inline int rv_inj_hit(JSContext *ctx, const char *file, int line, int id)
{
    const char *name = JS_RegvmOpName(rv_inj_op), *s;
    if (rv_inj_cover) {
        if (ctx->rt->current_stack_frame != rv_inj_frame) {
            rv_inj_cover_add(file, line, id, 1);
            rv_inj_armed = 0;
            return 0;
        }
        if (--rv_inj_armed == 0)
            rv_inj_cover_add(file, line, id, 0);
        return 0;
    }
    if (ctx->rt->current_stack_frame != rv_inj_frame) {
        /* a hook of JS that the instruction ran, which has a run of the
           loop of its own (RV_INJ_ENTER): a fault of the test, nothing is
           thrown */
        char what[64];
        snprintf(what, sizeof(what), "outside %s", name ? name : "?");
        rv_inj_none(what);
        return 0;
    }
    if (--rv_inj_armed)
        return 0;
    rv_inj_fired = 1;
    rv_inj_just = 1;
    for (s = file; *s; s++)
        if (*s == '/')
            file = s + 1;
    /* and the hook, by its file and line */
    snprintf(rv_inj_result, sizeof(rv_inj_result), "%s\nhit %s %d\nhook %s:%d:%d", rv_inj_key,
             name ? name : "?", rv_inj_place, file, line, id);
    JS_ThrowInternalError(ctx, "rv inject");
    return 1;
}

/* after a handler threw an error of its own on the way of a hook that
   threw (the checks of let and const throw theirs): the error of the test
   again */
static no_inline void rv_inj_again(JSContext *ctx)
{
    if (rv_inj_just)
        JS_ThrowInternalError(ctx, "rv inject");
}

/* RV_INJ_SLOW of a compare and branch with a fast path for its operands:
   0 when it is to throw at its first place, the call into the engine of its
   slow side, 1 when at its second, the reading of the flag: the fast path
   runs, and its reading of the flag is then the first hook it comes to */
static no_inline int rv_inj_fast(void)
{
    if (rv_inj_armed != 2)
        return 0;
    rv_inj_armed = 1;
    return 1;
}

/* a run of the register loop ends: an instruction of it that was to throw
   ended the run with no hook taken (an error of its own has made it
   rv_inj_none already), and the injection of the run it was entered from
   goes on */
static no_inline void rv_inj_leave(int saved)
{
    if (rv_inj_cover) {
        if (rv_inj_armed)
            rv_inj_cover_add(NULL, 0, -1, 2);
        rv_inj_depth--;
        if (rv_inj_depth < RV_INJ_DEPTH) {
            rv_inj_frame = rv_inj_outer[rv_inj_depth].frame;
            rv_inj_op = rv_inj_outer[rv_inj_depth].op;
            rv_inj_sub = rv_inj_outer[rv_inj_depth].sub;
        }
        rv_inj_armed = saved;
        return;
    }
    if (rv_inj_armed) {
        const char *name = JS_RegvmOpName(rv_inj_op);
        char what[64];
        snprintf(what, sizeof(what), "miss %s", name ? name : "?");
        rv_inj_none(what);
    }
    rv_inj_armed = saved;
}

/* the counts of the old instruction at pos of the function number func */
static RVInjCount *rv_inj_count(uint32_t func, int pos)
{
    uint32_t h = func * 131 + (uint32_t)pos;
    RVInjCount *c;
    for (;;) {
        h &= RV_INJ_TAB - 1;
        c = &rv_inj_tab[h];
        if (c->func == 0) {
            if (++rv_inj_used > RV_INJ_TAB / 2)
                abort();
            c->func = func;
            c->pos = pos;
            return c;
        }
        if (c->func == func && c->pos == pos)
            return c;
        h++;
    }
}

/* asks for an injection: the n-th place, or the j-th place of the
   site number site, 0 for none */
void JS_RegvmInject(long n, unsigned site, long j)
{
    rv_inj_n = site ? 0 : n;
    rv_inj_site = site;
    rv_inj_site_j = j;
    rv_inj_any = n > 0 || site > 0;
}

/* k is one of the count sorted keys of a */
static int rv_inj_cover_has(const long long *a, int count, long long k)
{
    int lo = 0, hi = count - 1, mid;

    while (lo <= hi) {
        mid = (lo + hi) >> 1;
        if (a[mid] < k)
            lo = mid + 1;
        else if (a[mid] > k)
            hi = mid - 1;
        else
            return 1;
    }
    return 0;
}

/* the cover mode arms the place N, the j-th of the site S */
static int rv_inj_cover_at(long n, uint32_t site, long j)
{
    return rv_inj_cover_has(rv_inj_cover_n, rv_inj_cover_nn, n) ||
        rv_inj_cover_has(rv_inj_cover_sj, rv_inj_cover_nsj, (long long)site << 32 | j);
}

/* the cover mode (see rv_inj_cover above) at the place 1 or 2 of each
   instruction: each place, or the first each places at each site and the
   places of the sorted lists n (N) and sj (S << 32 | J), as inject.sh
   samples a script with more places than it injects and runs the places its
   golden names too. The lists stay the caller's, to the end of the run. */
void JS_RegvmInjectCover(long each, int place, const long long *n, int nn,
                         const long long *sj, int nsj)
{
    rv_inj_cover = place;
    rv_inj_cover_each = each;
    rv_inj_cover_n = n;
    rv_inj_cover_nn = nn;
    rv_inj_cover_sj = sj;
    rv_inj_cover_nsj = nsj;
    rv_inj_n = 0;
    rv_inj_site = 0;
    rv_inj_any = 1;
}

/* the result of the cover mode: "none SEEN SITES", then a line a hook
   taken, "cover OP SUB FILE:LINE" (SUB the old opcode under OLD, WITH and
   W_GEN, or -), "outside OP SUB FILE:LINE" for a hook of JS an
   instruction ran, taken while it was armed, "miss OP SUB" for an
   instruction that ran to its end with no hook taken, and "lost" when the
   table was full */
static const char *rv_inj_cover_result(void)
{
    static char out[RV_INJ_HOOKS / 2 * 96 + 64];
    static const char *const kinds[] = { "cover", "outside", "miss" };
    size_t len;
    int i;

    if (rv_inj_armed)
        rv_inj_cover_add(NULL, 0, -1, 2);
    rv_inj_armed = 0;
    len =snprintf(out, sizeof(out), "none %ld %u\n", rv_inj_seen, rv_inj_sites);
    for (i = 0; i < RV_INJ_HOOKS; i++) {
        const RVInjHook *e = &rv_inj_hooks[i];
        const char *name;
        char sub[16];

        if (!e->file || len >= sizeof(out))
            continue;
        name = JS_RegvmOpName(e->op);
        if (e->sub < 0)
            snprintf(sub, sizeof(sub), "-");
        else
            snprintf(sub, sizeof(sub), "%d", e->sub);
        if (e->kind == 2)
            len += snprintf(out + len, sizeof(out) - len, "%s %s %s\n", kinds[e->kind],
                            name ? name : "?", sub);
        else
            len += snprintf(out + len, sizeof(out) - len, "%s %s %s %s:%d:%d\n", kinds[e->kind],
                            name ? name : "?", sub, e->file, e->line, e->id);
    }
    if (rv_inj_hooks_lost && len < sizeof(out))
        snprintf(out + len, sizeof(out) - len, "lost\n");
    return out;
}

/* what happened: "at FUNC POS K" when the register loop threw at a place
   that has a key, "nokey" when it threw at one that has none, "none SEEN
   SITES" when nothing threw, SEEN the
   places where an instruction could throw that ran and SITES the old
   instructions they are at, "real" when the instruction
   threw by itself first, "skip" when it had no place to fail, and "miss
   NAME" when the instruction NAME ran to its end with no hook taken,
   "outside NAME" when a hook of JS it ran was taken for it. "old" was
   the result when QuickJS's interpreter threw: every throw of the test
   now writes its own result, so it is not given. */
const char *JS_RegvmInjectResult(void)
{
    if (rv_inj_cover)
        return rv_inj_cover_result();
    if (rv_inj_armed) {
        /* the instruction that was to throw was the last one run */
        const char *name = JS_RegvmOpName(rv_inj_op);
        snprintf(rv_inj_result, sizeof(rv_inj_result), "miss %s", name ? name : "?");
    }
    if (!rv_inj_result[0])
        snprintf(rv_inj_result, sizeof(rv_inj_result), rv_inj_fired ? "old" : "none %ld %u",
                 rv_inj_seen, rv_inj_sites);
    return rv_inj_result;
}

/* a function was made: its number */
#define RV_INJ_FUNC(b) ((b)->rv_inj_id = ++rv_inj_funcs)

/* The hooks of the register loop. RV_INJ_ARMED: the instruction being run
   is to throw, so its handler takes no fast path. RV_INJ_HIT(ctx): 1 with
   the error thrown, in place of the call into the engine that follows.
   RV_INJ_AGAIN and RV_INJ_NONE: rv_inj_again and rv_inj_none above. All are
   constants in a build without RV_INJECT, which they do not change. */
#define RV_INJ_ARMED unlikely(rv_inj_armed != 0)
/* RV_INJ_SLOW: as RV_INJ_ARMED, for a compare and branch whose operands
   have a fast path, read after it found they have: when the place to throw
   is the second, the reading of the interrupt flag, the fast path runs and
   the flag is read where it reads it, and the first place, the call into
   the engine the fast path does not make, is passed by (rv_inj_fast) */
#define RV_INJ_SLOW (unlikely(rv_inj_armed != 0) && !rv_inj_fast())
/* RV_INJ_FAST(test): the test of a fast path that RV_INJ_ARMED came before,
   then RV_INJ_SLOW. test has no operator weaker than &&. In the build
   without RV_INJECT it is !0 && test, the C those handlers had: clang finds
   the profile of a function by a hash of the order of its operators, and
   JS_RegvmRun keeps its profile only with the same order */
#define RV_INJ_FAST(test) test && !RV_INJ_SLOW
#define RV_INJ_HIT(ctx) (unlikely(rv_inj_armed != 0) && rv_inj_hit(ctx, __FILE__, __LINE__, __COUNTER__))
#define RV_INJ_AGAIN(ctx) rv_inj_again(ctx)
#define RV_INJ_NONE(what) do { if (unlikely(rv_inj_armed != 0)) rv_inj_none(what); } while (0)
/* a run of the register loop starts and ends: the injection of the run it
   was entered from waits in saved (an int of the run) while it runs */
#define RV_INJ_ENTER(saved) ((saved) = rv_inj_armed, rv_inj_armed = 0,  \
                             rv_inj_cover ? rv_inj_cover_enter() : (void)0)
#define RV_INJ_LEAVE(saved) rv_inj_leave(saved)

#else /* RV_INJECT */

#define RV_INJ_FUNC(b) ((void)0)
#define RV_INJ_ARMED 0
#define RV_INJ_SLOW 0
#define RV_INJ_FAST(test) !RV_INJ_ARMED && test
#define RV_INJ_HIT(ctx) 0
#define RV_INJ_AGAIN(ctx) ((void)0)
#define RV_INJ_NONE(what) ((void)0)

#endif /* RV_INJECT */
