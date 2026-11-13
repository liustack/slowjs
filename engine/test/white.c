/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* white.c is the engine, quickjs.c itself, with functions added at its end
   that give the static functions and the structures of the engine to
   unit-white.c, which is built with the system's headers and cannot include
   quickjs.c. A white-box test is for what the public API cannot reach: a
   static function that a test feeds a made-up buffer, or a list the collector
   keeps. It is built as the engine is (freestanding, against engine/libc), and
   link instead of quickjs.o: build.sh does that. Everything here is a plain
   function over plain types, with names that begin with white_. */

/* white_poll_skip makes the compiler leave out the polls (insert_polls), so
   that a test sees check_polls stop it. */
static int white_poll_skip_flag;
#define JS_POLL_SKIP() white_poll_skip_flag

#include "../quickjs.h"

/* JS_HEAP_FAILS makes the engine fail the request to the heap number
   white_alloc_fail_at (counting from 1 since white_alloc_count was set to 0),
   and only that one. The requests after it work. A request to the heap is a
   new arena of small blocks or a large block, as a call of the allocator was
   on the ccgo engine.

   With white_alloc_calls set, JS_ALLOC_FAILS counts instead: the calls of
   js_malloc_rt and js_realloc_rt, whatever their size and wherever the block
   comes from (an arena, the front cache, the heap). A call for size 0 never
   counts, as the engine answers it without a block. A call of js_realloc_rt
   that fits the block it has does not count either: the engine answers it
   with the same block and cannot fail it. */
static long white_alloc_count, white_alloc_fail_at;
static int white_alloc_calls;
static int white_alloc_fails(void)
{
    if (white_alloc_calls)
        return 0;
    white_alloc_count++;
    return white_alloc_count == white_alloc_fail_at;
}
#define JS_HEAP_FAILS(s, size) white_alloc_fails()

static int white_call_fails(JSRuntime *rt, void *ptr, size_t size)
{
    if (!white_alloc_calls || size == 0)
        return 0;
    if (ptr && size <= js_malloc_usable_size_rt(rt, ptr))
        return 0;
    white_alloc_count++;
    return white_alloc_count == white_alloc_fail_at;
}
#define JS_ALLOC_FAILS(rt, ptr, size) white_call_fails(rt, ptr, size)

/* RV_SC_CAP gives each buffer of the pass (engine/regvm-pass.h) 16 bytes
   at first while white_sc_small is set (white_regvm_convert_failing mode
   2): every buffer runs short and its round is made again. RV_SC_SHORT
   counts those rounds (white_regvm_shorts). */
static int white_sc_small;
static long white_sc_shorts;
#define RV_SC_CAP(n) (white_sc_small ? 16 : (n))
#define RV_SC_SHORT() (white_sc_shorts++)

/* JS_WORK counts the units of work of the loops that read the interrupt flag
   (white_work), and each reading of the flag (cutils.h calls host_poll) closes
   a stretch: white_work_max is the longest stretch since white_work_reset,
   white_work_total the work in all of them. */
static unsigned long white_work, white_work_max, white_work_total;
#define JS_WORK(n) (white_work += (n), white_work_total += (n))
int host_poll(const volatile int *flag);

/* JS_SHAPE_ID_CHECK makes find_hashed_shape_prop compare the keys of a shape
   it finds by its parent id too, and abort when they differ, and
   JS_SHAPE_ID_HIT counts those shapes. */
static unsigned long white_shape_hits;
#define JS_SHAPE_ID_CHECK 1
#define JS_SHAPE_ID_HIT() (white_shape_hits++)

/* JS_GC_RUN counts the collections that run, full (0) and young (1). */
static unsigned long white_gc_runs[2];
#define JS_GC_RUN(young) (white_gc_runs[young]++)
#define host_poll white_host_poll

#include "../quickjs.c"

#undef host_poll
int white_host_poll(const volatile int *flag)
{
    if (white_work > white_work_max)
        white_work_max = white_work;
    white_work = 0;
    return host_poll(flag);
}

void white_work_reset(void)
{
    white_work = white_work_max = white_work_total = 0;
}

/* the longest stretch of work between two readings of the flag since
   white_work_reset, the one before the first reading and the one after the
   last included */
unsigned long white_work_longest(void)
{
    return white_work > white_work_max ? white_work : white_work_max;
}

unsigned long white_work_sum(void)
{
    return white_work_total;
}

/* LRE_WORK of libregexp.c (white-regexp.c) */
void white_lre_work(unsigned long n)
{
    JS_WORK(n);
}

/* ---- string buffers and the runs of JSON ---- */

/* A string buffer with room for size characters of 8 bits, or of 16 bits when
   wide is set, holding pre characters 'x'. NULL when it could not be made. */
StringBuffer *white_sb_new(JSContext *ctx, int size, int wide, int pre)
{
    StringBuffer *b = js_malloc(ctx, sizeof(*b));
    int i;

    if (!b)
        return NULL;
    if (string_buffer_init2(ctx, b, size, wide)) {
        js_free(ctx, b);
        return NULL;
    }
    for(i = 0; i < pre; i++) {
        if (string_buffer_putc8(b, 'x')) {
            string_buffer_free(b);
            js_free(ctx, b);
            return NULL;
        }
    }
    return b;
}

void white_sb_free(JSContext *ctx, StringBuffer *b)
{
    string_buffer_free(b);
    js_free(ctx, b);
}

/* The code units of b, at most cap of them. Returns how many b holds, or -1
   when it holds more than its room. */
int white_sb_units(const StringBuffer *b, uint16_t *out, int cap)
{
    int i;

    if (b->len > b->size)
        return -1;
    for(i = 0; i < b->len && i < cap; i++)
        out[i] = b->is_wide_char ? b->str->u.str16[i] : b->str->u.str8[i];
    return b->len;
}

int white_sb_error(const StringBuffer *b)
{
    return b->error_status;
}

/* Make b look as long as len characters in room for size, so that a run that
   would make the string longer than the limit is tried without that much
   input. */
void white_sb_fake_length(StringBuffer *b, int len, int size)
{
    b->len = len;
    b->size = size;
}

int white_string_len_max(void)
{
    return JS_STRING_LEN_MAX;
}

const uint8_t *white_json_string_run(StringBuffer *b, const uint8_t *p, const uint8_t *end, int sep)
{
    uint32_t work = 0;

    return json_string_run(b, p, end, sep, &work);
}

/* json_run_limit over addresses that need not be memory: the run is not read. */
uintptr_t white_json_run_limit(uintptr_t p, uintptr_t end)
{
    return (uintptr_t)json_run_limit((const uint8_t *)p, (const uint8_t *)end);
}

/* json_quote_run over the string value s from *pi. */
int white_json_quote_run(StringBuffer *b, JSValueConst s, int *pi)
{
    uint32_t work = 0;

    return json_quote_run(b, JS_VALUE_GET_STRING(s), pi, &work);
}

/* ---- strings built by appending ---- */

/* 1 when v is a rope, 0 when it is a flat string, -1 when it is no string. */
int white_string_is_rope(JSValueConst v)
{
    switch (JS_VALUE_GET_TAG(v)) {
    case JS_TAG_STRING_ROPE:
        return 1;
    case JS_TAG_STRING:
        return 0;
    default:
        return -1;
    }
}

/* The characters the block of the flat string v has room for past its
   length, -1 when v is no flat string. */
int white_string_room(JSContext *ctx, JSValueConst v)
{
    JSString *p;
    size_t size, used;

    if (JS_VALUE_GET_TAG(v) != JS_TAG_STRING)
        return -1;
    p = JS_VALUE_GET_STRING(v);
    size = js_malloc_usable_size(ctx, p);
    used = sizeof(*p) + (p->len << p->is_wide_char) + 1 - p->is_wide_char;
    return size < used ? -1 : (int)((size - used) >> p->is_wide_char);
}

/* ---- pending stacks ---- */

/* Whether the own property 'stack' of obj still holds the pending marker,
   that is, whether nothing has formatted it yet. -1 for no such property. */
int white_stack_pending(JSContext *ctx, JSValueConst obj)
{
    JSObject *p = JS_VALUE_GET_OBJ(obj);
    JSAtom atom = JS_NewAtom(ctx, "stack");
    JSProperty *pr;
    JSShapeProperty *prs = find_own_property(&pr, p, atom);
    int r = prs ? js_backtrace_pending(prs, pr) : -1;

    JS_FreeAtom(ctx, atom);
    return r;
}

/* The class of the marker has no call and no exotic handlers, whatever the
   memory it was built in held before, so that the marker is never taken for
   a function. */
int white_backtrace_class_has_handlers(JSRuntime *rt)
{
    JSClass *cl = &rt->class_array[JS_CLASS_BACKTRACE];

    return cl->call != NULL || cl->exotic != NULL;
}

/* The value of a property is only a JSValue when the property is a plain data
   property: a global variable holds a pointer to its variable reference, with
   whatever bytes were there before after it. With err an error whose stack is
   pending and the global variable 'stack' made by the caller, the checks of a
   pending stack must look at the kind of the property first, and never read
   such bytes as a marker. Returns 0 when they do, else a bit for each check
   that read a variable reference as a marker (1 settle, 2 memory usage, 4 the
   test itself found no such global variable). */
int white_pending_needs_data_property(JSContext *ctx, JSValueConst err)
{
    JSObject *g = JS_VALUE_GET_OBJ(ctx->global_obj);
    JSObject *e = JS_VALUE_GET_OBJ(err);
    JSAtom atom = JS_NewAtom(ctx, "stack");
    JSProperty *pr, fake;
    JSShapeProperty *prs, *eprs;
    JSMemoryUsage s;
    JSMemoryUsage_helper mh;
    int r = 0;

    prs = find_own_property(&pr, g, atom);
    eprs = find_own_property(&pr, e, atom);
    if (!prs || (prs->flags & JS_PROP_TMASK) != JS_PROP_VARREF || !eprs)
        return 4;
    /* a buffer that looks like a marker value, standing in for the bytes of a
       variable reference property */
    fake.u.value = JS_DupValue(ctx, pr->u.value);
    if (js_backtrace_settle(ctx, g, prs, &fake) != 0 || !JS_IsObject(fake.u.value) ||
        JS_VALUE_GET_OBJ(fake.u.value)->class_id != JS_CLASS_BACKTRACE)
        r |= 1;
    memset(&s, 0, sizeof(s));
    memset(&mh, 0, sizeof(mh));
    js_backtrace_memory_usage(&s, &mh, prs, &fake);
    if (s.memory_used_count != 0)
        r |= 2;
    JS_FreeValue(ctx, fake.u.value);
    JS_FreeAtom(ctx, atom);
    return r;
}

/* The bytes one frame of a pending stack takes, which the memory usage
   counts. */
int64_t white_backtrace_frame_size(void)
{
    return sizeof(JSBacktraceFrame);
}

/* ---- the lists of the collector ---- */

/* Check the GC objects of rt outside a collection: each is on gc_obj_list or
   on the young list, with a count above 0 and no mark, which js_weakref_free
   and the collectors rely on. Returns how many there are on each list, and a
   bit in *bad for a count of 0 or less (1) or a mark that is set (2). */
void white_gc_lists(JSRuntime *rt, int *old, int *young, int *bad)
{
    struct list_head *heads[2] = { &rt->gc_obj_list, &rt->young_obj_list };
    int counts[2], k;

    *bad = 0;
    for(k = 0; k < 2; k++) {
        struct list_head *el;

        counts[k] = 0;
        list_for_each(el, heads[k]) {
            JSGCObjectHeader *p = list_entry(el, JSGCObjectHeader, link);

            if (js_rc(p)->ref_count <= 0)
                *bad |= 1;
            if (js_rc(p)->mark)
                *bad |= 2;
            counts[k]++;
        }
    }
    *old = counts[0];
    *young = counts[1];
}

/* ---- the errors being made ---- */

/* Set the count of the errors JS_ThrowError2 is making to n, so that a test
   throws as if it were inside that many, and return the count it had. */
int white_error_nest(JSRuntime *rt, int n)
{
    int old = rt->nest[JS_NEST_ERROR];

    rt->nest[JS_NEST_ERROR] = n;
    return old;
}

/* ---- the properties of a new function ---- */

/* A C function as JS_NewCFunction2 makes it, with proto as its prototype and
   neither length nor name yet, or JS_EXCEPTION. */
JSValue white_bare_function(JSContext *ctx, JSValueConst proto)
{
    JSValue f = JS_NewObjectProtoClass(ctx, proto, JS_CLASS_C_FUNCTION);
    JSObject *p;

    if (JS_IsException(f))
        return f;
    p = JS_VALUE_GET_OBJ(f);
    p->u.cfunc.realm = JS_DupContext(ctx);
    p->u.cfunc.c_function.generic = NULL;
    p->u.cfunc.length = 0;
    p->u.cfunc.cproto = JS_CFUNC_generic;
    p->u.cfunc.magic = 0;
    p->is_constructor = FALSE;
    return f;
}

/* js_function_set_properties: 0, or -1 with the exception pending. */
int white_function_set_properties(JSContext *ctx, JSValueConst f, JSAtom name, int len)
{
    return js_function_set_properties(ctx, f, name, len);
}

/* ---- the polls of the bytecode ---- */

/* The opcodes the tests of the polls write code with, in the order of
   WhiteOp in unit-white.c. */
static const uint8_t white_ops[] = {
    OP_invalid, OP_nop, OP_push_0, OP_drop, OP_goto, OP_goto16, OP_goto8,
    OP_if_false, OP_if_true, OP_if_true8, OP_if_false8, OP_return_undef,
    OP_catch, OP_gosub, OP_ret, OP_with_get_var, OP_push_i32, OP_push_i16,
    OP_object_lit, OP_tail_call, OP_tail_call_method,
    OP_get_field, OP_get_field2, OP_push_atom_value, OP_strict_eq,
    OP_get_arg0_get_field, OP_get_arg1_get_field, OP_get_loc0_get_field,
    OP_get_loc8_get_field, OP_dup_case_atom,
    OP_typeof, OP_typeof_is_undefined, OP_typeof_is_function, OP_typeof_is, OP_lnot,
    OP_get_loc_check, OP_put_loc_check, OP_set_loc_check, OP_set_loc_uninitialized,
    OP_return,
};

int white_op(int i)
{
    return white_ops[i];
}

int white_op_count(void)
{
    return OP_COUNT;
}

int white_poll_run(void)
{
    return POLL_RUN;
}

void white_poll_skip(int on)
{
    white_poll_skip_flag = on;
}

/* NULL when code keeps to the rule of the polls (poll_scan), else what is
   wrong. */
const char *white_poll_scan(const uint8_t *code, int len)
{
    static uint8_t run[1 << 16];

    if (len > (int)sizeof(run))
        return "too long for the test";
    return poll_scan(code, len, run, NULL);
}

/* The code with the OP_nop that insert_polls puts in, in out, which has room
   for cap bytes: its length, or -1. */
int white_poll_insert(JSContext *ctx, const uint8_t *code, int len,
                      uint8_t *out, int cap)
{
    JSFunctionDef fd;
    DynBuf bc;
    int n = -1;

    memset(&fd, 0, sizeof(fd));
    js_dbuf_bytecode_init(ctx, &bc);
    if (!dbuf_put(&bc, code, len) && !insert_polls(ctx, &fd, &bc) &&
        (int)bc.size <= cap) {
        memcpy(out, bc.buf, bc.size);
        n = bc.size;
    }
    dbuf_free(&bc);
    return n;
}

/* The shape of code, in shape (two ints for each instruction but OP_nop):
   its opcode, with a short jump given as its long form, and for a jump the
   index of the first instruction other than OP_nop at its target, else -1.
   The number of instructions, or -1 for code it cannot follow or more than
   max. A jump moved with the code keeps its shape. */
int white_code_shape(const uint8_t *code, int len, int *shape, int max)
{
    static int index[1 << 16];
    int pos, n = 0, op, sz, target;
    const JSOpCode *oi;

    if (len > (int)countof(index))
        return -1;
    for (pos = 0; pos < len; pos += sz) {
        op = code[pos];
        if (op == OP_invalid || op >= OP_COUNT)
            return -1;
        sz = short_opcode_info(op).size;
        index[pos] = n;
        n += op != OP_nop;
    }
    if (n > max)
        return -1;
    n = 0;
    for (pos = 0; pos < len; pos += sz) {
        op = code[pos];
        oi = &short_opcode_info(op);
        sz = oi->size;
        if (op == OP_nop)
            continue;
        switch (oi->fmt) {
        case OP_FMT_label:
            target = pos + 1 + (int32_t)get_u32(code + pos + 1);
            break;
        case OP_FMT_label16:
            target = pos + 1 + (int16_t)get_u16(code + pos + 1);
            break;
        case OP_FMT_label8:
            target = pos + 1 + (int8_t)code[pos + 1];
            break;
        case OP_FMT_atom_label_u8:
            target = pos + 5 + (int32_t)get_u32(code + pos + 5);
            break;
        default:
            target = -1;
            break;
        }
        if (op == OP_goto8 || op == OP_goto16)
            op = OP_goto;
        else if (op == OP_if_false8)
            op = OP_if_false;
        else if (op == OP_if_true8)
            op = OP_if_true;
        shape[2 * n] = op;
        if (target >= 0) {
            while (target < len && code[target] == OP_nop)
                target++;
            if (target >= len)
                return -1;
            target = index[target];
        }
        shape[2 * n + 1] = target;
        n++;
    }
    return n;
}

/* The code of function i of the constant pool of fun, a function compiled
   with JS_EVAL_FLAG_COMPILE_ONLY, and its length in *len. NULL when there
   is no such function. */
const uint8_t *white_child_code(JSValueConst fun, int i, int *len)
{
    JSFunctionBytecode *b, *c;

    if (JS_VALUE_GET_TAG(fun) != JS_TAG_FUNCTION_BYTECODE)
        return NULL;
    b = JS_VALUE_GET_PTR(fun);
    if (i >= b->cpool_count || JS_VALUE_GET_TAG(b->cpool[i]) != JS_TAG_FUNCTION_BYTECODE)
        return NULL;
    c = JS_VALUE_GET_PTR(b->cpool[i]);
    *len = c->byte_code_len;
    return c->byte_code_buf;
}

/* The number of local variables of the function i of the constant pool of
   fun (the slots of its frame past the arguments), or -1. */
int white_child_var_count(JSValueConst fun, int i)
{
    JSFunctionBytecode *b;

    if (JS_VALUE_GET_TAG(fun) != JS_TAG_FUNCTION_BYTECODE)
        return -1;
    b = JS_VALUE_GET_PTR(fun);
    if (i >= b->cpool_count || JS_VALUE_GET_TAG(b->cpool[i]) != JS_TAG_FUNCTION_BYTECODE)
        return -1;
    return ((JSFunctionBytecode *)JS_VALUE_GET_PTR(b->cpool[i]))->var_count;
}

/* The places of the OP_nop of code in places, at most max: how many. */
int white_nops(const uint8_t *code, int len, int *places, int max)
{
    int pos, n = 0, op;

    for (pos = 0; pos < len; pos += short_opcode_info(op).size) {
        op = code[pos];
        if (op == OP_nop && n < max)
            places[n++] = pos;
    }
    return n;
}

/* ---- variables the compiler adds when code uses them ---- */

/* A script, a function f in it and an arrow function in f, as the parser
   leaves them before the variables are resolved. f is a named function
   expression with this, arguments, new.target, a home object and the active
   function of a derived class constructor, so the compiler adds the variable
   of each the first time code uses it. Its variable array is full and holds
   more than a thousand variables with no name, so that adding one more takes
   a large block, which a limit can refuse while small blocks still come. */
typedef struct {
    GetLineColCache cache;
    JSFunctionDef *script, *f, *arrow;
} WhiteFuncs;

static int white_funcs_new(JSContext *ctx, WhiteFuncs *w)
{
    static const uint8_t src[] = "";
    JSFunctionDef *f;

    memset(w, 0, sizeof(*w));
    w->cache.buf_start = src;
    w->cache.ptr = src;
    w->script = js_new_function_def(ctx, NULL, TRUE, FALSE, "<white>", src, &w->cache);
    if (!w->script)
        return -1;
    w->script->has_this_binding = TRUE;
    f = js_new_function_def(ctx, w->script, FALSE, TRUE, "<white>", src, &w->cache);
    if (!f)
        return -1;
    w->f = f;
    f->func_name = JS_NewAtom(ctx, "white_f");
    if (f->func_name == JS_ATOM_NULL)
        return -1;
    f->has_this_binding = TRUE;
    f->has_arguments_binding = TRUE;
    f->has_home_object = TRUE;
    f->is_derived_class_constructor = TRUE;
    w->arrow = js_new_function_def(ctx, f, FALSE, FALSE, "<white>", src, &w->cache);
    if (!w->arrow)
        return -1;
    while (f->var_count < 1024 || f->var_count < f->var_size) {
        if (add_var(ctx, f, JS_ATOM_NULL) < 0)
            return -1;
    }
    return 0;
}

static void white_funcs_free(JSContext *ctx, WhiteFuncs *w)
{
    if (w->script)
        js_free_function_def(ctx, w->script);
}

/* Lets the runtime allocate room bytes more than it holds, or lifts the
   limit when room is (size_t)-1. */
static void white_room(JSRuntime *rt, size_t room)
{
    if (room == (size_t)-1) {
        JS_SetMemoryLimit(rt, room);
    } else {
        JS_RunGC(rt);
        JS_SetMemoryLimit(rt, JS_GetMemorySize(rt) + room);
    }
}

/* The names white_resolve_var resolves, by kind: this, new.target, the home
   object, the active function, arguments and the name of f. */
#define WHITE_VAR_KINDS 6

/* The index of the variable of kind that f has, or -1. */
static int white_var_index(JSFunctionDef *f, int kind)
{
    switch (kind) {
    case 0: return f->this_var_idx;
    case 1: return f->new_target_var_idx;
    case 2: return f->home_object_var_idx;
    case 3: return f->this_active_func_var_idx;
    case 4: return f->arguments_var_idx;
    default: return f->func_var_idx;
    }
}

/* Resolve the name of kind as code of f reads it (in_arrow 0) or as code of
   the arrow function in f reads it (in_arrow 1), with room bytes of memory
   left ((size_t)-1 for no limit). Returns what resolve_scope_var returned (the
   position of the next opcode, or -1 for an error), and sets *pending when an
   exception is pending after it and *made when f has the variable then. The
   limit is lifted before it returns. -2 when the functions could not be
   made. */
int white_resolve_var(JSContext *ctx, int kind, int in_arrow, size_t room,
                      int *pending, int *made)
{
    static const JSAtom names[WHITE_VAR_KINDS - 1] = {
        JS_ATOM_this, JS_ATOM_new_target, JS_ATOM_home_object,
        JS_ATOM_this_active_func, JS_ATOM_arguments,
    };
    WhiteFuncs w;
    DynBuf bc;
    JSAtom name;
    int r;

    if (white_funcs_new(ctx, &w) < 0) {
        white_funcs_free(ctx, &w);
        return -2;
    }
    name = kind < WHITE_VAR_KINDS - 1 ? names[kind] : w.f->func_name;
    js_dbuf_init(ctx, &bc);
    white_room(ctx->rt, room);
    r = resolve_scope_var(ctx, in_arrow ? w.arrow : w.f, name, 0,
                          OP_scope_get_var, &bc, NULL, NULL, 0);
    white_room(ctx->rt, (size_t)-1);
    *pending = JS_HasException(ctx);
    *made = white_var_index(w.f, kind) >= 0;
    dbuf_free(&bc);
    white_funcs_free(ctx, &w);
    return r;
}

/* Add the variables a direct eval in f (in_arrow 0) or in the arrow function
   in f (in_arrow 1) can use, with room bytes of memory left, as
   white_resolve_var does. Returns what add_eval_variables returned, and sets
   *made when f has every variable white_resolve_var resolves then. */
int white_eval_vars(JSContext *ctx, int in_arrow, size_t room, int *pending,
                    int *made)
{
    WhiteFuncs w;
    int r, kind;

    if (white_funcs_new(ctx, &w) < 0) {
        white_funcs_free(ctx, &w);
        return -2;
    }
    white_room(ctx->rt, room);
    r = add_eval_variables(ctx, in_arrow ? w.arrow : w.f);
    white_room(ctx->rt, (size_t)-1);
    *pending = JS_HasException(ctx);
    *made = 1;
    for (kind = 0; kind < WHITE_VAR_KINDS; kind++)
        *made &= white_var_index(w.f, kind) >= 0;
    if (!in_arrow)
        *made &= w.f->var_object_idx >= 0;
    white_funcs_free(ctx, &w);
    return r;
}

/* ---- the own keys of an object ---- */

/* One key as "NAME(KIND)/ENUMERABLE" appended to out, which has room for cap
   bytes with n used; KIND is what JS_AtomGetKind says (0 string, 1 symbol, 2
   private name). Returns the new length. */
static size_t white_key_text(JSContext *ctx, char *out, size_t cap, size_t n,
                             JSAtom atom, int enumerable)
{
    const char *name = JS_AtomToCString(ctx, atom);

    n += snprintf(out + n, cap - n, "%s%s(%d)/%d", n ? "\n" : "", name ? name : "?",
                  (int)JS_AtomGetKind(ctx, atom), enumerable);
    JS_FreeCString(ctx, name);
    return n < cap ? n : cap - 1;
}

/* The own keys of the object obj listed by JS_GetOwnPropertyNames with flags
   (got set), or as JS_GetOwnPropertyNamesInternal lists them from the shape
   of an object that is not exotic (got clear): array indexes by value, then
   strings, then symbols and private names, each in the order of the shape,
   the kinds told by JS_AtomGetKind and JS_AtomIsArrayIndex. Written into out
   as the lines of white_key_text. Returns the length, or -1 when the listing
   threw, or -2 when the object is exotic and has no listing from its shape. */
int white_own_keys(JSContext *ctx, JSValueConst obj, int flags, int got,
                   char *out, size_t cap)
{
    JSObject *p = JS_VALUE_GET_OBJ(obj);
    size_t n = 0;

    out[0] = 0;
    if (got) {
        JSPropertyEnum *tab;
        uint32_t len, i;

        if (JS_GetOwnPropertyNames(ctx, &tab, &len, obj, flags))
            return -1;
        for (i = 0; i < len; i++)
            n = white_key_text(ctx, out, cap, n, tab[i].atom, tab[i].is_enumerable != 0);
        JS_FreePropertyEnum(ctx, tab, len);
        return n;
    } else {
        JSShape *sh = p->shape;
        JSShapeProperty *prs = get_shape_prop(sh);
        uint32_t nums[256];
        int order[3][256], count[3] = { 0, 0, 0 }, i, j, k, c;

        if (p->is_exotic)
            return -2;
        for (i = 0; i < sh->prop_count; i++) {
            JSAtom atom = prs[i].atom;
            int enumerable = (prs[i].flags & JS_PROP_ENUMERABLE) != 0;
            uint32_t v;

            if (atom == JS_ATOM_NULL || ((flags & JS_GPN_ENUM_ONLY) && !enumerable))
                continue;
            if (!((flags >> JS_AtomGetKind(ctx, atom)) & 1))
                continue;
            if (JS_AtomIsArrayIndex(ctx, &v, atom)) {
                nums[count[0]] = v;
                order[0][count[0]++] = i;
            } else if (JS_AtomGetKind(ctx, atom) == JS_ATOM_KIND_STRING) {
                order[1][count[1]++] = i;
            } else {
                order[2][count[2]++] = i;
            }
        }
        /* the indexes by value, equal values in the order of the shape */
        for (j = 1; j < count[0]; j++) {
            uint32_t v = nums[j];
            int o = order[0][j];

            for (k = j - 1; k >= 0 && nums[k] > v; k--) {
                nums[k + 1] = nums[k];
                order[0][k + 1] = order[0][k];
            }
            nums[k + 1] = v;
            order[0][k + 1] = o;
        }
        for (c = 0; c < 3; c++) {
            for (j = 0; j < count[c]; j++)
                n = white_key_text(ctx, out, cap, n, prs[order[c][j]].atom,
                                   (prs[order[c][j]].flags & JS_PROP_ENUMERABLE) != 0);
        }
        return n;
    }
}

/* ---- shared source text and line tables ---- */

/* The source text and the line table of function i of the constant pool of
   the script script (read with JS_ReadObject), and whether the function says
   they are shared. 0, or -1 when there is no such function. */
int white_function_debug(JSValueConst script, int i, const uint8_t **source,
                         size_t *source_len, const uint8_t **pc2line,
                         int *shared)
{
    JSFunctionBytecode *b, *f;

    if (JS_VALUE_GET_TAG(script) != JS_TAG_FUNCTION_BYTECODE)
        return -1;
    b = JS_VALUE_GET_PTR(script);
    if (i >= b->cpool_count || JS_VALUE_GET_TAG(b->cpool[i]) != JS_TAG_FUNCTION_BYTECODE)
        return -1;
    f = JS_VALUE_GET_PTR(b->cpool[i]);
    *source = (const uint8_t *)f->debug.source;
    *source_len = f->debug.source_len;
    *pc2line = f->debug.pc2line_buf;
    *shared = f->shared_debug;
    return b->cpool_count;
}

/* ---- the regexp cache ---- */

/* The characters of the string p below 128 appended to out (room cap, n used),
   others as '?'. Returns the new length. */
static size_t white_text_add(const JSString *p, char *out, size_t cap, size_t n)
{
    int i;

    for (i = 0; i < p->len && n + 1 < cap; i++) {
        uint32_t c = p->is_wide_char ? p->u.str16[i] : p->u.str8[i];

        out[n++] = c < 128 ? (char)c : '?';
    }
    out[n] = 0;
    return n;
}

/* The entries of the regexp cache of rt, newest first, written into out as
   "PATTERN FLAGS" separated by a comma ("undefined" for flags that are
   undefined), after checking the lists the way the cache keeps them: each
   entry's newer is the one before it, it is in its hash chain, the oldest is
   the last, and the count and the bytes are the sums. Returns how many
   entries there are, or -1 when a check fails. *bytes is the bytes the cache
   counts and *buckets the chains of its table. */
int white_regexp_cache(JSRuntime *rt, char *out, size_t cap, size_t *bytes,
                       uint32_t *buckets)
{
    JSRegExpCache *rc = &rt->regexp_cache;
    JSRegExpCacheEntry *e, *prev = NULL, *c;
    size_t sum = 0, n = 0;
    int count = 0, found;

    out[0] = 0;
    for (e = rc->newest; e; e = e->older) {
        if (e->newer != prev)
            return -1;
        prev = e;
        found = 0;
        for (c = rc->table[e->hash & (rc->buckets - 1)]; c; c = c->chain)
            found |= c == e;
        if (!found)
            return -1;
        if (count && n + 1 < cap) {
            out[n++] = ',';
            out[n] = 0;
        }
        n = white_text_add(e->pattern, out, cap, n);
        if (n + 1 < cap)
            out[n++] = ' ';
        if (e->flags) {
            n = white_text_add(e->flags, out, cap, n);
        } else {
            memcpy(out + n, "undefined", 10);
            n += 9;
        }
        sum += e->size;
        count++;
    }
    if (rc->oldest != prev || count != rc->count || sum != rc->bytes)
        return -1;
    *bytes = rc->bytes;
    *buckets = rc->buckets;
    return count;
}

/* ---- one request to the heap that fails ---- */

/* Compile js (not run) in ctx with request number fail failing, 0 for none, and
   the others working. A request is a request to the heap, or with bit 0 of
   mode set a call of js_malloc_rt or js_realloc_rt (see JS_ALLOC_FAILS
   above). js is a module when bit 1 of mode is set. *nallocs is how many
   requests the compile made. Returns what JS_Eval returns. */
JSValue white_compile_failing(JSContext *ctx, const char *js, int fail, int mode, int *nallocs)
{
    JSValue v;

    white_alloc_calls = mode & 1;
    white_alloc_count = 0;
    white_alloc_fail_at = fail;
    v = JS_Eval(ctx, js, strlen(js), "<unit>", ((mode & 2) ? JS_EVAL_TYPE_MODULE : JS_EVAL_TYPE_GLOBAL) | JS_EVAL_FLAG_COMPILE_ONLY);
    *nallocs = white_alloc_count;
    white_alloc_fail_at = 0;
    white_alloc_calls = 0;
    return v;
}

/* Read buf as JS_ReadObject does for bytecode, without
   JS_READ_OBJ_POLLS_CHECKED, with call number fail of js_malloc_rt and
   js_realloc_rt failing (0 for none), the others working. *nallocs is how
   many calls it made. Returns what JS_ReadObject returns. */
JSValue white_read_failing(JSContext *ctx, const uint8_t *buf, size_t len, int fail, int *nallocs)
{
    JSValue v;

    white_alloc_calls = 1;
    white_alloc_count = 0;
    white_alloc_fail_at = fail;
    v = JS_ReadObject(ctx, buf, len, JS_READ_OBJ_BYTECODE);
    *nallocs = white_alloc_count;
    white_alloc_fail_at = 0;
    white_alloc_calls = 0;
    return v;
}

/* Whether every block of the engine is a request to the heap of its own (the
   builds that take each block from the system malloc), and not only the
   arenas of small blocks and the large blocks: such a block has no room past
   what was asked for. */
int white_every_block_is_a_request(void)
{
    return JS_MALLOC_LARGE_BLOCKS_ONLY;
}

/* ---- the block of an object ---- */

/* The bytes the block of the object obj has room for, and sizeof(JSObject)
   in *full, or 0 when obj is no object. */
size_t white_object_block(JSContext *ctx, JSValueConst obj, size_t *full)
{
    if (JS_VALUE_GET_TAG(obj) != JS_TAG_OBJECT)
        return 0;
    *full = sizeof(JSObject);
    return js_malloc_usable_size(ctx, JS_VALUE_GET_OBJ(obj));
}

/* ---- the properties of an object ---- */

/* Whether the properties of obj sit in its block (1) or in a block of their
   own (0), or -1 when obj is no object. *room is the number of properties
   the shape of obj has room for. */
int white_props_inline(JSValueConst obj, int *room)
{
    JSObject *p;

    if (JS_VALUE_GET_TAG(obj) != JS_TAG_OBJECT)
        return -1;
    p = JS_VALUE_GET_OBJ(obj);
    *room = p->shape->prop_size;
    return p->prop == (JSProperty *)((uint8_t *)p + offsetof(JSObject, u));
}

/* Call func with this and the arguments, with call number fail of
   js_malloc_rt and js_realloc_rt failing (0 for none), the others working.
   *nallocs is how many calls it made. Returns what JS_Call returns. */
JSValue white_call_failing(JSContext *ctx, JSValueConst func, JSValueConst this_val,
                           int argc, JSValueConst *argv, int fail, int *nallocs)
{
    JSValue v;

    white_alloc_calls = 1;
    white_alloc_count = 0;
    white_alloc_fail_at = fail;
    v = JS_Call(ctx, func, this_val, argc, argv);
    *nallocs = white_alloc_count;
    white_alloc_fail_at = 0;
    white_alloc_calls = 0;
    return v;
}

/* The shape of obj as a number, 0 when obj is no object: two objects with
   the same number share a shape. */
uintptr_t white_shape_of(JSValueConst obj)
{
    if (JS_VALUE_GET_TAG(obj) != JS_TAG_OBJECT)
        return 0;
    return (uintptr_t)JS_VALUE_GET_OBJ(obj)->shape;
}

/* ---- the pairs of instructions that one opcode does ---- */

/* The number of instructions of code that are the fused opcode of each run
   of fused_runs, followed by the rest of that run, in the order of
   fused_runs, into count (cap entries, as many as fused_runs has, or it
   aborts), and in all. */
int white_fused_ops(const uint8_t *code, int len, int *count, size_t cap)
{
    int pos, op, n = 0;
    size_t i;

    if (cap != countof(fused_runs))
        abort(); /* the caller counts a different number of runs */
    for (i = 0; i < countof(fused_runs); i++)
        count[i] = 0;
    for (pos = 0; pos < len; pos += short_opcode_info(op).size) {
        op = code[pos];
        for (i = 0; i < countof(fused_runs); i++) {
            if (fused_runs[i][4] == op &&
                fused_run_at(code, len, pos, fused_runs[i], fused_runs[i][0])) {
                count[i]++;
                n++;
            }
        }
    }
    return n;
}

/* The names of the opcodes, by their number in the bytecode. */
static const char *const white_opcode_names[OP_COUNT] = {
#define FMT(f)
#define DEF(id, size, n_pop, n_push, f) #id,
#define def(id, size, n_pop, n_push, f)
#include "quickjs-opcode.h"
#undef def
#undef DEF
#undef FMT
};

/* Whether the text at p starts with s. */
static int white_starts(const char *p, const char *s)
{
    while (*s && *p == *s) {
        p++;
        s++;
    }
    return !*s;
}

/* Check short_opcode_index, the index the disassembler prints for an opcode
   of the format none_loc, none_arg or none_var_ref, against its name: the
   digit after the first "loc", "arg" or "var_ref" in it. The count of opcodes
   checked, or -1 with the name of the first one that is wrong in bad. */
int white_short_index_check(char *bad, size_t cap)
{
    int op, fmt, want, n = 0;
    const char *p;
    size_t k;

    for (op = 0; op < OP_COUNT; op++) {
        fmt = short_opcode_info(op).fmt;
        if (fmt != OP_FMT_none_loc && fmt != OP_FMT_none_arg && fmt != OP_FMT_none_var_ref)
            continue;
        want = -1;
        for (p = white_opcode_names[op]; *p && want < 0; p++) {
            k = white_starts(p, "var_ref") ? 7 : white_starts(p, "loc") || white_starts(p, "arg") ? 3 : 0;
            if (k && p[k] >= '0' && p[k] <= '9')
                want = p[k] - '0';
        }
        if (want < 0 || short_opcode_index(op) != want) {
            snprintf(bad, cap, "%s gives %d", white_opcode_names[op], short_opcode_index(op));
            return -1;
        }
        n++;
    }
    return n;
}

/* ---- the copy of a string buffer that grows ---- */

/* The bytes of the header of a string, which the block of a string buffer
   holds in front of its characters. */
int white_jsstring_header(void)
{
    return sizeof(JSString);
}

/* string_buffer_write8 of len bytes of p, and string_buffer_putc16 of c. */
int white_sb_write8(StringBuffer *b, const uint8_t *p, int len)
{
    return string_buffer_write8(b, p, len);
}

int white_sb_putc16(StringBuffer *b, uint32_t c)
{
    return string_buffer_putc16(b, c);
}

/* JS_WORK(n) for a test that stands for the loop that came before a call. */
void white_work_add(unsigned long n)
{
    JS_WORK(n);
}

/* The size the allocator gives the block of b, which the limit of the memory
   counts when the block is made larger. */
size_t white_sb_block_usable(const StringBuffer *b)
{
    return js_malloc_usable_size(b->ctx, b->str);
}

/* Give the front cache and the cache of large blocks back to the heap, so
   that the count of the heap is what the blocks in use make. */
void white_front_drain(JSContext *ctx)
{
    js_front_drain(&ctx->rt->malloc_ctx);
}

/* The limit of the memory that the heap checks. */
size_t white_heap_limit(JSContext *ctx)
{
    return ctx->rt->malloc_ctx.heap.limit;
}

/* The count the threshold of a collection is held to (js_trigger_gc). */
size_t white_gc_count(JSRuntime *rt)
{
    return rt->malloc_ctx.heap.count - rt->rv_stack_bytes;
}

/* ---- the keys and plain strings of JSON.parse ---- */

const uint8_t *white_json_plain_run(const uint8_t *p, const uint8_t *end)
{
    return json_plain_run(p, end);
}

int white_json_count_high_bytes(const uint8_t *p, int n)
{
    return json_count_high_bytes(p, n);
}

/* The references the atom of the ASCII string s has, not counting one the
   call takes and gives back, or -1 when s has no atom or a constant one. */
int white_atom_refs(JSContext *ctx, const char *s)
{
    JSAtom a = __JS_FindAtom(ctx->rt, s, strlen(s), JS_ATOM_TYPE_STRING);
    int n;

    if (a == JS_ATOM_NULL || __JS_AtomIsConst(a))
        return -1;
    n = js_rc(ctx->rt->atom_array[a])->ref_count - 1;
    JS_FreeAtom(ctx, a);
    return n;
}

/* ---- collections that run (JS_RunGC, JS_RunYoungGC) ---- */

/* The full (young 0) or young (young 1) collections that ran, past the check
   of the phase. */
unsigned long white_gc_run_count(int young)
{
    return white_gc_runs[young != 0];
}

/* ---- the ids of shapes (find_hashed_shape_prop) ---- */

/* The shapes find_hashed_shape_prop has found by their parent id. */
unsigned long white_shape_id_hits(void)
{
    return white_shape_hits;
}

/* The id the next shape of rt gets, and the largest id. */
uint32_t white_shape_id_next(JSRuntime *rt)
{
    return rt->shape_id_next;
}

uint32_t white_shape_id_max(void)
{
    return JS_SHAPE_ID_MAX;
}

void white_shape_id_set(JSRuntime *rt, uint32_t next)
{
    rt->shape_id_next = next;
}

/* Whether sh and sh1 have the same prototype, keys and flags. */
static int white_same_shape(JSShape *sh, JSShape *sh1)
{
    JSShapeProperty *pr = get_shape_prop(sh), *pr1 = get_shape_prop(sh1);
    int k;

    if (sh->proto != sh1->proto || sh->prop_count != sh1->prop_count)
        return 0;
    for(k = 0; k < sh->prop_count; k++) {
        if (pr[k].atom != pr1[k].atom || pr[k].flags != pr1[k].flags)
            return 0;
    }
    return 1;
}

/* The shapes of the shape hash table of rt that break the rule of the ids:
   one with id 0, or one whose parent id is the id of another shape of the
   table that it does not start with (another prototype, not one key less,
   or other keys or flags before its last), and two shapes of one id with
   another prototype, other keys or other flags. Two shapes of one id with
   the same of all are right: a copy add_property put in the table keeps the
   id of the shape it copies until it gets its key, and stays so when it
   cannot. The number of shapes in the table goes to *count, and the number
   with a parent id to *parented. */
int white_shape_ids_bad(JSRuntime *rt, int *count, int *parented)
{
    JSShape *sh, *sh1;
    JSShapeProperty *pr, *pr1;
    uint32_t i, j;
    int k, bad = 0;

    *count = *parented = 0;
    for(i = 0; i < rt->shape_hash_size; i++) {
        for(sh1 = rt->shape_hash[i]; sh1; sh1 = sh1->shape_hash_next) {
            (*count)++;
            if (!sh1->is_hashed || sh1->id == 0 || sh1->id >= rt->shape_id_next)
                bad++;
            if (sh1->parent_id != 0)
                (*parented)++;
            for(j = 0; j < rt->shape_hash_size; j++) {
                for(sh = rt->shape_hash[j]; sh; sh = sh->shape_hash_next) {
                    if (sh != sh1 && sh->id == sh1->id && !white_same_shape(sh, sh1))
                        bad++;
                    if (sh1->parent_id == 0 || sh == sh1 || sh->id != sh1->parent_id)
                        continue;
                    if (sh->proto != sh1->proto || sh->prop_count + 1 != sh1->prop_count) {
                        bad++;
                        continue;
                    }
                    pr = get_shape_prop(sh);
                    pr1 = get_shape_prop(sh1);
                    for(k = 0; k < sh->prop_count; k++) {
                        if (pr[k].atom != pr1[k].atom || pr[k].flags != pr1[k].flags) {
                            bad++;
                            break;
                        }
                    }
                }
            }
        }
    }
    return bad;
}

/* The pairs of shapes of the shape hash table of rt that have one id. */
int white_shape_id_twins(JSRuntime *rt)
{
    JSShape *sh, *sh1;
    uint32_t i, j;
    int n = 0;

    for(i = 0; i < rt->shape_hash_size; i++) {
        for(sh1 = rt->shape_hash[i]; sh1; sh1 = sh1->shape_hash_next) {
            for(j = 0; j < rt->shape_hash_size; j++) {
                for(sh = rt->shape_hash[j]; sh; sh = sh->shape_hash_next) {
                    if (sh != sh1 && sh->id == sh1->id)
                        n++;
                }
            }
        }
    }
    /* each pair was counted from both of its shapes */
    return n / 2;
}

/* Whether the shape of obj is in the shape hash table. */
int white_shape_hashed(JSValueConst obj)
{
    return JS_VALUE_GET_OBJ(obj)->shape->is_hashed;
}

/* ---- the atom a string is as a key ---- */

/* The key_atom of the string v (a JS_TAG_STRING value). */
uint32_t white_key_atom(JSValueConst v)
{
    return JS_VALUE_GET_STRING(v)->key_atom;
}

/* The atoms of rt whose string has a key_atom other than the one
   JS_ValueToAtom's rule gives (the atom's own index for an atom of the type
   string, not empty, not starting with a digit, else JS_ATOM_NULL), with the
   live atoms in *live and those that are their own key in *keys. */
int white_key_atoms_bad(JSRuntime *rt, int *live, int *keys)
{
    uint32_t i, want;
    JSAtomStruct *p;
    int bad = 0;

    *live = *keys = 0;
    for(i = 1; i < rt->atom_size; i++) {
        p = rt->atom_array[i];
        if (atom_is_free(p))
            continue;
        (*live)++;
        want = p->atom_type == JS_ATOM_TYPE_STRING && p->len != 0 &&
            !is_digit(string_get(p, 0)) ? i : JS_ATOM_NULL;
        if (want)
            (*keys)++;
        if (p->key_atom != want)
            bad++;
    }
    return bad;
}

/* ---- the cache of the start and end tags ---- */

/* The sizes of the tables of names, of style names and of end tags of ctx
   (0 for one not made yet) in size[3] and the entries in use in count[3].
   Returns -1 when ctx has no cache, -2 when an entry in use is not where a
   lookup from its hash finds it, or a count is not the entries in use. */
int white_emit_tables(JSContext *ctx, int size[3], int count[3])
{
    JSEmitCache *ec = ctx->emit_cache;
    JSEmitNames *t;
    uint32_t i, h, n, mask, used;
    int k;

    if (!ec)
        return -1;
    for (k = 0; k < 2; k++) {
        t = k ? &ec->style : &ec->attr;
        size[k] = js_emit_names_made(t) ? 1 << t->bits : 0;
        count[k] = t->count;
        if (!js_emit_names_made(t))
            continue;
        n = (uint32_t)1 << t->bits;
        mask = n - 1;
        used = 0;
        for (i = 0; i < n; i++) {
            if (t->tab[i].atom == JS_ATOM_NULL)
                continue;
            used++;
            for (h = js_emit_hash(t->tab[i].atom, t->bits); h != i; h = (h + 1) & mask) {
                if (t->tab[h].atom == JS_ATOM_NULL || t->tab[h].atom == t->tab[i].atom)
                    return -2;
            }
        }
        if (used != t->count)
            return -2;
    }
    size[2] = ec->end != js_emit_no_ends ? 1 << ec->end_bits : 0;
    count[2] = ec->end_count;
    if (ec->end != js_emit_no_ends) {
        n = (uint32_t)1 << ec->end_bits;
        mask = n - 1;
        used = 0;
        for (i = 0; i < n; i++) {
            JSString *p;

            if (JS_VALUE_GET_TAG(ec->end[i].tag) != JS_TAG_STRING)
                continue;
            used++;
            p = JS_VALUE_GET_STRING(ec->end[i].tag);
            for (h = js_emit_end_hash(p, ec->end_bits); h != i; h = (h + 1) & mask) {
                if (JS_VALUE_GET_TAG(ec->end[h].tag) != JS_TAG_STRING ||
                    JS_VALUE_GET_STRING(ec->end[h].tag) == p)
                    return -2;
            }
        }
        if (used != ec->end_count)
            return -2;
    }
    return 0;
}

/* How many size classes the block of the string v is above the class of its
   length: 0 when the string has the block its text needs. -1 when v is not
   a string, -3 when its block is not of an arena (a large block, or a build
   where every block is one, which has no classes). */
int white_string_slack(JSValueConst v)
{
    JSString *p;
    JSMallocBlockHeader *b;
    size_t need;

    if (JS_VALUE_GET_TAG(v) != JS_TAG_STRING)
        return -1;
    p = JS_VALUE_GET_STRING(v);
    b = js_rc(p);
    if (JS_MALLOC_LARGE_BLOCKS_ONLY || b->u.block_idx == FREE_NIL)
        return -3;
    need = sizeof(JSString) + (p->len << p->is_wide_char) + 1 - p->is_wide_char;
    need = ((need + JS_MALLOC_ALIGN - 1) & ~(size_t)(JS_MALLOC_ALIGN - 1)) +
        sizeof(JSMallocBlockHeader);
    return b->block_size_idx - get_block_size_index(need);
}

/* The most size classes a name, style name or end tag kept by the cache of ctx
   is above the class of its length (see white_string_slack), 0 when each has
   the block its text needs. -1 when ctx has no cache. */
int white_emit_chunks_slack(JSContext *ctx)
{
    JSEmitCache *ec = ctx->emit_cache;
    JSEmitNames *t;
    uint32_t i;
    int k, s, most = 0;

    if (!ec)
        return -1;
    for (k = 0; k < 2; k++) {
        t = k ? &ec->style : &ec->attr;
        if (!js_emit_names_made(t))
            continue;
        for (i = 0; i < (uint32_t)1 << t->bits; i++) {
            if (t->tab[i].atom == JS_ATOM_NULL)
                continue;
            s = white_string_slack(t->tab[i].chunk);
            if (s > most)
                most = s;
        }
    }
    if (ec->end != js_emit_no_ends) {
        for (i = 0; i < (uint32_t)1 << ec->end_bits; i++) {
            s = white_string_slack(ec->end[i].chunk);
            if (s > most)
                most = s;
        }
    }
    return most;
}

/* The buffer the start tag of ctx writes in. op 0 gives its size in units, 0
   when the cache holds none. op 1 takes it as a call does at its start (a
   call made then makes a buffer of its own), and op 2 gives it back as that
   call does at its end. -1 when ctx has no cache. */
static StringBuffer white_emit_held;
int white_emit_buf(JSContext *ctx, int op)
{
    JSEmitCache *ec = ctx->emit_cache;

    if (!ec)
        return -1;
    if (op == 1) {
        js_emit_buf_take(ctx, ec, &white_emit_held);
        return white_emit_held.size;
    }
    if (op == 2)
        js_emit_buf_end(ctx, ec, &white_emit_held, FALSE);
    return ec->buf ? ec->buf_size : 0;
}

/* ---- the register loop ---- */

/* converts the function f to register code (rv_convert) with the request
   fail of the conversion failing, as white_compile_failing does: mode 1
   counts every call of the allocator, mode 2 starts each buffer of the
   pass at 16 bytes (RV_SC_CAP). A collection first gives the scratch of
   the pass back, so that the conversion makes it. *nallocs gets the
   requests the conversion made. 1 when f has register code, 0 when it has
   none, -1 when f is no function of bytecode (a generator or an async
   function is one) that has not converted yet. */
int white_regvm_convert_failing(JSContext *ctx, JSValueConst f, int fail, int mode, int *nallocs)
{
    JSFunctionBytecode *b;
    RVFunc *rf;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT ||
        !js_class_has_bytecode(JS_VALUE_GET_OBJ(f)->class_id))
        return -1;
    b = JS_VALUE_GET_OBJ(f)->u.func.function_bytecode;
    if (b->rv != RV_UNCONVERTED)
        return -1;
    JS_RunGC(ctx->rt);
    white_sc_small = (mode & 2) != 0;
    white_sc_shorts = 0;
    white_alloc_calls = mode & 1;
    white_alloc_count = 0;
    white_alloc_fail_at = fail;
    rf = rv_convert(ctx, b);
    *nallocs = white_alloc_count;
    /* a conversion of its own, not that of a call */
    ctx->rt->rv_oom_b = NULL;
    white_alloc_fail_at = 0;
    white_alloc_calls = 0;
    white_sc_small = 0;
    return rf->arg_count >= 0;
}

/* the rounds of the pass made again with a larger scratch in the last
   conversion of white_regvm_convert_failing */
long white_regvm_shorts(void)
{
    return white_sc_shorts;
}

/* an FNV-1a hash of the register code of f: its code, pcmap and the temps
   past T[31] of it, the return addresses of its GOSUBs, the slots of its
   frame, its first temp and the tags of its constants, and in *code_len
   the bytes of its code. 0 when f has none. */
uint32_t white_regvm_code_hash(JSValueConst f, int *code_len)
{
    JSFunctionBytecode *b;
    const RVFunc *rf;
    uint32_t h = 2166136261u;
    int i;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT ||
        !js_class_has_bytecode(JS_VALUE_GET_OBJ(f)->class_id))
        return 0;
    b = JS_VALUE_GET_OBJ(f)->u.func.function_bytecode;
    rf = b->rv;
    if (rf == RV_UNCONVERTED || rf == RV_REFUSED || rf->arg_count < 0)
        return 0;
    *code_len = rf->code_len;
#define WHITE_FNV(p, n) do { const uint8_t *p_ = (const uint8_t *)(p); size_t n_;    \
        for (n_ = 0; n_ < (size_t)(n); n_++) h = (h ^ p_[n_]) * 16777619u; } while (0)
    WHITE_FNV(rf->code, rf->code_len);
    WHITE_FNV(rf->pcmap, sizeof(RVPcMask) * rf->pcmap_count);
    if (rf->pcmap_hi)
        WHITE_FNV(rf->pcmap_hi, sizeof(RVPcHi) * rf->pcmap_count);
    WHITE_FNV(rf->rets, 16 * rf->ret_count);
    WHITE_FNV(&rf->fsize, sizeof(rf->fsize));
    WHITE_FNV(&rf->temp0, sizeof(rf->temp0));
    WHITE_FNV(&rf->k_count, sizeof(rf->k_count));
    for (i = 0; i < rf->k_count; i++) {
        int tag = JS_VALUE_GET_TAG(rf->k[i]);
        WHITE_FNV(&tag, sizeof(tag));
    }
#undef WHITE_FNV
    return h;
}

/* the register code of f, its length in *code_len, or NULL when f has
   none */
const uint8_t *white_regvm_code(JSValueConst f, int *code_len)
{
    const RVFunc *rf;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT ||
        !js_class_has_bytecode(JS_VALUE_GET_OBJ(f)->class_id))
        return NULL;
    rf = JS_VALUE_GET_OBJ(f)->u.func.function_bytecode->rv;
    if (rf == RV_UNCONVERTED || rf == RV_REFUSED || rf->arg_count < 0)
        return NULL;
    *code_len = rf->code_len;
    return rf->code;
}

/* the for_of_start of the function f, converted: how many the pass made
   DSTART (*dst) and how many W_FOR_OF_START (*win), found by their pcmap
   entries, which name the old for_of_start and end after the two bytes of
   the instruction (engine/regvm-ops-dst.h). -1 when f has no register code. */
int white_regvm_dst_sites(JSValueConst f, int *dst, int *win)
{
    JSFunctionBytecode *b;
    const RVFunc *rf;
    int i;

    *dst = *win = 0;
    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT ||
        !js_class_has_bytecode(JS_VALUE_GET_OBJ(f)->class_id))
        return -1;
    b = JS_VALUE_GET_OBJ(f)->u.func.function_bytecode;
    rf = b->rv;
    if (rf == RV_UNCONVERTED || rf == RV_REFUSED || rf->arg_count < 0)
        return -1;
    for (i = 0; i < rf->pcmap_count; i++) {
        const RVPcMask *m = &rf->pcmap[i];
        if (m->old < 0 || m->old >= b->byte_code_len ||
            b->byte_code_buf[m->old] != OP_for_of_start || m->end < 2)
            continue;
        if (rf->code[m->end - 2] == RV_DSTART)
            (*dst)++;
        else if (rf->code[m->end - 2] == RV_W_FOR_OF_START)
            (*win)++;
    }
    return 0;
}

/* Code no compiler makes, made from the function f of bytecode for the
   check of JS_ReadObject (rv_read_check of engine/regvm-entry.h), one kind
   at a time: 0 an instruction that pops more than the stack holds (its
   first push_1 made a drop), 1 a stack larger than stack_size (stack_size
   0), 2 two paths that meet with different depths (its first push_2 made a
   nop), 3 nip_catch with no catch (its first add made a nip_catch), 4
   mapped arguments with no closure state (var_ref_count 0), 5 to 8 a
   catch offset or a jump that leaves its place (white_crafted_jump). With
   undo, f gets back what the last call changed. -1 when f has no place
   for the kind. Such code must never run. */
static struct {
    JSFunctionBytecode *b;
    int pos, byte, stack_size, var_ref_count;
} white_crafted_saved;

/* the byte of b that white_crafted changes for the kinds 5 to 8, and *to
   its new value: 5 the drop of the catch offset of the first OP_catch made
   a return (the catch offset leaves the function), 6 the first jump made
   to go before the start of the code, 7 the first OP_catch made to go
   there, 8 the offset of the first for_of_next made 1, so that it takes
   the entry under its own three for its iterator. -1 when b has none. */
static int white_crafted_jump(JSFunctionBytecode *b, int kind, int *to)
{
    const uint8_t *bc = b->byte_code_buf;
    int pos, op, d = -1;

    for (pos = 0; pos < b->byte_code_len; pos += short_opcode_info(op).size) {
        op = bc[pos];
        switch (kind) {
        case 5:
            if (op == OP_catch) {
                d = 1;
                continue;
            }
            if (d < 0)
                continue;
            if (op == OP_drop && d == 1) {
                *to = OP_return;
                return pos;
            }
            /* the code of the try: straight, of the formats with their
               pops in the table */
            if (short_opcode_info(op).fmt == OP_FMT_npop ||
                short_opcode_info(op).fmt == OP_FMT_npop_u16 ||
                short_opcode_info(op).fmt == OP_FMT_npopx ||
                short_opcode_info(op).fmt == OP_FMT_label ||
                short_opcode_info(op).fmt == OP_FMT_label8 ||
                short_opcode_info(op).fmt == OP_FMT_label16)
                return -1;
            d += short_opcode_info(op).n_push - short_opcode_info(op).n_pop;
            break;
        case 6:
            if (op == OP_if_false8 || op == OP_if_true8 || op == OP_goto8) {
                if (pos + 1 >= 128)
                    return -1;
                *to = 0x80; /* -128 */
                return pos + 1;
            }
            if (op == OP_if_false || op == OP_if_true || op == OP_goto) {
                *to = 0x80; /* the top byte of the offset */
                return pos + 4;
            }
            break;
        case 7:
            if (op == OP_catch) {
                *to = 0x80;
                return pos + 4;
            }
            break;
        case 8:
            if (op == OP_for_of_next && bc[pos + 1] == 0) {
                *to = 1;
                return pos + 1;
            }
            break;
        }
    }
    return -1;
}

int white_crafted(JSContext *ctx, JSValueConst f, int kind, int undo)
{
    JSFunctionBytecode *b;
    int pos, op, want = -1, to = -1;

    (void)ctx;
    if (undo) {
        b = white_crafted_saved.b;
        if (!b)
            return -1;
        if (white_crafted_saved.pos >= 0)
            b->byte_code_buf[white_crafted_saved.pos] = white_crafted_saved.byte;
        b->stack_size = white_crafted_saved.stack_size;
        b->var_ref_count = white_crafted_saved.var_ref_count;
        white_crafted_saved.b = NULL;
        return 0;
    }
    if (JS_VALUE_GET_TAG(f) == JS_TAG_FUNCTION_BYTECODE)
        b = JS_VALUE_GET_PTR(f);
    else if (JS_VALUE_GET_TAG(f) == JS_TAG_OBJECT &&
             JS_VALUE_GET_OBJ(f)->class_id == JS_CLASS_BYTECODE_FUNCTION)
        b = JS_VALUE_GET_OBJ(f)->u.func.function_bytecode;
    else
        return -1;
    white_crafted_saved.b = b;
    white_crafted_saved.pos = -1;
    white_crafted_saved.stack_size = b->stack_size;
    white_crafted_saved.var_ref_count = b->var_ref_count;
    switch (kind) {
    case 0: want = OP_push_1; to = OP_drop; break;
    case 1: b->stack_size = 0; return 0;
    case 2: want = OP_push_2; to = OP_nop; break;
    case 3: want = OP_add; to = OP_nip_catch; break;
    case 4:
        if (!b->var_ref_count)
            return -1;
        b->var_ref_count = 0;
        return 0;
    case 5: case 6: case 7: case 8:
        pos = white_crafted_jump(b, kind, &to);
        if (pos < 0)
            break;
        white_crafted_saved.pos = pos;
        white_crafted_saved.byte = b->byte_code_buf[pos];
        b->byte_code_buf[pos] = to;
        return 0;
    default:
        return -1;
    }
    if (kind >= 5) {
        white_crafted_saved.b = NULL;
        return -1;
    }
    for (pos = 0; pos < b->byte_code_len; pos += short_opcode_info(op).size) {
        op = b->byte_code_buf[pos];
        if (op == want) {
            white_crafted_saved.pos = pos;
            white_crafted_saved.byte = op;
            b->byte_code_buf[pos] = to;
            return 0;
        }
    }
    white_crafted_saved.b = NULL;
    return -1;
}

/* the check of JS_ReadObject on the function f (rv_read_check): 0 when it
   lets f through, -1 with the exception pending */
/* whether f is a function of bytecode: plain, async, a generator or an
   async generator */
static BOOL white_is_bytecode_function(JSValueConst f)
{
    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT)
        return FALSE;
    switch (JS_VALUE_GET_OBJ(f)->class_id) {
    case JS_CLASS_BYTECODE_FUNCTION: case JS_CLASS_ASYNC_FUNCTION:
    case JS_CLASS_GENERATOR_FUNCTION: case JS_CLASS_ASYNC_GENERATOR_FUNCTION:
        return TRUE;
    default:
        return FALSE;
    }
}

int white_regvm_read_check(JSContext *ctx, JSValueConst f)
{
    if (!white_is_bytecode_function(f))
        return -2;
    return rv_read_check(ctx, JS_VALUE_GET_OBJ(f)->u.func.function_bytecode);
}

/* The code of the function f replaced from its start by the code of kind,
   the rest made OP_nop, with its ret_bits only where that code returns:
   code only a file could hold. 0: iterator_get_value_done with no
   iterator under it (undefined undefined undefined object
   iterator_get_value_done drop drop return_async), 1: a gosub whose
   finally drops what the gosub had and returns with a catch offset of its
   own under the return address (undefined gosub, return, then drop drop
   catch push_i32 ret, and return at the catch), 2: a gosub whose finally
   returns at once (undefined gosub, return, then ret), 3 to 11: records
   of an array destructured, read past or through their catch offset, or
   made by no for_of_start (see the cases). With undo, f gets
   back its code and its ret_bits, as it must before it is freed, which
   frees the atoms its code names. -1 when f has too little code, or no
   ret_bits for a gosub. */
static struct {
    JSFunctionBytecode *b;
    uint8_t *code;
    uint32_t *bits;
} white_ccode_saved;

int white_crafted_code(JSContext *ctx, JSValueConst f, int kind, int undo)
{
    JSFunctionBytecode *b;
    uint8_t code[32];
    int words, len = 0, ret = -1;

    if (undo) {
        b = white_ccode_saved.b;
        if (!b)
            return -1;
        words = (b->byte_code_len + 31) / 32;
        memcpy(b->byte_code_buf, white_ccode_saved.code, b->byte_code_len);
        if (b->ret_bits)
            memcpy(b->ret_bits, white_ccode_saved.bits, sizeof(uint32_t) * words);
        js_free_rt(ctx->rt, white_ccode_saved.code);
        js_free_rt(ctx->rt, white_ccode_saved.bits);
        white_ccode_saved.b = NULL;
        return 0;
    }
    if (!white_is_bytecode_function(f))
        return -1;
    b = JS_VALUE_GET_OBJ(f)->u.func.function_bytecode;
    switch (kind) {
    case 0:
        code[len++] = OP_undefined;
        code[len++] = OP_undefined;
        code[len++] = OP_undefined;
        code[len++] = OP_object;
        code[len++] = OP_iterator_get_value_done;
        code[len++] = OP_drop;
        code[len++] = OP_drop;
        code[len++] = OP_return_async;
        break;
    case 1: case 2:
        /* 0 undefined, 1 gosub 7, 6 return, 7 the finally */
        code[len++] = OP_undefined;
        code[len++] = OP_gosub;
        put_u32(code + len, 7 - 2);
        len += 4;
        ret = len;
        code[len++] = OP_return;
        if (kind == 2) {
            code[len++] = OP_ret;
            break;
        }
        /* 7 drop, 8 drop, 9 catch 20, 14 push_i32 6, 19 ret, 20 return */
        code[len++] = OP_drop;
        code[len++] = OP_drop;
        code[len++] = OP_catch;
        put_u32(code + len, 20 - 10);
        len += 4;
        code[len++] = OP_push_i32;
        put_u32(code + len, ret);
        len += 4;
        code[len++] = OP_ret;
        code[len++] = OP_return;
        break;
    case 3: case 4:
        /* the iterator of the record of the argument, returned: 3 its
           catch offset dropped (get_arg0 for_of_start drop), 4 taken off by
           nip_catch with no close after (get_arg0 for_of_start push_1
           nip_catch drop), then drop put_loc0 get_loc0 return */
        code[len++] = OP_get_arg0;
        code[len++] = OP_for_of_start;
        if (kind == 3) {
            code[len++] = OP_drop;
        } else {
            code[len++] = OP_push_1;
            code[len++] = OP_nip_catch;
            code[len++] = OP_drop;
        }
        code[len++] = OP_drop;
        code[len++] = OP_put_loc0;
        code[len++] = OP_get_loc0;
        code[len++] = OP_return;
        break;
    case 5:
        /* the first element into loc0, then the record closed as the return
           of a generator from a yield closes it (emit_return): get_arg0
           for_of_start for_of_next 0 drop put_loc0 push_1 nip_catch rot3r
           undefined iterator_close drop get_loc0 return */
        code[len++] = OP_get_arg0;
        code[len++] = OP_for_of_start;
        code[len++] = OP_for_of_next;
        code[len++] = 0;
        code[len++] = OP_drop;
        code[len++] = OP_put_loc0;
        code[len++] = OP_push_1;
        code[len++] = OP_nip_catch;
        code[len++] = OP_rot3r;
        code[len++] = OP_undefined;
        code[len++] = OP_iterator_close;
        code[len++] = OP_drop;
        code[len++] = OP_get_loc0;
        code[len++] = OP_return;
        break;
    case 6: case 7: case 8:
        /* a record no for_of_start made, whose next method is the
           JS_UNINITIALIZED of a local in its dead zone, which get_loc0
           reads with no check: set_loc_uninitialized 0, then 6 null, 7 and
           8 get_arg0, then get_loc0 undefined, then 6 and 7 for_of_next 0
           drop return, 8 iterator_close undefined return */
        code[len++] = OP_set_loc_uninitialized;
        put_u16(code + len, 0);
        len += 2;
        code[len++] = kind == 6 ? OP_null : OP_get_arg0;
        code[len++] = OP_get_loc0;
        code[len++] = OP_undefined;
        if (kind < 8) {
            code[len++] = OP_for_of_next;
            code[len++] = 0;
            code[len++] = OP_drop;
        } else {
            code[len++] = OP_iterator_close;
            code[len++] = OP_undefined;
        }
        code[len++] = OP_return;
        break;
    case 9:
        /* a finally run with the record of the argument under its return
           address, which it leaves as it is: 0 get_arg0, 1 for_of_start,
           2 gosub 14, 7 for_of_next 0, 9 drop, 10 put_loc0,
           11 iterator_close, 12 get_loc0, 13 return, 14 ret */
        code[len++] = OP_get_arg0;
        code[len++] = OP_for_of_start;
        code[len++] = OP_gosub;
        put_u32(code + len, 14 - 3);
        len += 4;
        ret = len;
        code[len++] = OP_for_of_next;
        code[len++] = 0;
        code[len++] = OP_drop;
        code[len++] = OP_put_loc0;
        code[len++] = OP_iterator_close;
        code[len++] = OP_get_loc0;
        code[len++] = OP_return;
        code[len++] = OP_ret;
        break;
    case 10: case 11:
        /* the catch offset of the record of the argument read: 10 by
           iterator_get_value_done, which writes the catch offset 0 of an
           iterator back (get_arg0 for_of_start object
           iterator_get_value_done drop drop), 11 by swap, which a read
           refuses (get_arg0 for_of_start swap swap), then for_of_next 0
           drop put_loc0 iterator_close get_loc0 return */
        code[len++] = OP_get_arg0;
        code[len++] = OP_for_of_start;
        if (kind == 10) {
            code[len++] = OP_object;
            code[len++] = OP_iterator_get_value_done;
            code[len++] = OP_drop;
            code[len++] = OP_drop;
        } else {
            code[len++] = OP_swap;
            code[len++] = OP_swap;
        }
        code[len++] = OP_for_of_next;
        code[len++] = 0;
        code[len++] = OP_drop;
        code[len++] = OP_put_loc0;
        code[len++] = OP_iterator_close;
        code[len++] = OP_get_loc0;
        code[len++] = OP_return;
        break;
    default:
        return -1;
    }
    if (len > b->byte_code_len || (ret >= 0 && (!b->ret_bits || ret >= b->byte_code_len)))
        return -1;
    words = (b->byte_code_len + 31) / 32;
    white_ccode_saved.code = js_malloc_rt(ctx->rt, b->byte_code_len);
    white_ccode_saved.bits = js_malloc_rt(ctx->rt, sizeof(uint32_t) * words);
    if (!white_ccode_saved.code || !white_ccode_saved.bits) {
        js_free_rt(ctx->rt, white_ccode_saved.code);
        js_free_rt(ctx->rt, white_ccode_saved.bits);
        return -1;
    }
    white_ccode_saved.b = b;
    memcpy(white_ccode_saved.code, b->byte_code_buf, b->byte_code_len);
    if (b->ret_bits) {
        memcpy(white_ccode_saved.bits, b->ret_bits, sizeof(uint32_t) * words);
        memset(b->ret_bits, 0, sizeof(uint32_t) * words);
        if (ret >= 0)
            b->ret_bits[ret >> 5] |= (uint32_t)1 << (ret & 31);
    }
    memcpy(b->byte_code_buf, code, len);
    memset(b->byte_code_buf + len, OP_nop, b->byte_code_len - len);
    return 0;
}

/* the register stack of ctx's runtime (engine/regvm-entry.h): out[0] the
   slots of its first segment, out[1] its segments, out[2] the bytes they
   take, out[3] the most slots it needed since it last settled, out[4] the
   bytes the heap is asked for the first segment past the most a new shared
   chunk of 64 KiB takes (HEAP_SHARED_MAX of engine/heap.h, the test of
   heap_grow_block), out[5] RV_SEG_FIRST and out[6] RV_SEG_MAX. -1 when it
   has none. */
int white_regvm_stack(JSContext *ctx, long out[7])
{
    JSRuntime *rt = ctx->rt;
    RVSeg *s = rt->rv_seg;

    if (!s)
        return -1;
    while (s->prev)
        s = s->prev;
    out[0] = s->size;
    out[1] = rv_stack_segments(rt);
    out[2] = rt->rv_stack_bytes;
    out[3] = rt->rv_hw;
    out[4] = (long)(sizeof(JSMallocLargeBlockHeader) + sizeof(RVSeg) + s->size * sizeof(JSValue)) -
             (long)HEAP_SHARED_MAX(65536);
    out[5] = RV_SEG_FIRST;
    out[6] = RV_SEG_MAX;
    return 0;
}

/* Code no compiler makes for the operand checks of the pass
   (rv_operands_ok of engine/regvm-pass.h): the start of the code of the
   function f replaced by the len bytes of code, its stack_size by
   stack_size, or with undo both put back. -1 when f's code is shorter. */
static struct {
    JSFunctionBytecode *b;
    uint8_t code[64];
    int len, stack_size;
} white_code_saved;

int white_set_code(JSContext *ctx, JSValueConst f, const uint8_t *code, int len,
                   int stack_size, int undo)
{
    JSFunctionBytecode *b;

    (void)ctx;
    if (undo) {
        b = white_code_saved.b;
        if (!b)
            return -1;
        memcpy(b->byte_code_buf, white_code_saved.code, white_code_saved.len);
        b->stack_size = white_code_saved.stack_size;
        white_code_saved.b = NULL;
        return 0;
    }
    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT ||
        JS_VALUE_GET_OBJ(f)->class_id != JS_CLASS_BYTECODE_FUNCTION)
        return -1;
    b = JS_VALUE_GET_OBJ(f)->u.func.function_bytecode;
    if (len > b->byte_code_len || len > (int)sizeof(white_code_saved.code))
        return -1;
    white_code_saved.b = b;
    white_code_saved.len = len;
    white_code_saved.stack_size = b->stack_size;
    memcpy(white_code_saved.code, b->byte_code_buf, len);
    memcpy(b->byte_code_buf, code, len);
    b->stack_size = stack_size;
    return 0;
}

/* the variable definitions of the first function of the compiled script
   f taken away (on), or put back (off), so that JS_WriteObject writes none
   for its arguments and locals. -1 when f has no function. */
int white_drop_vardefs(JSValueConst f, int on)
{
    static JSBytecodeVarDef *saved;
    JSFunctionBytecode *b;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_FUNCTION_BYTECODE)
        return -1;
    b = JS_VALUE_GET_PTR(f);
    if (b->cpool_count < 1 || JS_VALUE_GET_TAG(b->cpool[0]) != JS_TAG_FUNCTION_BYTECODE)
        return -1;
    b = JS_VALUE_GET_PTR(b->cpool[0]);
    if (on) {
        saved = b->vardefs;
        b->vardefs = NULL;
    } else {
        b->vardefs = saved;
    }
    return 0;
}

/* the opcodes the tests of the operand checks write */
int white_opcode(int which)
{
    static const int ops[] = { OP_gosub, OP_return_undef, OP_drop, OP_set_loc_uninitialized,
                               OP_push_const8, OP_return, OP_fclosure8, OP_nop, OP_push_atom_value };
    return which >= 0 && which < (int)countof(ops) ? ops[which] : -1;
}

/* whether a conversion that ran out of memory waits for the call out of
   the register loop that throws its error (rt->rv_oom_b) */
int white_regvm_oom_pending(JSContext *ctx)
{
    return ctx->rt->rv_oom_b != NULL;
}

/* js_heap_drain, as when the heap fails a request, with rt->rv_in_pass set
   as while a conversion works in the scratch (in_pass) or not: the bytes of
   scratch kept after it. */
int white_regvm_scratch_drain(JSContext *ctx, int in_pass)
{
    JSRuntime *rt = ctx->rt;
    BOOL was = rt->rv_in_pass;

    rt->rv_in_pass = in_pass;
    js_heap_drain(&rt->malloc_ctx);
    rt->rv_in_pass = was;
    return (int)rt->rv_scratch_size;
}

/* ---- captures that never change (capture_by_value) ---- */

static JSFunctionBytecode *white_find_function(JSContext *ctx, JSFunctionBytecode *b, const char *name)
{
    char buf[ATOM_GET_STR_BUF_SIZE];
    int i;

    if (!strcmp(JS_AtomGetStr(ctx, buf, sizeof(buf), b->func_name), name))
        return b;
    for (i = 0; i < b->cpool_count; i++) {
        if (JS_VALUE_GET_TAG(b->cpool[i]) == JS_TAG_FUNCTION_BYTECODE) {
            JSFunctionBytecode *f = white_find_function(ctx, JS_VALUE_GET_PTR(b->cpool[i]), name);
            if (f)
                return f;
        }
    }
    return NULL;
}

/* whether the closure variables of the functions below b that reach the
   slots of b are marked as those slots are: 0, or -1 */
/* the marks of the closure variables of c, a function below b whose
   constant pool up holds it, agree with the slots of b and the closure
   variables of up */
static int white_by_value_marks(JSFunctionBytecode *b, JSFunctionBytecode *up, JSFunctionBytecode *c, int depth)
{
    int i, k;

    for (k = 0; k < c->closure_var_count; k++) {
        JSClosureVar *cv = &c->closure_var[k];
        int want;
        if (cv->closure_type == JS_CLOSURE_REF) {
            if (cv->var_idx >= up->closure_var_count)
                return -1;
            want = up->closure_var[cv->var_idx].by_value;
        } else if (depth > 0 && (cv->closure_type == JS_CLOSURE_LOCAL || cv->closure_type == JS_CLOSURE_ARG))
            continue; /* the slots of the function above it */
        else if (cv->closure_type == JS_CLOSURE_LOCAL)
            want = b->vardefs[b->arg_count + cv->var_idx].by_value;
        else if (cv->closure_type == JS_CLOSURE_ARG)
            want = b->vardefs[cv->var_idx].by_value;
        else
            want = 0;
        if (cv->by_value != want)
            return -1;
    }
    for (i = 0; i < c->cpool_count; i++) {
        if (JS_VALUE_GET_TAG(c->cpool[i]) == JS_TAG_FUNCTION_BYTECODE &&
            white_by_value_marks(b, c, JS_VALUE_GET_PTR(c->cpool[i]), depth + 1))
            return -1;
    }
    return 0;
}

/* code no compiler makes, for white_by_value_crafted: 1 the push_const
   before the first define_class of f names no function of the pool, 2 the
   first jump of 8 bits of f lands on its first define_class. 0, or -1
   when f has no such place. */
static int white_by_value_craft(JSFunctionBytecode *f, int craft)
{
    uint8_t *buf = f->byte_code_buf;
    int pos, prev = -1, dc = -1, op;

    for (pos = 0; pos < f->byte_code_len; prev = pos, pos += short_opcode_info(buf[pos]).size) {
        op = buf[pos];
        if (op == OP_define_class || op == OP_define_class_computed) {
            dc = pos;
            break;
        }
    }
    if (dc < 0)
        return -1;
    if (craft == 1) {
        if (prev < 0 || buf[prev] != OP_push_const8)
            return -1;
        buf[prev + 1] = 255;
        return 0;
    }
    for (pos = 0; pos < f->byte_code_len; pos += short_opcode_info(buf[pos]).size) {
        if (short_opcode_info(buf[pos]).fmt == OP_FMT_label8 &&
            dc - (pos + 1) >= -128 && dc - (pos + 1) <= 127) {
            buf[pos + 1] = (uint8_t)(int8_t)(dc - (pos + 1));
            return 0;
        }
    }
    return -1;
}

/* The verdict of capture_by_value on the captured arguments and locals of
   the function named name in the script src, compiled, or with read set
   written with JS_WriteObject and read back: "a=1 b=0", the arguments then
   the locals. Returns 0, -1 when the script does not compile or has no such
   function, -2 when the marks of the closure variables below it do not
   match its slots. */
static int white_by_value_run(JSContext *ctx, const char *src, const char *name, int read, int craft, char *out, size_t cap)
{
    JSValue v;
    JSFunctionBytecode *b, *f;
    size_t n = 0;
    int i, ret = 0;

    out[0] = 0;
    v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(v))
        return -1;
    if (read) {
        size_t len;
        uint8_t *buf = JS_WriteObject(ctx, &len, v, JS_WRITE_OBJ_BYTECODE);
        JS_FreeValue(ctx, v);
        if (!buf)
            return -1;
        v = JS_ReadObject(ctx, buf, len, JS_READ_OBJ_BYTECODE);
        js_free(ctx, buf);
        if (JS_IsException(v))
            return -1;
    }
    b = JS_VALUE_GET_PTR(v);
    f = white_find_function(ctx, b, name);
    if (!f) {
        JS_FreeValue(ctx, v);
        return -1;
    }
    if (craft) {
        if (white_by_value_craft(f, craft)) {
            JS_FreeValue(ctx, v);
            return -3;
        }
        /* the run again on the changed code, from no marks: the functions
           of the cases have no function below a function below f */
        for (i = 0; i < f->arg_count + f->var_count; i++)
            f->vardefs[i].by_value = 0;
        for (i = 0; i < f->cpool_count; i++) {
            if (JS_VALUE_GET_TAG(f->cpool[i]) == JS_TAG_FUNCTION_BYTECODE) {
                JSFunctionBytecode *c = JS_VALUE_GET_PTR(f->cpool[i]);
                int k;
                for (k = 0; k < c->closure_var_count; k++)
                    c->closure_var[k].by_value = 0;
            }
        }
        capture_by_value(ctx, f);
    }
    for (i = 0; i < f->arg_count + f->var_count; i++) {
        char buf[ATOM_GET_STR_BUF_SIZE];
        JSBytecodeVarDef *vd = &f->vardefs[i];
        if (!vd->is_captured)
            continue;
        n += snprintf(out + n, n < cap ? cap - n : 0, "%s%s=%d", n ? " " : "",
                      JS_AtomGetStr(ctx, buf, sizeof(buf), vd->var_name), vd->by_value);
    }
    for (i = 0; i < f->cpool_count; i++) {
        if (JS_VALUE_GET_TAG(f->cpool[i]) == JS_TAG_FUNCTION_BYTECODE &&
            white_by_value_marks(f, f, JS_VALUE_GET_PTR(f->cpool[i]), 0))
            ret = -2;
    }
    JS_FreeValue(ctx, v);
    return ret;
}

int white_by_value(JSContext *ctx, const char *src, const char *name, int read, char *out, size_t cap)
{
    return white_by_value_run(ctx, src, name, read, 0, out, cap);
}

static size_t white_marks_rec(JSContext *ctx, JSFunctionBytecode *b, char *out, size_t cap, size_t n)
{
    char buf[ATOM_GET_STR_BUF_SIZE];
    int i, k;

    n += snprintf(out + n, n < cap ? cap - n : 0, "%s%s:", n ? " " : "",
                  JS_AtomGetStr(ctx, buf, sizeof(buf), b->func_name));
    for (k = 0; k < b->closure_var_count; k++)
        n += snprintf(out + n, n < cap ? cap - n : 0, "%d", b->closure_var[k].by_value);
    n += snprintf(out + n, n < cap ? cap - n : 0, "/%d", b->by_value_count);
    for (i = 0; i < b->cpool_count; i++) {
        if (JS_VALUE_GET_TAG(b->cpool[i]) == JS_TAG_FUNCTION_BYTECODE)
            n = white_marks_rec(ctx, JS_VALUE_GET_PTR(b->cpool[i]), out, cap, n);
    }
    return n;
}

/* the marks of the closure variables of the function f and of those below
   it, each function with its count: "f:/0 g:110/2" */
void white_marks(JSContext *ctx, JSValueConst f, char *out, size_t cap)
{
    out[0] = 0;
    if (JS_VALUE_GET_TAG(f) == JS_TAG_FUNCTION_BYTECODE)
        white_marks_rec(ctx, JS_VALUE_GET_PTR(f), out, cap, 0);
}

/* the mark of the closure variable at of the function name below f, or
   with at -1 of its first closure variable whose mark is was, turned the
   other way: its index, or -1 when there is none. by_value_count stays as
   it was: JS_WriteObject reads the marks only. */
int white_flip_by_value(JSContext *ctx, JSValueConst f, const char *name, int was, int at)
{
    JSFunctionBytecode *b;
    int k;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_FUNCTION_BYTECODE)
        return -1;
    b = white_find_function(ctx, JS_VALUE_GET_PTR(f), name);
    if (!b)
        return -1;
    if (at >= 0) {
        if (at >= b->closure_var_count)
            return -1;
        b->closure_var[at].by_value = !b->closure_var[at].by_value;
        return at;
    }
    for (k = 0; k < b->closure_var_count; k++) {
        if (b->closure_var[k].by_value == was) {
            b->closure_var[k].by_value = !was;
            return k;
        }
    }
    return -1;
}

/* with on, the function name below f gets n closure variables (n at least
   its count): its own, then copies of its first. Without, it gets back
   its own. 0, or -1. */
int white_wide_closure_vars(JSContext *ctx, JSValueConst f, const char *name, int n, int on)
{
    static JSClosureVar *saved;
    static int saved_count;
    JSFunctionBytecode *b;
    JSClosureVar *cv;
    int k;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_FUNCTION_BYTECODE)
        return -1;
    b = white_find_function(ctx, JS_VALUE_GET_PTR(f), name);
    if (!b)
        return -1;
    if (!on) {
        js_free(ctx, b->closure_var);
        b->closure_var = saved;
        b->closure_var_count = saved_count;
        return 0;
    }
    if (b->closure_var_count < 1 || n < b->closure_var_count)
        return -1;
    cv = js_malloc(ctx, sizeof(cv[0]) * n);
    if (!cv)
        return -1;
    for (k = 0; k < n; k++)
        cv[k] = b->closure_var[k < b->closure_var_count ? k : 0];
    saved = b->closure_var;
    saved_count = b->closure_var_count;
    b->closure_var = cv;
    b->closure_var_count = n;
    return 0;
}

/* white_by_value on the function name of src compiled and then changed as
   white_by_value_craft does with craft, the verdict made again: -3 when
   the change has no place in it */
int white_by_value_crafted(JSContext *ctx, const char *src, const char *name, int craft, char *out, size_t cap)
{
    return white_by_value_run(ctx, src, name, 0, craft, out, cap);
}

/* 1 when the compiled function name of src has an OP_nop between the
   push_const of a class constructor and its define_class, 0 when not, -1
   when the script does not compile or has no such function */
int white_class_after_nop(JSContext *ctx, const char *src, const char *name)
{
    JSValue v;
    JSFunctionBytecode *f;
    const uint8_t *buf;
    int pos, ret = 0;

    v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(v))
        return -1;
    f = white_find_function(ctx, JS_VALUE_GET_PTR(v), name);
    if (!f) {
        JS_FreeValue(ctx, v);
        return -1;
    }
    buf = f->byte_code_buf;
    for (pos = 0; pos < f->byte_code_len; pos += short_opcode_info(buf[pos]).size) {
        if ((buf[pos] == OP_push_const || buf[pos] == OP_push_const8)) {
            int next = pos + short_opcode_info(buf[pos]).size, n = 0;
            while (next < f->byte_code_len && buf[next] == OP_nop) {
                next++;
                n++;
            }
            if (n > 0 && next < f->byte_code_len &&
                (buf[next] == OP_define_class || buf[next] == OP_define_class_computed))
                ret = 1;
        }
    }
    JS_FreeValue(ctx, v);
    return ret;
}

/* The cache of the object rest site of the function fn (a function value):
   the shapes of the sources it keeps, as white_shape_of numbers them, into
   src (up to max), and the entries a miss replaced so far into *swaps.
   Return the entries, or -1 when fn has no register code or no such site
   (engine/regvm-ops-copy.h). */
int white_copy_site(JSValueConst fn, int site, uintptr_t *src, int max, int *swaps)
{
    JSFunctionBytecode *b;
    RVCopySite *cs;
    int i;

    if (JS_VALUE_GET_TAG(fn) != JS_TAG_OBJECT ||
        JS_VALUE_GET_OBJ(fn)->class_id != JS_CLASS_BYTECODE_FUNCTION)
        return -1;
    b = JS_VALUE_GET_OBJ(fn)->u.func.function_bytecode;
    if (!b || !b->rv || b->rv->arg_count < 0 || site >= b->rv->copy_count)
        return -1;
    cs = &b->rv->copy[site];
    for (i = 0; i < RV_COPY_WAYS && cs->e[i]; i++) {
        if (i < max)
            src[i] = (uintptr_t)cs->e[i]->src;
    }
    *swaps = cs->swaps;
    return i;
}
