/*
 * Copyright (c) 2026 The slowjs Authors.
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

/* JS_WORK counts the units of work of the loops that read the interrupt flag
   (white_work), and each reading of the flag (cutils.h calls host_poll) closes
   a stretch: white_work_max is the longest stretch since white_work_reset,
   white_work_total the work in all of them. */
static unsigned long white_work, white_work_max, white_work_total;
#define JS_WORK(n) (white_work += (n), white_work_total += (n))
int host_poll(const volatile int *flag);
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
