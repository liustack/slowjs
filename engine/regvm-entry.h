/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The entries of the register loop: what lets a call run in it where it
   would have gone to the old loop. Included by regvm.h after regvm-pass.h,
   before regvm-run.h.

   The register stack in segments. A frame never moves (var_refs and the
   argv of a call point into it), so a full stack is not grown by a
   realloc: another segment is linked above it, twice the size of the one
   below up to RV_SEG_MAX slots, or more for a call that needs more (a
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
   segment is replaced by one of the power of two that holds them, up to
   RV_SEG_MAX, and the others are freed (rv_settle). No frame moves, as
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

static void rv_seg_free(JSRuntime *rt, RVSeg *s)
{
    rt->rv_stack_bytes -= js_malloc_usable_size_rt(rt, s);
    js_free_rt(rt, s);
}

/* the first segment, made by JS_SetRegvm or at the first conversion: 0,
   or -1 when it could not be allocated */
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
        size = s->size < RV_SEG_MAX / 2 ? s->size * 2 : RV_SEG_MAX;
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
   no frame left. The first segment becomes one of the power of two that
   holds rt->rv_hw slots, up to RV_SEG_MAX, and the others are freed. When
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
        size *= 2;
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

/* The first call. A function made with the register loop on is converted
   at its first call, before it runs, by the entry test of the caller
   (rv_entry_fast for the old loop and C, RV_CALLOP in the loop), so that
   its first run is in the register loop too: script code that runs once
   (top-level code, eval code, a module body, a test body) runs there. The
   conversion is the same at any call, so the test only moves it. */
static no_inline BOOL rv_first_call(JSRuntime *rt, JSFunctionBytecode *b)
{
    return rv_convert_rt(rt, b)->arg_count >= 0;
}

/* The functions that still run in the old loop, and why: the gate for
   removing it. JS_SetRegvm(ctx, 2) keeps a record of each one, with the
   count of its runs there, which JS_RegvmFallbacks gives to the host;
   JS_SetRegvm(ctx, 3) makes each such run an error (an InternalError that
   names the function and the reason), so that a test sees it. Both read
   at the entries of JS_CallInternal only (rv_old_entry). */

enum {
    RV_FB_REFUSED, /* the pass refused it */
    RV_FB_OOM,     /* its conversion ran out of memory */
    RV_FB_CTOR,    /* called as a constructor */
    RV_FB_GEN,     /* the body of a generator or of an async function */
    RV_FB_OFF,     /* made with the register loop off */
    RV_FB_OTHER,   /* converted, and run in the old loop all the same */
};

typedef struct RVFallback {
    const JSFunctionBytecode *b; /* NULL once b is freed */
    int reason, op, pos, line, col;
    int64_t calls;
    char name[64], file[128];
} RVFallback;

typedef struct RVFallbacks {
    RVFallback *v;
    int count, size;
    int *index;        /* open addressing on b, -1 for none, index_size a power of 2 */
    int index_size;
} RVFallbacks;

static uint32_t rv_fb_hash(const void *b, int size)
{
    uintptr_t h = (uintptr_t)b;
    h ^= h >> 17;
    h *= 0x9E3779B1u;
    return (uint32_t)(h >> 7) & (size - 1);
}

static int rv_fb_find(RVFallbacks *t, const JSFunctionBytecode *b)
{
    uint32_t i;

    if (!t->index_size)
        return -1;
    for (i = rv_fb_hash(b, t->index_size);; i = (i + 1) & (t->index_size - 1)) {
        if (t->index[i] < 0)
            return -1;
        if (t->v[t->index[i]].b == b)
            return t->index[i];
    }
}

/* the index of the records, made again twice as large when it is half
   full: a record whose function was freed (b NULL) keeps its place in it
   and matches nothing */
static int rv_fb_reindex(JSRuntime *rt, RVFallbacks *t, int size)
{
    int *index = js_malloc_rt(rt, sizeof(int) * size), i;
    uint32_t j;

    if (!index)
        return -1;
    for (i = 0; i < size; i++)
        index[i] = -1;
    for (i = 0; i < t->count; i++) {
        if (!t->v[i].b)
            continue;
        for (j = rv_fb_hash(t->v[i].b, size); index[j] >= 0; j = (j + 1) & (size - 1))
            ;
        index[j] = i;
    }
    js_free_rt(rt, t->index);
    t->index = index;
    t->index_size = size;
    return 0;
}

/* b is freed: its record stays, with no b, so that a new function at the
   same address gets one of its own */
static void rv_fb_forget(JSRuntime *rt, JSFunctionBytecode *b)
{
    RVFallbacks *t = rt->rv_fb;
    int i = rv_fb_find(t, b);

    if (i < 0)
        return;
    t->v[i].b = NULL;
}

static const char *const rv_fb_reasons[] = {
    "refused", "out of memory", "constructor", "generator or async",
    "made with the register loop off", "other",
};

/* the record of b, made at its first run in the old loop, NULL when there
   is no memory for it (the run is then not counted) */
static RVFallback *rv_fb_record(JSContext *ctx, JSFunctionBytecode *b, int reason)
{
    JSRuntime *rt = ctx->rt;
    RVFallbacks *t = rt->rv_fb;
    RVFallback *f;
    char buf[ATOM_GET_STR_BUF_SIZE];
    uint32_t j;
    int i;

    if (!t) {
        t = js_mallocz_rt(rt, sizeof(*t));
        if (!t)
            return NULL;
        rt->rv_fb = t;
    }
    i = rv_fb_find(t, b);
    if (i >= 0 && t->v[i].reason == reason)
        return &t->v[i];
    if (t->count == t->size) {
        int size = t->size ? 2 * t->size : 64;
        RVFallback *v = js_realloc_rt(rt, t->v, sizeof(*v) * size);
        if (!v)
            return NULL;
        t->v = v;
        t->size = size;
    }
    if (2 * (t->count + 1) > t->index_size &&
        rv_fb_reindex(rt, t, t->index_size ? 2 * t->index_size : 128) < 0)
        return NULL;
    if (i >= 0)
        t->v[i].b = NULL; /* the same function, another reason: a record of its own */
    f = &t->v[t->count++];
    memset(f, 0, sizeof(*f));
    f->b = b;
    f->reason = reason;
    f->op = f->pos = 0;
    if (reason == RV_FB_REFUSED) {
        /* what the pass stops at, from a run of it again, with the counts
           of JS_RegvmStat kept as they were (as JS_RegvmFuncStat does) */
        int st[5];
        RVFunc *rf;
        memcpy(st, rt->rv_stat, sizeof(st));
        rt->rv_stat[3] = -99;
        rt->rv_stat[4] = 0;
        rf = rv_build(rt, b, NULL);
        if (rf)
            rv_free_func(rt, rf);
        f->op = rt->rv_stat[3];
        f->pos = rt->rv_stat[4];
        memcpy(rt->rv_stat, st, sizeof(st));
    }
    if (b->func_name != JS_ATOM_NULL)
        snprintf(f->name, sizeof(f->name), "%s", JS_AtomGetStrRT(rt, buf, sizeof(buf), b->func_name));
    if (b->has_debug) {
        snprintf(f->file, sizeof(f->file), "%s", JS_AtomGetStrRT(rt, buf, sizeof(buf), b->debug.filename));
        f->line = find_line_num(ctx, b, -1, &f->col);
    }
    for (j = rv_fb_hash(b, t->index_size); t->index[j] >= 0; j = (j + 1) & (t->index_size - 1))
        ;
    t->index[j] = f - t->v;
    return f;
}

static void rv_fb_free(JSRuntime *rt)
{
    RVFallbacks *t = rt->rv_fb;

    if (!t)
        return;
    js_free_rt(rt, t->v);
    js_free_rt(rt, t->index);
    js_free_rt(rt, t);
    rt->rv_fb = NULL;
}

/* why b runs in the old loop, called with flags of JS_CallInternal */
static int rv_fb_reason(JSFunctionBytecode *b, int flags)
{
    if (b->rv == RV_OFF)
        return RV_FB_OFF;
    if (flags & JS_CALL_FLAG_GENERATOR)
        return RV_FB_GEN;
    if (b->rv == RV_REFUSED)
        return RV_FB_REFUSED;
    if (b->rv == RV_FAILED)
        return RV_FB_OOM;
    if (flags & JS_CALL_FLAG_CONSTRUCTOR)
        return RV_FB_CTOR;
    return RV_FB_OTHER;
}

/* JS_CallInternal is about to run b in the old loop (flags its own): the
   conversion of a function not converted yet, which a call that came here
   straight (a constructor, a resumed generator) has not had, the record
   of JS_SetRegvm 2 and the error of 3. -1 with an exception: the
   conversion ran out of memory and RV_OOM_FALLBACK is 0, or mode 3. */
static no_inline int rv_old_entry(JSContext *ctx, JSFunctionBytecode *b, int flags)
{
    JSRuntime *rt = ctx->rt;
    int reason;
    RVFallback *f;

    /* the conversion at the entry test of this call, or at the resume of
       its generator (rv_gen_ok), ran out of memory */
    if (unlikely(rt->rv_oom_b == b)) {
        rt->rv_oom_b = NULL;
        JS_ThrowOutOfMemory(ctx);
        return -1;
    }
    if (b->rv == RV_UNCONVERTED && !(flags & JS_CALL_FLAG_GENERATOR)) {
        rv_convert(ctx, b);
        if (b->rv == RV_UNCONVERTED) {
            /* out of memory, with the fallback off (rv_convert): tried
               again at the next call */
            rt->rv_oom_b = NULL;
            JS_ThrowOutOfMemory(ctx);
            return -1;
        }
    }
    if (rt->regvm_on < 2)
        return 0;
    reason = rv_fb_reason(b, flags);
    f = rv_fb_record(ctx, b, reason);
    if (f)
        f->calls++;
    if (rt->regvm_on == 3) {
        char buf[ATOM_GET_STR_BUF_SIZE];
        JS_ThrowInternalError(ctx, "regvm: %s runs in the old loop: %s",
                              b->func_name == JS_ATOM_NULL ? "<anonymous>" :
                              JS_AtomGetStrRT(rt, buf, sizeof(buf), b->func_name),
                              rv_fb_reasons[reason]);
        return -1;
    }
    return 0;
}

int JS_RegvmFallbacks(JSRuntime *rt,
                      int (*fn)(void *opaque, const JSRegvmFallback *f),
                      void *opaque)
{
    RVFallbacks *t = rt->rv_fb;
    JSRegvmFallback e;
    int i, r;

    if (!t)
        return 0;
    for (i = 0; i < t->count; i++) {
        RVFallback *f = &t->v[i];
        e.reason = rv_fb_reasons[f->reason];
        e.op = f->op;
        e.pos = f->pos;
        e.name = f->name;
        e.file = f->file;
        e.line = f->line;
        e.col = f->col;
        e.calls = f->calls;
        r = fn(opaque, &e);
        if (r)
            return r;
    }
    return 0;
}

/* Code that JS_ReadObject read, before it can run: without the old loop
   (JS_NO_OLD_LOOP), and in the test mode with no fallback (JS_SetRegvm 3),
   a function the pass refuses for code no compiler makes (RV_CRAFTED of
   regvm-pass.h) is a SyntaxError of the read, so that no script stops at
   it at a first call later. The pass runs on each function read and its
   code is dropped: the conversion stays at the first call. A read with
   JS_READ_OBJ_POLLS_CHECKED skips it with the rule of the polls (the same
   bytes were read with the checks before), so a SharedBytecode pays it at
   its first load only. -1 with an exception: the SyntaxError, or out of
   memory. */
static int rv_read_check(JSContext *ctx, JSFunctionBytecode *b)
{
    JSRuntime *rt = ctx->rt;
    int st[5], op, pos;
    BOOL oom;
    RVFunc *rf;

#ifndef JS_NO_OLD_LOOP
    if (rt->regvm_on != 3)
        return 0;
#endif
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
