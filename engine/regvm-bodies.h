/*
 * QuickJS Javascript Engine: the bodies of handlers the register loop runs
 *
 * Copyright (c) 2017-2025 Fabrice Bellard
 * Copyright (c) 2017-2025 Charlie Gordon
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
 * THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 */

/* The handlers of old instructions that the register loop runs as window
   ops (engine/regvm-ops-window.h) or through OLD (engine/regvm-ops-old.h),
   as macros OP_BODY_<op>, and the operand values of throw_error and
   define_method that the compiler and the register loop read, as the
   handlers of QuickJS's interpreter had them. They can take whatever shape
   suits the register loop. */

#define OP_BODY_object_lit(ctx, b, sp, pc, ret_val, exception)              \
            {                                                               \
                int n = get_u16(pc);                                        \
                int k = get_u16(pc + 2);                                    \
                pc += 4;                                                    \
                ret_val = js_new_object_lit(ctx, b, k, sp - n);             \
                sp -= n;                                                    \
                if (unlikely(JS_IsException(ret_val)))                      \
                    goto exception;                                         \
                *sp++ = ret_val;                                            \
            }

#define OP_BODY_array_from(ctx, sp, pc, call_argc, ret_val, exception)      \
            call_argc = get_u16(pc);                                        \
            pc += 2;                                                        \
            ret_val = js_create_array_free(ctx, call_argc, sp - call_argc); \
            sp -= call_argc;                                                \
            if (unlikely(JS_IsException(ret_val)))                          \
                goto exception;                                             \
            *sp++ = ret_val;

#define OP_BODY_apply(ctx, sp, pc, ret_val, save_pc, exception)             \
            {                                                               \
                int magic;                                                  \
                magic = get_u16(pc);                                        \
                pc += 2;                                                    \
                save_pc;                                                    \
                ret_val = js_function_apply(ctx, sp[-3], 2, (JSValueConst *)&sp[-2], magic); \
                if (unlikely(JS_IsException(ret_val)))                      \
                    goto exception;                                         \
                JS_FreeValue(ctx, sp[-3]);                                  \
                JS_FreeValue(ctx, sp[-2]);                                  \
                JS_FreeValue(ctx, sp[-1]);                                  \
                sp -= 3;                                                    \
                *sp++ = ret_val;                                            \
            }

#define JS_THROW_VAR_RO             0
#define JS_THROW_VAR_REDECL         1
#define JS_THROW_VAR_UNINITIALIZED  2
#define JS_THROW_ERROR_DELETE_SUPER   3
#define JS_THROW_ERROR_ITERATOR_THROW 4
#define JS_THROW_ERROR_INVALID_LVALUE_CALL 5

#define OP_BODY_for_in_start(ctx, sp, save_pc, exception)                   \
            save_pc;                                                        \
            if (js_for_in_start(ctx, sp))                                   \
                goto exception;

#define OP_BODY_for_in_next(ctx, sp, save_pc, exception)                    \
            save_pc;                                                        \
            if (js_for_in_next(ctx, sp))                                    \
                goto exception;                                             \
            sp += 2;

#define OP_BODY_for_of_start(ctx, sp, save_pc, exception)                   \
            save_pc;                                                        \
            if (js_for_of_start(ctx, sp, FALSE))                            \
                goto exception;                                             \
            sp += 1;                                                        \
            *sp++ = JS_NewCatchOffset(ctx, 0);

#define OP_BODY_for_of_next(ctx, sp, pc, save_pc, exception)                \
            {                                                               \
                int offset = -3 - pc[0];                                    \
                pc += 1;                                                    \
                save_pc;                                                    \
                if (js_for_of_next(ctx, sp, offset))                        \
                    goto exception;                                         \
                sp += 2;                                                    \
            }

#define OP_BODY_iterator_close(ctx, sp, save_pc, exception)                 \
            /* iter_obj next catch_offset -> */                             \
            sp--; /* drop the catch offset to avoid getting caught by exception */ \
            JS_FreeValue(ctx, sp[-1]); /* drop the next method */           \
            sp--;                                                           \
            if (!JS_IsUndefined(sp[-1])) {                                  \
                save_pc;                                                    \
                if (JS_IteratorClose(ctx, sp[-1], FALSE))                   \
                    goto exception;                                         \
                JS_FreeValue(ctx, sp[-1]);                                  \
            }                                                               \
            sp--;

#define OP_BODY_set_name(ctx, sp, pc, exception)                            \
            {                                                               \
                int ret;                                                    \
                JSAtom atom;                                                \
                atom = get_u32(pc);                                         \
                pc += 4;                                                    \
                ret = JS_DefineObjectName(ctx, sp[-1], atom, JS_PROP_CONFIGURABLE); \
                if (unlikely(ret < 0))                                      \
                    goto exception;                                         \
            }

#define OP_BODY_append(ctx, sp, save_pc, exception)                         \
            {                                                               \
                save_pc;                                                    \
                if (js_append_enumerate(ctx, sp))                           \
                    goto exception;                                         \
                JS_FreeValue(ctx, *--sp);                                   \
            }

#define OP_BODY_copy_data_properties(ctx, sp, pc, save_pc, exception)       \
            {                                                               \
                /* stack offsets (-1 based):                                \
                   2 bits for target,                                       \
                   3 bits for source,                                       \
                   2 bits for exclusionList */                              \
                int mask;                                                   \
                mask = *pc++;                                               \
                save_pc;                                                    \
                if (JS_CopyDataProperties(ctx, sp[-1 - (mask & 3)],         \
                                          sp[-1 - ((mask >> 2) & 7)],       \
                                          sp[-1 - ((mask >> 5) & 7)], 0))   \
                    goto exception;                                         \
            }

/* flags: JS_PROP_THROW_STRICT in the old loop, rv_throw_flags(F) in the
   register loop (js_operator_delete) */
#define OP_BODY_delete(ctx, sp, flags, save_pc, exception)                  \
            save_pc;                                                        \
            if (js_operator_delete(ctx, sp, flags))                         \
                goto exception;                                             \
            sp--;

#define OP_BODY_to_object(ctx, sp, ret_val, save_pc, exception)             \
            if (JS_VALUE_GET_TAG(sp[-1]) != JS_TAG_OBJECT) {                \
                save_pc;                                                    \
                ret_val = JS_ToObject(ctx, sp[-1]);                         \
                if (JS_IsException(ret_val))                                \
                    goto exception;                                         \
                JS_FreeValue(ctx, sp[-1]);                                  \
                sp[-1] = ret_val;                                           \
            }

#define OP_BODY_home_object(ctx, p, sp)                                     \
                    {                                                       \
                        JSObject *p1;                                       \
                        p1 = p->u.func.home_object;                         \
                        if (unlikely(!p1))                                  \
                            *sp++ = JS_UNDEFINED;                           \
                        else                                                \
                            *sp++ = JS_DupValue(ctx, JS_MKPTR(JS_TAG_OBJECT, p1)); \
                    }

#define OP_BODY_import_meta(ctx, sp, exception)                             \
                    *sp++ = js_import_meta(ctx);                            \
                    if (unlikely(JS_IsException(sp[-1])))                   \
                        goto exception;

#define OP_BODY_check_brand(ctx, sp, exception)                             \
            {                                                               \
                int ret = JS_CheckBrand(ctx, sp[-2], sp[-1]);               \
                if (ret < 0)                                                \
                    goto exception;                                         \
                if (!ret) {                                                 \
                    JS_ThrowTypeError(ctx, "invalid brand on object");      \
                    goto exception;                                         \
                }                                                           \
            }

#define OP_BODY_add_brand(ctx, sp, exception)                               \
            if (JS_AddBrand(ctx, sp[-2], sp[-1]) < 0)                       \
                goto exception;                                             \
            JS_FreeValue(ctx, sp[-2]);                                      \
            JS_FreeValue(ctx, sp[-1]);                                      \
            sp -= 2;

#define OP_BODY_get_super(ctx, sp, save_pc, exception)                      \
            {                                                               \
                JSValue proto;                                              \
                save_pc;                                                    \
                proto = JS_GetPrototype(ctx, sp[-1]);                       \
                if (JS_IsException(proto))                                  \
                    goto exception;                                         \
                JS_FreeValue(ctx, sp[-1]);                                  \
                sp[-1] = proto;                                             \
            }

#define OP_BODY_import(ctx, sp, save_pc, exception)                         \
            {                                                               \
                JSValue val;                                                \
                save_pc;                                                    \
                val = js_dynamic_import(ctx, sp[-2], sp[-1]);               \
                if (JS_IsException(val))                                    \
                    goto exception;                                         \
                JS_FreeValue(ctx, sp[-2]);                                  \
                JS_FreeValue(ctx, sp[-1]);                                  \
                sp--;                                                       \
                sp[-1] = val;                                               \
            }

#define OP_BODY_put_var_ref_check_init(ctx, b, var_refs, sp, pc, exception) \
            {                                                               \
                int idx;                                                    \
                idx = get_u16(pc);                                          \
                pc += 2;                                                    \
                if (unlikely(!JS_IsUninitialized(*var_refs[idx]->pvalue))) { \
                    JS_ThrowReferenceErrorUninitialized2(ctx, b, idx, TRUE); \
                    goto exception;                                         \
                }                                                           \
                set_value(ctx, var_refs[idx]->pvalue, sp[-1]);              \
                sp--;                                                       \
            }

#define OP_BODY_private_symbol(ctx, sp, pc, exception)                      \
            {                                                               \
                JSAtom atom;                                                \
                JSValue val;                                                \
                                                                            \
                atom = get_u32(pc);                                         \
                pc += 4;                                                    \
                val = JS_NewSymbolFromAtom(ctx, atom, JS_ATOM_TYPE_PRIVATE); \
                if (JS_IsException(val))                                    \
                    goto exception;                                         \
                *sp++ = val;                                                \
            }

#define OP_BODY_get_private_field(ctx, sp, exception)                       \
            {                                                               \
                JSValue val;                                                \
                                                                            \
                val = JS_GetPrivateField(ctx, sp[-2], sp[-1]);              \
                JS_FreeValue(ctx, sp[-1]);                                  \
                JS_FreeValue(ctx, sp[-2]);                                  \
                sp[-2] = val;                                               \
                sp--;                                                       \
                if (unlikely(JS_IsException(val)))                          \
                    goto exception;                                         \
            }

#define OP_BODY_put_private_field(ctx, sp, exception)                       \
            {                                                               \
                int ret;                                                    \
                ret = JS_SetPrivateField(ctx, sp[-3], sp[-1], sp[-2]);      \
                JS_FreeValue(ctx, sp[-3]);                                  \
                JS_FreeValue(ctx, sp[-1]);                                  \
                sp -= 3;                                                    \
                if (unlikely(ret < 0))                                      \
                    goto exception;                                         \
            }

#define OP_BODY_define_private_field(ctx, sp, exception)                    \
            {                                                               \
                int ret;                                                    \
                ret = JS_DefinePrivateField(ctx, sp[-3], sp[-2], sp[-1]);   \
                JS_FreeValue(ctx, sp[-2]);                                  \
                sp -= 2;                                                    \
                if (unlikely(ret < 0))                                      \
                    goto exception;                                         \
            }

#define OP_BODY_set_name_computed(ctx, sp, exception)                       \
            {                                                               \
                int ret;                                                    \
                ret = JS_DefineObjectNameComputed(ctx, sp[-1], sp[-2], JS_PROP_CONFIGURABLE); \
                if (unlikely(ret < 0))                                      \
                    goto exception;                                         \
            }

#define OP_BODY_set_proto(ctx, sp, save_pc, exception)                      \
            {                                                               \
                JSValue proto;                                              \
                save_pc;                                                    \
                proto = sp[-1];                                             \
                if (JS_IsObject(proto) || JS_IsNull(proto)) {               \
                    if (JS_SetPrototypeInternal(ctx, sp[-2], proto, TRUE) < 0) \
                        goto exception;                                     \
                }                                                           \
                JS_FreeValue(ctx, proto);                                   \
                sp--;                                                       \
            }

#define OP_BODY_set_home_object(ctx, sp)                                    \
            js_method_set_home_object(ctx, sp[-1], sp[-2]);
#define OP_DEFINE_METHOD_METHOD 0
#define OP_DEFINE_METHOD_GETTER 1
#define OP_DEFINE_METHOD_SETTER 2
#define OP_DEFINE_METHOD_ENUMERABLE 4

#define OP_BODY_define_method(ctx, opcode, sp, pc, exception)               \
            {                                                               \
                JSValue getter, setter, value;                              \
                JSValueConst obj;                                           \
                JSAtom atom;                                                \
                int flags, ret, op_flags;                                   \
                BOOL is_computed;                                           \
                                                                            \
                is_computed = (opcode == OP_define_method_computed);        \
                if (is_computed) {                                          \
                    atom = JS_ValueToAtom(ctx, sp[-2]);                     \
                    if (unlikely(atom == JS_ATOM_NULL))                     \
                        goto exception;                                     \
                    opcode += OP_define_method - OP_define_method_computed; \
                } else {                                                    \
                    atom = get_u32(pc);                                     \
                    pc += 4;                                                \
                }                                                           \
                op_flags = *pc++;                                           \
                                                                            \
                obj = sp[-2 - is_computed];                                 \
                flags = JS_PROP_HAS_CONFIGURABLE | JS_PROP_CONFIGURABLE |   \
                    JS_PROP_HAS_ENUMERABLE | JS_PROP_THROW;                 \
                if (op_flags & OP_DEFINE_METHOD_ENUMERABLE)                 \
                    flags |= JS_PROP_ENUMERABLE;                            \
                op_flags &= 3;                                              \
                value = JS_UNDEFINED;                                       \
                getter = JS_UNDEFINED;                                      \
                setter = JS_UNDEFINED;                                      \
                if (op_flags == OP_DEFINE_METHOD_METHOD) {                  \
                    value = sp[-1];                                         \
                    flags |= JS_PROP_HAS_VALUE | JS_PROP_HAS_WRITABLE | JS_PROP_WRITABLE; \
                } else if (op_flags == OP_DEFINE_METHOD_GETTER) {           \
                    getter = sp[-1];                                        \
                    flags |= JS_PROP_HAS_GET;                               \
                } else {                                                    \
                    setter = sp[-1];                                        \
                    flags |= JS_PROP_HAS_SET;                               \
                }                                                           \
                ret = js_method_set_properties(ctx, sp[-1], atom, flags, obj); \
                if (ret >= 0) {                                             \
                    ret = JS_DefineProperty(ctx, obj, atom, value,          \
                                            getter, setter, flags);         \
                }                                                           \
                JS_FreeValue(ctx, sp[-1]);                                  \
                if (is_computed) {                                          \
                    JS_FreeAtom(ctx, atom);                                 \
                    JS_FreeValue(ctx, sp[-2]);                              \
                }                                                           \
                sp -= 1 + is_computed;                                      \
                if (unlikely(ret < 0))                                      \
                    goto exception;                                         \
            }

#define OP_BODY_define_class(ctx, opcode, sp, pc, var_refs, sf, exception)  \
            {                                                               \
                int class_flags;                                            \
                JSAtom atom;                                                \
                                                                            \
                atom = get_u32(pc);                                         \
                class_flags = pc[4];                                        \
                pc += 5;                                                    \
                if (js_op_define_class(ctx, sp, atom, class_flags,          \
                                       var_refs, sf,                        \
                                       (opcode == OP_define_class_computed)) < 0) \
                    goto exception;                                         \
            }

#define OP_BODY_get_super_value(ctx, sp, save_pc, exception)                \
            {                                                               \
                JSValue val;                                                \
                JSAtom atom;                                                \
                save_pc;                                                    \
                atom = JS_ValueToAtom(ctx, sp[-1]);                         \
                if (unlikely(atom == JS_ATOM_NULL))                         \
                    goto exception;                                         \
                val = JS_GetPropertyInternal(ctx, sp[-2], atom, sp[-3], FALSE); \
                JS_FreeAtom(ctx, atom);                                     \
                if (unlikely(JS_IsException(val)))                          \
                    goto exception;                                         \
                JS_FreeValue(ctx, sp[-1]);                                  \
                JS_FreeValue(ctx, sp[-2]);                                  \
                JS_FreeValue(ctx, sp[-3]);                                  \
                sp[-3] = val;                                               \
                sp -= 2;                                                    \
            }

#define OP_BODY_put_super_value(ctx, sp, throw_flags, save_pc, exception)   \
            {                                                               \
                int ret;                                                    \
                JSAtom atom;                                                \
                save_pc;                                                    \
                if (JS_VALUE_GET_TAG(sp[-3]) != JS_TAG_OBJECT) {            \
                    JS_ThrowTypeErrorNotAnObject(ctx);                      \
                    goto exception;                                         \
                }                                                           \
                atom = JS_ValueToAtom(ctx, sp[-2]);                         \
                if (unlikely(atom == JS_ATOM_NULL))                         \
                    goto exception;                                         \
                ret = JS_SetPropertyInternal(ctx, sp[-3], atom, sp[-1], sp[-4], \
                                             throw_flags);                  \
                JS_FreeAtom(ctx, atom);                                     \
                JS_FreeValue(ctx, sp[-4]);                                  \
                JS_FreeValue(ctx, sp[-3]);                                  \
                JS_FreeValue(ctx, sp[-2]);                                  \
                sp -= 4;                                                    \
                if (ret < 0)                                                \
                    goto exception;                                         \
            }

#define OP_BODY_private_in(ctx, sp, save_pc, exception)                     \
            save_pc;                                                        \
            if (js_operator_private_in(ctx, sp))                            \
                goto exception;                                             \
            sp--;

#define OP_BODY_var_object(ctx, sp, exception)                              \
                    *sp++ = JS_NewObjectProto(ctx, JS_NULL);                \
                    if (unlikely(JS_IsException(sp[-1])))                   \
                        goto exception;

/* the bodies of eval and apply_eval, which the register loop runs too
   (engine/regvm-ops-eval.h): a direct eval when the function is the eval
   of the realm, which reads the running frame of rt, a plain call when not */
#define OP_BODY_eval(ctx, sp, pc, call_argc, call_argv, ret_val, save_pc, exception) \
            {                                                               \
                JSValueConst obj;                                           \
                int scope_idx, ai;                                          \
                call_argc = get_u16(pc);                                    \
                scope_idx = get_u16(pc + 2) + ARG_SCOPE_END;                \
                pc += 4;                                                    \
                call_argv = sp - call_argc;                                 \
                save_pc;                                                    \
                if (js_same_value(ctx, call_argv[-1], ctx->eval_obj)) {     \
                    if (call_argc >= 1)                                     \
                        obj = call_argv[0];                                 \
                    else                                                    \
                        obj = JS_UNDEFINED;                                 \
                    ret_val = JS_EvalObject(ctx, JS_UNDEFINED, obj,         \
                                            JS_EVAL_TYPE_DIRECT, scope_idx); \
                } else if (rv_entry_fast(ctx->rt, call_argv[-1])) {         \
                    ret_val = JS_RegvmRun(ctx, call_argv[-1], JS_UNDEFINED,  \
                                          JS_UNDEFINED, call_argc, call_argv); \
                } else {                                                    \
                    ret_val = JS_CallInternal(ctx, call_argv[-1], JS_UNDEFINED, \
                                              JS_UNDEFINED, call_argc, call_argv, 0); \
                }                                                           \
                if (unlikely(JS_IsException(ret_val)))                      \
                    goto exception;                                         \
                for(ai = -1; ai < call_argc; ai++)                          \
                    JS_FreeValue(ctx, call_argv[ai]);                       \
                sp -= call_argc + 1;                                        \
                *sp++ = ret_val;                                            \
            }

#define OP_BODY_apply_eval(ctx, sp, pc, ret_val, save_pc, exception)        \
            {                                                               \
                int scope_idx;                                              \
                uint32_t len;                                               \
                JSValue *tab;                                               \
                JSValueConst obj;                                           \
                                                                            \
                scope_idx = get_u16(pc) + ARG_SCOPE_END;                    \
                pc += 2;                                                    \
                save_pc;                                                    \
                tab = build_arg_list(ctx, &len, sp[-1]);                    \
                if (!tab)                                                   \
                    goto exception;                                         \
                if (js_same_value(ctx, sp[-2], ctx->eval_obj)) {            \
                    if (len >= 1)                                           \
                        obj = tab[0];                                       \
                    else                                                    \
                        obj = JS_UNDEFINED;                                 \
                    ret_val = JS_EvalObject(ctx, JS_UNDEFINED, obj,         \
                                            JS_EVAL_TYPE_DIRECT, scope_idx); \
                } else {                                                    \
                    ret_val = js_call(ctx, sp[-2], JS_UNDEFINED, len,       \
                                      (JSValueConst *)tab);                 \
                }                                                           \
                free_arg_list(ctx, tab, len);                               \
                if (unlikely(JS_IsException(ret_val)))                      \
                    goto exception;                                         \
                JS_FreeValue(ctx, sp[-2]);                                  \
                JS_FreeValue(ctx, sp[-1]);                                  \
                sp -= 2;                                                    \
                *sp++ = ret_val;                                            \
            }

#define OP_BODY_make_ref(ctx, opcode, sp, pc, var_refs, sf, exception)      \
            {                                                               \
                JSVarRef *var_ref;                                          \
                JSProperty *pr;                                             \
                JSAtom atom;                                                \
                int idx;                                                    \
                atom = get_u32(pc);                                         \
                idx = get_u16(pc + 4);                                      \
                pc += 6;                                                    \
                *sp++ = JS_NewObjectProto(ctx, JS_NULL);                    \
                if (unlikely(JS_IsException(sp[-1])))                       \
                    goto exception;                                         \
                if (opcode == OP_make_var_ref_ref) {                        \
                    var_ref = var_refs[idx];                                \
                    js_rc(var_ref)->ref_count++;                            \
                } else {                                                    \
                    var_ref = get_var_ref(ctx, sf, idx, opcode == OP_make_arg_ref); \
                    if (!var_ref)                                           \
                        goto exception;                                     \
                }                                                           \
                pr = add_property(ctx, JS_VALUE_GET_OBJ(sp[-1]), atom,      \
                                  JS_PROP_WRITABLE | JS_PROP_VARREF);       \
                if (!pr) {                                                  \
                    free_var_ref(rt, var_ref);                              \
                    goto exception;                                         \
                }                                                           \
                pr->u.var_ref = var_ref;                                    \
                *sp++ = goasm_atom_to_value(ctx, atom);                     \
            }

#define OP_BODY_make_var_ref(ctx, sp, pc, save_pc, exception)               \
            {                                                               \
                JSAtom atom;                                                \
                atom = get_u32(pc);                                         \
                pc += 4;                                                    \
                save_pc;                                                    \
                                                                            \
                if (JS_GetGlobalVarRef(ctx, atom, sp))                      \
                    goto exception;                                         \
                sp += 2;                                                    \
            }

#define OP_BODY_get_ref_value(ctx, sp, is_strict, save_pc, exception)       \
            {                                                               \
                JSValue val;                                                \
                JSAtom atom;                                                \
                int ret;                                                    \
                                                                            \
                save_pc;                                                    \
                atom = JS_ValueToAtom(ctx, sp[-1]);                         \
                if (atom == JS_ATOM_NULL)                                   \
                    goto exception;                                         \
                if (unlikely(JS_IsUndefined(sp[-2]))) {                     \
                    JS_ThrowReferenceErrorNotDefined(ctx, atom);            \
                    JS_FreeAtom(ctx, atom);                                 \
                    goto exception;                                         \
                }                                                           \
                ret = JS_HasProperty(ctx, sp[-2], atom);                    \
                if (unlikely(ret <= 0)) {                                   \
                    if (ret < 0) {                                          \
                        JS_FreeAtom(ctx, atom);                             \
                        goto exception;                                     \
                    }                                                       \
                    if (is_strict) {                                        \
                        JS_ThrowReferenceErrorNotDefined(ctx, atom);        \
                        JS_FreeAtom(ctx, atom);                             \
                        goto exception;                                     \
                    }                                                       \
                    val = JS_UNDEFINED;                                     \
                } else {                                                    \
                    val = JS_GetProperty(ctx, sp[-2], atom);                \
                }                                                           \
                JS_FreeAtom(ctx, atom);                                     \
                if (unlikely(JS_IsException(val)))                          \
                    goto exception;                                         \
                sp[0] = val;                                                \
                sp++;                                                       \
            }

#define OP_BODY_put_ref_value(ctx, sp, is_strict, throw_flags, save_pc, exception) \
            {                                                               \
                int ret;                                                    \
                JSAtom atom;                                                \
                save_pc;                                                    \
                atom = JS_ValueToAtom(ctx, sp[-2]);                         \
                if (unlikely(atom == JS_ATOM_NULL))                         \
                    goto exception;                                         \
                if (unlikely(JS_IsUndefined(sp[-3]))) {                     \
                    if (is_strict) {                                        \
                        JS_ThrowReferenceErrorNotDefined(ctx, atom);        \
                        JS_FreeAtom(ctx, atom);                             \
                        goto exception;                                     \
                    } else {                                                \
                        sp[-3] = JS_DupValue(ctx, ctx->global_obj);         \
                    }                                                       \
                }                                                           \
                ret = JS_HasProperty(ctx, sp[-3], atom);                    \
                if (unlikely(ret <= 0)) {                                   \
                    if (unlikely(ret < 0)) {                                \
                        JS_FreeAtom(ctx, atom);                             \
                        goto exception;                                     \
                    }                                                       \
                    if (is_strict) {                                        \
                        JS_ThrowReferenceErrorNotDefined(ctx, atom);        \
                        JS_FreeAtom(ctx, atom);                             \
                        goto exception;                                     \
                    }                                                       \
                }                                                           \
                ret = JS_SetPropertyInternal(ctx, sp[-3], atom, sp[-1], sp[-3], throw_flags); \
                JS_FreeAtom(ctx, atom);                                     \
                JS_FreeValue(ctx, sp[-2]);                                  \
                JS_FreeValue(ctx, sp[-3]);                                  \
                sp -= 3;                                                    \
                if (unlikely(ret < 0))                                      \
                    goto exception;                                         \
            }

/* the body of delete_var, which the register loop runs too
   (engine/regvm-ops-eval.h) */
#define OP_BODY_delete_var(ctx, sp, pc, save_pc, exception)                 \
            {                                                               \
                JSAtom atom;                                                \
                int ret;                                                    \
                                                                            \
                atom = get_u32(pc);                                         \
                pc += 4;                                                    \
                save_pc;                                                    \
                                                                            \
                ret = JS_DeleteGlobalVar(ctx, atom);                        \
                if (unlikely(ret < 0))                                      \
                    goto exception;                                         \
                *sp++ = JS_NewBool(ctx, ret);                               \
            }

/* the body of the with_* ops, which the register loop runs too
   (engine/regvm-ops-eval.h): the name atom looked up in the object at
   sp[-1]. When it is found, the op's result is on the stack and taken is
   set (the caller jumps), when not, the object is dropped. is_strict and
   throw_flags: the mode of the running function, for a missing name and a
   store. */
#define OP_BODY_with(ctx, opcode, sp, atom, is_with, is_strict, throw_flags, taken, exception) \
            {                                                               \
                JSValue obj, val;                                           \
                int ret;                                                    \
                                                                            \
                obj = sp[-1];                                               \
                ret = JS_HasProperty(ctx, obj, atom);                       \
                if (unlikely(ret < 0))                                      \
                    goto exception;                                         \
                if (ret) {                                                  \
                    if (is_with) {                                          \
                        ret = js_has_unscopable(ctx, obj, atom);            \
                        if (unlikely(ret < 0))                              \
                            goto exception;                                 \
                        if (ret)                                            \
                            goto no_with;                                   \
                    }                                                       \
                    switch (opcode) {                                       \
                    case OP_with_get_var:                                   \
                        /* in Object Environment Records, GetBindingValue() calls HasProperty() */ \
                        ret = JS_HasProperty(ctx, obj, atom);               \
                        if (unlikely(ret <= 0)) {                           \
                            if (ret < 0)                                    \
                                goto exception;                             \
                            if (is_strict) {                                \
                                JS_ThrowReferenceErrorNotDefined(ctx, atom); \
                                goto exception;                             \
                            }                                               \
                            val = JS_UNDEFINED;                             \
                        } else {                                            \
                            val = JS_GetProperty(ctx, obj, atom);           \
                            if (unlikely(JS_IsException(val)))              \
                                goto exception;                             \
                        }                                                   \
                        set_value(ctx, &sp[-1], val);                       \
                        break;                                              \
                    case OP_with_put_var: /* used e.g. in for in/of */      \
                        /* in Object Environment Records, SetMutableBinding() calls HasProperty() */ \
                        ret = JS_HasProperty(ctx, obj, atom);               \
                        if (unlikely(ret <= 0)) {                           \
                            if (ret < 0)                                    \
                                goto exception;                             \
                            if (is_strict) {                                \
                                JS_ThrowReferenceErrorNotDefined(ctx, atom); \
                                goto exception;                             \
                            }                                               \
                        }                                                   \
                        ret = JS_SetPropertyInternal(ctx, obj, atom, sp[-2], obj, \
                                                     throw_flags);          \
                        JS_FreeValue(ctx, sp[-1]);                          \
                        sp -= 2;                                            \
                        if (unlikely(ret < 0))                              \
                            goto exception;                                 \
                        break;                                              \
                    case OP_with_delete_var:                                \
                        ret = JS_DeleteProperty(ctx, obj, atom, 0);         \
                        if (unlikely(ret < 0))                              \
                            goto exception;                                 \
                        JS_FreeValue(ctx, sp[-1]);                          \
                        sp[-1] = JS_NewBool(ctx, ret);                      \
                        break;                                              \
                    case OP_with_make_ref:                                  \
                        /* produce a pair object/propname on the stack */   \
                        *sp++ = goasm_atom_to_value(ctx, atom);             \
                        break;                                              \
                    case OP_with_get_ref:                                   \
                        /* produce a pair object/method on the stack */     \
                        /* in Object Environment Records, GetBindingValue() calls HasProperty() */ \
                        ret = JS_HasProperty(ctx, obj, atom);               \
                        if (unlikely(ret < 0))                              \
                            goto exception;                                 \
                        if (!ret) {                                         \
                            val = JS_UNDEFINED;                             \
                        } else {                                            \
                            val = JS_GetProperty(ctx, obj, atom);           \
                            if (unlikely(JS_IsException(val)))              \
                                goto exception;                             \
                        }                                                   \
                        *sp++ = val;                                        \
                        break;                                              \
                    }                                                       \
                    taken = 1;                                              \
                } else {                                                    \
                no_with:                                                    \
                    /* if not jumping, drop the object argument */          \
                    JS_FreeValue(ctx, sp[-1]);                              \
                    sp--;                                                   \
                }                                                           \
            }
