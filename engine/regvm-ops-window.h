/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The ops that run on a window of the old stack in the register
   loop, with the source of the old loop. Read once for each part, see
   regvm-areas.h.

   A window op is an old instruction with no register form: the register
   loop runs the body of its old handler, the macro OP_BODY_<op> defined at
   the handler in JS_CallInternal (quickjs.c), so that both loops run one
   source. The body runs with sp at the temp of the depth the old stack has
   there, and the pass first puts every entry the body reads into its temp,
   so that the temps from the first one up to sp are the old stack. sp is
   computed from F at each op and is not live across a dispatch. The
   instruction is the opcode, the slot of sp, then the operands of the old
   instruction as it has them: the body reads them through pc as the old
   loop does, so after the opcode the instruction has as many bytes as the
   old one. */

#if RV_PART == RV_PART_LIST
/* w: the slot of the old sp, then the operands of the old instruction */
RV_OP(W_OBJECT_LIT)     /* w n16 k16 */
RV_OP(W_ARRAY_FROM)     /* w n16 */
RV_OP(W_APPLY)          /* w magic16 */
RV_OP(W_FOR_IN_START)   /* w */
RV_OP(W_FOR_IN_NEXT)    /* w */
RV_OP(W_FOR_OF_START)   /* w */
RV_OP(W_FOR_OF_NEXT)    /* w offset8 */
RV_OP(W_ITERATOR_CLOSE) /* w */
RV_OP(W_TO_OBJECT)      /* w */
RV_OP(W_SET_NAME)       /* w atom */
RV_OP(W_DELETE)         /* w */
RV_OP(W_APPEND)         /* w */
RV_OP(W_COPY_DATA_PROPERTIES) /* w mask8 */
#elif RV_PART == RV_PART_INFO
    case OP_object_lit: case OP_array_from:
        *npop = get_u16(bc + pos + 1);
        return 0;
    case OP_apply: case OP_for_in_start: case OP_for_in_next:
    case OP_for_of_start: case OP_for_of_next: case OP_iterator_close:
    case OP_to_object: case OP_set_name: case OP_delete: case OP_append:
    case OP_copy_data_properties:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* a window op: the top nread entries go into their temps, the old stack,
   then the instruction with the slot of sp and the old operands */
static void rv_window(RVPass *s, const uint8_t *p, int rvop, int nread)
{
    int i, len = short_opcode_info(p[0]).size;
    if (nread > s->sp) {
        s->failed = TRUE;
        return;
    }
    for (i = s->sp - nread; i < s->sp; i++)
        rv_materialize(s, i);
    rv_emit(s, rvop);
    rv_byte(s, rv_tslot(s, s->sp));
    for (i = 1; i < len; i++)
        rv_byte(s, p[i]);
}

/* after a window op: npop entries go, npush temps come at their slots */
static void rv_window_end(RVPass *s, int npop, int npush)
{
    s->sp -= npop;
    while (npush-- > 0)
        rv_push(s, RV_E_TEMP, rv_tslot(s, s->sp), 0);
}
#elif RV_PART == RV_PART_PASS
    /* the bodies that take their operands and free them when they throw:
       the operands are not owned across the throw */
    case OP_object_lit:
    case OP_array_from:
        i = get_u16(p + 1);
        rv_window(s, p, op == OP_object_lit ? RV_W_OBJECT_LIT : RV_W_ARRAY_FROM, i);
        rv_throws(s, rv_owned(s, i));
        rv_window_end(s, i, 1);
        break;
    /* the bodies that leave their operands on the stack when they throw */
    case OP_apply:
        rv_window(s, p, RV_W_APPLY, 3);
        rv_throws(s, rv_owned(s, 0));
        rv_window_end(s, 3, 1);
        break;
    case OP_for_in_start:
        /* build_for_in_iterator frees the operand when it fails and leaves
           JS_EXCEPTION, which has no reference to free */
        rv_window(s, p, RV_W_FOR_IN_START, 1);
        rv_throws(s, rv_owned(s, 0));
        rv_window_end(s, 1, 1);
        break;
    case OP_for_in_next:
        rv_window(s, p, RV_W_FOR_IN_NEXT, 1);
        rv_throws(s, rv_owned(s, 0));
        rv_window_end(s, 1, 3);
        break;
    case OP_for_of_start:
        /* iterator, next method and the catch offset 0 that closes the
           iterator when an exception passes it (rv_unwind) */
        rv_window(s, p, RV_W_FOR_OF_START, 1);
        rv_throws(s, rv_owned(s, 0));
        rv_window_end(s, 1, 3);
        break;
    case OP_for_of_next:
        /* the iterator is p[1] entries below the top three */
        rv_window(s, p, RV_W_FOR_OF_NEXT, 3 + p[1]);
        rv_throws(s, rv_owned(s, 0));
        rv_hooks(s, RV_HOOK_APART);
        rv_window_end(s, 3, 5);
        break;
    case OP_iterator_close:
        /* when the close throws, only the iterator of the three is left:
           the catch offset was dropped and the next method freed */
        rv_window(s, p, RV_W_ITERATOR_CLOSE, 3);
        if (!s->failed) {
            RVMask m = rv_owned(s, 3);
            rv_mask_set(&m, s->st[s->sp - 3].slot - s->first_temp);
            rv_throws(s, m);
        }
        rv_hooks(s, RV_HOOK_APART);
        rv_window_end(s, 3, 0);
        break;
    case OP_to_object:
    case OP_set_name:
        rv_window(s, p, op == OP_to_object ? RV_W_TO_OBJECT : RV_W_SET_NAME, 1);
        rv_throws(s, rv_owned(s, 0));
        rv_window_end(s, 1, 1);
        break;
    case OP_delete:
        rv_window(s, p, RV_W_DELETE, 2);
        rv_throws(s, rv_owned(s, 0));
        rv_window_end(s, 2, 1);
        break;
    case OP_append:
        rv_window(s, p, RV_W_APPEND, 3);
        rv_throws(s, rv_owned(s, 0));
        rv_window_end(s, 3, 2);
        break;
    case OP_copy_data_properties:
        {
            /* three entries named by the bits of the mask, up to 8 deep */
            int m = p[1], n = 3;
            n = max_int(n, 1 + (m & 3));
            n = max_int(n, 1 + ((m >> 2) & 7));
            n = max_int(n, 1 + ((m >> 5) & 7));
            rv_window(s, p, RV_W_COPY_DATA_PROPERTIES, n);
            rv_throws(s, rv_owned(s, 0));
            rv_window_end(s, 3, 3);
        }
        break;
#elif RV_PART == RV_PART_RUN_FN
/* The slow sides: each runs the body of the old handler on the window of
   the frame F whose sp is the slot ip[0], with pc at the old operands, and
   returns -1 where the body goes to the exception path. The old loop also
   stores its pc in sf->cur_pc there, for a backtrace: these store nothing,
   as RV_SYNC_TOP stored the end of the instruction, which the pcmap names
   (rv_frame_pc). */
#define RV_W_BEGIN                                                      \
    JSValue *sp = F + (int8_t)ip[0];                                    \
    const uint8_t *pc = ip + 1;                                         \
    (void)pc;
#define RV_W_END                                                        \
    (void)sp;                                                           \
    return 0;                                                           \
 exception:                                                             \
    return -1;

/* Copies. The bodies run here call these in place of a function of the
   engine (a #define around the body), each with the hook of the throw
   injection. They were copies so that the old loop, the one caller of the
   originals, kept its code: js_for_in_start and js_create_array_free went
   with it, and rv_define_object_name is still a copy of JS_DefineObjectName
   and must change with it: docs/upgrading.md. */

/* The throw-injection test (engine/regvm-inject.h) in a body: where the
   body calls the function of the engine that can fail, the hook fails in
   its place and leaves the stack as that function leaves it when it fails.
   The body then goes its own way to the exception path. */

/* the n values of tab freed, as js_new_object_lit and js_create_array_free
   take them when they fail */
static JSValue rv_inj_take(JSContext *ctx, JSValue *tab, int n)
{
    int i;
    for (i = 0; i < n; i++)
        JS_FreeValue(ctx, tab[i]);
    return JS_EXCEPTION;
}

/* as js_for_of_next when the next method throws: the iterator is freed and
   its slot left undefined, so that no exception closes it */
static int rv_inj_for_of_next(JSContext *ctx, JSValue *sp, int offset)
{
    JS_FreeValue(ctx, sp[offset]);
    sp[offset] = JS_UNDEFINED;
    return -1;
}

/* js_for_in_start of upstream's interpreter, with the hook of the throw
   injection: it calls build_for_in_iterator */
static int rv_for_in_start(JSContext *ctx, JSValue *sp)
{
    if (RV_INJ_HIT(ctx)) {
        /* build_for_in_iterator takes the value when it fails */
        JS_FreeValue(ctx, sp[-1]);
        sp[-1] = JS_EXCEPTION;
        return -1;
    }
    sp[-1] = build_for_in_iterator(ctx, sp[-1]);
    if (JS_IsException(sp[-1]))
        return -1;
    return 0;
}

/* a copy of JS_DefineObjectName with js_object_has_name in it
   (quickjs.c:15022 and 15003) */
static int rv_define_object_name(JSContext *ctx, JSValueConst obj,
                                 JSAtom name, int flags)
{
    JSProperty *pr;
    JSShapeProperty *prs;
    JSValueConst val;

    if (RV_INJ_HIT(ctx))
        return -1;
    if (name == JS_ATOM_NULL || !JS_IsObject(obj))
        return 0;
    /* js_object_has_name */
    prs = find_own_property(&pr, JS_VALUE_GET_OBJ(obj), JS_ATOM_name);
    if (prs) {
        if ((prs->flags & JS_PROP_TMASK) != JS_PROP_NORMAL)
            return 0;
        val = pr->u.value;
        if (JS_VALUE_GET_TAG(val) != JS_TAG_STRING)
            return 0;
        if (JS_VALUE_GET_STRING(val)->len != 0)
            return 0;
    }
    if (JS_DefinePropertyValue(ctx, obj, JS_ATOM_name,
                               JS_AtomToString(ctx, name), flags) < 0)
        return -1;
    return 0;
}

/* js_create_array_free of upstream's interpreter, with the hook of the
   throw injection: the array of the len values of tab, which it takes */
static JSValue rv_create_array_free(JSContext *ctx, int len, JSValue *tab)
{
    JSValue obj;
    int i;
    if (RV_INJ_HIT(ctx))
        return rv_inj_take(ctx, tab, len);
    obj = js_create_array(ctx, len, tab);
    for (i = 0; i < len; i++)
        JS_FreeValue(ctx, tab[i]);
    return obj;
}

static no_inline int rv_w_object_lit(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSFunctionBytecode *b = ((RVHeader *)F)->rf->b;
    JSValue ret_val;
    RV_W_BEGIN
#define js_new_object_lit(ctx, b, k, tab)                               \
    (RV_INJ_HIT(ctx) ? rv_inj_take(ctx, tab, (b)->object_lits[k].count) : \
     js_new_object_lit(ctx, b, k, tab))
    OP_BODY_object_lit(ctx, b, sp, pc, ret_val, exception)
#undef js_new_object_lit
    RV_W_END
}

static no_inline int rv_w_array_from(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue ret_val;
    int call_argc;
    RV_W_BEGIN
#define js_create_array_free rv_create_array_free
    OP_BODY_array_from(ctx, sp, pc, call_argc, ret_val, exception)
#undef js_create_array_free
    RV_W_END
}

static no_inline int rv_w_apply(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue ret_val;
    RV_W_BEGIN
#define js_function_apply(ctx, f, n, argv, magic)                       \
    (RV_INJ_HIT(ctx) ? JS_EXCEPTION : js_function_apply(ctx, f, n, argv, magic))
    OP_BODY_apply(ctx, sp, pc, ret_val, (void)0, exception)
#undef js_function_apply
    RV_W_END
}

static no_inline int rv_w_for_in_start(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
#define js_for_in_start rv_for_in_start
    OP_BODY_for_in_start(ctx, sp, (void)0, exception)
#undef js_for_in_start
    RV_W_END
}

static no_inline int rv_w_for_in_next(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
#define js_for_in_next(ctx, sp) (RV_INJ_HIT(ctx) ? -1 : js_for_in_next(ctx, sp))
    OP_BODY_for_in_next(ctx, sp, (void)0, exception)
#undef js_for_in_next
    RV_W_END
}

static no_inline int rv_w_for_of_start(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
#define js_for_of_start(ctx, sp, is_async)                              \
    (RV_INJ_HIT(ctx) ? -1 : js_for_of_start(ctx, sp, is_async))
    OP_BODY_for_of_start(ctx, sp, (void)0, exception)
#undef js_for_of_start
    RV_W_END
}

static no_inline int rv_w_for_of_next(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
    /* an iterator that is done has no next to fail */
    if (JS_IsUndefined(sp[-3 - ip[1]]))
        RV_INJ_NONE("skip");
#define js_for_of_next(ctx, sp, offset)                                 \
    (RV_INJ_HIT(ctx) ? rv_inj_for_of_next(ctx, sp, offset) : js_for_of_next(ctx, sp, offset))
    OP_BODY_for_of_next(ctx, sp, pc, (void)0, exception)
#undef js_for_of_next
    RV_W_END
}

static no_inline int rv_w_iterator_close(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
    /* an iterator that is done is not closed: nothing to fail */
    if (JS_IsUndefined(sp[-3]))
        RV_INJ_NONE("skip");
#define JS_IteratorClose(ctx, it, is_throw)                             \
    (RV_INJ_HIT(ctx) ? -1 : JS_IteratorClose(ctx, it, is_throw))
    OP_BODY_iterator_close(ctx, sp, (void)0, exception)
#undef JS_IteratorClose
    RV_W_END
}

static no_inline int rv_w_to_object(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue ret_val;
    RV_W_BEGIN
    /* an object has no conversion to fail */
    if (JS_VALUE_GET_TAG(sp[-1]) == JS_TAG_OBJECT)
        RV_INJ_NONE("skip");
#define JS_ToObject(ctx, v) (RV_INJ_HIT(ctx) ? JS_EXCEPTION : JS_ToObject(ctx, v))
    OP_BODY_to_object(ctx, sp, ret_val, (void)0, exception)
#undef JS_ToObject
    RV_W_END
}

static no_inline int rv_w_set_name(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
#define JS_DefineObjectName rv_define_object_name
    OP_BODY_set_name(ctx, sp, pc, exception)
#undef JS_DefineObjectName
    RV_W_END
}

static no_inline int rv_w_delete(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
#define js_operator_delete(ctx, sp, flags) (RV_INJ_HIT(ctx) ? -1 : js_operator_delete(ctx, sp, flags))
    OP_BODY_delete(ctx, sp, rv_throw_flags(F), (void)0, exception)
#undef js_operator_delete
    RV_W_END
}

static no_inline int rv_w_append(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
#define js_append_enumerate(ctx, sp) (RV_INJ_HIT(ctx) ? -1 : js_append_enumerate(ctx, sp))
    OP_BODY_append(ctx, sp, (void)0, exception)
#undef js_append_enumerate
    RV_W_END
}

static no_inline int rv_w_copy_data_properties(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RV_W_BEGIN
#define JS_CopyDataProperties(ctx, target, source, excluded, setprop)   \
    (RV_INJ_HIT(ctx) ? -1 : JS_CopyDataProperties(ctx, target, source, excluded, setprop))
    OP_BODY_copy_data_properties(ctx, sp, pc, (void)0, exception)
#undef JS_CopyDataProperties
    RV_W_END
}
#undef RV_W_BEGIN
#undef RV_W_END
#elif RV_PART == RV_PART_RUN
    /* the window ops: len is the size of the old instruction, the slot of
       sp and the old operands. The body may run JS (a getter, an iterator,
       a call), so the register stack is given to it above this frame. F
       and pc are all that is live across it, and pc is at the end of the
       instruction when it throws, for the pcmap. */
#define RV_WINDOW(label, fn, len)                                       \
 label:                                                                 \
    ip = pc;                                                            \
    pc += (len);                                                        \
    RV_SYNC_TOP();                                                      \
    if (fn(ctx, F, ip))                                                 \
        goto rv_exception;                                              \
    RV_NEXT();

    RV_WINDOW(rv_W_OBJECT_LIT, rv_w_object_lit, 5)
    RV_WINDOW(rv_W_ARRAY_FROM, rv_w_array_from, 3)
    RV_WINDOW(rv_W_APPLY, rv_w_apply, 3)
    RV_WINDOW(rv_W_FOR_IN_START, rv_w_for_in_start, 1)
    RV_WINDOW(rv_W_FOR_IN_NEXT, rv_w_for_in_next, 1)
    RV_WINDOW(rv_W_FOR_OF_START, rv_w_for_of_start, 1)
    RV_WINDOW(rv_W_FOR_OF_NEXT, rv_w_for_of_next, 2)
    RV_WINDOW(rv_W_ITERATOR_CLOSE, rv_w_iterator_close, 1)
    RV_WINDOW(rv_W_TO_OBJECT, rv_w_to_object, 1)
    RV_WINDOW(rv_W_SET_NAME, rv_w_set_name, 5)
    RV_WINDOW(rv_W_DELETE, rv_w_delete, 1)
    RV_WINDOW(rv_W_APPEND, rv_w_append, 1)
    RV_WINDOW(rv_W_COPY_DATA_PROPERTIES, rv_w_copy_data_properties, 2)
#undef RV_WINDOW
#endif
