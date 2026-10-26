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
