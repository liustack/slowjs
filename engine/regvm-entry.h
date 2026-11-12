/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The entries of the register loop: what lets a call run in it. Included
   by regvm.h after regvm-pass.h, before regvm-run.h.

   The register stack in segments. A frame never moves (var_refs and the
   argv of a call point into it), so a full stack is not grown by a
   realloc: another segment is linked above it, of the next size of the
   segments (RV_SEG_SLOTS of regvm.h: about twice the one below) up to
   RV_SEG_MAX slots, or more for a call that needs more (a
   function that reads its arguments, called with thousands of them). A
   call whose frame does not fit at the top of the segment in use starts
   the next one:

   - in the loop (rv_call_seg of regvm-ops-basic.h): func and the
     arguments are moved from the caller's window to the start of the next
     segment (rv_seg_move) and the frame is made there by the slow entry
     of a call, its dst flagged RV_DST_SEG, so that its return (and the
     unwinding of an exception) steps back to the segment of its caller
     (rv_seg_back) before it stores the result;
   - from C (JS_RegvmRun's entry): rv_run_seg runs the call on the next
     segment and steps back after it.

   Each segment is allocated from the heap of the runtime, so it counts in
   MemorySize and under the memory limit, but not toward the threshold of
   the collector (rt->rv_stack_bytes, js_trigger_gc), which cannot free
   it. It is allocated the first time it is needed and kept while the stack
   below it is in use: stepping back from a segment frees the one above
   it, so at most one segment is kept that no frame uses. All are freed with
   the runtime. What bounds the depth is the call depth, as for every call
   (max_call_depth), and the memory.

   The first segment is small (RV_SEG_FIRST) and the stack settles. A step
   onto a next segment keeps, as the most slots the stack may need
   (rt->rv_hw), those in use below it and all of its own: the depth reached
   inside a segment is not followed, only this slow path sets the mark.
   When the outermost run of the loop returns with no frame left, the first
   segment is replaced by one of the smallest size of the segments that
   holds them, up to RV_SEG_MAX, and the others are freed (rv_settle). No frame moves, as
   none is alive then. The same run again then steps onto no next segment:
   a VM that renders the same pages steps onto one for its first render
   only, and its first segment may be larger than the stack needed, never
   larger than RV_SEG_MAX. When the larger segment cannot be allocated,
   the stack stays as it is and the next outermost return tries again. */

typedef struct RVSeg {
    struct RVSeg *prev, *next;
    uintptr_t size;     /* its slots */
    uintptr_t below;    /* the slots in use below it, in the segments under
                           it (rv_settle); the slots start 16 bytes aligned,
                           as a JSValue */
    JSValue slots[];
} RVSeg;

static RVSeg *rv_seg_alloc(JSRuntime *rt, uintptr_t size)
{
    RVSeg *s = js_malloc_rt(rt, sizeof(RVSeg) + sizeof(JSValue) * size);

    if (!s)
        return NULL;
    s->next = NULL;
    s->size = size;
    s->below = 0;
    rt->rv_seg_made++;
    rt->rv_stack_bytes += js_malloc_usable_size_rt(rt, s);
    return s;
}

/* the slots of the segment after one of size slots: the next size of the
   segments (RV_SEG_SLOTS) above it, up to RV_SEG_MAX */
static uintptr_t rv_seg_grow(uintptr_t size)
{
    uintptr_t r = RV_SEG_FIRST_BYTES;

    while (r < RV_SEG_MAX_BYTES && RV_SEG_SLOTS(r) <= size)
        r *= 2;
    return RV_SEG_SLOTS(r);
}

static void rv_seg_free(JSRuntime *rt, RVSeg *s)
{
    rt->rv_stack_bytes -= js_malloc_usable_size_rt(rt, s);
    js_free_rt(rt, s);
}

/* the first segment, made by JS_NewRuntime, or at the first conversion
   when it could not be made then: 0, or -1 when it could not be allocated */
static int rv_stack_make(JSRuntime *rt)
{
    RVSeg *s;

    if (rt->rv_seg)
        return 0;
    s = rv_seg_alloc(rt, RV_SEG_FIRST);
    if (!s)
        return -1;
    s->prev = NULL;
    rt->rv_seg = s;
    rt->rv_base = rt->rv_top = s->slots;
    rt->rv_end = s->slots + s->size;
    return 0;
}

static void rv_stack_free(JSRuntime *rt)
{
    RVSeg *s = rt->rv_seg, *n;

    if (!s)
        return;
    while (s->prev)
        s = s->prev;
    while (s) {
        n = s->next;
        rv_seg_free(rt, s);
        s = n;
    }
    rt->rv_seg = NULL;
    rt->rv_base = rt->rv_top = rt->rv_end = NULL;
    rt->rv_hw = 0;
}

/* the segments the register stack has now, for JS_RegvmStat 8 */
static int rv_stack_segments(JSRuntime *rt)
{
    RVSeg *s = rt->rv_seg;
    int n = 0;

    if (!s)
        return 0;
    while (s->prev)
        s = s->prev;
    for (; s; s = s->next)
        n++;
    return n;
}

/* makes the segment above the one in use, with need slots at least, the
   one in use: allocated the first time, or again when the spare one is
   too small. The stack in use ends at rt->rv_top. Its slots, or NULL when
   it could not be allocated. */
static JSValue *rv_seg_next(JSRuntime *rt, uintptr_t need)
{
    RVSeg *s = rt->rv_seg, *n = s->next;
    uintptr_t size, below = s->below + (rt->rv_top - s->slots);

    if (n && n->size < need) {
        rv_seg_free(rt, n);
        s->next = n = NULL;
    }
    if (!n) {
        size = rv_seg_grow(s->size);
        n = rv_seg_alloc(rt, need > size ? need : size);
        if (!n)
            return NULL;
        n->prev = s;
        s->next = n;
    }
    n->below = below;
    /* the depth reached in n is not followed: all of it counts, so that
       the settled stack holds the same run again (rv_settle) */
    if (below + n->size > rt->rv_hw)
        rt->rv_hw = below + n->size;
    rt->rv_seg = n;
    rt->rv_top = n->slots;
    rt->rv_end = n->slots + n->size;
    return n->slots;
}

/* back to the segment below the one in use, which stays as the spare; the
   spare above it is freed. top is the top of the stack below: the end of
   the frame of the caller. */
static no_inline void rv_seg_back(JSRuntime *rt, JSValue *top)
{
    RVSeg *s = rt->rv_seg;

    if (s->next) {
        rv_seg_free(rt, s->next);
        s->next = NULL;
    }
    rt->rv_seg = s->prev;
    rt->rv_top = top;
    rt->rv_end = s->prev->slots + s->prev->size;
}

/* the stack settles (see above): called with rt->rv_hw set when the run of
   the loop that started at the bottom of the first segment returned, with
   no frame left. The first segment becomes one of the smallest size of the
   segments that holds rt->rv_hw slots, up to RV_SEG_MAX, and the others are freed. When
   it is that large already, the spare above it stays. When the new one
   cannot be allocated, nothing changes and rt->rv_hw stays for the next
   try. */
static no_inline void rv_settle(JSRuntime *rt)
{
    RVSeg *s = rt->rv_seg, *f, *n;
    uintptr_t size = RV_SEG_FIRST;

    if (s->prev || rt->rv_top != s->slots)
        return;
    while (size < rt->rv_hw && size < RV_SEG_MAX)
        size = rv_seg_grow(size);
    if (size > s->size) {
        f = rv_seg_alloc(rt, size);
        if (!f)
            return;
        f->prev = NULL;
        while (s) {
            n = s->next;
            rv_seg_free(rt, s);
            s = n;
        }
        rt->rv_seg = f;
        rt->rv_base = rt->rv_top = f->slots;
        rt->rv_end = f->slots + f->size;
    }
    rt->rv_hw = 0;
}

/* the top of the stack under the frame F: its end */
static inline JSValue *rv_frame_end(JSValue *F)
{
    return F + ((RVHeader *)F)->rf->fsize;
}

/* the slots a call of rf with argc arguments needs from its window: the
   extra arguments with the reference to func below them, when it has any
   (rv_enter_args, rv_entry_extra of regvm-run.h), then its frame */
static inline uintptr_t rv_call_need(RVFunc *rf, int argc)
{
    return (argc > rf->arg_count ? argc + 1 : 0) + rf->room_b / sizeof(JSValue);
}

/* a call of the loop from the window W (func, then n arguments) of the
   frame F, whose callee rf does not fit in the segment in use: the window
   moved to the start of the next segment, made large enough for it, where
   the slow entry of a call makes the frame. NULL when the segment could
   not be allocated: the window and this_v are freed and the exception is
   thrown. */
static no_inline JSValue *rv_seg_move(JSContext *ctx, JSValue *F, JSValue *W,
                                      int n, JSValue this_v, RVFunc *rf)
{
    JSRuntime *rt = ctx->rt;
    JSValue *Wn;
    int i;

    rt->rv_top = rv_frame_end(F);
    Wn = rv_seg_next(rt, rv_call_need(rf, n));
    if (!Wn) {
        for (i = 0; i <= n; i++)
            JS_FreeValue(ctx, W[i]);
        JS_FreeValue(ctx, this_v);
        JS_ThrowOutOfMemory(ctx);
        return NULL;
    }
    for (i = 0; i <= n; i++)
        Wn[i] = W[i];
    return Wn;
}

/* a call from C of rf, a function with register code, whose entry frame
   does not fit in the segment in use: run on the next segment, made large
   enough for it. new_target is taken, as JS_RegvmRun takes it. */
static no_inline JSValue rv_run_seg(JSContext *ctx, RVFunc *rf, JSValueConst func_obj,
                                    JSValueConst this_obj, JSValue new_target,
                                    int argc, JSValueConst *argv)
{
    JSRuntime *rt = ctx->rt;
    JSValue *top = rt->rv_top, ret;

    if (!rv_seg_next(rt, rv_call_need(rf, argc))) {
        JS_FreeValue(ctx, new_target);
        return JS_ThrowOutOfMemory(ctx);
    }
    ret = JS_RegvmRun(ctx, func_obj, this_obj, new_target, argc, argv);
    rv_seg_back(rt, top);
    if (rt->rv_hw && top == rt->rv_base)
        rv_settle(rt);
    return ret;
}

/* The first call. A function is converted at its first call, before it
   runs, by the entry test of the caller (rv_entry_fast for C, RV_CALLOP in
   the loop), so that its first run is in the register loop too: script
   code that runs once (top-level code, eval code, a module body, a test
   body) runs there. The conversion is the same at any call, so the test
   only moves it. */
static no_inline BOOL rv_first_call(JSRuntime *rt, JSFunctionBytecode *b)
{
    return rv_convert_rt(rt, b)->arg_count >= 0;
}

/* Code that JS_ReadObject read, before it can run: a function the pass
   refuses for code no compiler makes (RV_CRAFTED of regvm-pass.h) is a
   SyntaxError of the read, so that no script stops at it at a first call
   later. The pass runs on each function read and its code is dropped: the
   conversion stays at the first call. A read with JS_READ_OBJ_POLLS_CHECKED
   skips it with the rule of the polls (the same bytes were read with the
   checks before), so a SharedBytecode pays it at its first load only. -1
   with an exception: the SyntaxError, or out of memory. */
static int rv_read_check(JSContext *ctx, JSFunctionBytecode *b)
{
    JSRuntime *rt = ctx->rt;
    int st[5], op, pos;
    BOOL oom;
    RVFunc *rf;

    if (b->rv != RV_UNCONVERTED)
        return 0;
    memcpy(st, rt->rv_stat, sizeof(st));
    rt->rv_stat[3] = -99;
    rt->rv_stat[4] = 0;
    rf = rv_build(rt, b, &oom);
    op = rt->rv_stat[3];
    pos = rt->rv_stat[4];
    memcpy(rt->rv_stat, st, sizeof(st));
    if (rf) {
        rv_free_func(rt, rf);
        return 0;
    }
    if (oom) {
        JS_ThrowOutOfMemory(ctx);
        return -1;
    }
    if (op != RV_CRAFTED)
        return 0;
    JS_ThrowSyntaxError(ctx, "invalid bytecode: code no compiler makes, at %d", pos);
    return -1;
}
