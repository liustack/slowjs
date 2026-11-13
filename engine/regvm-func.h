/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The register code of a function (engine/regvm.h), and the test of a call
   that enters the register loop with it (rv_entry_fast): included by
   quickjs.c before JS_CallInternal and the call ops of the register loop,
   which make that test inline. */

/* the temps a frame can have: 128 less the header, so that each is named by
   a byte operand */
#define RV_TEMPS_MAX 124

/* the slots a frame can have below F: MOVW, SETLW and CHKLW name a local,
   argument, this or func with an int16_t, so func, the farthest, is at
   slot -32768 at most. A function of more is refused. */
#define RV_FAR_MAX 32768

/* the header of a frame (RV_HDR of regvm.h) and the slots of its
   JSStackFrame alone (RV_SF_SLOTS(0)), for the parser, which reads them
   before regvm.h */
#define RV_FUNC_HDR 3
#define RV_SF0_SLOTS ((int)((sizeof(JSStackFrame) + sizeof(JSValue) - 1) / sizeof(JSValue)))

/* A stack this deep or less fits the temps in every function: with the
   header, the JSStackFrame (the var_refs on the heap) and the scratch slot
   of a wide frame. A deeper one may fold (regvm-fold.h), into a frame of
   RV_FOLD_TCAP temps, of which the part above the innermost catch offset
   takes RV_FOLD_TOP_MAX at most: of Arrays of 4 entries, six levels with
   up to 4 in no Array at each and a seventh with up to 3 (the most
   JS_STACK_SIZE_MAX needs), up to 5 entries in no Array, the 34 entries a
   call of 32 arguments reads, and 4 for what an instruction pushes and the
   end of its window (the test build, with Arrays of 2, has 15 levels with
   up to 2 at each in the same room).
   The catch offsets and what goes with them (1 for a catch, 3 for a
   for_of_start) and one temp for each part frozen under one take the rest,
   RV_FOLD_PINS_MAX: the parser refuses a function deeper than
   RV_STACK_FIT that needs more (compute_stack_size, quickjs.c). */
#define RV_STACK_FIT (127 - RV_FUNC_HDR - RV_SF0_SLOTS - 1)
#define RV_FOLD_TCAP (RV_TEMPS_MAX - 1 < RV_STACK_FIT ? RV_TEMPS_MAX - 1 : RV_STACK_FIT)
#define RV_FOLD_TOP_MAX 70
#define RV_FOLD_PINS_MAX (RV_FOLD_TCAP - RV_FOLD_TOP_MAX)

/* a set of the temps T[0] to T[127], word i for T[32 * i] up */
typedef struct RVMask {
    uint32_t w[4];
} RVMask;

static inline void rv_mask_set(RVMask *x, int i)
{
    x->w[i >> 5] |= 1u << (i & 31);
}

static inline void rv_mask_clear(RVMask *x, int i)
{
    x->w[i >> 5] &= ~(1u << (i & 31));
}

static inline int rv_mask_has(const RVMask *x, int i)
{
    return (x->w[i >> 5] >> (i & 31)) & 1;
}

/* the temps T[32] up of a pcmap entry (RVFunc.pcmap_hi, one for each entry
   of the pcmap of a function where one of them owns such a temp) */
typedef struct RVPcHi {
    uint32_t mask[3];
} RVPcHi;

/* an entry of the pcmap of a function: one for each instruction that can
   throw, sorted by end (engine/regvm-run.h rv_pc_find) */
typedef struct RVPcMask {
    uint32_t end;  /* offset of the end of an instruction that can throw */
    int32_t old;   /* the pc of the old bytecode named for an exception
                      there (find_line_num), as QuickJS's interpreter names
                      it: -1, the place the function is defined at, for a
                      jump back to offset 0 */
    uint32_t mask; /* the temps owned across it, bit i for T[i], i < 32: the
                      others are in RVFunc.pcmap_hi */
#ifdef RV_INJECT
    /* the throw-injection test (engine/regvm-inject.h): where the
       instruction starts, its old instruction, and in the entry at the end
       of the instruction the places where the test makes it throw:
       RV_HOOK_NONE none. RV_HOOK_OWN one, the instruction itself, where
       QuickJS's interpreter can be made to throw the same, at the dispatch of
       the old instruction. RV_HOOK_OWN_POLL two, that one and then the reading
       of the interrupt flag. RV_HOOK_APART one and RV_HOOK_APART2 two that
       QuickJS's interpreter cannot throw the same at: the reading of the flag
       of a conditional jump (QuickJS's interpreter reads it after it jumped),
       the cases of CASES (QuickJS's interpreter runs a case in the dispatch of
       the one before it), for_of_next and iterator_close (they fail with the
       iterator given up, where a throw at their dispatch leaves it to be
       closed) */
    uint32_t start, pos;
    uint32_t hooks;
#endif
} RVPcMask;

enum { RV_HOOK_NONE, RV_HOOK_OWN, RV_HOOK_OWN_POLL, RV_HOOK_APART, RV_HOOK_APART2 };

typedef struct RVFunc {
    struct JSFunctionBytecode *b;
    int arg_count, var_count, stack_size;
    int fofs;       /* arg_count + 2 + var_count: func to the last local */
    int fsize;      /* temp0 + stack_size: slots from F to the end */
    int temp0;      /* the slot of T[0]: RV_HDR, or above the closure state */
    int var_ref_count; /* b->var_ref_count: the frame has a closure state */
    int vr_heap;    /* its var_refs are on the heap, as too many for the room
                       under the temps (regvm-ops-wide.h) */
    int args;       /* the code reads its arguments: arguments or rest */
    int argmax;     /* the most arguments a call can pass on the fast entry
                       of the call op: arg_count, or -1 when args is set (and
                       in the markers), so that such a call takes the slow
                       entry, which stores argc (regvm-ops-basic.h) */
    /* new.target has a slot, the first local F[-var_count], below the
       locals of b (var_count is then b->var_count + 1): a call writes
       undefined there with the other locals, a construct call new.target
       (regvm-ops-ctor.h) */
    uint8_t nt;
    /* in bytes from the first slot of the frame, func: F, its this, and
       the end of the frame, for the call */
    intptr_t fofs_b, this_b, room_b;
    int k_count, pcmap_count, code_len;
    uint32_t runs;  /* the frames set up for it, counted with -DRV_STATS only */
    JSValue *k;
    RVPcMask *pcmap;
    int ret_count;  /* the GOSUBs: four rets each, the end of the old gosub
                       (the return address), the end of the GOSUB, and the
                       depth and the catchpos of the pass there, by the
                       first (regvm-ops-except.h rv_ret_target) */
    uint32_t *rets;
    RVPcHi *pcmap_hi; /* NULL when no entry owns a temp past T[31] */
    uint8_t code[0];
} RVFunc;

/* b->rv of a function not yet converted and of one the pass refused: an
   arg_count of -1 sends every call of it out of the register loop to
   JS_CallInternal, which converts an RV_UNCONVERTED one. */
static const RVFunc rv_unconverted_s = { .arg_count = -1, .argmax = -1 };
static const RVFunc rv_refused_s = { .arg_count = -1, .argmax = -1 };
#define RV_UNCONVERTED ((RVFunc *)&rv_unconverted_s)
#define RV_REFUSED ((RVFunc *)&rv_refused_s)

static no_inline BOOL rv_first_call(JSRuntime *rt, struct JSFunctionBytecode *b);

/* whether a call from C (js_call) of func_obj runs in JS_RegvmRun: a
   function with register code, converted here at its first call
   (rv_first_call, engine/regvm-entry.h). Room on the register stack is
   the entry's job: JS_RegvmRun takes another segment of it when the frame
   does not fit. */
static inline BOOL rv_entry_fast(JSRuntime *rt, JSValueConst func_obj)
{
    JSObject *p;
    RVFunc *rf;

    if (JS_VALUE_GET_TAG(func_obj) != JS_TAG_OBJECT)
        return FALSE;
    p = JS_VALUE_GET_OBJ(func_obj);
    if (p->class_id != JS_CLASS_BYTECODE_FUNCTION)
        return FALSE;
    rf = p->u.func.function_bytecode->rv;
    if (likely(rf->arg_count >= 0))
        return TRUE;
    /* not when its conversion just ran out of memory in this call
       (rv_oom_b): the call ends with that error, a later one tries again */
    return rf == RV_UNCONVERTED && rt->rv_oom_b != p->u.func.function_bytecode &&
           rv_first_call(rt, p->u.func.function_bytecode);
}

/* the new.target a construct call passes to JS_RegvmRun: a reference for
   the slot of a function that has one, else undefined */
static inline JSValue rv_ctor_new_target(JSContext *ctx, JSValueConst func_obj,
                                         JSValueConst new_target)
{
    RVFunc *rf = JS_VALUE_GET_OBJ(func_obj)->u.func.function_bytecode->rv;
    return rf->nt ? JS_DupValue(ctx, new_target) : JS_UNDEFINED;
}

/* the pc named for an exception at the old instruction at pos of bc, as
   QuickJS's interpreter names it (the value find_line_num takes): one inside
   it, but for a goto and a gosub, which read the interrupt flag once they
   moved pc, the one before their target */
static inline int32_t rv_old_pc_at(const uint8_t *bc, int pos)
{
    const uint8_t *p = bc + pos;
    switch (p[0]) {
    case OP_goto: case OP_gosub:
        return pos + (int32_t)get_u32(p + 1);
    case OP_goto16:
        return pos + (int16_t)get_u16(p + 1);
    case OP_goto8:
        return pos + (int8_t)p[1];
    default:
        return pos;
    }
}
