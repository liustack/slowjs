/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The pass of the register loop: the old bytecode of a function to register
   code, at its first call. Included by regvm.h. The cases of each op are in
   the area files (regvm-areas.h), read here as the parts RV_PART_INFO (the
   allow-list, in rv_old_info), RV_PART_PASS_FN (the helpers of the area, at
   file scope) and RV_PART_PASS (the cases of rv_one). */

/* an entry of the abstract stack. RV_E_FOLD: an entry of a folded range
   but its first, which is an RV_E_TEMP whose slot holds the Array of the
   range (engine/regvm-fold.h) */
enum { RV_E_SLOT, RV_E_TEMP, RV_E_K, RV_E_FOLD };
typedef struct RVEnt {
    uint8_t kind;
    int8_t slot; /* RV_E_SLOT: the local or argument, RV_E_TEMP: the temp */
    uint8_t k;
} RVEnt;

/* how a block is reached: by falling through from a branch or a return
   before it, or by a jump (and the entry) */
#define RV_LEAD_FALL 1
#define RV_LEAD_JUMP 2
#define RV_LEAD_CASE 4 /* a case a CASES before it took by a jump (regvm-ops-cmp.h) */
#define RV_LEAD_CATCH 8 /* the target of a catch, which only an exception reaches */
#define RV_LEAD_OTHER 16 /* the target of a jump that is not a catch */

/* the entries of a folded Array of the stack (engine/regvm-fold.h): 32,
   then RV_FOLD_RMIN when that does not fit, or fewer in the test build
   that folds every function it can. RV_FOLD_LMAX levels of Arrays of
   Arrays hold any stack the old code can have (JS_STACK_SIZE_MAX) at the
   smallest. A frozen part under a catch offset is one Array of up to a
   frame of temps, whose elements GETELK reads by the constants 0 to
   RV_FOLD_KMAX - 1. */
#ifdef RV_FOLD_TEST
#define RV_FOLD_R RV_FOLD_TEST
#define RV_FOLD_RMIN RV_FOLD_TEST
#else
#define RV_FOLD_R 32
#define RV_FOLD_RMIN 4
#endif
#define RV_FOLD_LMAX 16
#define RV_FOLD_KMAX 128

/* the buffers of the pass in its scratch, in this order after the arrays
   of the old instructions */
enum {
    RV_SC_CODE, RV_SC_FIX, RV_SC_FIXW, RV_SC_PCMAP, RV_SC_HI, RV_SC_RETS, RV_SC_K,
    RV_SC_N
};

typedef struct RVPass {
    JSRuntime *rt;
    JSFunctionBytecode *b;
    const uint8_t *bc;
    int len;
    int16_t *depth;     /* stack depth before each instruction, -1 unreached */
    int32_t *catchpos;  /* the innermost catch there (rv_depths), -1 none */
    uint8_t *leader;    /* RV_LEAD_ of each instruction that starts a block */
    int32_t *newpc;     /* new offset of each leader, and before it the work
                           list of rv_depths (len + 1 entries) */
    DynBuf code;
    DynBuf fix;         /* the RVFix of each off16 */
    DynBuf fixw;        /* (int32 pos of an off32, int32 old target): JMPW */
    uint8_t *farj;      /* the old instructions whose jumps get an island, or
                           NULL: rv_islands. In the scratch at farj_at */
    uint8_t *farj_at;
    RVEnt *st_at;       /* the st of a function that folds, in the scratch */
    DynBuf pcmap;
    DynBuf rets;        /* (old return address, end of its GOSUB) */
    DynBuf pcmap_hi;    /* an RVPcHi for each entry of pcmap */
    BOOL hi;            /* an entry owns a temp past T[31] */
    DynBuf kbuf;        /* the constants, borrowed from b: its atoms and cpool,
                           or no count */
    JSValue *k;         /* kbuf.buf */
    uint16_t *kh;       /* the index of k[0] to k[RV_K_NEAR - 1] by value
                           (rv_k), in the scratch */
    uint64_t kref[4];   /* bit i: k[i] has a reference count, i < 256 */
    int k_count;
    /* the stack model, with a few entries past the most stack_size can be
       that nothing fills: an instruction that writes past stack_size is
       stopped there (rv_overflow), and the pass ends before the next one.
       st is st_in, or for a function that folds (fold) one of stack_size
       entries and as many more */
    RVEnt *st;
    /* a stack deeper than the temps, folded (engine/regvm-fold.h): chain
       is the innermost catch offset (catchpos), base the depth and bslot
       the temp where the part above it starts, lev[k] the Arrays of level
       k + 1 there, R the entries of an Array, tcap the temps of the frame.
       trans: a part was just frozen for the catch or for_of_start that is
       read now. fold_full: a temp past tcap was asked for. */
    BOOL fold, trans, fold_full;
    int R, chain, base, bslot, tcap;
    int fold_m;         /* the entries of the top part whose layout lev is */
    int sp;
    int var_count;      /* the locals of the frame: b->var_count, one more with
                           the slot of new.target (rf->nt, regvm-ops-ctor.h) */
    int first_temp;
    int last;           /* offset of the last instruction emitted, -1 at a leader */
    int last_dst;       /* its d, when it wrote a temp the pass may retarget */
    int pos;            /* offset of the old instruction rv_one reads */
#ifdef RV_INJECT
    int ipos, iprev;    /* QuickJS's dispatch of it, the one before */
#endif
    int skip_to;        /* the old instructions before it were read with the last one */
    BOOL wide;          /* slots below F past -128: rv_wide_one reads first, and
                           the frame has a scratch slot past its stack */
    BOOL vr_heap;       /* the var_refs of the closure state are on the heap
                           (regvm-ops-wide.h) */
    int drop_at;        /* the drop a JFND or JTND ran, or -1 */
    BOOL args;          /* the code reads its arguments (regvm-ops-args.h) */
    BOOL failed;
    BOOL oom;           /* an allocation of the pass failed */
    BOOL crafted;       /* it stops at code no compiler makes (RV_CRAFTED) */
    /* the scratch (rv_scratch_fit): the bytes of each buffer, the bytes
       before the first, the entries of st there, and the round ran out of
       one buffer (rv_scratch_short) */
    size_t cap[RV_SC_N];
    size_t fixed;
    int st_n;
    BOOL sc_short;
    /* not cleared at the start of a conversion (rv_build): an entry of
       the stack model is written before it is read, and rv_fold_init sets
       lev and fold_k */
    RVEnt st_in[RV_TEMPS_MAX + 8];
    int lev[RV_FOLD_LMAX + 1];
    uint8_t fold_k[RV_FOLD_KMAX]; /* the constants 0 to RV_FOLD_KMAX - 1 */
} RVPass;

/* The pass refuses only code that no compiler makes, which only
   JS_ReadObject can give, and rejects it when it is read (rv_read_check,
   regvm-entry.h): its refusal (JS_RegvmStat 3) is RV_CRAFTED. Such code
   has an opcode the pass does not take (OP_invalid or past the last), depths
   of the stack or of the catches that do not agree, an instruction that
   pops more than the stack holds, code that runs off its end or jumps where
   nothing was emitted, a ret with no gosub, an operand out of the tables of
   the function (rv_operands_ok), a special_object of no kind, mapped
   arguments or a make_loc_ref with no closure state, an init_ctor with
   parameters, a call of more than 255 arguments (the parser passes those
   past JS_CALL_ARGS_ON_STACK in an array), more locals and arguments than
   MOVW names (the parser stops at RV_FAR_MAX, JS_FRAME_FULL of quickjs.c),
   or a stack whose catch offsets need more temps than a folded frame has
   (the parser stops at RV_FOLD_PINS_MAX, regvm-func.h). Anything else that
   stops the pass is a mistake of the pass (rv_overflow), or out of memory. */
#define RV_CRAFTED (-3)

static void rv_crafted(RVPass *s)
{
    s->failed = TRUE;
    s->crafted = TRUE;
}

static int rv_old_argc(const uint8_t *p, int op)
{
    switch (op) {
    case OP_call0: case OP_call1: case OP_call2: case OP_call3:
        return op - OP_call0;
    default:
        return get_u16(p + 1);
    }
}

/* pops and pushes of an old instruction this pass takes, or -1 for one it
   does not. *term: no fall through. *target: a jump's target or -1. */
static int rv_old_info(const uint8_t *bc, int pos, int *npop, int *npush,
                       int *term, int *target)
{
    int op = bc[pos];
    const JSOpCode *oi = &short_opcode_info(op);
    *term = 0;
    *target = -1;
    *npop = oi->n_pop;
    *npush = oi->n_push;
    switch (op) {
#define RV_PART RV_PART_INFO
#include "regvm-areas.h"
#undef RV_PART
    default:
        return -1;
    }
}

/* the depth of the catch offset of the catch or for_of_start at c (the old
   pos of the instruction that pushed it): its index on the stack */
static int rv_catch_level(RVPass *s, int c)
{
    return s->depth[c] + (s->bc[c] != OP_catch);
}

/* whether the operands of the instruction op at pos that index the tables
   of the function are in them: its locals, arguments, closure variables
   and constants, the constant of fclosure a function, and the atom of
   push_atom_value not an index (the parser puts those in the constants).
   Only code no compiler makes fails (RV_CRAFTED): the pass then reads no
   table out of its bounds. It is not a verifier of bytecode. */
static BOOL rv_operands_ok(RVPass *s, int pos, int op)
{
    JSFunctionBytecode *b = s->b;
    const uint8_t *p = s->bc + pos;
    uint32_t idx;

    switch (op) {
    case OP_make_loc_ref:
        return get_u16(p + 5) < b->var_count;
    case OP_make_arg_ref:
        return get_u16(p + 5) < b->arg_count;
    case OP_make_var_ref_ref:
        return get_u16(p + 5) < b->closure_var_count;
    case OP_push_atom_value:
        return !__JS_AtomIsTaggedInt(get_u32(p + 1));
    }
    switch (short_opcode_info(op).fmt) {
    case OP_FMT_none_loc:
        return short_opcode_index(op) < b->var_count;
    case OP_FMT_loc8:
        return p[1] < b->var_count;
    case OP_FMT_loc:
        return get_u16(p + 1) < b->var_count;
    case OP_FMT_none_arg:
        return short_opcode_index(op) < b->arg_count;
    case OP_FMT_arg:
        return get_u16(p + 1) < b->arg_count;
    case OP_FMT_none_var_ref:
        return short_opcode_index(op) < b->closure_var_count;
    case OP_FMT_var_ref:
        return get_u16(p + 1) < b->closure_var_count;
    case OP_FMT_const8:
        idx = p[1];
        goto has_const;
    case OP_FMT_const:
        idx = get_u32(p + 1);
    has_const:
        if (idx >= (uint32_t)b->cpool_count)
            return FALSE;
        if (op == OP_fclosure || op == OP_fclosure8)
            return JS_VALUE_GET_TAG(b->cpool[idx]) == JS_TAG_FUNCTION_BYTECODE;
        return TRUE;
    default:
        return TRUE;
    }
}

/* the depth before each instruction, the catch or for_of_start whose
   catch offset is the innermost on the stack there (catchpos, -1 for none)
   and the leaders, as compute_stack_size finds them for the instructions
   above, with its rules for the catch offsets */
static int rv_depths(RVPass *s)
{
    int32_t *work = s->newpc; /* not yet filled in */
    int nwork = 0, pos, npop, npush, term, target, d, c, tc, td, op;
    s->depth[0] = 0;
    s->catchpos[0] = -1;
    s->leader[0] = RV_LEAD_JUMP;
    work[nwork++] = 0;
    while (nwork > 0) {
        pos = work[--nwork];
        for (;;) {
            if (rv_old_info(s->bc, pos, &npop, &npush, &term, &target) < 0)
                goto fail_op;
            op = s->bc[pos];
            d = s->depth[pos];
            c = s->catchpos[pos];
            if (!rv_operands_ok(s, pos, op))
                goto fail;
            if (d < npop)
                goto fail;
            d = d - npop + npush;
            if (d > s->b->stack_size)
                goto fail;
            /* the target gets d and c, but for these */
            td = d;
            tc = c;
            switch (op) {
            case OP_catch:
                c = pos;
                break;
            case OP_gosub:
                td = d + 1; /* the return address */
                if (td > s->b->stack_size)
                    goto fail;
                break;
            case OP_for_of_start: case OP_for_await_of_start:
                c = pos;
                break;
            case OP_drop:
                if (c >= 0 && d == rv_catch_level(s, c))
                    c = s->catchpos[c];
                break;
            case OP_nip: case OP_nip1:
                if (c >= 0 && d - 1 == rv_catch_level(s, c))
                    c = s->catchpos[c];
                break;
            case OP_iterator_close:
                if (c >= 0 && d + 2 == rv_catch_level(s, c))
                    c = s->catchpos[c];
                break;
            case OP_with_get_var: case OP_with_delete_var:
                td = d + 1; /* the result in place of the object */
                break;
            case OP_with_make_ref: case OP_with_get_ref:
                td = d + 2; /* the object and the result */
                break;
            case OP_with_put_var:
                td = d - 1; /* the value taken with the object */
                break;
            case OP_nip_catch:
                /* catch_offset ... a -> a */
                if (c < 0)
                    goto fail;
                d = rv_catch_level(s, c) + 1;
                c = s->catchpos[c];
                break;
            }
            if (target >= 0) {
                if (target >= s->len)
                    goto fail;
                s->leader[target] |= RV_LEAD_JUMP |
                    (op == OP_catch ? RV_LEAD_CATCH : RV_LEAD_OTHER);
                if (s->depth[target] < 0) {
                    /* a conditional jump popped its condition: d is after it */
                    s->depth[target] = td;
                    s->catchpos[target] = tc;
                    work[nwork++] = target;
                } else if (s->depth[target] != td || s->catchpos[target] != tc) {
                    goto fail;
                }
            }
            pos += short_opcode_info(op).size;
            if (pos >= s->len)
                break;
            if (term || target >= 0)
                s->leader[pos] |= RV_LEAD_FALL;
            if (term)
                break;
            if (s->depth[pos] >= 0) {
                if (s->depth[pos] != d || s->catchpos[pos] != c)
                    goto fail;
                break;
            }
            s->depth[pos] = d;
            s->catchpos[pos] = c;
        }
    }
    return 0;
 fail_op:
    /* the pass takes every opcode of final code (engine/test/regvm/opcodes.txt):
       one it does not is OP_invalid or past the last */
    s->crafted = TRUE;
    s->rt->rv_stat[3] = s->bc[pos];
    s->rt->rv_stat[4] = pos;
    return -1;
 fail:
    s->crafted = TRUE;
    s->rt->rv_stat[3] = -2;
    s->rt->rv_stat[4] = pos;
    return -1;
}

static void rv_emit(RVPass *s, int op)
{
    s->last = s->code.size;
    s->last_dst = -1;
    if (dbuf_putc(&s->code, op))
        s->failed = TRUE;
}

static void rv_byte(RVPass *s, int v);

/* a return of kind 0 to 3 (RV_RET to RV_RETU): the form for the number of
   slots of the frame when there is one */
static void rv_emit_ret(RVPass *s, int kind)
{
    int fofs = s->b->arg_count + 2 + s->var_count;
    if (s->vr_heap) {
        /* the var_refs on the heap closed and freed, then a return of a
           frame with none (regvm-ops-wide.h) */
        rv_emit(s, RV_CLW);
        rv_byte(s, 1);
    } else if (s->b->var_ref_count) {
        rv_emit(s, RV_RETCT + kind); /* closes the var_refs first */
        return;
    }
    if (fofs <= RV_RETN_MAX)
        rv_emit(s, RV_RETN + kind * (RV_RETN_MAX + 1) + fofs);
    else
        rv_emit(s, RV_RET + kind);
}

/* a value written into a field too narrow for it (a constant index or a
   slot into a byte, a slot into int8_t, a temp into a mask, an entry past
   the stack of the old code), or any other state the pass cannot be in for
   code a compiler made: a mistake of the pass. The native test host
   (engine/test/build.sh defines RV_SELF_CHECK) stops at once, so that its
   corpus and test262 find it. Any other build gives the conversion up, and
   a call of the function throws (rv_refused, regvm.h). */
static void rv_overflow(RVPass *s)
{
#ifdef RV_SELF_CHECK
    abort();
#endif
    s->failed = TRUE;
}

/* an operand byte: a slot (int8_t), a constant index or a count (uint8_t) */
static void rv_byte(RVPass *s, int v)
{
    if (v < -128 || v > 255)
        rv_overflow(s);
    if (dbuf_putc(&s->code, (uint8_t)v))
        s->failed = TRUE;
}

static void rv_u16(RVPass *s, int v)
{
    if (v < -32768 || v > 65535)
        rv_overflow(s);
    if (dbuf_put_u16(&s->code, v))
        s->failed = TRUE;
}

static void rv_u32(RVPass *s, uint32_t v)
{
    if (dbuf_put_u32(&s->code, v))
        s->failed = TRUE;
}

/* a jump to fill in when the code is done (rv_patch) */
typedef struct RVFix {
    int32_t pos;        /* of the off16 */
    int32_t target;     /* the old target */
    int32_t island;     /* the JMPW it can go through, -1 none */
    int32_t at;         /* the old instruction that made it */
} RVFix;

static void rv_jump_to(RVPass *s, int old_target)
{
    RVFix f;
    f.pos = s->code.size;
    f.target = old_target;
    f.island = -1;
    f.at = s->pos;
    if (dbuf_put(&s->fix, (uint8_t *)&f, sizeof(f)))
        s->failed = TRUE;
    rv_u16(s, 0);
}

/* JMPW to the old target */
static void rv_jmpw(RVPass *s, int old_target)
{
    int32_t f[2];
    rv_emit(s, RV_JMPW);
    f[0] = s->code.size;
    f[1] = old_target;
    if (dbuf_put(&s->fixw, (uint8_t *)f, sizeof(f)))
        s->failed = TRUE;
    rv_u32(s, 0);
}

/* the islands of the jumps made by the old instruction just read, from the
   RVFix at fstart on, in a function whose code is too long for an off16
   (rv_build): after the instruction, a JMPW to the old target for each jump,
   which the off16 names when its target is too far, and a JMPW over them
   for the code that falls through. JMPW does not poll the interrupt flag,
   so that a jump through an island polls it once, as QuickJS's interpreter
   does. */
static void rv_islands(RVPass *s, size_t fstart, int op)
{
    int last = s->last == -2 ? -2 : -1, over = -1;
    size_t i, n = s->fix.size / sizeof(RVFix);
    if (op != OP_goto && op != OP_goto8 && op != OP_goto16) {
        rv_emit(s, RV_JMPW);
        over = s->code.size;
        rv_u32(s, 0);
    }
    for (i = fstart; i < n; i++) {
        int32_t island = s->code.size, target = ((RVFix *)s->fix.buf)[i].target;
        rv_jmpw(s, target);
        ((RVFix *)s->fix.buf)[i].island = island;
    }
    if (over >= 0 && !s->failed)
        put_u32(s->code.buf + over, s->code.size - (over + 4));
    s->last = last;
    s->last_dst = -1;
}

/* the folding of regvm-fold.h, which reads the helpers below */
static int rv_fold_tslot(RVPass *s, int depth);
static BOOL rv_fold_folded(RVPass *s, int i);
static void rv_fold_block_end(RVPass *s, int n);

/* the slot of the temp of a depth, an int8_t: one below stack_size, or the
   one at stack_size, the end of the stack a window op names
   (regvm-ops-window.h). In a function that folds, the temp of the layout
   of regvm-fold.h, and a depth inside a folded Array has none. */
static int rv_tslot(RVPass *s, int depth)
{
    int t = depth;
    if (depth < 0 || depth > s->b->stack_size)
        rv_overflow(s);
    if (unlikely(s->fold))
        return rv_fold_tslot(s, depth);
    if (t > s->b->stack_size || s->first_temp + t > 127)
        rv_overflow(s);
    return s->first_temp + t;
}

/* the scratch slot of a wide frame (regvm-ops-wide.h), past the temps */
static int rv_scratch(RVPass *s)
{
    int t = s->fold ? s->tcap : s->b->stack_size;
    if (s->first_temp + t > 127)
        rv_overflow(s);
    return s->first_temp + t;
}

/* the temps owned by the stack below its top n entries. The catch offsets
   there are among them (each is a temp of its own, made by CATCH or
   for_of_start), so that the unwinding (rv_unwind) finds them: a stack
   model that lost one would let an exception run past its catch, so it
   stops here.

   No function of the register loop or of its pass takes or returns a
   struct by value, which the generator of internal/cengine refuses: the
   mask goes by pointer, and these macros keep the forms of the call
   sites, such as rv_throws(s, rv_owned(s, 0)). */
#define rv_owned(s, n) \
    ({ RVMask rv_owned_m_; rv_owned_p((s), (n), &rv_owned_m_); rv_owned_m_; })
#define rv_pcmap_put(s, end, old, m) \
    do { RVMask rv_put_m_ = (m); rv_pcmap_put_p((s), (end), (old), &rv_put_m_); } while (0)
#define rv_throws(s, m) \
    do { RVMask rv_throws_m_ = (m); rv_throws_p((s), &rv_throws_m_); } while (0)
#define rv_jump_throws(s, m, q, own) \
    do { RVMask rv_jump_m_ = (m); rv_jump_throws_p((s), &rv_jump_m_, (q), (own)); } while (0)

static void rv_owned_p(RVPass *s, int below_top, RVMask *out)
{
    RVMask mask = { { 0, 0, 0, 0 } };
    int i, c, l;
    for (i = 0; i < s->sp - below_top; i++) {
        if (s->st[i].kind != RV_E_TEMP)
            continue;
        if (s->st[i].slot < s->first_temp || s->st[i].slot - s->first_temp >= RV_TEMPS_MAX) {
            rv_overflow(s);
            continue;
        }
        rv_mask_set(&mask, s->st[i].slot - s->first_temp);
    }
    for (c = s->catchpos[s->pos]; c >= 0; c = s->catchpos[c]) {
        l = rv_catch_level(s, c);
        if (l < s->sp - below_top && !rv_mask_has(&mask, rv_tslot(s, l) - s->first_temp))
            abort();
    }
    *out = mask;
}

/* an entry of the pcmap: the instruction just emitted leaves pc at end when
   it throws there, QuickJS's interpreter names that at its pc old, and the
   temps of mask are owned across it. The entries are sorted by end as the code
   is emitted in order. The temps T[32] up go into pcmap_hi, which rv_build
   keeps when an entry has one. */
static void rv_pcmap_put_p(RVPass *s, uint32_t end, int32_t old, const RVMask *mask)
{
    RVPcMask m;
    RVPcHi h;
    /* the conversion is given up (a buffer of the pass could not grow, or
       the instruction is one the pass refuses): the code may hold nothing of
       this instruction, so there is no end to record and none to check */
    if (s->failed)
        return;
    m.end = end;
    m.old = old;
    m.mask = mask->w[0];
    memcpy(h.mask, mask->w + 1, sizeof(h.mask));
    s->hi |= (mask->w[1] | mask->w[2] | mask->w[3]) != 0;
#ifdef RV_INJECT
    m.start = s->last;
    m.pos = s->ipos;
    m.hooks = RV_HOOK_OWN;
#endif
    if (s->pcmap.size &&
        ((RVPcMask *)(s->pcmap.buf + s->pcmap.size) - 1)->end >= m.end)
        abort(); /* two entries for one pc */
    if (dbuf_put(&s->pcmap, (uint8_t *)&m, sizeof(m)) ||
        dbuf_put(&s->pcmap_hi, (uint8_t *)&h, sizeof(h)))
        s->failed = TRUE;
}

/* for the throw-injection test (engine/regvm-inject.h): the places where
   the instruction whose last pcmap entry was just put can throw, when it is
   not the one place of an instruction that throws by itself (RV_HOOK_OWN) */
static void rv_hooks(RVPass *s, int hooks)
{
#ifdef RV_INJECT
    if (!s->failed)
        ((RVPcMask *)(s->pcmap.buf + s->pcmap.size) - 1)->hooks = hooks;
#else
    (void)s;
    (void)hooks;
#endif
}

/* the instruction just emitted can throw: the temps owned across it, and
   the old pc of rv_one's instruction for its line */
static void rv_throws_p(RVPass *s, const RVMask *mask)
{
    rv_pcmap_put_p(s, s->code.size, rv_old_pc_at(s->bc, s->pos), mask);
}

/* the conditional jump just emitted, for the old branch at q (if_false,
   if_true and their short forms), can stop at an interrupt. QuickJS's
   interpreter reads the flag once it has jumped or not, and names the pc
   before its target or the one inside the branch. The register jump reads it
   before it moves pc, and says which of the two it was by the pc it leaves
   (RV_JUMP_POLL in regvm-run.h): one before its end when it jumps, its end
   when it does not. With own set the instruction can also throw by itself
   (a compare), at its end and named at rv_one's instruction, and the two of
   the interrupt are one further down. */
static void rv_jump_throws_p(RVPass *s, const RVMask *mask, int q, BOOL own)
{
    const uint8_t *p = s->bc + q;
    uint32_t end = s->code.size;
    int32_t target, next;
    if (p[0] == OP_if_false8 || p[0] == OP_if_true8) {
        target = q + 1 + (int8_t)p[1];
        next = q + 2;
    } else {
        target = q + 1 + (int32_t)get_u32(p + 1);
        next = q + 5;
    }
    rv_pcmap_put_p(s, end - 1 - own, target - 1, mask);
    rv_pcmap_put_p(s, end - own, next - 1, mask);
    if (own)
        rv_pcmap_put_p(s, end, s->pos, mask);
    rv_hooks(s, own ? RV_HOOK_OWN_POLL : RV_HOOK_APART);
}

/* the constant v, borrowed: it lives as long as b (an atom of its code or an
   entry of its cpool), so the register code holds no reference to it. An
   instruction that keeps a constant with a reference count takes one
   (LOADKR, LOADKRL, the slow sides). The first RV_K_NEAR constants are
   named by a byte and shared by every use of the same value. A constant
   past them gets an index of its own at each use, read only by LOADKW
   (rv_push), so that a function of thousands of constants converts in a
   time linear in its size. The near ones are found by their value in kh,
   a table of RV_KH_N slots that holds 1 + the index of each, or 0, filled
   in the order they come: a constant gets the index it would get from a
   search of k[0] to k[RV_K_NEAR - 1], in a few reads in place of up to
   RV_K_NEAR. */
#define RV_K_NEAR 256
#define RV_KH_BITS 9
#define RV_KH_N (1 << RV_KH_BITS) /* twice RV_K_NEAR: a free slot is near */
static int rv_k(RVPass *s, JSValue v)
{
    uint64_t key = v.u.uint64 ^ ((uint64_t)(uint32_t)JS_VALUE_GET_TAG(v) << 32);
    uint32_t h = (uint32_t)((key * 0x9e3779b97f4a7c15ull) >> (64 - RV_KH_BITS));
    int i;
    while ((i = s->kh[h]) != 0) {
        i--;
        if (JS_VALUE_GET_TAG(s->k[i]) == JS_VALUE_GET_TAG(v) &&
            s->k[i].u.uint64 == v.u.uint64)
            return i;
        h = (h + 1) & (RV_KH_N - 1);
    }
    if (dbuf_put(&s->kbuf, (uint8_t *)&v, sizeof(v))) {
        s->failed = TRUE;
        return 0;
    }
    s->k = (JSValue *)s->kbuf.buf;
    if (s->k_count < RV_K_NEAR) {
        s->kh[h] = s->k_count + 1;
        if (JS_VALUE_HAS_REF_COUNT(v))
            s->kref[s->k_count >> 6] |= (uint64_t)1 << (s->k_count & 63);
    }
    return s->k_count++;
}

/* whether a closure captures the local or argument of the slot: a closure
   can write it through its JSVarRef, under a lazy read of it */
static BOOL rv_captured(RVPass *s, int slot)
{
    JSFunctionBytecode *b = s->b;
    int i;
    if (!b->var_ref_count)
        return FALSE;
    if (slot < 0 && slot >= -b->var_count)
        i = b->arg_count + b->var_count + slot;      /* a local */
    else if (slot < -s->var_count - 1 && slot >= -(b->arg_count + 1 + s->var_count))
        i = slot + b->arg_count + 1 + s->var_count;  /* an argument */
    else
        return FALSE;
    return b->vardefs[i].is_captured;
}

static void rv_materialize(RVPass *s, int i);

static BOOL rv_kref(RVPass *s, int k)
{
    return (s->kref[k >> 6] >> (k & 63)) & 1;
}

/* a slot no byte names: a local, argument, this or func of a function of
   more than 128 of them. Only MOVW, SETLW and CHKLW (regvm-ops-wide.h) name
   it, with 16 bits. */
static BOOL rv_far_slot(int slot)
{
    return slot < -128 || slot > 127;
}

/* op d16 s16 */
static void rv_movw(RVPass *s, int op, int d, int src)
{
    rv_emit(s, op);
    rv_u16(s, d);
    rv_u16(s, src);
}

static void rv_push(RVPass *s, int kind, int slot, int k)
{
    RVEnt *e;
    if (s->sp >= s->b->stack_size)
        rv_overflow(s);
    e = &s->st[s->sp++];
    if (kind == RV_E_SLOT && rv_far_slot(slot)) {
        /* a far local is copied into its temp now, so that an entry of
           kind RV_E_SLOT always has a byte slot */
        int t = rv_tslot(s, s->sp - 1);
        rv_movw(s, RV_MOVW, t, slot);
        e->kind = RV_E_TEMP;
        e->slot = t;
        e->k = 0;
        return;
    }
    if (kind == RV_E_K && k >= RV_K_NEAR) {
        /* a constant no byte can name goes into its temp now, so that an
           entry of kind RV_E_K always has a byte index */
        int t = rv_tslot(s, s->sp - 1);
        rv_emit(s, RV_LOADKW);
        rv_u16(s, t);
        rv_u32(s, k);
        e->kind = RV_E_TEMP;
        e->slot = t;
        e->k = 0;
        return;
    }
    if (slot != (int8_t)slot || k != (uint8_t)k)
        rv_overflow(s);
    e->kind = kind;
    e->slot = slot;
    e->k = k;
    /* a captured local is never read lazily: it is copied into its temp
       here, before anything can run that writes it */
    if (kind == RV_E_SLOT && rv_captured(s, slot))
        rv_materialize(s, s->sp - 1);
}

/* the instruction just emitted wrote one result, to the temp d given as its
   first operand byte, through RV_PUTD (regvm-run.h) and after it can
   throw: a store of that temp to a local right after (rv_store) makes the
   byte name the local instead. Called after the last byte of the
   instruction and its rv_throws. */
static void rv_mark_dst(RVPass *s, int d)
{
    s->last_dst = d;
}

/* entry i into its canonical temp, as a block end wants it. An entry of a
   folded range is in its Array already. */
static void rv_materialize(RVPass *s, int i)
{
    RVEnt *e = &s->st[i];
    int t;
    if (unlikely(s->fold) && rv_fold_folded(s, i))
        return;
    t = rv_tslot(s, i);
    switch (e->kind) {
    case RV_E_SLOT:
        rv_emit(s, RV_MOV);
        rv_byte(s, t);
        rv_byte(s, e->slot);
        break;
    case RV_E_K:
        rv_emit(s, rv_kref(s, e->k) ? RV_LOADKR : RV_LOADK);
        rv_byte(s, t);
        rv_byte(s, e->k);
        break;
    case RV_E_TEMP:
        /* temps are made at the slot of their depth and do not move */
        if (e->slot != t)
            rv_overflow(s);
        return;
    }
    /* a store of it right after makes it MOVL, LOADKL or LOADKRL */
    rv_mark_dst(s, t);
    e->kind = RV_E_TEMP;
    e->slot = t;
}

static void rv_block_end(RVPass *s, int n)
{
    int i;
    /* a stack that folds has the layout of the depth the block ends with
       (regvm-fold.h) */
    if (unlikely(s->fold))
        rv_fold_block_end(s, n);
    for (i = 0; i < n; i++)
        rv_materialize(s, i);
}

/* before a store to the local or argument slot: the lazy reads of it on the
   stack become copies */
static void rv_before_store(RVPass *s, int slot)
{
    int i;
    for (i = 0; i < s->sp; i++) {
        if (s->st[i].kind == RV_E_SLOT && s->st[i].slot == slot)
            rv_materialize(s, i);
    }
}

/* an operand that must be a slot: a constant goes into its temp first */
static int rv_slot_of(RVPass *s, int i)
{
    if (s->st[i].kind == RV_E_K)
        rv_materialize(s, i);
    return s->st[i].slot;
}

/* the slots of local i, argument i and this, below F */
static int rv_loc(RVPass *s, int i)
{
    return -s->b->var_count + i;
}

static int rv_arg(RVPass *s, int i)
{
    return -(s->b->arg_count + 1 + s->var_count) + i;
}

static int rv_this(RVPass *s)
{
    return -(s->var_count + 1);
}

static int rv_local_slot(RVPass *s, int op, const uint8_t *p)
{
    switch (op) {
    case OP_get_loc: case OP_put_loc: case OP_set_loc:
        return rv_loc(s, get_u16(p + 1));
    case OP_get_loc8: case OP_put_loc8: case OP_set_loc8:
    case OP_inc_loc: case OP_dec_loc: case OP_get_loc8_get_field:
        return rv_loc(s, p[1]);
    case OP_get_arg: case OP_put_arg: case OP_set_arg:
        return rv_arg(s, get_u16(p + 1));
    case OP_get_loc0: case OP_get_loc1: case OP_get_loc2: case OP_get_loc3:
        return rv_loc(s, op - OP_get_loc0);
    case OP_put_loc0: case OP_put_loc1: case OP_put_loc2: case OP_put_loc3:
        return rv_loc(s, op - OP_put_loc0);
    case OP_set_loc0: case OP_set_loc1: case OP_set_loc2: case OP_set_loc3:
        return rv_loc(s, op - OP_set_loc0);
    case OP_get_loc0_get_field:
        return rv_loc(s, 0);
    case OP_get_arg0: case OP_get_arg1: case OP_get_arg2: case OP_get_arg3:
        return rv_arg(s, op - OP_get_arg0);
    case OP_put_arg0: case OP_put_arg1: case OP_put_arg2: case OP_put_arg3:
        return rv_arg(s, op - OP_put_arg0);
    case OP_set_arg0: case OP_set_arg1: case OP_set_arg2: case OP_set_arg3:
        return rv_arg(s, op - OP_set_arg0);
    case OP_get_arg0_get_field:
        return rv_arg(s, 0);
    case OP_get_arg1_get_field:
        return rv_arg(s, 1);
    default:
        abort();
    }
}

/* put the top of the stack into the local or argument slot */
static void rv_store(RVPass *s, int slot)
{
    RVEnt e;
    if (rv_far_slot(slot)) {
        /* through the temp of the entry: no entry reads a far slot
           lazily, so nothing on the stack is a copy to make first */
        rv_materialize(s, s->sp - 1);
        rv_movw(s, RV_SETLW, slot, s->st[--s->sp].slot);
        return;
    }
    rv_before_store(s, slot);
    e = s->st[--s->sp];
    switch (e.kind) {
    case RV_E_TEMP:
        if (s->last >= 0 && s->last_dst == e.slot) {
            /* fold into the instruction that made it, which marked its d
               (rv_mark_dst): its d, the first operand byte, names the
               local, and add, sub, MOV, the LOADKs, GETF and GETVR take their L forms */
            uint8_t *c = s->code.buf + s->last;
            switch (c[0]) {
            case RV_ADD: c[0] = RV_ADDL; break;
            case RV_ADDK: c[0] = RV_ADDKL; break;
            case RV_SUB: c[0] = RV_SUBL; break;
            case RV_SUBK: c[0] = RV_SUBKL; break;
            case RV_MOV: c[0] = RV_MOVL; break;
            case RV_LOADK: c[0] = RV_LOADKL; break;
            case RV_LOADKR: c[0] = RV_LOADKRL; break;
            case RV_GETF: c[0] = RV_GETFL; break;
            case RV_GETVR: c[0] = RV_GETVRL; break;
            }
            c[1] = slot;
            s->last_dst = -1;
            return;
        }
        rv_emit(s, RV_SETL);
        rv_byte(s, slot);
        rv_byte(s, e.slot);
        break;
    case RV_E_SLOT:
        if (e.slot == slot)
            return;
        rv_emit(s, RV_MOVL);
        rv_byte(s, slot);
        rv_byte(s, e.slot);
        break;
    case RV_E_K:
        rv_emit(s, rv_kref(s, e.k) ? RV_LOADKRL : RV_LOADKL);
        rv_byte(s, slot);
        rv_byte(s, e.k);
        break;
    }
}

/* a shuffle of the top n_in entries into n_out, at most 6 (dup, nip, swap,
   insert2, insert3, perm3, rot3r, and the rest in regvm-ops-object.h): new
   entry j is old entry map[j]. A local or a constant is only named again.
   A temp keeps the rule that it lives at the slot of its depth: one new
   place owns it (its own depth if it stays there), and it is brought there
   by SWAP or MOVT, then every other place gets a copy by MOV. A temp no new
   entry names is freed. Nothing here can throw. */
static void rv_shuffle(RVPass *s, int n_in, const int8_t *map, int n_out)
{
    int base = s->sp - n_in, n = max_int(n_in, n_out), i, j, k, x;
    RVEnt old[6];
    int owner[6], cur[6];

    if (base + n_out > s->b->stack_size)
        rv_overflow(s);
    for (i = 0; i < n_in; i++) {
        old[i] = s->st[base + i];
        owner[i] = -1;
        if (old[i].kind != RV_E_TEMP)
            continue;
        for (j = 0; j < n_out; j++) {
            if (map[j] == i && (j == i || owner[i] < 0))
                owner[i] = j;
        }
        if (owner[i] < 0) {
            rv_emit(s, RV_FREE);
            rv_byte(s, old[i].slot);
        }
    }
    /* cur[j]: the old entry whose value is in the temp of depth base + j */
    for (j = 0; j < n; j++)
        cur[j] = j < n_in && old[j].kind == RV_E_TEMP && owner[j] >= 0 ? j : -1;
    for (j = 0; j < n_out; j++) {
        x = map[j];
        if (old[x].kind != RV_E_TEMP || owner[x] != j || cur[j] == x)
            continue;
        for (k = 0; cur[k] != x; k++)
            ;
        rv_emit(s, cur[j] >= 0 ? RV_SWAP : RV_MOVT);
        rv_byte(s, rv_tslot(s, base + j));
        rv_byte(s, rv_tslot(s, base + k));
        cur[k] = cur[j];
        cur[j] = x;
    }
    for (j = 0; j < n_out; j++) {
        x = map[j];
        if (old[x].kind != RV_E_TEMP) {
            s->st[base + j] = old[x];
            continue;
        }
        if (owner[x] != j) {
            rv_emit(s, RV_MOV);
            rv_byte(s, rv_tslot(s, base + j));
            rv_byte(s, rv_tslot(s, base + owner[x]));
        }
        s->st[base + j].kind = RV_E_TEMP;
        s->st[base + j].slot = rv_tslot(s, base + j);
        s->st[base + j].k = 0;
    }
    s->sp = base + n_out;
}

#define RV_PART RV_PART_PASS_FN
#include "regvm-areas.h"
#undef RV_PART

/* one old instruction at pos */
static void rv_one(RVPass *s, int pos)
{
    const uint8_t *p = s->bc + pos;
    int op = p[0], slot, d, i;
    RVEnt e;

    if (unlikely(s->wide) && rv_wide_one(s, op, p))
        return;
    switch (op) {
#define RV_PART RV_PART_PASS
#include "regvm-areas.h"
#undef RV_PART
    default:
        /* rv_old_info took it: an area has no case of it */
        rv_overflow(s);
        break;
    }
    (void)i;
}

/* whether newpc has the old offset pos: a leader that is reached, which a
   whole round of rv_code gave its new offset. Nothing else of newpc is
   written in the round (it holds the work list of rv_depths). */
static BOOL rv_emitted(RVPass *s, int pos)
{
    return s->leader[pos] && s->depth[pos] >= 0;
}

/* the jumps, once the code is done: 1 when one is too far for its off16
   and has no island, its old instruction then marked in farj for the next
   round */
static int rv_patch(RVPass *s)
{
    size_t i;
    int again = 0;
    for (i = 0; i < s->fix.size / sizeof(RVFix); i++) {
        RVFix *f = (RVFix *)s->fix.buf + i;
        int to, off;
        if (!rv_emitted(s, f->target)) {
            s->crafted = TRUE; /* a jump to code not emitted */
            return -1;
        }
        to = s->newpc[f->target];
        off = to - (f->pos + 2);
        if (off != (int16_t)off) {
            if (f->island < 0) {
                if (!s->farj) {
                    s->farj = s->farj_at;
                    memset(s->farj, 0, s->len);
                }
                s->farj[f->at] = 1;
                again = 1;
                continue;
            }
            off = f->island - (f->pos + 2);
            if (off != (int16_t)off) {
                rv_overflow(s); /* the island is right after the jump */
                return -1;
            }
        }
        put_u16(s->code.buf + f->pos, off);
    }
    for (i = 0; i < s->fixw.size / 8; i++) {
        int32_t *f = (int32_t *)s->fixw.buf + 2 * i;
        if (!rv_emitted(s, f[1])) {
            s->crafted = TRUE;
            return -1;
        }
        put_u32(s->code.buf + f[0], s->newpc[f[1]] - (f[0] + 4));
    }
    return again;
}

/* folding a stack deeper than the temps */
#include "regvm-fold.h"

/* one round of the code of the function: 0 done, 1 to make again with
   more islands (rv_patch), -1 refused */
static int rv_code(RVPass *s)
{
    JSRuntime *rt = s->rt;
    int pos, i, size;
    size_t fstart;

    if (s->vr_heap)
        rv_clw_init_pass(s);
    else if (s->b->var_ref_count)
        rv_emit(s, RV_CLINIT);
    if (unlikely(s->fold))
        rv_fold_init(s);
    for (pos = 0; pos < s->len && !s->failed; pos += size) {
        size = short_opcode_info(s->bc[pos]).size;
        if (s->leader[pos]) {
            if (s->depth[pos] < 0) {
                /* not reached: skip to the next leader */
                s->sp = -1;
                continue;
            }
            i = s->sp >= 0;
            if (i)
                rv_block_end(s, s->sp);
            s->newpc[pos] = s->code.size;
            s->sp = s->depth[pos];
            if (unlikely(s->fold)) {
                s->pos = pos;
                rv_fold_leader(s, pos, i);
            } else {
                for (i = 0; i < s->sp; i++) {
                    s->st[i].kind = RV_E_TEMP;
                    s->st[i].slot = rv_tslot(s, i);
                }
            }
            s->last = -1;
        } else if (s->sp < 0) {
            continue;
        }
        /* lt and strict_eq read the branch after them */
        if (s->last == -2) {
            s->last = -1;
            continue;
        }
        /* a case chain read the instructions up to skip_to */
        if (pos < s->skip_to)
            continue;
        s->pos = pos;
#ifdef RV_INJECT
        /* QuickJS's interpreter runs a get_field after a fused head
           (get_arg0_get_field and the like) in the dispatch of the head */
        s->ipos = pos;
        if (s->iprev >= 0 && s->iprev + short_opcode_info(s->bc[s->iprev]).size == pos) {
            switch (s->bc[s->iprev]) {
            case OP_get_arg0_get_field: case OP_get_arg1_get_field:
            case OP_get_loc0_get_field: case OP_get_loc8_get_field:
                s->ipos = s->iprev;
                break;
            }
        }
        s->iprev = pos;
#endif
        fstart = s->fix.size / sizeof(RVFix);
        /* a fold that failed (out of memory) leaves a layout that the
           instruction cannot name its temps in */
        if (unlikely(s->fold))
            rv_fold_before(s, pos);
        if (!s->failed)
            rv_one(s, pos);
        if (s->failed) {
            rt->rv_stat[3] = s->bc[pos];
            rt->rv_stat[4] = pos;
        }
        if (unlikely(s->farj) && s->farj[pos] && s->fix.size / sizeof(RVFix) > fstart)
            rv_islands(s, fstart, s->bc[pos]);
        switch (s->bc[pos]) {
        case OP_goto: case OP_goto8: case OP_goto16:
        case OP_return: case OP_return_undef: case OP_return_async:
        case OP_tail_call: case OP_tail_call_method:
            s->sp = -1;
            break;
        }
        if (unlikely(s->fold) && s->sp >= 0 && !s->failed)
            rv_fold_after(s, pos);
    }
    if (!s->failed && s->sp >= 0)
        s->crafted = TRUE;
    if (s->failed || s->sp >= 0)
        return -1; /* the code must not run off its end */
    return rv_patch(s);
}

/* The scratch of the pass: one block that the runtime keeps from one
   conversion to the next (rt->rv_scratch), so that a conversion allocates
   its register code and nothing else, and makes no short lived blocks
   between the ones that stay. It holds the arrays of the old instructions
   (depth, catchpos, leader, newpc, farj), the st of a function that
   folds and the index of its constants (rv_k), then the buffers of
   rv_code, each of cap bytes. A buffer that needs more ends its round
   (rv_scratch_short), and the round is made again with that buffer twice
   as large, the block grown to fit, which leaves the code the same. The
   block is made at the first conversion. One larger than RV_SCRATCH_KEEP
   goes back at the end of its conversion, and any goes back at a
   collection (JS_RunGC), when the heap fails a request (js_heap_drain of
   quickjs.c, as the front cache does), at JS_ReleaseStart and with the
   runtime (rv_scratch_free). A conversion runs no script and makes no
   object, so none starts while another runs: rt->rv_in_pass holds the
   scratch while one works in it, and the test builds abort on a second. */
#define RV_SCRATCH_KEEP (64 * 1024)
#define RV_SC_ALIGN(n) (((size_t)(n) + 15) & ~(size_t)15)

/* RV_SC_CAP(n) is the first bytes n of a buffer of the pass: white.c sets
   it small, so that its tests see every buffer run short and its round made
   again, and RV_SC_SHORT() counts those rounds. They are n and nothing
   otherwise, and the engine is the same. */
#ifndef RV_SC_CAP
#define RV_SC_CAP(n) (n)
#endif
#ifndef RV_SC_SHORT
#define RV_SC_SHORT() ((void)0)
#endif

/* a buffer of the pass ran out of its bytes: the round ends, to be made
   again (rv_build) */
static void *rv_scratch_short(void *opaque, void *ptr, size_t size)
{
    RVPass *s = opaque;
    (void)ptr;
    (void)size;
    if (!s->sc_short)
        RV_SC_SHORT();
    s->sc_short = TRUE;
    return NULL;
}

/* the bytes of the scratch before the buffers */
static size_t rv_scratch_fixed(int len, int st_n)
{
    return RV_SC_ALIGN(sizeof(int16_t) * len) + RV_SC_ALIGN(sizeof(int32_t) * len) +
        RV_SC_ALIGN(len) + RV_SC_ALIGN(sizeof(int32_t) * (len + 1)) + RV_SC_ALIGN(len) +
        RV_SC_ALIGN(sizeof(RVEnt) * st_n) + RV_SC_ALIGN(sizeof(uint16_t) * RV_KH_N);
}

/* the first bytes of each buffer, from the length of the code: enough for
   nearly every function a compiler makes (rv_scratch_grow for the rest) */
static void rv_scratch_caps(RVPass *s, BOOL fold)
{
    size_t n = s->len;
    s->cap[RV_SC_CODE] = RV_SC_ALIGN(RV_SC_CAP((fold ? 6 : 2) * n + 64));
    s->cap[RV_SC_FIX] = RV_SC_ALIGN(RV_SC_CAP(sizeof(RVFix) * (n / 8 + 4)));
    s->cap[RV_SC_FIXW] = RV_SC_ALIGN(RV_SC_CAP(8 * 4));
    s->cap[RV_SC_PCMAP] = RV_SC_ALIGN(RV_SC_CAP(sizeof(RVPcMask) * ((fold ? n : n / 3) + 4)));
    s->cap[RV_SC_HI] = RV_SC_ALIGN(RV_SC_CAP(sizeof(RVPcHi) * ((fold ? n : n / 3) + 4)));
    s->cap[RV_SC_RETS] = RV_SC_ALIGN(RV_SC_CAP(8 * (n / 64 + 4)));
    s->cap[RV_SC_K] = RV_SC_ALIGN(RV_SC_CAP(sizeof(JSValue) * (n / 8 + 8 + (fold ? RV_FOLD_KMAX : 0))));
}

static void rv_scratch_drop(JSRuntime *rt)
{
    js_free_rt(rt, rt->rv_scratch);
    rt->rv_scratch = NULL;
    rt->rv_scratch_size = 0;
}

/* the scratch back, from outside a conversion */
static void rv_scratch_free(JSRuntime *rt)
{
#ifdef RV_SELF_CHECK
    if (rt->rv_in_pass)
        abort(); /* a conversion runs */
#endif
    rv_scratch_drop(rt);
}

/* the arrays and buffers of s in the scratch, which holds s->fixed and
   the caps. With keep, the arrays keep what they hold when the block
   grows, since their place depends on the length of the code only.
   Without, a block too small goes back before the larger one is made, so
   that the two never count at once under the memory limit. */
static int rv_scratch_fit(RVPass *s, BOOL keep)
{
    JSRuntime *rt = s->rt;
    size_t need = s->fixed;
    uint8_t *p;
    int i;

    for (i = 0; i < RV_SC_N; i++)
        need += s->cap[i];
    if (need > rt->rv_scratch_size) {
        if (!keep)
            rv_scratch_drop(rt);
        p = js_realloc_rt(rt, rt->rv_scratch, need);
        if (!p)
            return -1;
        rt->rv_scratch = p;
        rt->rv_scratch_size = js_malloc_usable_size_rt(rt, p);
    }
    p = rt->rv_scratch;
    s->depth = (int16_t *)p;
    p += RV_SC_ALIGN(sizeof(int16_t) * s->len);
    s->catchpos = (int32_t *)p;
    p += RV_SC_ALIGN(sizeof(int32_t) * s->len);
    s->leader = p;
    p += RV_SC_ALIGN(s->len);
    s->newpc = (int32_t *)p;
    p += RV_SC_ALIGN(sizeof(int32_t) * (s->len + 1));
    s->farj_at = p;
    if (s->farj)
        s->farj = p;
    p += RV_SC_ALIGN(s->len);
    s->st_at = (RVEnt *)p;
    if (s->st != s->st_in)
        s->st = s->st_at;
    p += RV_SC_ALIGN(sizeof(RVEnt) * s->st_n);
    s->kh = (uint16_t *)p;
    p += RV_SC_ALIGN(sizeof(uint16_t) * RV_KH_N);
    for (i = 0; i < RV_SC_N; i++) {
        DynBuf *d = i == RV_SC_CODE ? &s->code : i == RV_SC_FIX ? &s->fix :
            i == RV_SC_FIXW ? &s->fixw : i == RV_SC_PCMAP ? &s->pcmap :
            i == RV_SC_HI ? &s->pcmap_hi : i == RV_SC_RETS ? &s->rets : &s->kbuf;
        d->buf = p;
        d->size = 0;
        d->allocated_size = s->cap[i];
        d->error = FALSE;
        d->realloc_func = rv_scratch_short;
        d->opaque = s;
        p += s->cap[i];
    }
    return 0;
}

/* after a round that ran out of a buffer: that buffer twice as large */
static int rv_scratch_grow(RVPass *s)
{
    const DynBuf *d[RV_SC_N] = { &s->code, &s->fix, &s->fixw, &s->pcmap, &s->pcmap_hi,
                                 &s->rets, &s->kbuf };
    int i;

    for (i = 0; i < RV_SC_N; i++)
        if (d[i]->error)
            s->cap[i] *= 2;
    s->sc_short = FALSE;
    return rv_scratch_fit(s, TRUE);
}

/* the state of a round of rv_code, kept: depth, catchpos, leader, farj */
static void rv_round_init(RVPass *s)
{
    s->code.size = s->fix.size = s->fixw.size = s->pcmap.size = 0;
    s->pcmap_hi.size = s->rets.size = s->kbuf.size = 0;
    s->hi = FALSE;
    s->k = NULL;
    /* the words of kref the round before used */
    memset(s->kref, 0, sizeof(uint64_t) * ((min_int(s->k_count, RV_K_NEAR) + 63) >> 6));
    memset(s->kh, 0, sizeof(uint16_t) * RV_KH_N);
    s->k_count = 0;
    s->sp = 0;
    s->last = -1;
    s->last_dst = -1;
    s->skip_to = 0;
    s->drop_at = -1;
    s->failed = FALSE;
    s->args = FALSE;
#ifdef RV_INJECT
    s->iprev = -1;
#endif
}

/* the register code of b, or NULL: the pass refused it, or with *oom set
   (when oom is not NULL) it ran out of memory */
static RVFunc *rv_build(JSRuntime *rt, JSFunctionBytecode *b, BOOL *oom)
{
    RVPass s_s, *s = &s_s;
    RVFunc *rf = NULL;
    int i, size, round, r, first_temp, wide, fsize, vr_heap, deep, fold;
    int first_temp0, fsize0, vr_heap0;
    /* the parser and the reader set new_target_read (quickjs.c) */
    int nt = b->func_kind == JS_FUNC_NORMAL && b->new_target_read;

#ifdef RV_SELF_CHECK
    if (nt != rv_reads_new_target(b))
        abort(); /* the flag says other than the code */
#endif
    if (oom)
        *oom = FALSE;
    /* captured locals: the closure state goes under the temps, and CLINIT
       first fills it in, before the code of offset 0, which a jump may
       reach again. A frame is named by int8_t slots: its size is known
       here, before any slot is narrowed into one. */
    first_temp = RV_HDR + (b->var_ref_count ? RV_SF_SLOTS(b->var_ref_count) : 0);
    wide = b->arg_count + 2 + b->var_count + nt > 128;
    fsize = first_temp + b->stack_size + wide;
    /* so many captured locals that their var_refs would push the temps
       past slot 127: the frame keeps its JSStackFrame only, and the
       var_refs go on the heap (CLW, regvm-ops-wide.h) */
    vr_heap = b->var_ref_count && fsize > 127;
    if (vr_heap) {
        first_temp = RV_HDR + RV_SF_SLOTS(0);
        fsize = first_temp + b->stack_size + wide;
    }
    /* a stack too deep for the temps folds (regvm-fold.h), in a generator
       or async function too, whose heap frame keeps the folded temps while
       it is suspended (regvm-ops-gen.h) */
    deep = b->stack_size > RV_TEMPS_MAX || fsize > 127;
#ifdef RV_FOLD_TEST
    fold = TRUE;
#else
    fold = deep;
#endif
    /* A generator or async function has no slot of new.target (nt is 0):
       new.target is undefined in it. One with vr_heap has its table of
       var_refs only while it runs (regvm-ops-gen.h). */
    /* MOVW, SETLW and CHKLW name a local, argument, this or func with 16
       bits: the farthest, func, is slot -(arg_count + 2 + var_count + nt),
       which the parser keeps within RV_FAR_MAX (JS_FRAME_FULL of quickjs.c) */
    if (b->arg_count + 2 + b->var_count + nt > RV_FAR_MAX) {
        rt->rv_stat[3] = RV_CRAFTED;
        rt->rv_stat[4] = 0;
        return NULL;
    }
    memset(s, 0, offsetof(RVPass, st_in));
#ifdef RV_SELF_CHECK
    /* what was not cleared is not read before it is written */
    memset(s->st_in, 0xff, sizeof(*s) - offsetof(RVPass, st_in));
#endif
    s->st = s->st_in;
    s->rt = rt;
    s->b = b;
    s->bc = b->byte_code_buf;
    s->len = b->byte_code_len;
    s->var_count = b->var_count + nt;
    s->first_temp = first_temp;
    s->wide = wide;
    s->vr_heap = vr_heap;
#ifdef RV_SELF_CHECK
    if (rt->rv_in_pass)
        abort(); /* a conversion inside another */
#endif
    rt->rv_in_pass = TRUE;
    s->st_n = fold ? b->stack_size + 8 : 0;
    s->fixed = rv_scratch_fixed(s->len, s->st_n);
    rv_scratch_caps(s, fold);
    if (rv_scratch_fit(s, FALSE) < 0) {
        s->oom = TRUE;
        goto done;
    }
    memset(s->leader, 0, s->len);
    for (i = 0; i < s->len; i++) {
        s->depth[i] = -1;
        s->catchpos[i] = -1;
    }
    if (rv_depths(s))
        goto done;
    vr_heap0 = vr_heap;
    first_temp0 = first_temp;
    fsize0 = fsize;
    if (fold) {
        s->st = s->st_at;
        s->fold = TRUE;
        s->R = RV_FOLD_R;
    }
    /* a function whose code is too long for the off16 of a jump is made
       again with an island (rv_islands) after each jump that was too far,
       until none is. The islands push other jumps farther: after a few
       rounds, every jump gets one. A function that folds and runs out of
       temps is made again with Arrays of RV_FOLD_RMIN entries, and the
       test build makes one whose stack fits without folding when that
       does not fit either. */
    for (;;) {
        if (s->fold) {
            /* the frame of a function that folds is all 127 slots, with its
               var_refs on the heap, so that how deep its catch offsets nest
               is all that decides whether it fits (regvm-fold.h) */
            s->vr_heap = vr_heap = b->var_ref_count != 0;
            s->first_temp = first_temp = RV_HDR + (vr_heap ? RV_SF_SLOTS(0) : 0);
            s->tcap = min_int(RV_TEMPS_MAX - 1, 127 - first_temp - wide);
            fsize = first_temp + s->tcap + wide;
        }
        for (round = 0;;) {
            int st3 = rt->rv_stat[3], st4 = rt->rv_stat[4];
            rv_round_init(s);
            r = rv_code(s);
            if (unlikely(s->sc_short)) {
                /* the round again, with more room: what it set is undone */
                rt->rv_stat[3] = st3;
                rt->rv_stat[4] = st4;
                s->crafted = FALSE;
                if (rv_scratch_grow(s) < 0) {
                    s->oom = TRUE;
                    goto done;
                }
                continue;
            }
            if (r <= 0)
                break;
            if (round == 4)
                memset(s->farj, 1, s->len);
            round++;
        }
        if (r >= 0 || !s->fold || !s->fold_full)
            break;
        if (s->R > RV_FOLD_RMIN) {
            s->R = RV_FOLD_RMIN;
        } else if (!deep) {
            s->fold = FALSE;
            s->vr_heap = vr_heap = vr_heap0;
            s->first_temp = first_temp = first_temp0;
            fsize = fsize0;
        } else {
            /* the parser keeps the temps the catch offsets need within
               RV_FOLD_PINS_MAX (regvm-func.h), which fit at RV_FOLD_RMIN */
            s->crafted = TRUE;
            goto done;
        }
    }
    if (r < 0)
        goto done;
    /* every place a ret goes to in QuickJS's interpreter (ret_bits) has its
       GOSUB: a gosub the pass never reached, which only code read by
       JS_ReadObject has, has none, and the function is refused */
    if (b->ret_bits) {
        int n = 0;
        const uint32_t *r = (const uint32_t *)s->rets.buf;
        for (i = 0; i < (s->len + 31) / 32; i++)
            n += __builtin_popcount(b->ret_bits[i]);
        for (i = 0; i < (int)(s->rets.size / 8); i++)
            n -= r[2 * i] < (uint32_t)s->len;
        if (n != 0) {
            s->crafted = TRUE;
            goto done;
        }
    }
    {
        size_t hi = s->hi ? s->pcmap_hi.size : 0;
        size = sizeof(RVFunc) + s->code.size;
        size = (size + 15) & ~15;
        rf = js_malloc_rt(rt, size + sizeof(JSValue) * s->k_count + s->pcmap.size + hi +
                        s->rets.size);
        if (!rf) {
            s->oom = TRUE;
            goto done;
        }
        rf->b = b;
        rf->arg_count = b->arg_count;
        rf->var_count = s->var_count;
        rf->nt = nt;
        rf->stack_size = b->stack_size;
        rf->fofs = b->arg_count + 2 + s->var_count;
        rf->fofs_b = sizeof(JSValue) * rf->fofs;
        rf->this_b = sizeof(JSValue) * (1 + b->arg_count);
        rf->room_b = sizeof(JSValue) * (rf->fofs + fsize);
        rf->fsize = fsize;
        rf->temp0 = s->first_temp;
        rf->var_ref_count = b->var_ref_count;
        rf->vr_heap = vr_heap;
        rf->args = s->args;
        rf->argmax = s->args ? -1 : b->arg_count;
        rf->k_count = s->k_count;
        rf->runs = 0;
        rf->code_len = s->code.size;
        rf->k = (JSValue *)((uint8_t *)rf + size);
        rf->pcmap = (RVPcMask *)(rf->k + s->k_count);
        rf->pcmap_count = s->pcmap.size / sizeof(RVPcMask);
        memcpy(rf->code, s->code.buf, s->code.size);
        if (s->k_count)
            memcpy(rf->k, s->k, sizeof(JSValue) * s->k_count);
        memcpy(rf->pcmap, s->pcmap.buf, s->pcmap.size);
        /* the temps T[32] up of each entry, past the pcmap */
        rf->pcmap_hi = NULL;
        if (hi) {
            rf->pcmap_hi = (RVPcHi *)(rf->pcmap + rf->pcmap_count);
            memcpy(rf->pcmap_hi, s->pcmap_hi.buf, hi);
        }
        /* the return addresses of the GOSUBs, past them */
        rf->rets = (uint32_t *)((uint8_t *)(rf->pcmap + rf->pcmap_count) + hi);
        rf->ret_count = s->rets.size / 8;
        memcpy(rf->rets, s->rets.buf, s->rets.size);
    }
 done:
    if (!rf && s->crafted)
        rt->rv_stat[3] = RV_CRAFTED;
    if (oom)
        *oom = !rf && s->oom;
    rt->rv_in_pass = FALSE;
    if (rt->rv_scratch_size > RV_SCRATCH_KEEP)
        rv_scratch_free(rt);
    return rf;
}

/* the register code of b, made at its first call, or RV_REFUSED. When it
   ran out of memory b->rv stays RV_UNCONVERTED, and rv_oom_b names it: its
   call throws out of memory (JS_CallInternal), a later call tries again.
   The first conversion makes the register stack when JS_NewRuntime could
   not. */
static no_inline RVFunc *rv_convert_rt(JSRuntime *rt, JSFunctionBytecode *b)
{
    BOOL oom;
    RVFunc *rf = rv_build(rt, b, &oom);
    if (rf && rv_stack_make(rt) < 0) {
        rv_free_func(rt, rf);
        rf = NULL;
        oom = TRUE;
    }
    if (!rf && oom) {
        rt->rv_oom_b = b;
        return b->rv;
    }
    rt->rv_stat[rf ? 0 : 1]++;
    b->rv = rf ? rf : RV_REFUSED;
    return b->rv;
}

static no_inline RVFunc *rv_convert(JSContext *ctx, JSFunctionBytecode *b)
{
    return rv_convert_rt(ctx->rt, b);
}
