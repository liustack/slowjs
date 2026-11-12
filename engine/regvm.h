/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The register loop, the interpreter of the engine: included by quickjs.c
   after free_function_bytecode. A function is converted from its bytecode
   to register code at its first call (in JS_CallInternal, or in JS_Call
   from the host), and every call runs in JS_RegvmRun: from C (js_call) and
   from the register loop itself (rv_entry_fast). Calls between functions
   stay in that loop, with the arguments where the caller put them. The
   body of a generator or an async function converts at its first resume
   and runs as the entry frame of a run of its own, moved between its heap
   frame and the register stack at each suspension and resume
   (regvm-ops-gen.h). The pass refuses only code no compiler makes
   (rv_refused).

   A frame on the register stack, from the pointer F:

     F[-fofs]              func              (fofs = arg_count + 2 + var_count)
     F[-fofs + 1 ...]      the arguments
     F[-(var_count + 1)]   this
     F[-var_count ...]     the locals (the first is new.target when the
                           function reads it, regvm-ops-ctor.h)
     F[0 .. 2]             the header (RVHeader)
     F[3 ...]              the closure state, only when locals are captured
     F[temp0 ...]          the temps T[0 .. stack_size)

   The slots a frame owns, func to its last local, are one run [F - fofs, F)
   that its return frees.

   A frame called with argc > arg_count keeps the x = argc - arg_count
   extra arguments right below func, F[-fofs - x] to F[-fofs - 1], in
   order, as Lua keeps the extra arguments of a vararg function: the call
   moves func and the declared arguments up past them, so every slot keeps
   its place from F. Right below the extra ones, F[-fofs - x - 1] holds a
   reference to func of its own: the return frees the frame's slots first,
   func among them, and when that was the last reference to func its code
   and rf are freed with it, but the extra ones are freed after the frame
   and their count and place come from rf (rv_free_extra). Such a frame has
   RV_DST_EXTRA in its dst, which the return tests behind the one test it
   has anyway, and argc in its header. A C entry copies them only for a
   function that reads its arguments (rf->args): for any other the C
   caller's argv keeps them alive, as it does for JS_CallInternal.

   A function whose locals or arguments a closure captures (var_ref_count of
   its bytecode not 0) has a closure state between the header and the temps:
   a JSStackFrame and its var_refs[var_ref_count] (RV_SF), so that
   get_var_ref, free_var_ref, close_var_refs and close_lexical_var of the old
   loop work on it as on a frame of JS_CallInternal. Its first instruction,
   CLINIT, fills it in. Its returns are RETCT to RETCU, and the unwinding of
   rv_exception closes it too (RV_CLOSE_VAR_REFS): each closes the var_refs
   before the slots are freed. The temps then start at temp0 = RV_HDR + the
   slots of the state, so a pcmap mask counts from F[temp0]. A frame with
   so many that their var_refs would push the temps past slot 127 keeps
   only the JSStackFrame there and its var_refs on the heap (CLW,
   regvm-ops-wide.h). Every other
   function has temp0 = RV_HDR and pays nothing at its exit. A call is made on the temps of the caller where the
   old stack has its operands: func and the arguments are where the callee's
   frame wants them, and the callee's this and locals go above them. A temp
   of the caller is computed in place, a local is copied there by a MOV.

   An operand is a signed byte, a slot from F. The pass knows for each
   operand whether it is a local, an argument or this (read without a
   reference, kept by its slot) or a temp (owned, moved out by the
   instruction that reads it), and picks the instruction for it.

   What stage 0 leaves out: try and catch,
   arguments, sloppy functions, constants with a reference count, calls with
   more arguments than the function has parameters (they go through
   JS_CallInternal), and stack traces of the frames inside the loop (a
   backtrace sees the function JS_RegvmRun was entered with).

   The files: regvm-func.h has RVFunc and rv_entry_fast, read by quickjs.c
   before its call ops. This file has the frame, the instruction enum, the
   parts of an area. regvm-pass.h is the pass, regvm-run.h
   the loop. Each op lives in one area file, regvm-ops-*.h, listed in
   regvm-areas.h. */

#define RV_HDR 3                     /* slots of the header, the first temp */
/* RVHeader.dst: the low 16 bits are the caller's slot for the result as an
   int16_t (RV_DST_SLOT), the bits above are flags. The return's one test
   (RV_DST_FAST) lets through only a temp of the caller with no flag:
   everything else goes out of line, in this order: RV_DST_EXTRA, RV_DST_SEG,
   then the slot. Code that reads dst uses these names, never a number or
   the sign. */
#define RV_DST_ENTRY 0               /* slot: the frame was entered from C */
#define RV_DST_DROP 1                /* slot: the caller drops the result: the
                                        return frees it */
#define RV_DST_EXTRA (1 << 16)       /* flag: extra arguments below func */
#define RV_DST_SEG (1 << 17)         /* flag: the frame starts a segment of the
                                        register stack, its return steps back
                                        to the caller's (engine/regvm-entry.h) */
#define RV_DST_SLOT(d) ((int16_t)(d))
#define RV_DST_FAST(d) ((uint32_t)((d) - RV_HDR) < 128 - RV_HDR)
/* The slots of a segment of register stack of about r bytes: the most that a
   new shared chunk of 2 * r bytes takes (HEAP_SHARED_MAX of engine/heap.h),
   less the header of the block and of the segment. One slot more and the
   segment would get a chunk of its own, which the host maps and unmaps with
   it. The first segment is of 32 KiB (RV_SEG_FIRST), a next one of twice
   the one below, up to 256 KiB (RV_SEG_MAX), unless a call needs more
   (engine/regvm-entry.h). */
#define RV_SEG_SLOTS(r) \
    ((HEAP_SHARED_MAX(2 * (r)) - sizeof(JSMallocLargeBlockHeader) - sizeof(RVSeg)) / sizeof(JSValue))
#define RV_SEG_FIRST_BYTES (32 * 1024)
#define RV_SEG_MAX_BYTES (256 * 1024)
#define RV_SEG_FIRST RV_SEG_SLOTS(RV_SEG_FIRST_BYTES)
#define RV_SEG_MAX RV_SEG_SLOTS(RV_SEG_MAX_BYTES)

typedef struct RVHeader {
    JSValue *caller;      /* F of the caller, NULL for an entry frame */
    union {
        const uint8_t *retpc;  /* where the caller goes on */
        JSContext *caller_ctx; /* of an entry frame: the caller_ctx of
                                  JS_RegvmRun, where the errors of a
                                  derived constructor are thrown */
    };
    RVFunc *rf;
    JSValue *k;           /* rf->k */
    int32_t dst;          /* the caller's slot for the result and the flags
                             RV_DST_ above */
    int32_t argc;         /* the arguments passed: stored by a C entry and by
                             the slow entry of a call (rv_enter_args) and a
                             construct in the loop (rv_new_frame) only */
    JSVarRef **var_refs;  /* the closure variables of func */
} RVHeader;

_Static_assert(sizeof(RVHeader) == RV_HDR * sizeof(JSValue),
               "the header is three slots");
_Static_assert(sizeof(JSValue) == 16, "a slot is a JSValue of 16 bytes");

/* the closure variable i of the frame F. The check build compares one
   that holds its value with its variable at each use (quickjs.c,
   js_value_ref_check). */
#ifdef RV_CBV_CHECK
static inline JSVarRef *rv_vref_check(JSValue *F, int i)
{
    JSVarRef *v = ((RVHeader *)F)->var_refs[i];
    if (((RVHeader *)F)->rf->b->closure_var[i].by_value)
        js_value_ref_check(v);
    return v;
}
#define RV_VREF(F, i) rv_vref_check(F, i)
#else
#define RV_VREF(F, i) (((RVHeader *)(F))->var_refs[i])
#endif

/* the closure state of a frame with captured locals: a JSStackFrame at F[3],
   its var_refs right after it */
#define RV_SF(F) ((JSStackFrame *)((F) + RV_HDR))
#define RV_SF_SLOTS(n) ((int)((sizeof(JSStackFrame) + sizeof(JSVarRef *) * (n) + \
                               sizeof(JSValue) - 1) / sizeof(JSValue)))
/* what the parser reads of them (regvm-func.h) */
_Static_assert(RV_FUNC_HDR == RV_HDR && RV_SF0_SLOTS == RV_SF_SLOTS(0),
               "the frame the parser counts is the one of the register loop");

/* The parts of an area file (regvm-areas.h). Each area file is read once
   for each part, with RV_PART set to it, and gives what it has of that
   part. */
#define RV_PART_LIST 1    /* RV_OP(name) for each op: the enum and the table */
#define RV_PART_INFO 2    /* cases of the allow-list, in rv_old_info */
#define RV_PART_PASS_FN 3 /* helpers of the pass, at file scope */
#define RV_PART_PASS 4    /* cases of rv_one */
#define RV_PART_RUN_FN 5  /* slow sides of the handlers, at file scope */
#define RV_PART_RUN 6     /* the handlers, inside JS_RegvmRun */

/* The register instructions, in the order of the areas. The returns of a
   frame with fofs up to RV_RETN_MAX come last: RV_RETN + kind *
   (RV_RETN_MAX + 1) + fofs, kind 0 to 3 as RV_RET to RV_RETU, which free
   the slots of the frame with no switch on their count. */
enum {
#define RV_OP(name) RV_##name,
#define RV_PART RV_PART_LIST
#include "regvm-areas.h"
#undef RV_PART
#undef RV_OP
    RV_RETN,
    RV_OP_COUNT = RV_RETN + 4 * 9
};

/* PUTF flags */
#define RV_PUT_OBJ_TEMP 1
#define RV_PUT_VAL_TEMP 2
#define RV_PUT_VAL_K 4

#define RV_RETN_MAX 8
_Static_assert(RV_OP_COUNT == RV_RETN + 4 * (RV_RETN_MAX + 1), "four kinds of return");
_Static_assert(RV_ADDL == RV_ADD + 1 && RV_ADDK == RV_ADD + 2 && RV_SUB == RV_ADD + 4 &&
               RV_SUBKL == RV_ADD + 7, "rv_arith_other reads the kind of add or sub from the opcode");

static int rv_stack_make(JSRuntime *rt);
static void rv_stack_free(JSRuntime *rt);
static int rv_stack_segments(JSRuntime *rt);

static void rv_measure_frame(JSContext *ctx);
static int rv_takes_opcode(int op);

int JS_RegvmStat(JSContext *ctx, int what)
{
    if (what >= 2000 && what < 2256)
        return rv_takes_opcode(what - 2000);
    if (what == 5) {
        rv_measure_frame(ctx);
        return ctx->rt->rv_frame;
    }
    if (what == 6)
        return ctx->rt->call_frame;
    if (what == 7)
        return ctx->rt->rv_seg_made;
    if (what == 8)
        return rv_stack_segments(ctx->rt);
    if (what == 9)
        return (int)ctx->rt->rv_stack_bytes;
    if (what == 11)
        return (int)ctx->rt->rv_scratch_size;
    if (what == 12)
        return (int)ctx->rt->c_hop_debt;
    if (what == 10)
        return ctx->rt->rv_count[0];
    if (what >= 13 && what < 16)
        return ctx->rt->rv_count[what - 12];
#ifdef RV_STATS
    if (what >= 1000 && what < 1256)
        return ctx->rt->rv_opcount[what - 1000];
    if (what == 16)
        return (int)ctx->rt->rv_peak;
#endif
    if (what < 0 || what >= (int)countof(ctx->rt->rv_stat))
        return 0;
    return ctx->rt->rv_stat[what];
}

static void rv_free_func(JSRuntime *rt, RVFunc *rf)
{
    if (rf->arg_count >= 0)
        js_free_rt(rt, rf);
}

#include "regvm-pass.h"

static void rv_free_runtime(JSRuntime *rt)
{
    rv_stack_free(rt);
    rv_scratch_free(rt);
}

#include "regvm-entry.h"
#include "regvm-run.h"

/* a function the pass refused: code no compiler makes that JS_ReadObject
   read with JS_READ_OBJ_POLLS_CHECKED, which skips rv_read_check
   (regvm-entry.h), or a mistake of the pass (rv_overflow, regvm-pass.h) in a
   build without RV_SELF_CHECK. Every call of it throws. */
static no_inline JSValue rv_refused(JSContext *ctx, JSFunctionBytecode *b)
{
    char name[ATOM_GET_STR_BUF_SIZE];

    return JS_ThrowInternalError(ctx, "invalid bytecode: the register loop refuses %s",
                                 JS_AtomGetStr(ctx, name, sizeof(name), b->func_name));
}

/* JS_RegvmStat 2000 + op, for the test of the opcode table
   (engine/test/regvm/rvops.c): 1 when the pass takes the old opcode op
   (its allow-list, rv_old_info), 0 when it refuses every instruction of
   it, -1 when op is no opcode of the code of a function that runs
   (OP_invalid, which check_polls refuses, or past the last) */
static int rv_takes_opcode(int op)
{
    uint8_t bc[16] = { 0 };
    int npop, npush, term, target;

    if (op <= OP_invalid || op >= OP_COUNT)
        return -1;
    bc[0] = op;
    return rv_old_info(bc, 0, &npop, &npush, &term, &target) >= 0;
}

/* JS_RegvmFuncStat, for tests: what 0 the state of func in the register
   loop, 1 it has register code, 0 not converted yet (not called yet), -1
   refused, -3 not a function of bytecode. 1 the frames set up for it
   (counted with -DRV_STATS only). 2 and 3 for a refused function, what the
   pass stops at (as JS_RegvmStat 3 and 4): -3 and its offset for code no
   compiler makes (RV_CRAFTED of regvm-pass.h), or in a build without
   RV_SELF_CHECK the opcode and its offset of a mistake of the pass
   (rv_overflow), -99 when it stops at its jumps. The pass runs again for 2
   and 3, with the counts of JS_RegvmStat kept as they were. */
int JS_RegvmFuncStat(JSContext *ctx, JSValueConst func, int what)
{
    JSRuntime *rt = ctx->rt;
    JSFunctionBytecode *b;
    RVFunc *rf;
    int st[5], r;

    if (JS_VALUE_GET_TAG(func) != JS_TAG_OBJECT ||
        !js_class_has_bytecode(JS_VALUE_GET_OBJ(func)->class_id))
        return what == 0 ? -3 : 0;
    b = JS_VALUE_GET_OBJ(func)->u.func.function_bytecode;
    switch (what) {
    case 0:
        if (b->rv == RV_UNCONVERTED)
            return 0;
        return b->rv == RV_REFUSED ? -1 : 1;
    case 1:
        return b->rv->arg_count >= 0 ? (int)b->rv->runs : 0;
    case 2:
    case 3:
        if (b->rv != RV_REFUSED)
            return 0;
        memcpy(st, rt->rv_stat, sizeof(st));
        rt->rv_stat[3] = -99;
        rt->rv_stat[4] = 0;
        rf = rv_build(rt, b, NULL);
        if (rf)
            rv_free_func(rt, rf);
        r = rt->rv_stat[what + 1];
        memcpy(rt->rv_stat, st, sizeof(st));
        return r;
    default:
        return 0;
    }
}

/* the name of register op op, or NULL past the last one */
const char *JS_RegvmOpName(int op)
{
    static const char *const names[RV_OP_COUNT] = {
#define RV_OP(name) [RV_##name] = #name,
#define RV_PART RV_PART_LIST
#include "regvm-areas.h"
#undef RV_PART
#undef RV_OP
    };
    if (op < 0 || op >= RV_OP_COUNT)
        return NULL;
    return names[op] ? names[op] : "RETN";
}
