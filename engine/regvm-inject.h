/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
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
static char rv_inj_result[192];
static char rv_inj_key[96];     /* the result once the hook has thrown */

/* the instruction that was to throw threw an error of its own before its
   hook, or has no place to fail with these operands (iterator_close of an
   iterator that is done): the run goes on with nothing thrown by the test */
static no_inline void rv_inj_none(const char *what)
{
    rv_inj_armed = 0;
    rv_inj_fired = 1;
    snprintf(rv_inj_result, sizeof(rv_inj_result), "%s", what);
}

/* a hook of the instruction that is to throw: 1 with the error thrown when
   it is the one, in place of the call into the engine after it */
static no_inline int rv_inj_hit(JSContext *ctx)
{
    const char *name = JS_RegvmOpName(rv_inj_op);
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
    snprintf(rv_inj_result, sizeof(rv_inj_result), "%s\nhit %s %d", rv_inj_key,
             name ? name : "?", rv_inj_place);
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

/* a run of the register loop ends: an instruction of it that was to throw
   ended the run with no hook taken (an error of its own has made it
   rv_inj_none already), and the injection of the run it was entered from
   goes on */
static no_inline void rv_inj_leave(int saved)
{
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
#define RV_INJ_HIT(ctx) (unlikely(rv_inj_armed != 0) && rv_inj_hit(ctx))
#define RV_INJ_AGAIN(ctx) rv_inj_again(ctx)
#define RV_INJ_NONE(what) do { if (unlikely(rv_inj_armed != 0)) rv_inj_none(what); } while (0)
/* a run of the register loop starts and ends: the injection of the run it
   was entered from waits in saved (an int of the run) while it runs */
#define RV_INJ_ENTER(saved) ((saved) = rv_inj_armed, rv_inj_armed = 0)
#define RV_INJ_LEAVE(saved) rv_inj_leave(saved)

#else /* RV_INJECT */

#define RV_INJ_FUNC(b) ((void)0)
#define RV_INJ_ARMED 0
#define RV_INJ_HIT(ctx) 0
#define RV_INJ_AGAIN(ctx) ((void)0)
#define RV_INJ_NONE(what) ((void)0)

#endif /* RV_INJECT */
