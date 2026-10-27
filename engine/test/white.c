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

#include "../quickjs.c"

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
    return json_string_run(b, p, end, sep);
}

/* json_quote_run over the string value s from *pi. */
int white_json_quote_run(StringBuffer *b, JSValueConst s, int *pi)
{
    return json_quote_run(b, JS_VALUE_GET_STRING(s), pi);
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
