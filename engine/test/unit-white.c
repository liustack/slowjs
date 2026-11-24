/*
 * Copyright (c) 2026 The liustack/beastjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE at the
 * root of this repository.
 */

/* unit-white checks what the public API cannot reach: the static functions of
   the JSON runs, which it feeds buffers of every room, the marker of a pending
   stack, and the lists the collector keeps. The engine it runs on is white.c,
   which is quickjs.c with the functions below added (build.sh links it in
   place of quickjs.o). They took over the
   tests of JSON, backtraces and the young collection that did this on the
   engine before it.

     unit-white */
#include <errno.h>
#include <limits.h>
#include <spawn.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

#include "compile-scripts.h"
#include "testutil.h"

extern char **environ;

/* ---- a case in a process of its own ----

   run_self runs this program again with args (it sets args[0]) and gives the
   status the program ended with. A case that may abort runs so, and is then
   one failed case, not the end of the test. It is a new program, not a fork:
   the watchdog thread of host.c holds host_lock for a moment once a second,
   and the child of a fork made in that moment waits for the lock forever. The
   child of a fork would also have no watchdog. */
static const char *self_path;

static int run_self(char **args)
{
    pid_t pid, r;
    int st, err;

    args[0] = (char *)self_path;
    fflush(stdout);
    err = strchr(self_path, '/') ? posix_spawn(&pid, self_path, NULL, NULL, args, environ)
                                 : posix_spawnp(&pid, self_path, NULL, NULL, args, environ);
    if (err) {
        fprintf(stderr, "posix_spawn %s: %s\n", self_path, strerror(err));
        exit(2);
    }
    while ((r = waitpid(pid, &st, 0)) < 0 && errno == EINTR)
        ;
    if (r != pid) {
        perror("waitpid");
        exit(2);
    }
    return st;
}

struct StringBuffer;
struct StringBuffer *white_sb_new(JSContext *ctx, int size, int wide, int pre);
void white_sb_free(JSContext *ctx, struct StringBuffer *b);
int white_sb_units(const struct StringBuffer *b, uint16_t *out, int cap);
int white_sb_error(const struct StringBuffer *b);
void white_sb_fake_length(struct StringBuffer *b, int len, int size);
int white_string_len_max(void);
const uint8_t *white_json_string_run(struct StringBuffer *b, const uint8_t *p, const uint8_t *end, int sep);
int white_json_quote_run(struct StringBuffer *b, JSValueConst s, int *pi);
uintptr_t white_json_run_limit(uintptr_t p, uintptr_t end);
const uint8_t *white_json_plain_run(const uint8_t *p, const uint8_t *end);
int white_json_count_high_bytes(const uint8_t *p, int n);
void white_json_walk(int on);
unsigned long white_json_walks(void);
int white_atom_refs(JSContext *ctx, const char *s);
int white_ref_count(JSValueConst v);
int white_emit_tables(JSContext *ctx, int size[3], int count[3]);
int white_string_slack(JSValueConst v);
int white_emit_chunks_slack(JSContext *ctx);
int white_emit_buf(JSContext *ctx, int op);
int white_stack_pending(JSContext *ctx, JSValueConst obj);
int white_backtrace_class_has_handlers(JSRuntime *rt);
int white_pending_needs_data_property(JSContext *ctx, JSValueConst err);
int64_t white_backtrace_frame_size(void);
void white_gc_lists(JSRuntime *rt, int *old, int *young, int *bad);
int white_gc_outside(JSRuntime *rt, int record, int *n);
void white_young_gc_failing(JSRuntime *rt, int fail, int *nallocs);
int white_error_nest(JSRuntime *rt, int n);
JSValue white_bare_function(JSContext *ctx, JSValueConst proto);
int white_function_set_properties(JSContext *ctx, JSValueConst f, JSAtom name, int len);
int white_own_keys(JSContext *ctx, JSValueConst obj, int flags, int got, char *out, size_t cap);
int white_function_debug(JSValueConst script, int i, const uint8_t **source, size_t *source_len, const uint8_t **pc2line, int *shared);
int white_regexp_cache(JSRuntime *rt, char *out, size_t cap, size_t *bytes, uint32_t *buckets);
int white_regexp_standard(JSContext *ctx, const char *name);
int white_lre_split_budget(void);
int white_lre_chains(const char *re, int flags, void *opaque, char *out, size_t n);
int white_lre_literal(const char *re, int flags, void *opaque, char *out, size_t n);
int white_lre_plan_size(const char *re, int flags, void *opaque, int *rest, char *names, size_t n);
int white_lre_plan(const char *re, int flags, void *opaque, char *out, size_t n);
int white_lre_split_same(const char *re, int flags, void *opaque, const uint32_t *units, int n, long *calls);
JSValue white_compile_failing(JSContext *ctx, const char *js, int fail, int mode, int *nallocs);
JSValue white_read_failing(JSContext *ctx, const uint8_t *buf, size_t len, int fail, int *nallocs);
int white_op(int i);
int white_op_count(void);
void white_days_to_date(int64_t days, int *y, int *m, int *d);
int white_poll_run(void);
void white_poll_skip(int on);
int white_compile_count(JSContext *ctx, int left, int n, int times);
const char *white_poll_scan(const uint8_t *code, int len);
int white_poll_insert(JSContext *ctx, const uint8_t *code, int len, uint8_t *out, int cap);
int white_code_shape(const uint8_t *code, int len, int *shape, int max);
const uint8_t *white_child_code(JSValueConst fun, int i, int *len);
int white_child_var_count(JSValueConst fun, int i);
int white_by_value(JSContext *ctx, const char *src, const char *name, int read, char *out, size_t cap);
int white_by_value_crafted(JSContext *ctx, const char *src, const char *name, int craft, char *out, size_t cap);
int white_wide_closure_vars(JSContext *ctx, JSValueConst f, const char *name, int n, int on);
void white_marks(JSContext *ctx, JSValueConst f, char *out, size_t cap);
int white_flip_by_value(JSContext *ctx, JSValueConst f, const char *name, int was, int at);
int white_class_after_nop(JSContext *ctx, const char *src, const char *name);
int white_nops(const uint8_t *code, int len, int *places, int max);
int white_resolve_var(JSContext *ctx, int kind, int in_arrow, size_t room, int *pending, int *made);
int white_eval_vars(JSContext *ctx, int in_arrow, size_t room, int *pending, int *made);
int white_string_is_rope(JSValueConst v);
int white_string_room(JSContext *ctx, JSValueConst v);
int white_string_wide(JSValueConst v);
JSValue white_wide_latin1(JSContext *ctx, const char *s);
int white_join_plan(JSContext *ctx, JSValueConst a, JSValueConst sep, int64_t *len, int *wide);
JSValue white_concat(JSContext *ctx, JSValueConst a, JSValueConst b);
int white_every_block_is_a_request(void);
int white_fused_ops(const uint8_t *code, int len, int *count, size_t cap);
int white_short_index_check(char *bad, size_t cap);
size_t white_object_block(JSContext *ctx, JSValueConst obj, size_t *full);
int white_props_inline(JSValueConst obj, int *room);
uintptr_t white_shape_of(JSValueConst obj);
int white_copy_site(JSValueConst fn, int site, uintptr_t *src, uintptr_t *tgt, int max,
                    int *swaps);
unsigned long white_shape_id_hits(void);
uint32_t white_shape_id_next(JSRuntime *rt);
uint32_t white_shape_id_max(void);
void white_shape_id_set(JSRuntime *rt, uint32_t next);
int white_shape_ids_bad(JSRuntime *rt, int *count, int *parented);
int white_shape_id_twins(JSRuntime *rt);
int white_shape_hashed(JSValueConst obj);
int white_shape_parents_bad(JSRuntime *rt, JSValueConst proto, int *count, int *parented);
uint32_t white_key_atom(JSValueConst v);
int white_string_plain(JSContext *ctx, JSValueConst v);
long white_call_debt(JSContext *ctx, int direct, int *depth);
int white_key_atoms_bad(JSRuntime *rt, int *live, int *keys);
unsigned long white_gc_run_count(int young);
int white_block_gc(JSRuntime *rt, size_t size);
unsigned white_block_wait(JSRuntime *rt, unsigned *skip);
unsigned long white_module_steps_take(void);
uint32_t white_module_names(JSContext *ctx, int *bits);
JSValue white_call_failing(JSContext *ctx, JSValueConst func, JSValueConst this_val, int argc, JSValueConst *argv, int fail, int *nallocs);
JSValue white_eval_function_failing(JSContext *ctx, JSValue m, int fail, int *nallocs);
int white_jobs_failing(JSRuntime *rt, JSContext **pctx, int fail, int *nallocs);
int white_regvm_convert_failing(JSContext *ctx, JSValueConst f, int fail, int mode, int *nallocs);
uint32_t white_regvm_code_hash(JSValueConst f, int *code_len);
const uint8_t *white_regvm_code(JSValueConst f, int *code_len);
long white_regvm_shorts(void);
int white_regvm_scratch_drain(JSContext *ctx, int in_pass);
int white_crafted(JSContext *ctx, JSValueConst f, int kind, int undo);
int white_crafted_code(JSContext *ctx, JSValueConst f, int kind, int undo);
int white_regvm_read_check(JSContext *ctx, JSValueConst f);
int white_regvm_dst_sites(JSValueConst f, int *dst, int *win);
int white_regvm_forin_sites(JSValueConst f, int n[4]);
int white_regvm_getf_slots(JSValueConst f, uint32_t *slots, uint32_t *ids, int max);
int white_regvm_putf_sites(JSValueConst f, uint32_t *s, int max);
uint32_t white_shape_step_epoch(JSRuntime *rt);
void white_shape_step_epoch_set(JSRuntime *rt, uint32_t e);
int white_regvm_call_record(JSValueConst f);
int white_func_class_objects(JSContext *ctx);
int white_regvm_stack(JSContext *ctx, long out[7]);
int white_set_code(JSContext *ctx, JSValueConst f, const uint8_t *code, int len, int stack_size, int undo);
uint32_t white_array_fake_count(JSContext *ctx, JSValueConst a, uint32_t count);
int64_t white_array_room(JSValueConst a);
int64_t white_shape_fake_count(JSValueConst o, uint32_t count);
uint32_t white_shape_prop_max(void);
void white_set_array_from_first(long long k);
int white_drop_vardefs(JSValueConst f, int on);
int white_opcode(int which);
int white_regvm_oom_pending(JSContext *ctx);
void white_work_reset(void);
int white_jsstring_header(void);
int white_sb_write8(struct StringBuffer *b, const uint8_t *p, int len);
int white_sb_putc16(struct StringBuffer *b, uint32_t c);
unsigned long white_work_longest(void);
unsigned long white_work_sum(void);
void white_work_add(unsigned long n);
size_t white_sb_block_usable(const struct StringBuffer *b);
void white_front_drain(JSContext *ctx);
size_t white_heap_limit(JSContext *ctx);
size_t white_gc_count(JSRuntime *rt);
void white_drain_once(void);
void white_drain_once_then(void (*f)(void *), void *arg);
void white_drain_once_refused(void);
int white_kept_shapes(JSRuntime *rt);
int white_shape_cache_size(JSRuntime *rt);
JSClassID white_class_object(void);
void *white_kept_shape(JSRuntime *rt, size_t *size);
int white_take_slots(JSRuntime *rt, size_t size, void **out, int cap);
size_t white_found_resize(JSContext *ctx, JSValueConst obj, const char *key);
int white_array_grow(JSContext *ctx, JSValueConst arr, uint32_t new_len);
int white_being_freed(JSValueConst obj);
void white_gc_failing(JSRuntime *rt, int young, int fail, int *nallocs, int *early);
int white_kept_young_shape(JSRuntime *rt, JSValueConst obj);
long white_gc_window_count(void);

/* ---- texts of every kind of character ---- */

/* A text is code points; text_utf8 writes it as UTF-8 and text_utf16 as the
   code units a string of the engine holds. */
typedef struct {
    uint32_t cp[700];
    int n;
} Text;

static void text_add(Text *t, uint32_t c, int times)
{
    while (times-- > 0)
        t->cp[t->n++] = c;
}

static size_t text_utf8(const Text *t, uint8_t *out)
{
    size_t len = 0;
    int i;

    for (i = 0; i < t->n; i++) {
        uint32_t c = t->cp[i];

        if (c < 0x80) {
            out[len++] = c;
        } else if (c < 0x800) {
            out[len++] = 0xc0 | (c >> 6);
            out[len++] = 0x80 | (c & 63);
        } else if (c < 0x10000) {
            out[len++] = 0xe0 | (c >> 12);
            out[len++] = 0x80 | ((c >> 6) & 63);
            out[len++] = 0x80 | (c & 63);
        } else {
            out[len++] = 0xf0 | (c >> 18);
            out[len++] = 0x80 | ((c >> 12) & 63);
            out[len++] = 0x80 | ((c >> 6) & 63);
            out[len++] = 0x80 | (c & 63);
        }
    }
    return len;
}

static int text_utf16(const Text *t, uint16_t *out)
{
    int len = 0, i;

    for (i = 0; i < t->n; i++) {
        uint32_t c = t->cp[i];

        if (c >= 0x10000) {
            out[len++] = 0xd800 + ((c - 0x10000) >> 10);
            out[len++] = 0xdc00 + (c & 0x3ff);
        } else {
            out[len++] = c;
        }
    }
    return len;
}

#define A 'a'
#define B 'b'
#define E_ACUTE 0xe9
#define HAN 0x6f22
#define EMOJI 0x1f600

/* the strings of the run tests: ASCII, Latin-1, wide characters and a
   surrogate pair, at the lengths around the room of the buffers */
static const int lengths[] = { 0, 1, 2, 7, 8, 9, 15, 16, 17, 30, 31, 32, 33, 34, 47, 48, 49, 63, 64, 65, 66, 100, 300 };

static void run_text(Text *t, int length_index, int kind)
{
    int n = lengths[length_index];

    t->n = 0;
    switch (kind) {
    case 0: text_add(t, A, n); break;
    case 1: text_add(t, E_ACUTE, n); break;
    case 2: text_add(t, A, 1); text_add(t, B, 1); text_add(t, HAN, n); break;
    case 3: text_add(t, A, n); text_add(t, E_ACUTE, 1); break;
    case 4: text_add(t, A, n); text_add(t, HAN, 1); text_add(t, B, n); break;
    default: text_add(t, A, n); text_add(t, EMOJI, 1); break;
    }
}

static const int sizes[] = { 1, 2, 3, 8, 16, 31, 32, 33, 64 };

/* ---- the runs of JSON ---- */

/* Every run of JSON.parse, into a buffer of every room and kind, leaves the
   characters of the string in it as string_buffer_putc would, stops at the
   quote and never fills the buffer past its room. */
static void test_json_string_run(void)
{
    VM v = vm_new();
    size_t si, li;
    int pi, wide, kind;
    static uint8_t src[4000];
    static uint16_t want[1000], got[1000];

    for (si = 0; si < countof(sizes); si++) {
        int size = sizes[si];
        int pres[4] = { 0, 1, size - 1, size };

        for (pi = 0; pi < 4; pi++) {
            for (wide = 0; wide < 2; wide++) {
                for (li = 0; li < countof(lengths); li++) {
                    for (kind = 0; kind < 6; kind++) {
                        Text t;
                        struct StringBuffer *b;
                        size_t n8;
                        int nw, ng, k;
                        const uint8_t *r;

                        run_text(&t, li, kind);
                        n8 = text_utf8(&t, src);
                        src[n8] = '"';
                        b = white_sb_new(v.ctx, size, wide, pres[pi]);
                        CHECK(b != NULL, "no buffer");
                        if (!b)
                            continue;
                        r = white_json_string_run(b, src, src + n8 + 1, '"');
                        CHECK(r == src + n8, "size %d, %d before, wide %d, kind %d, %d: stops %ld bytes on, want %zu", size, pres[pi], wide, kind, lengths[li],
                              r ? (long)(r - src) : -1L, n8);
                        for (k = 0; k < pres[pi]; k++)
                            want[k] = 'x';
                        nw = pres[pi] + text_utf16(&t, want + pres[pi]);
                        ng = white_sb_units(b, got, 1000);
                        CHECK(ng == nw && memcmp(got, want, nw * sizeof(uint16_t)) == 0,
                              "size %d, %d before, wide %d, kind %d, %d: holds %d units, want %d", size, pres[pi], wide, kind, lengths[li], ng, nw);
                        white_sb_free(v.ctx, b);
                    }
                }
            }
        }
    }
    vm_free(&v);
}

/* Every run of JSON.stringify, from a string of 8 or 16 bits into a buffer of
   every room and kind, leaves the characters up to the first one to escape in
   it and never fills the buffer past its room. */
static void test_json_quote_run(void)
{
    static const char *const tails[] = { "", "\"z", "\n" };
    VM v = vm_new();
    size_t si, li, ti;
    int pi, wide, kind;
    static uint8_t src[4000];
    static uint16_t want[1000], got[1000];

    for (si = 0; si < countof(sizes); si++) {
        int size = sizes[si];
        int pres[4] = { 0, 1, size - 1, size };

        for (pi = 0; pi < 4; pi++) {
            for (wide = 0; wide < 2; wide++) {
                for (li = 0; li < countof(lengths); li++) {
                    for (kind = 0; kind < 6; kind++) {
                        for (ti = 0; ti < countof(tails); ti++) {
                            Text t, kept;
                            struct StringBuffer *b;
                            size_t n8;
                            JSValue str;
                            int i = 0, nw, ng, k;

                            run_text(&t, li, kind);
                            n8 = text_utf8(&t, src);
                            memcpy(src + n8, tails[ti], strlen(tails[ti]));
                            str = JS_NewStringLen(v.ctx, (const char *)src, n8 + strlen(tails[ti]));
                            CHECK(JS_IsString(str), "no string");
                            b = white_sb_new(v.ctx, size, wide, pres[pi]);
                            if (!b || !JS_IsString(str)) {
                                CHECK(0, "no buffer");
                                continue;
                            }
                            CHECK(white_json_quote_run(b, str, &i) == 0, "size %d, %d before, wide %d, kind %d: the run failed", size, pres[pi], wide, kind);
                            /* the run stops at the surrogate pair, which the loop writes */
                            kept = t;
                            for (k = 0; k < kept.n; k++)
                                if (kept.cp[k] == EMOJI) {
                                    kept.n = k;
                                    break;
                                }
                            for (k = 0; k < pres[pi]; k++)
                                want[k] = 'x';
                            nw = pres[pi] + text_utf16(&kept, want + pres[pi]);
                            CHECK(i == nw - pres[pi], "size %d, %d before, wide %d, kind %d, %d, tail %zu: stops at %d, want %d", size, pres[pi], wide, kind, lengths[li], ti, i, nw - pres[pi]);
                            ng = white_sb_units(b, got, 1000);
                            CHECK(ng == nw && memcmp(got, want, nw * sizeof(uint16_t)) == 0, "size %d, %d before, wide %d, kind %d, %d: holds %d units, want %d", size, pres[pi], wide, kind, lengths[li], ng, nw);
                            white_sb_free(v.ctx, b);
                            JS_FreeValue(v.ctx, str);
                        }
                    }
                }
            }
        }
    }
    vm_free(&v);
}

/* The blocks fill_memory took, to give back. */
static struct {
    void **p;
    size_t n, cap;
} taken;

static void fill_memory(VM *v)
{
    size_t size;

    JS_RunGC(v->rt);
    JS_SetMemoryLimit(v->rt, JS_GetMemorySize(v->rt));
    for (size = 4096; size >= 8; size -= 8) {
        for (;;) {
            void *p = js_malloc_rt(v->rt, size);

            if (!p)
                break;
            if (taken.n == taken.cap) {
                taken.cap = taken.cap ? taken.cap * 2 : 1024;
                taken.p = realloc(taken.p, taken.cap * sizeof(void *));
            }
            taken.p[taken.n++] = p;
        }
    }
}

static void release_memory(VM *v)
{
    while (taken.n)
        js_free_rt(v->rt, taken.p[--taken.n]);
    unlimit(v);
}

/* A run that needs more memory than is left fails as string_buffer_putc does,
   with the out of memory error: one that grows an 8-bit buffer, and a first
   wide character that widens a buffer with room left, for both runs. */
static void test_json_runs_oom(void)
{
    static const struct { const char *name; uint32_t cp; int count, pre; } cases[] = {
        { "grow", 'a', 100, 0 },
        { "widen", HAN, 1, 2 },
    };
    VM v = vm_new();
    size_t c;
    int quote;

    for (c = 0; c < countof(cases); c++) {
        for (quote = 0; quote < 2; quote++) {
            Text t;
            uint8_t src[256];
            struct StringBuffer *b = white_sb_new(v.ctx, 32, 0, cases[c].pre);
            JSValue str;
            size_t n8;
            int i = 0, failed;
            char *ex;
            int unc;

            t.n = 0;
            text_add(&t, cases[c].cp, cases[c].count);
            n8 = text_utf8(&t, src);
            src[n8] = '"';
            str = JS_NewStringLen(v.ctx, (const char *)src, n8);
            fill_memory(&v);
            if (quote)
                failed = white_json_quote_run(b, str, &i) != 0;
            else
                failed = white_json_string_run(b, src, src + n8 + 1, '"') == NULL;
            release_memory(&v);
            CHECK(failed, "%s, quote %d: the run did not fail", cases[c].name, quote);
            unc = JS_IsUncatchableException(v.ctx);
            ex = exception_text(v.ctx);
            CHECK(ex != NULL && unc, "%s, quote %d: no out of memory error", cases[c].name, quote);
            free(ex);
            CHECK(white_sb_error(b) != 0, "%s, quote %d: the buffer has no error", cases[c].name, quote);
            white_sb_free(v.ctx, b);
            JS_FreeValue(v.ctx, str);
        }
    }
    vm_free(&v);
}

/* An ASCII run of JSON.parse ends JS_STRING_LEN_MAX + 1 bytes on at the
   latest, so its length fits an int. The addresses are made up: no run of a
   gigabyte is read. */
static void test_json_run_limit(void)
{
    uintptr_t p = 0x1000, max = (uintptr_t)white_string_len_max();
    struct { uintptr_t end, want; } cases[] = {
        { p, p },
        { p + 1, p + 1 },
        { p + max, p + max },
        { p + max + 1, p + max + 1 },
        { p + max + 2, p + max + 1 },
        { p + ((uintptr_t)3 << 30), p + max + 1 },
        { p + ((uintptr_t)1 << 32) - 1, p + max + 1 },
        { p + ((uintptr_t)1 << 32) + 5, p + max + 1 },
    };
    size_t i;

    for (i = 0; i < countof(cases); i++) {
        uintptr_t got = white_json_run_limit(p, cases[i].end);

        CHECK(got == cases[i].want, "json_run_limit(%#lx, %#lx) = %#lx, want %#lx",
              (unsigned long)p, (unsigned long)cases[i].end, (unsigned long)got,
              (unsigned long)cases[i].want);
    }
}

/* A run that would make the string longer than the longest string throws
   "string too long", as string_buffer_putc did. The buffer is made to look
   that long, so no gigabyte of input is needed. */
static void test_json_run_too_long(void)
{
    static const uint8_t text[] = "aaaaaaaaaa\"";
    VM v = vm_new();
    int wide;

    for (wide = 0; wide < 2; wide++) {
        struct StringBuffer *b = white_sb_new(v.ctx, 32, wide, 0);
        int max = white_string_len_max();
        const uint8_t *r;
        char *ex;

        white_sb_fake_length(b, max - 3, max - 3);
        r = white_json_string_run(b, text, text + sizeof(text) - 1, '"');
        CHECK(r == NULL, "wide %d: the run did not fail", wide);
        ex = exception_text(v.ctx);
        CHECK(ex && strcmp(ex, "InternalError: string too long") == 0, "wide %d: %s, want InternalError: string too long", wide, ex ? ex : "no exception");
        free(ex);
        white_sb_fake_length(b, 0, 32);
        white_sb_free(v.ctx, b);
    }
    vm_free(&v);
}
/* json_plain_run against a loop of one byte at a time: every length up to 40,
   a byte that ends the run at every place or none, and random bytes, in
   blocks of the exact length (so that a read past the end shows under
   AddressSanitizer), and a run that stops 65535 bytes on. */
static size_t plain_run_bytes(const uint8_t *p, size_t len)
{
    size_t i, lim = len > 65535 ? 65535 : len;

    for (i = 0; i < lim; i++) {
        if (p[i] < 0x20 || p[i] >= 0x80 || p[i] == '"' || p[i] == '\\')
            break;
    }
    return i;
}

static void test_json_plain_run(void)
{
    static const uint8_t stops[] = { 0x00, 0x01, 0x1f, '"', '\\', 0x80, 0xc3, 0xff };
    static const uint8_t plains[] = { 0x20, 0x21, 0x23, 0x5b, 0x5d, 0x7e, 0x7f, 'a', '0', ':' };
    uint32_t seed = 12345;
    size_t len, pos, s, i;
    uint8_t *buf;

    for (len = 0; len <= 40; len++) {
        for (pos = 0; pos <= len; pos++) {
            for (s = 0; s < countof(stops); s++) {
                buf = malloc(len ? len : 1);
                for (i = 0; i < len; i++)
                    buf[i] = plains[(i * 7 + len + s) % countof(plains)];
                if (pos < len)
                    buf[pos] = stops[s];
                CHECK((size_t)(white_json_plain_run(buf, buf + len) - buf) == plain_run_bytes(buf, len),
                      "plain run of %zu bytes with 0x%02x at %zu: %zu, want %zu", len, stops[s], pos,
                      (size_t)(white_json_plain_run(buf, buf + len) - buf), plain_run_bytes(buf, len));
                free(buf);
                if (pos == len)
                    break;
            }
        }
    }
    for (i = 0; i < 20000; i++) {
        size_t n, k;

        seed = seed * 1103515245 + 12345;
        n = (seed >> 16) % 64;
        buf = malloc(n ? n : 1);
        for (k = 0; k < n; k++) {
            seed = seed * 1103515245 + 12345;
            /* mostly plain bytes, so that runs are long */
            buf[k] = (seed >> 16) % 16 ? 0x20 + (seed >> 8) % 0x60 : (seed >> 8) & 0xff;
        }
        CHECK((size_t)(white_json_plain_run(buf, buf + n) - buf) == plain_run_bytes(buf, n),
              "random plain run %zu of %zu bytes", i, n);
        free(buf);
    }
    len = 70000;
    buf = malloc(len);
    memset(buf, 'a', len);
    CHECK(white_json_plain_run(buf, buf + len) - buf == 65535, "a run longer than the step");
    buf[65534] = '"';
    CHECK(white_json_plain_run(buf, buf + len) - buf == 65534, "a quote at the last byte of the step");
    buf[65534] = 'a';
    buf[65535] = '"';
    CHECK(white_json_plain_run(buf, buf + len) - buf == 65535, "a quote just past the step");
    free(buf);
}

/* json_count_high_bytes against a loop of one byte at a time, at every
   offset from a word and length up to 70 */
static void test_json_count_high_bytes(void)
{
    uint32_t seed = 777;
    uint8_t *buf = malloc(80);
    int off, n, k, round;

    for (round = 0; round < 50; round++) {
        for (k = 0; k < 80; k++) {
            seed = seed * 1103515245 + 12345;
            buf[k] = (seed >> 16) % (round % 4 + 2) ? (seed >> 8) % 0x80 : 0x80 | (seed >> 8);
        }
        for (off = 0; off < 8; off++) {
            for (n = 0; n + off <= 78; n++) {
                int want = 0;

                for (k = 0; k < n; k++)
                    want += buf[off + k] >> 7;
                CHECK(white_json_count_high_bytes(buf + off, n) == want, "round %d, %d bytes at %d: %d, want %d",
                      round, n, off, white_json_count_high_bytes(buf + off, n), want);
            }
        }
    }
    free(buf);
}

/* JSON.parse keeps no reference to the atoms of the keys it read once it
   returns: after a parse that works, one that throws a SyntaxError near the
   end, one with a reviver, and one with each call of the allocator failing in
   turn, the atom of a key only the text has is gone once the result is, and
   the keys the script holds have their references of before. The text has
   rows of keys in changing orders, more keys than the cache has slots, keys
   that share a slot (abcde and abcxe), an escaped key equal to a plain one
   and keys repeated in one row. Each failing call also stringifies what it
   parsed and compares it with the text the setup made. */
static void test_json_key_cache(void)
{
    static const char setup[] =
        "var only = 'qq_' + 'json' + '_only', keys = ['abcde', 'abcxe', 'id', 'name', 'k5', only]\n"
        "for (var j = 0; j < 90; j++) keys.push('k' + j)\n"
        "var parts = []\n"
        "for (var i = 0; i < 120; i++) {\n"
        "  var o = [], n = 3 + i % 7\n"
        "  for (var j = 0; j < n; j++) o.push(JSON.stringify(keys[(i * 3 + j * 7) % keys.length]) + ':' + i * j)\n"
        "  if (i % 5 == 0) o.push('\"n\\\\u0061me\":\"e\"')\n"
        "  if (i % 11 == 0) o.push('\"abcde\":\"dup\"')\n"
        "  o.push(JSON.stringify(only) + ':' + i)\n"
        "  parts.push('{' + o.join(',') + '}')\n"
        "}\n"
        "var text = '[' + parts.join(',\\n') + ']', bad = text.slice(0, -40) + '}x', held = { abcde: 1, k5: 2 }\n"
        "var want = JSON.stringify(JSON.parse(text))\n"
        "globalThis.f = () => JSON.stringify(JSON.parse(text)) === want; 0";
    static const char *const held[] = { "abcde", "abcxe", "k5", "k77" };
    static const char *const calls[] = {
        "JSON.parse(text).length",
        "try { JSON.parse(bad) } catch (e) { e.name }",
        "JSON.parse(text, function (k, v) { return v }).length",
    };
    VM v = vm_new();
    int before[countof(held)], k, fail, n = 0, ooms = 0;
    size_t c;
    JSValue f, g;

    JS_SetShapeCache(v.ctx, 0);
    CHECK(eval_ok(v.ctx, setup), "the setup threw");
    JS_RunGC(v.rt);
    for (k = 0; k < (int)countof(held); k++)
        before[k] = white_atom_refs(v.ctx, held[k]);
    CHECK(before[0] > 0, "abcde has no atom");
    CHECK(white_atom_refs(v.ctx, "qq_json_only") == -1, "the key only the text has has an atom before the parse");
    for (c = 0; c < countof(calls); c++) {
        CHECK(eval_ok(v.ctx, calls[c]), "%s threw", calls[c]);
        JS_RunGC(v.rt);
        CHECK(white_atom_refs(v.ctx, "qq_json_only") == -1, "%s: the key only the text has keeps its atom", calls[c]);
        for (k = 0; k < (int)countof(held); k++)
            CHECK(white_atom_refs(v.ctx, held[k]) == before[k], "%s: %s has %d references, want %d", calls[c], held[k],
                  white_atom_refs(v.ctx, held[k]), before[k]);
    }
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "f");
    JS_FreeValue(v.ctx, g);
    for (fail = 0; fail == 0 || fail <= n; fail++) {
        int made;
        JSValue r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);

        if (fail == 0)
            n = made;
        if (JS_IsException(r)) {
            CHECK(fail > 0 && JS_IsUncatchableException(v.ctx), "call %d failing: an exception that is not out of memory", fail);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            ooms++;
        } else {
            CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_BOOL && JS_VALUE_GET_INT(r) == 1, "call %d failing: another result", fail);
        }
        JS_RunGC(v.rt);
        CHECK(white_atom_refs(v.ctx, "qq_json_only") == -1, "call %d failing: the key only the text has keeps its atom", fail);
        for (k = 0; k < (int)countof(held); k++)
            CHECK(white_atom_refs(v.ctx, held[k]) == before[k], "call %d failing: %s has %d references, want %d", fail,
                  held[k], white_atom_refs(v.ctx, held[k]), before[k]);
    }
    CHECK(ooms > 100, "only %d of %d calls failed the parse", ooms, n);
    JS_FreeValue(v.ctx, f);
    vm_free(&v);
}

/* JSON.stringify of an array reads each element, then converts its index to
   a string for js_json_check. When that conversion runs out of memory, the
   element is freed: it used to stay, with its shape and the atoms of its
   keys, until the runtime was freed. Each call of the allocator fails in turn
   on an array of objects, arrays and strings made for the call, and after
   each the runtime holds as many objects and strings as before it. */
static void test_json_stringify_oom(void)
{
    static const char setup[] =
        "globalThis.f = (n) => JSON.stringify([{ a: 1, ['k' + n]: 's' + n }, [2, { b: 'x'.repeat(n) }], 'v' + n, {}])\n"
        "f(40)";
    static const char want[] =
        "[{\"a\":1,\"k40\":\"s40\"},[2,{\"b\":\"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\"}],\"v40\",{}]";
    VM v = vm_new();
    JSValue g, f, arg = JS_NewInt32(v.ctx, 40);
    int fail, n = 0, ooms = 0;

    CHECK(eval_ok(v.ctx, setup), "the setup threw");
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "f");
    JS_FreeValue(v.ctx, g);
    for (fail = 0; fail == 0 || fail <= n; fail++) {
        JSMemoryUsage before, after;
        int made;
        JSValue r;

        JS_RunGC(v.rt);
        JS_ComputeMemoryUsage(v.rt, &before);
        r = white_call_failing(v.ctx, f, JS_UNDEFINED, 1, &arg, fail, &made);
        if (fail == 0)
            n = made;
        if (JS_IsException(r)) {
            CHECK(fail > 0 && JS_IsUncatchableException(v.ctx), "call %d failing: an exception that is not out of memory", fail);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            ooms++;
        } else {
            const char *s = JS_ToCString(v.ctx, r);

            CHECK(s && !strcmp(s, want), "call %d failing: %s", fail, s ? s : "(no string)");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, r);
        }
        JS_RunGC(v.rt);
        JS_ComputeMemoryUsage(v.rt, &after);
        CHECK(after.obj_count == before.obj_count && after.str_count == before.str_count,
              "call %d failing: %lld objects and %lld strings, before it %lld and %lld", fail,
              (long long)after.obj_count, (long long)after.str_count, (long long)before.obj_count,
              (long long)before.str_count);
    }
    CHECK(ooms > 10, "only %d of %d calls failed the stringify", ooms, n);
    JS_FreeValue(v.ctx, f);
    vm_free(&v);
}

/* each_call_failing runs setup, then calls its global f with each call of the
   allocator failing in turn. Each call either throws the out of memory error
   or returns want, and leaves as many objects and strings as there were
   before it. At least min_ooms of the calls fail. */
static void each_call_failing(const char *what, const char *setup, const char *want, int min_ooms)
{
    VM v = vm_new();
    JSValue g, f;
    int fail, n = 0, ooms = 0;

    CHECK(eval_ok(v.ctx, setup), "%s: the setup threw", what);
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "f");
    JS_FreeValue(v.ctx, g);
    for (fail = 0; fail == 0 || fail <= n; fail++) {
        JSMemoryUsage before, after;
        int made;
        JSValue r;

        JS_RunGC(v.rt);
        JS_ComputeMemoryUsage(v.rt, &before);
        r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);
        if (fail == 0)
            n = made;
        if (JS_IsException(r)) {
            CHECK(fail > 0 && JS_IsUncatchableException(v.ctx), "%s, call %d failing: an exception that is not out of memory", what, fail);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            ooms++;
        } else {
            const char *s = JS_ToCString(v.ctx, r);

            CHECK(s && !strcmp(s, want), "%s, call %d failing: %s", what, fail, s ? s : "(no string)");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, r);
        }
        CHECK(!JS_HasException(v.ctx), "%s, call %d failing: an exception is pending", what, fail);
        JS_RunGC(v.rt);
        JS_ComputeMemoryUsage(v.rt, &after);
        CHECK(after.obj_count == before.obj_count && after.str_count == before.str_count,
              "%s, call %d failing: %lld objects and %lld strings, before it %lld and %lld", what, fail,
              (long long)after.obj_count, (long long)after.str_count, (long long)before.obj_count,
              (long long)before.str_count);
    }
    CHECK(ooms >= min_ooms, "%s: only %d of %d calls failed", what, ooms, n);
    JS_FreeValue(v.ctx, f);
    vm_free(&v);
}

/* An array made with a prototype that is not that of arrays starts from the
   empty shape of that prototype and adds length. When that shape is shared,
   held by another object, adding length copies it, and when the copy runs out of memory the
   construction throws the out of memory error: it used to write the length
   through the null it got. Setting the prototype of an object whose shape is
   shared copies the shape too, and when that runs out of memory the new
   prototype keeps no reference more: it used to stay alive with the one taken
   before the copy. */
static void test_shared_shape_oom(void)
{
    each_call_failing("an array with the prototype of Map",
                      "globalThis.keep = new Map()\n"
                      "function F() {}\n"
                      "F.prototype = Map.prototype\n"
                      "globalThis.f = () => { const a = Reflect.construct(Array, [], F)\n"
                      "  return [Object.getPrototypeOf(a) === Map.prototype, a.length, Array.isArray(a)].join() }\n"
                      "f(); f()",
                      "true,0,true", 1);
    each_call_failing("the prototype of a new Map set",
                      "globalThis.keep = new Map()\n"
                      "globalThis.f = () => { const p = { x: 1 }, m = new Map()\n"
                      "  Object.setPrototypeOf(m, p)\n"
                      "  return [Object.getPrototypeOf(m) === p, m.x, m.size].join() }\n"
                      "f(); f()",
                      "true,1,", 1);
}

/* JSON.stringify with an array as its replacer copies the names of that
   array into a list of its own. When adding a name to the list runs out of
   memory, the stringify fails with the out of memory error: it used to go
   on without that name and return a string that lacked its property, with
   the error still pending. Each call of the allocator fails in turn, and
   each call either throws the out of memory error or returns the whole
   string with no error pending, and leaves as many objects and strings as
   there were before it. */
/* The walks of JSON.stringify against its general path. First which objects
   and arrays the walks take. Then values made at random from a seed:
   nested objects and arrays of numbers, strings with escapes and lone
   surrogates, keys that are indexes or need escapes, values left out,
   getters, toJSON methods (own, on a prototype, added to Object.prototype
   and Array.prototype while the stringify runs), Dates, boxed primitives,
   proxies, BigInts, holes, cycles, and code that deletes, adds and changes
   keys and elements of the objects being written. Each value is made and
   written twice from its seed, with the walks and without them: the result
   or the error and what the code of the value saw, in its order, must be the
   same. The code draws from the same generator, so a read made in another
   order changes what follows. */
static void test_json_walk(void)
{
    static const char setup[] =
        "var L = []\n"
        "function rng(seed) { var s = seed >>> 0; return function () { s = (s + 0x6D2B79F5) >>> 0; var t = s;\n"
        "  t = Math.imul(t ^ (t >>> 15), t | 1); t ^= t + Math.imul(t ^ (t >>> 7), t | 61); return ((t ^ (t >>> 14)) >>> 0) / 4294967296 } }\n"
        "var KEYS = ['a', 'b', 'id', 'name', '', '0', '1', '17', '01', '4294967294', '4294967295', 'k\"q', 'k\\\\b', 'k\\n',\n"
        "  '\\u00e9', '\\u4e2d', '\\ud800', 'toJSON', 'length', 'x'.repeat(130), 'y\"'.repeat(70)]\n"
        "var STRS = ['', 'plain', 'a\"b', '\\\\', '\\n\\t', '\\u0001', '\\u00e9t\\u00e9', '\\u4e2d\\u6587', '\\ud83d\\ude00', '\\ud800x', 'x\\udc00', 'y'.repeat(70)]\n"
        "var NUMS = [0, -0, 1, -1, 42, 2147483647, -2147483648, 2147483648, 1.5, -0.1, 1e21, 1e-7, NaN, Infinity, -Infinity, 5e-324]\n"
        "function gen(seed) {\n"
        "  var r = rng(seed), all = []\n"
        "  function pick(a) { return a[Math.floor(r() * a.length)] }\n"
        "  function prim() { var x = r()\n"
        "    if (x < 0.3) return pick(NUMS)\n"
        "    if (x < 0.6) return pick(STRS)\n"
        "    if (x < 0.7) return r() < 0.5\n"
        "    if (x < 0.8) return null\n"
        "    if (x < 0.86) return undefined\n"
        "    if (x < 0.9) return function () {}\n"
        "    if (x < 0.94) return Symbol('s')\n"
        "    if (x < 0.99) return pick(STRS) + pick(STRS) + 'x'.repeat(20)\n"
        "    return 'z'.repeat(9000) + pick(STRS) }\n"
        "  function mutate() { var o = pick(all), x = r(); if (!o) return\n"
        "    if (x < 0.25) delete o[pick(KEYS)]\n"
        "    else if (x < 0.45) o[pick(KEYS)] = prim()\n"
        "    else if (x < 0.55) o[pick(KEYS)] = pick(all)\n"
        "    else if (x < 0.65) { if (Array.isArray(o)) o.length = Math.floor(r() * 4); else Object.defineProperty(o, pick(KEYS), { enumerable: false }) }\n"
        "    else if (x < 0.72) Object.prototype.toJSON = function (k) { L.push('O.toJSON ' + k); return 'O' }\n"
        "    else if (x < 0.78) Array.prototype.toJSON = function (k) { L.push('A.toJSON ' + k); return 'A' }\n"
        "    else if (x < 0.85) { if (Array.isArray(o)) o.push(prim()); else Object.defineProperty(o, pick(KEYS), { get: function () { L.push('late getter'); return 1 }, enumerable: true, configurable: true }) }\n"
        "  }\n"
        "  function odd(d) { var x = r(), k = 'g' + Math.floor(r() * 3), o\n"
        "    if (x < 0.15) { o = {}; o.a = val(d + 1); Object.defineProperty(o, k, { get: function () { L.push('get ' + k); if (r() < 0.5) mutate(); return prim() }, enumerable: r() < 0.8, configurable: true }); o.z = val(d + 1); all.push(o); return o }\n"
        "    if (x < 0.3) { o = { v: val(d + 1), toJSON: function (key) { L.push('toJSON ' + key); if (r() < 0.6) mutate(); return r() < 0.5 ? this.v : prim() } }; all.push(o); return o }\n"
        "    if (x < 0.38) { o = Object.create({ toJSON: function (key) { L.push('proto toJSON ' + key); return { w: prim() } } }); o.q = 1; return o }\n"
        "    if (x < 0.44) return new Date(Math.floor(r() * 1e12))\n"
        "    if (x < 0.5) return pick([new Number(r() * 10), new String(pick(STRS)), new Boolean(r() < 0.5), Object(Symbol('b'))])\n"
        "    if (x < 0.58) { o = obj(d); return new Proxy(o, { get: function (t, p, rc) { L.push('trap ' + String(p)); return Reflect.get(t, p, rc) } }) }\n"
        "    if (x < 0.63) return r() < 0.5 ? 12345678901234567890n : 3n\n"
        "    if (x < 0.7) { var a = [1, 2, 3]; delete a[1]; return a }\n"
        "    if (x < 0.76) return all.length ? pick(all) : 1\n"
        "    if (x < 0.82) return new Uint8Array([1, 2])\n"
        "    if (x < 0.88) { o = Object.create(null); o.n = val(d + 1); return o }\n"
        "    return { get [k]() { L.push('getter ' + k); return val(d + 1) }, b: 2 } }\n"
        "  function obj(d) { var o = r() < 0.1 ? new (class C {})() : {}, n = Math.floor(r() * 6)\n"
        "    for (var i = 0; i < n; i++) o[pick(KEYS)] = val(d + 1)\n"
        "    if (r() < 0.1) delete o[pick(KEYS)]\n"
        "    if (r() < 0.08) Object.defineProperty(o, 'ne', { value: val(d + 1), enumerable: false })\n"
        "    if (r() < 0.05) o[Symbol('k')] = 1\n"
        "    all.push(o); return o }\n"
        "  function arr(d) { var n = Math.floor(r() * 6), a = []\n"
        "    for (var i = 0; i < n; i++) a.push(val(d + 1))\n"
        "    if (r() < 0.05) a.length += 2\n"
        "    if (r() < 0.05) a.extra = 1\n"
        "    all.push(a); return a }\n"
        "  function val(d) { var x = r()\n"
        "    if (d > 3 || x < 0.35) return prim()\n"
        "    if (x < 0.62) return obj(d)\n"
        "    if (x < 0.85) return arr(d)\n"
        "    return odd(d) }\n"
        "  var root = val(0)\n"
        "  if (r() < 0.05 && all.length) { var c = pick(all); c[0] = root }\n"
        "  return root }\n"
        "function run(seed) { L.length = 0; var res\n"
        "  try { res = JSON.stringify(gen(seed)) } catch (e) { res = e.name + ': ' + e.message }\n"
        "  delete Object.prototype.toJSON; delete Array.prototype.toJSON\n"
        "  return String(res) + ' | ' + L.join() }\n";
    enum { SEEDS = 3000, SEEDS_VM = 100 };
    VM v = vm_new();
    unsigned long walks = 0;
    int seed, same = 0;

    CHECK(eval_ok(v.ctx, setup), "the setup threw");
    white_json_walks();
    CHECK(eval_is(v.ctx, "JSON.stringify({ a: 1, b: [1, 2, { c: 'x' }], d: { e: null } })",
                  "{\"a\":1,\"b\":[1,2,{\"c\":\"x\"}],\"d\":{\"e\":null}}"), "a tree");
    CHECK(white_json_walks() == 4, "the walks wrote the object, the array and the two inside");
    CHECK(eval_is(v.ctx, "JSON.stringify({ a: [1] }, (k, x) => x) + JSON.stringify({ a: [1] }, ['a']) + JSON.stringify({ a: [1] }, null, 1)",
                  "{\"a\":[1]}{\"a\":[1]}{\n \"a\": [\n  1\n ]\n}"), "a replacer, a list, a gap");
    CHECK(white_json_walks() == 0, "the walks take no call with a replacer, a property list or a gap");
    CHECK(eval_is(v.ctx, "JSON.stringify({ get a() { return 1 }, b: { c: 1 } })", "{\"a\":1,\"b\":{\"c\":1}}"), "a getter");
    CHECK(white_json_walks() == 1, "an object with an enumerable getter goes to the general path, its value does not");
    CHECK(eval_is(v.ctx, "JSON.stringify([{ toJSON() { return { x: [1] } } }, new Proxy({}, {}), { 2: 1 }, Object.assign([1, , 3])])",
                  "[{\"x\":[1]},{},{\"2\":1},[1,null,3]]"), "toJSON, a proxy, an index key, a hole");
    CHECK(white_json_walks() == 3, "the outer array and what toJSON gives");
    for (seed = 0; seed < SEEDS; seed++) {
        char src[64], *a, *b;

        /* a VM for each SEEDS_VM seeds, which the host's timeout counts
           from its start */
        if (seed % SEEDS_VM == SEEDS_VM - 1) {
            vm_free(&v);
            v = vm_new();
            CHECK(eval_ok(v.ctx, setup), "the setup threw");
        }
        snprintf(src, sizeof(src), "run(%d)", seed);
        white_json_walk(1);
        a = eval_text(v.ctx, src);
        if (!a)
            a = exception_text(v.ctx);
        walks += white_json_walks();
        white_json_walk(0);
        b = eval_text(v.ctx, src);
        if (!b)
            b = exception_text(v.ctx);
        CHECK(white_json_walks() == 0, "seed %d: a walk without the walks", seed);
        white_json_walk(1);
        CHECK(!a || !strstr(a, "interrupted"), "seed %d: the host's timeout stopped it", seed);
        CHECK(a && b && !strcmp(a, b), "seed %d: with the walks %.300s\nwithout %.300s", seed, a ? a : "(threw)", b ? b : "(threw)");
        same += a && b && !strcmp(a, b) && strstr(a, " | ") != NULL;
        free(a);
        free(b);
    }
    CHECK(walks > 5000, "only %lu walks", walks);
    printf("json walk: %d seeds the same, %lu walks\n", same, walks);
    vm_free(&v);
}

static void test_json_stringify_list_oom(void)
{
    static const char setup[] =
        "globalThis.f = () => JSON.stringify({ x: { a: 1 }, y: [2], z: 3 }, ['x', 'y', 'a', new String('z')], ' ')\n"
        "f(); f()";
    static const char want[] = "{\n \"x\": {\n  \"a\": 1\n },\n \"y\": [\n  2\n ],\n \"z\": 3\n}";
    VM v = vm_new();
    JSValue g, f;
    int fail, n = 0, ooms = 0;

    CHECK(eval_ok(v.ctx, setup), "the setup threw");
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "f");
    JS_FreeValue(v.ctx, g);
    for (fail = 0; fail == 0 || fail <= n; fail++) {
        JSMemoryUsage before, after;
        int made;
        JSValue r;

        JS_RunGC(v.rt);
        JS_ComputeMemoryUsage(v.rt, &before);
        r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);
        if (fail == 0)
            n = made;
        if (JS_IsException(r)) {
            CHECK(fail > 0 && JS_IsUncatchableException(v.ctx), "call %d failing: an exception that is not out of memory", fail);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            ooms++;
        } else {
            const char *s;

            CHECK(!JS_HasException(v.ctx), "call %d failing: a result with an exception pending", fail);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            s = JS_ToCString(v.ctx, r);
            CHECK(s && !strcmp(s, want), "call %d failing: %s", fail, s ? s : "(no string)");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, r);
        }
        JS_RunGC(v.rt);
        JS_ComputeMemoryUsage(v.rt, &after);
        CHECK(after.obj_count == before.obj_count && after.str_count == before.str_count,
              "call %d failing: %lld objects and %lld strings, before it %lld and %lld", fail,
              (long long)after.obj_count, (long long)after.str_count, (long long)before.obj_count,
              (long long)before.str_count);
    }
    CHECK(ooms > 10, "only %d of %d calls failed the stringify", ooms, n);
    JS_FreeValue(v.ctx, f);
    vm_free(&v);
}


/* ---- pending stacks ---- */

static const char deep_error[] =
    "function deep(n) { return n ? deep(n - 1) : new Error(\"deep\") }\n"
    "function make() { return deep(40) }\n";

/* Operations that look only at whether 'stack' exists, its attributes or
   whether it is a getter do not need its text, so they leave the stack pending
   and cannot fail for lack of memory to format it. */
static void test_backtrace_attribute_checks(void)
{
    static const char *const ops[] = {
        "Object.keys(e)",
        "e.__lookupGetter__(\"stack\")",
        "e.__lookupSetter__(\"stack\")",
        "e.propertyIsEnumerable(\"stack\")",
        "Object.isFrozen(e)",
        "Object.isSealed(e)",
        "Object.seal(e)",
        "Object.freeze(e)",
        "({ ...e })",
        "Object.assign({}, e)",
        "Object.entries(e)",
        "new Proxy(e, {}).hasOwnProperty(\"stack\")",
        "\"stack\" in new Proxy(e, { has() { return false } })",
        "Reflect.ownKeys(new Proxy(e, { ownKeys(t) { return Reflect.ownKeys(t) } }))",
        "Reflect.deleteProperty(new Proxy(e, { deleteProperty() { return true } }), \"stack\")",
    };
    size_t i;

    for (i = 0; i < countof(ops); i++) {
        VM v = vm_new();
        JSValue ref, e, g;
        char *want, *got, src[400];

        eval_ok(v.ctx, deep_error);
        ref = call_global(v.ctx, "make");
        want = stack_text(v.ctx, ref);
        JS_FreeValue(v.ctx, ref);
        e = call_global(v.ctx, "make");
        snprintf(src, sizeof(src), "var held; function op() { const e = held; return %s }", ops[i]);
        eval_ok(v.ctx, src);
        g = JS_GetGlobalObject(v.ctx);
        JS_SetPropertyStr(v.ctx, g, "held", JS_DupValue(v.ctx, e));
        JS_FreeValue(v.ctx, g);
        JS_FreeValue(v.ctx, call_global(v.ctx, "op"));
        CHECK(white_stack_pending(v.ctx, e) == 1, "%s formatted the stack", ops[i]);
        got = stack_text(v.ctx, e);
        CHECK(got && want && strcmp(got, want) == 0, "read after %s gave a different text", ops[i]);
        free(got);
        free(want);
        JS_FreeValue(v.ctx, e);
        vm_free(&v);
    }
}

/* The class definition of the marker has no call or exotic handlers, so the
   marker is never taken for a function. */
static void test_backtrace_class(void)
{
    VM v = vm_new();

    eval_ok(v.ctx, deep_error);
    JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
    CHECK(!white_backtrace_class_has_handlers(v.rt), "the class of the marker has a call or exotic handler");
    vm_free(&v);
}

/* The value of a property is only a JSValue when the property is a plain data
   property: an accessor holds two object pointers there and a global variable
   a pointer to its variable reference, with whatever bytes were there before
   after it. The pending-stack checks must look at the kind of the property
   first, and never read such bytes as a marker. */
static void test_backtrace_needs_data_property(void)
{
    VM v = vm_new();
    JSValue e;

    eval_ok(v.ctx, deep_error);
    e = call_global(v.ctx, "make");
    eval_ok(v.ctx, "var stack = 1");
    CHECK(white_pending_needs_data_property(v.ctx, e) == 0, "a variable reference was read as a pending stack");
    JS_FreeValue(v.ctx, e);
    vm_free(&v);
}

/* JS_ComputeMemoryUsage counts the frames of a pending stack. */
static void test_backtrace_memory_usage(void)
{
    VM v = vm_new();
    JSMemoryUsage base, held, freed;
    int64_t frames = 1000 * white_backtrace_frame_size();

    eval_ok(v.ctx,
            "var kept\n"
            "function deep(n) { return n ? deep(n - 1) : new Error(\"deep\") }\n"
            "function make() { kept = deep(1000) }\n"
            "function drop() { kept = undefined }\n");
    /* The first call allocates what stays for later calls. */
    JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
    JS_FreeValue(v.ctx, call_global(v.ctx, "drop"));
    JS_RunGC(v.rt);
    JS_ComputeMemoryUsage(v.rt, &base);
    JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
    JS_RunGC(v.rt);
    JS_ComputeMemoryUsage(v.rt, &held);
    JS_FreeValue(v.ctx, call_global(v.ctx, "drop"));
    JS_RunGC(v.rt);
    JS_ComputeMemoryUsage(v.rt, &freed);
    CHECK(held.memory_used_size - base.memory_used_size >= frames,
          "a pending stack of 1001 frames adds %lld bytes of memory_used_size, want at least %lld",
          (long long)(held.memory_used_size - base.memory_used_size), (long long)frames);
    CHECK(freed.memory_used_size == base.memory_used_size, "memory_used_size %lld after freeing, want %lld",
          (long long)freed.memory_used_size, (long long)base.memory_used_size);
    vm_free(&v);
}

/* ---- the block of an object ---- */

/* An ordinary object has no room for the union of the data of the classes
   (the engine allocates the bytes in front of it), and every other object has.
   The block of an object literal also has the room of its properties. */
static void test_object_block(void)
{
    VM v = vm_new();
    JSValue plain = JS_NewObject(v.ctx), arr = JS_NewArray(v.ctx), lit;
    const char *src = "({a: 1, b: \"x\"})";
    size_t full = 0, a, p, f, plain_room;

    p = white_object_block(v.ctx, plain, &full);
    plain_room = p;
    a = white_object_block(v.ctx, arr, &f);
    CHECK(p > 0 && p < full, "an ordinary object has room for %zu bytes, a JSObject is %zu", p, full);
    CHECK(a >= full, "an array has room for %zu bytes, a JSObject is %zu", a, full);
    JS_FreeValue(v.ctx, plain);
    JS_FreeValue(v.ctx, arr);
    lit = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    p = white_object_block(v.ctx, lit, &full);
    /* the block of an object literal of two keys also holds their two properties of 16 bytes */
    CHECK(p >= plain_room + 2 * 16 && p < plain_room + 2 * 16 + 8, "an object literal of 2 keys has room for %zu bytes, an ordinary object %zu", p, plain_room);
    JS_FreeValue(v.ctx, lit);
    vm_free(&v);
}

/* ---- the properties of an object ---- */

static int props_inline(VM *v, const char *expr, int *room)
{
    JSValue o = JS_Eval(v->ctx, expr, strlen(expr), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int r = white_props_inline(o, room);

    JS_FreeValue(v->ctx, o);
    return r;
}

/* The properties of an object literal sit in the block of the object, with
   room for the properties of its shape. Adding a key past that room moves
   them to a block of their own, and everything else that changes an object
   (flags, prototype, deleting keys, even until the properties are packed)
   leaves them where they are. */
static void test_object_inline_props(void)
{
    VM v = vm_new();
    int room = -1;

    CHECK(eval_ok(v.ctx, "var lit = { a: 1, b: [2], c: 'three' }; var open = {}; var arr = [1];\n"
                         "var big = { k0: 0, k1: 1, k2: 2, k3: 3, k4: 4, k5: 5, k6: 6, k7: 7,\n"
                         "            k8: 8, k9: 9, k10: 10, k11: 11, k12: 12, k13: 13, k14: 14, k15: 15 };\n"
                         "var made = []; for (var i = 0; i < 4; i++) made.push({ a: i, b: i });\n"
                         "void 0"), "the script threw");
    CHECK(props_inline(&v, "lit", &room) == 1 && room >= 3, "a literal of 3 keys: room %d", room);
    CHECK(props_inline(&v, "made[3]", &room) == 1 && room >= 2, "a literal of 2 keys: room %d", room);
    CHECK(props_inline(&v, "big", &room) == 1 && room >= 16, "a literal of 16 keys: room %d", room);
    CHECK(props_inline(&v, "open", &room) == 0, "{} has its properties apart");
    CHECK(props_inline(&v, "arr", &room) == 0, "an array has its properties apart");
    CHECK(props_inline(&v, "Object.create(null)", &room) == 0, "Object.create has its properties apart");
    /* changes that keep the room */
    CHECK(eval_ok(v.ctx, "Object.setPrototypeOf(lit, { p: 1 }); Object.defineProperty(lit, 'a', { writable: false });\n"
                         "Object.defineProperty(lit, 'c', { get() { return 5 }, enumerable: true, configurable: true }); void 0"),
          "the script threw");
    CHECK(props_inline(&v, "lit", &room) == 1, "after a prototype, a flag and an accessor");
    CHECK(eval_is(v.ctx, "lit.p + ',' + lit.a + ',' + lit.b + ',' + lit.c + ',' + Object.keys(lit)", "1,1,2,5,a,b,c"),
          "values after the changes");
    CHECK(eval_ok(v.ctx, "Object.freeze(made[1]); Object.seal(made[2]); Object.preventExtensions(made[3]); void 0"), "the script threw");
    CHECK(props_inline(&v, "made[1]", &room) == 1 && props_inline(&v, "made[2]", &room) == 1 &&
          props_inline(&v, "made[3]", &room) == 1, "frozen, sealed, not extensible");
    /* deleting keys, until the shape packs what is left */
    CHECK(eval_ok(v.ctx, "for (var i = 0; i < 12; i++) delete big['k' + i]; void 0"), "the script threw");
    CHECK(props_inline(&v, "big", &room) == 1, "after deleting 12 of 16 keys: room %d", room);
    CHECK(eval_is(v.ctx, "Object.keys(big) + ' ' + JSON.stringify(big)", "k12,k13,k14,k15 {\"k12\":12,\"k13\":13,\"k14\":14,\"k15\":15}"),
          "keys after deleting");
    /* one key more than the room moves them out */
    CHECK(eval_ok(v.ctx, "big.x = 1; big.y = 2; void 0"), "the script threw");
    CHECK(eval_is(v.ctx, "Object.keys(big) + ' ' + big.k12 + big.k15 + big.x + big.y", "k12,k13,k14,k15,x,y 121512"),
          "keys after adding");
    CHECK(eval_ok(v.ctx, "var o2 = { a: 1, b: 2 }; for (var i = 0; i < 20; i++) o2['n' + i] = i; void 0"), "the script threw");
    CHECK(props_inline(&v, "o2", &room) == 0, "20 more keys: they moved out");
    CHECK(eval_is(v.ctx, "o2.a + o2.b + o2.n19 + Object.keys(o2).length", "44"), "values after growing");
    /* once an object of a literal got a key, the next objects of that literal have
       their properties apart, and the objects of other literals do not */
    CHECK(eval_ok(v.ctx, "function mk() { return { q1: 1, q2: 2, q3: 3 } }\n"
                         "function other() { return { r1: 1, r2: 2, r3: 3 } }\n"
                         "var m1 = mk(), m2 = mk(); void 0"), "the script threw");
    CHECK(props_inline(&v, "m1", &room) == 1 && props_inline(&v, "m2", &room) == 1, "two objects of a literal");
    CHECK(eval_ok(v.ctx, "m1.q4 = 4; var m3 = mk(), o1 = other(); o1.r4 = 4; void 0"), "the script threw");
    CHECK(props_inline(&v, "m1", &room) == 0, "the object that got a key");
    CHECK(props_inline(&v, "m2", &room) == 1, "the object made before it");
    CHECK(props_inline(&v, "m3", &room) == 0, "an object made after it: room %d", room);
    CHECK(props_inline(&v, "other()", &room) == 0, "an object of a literal with the same number of keys that got a key");
    CHECK(props_inline(&v, "({ s1: 1, s2: 2, s3: 3 })", &room) == 1, "an object of a literal with other keys");
    CHECK(eval_is(v.ctx, "[m1, m2, m3].map(o => o.q1 + o.q2 + o.q3).join() + Object.keys(m1) + Object.keys(m3)", "6,6,6q1,q2,q3,q4q1,q2,q3"),
          "values of the objects");
    /* objects in a cycle, under a WeakRef and as the key of a WeakMap are freed by the
       collectors, and the block of one a weak reference still needs stays until it goes */
    CHECK(eval_ok(v.ctx, "var wm = new WeakMap(), wr = [], keep = [];\n"
                         "for (var i = 0; i < 60; i++) {\n"
                         "  var c = { self: null, n: i, w: 'x' + i }; c.self = c;\n"
                         "  var d = { a: i, b: { c } };\n"
                         "  wr.push(new WeakRef(c)); wm.set(c, i); wm.set(d, i);\n"
                         "  if (i % 10 == 0) { keep.push(c); c.z = 1; c.y = 2; c.x = 3 }\n"
                         "}\n"
                         "void 0"), "the script threw");
    JS_RunYoungGC(v.rt);
    JS_RunGC(v.rt);
    CHECK(eval_is(v.ctx, "keep.length + ' ' + keep[3].n + keep[3].self.w + keep[5].x", "6 30x303"), "the objects the script kept");
    CHECK(eval_ok(v.ctx, "keep = null; wr = null; wm = null; c = d = null; void 0"), "the script threw");
    JS_RunGC(v.rt);
    JS_RunGC(v.rt);
    CHECK(eval_ok(v.ctx, "lit = big = o2 = made = null; void 0"), "the script threw");
    JS_RunGC(v.rt);
    vm_free(&v);
}

/* Whether chk(arg) ran and returned true: a check that throws is a failed
   check, not a passed one (JS_ToBool says -1 for an exception). */
static int check_true(JSContext *ctx, JSValueConst chk, JSValueConst arg)
{
    JSValue r = JS_Call(ctx, chk, JS_UNDEFINED, 1, &arg);
    int ok;

    if (JS_IsException(r)) {
        JS_FreeValue(ctx, JS_GetException(ctx));
        return 0;
    }
    ok = JS_ToBool(ctx, r) == 1;
    JS_FreeValue(ctx, r);
    return ok;
}

/* A call of the allocator that fails while an object grows or packs its
   properties, or while the first object of a literal is made, leaves the
   object as it was (or no object), and the heap whole (vm_free finds a
   leak). Every call is failed in turn. */
static void test_object_inline_oom(void)
{
    static const char setup[] =
        "function mk() { return { a: 1, b: [2], c: 'three' } }\n"
        "function mk16() { return { k0: 0, k1: 1, k2: 2, k3: 3, k4: 4, k5: 5, k6: 6, k7: 7,\n"
        "                          k8: 8, k9: 9, k10: 10, k11: 11, k12: 12, k13: 13, k14: 14, k15: 15 } }\n"
        "function grow(o) { for (var i = 0; i < 8; i++) o['n' + i] = i; return o }\n"
        "function shrink(o) { for (var i = 0; i < 12; i++) delete o['k' + i]; return o }\n"
        /* the properties leave the block, then a delete packs them in a block of their own */
        "function growshrink(o) { grow(o); return shrink(o) }\n"
        /* the shapes {a}, {a,b}, {a,b,c} and {a,b,c,d} are in the shape hash table, with room for 2, 3, 3 and 4 */
        "var keep = [{}, {}]; for (var o of keep) o.a = 1; for (var o of keep) o.b = [2]; for (var o of keep) o.c = 'three'; for (var o of keep) o.d = 4;\n"
        "function mk3() { return { a: 1, b: [2], c: 'three' } }\n"
        "function addd(o) { o.d = 4; return o }\n"
        /* the first run of this literal makes its shape */
        "function mkfirst() { return { fa: 1, fb: [2], fc: 'three' } }\n"
        "function checkf(o) { return Object.keys(o).join() == 'fa,fb,fc' && o.fa === 1 && o.fb[0] === 2 && o.fc === 'three' }\n"
        "function check3(o) { return Object.keys(o).slice(0, 3).join() == 'a,b,c' && o.a === 1 && o.b[0] === 2 && o.c === 'three' }\n"
        "function check16(o) { var ks = Object.keys(o); for (var i = 12; i < 16; i++) if (o['k' + i] !== i) return false; return ks.length >= 4 }\n"
        "function checkbad(o) { throw new Error('check failed') }\n"
        "void 0";
    static const struct {
        const char *name, *mk, *op, *chk;
        int factory; /* op makes the object, there is none before it */
        int moves;   /* the object leaves its block when op succeeds */
    } kinds[] = {
        { "grow", "mk", "grow", "check3", 0, 1 },
        { "shrink", "mk16", "shrink", "check16", 0, 0 },
        { "grow then shrink", "mk16", "growshrink", "check16", 0, 1 },
        { "found shape", "mk3", "addd", "check3", 0, 1 },
        { "first run", NULL, "mkfirst", "checkf", 1, 0 },
    };
    long cases = 0, failed_calls = 0;
    int kind;

    /* the check must see a check that throws as a failure */
    {
        VM v = vm_new();
        JSValue global, chk;

        CHECK(eval_ok(v.ctx, setup), "the script threw");
        global = JS_GetGlobalObject(v.ctx);
        chk = JS_GetPropertyStr(v.ctx, global, "checkbad");
        CHECK(!check_true(v.ctx, chk, JS_UNDEFINED), "a check that throws passed");
        JS_FreeValue(v.ctx, chk);
        JS_FreeValue(v.ctx, global);
        vm_free(&v);
    }
    for (kind = 0; kind < (int)countof(kinds); kind++) {
        int n = 0, fail;

        for (fail = 0; fail <= 1 || fail <= n; fail++) {
            VM v = vm_new();
            JSValue g, mk = JS_UNDEFINED, chk, obj = JS_UNDEFINED, r, global;
            int made, room;

            CHECK(eval_ok(v.ctx, setup), "the script threw");
            global = JS_GetGlobalObject(v.ctx);
            if (kinds[kind].mk)
                mk = JS_GetPropertyStr(v.ctx, global, kinds[kind].mk);
            g = JS_GetPropertyStr(v.ctx, global, kinds[kind].op);
            chk = JS_GetPropertyStr(v.ctx, global, kinds[kind].chk);
            if (!kinds[kind].factory) {
                obj = JS_Call(v.ctx, mk, JS_UNDEFINED, 0, NULL);
                CHECK(white_props_inline(obj, &room) == 1, "%s: the object starts in its block", kinds[kind].name);
            }
            r = white_call_failing(v.ctx, g, JS_UNDEFINED, !kinds[kind].factory, &obj, fail, &made);
            if (fail == 0)
                n = made;
            cases++;
            if (JS_IsException(r)) {
                failed_calls++;
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            } else if (JS_HasException(v.ctx)) {
                /* the call worked and an error is still pending: a delete packs the properties, which
                   is optional, and a failed packing must leave nothing behind */
                CHECK(0, "%s with call %d failing: the call worked and left an error pending", kinds[kind].name, fail);
                JS_FreeValue(v.ctx, r);
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            } else if (kinds[kind].factory) {
                CHECK(check_true(v.ctx, chk, r), "%s with call %d failing: the new object is wrong", kinds[kind].name, fail);
                JS_FreeValue(v.ctx, r);
            } else {
                JS_FreeValue(v.ctx, r);
            }
            if (kinds[kind].factory) {
                /* the shape the failed run may have made or left must serve the next run */
                r = JS_Call(v.ctx, g, JS_UNDEFINED, 0, NULL);
                CHECK(!JS_IsException(r) && check_true(v.ctx, chk, r), "%s with call %d failing: the next object is wrong", kinds[kind].name, fail);
                JS_FreeValue(v.ctx, r);
            } else {
                CHECK(check_true(v.ctx, chk, obj), "%s with call %d failing: the object lost a property", kinds[kind].name, fail);
                if (fail == 0 && kinds[kind].moves)
                    CHECK(white_props_inline(obj, &room) == 0, "%s: the properties left the block", kinds[kind].name);
                if (fail == 0 && !kinds[kind].moves)
                    CHECK(white_props_inline(obj, &room) == 1, "%s: the properties stay in the block", kinds[kind].name);
                if (fail == 0 && !strcmp(kinds[kind].name, "found shape")) {
                    JSValue k = JS_Eval(v.ctx, "keep[0]", 7, "<unit>", JS_EVAL_TYPE_GLOBAL);

                    CHECK(white_shape_of(obj) == white_shape_of(k), "found shape: the object took the shape that was in the shape hash table");
                    JS_FreeValue(v.ctx, k);
                }
            }
            JS_FreeValue(v.ctx, obj);
            JS_FreeValue(v.ctx, chk);
            JS_FreeValue(v.ctx, g);
            JS_FreeValue(v.ctx, mk);
            JS_FreeValue(v.ctx, global);
            vm_free(&v);
        }
        CHECK(n > 0, "%s makes no call of the allocator", kinds[kind].name);
    }
    CHECK(failed_calls > 0, "no call of %ld failed", cases);
    printf("unit-white: test_object_inline_oom: %ld cases, %ld failed calls\n", cases, failed_calls);
}


/* A string buffer that holds more than JS_COPY_STEP bytes and has to grow is
   copied by hand, JS_COPY_STEP bytes at a time with the flag read after each
   step, and not by the allocator in one call. Growing it by one character
   copies the header and the 1500000 characters it holds. Widening it (a
   character above 0xff) copies them the same way and then widens the 1500000
   characters, and no stretch of work is longer than a step of the copy. */
/* The limit of the memory for a buffer that grows by the copy of its block
   (string_buffer_realloc2): the new block is asked for as heap_realloc does,
   against the count less the old block. A limit that has room for the new
   block and not for both makes the growth, and one byte less fails with out
   of memory, the buffer freed. When the host holds memory for the VM and the
   growth fits only once the host gives it back, the host is asked while the
   limit has the room of the old block, and after it the limit is the one set
   less what the host still holds. */
static size_t buffer_pressures, buffer_held_after;

static size_t buffer_pressure(uint32_t vm, size_t request)
{
    (void)vm;
    (void)request;
    buffer_pressures++;
    return buffer_held_after;
}

static void test_buffer_limit(void)
{
    /* Where every block is a request to the system malloc (asan) the heap counts
       a block by the size the system gives, with 8 bytes more for each, and the
       numbers below are those of the heap of the engine. */
    if (white_every_block_is_a_request()) {
        printf("unit-white: test_buffer_limit skipped, every block is a request in this build\n");
        return;
    }

    VM v = vm_new();
    struct StringBuffer *b;
    unsigned long hdr = white_jsstring_header();
    size_t used, usable, request, limit, *words = JS_GetHostLimit(v.rt);
    int pass;

    host_pressure_call = buffer_pressure;
    for (pass = 0; pass < 3; pass++) {
        /* a buffer of 1500000 characters, full: the next one makes it 2250000 */
        b = white_sb_new(v.ctx, 1500000, 0, 1500000);
        CHECK(b != NULL, "the buffer");
        if (!b)
            break;
        /* what the caches hold would be given back when the first request fails, and the count with it */
        white_front_drain(v.ctx);
        used = JS_GetMemorySize(v.rt);
        usable = white_sb_block_usable(b);
        request = hdr + 2250000 + 1;
        limit = used - usable + request - (pass == 1 ? 1 : 0);
        JS_SetMemoryLimit(v.rt, limit);
        if (pass == 2) {
            /* the host holds 1000 bytes, writes held and limit, and gives them all back when asked */
            words[2] = 1000;
            words[0] = words[1] - 1000;
            buffer_pressures = 0;
            buffer_held_after = 0;
            CHECK(white_sb_write8(b, (const uint8_t *)"y", 1) == 0, "the buffer did not grow once the host gave back what it held");
            CHECK(!white_sb_error(b), "the buffer is in error");
            CHECK(buffer_pressures == 1 && words[2] == 0, "%zu requests to the host, %zu still held, want 1 and 0", buffer_pressures, words[2]);
            CHECK(white_heap_limit(v.ctx) == limit && words[1] == limit, "the limit is %lu after the growth with held given back, want %lu",
                  (unsigned long)white_heap_limit(v.ctx), (unsigned long)limit);
        } else if (pass == 0) {
            CHECK(white_sb_write8(b, (const uint8_t *)"y", 1) == 0, "the buffer did not grow with the room of the old block in the limit");
            CHECK(!white_sb_error(b), "the buffer is in error");
            CHECK(white_heap_limit(v.ctx) == limit, "the limit is %lu after the growth, want %lu", (unsigned long)white_heap_limit(v.ctx), (unsigned long)limit);
        } else {
            CHECK(white_sb_write8(b, (const uint8_t *)"y", 1) != 0, "the buffer grew past the limit");
            CHECK(white_sb_error(b), "the buffer is not in error");
            CHECK(white_heap_limit(v.ctx) == limit, "the limit is %lu after the failure, want %lu", (unsigned long)white_heap_limit(v.ctx), (unsigned long)limit);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
        }
        JS_SetMemoryLimit(v.rt, (size_t)-1);
        white_sb_free(v.ctx, b);
    }
    host_pressure_call = NULL;
    vm_free(&v);
}

static void test_buffer_copy_work(void)
{
    VM v = vm_new();
    struct StringBuffer *b;
    unsigned long hdr = white_jsstring_header();

    b = white_sb_new(v.ctx, 1500000, 0, 1500000);
    CHECK(b != NULL, "the buffer");
    if (b) {
        white_work_reset();
        CHECK(white_sb_write8(b, (const uint8_t *)"y", 1) == 0, "the write failed");
        CHECK(white_work_sum() == hdr + 1500000, "grown: %lu units of work, want %lu", white_work_sum(), hdr + 1500000);
        CHECK(white_work_longest() <= (1 << 20), "grown: %lu units of work between two readings of the flag", white_work_longest());
        white_sb_free(v.ctx, b);
    }
    b = white_sb_new(v.ctx, 2000000, 0, 1500000);
    CHECK(b != NULL, "the buffer");
    if (b) {
        white_work_reset();
        CHECK(white_sb_putc16(b, 0x4e2d) == 0, "the putc failed");
        CHECK(white_work_sum() == hdr + 1500000 + 1500000, "widened: %lu units of work, want %lu", white_work_sum(), hdr + 3000000);
        CHECK(white_work_longest() <= (1 << 20), "widened: %lu units of work between two readings of the flag", white_work_longest());
        white_sb_free(v.ctx, b);
    }
    /* the work that came before the call is not added to the first step of the copy: 60000 units, then 1500000 bytes of the buffer, in steps of 1 MiB */
    b = white_sb_new(v.ctx, 1500000, 0, 1500000);
    CHECK(b != NULL, "the buffer");
    if (b) {
        white_work_reset();
        white_work_add(60000);
        CHECK(white_sb_write8(b, (const uint8_t *)"y", 1) == 0, "the write failed");
        CHECK(white_work_sum() == 60000 + hdr + 1500000, "after a scan: %lu units of work, want %lu", white_work_sum(), 60000 + hdr + 1500000);
        CHECK(white_work_longest() <= (1 << 20), "after a scan: %lu units of work between two readings of the flag", white_work_longest());
        white_sb_free(v.ctx, b);
    }
    vm_free(&v);
}

/* The copy of a spread, a rest and Object.assign reads the interrupt flag
   every 4096 keys (JS_COPY_POLL_KEYS), a unit of work a key, in both of its
   loops: js_copy_data, of a plain object, and js_copy_data_properties, of a
   String object or into Object.assign. */
static void test_copy_poll_work(void)
{
    static const char *const calls[] = {
        "({...s}), 0",
        "({a: 1, ...s}), 0",
        "Object.assign({}, s), 0",
        "({a: 1, ...big}), 0",
        "(({k0, ...r}) => r)(big), 0",
        "Object.assign({}, big), 0",
    };
    VM v = vm_new();
    size_t i;

    CHECK(eval_ok(v.ctx, "var s = 'x'.repeat(100000), big = {};\n"
                         "for (var i = 0; i < 100000; i++) big['k' + i] = i; void 0"),
          "the sources");
    for (i = 0; i < sizeof(calls) / sizeof(calls[0]); i++) {
        white_work_reset();
        CHECK(eval_is(v.ctx, calls[i], "0"), "%s threw", calls[i]);
        CHECK(white_work_sum() >= 100000 && white_work_sum() < 101000,
              "%s: %lu units of work, want a key each", calls[i], white_work_sum());
        CHECK(white_work_longest() <= 4096, "%s: %lu keys between two readings of the flag",
              calls[i], white_work_longest());
    }
    vm_free(&v);
}

/* The loops that read the interrupt flag after so many units of work say how
   much they did (JS_WORK in quickjs.c), and white.c adds it up between two
   readings of the flag. A call below does a known amount of work in all (the
   characters a pad writes, a slice scans and copies, a key's hash walks, and
   a leaf of a rope counts one), and no stretch between two readings, nor the
   one before the first or the one after the last, is longer than the step.
   The count of readings alone cannot show that: a loop that read the flag
   twice as often in one place and not at all in another reads as often. */
static void test_poll_work_gaps(void)
{
    enum { ROPE_STEP = 1 << 16, PAD_STEP = 1 << 16, SUB_STEP = 1 << 20, BIG_STEP = 1 << 15 };
    /* a case whose total is not checked: the code of a regexp is copied as it
       grows, and how often depends on the room the allocator gives each block
       (the sanitizers use the system's) */
    const unsigned long ANY = (unsigned long)-1;
    static const char setup[] =
        "var f1 = 'ab'.repeat(65535) + 'c', f2 = 'ab'.repeat(32768), f3 = 'ab'.repeat(32769) + 'c'\n"
        "var wide = ['\\u4e00', '\\u00e9'.repeat(2 << 20)].join('')\n"
        "var wlast = ['\\u00e9'.repeat(3 << 20), '\\u4e00'].join('')\n"
        "var w15 = ['\\u4e00', '\\u00e9'.repeat(3 << 19)].join('')\n"
        "var nar = '\\u00e9'.repeat((3 << 20) + 5)\n"
        /* patterns of regexps: 2.2M characters of groups, a group of 2M characters with a quantifier, an alternative of 2M characters, a name used before its group, and 1M wide characters */
        "var xa = '(?:a|b|c)+d'.repeat(200000), xg = '(?:' + 'a'.repeat(1 << 21) + ')+', xo = 'a'.repeat(1 << 21) + '|b'\n"
        "var xk = '\\\\k<n>' + 'a'.repeat(1 << 21) + '(?<n>x)', xw = '\\u4e2d'.repeat(1 << 20)\n"
        "xg = String(xg), xo = String(xo), xk = String(xk)\n"
        /* terms of 2M characters: a count, an escape and a group name of leading zeros, and two patterns of 900,002 characters that are the same (the cache keeps the first) */
        "var xd = String('a{' + '0'.repeat(1 << 21) + '1}'), xu = String('\\\\u{' + '0'.repeat(1 << 21) + '61}')\n"
        "var xn = String('(?<\\\\u{' + '0'.repeat(1 << 21) + '61}>b)')\n"
        "var yc = String('[' + 'a'.repeat(900000) + ']'), yc2 = String('[' + 'a'.repeat(900000) + ']'); new RegExp(yc)\n"
        /* a class of 32 strings of one hash and one length, which differ only in their last 10 characters */
        "var yq = []; for (let i = 0; i < 32; i++) { let s = 'x'.repeat(1 << 12); for (let j = 0; j < 5; j++) s += i >> j & 1 ? 'b\\u00f9' : 'a\\u0200'; yq.push(s) }\n"
        "yq = String('[\\\\q{' + yq.join('|') + '}]')\n"
        /* a class of 60,000 characters above 0xFFFF, downward, and one of 16,384 strings of 5 to 68 characters */
        "var zd = []; for (var c = 0x10000 + 120000; c > 0x10000; c -= 2) zd.push(String.fromCodePoint(c)); zd = String('[' + zd.join('') + ']')\n"
        "var zl = []; for (let i = 0; i < 1 << 14; i++) zl.push((i + 1e6).toString(36) + 'x'.repeat(i * 7919 % 64)); zl = String('[\\\\q{' + zl.join('|') + '}]')\n"
        "var l1 = 'x'.repeat(65534), l2 = 'y'.repeat(65536), l3 = 'z'.repeat(65535), l4 = 'w'.repeat(65533)\n"
        "var flat = 'ab'.repeat(1 << 20)\n"
        "var ra = l1 + l2 + l3, rb = l1 + l2 + l3\n"
        "var fl = 'ab'.repeat(1 << 19), fl2 = 'ab'.repeat(1 << 19), mp = new Map([[fl, 1]])\n"
        "var kx = 'x'.repeat(1 << 21), ky = 'y'.repeat(1 << 21), ky2 = 'y'.repeat(1 << 21), ko = {}; ko[ky] = 1\n"
        "var h1 = 'x'.repeat(131071), h2 = 'x'.repeat(131071), hm = new Map([[h1, 1]]), ho = {}; ho[h1] = 1\n"
        "var g1 = 'x'.repeat(65537), g2 = 'x'.repeat(65537), gm = new Map([[g1, 1]]), go = {}; go[g1] = 1\n"
        /* 64 keys of the same hash and length that differ from cq at the first character of the pair A or B: (256*263+519 == 257*263+256) */
        "var A = '\\u0100\\u0207', B = '\\u0101\\u0100', P = 'x'.repeat(65532)\n"
        "function tail(i) { var r = []; for (var j = 0; j < 6; j++) r.push((i >> j & 1) ? A : B); return r.join('') }\n"
        "var ck = [], cm = new Map(), co = {}; for (var i = 0; i < 64; i++) { var k = [P, B, tail(i)].join(''); ck.push(k); cm.set(k, i); co[k] = i }\n"
        "var cq = [P, A, tail(0)].join('')\n"
        /* JSON text: strings of one run, of escapes, of runs and escapes, a long ASCII run and then a character of 3 bytes */
        "var ja = '\"' + 'a'.repeat(65537) + '\"'\n"
        "var je = '\"' + '\\\\n'.repeat(40000) + '\"'\n"
        "var jm = '\"' + ('a'.repeat(32768) + '\\\\n').repeat(3) + '\"'\n"
        "var ju = '\"' + '\\\\u0061'.repeat(20000) + '\"'\n"
        "var jw = ['\"', 'a'.repeat(70000), '\\u4e2d\"'].join(''), jwr = '\"' + 'a'.repeat(70000) + '\\u4e2d\"'\n"
        "var jx = '\"' + 'a'.repeat(70000) + '\\\\u4e2d\"'\n"
        "var jk = '{\"' + 'z'.repeat(200000) + '\":1}'\n"
        "var pn = 'a'.repeat(70000)\n"
        "var qa = 'a'.repeat(200000), qe = '\\n'.repeat(70000), qm = ('a'.repeat(32768) + '\\n').repeat(3), qw = '\\u4e2d'.repeat(70000)\n"
        "var e1 = 'x'.repeat(200000), e2 = 'x'.repeat(200000), e3 = 'x'.repeat(199999) + 'y'\n"
        "var x1 = '\\u4e2d'.repeat(200000), x2 = '\\u4e2d'.repeat(200000)\n"
        /* strings of 65535 and 65536 characters that differ at the last one, 40 of the same string in an array, and 32 keys of the hash of dq for each length (7 blocks of A or B after the prefix: 65536 characters, 8 blocks: 65538 is not used here) */
        "var s1a = ['x'.repeat(65534), 'a'].join(''), s1b = ['x'.repeat(65534), 'b'].join('')\n"
        "var s2a = ['x'.repeat(65535), 'a'].join(''), s2b = ['x'.repeat(65535), 'b'].join('')\n"
        "var ar1 = Array(40).fill(s1a), ar2 = Array(40).fill(s2a)\n"
        /* two flat strings of 32768 characters use up the budget exactly, and a rope of the same length follows */
        "var q3 = ['x'.repeat(32767), 'b'].join(''), f3 = ['x'.repeat(32767), 'a'].join(''), r3 = 'x'.repeat(32767) + 'a'\n"
        "var ar3 = [f3, f3, r3], ar3r = [r3, f3, f3]\n"
        "function tail7(i) { var r = []; for (var j = 0; j < 7; j++) r.push((i >> j & 1) ? A : B); return r.join('') }\n"
        "var D2 = 'x'.repeat(65520), dm2 = new Map(), do2 = {}; for (var i = 0; i < 32; i++) { var k = [D2, B, tail7(i)].join(''); dm2.set(k, i); do2[k] = i }\n"
        "var dq2 = [D2, A, tail7(0)].join('')\n"
        "var D1 = 'x'.repeat(65519), dm1 = new Map(); for (var i = 0; i < 32; i++) { var k = [D1, B, tail7(i)].join(''); dm1.set(k, i) }\n"
        "var dq1 = [D1, A, tail7(0)].join('')\n"
        /* a JSON string whose buffer is wide before runs of 32768 characters and an escape each */
        "var jn = '\"\\\\u4e2d' + ('a'.repeat(32768) + '\\\\n').repeat(8) + '\"'\n"
        "var qn = ['\\u4e2d', ('a'.repeat(32768) + '\\n').repeat(8)]\n"
        /* tables of 3000 rows: keys and values of ASCII, values that end in a character of 2 bytes, and of 3 (a wide text) */
        "var jt = '[' + Array(3000).fill('{\"id\":\"abcdefgh\",\"name\":\"ijkl\"}').join(',') + ']'\n"
        "var jl = '[' + Array(3000).fill('{\"id\":\"abcdefg\\u00e9\",\"name\":\"ijkl\"}').join(',') + ']'\n"
        "var jy = '[' + Array(3000).fill('{\"id\":\"abcdefg\\u4e2d\",\"name\":\"ijkl\"}').join(',') + ']'\n"
        /* strings of a rope longer than a step: 200000 = 3 steps and 3392, 70000, 600 (made a rope of), and a wide one */
        "var rf1 = 'a'.repeat(200000), rf2 = 'b'.repeat(70000), rf3 = 'x'.repeat(600), rfw = '\\u4e2d'.repeat(100000), rraw = { raw: ['', ''] }, rs1 = 'a'.repeat(40000), rsw = '\\u4e2d'.repeat(40000)\n"
        /* the ropes of the cases that are not about ropes are made flat here, so that a case counts the work of its own call */
        "f1 = String(f1), e3 = String(e3), ja = String(ja), je = String(je), jm = String(jm), ju = String(ju), jx = String(jx), jk = String(jk)\n"
        "jt = String(jt), jl = String(jl), jy = String(jy), jn = String(jn)\n"
        /* BigInts: a product of 10001 limbs by 5001, the same divided, 200,000 binary digits, and a decimal number of 20,000 digits */
        "var ba = 2n ** 320000n - 1n, bb = 2n ** 160000n + 1n, yb = 2n ** 200000n - 1n, sb2 = '0b' + '1'.repeat(200000)\n"
        "var xw = 10n ** 20000n, sd = '9'.repeat(20000)\n"
        "var s0 = new Set(), c0; 0";
    static const struct {
        const char *call;
        unsigned long step, total;
    } cases[] = {
        /* a filler that ends in the middle of a step, or that is longer than one */
        { "'x'.padEnd(400000, f1)", PAD_STEP, 399999 },
        { "'x'.padStart(400000, f1)", PAD_STEP, 399999 },
        { "'x'.padEnd(400000, f2)", PAD_STEP, 399999 },
        { "'x'.padStart(400000, f3)", PAD_STEP, 399999 },
        { "'x'.padEnd(400000, 'abc')", PAD_STEP, 399999 },
        { "'x'.padEnd(400000, ' ')", PAD_STEP, 399999 },
        /* a wide character before or after a long narrow string: the buffer is widened (the characters before it), or the narrow string is written into a wide buffer */
        { "pn.padStart(70001, '\\u4e2d')", ROPE_STEP, 70001 },
        { "pn.padEnd(70001, '\\u4e2d')", ROPE_STEP, 70001 },
        /* a scan of 2097152 characters and a copy of as many, and a scan that stops at the first wide one */
        { "wide.slice(1)", SUB_STEP, 4194304 },
        /* the scan ends in the middle of a step, and the copy goes on from there */
        { "w15.slice(1)", SUB_STEP, 3 << 20 },
        { "wlast.slice(0, 3 << 20)", SUB_STEP, 6291456 },
        { "wlast.slice(1)", SUB_STEP, 6291456 },
        /* a string of 8 bits is copied, not scanned */
        { "nar.slice(3)", SUB_STEP, (3 << 20) + 2 },
        /* the rope of the space and nar is made flat first: its characters and one for each of its 2 strings */
        { "(' ' + nar).trim()", SUB_STEP, (3 << 20) + 5 + (3 << 20) + 6 + 2 },
        /* the hash of a key: leaves of 65534, 65536 and 65535 characters, and one for each leaf */
        { "s0.has(l1 + l2)", ROPE_STEP, 65534 + 65536 + 2 },
        { "s0.has(l3 + l2 + l1)", ROPE_STEP, 65535 + 65536 + 65534 + 3 },
        { "s0.has(l2 + l1 + l2 + l1)", ROPE_STEP, 2 * 65536 + 2 * 65534 + 4 },
        { "s0.has(l1 + l1 + l1 + l1 + l1)", ROPE_STEP, 5 * 65534 + 5 },
        { "s0.has(l4 + l1 + l3 + l2)", ROPE_STEP, 65533 + 65534 + 65535 + 65536 + 4 },
        { "s0.has(flat)", ROPE_STEP, 1 << 21 },
        /* equality: leaf by leaf, and a flat string in steps (each step counts one more than its characters, and a step is 65535) */
        { "fl === fl2", ROPE_STEP, (1 << 20) + 17 },
        /* ropes of 65534, 65536 and 65535 characters: 6 steps */
        { "ra === rb", ROPE_STEP, 196605 + 6 },
        { "ra < rb", ROPE_STEP, 196605 + 6 },
        { "mp.get(fl2)", ROPE_STEP, (1 << 21) + 17 },
        /* a key of a property: the hash, and the comparison with the atom of an equal string */
        { "ko[kx] = 1", ROPE_STEP, 1 << 21 },
        { "ko[ky2] = 2", ROPE_STEP, (1 << 22) + 33 },
        /* the tail of the hash and the head of the compare do not share a step: 131071 is 2 steps and 1 character, 65537 is 1 and 2 */
        { "hm.get(h2)", ROPE_STEP, 131071 + 131071 + 3 },
        { "ho[h2] = 2", ROPE_STEP, 131071 + 131071 + 3 },
        { "gm.get(g2)", ROPE_STEP, 65537 + 65537 + 2 },
        { "go[g2] = 2", ROPE_STEP, 65537 + 65537 + 2 },
        /* 64 keys of the hash of cq, each different from it in the first step: the compares of the chain share the budget and a compare that ends at a difference counts what it did */
        { "cm.get(cq)", ROPE_STEP, 65546 + 64 * 65536 },
        { "cm.has(cq)", ROPE_STEP, 65546 + 64 * 65536 },
        { "cm.delete(cq)", ROPE_STEP, 65546 + 64 * 65536 },
        { "co[cq] = 1", ROPE_STEP, 65546 + 64 * 65536 },
        /* 32 keys of 65536 characters (a step: compared in steps) and of 65535 (compared at once, the compares add up) */
        { "dm2.get(dq2)", ROPE_STEP, 33 * 65536 },
        { "dm2.has(dq2)", ROPE_STEP, 33 * 65536 },
        { "dm2.delete(dq2)", ROPE_STEP, 33 * 65536 },
        { "do2[dq2] = 1", ROPE_STEP, 33 * 65536 },
        { "dm1.get(dq1)", ROPE_STEP, 32 * 65535 },
        { "dm1.has(dq1)", ROPE_STEP, 32 * 65535 },
        { "dm1.delete(dq1)", ROPE_STEP, 32 * 65535 },
        /* a search for a string of the length of the one searched: the compare goes in steps */
        { "e1.indexOf(e2)", ROPE_STEP, 200000 },
        { "e1.indexOf(e3)", ROPE_STEP, 200000 },
        { "e1.lastIndexOf(e2)", ROPE_STEP, 200000 },
        { "e1.includes(e2)", ROPE_STEP, 200000 },
        { "e1.startsWith(e2)", ROPE_STEP, 200000 },
        { "e1.endsWith(e2)", ROPE_STEP, 200000 },
        { "e1.replace(e2, 'y').length", ROPE_STEP, 200000 },
        { "e1.split(e2).length", ROPE_STEP, 200000 },
        { "x1.indexOf(x2)", ROPE_STEP, 200000 },
        /* Array.prototype.indexOf, lastIndexOf and includes, and Object.is: equal strings of 200000 characters are 4 steps of 65535 */
        { "[1, e1].indexOf(e2)", ROPE_STEP, 200000 + 4 },
        { "[e1, 1].lastIndexOf(e2)", ROPE_STEP, 200000 + 4 },
        { "[1, e1].includes(e2)", ROPE_STEP, 200000 + 4 },
        { "Object.is(e1, e2)", ROPE_STEP, 200000 + 4 },
        /* the same string in 40 elements: strings of 65535 characters are compared at once, and the 40 compares add up; those of 65536 are compared in steps */
        { "ar1.indexOf(s1b)", ROPE_STEP, 40 * 65535 },
        { "ar1.includes(s1b)", ROPE_STEP, 40 * 65535 },
        { "ar2.indexOf(s2b)", ROPE_STEP, 40 * 65536 + 80 },
        { "ar2.includes(s2b)", ROPE_STEP, 40 * 65536 + 80 },
        { "ar1.lastIndexOf(s1b)", ROPE_STEP, 40 * 65535 },
        /* 32768 + 32768 for the two flat ones, 32768 for the first leaf of the rope and its one character, and 2 for the leaf and the step that ends at the difference */
        { "ar3.indexOf(q3)", ROPE_STEP, 32768 + 32768 + 32768 + 2 },
        { "ar3.includes(q3)", ROPE_STEP, 32768 + 32768 + 32768 + 2 },
        { "ar3r.lastIndexOf(q3)", ROPE_STEP, 32768 + 32768 + 32768 + 2 },
        /* JSON.parse: the C string (once for each character of an 8-bit string without escapes, and once more for a string that has to be converted), then the bytes of the string, escapes with their bytes, and the characters before a wide one when the buffer is widened */
        { "JSON.parse(ja)", ROPE_STEP + 5, 65539 + 65537 },
        { "JSON.parse(je)", ROPE_STEP + 5, 80002 + 80000 },
        { "JSON.parse(jm)", ROPE_STEP + 5, 98312 + 98310 },
        { "JSON.parse(ju)", ROPE_STEP + 5, 120002 + 120000 },
        { "JSON.parse(jw)", ROPE_STEP + 5, 70003 + 70003 + 70000 },
        /* a rope is made flat first: its 70003 characters and one for each of its 3 strings */
        { "JSON.parse(jwr)", ROPE_STEP + 5, 70003 + 70003 + 70000 + 70006 },
        { "JSON.parse(jx)", ROPE_STEP + 5, 70008 + 70006 + 70000 },
        /* the same for a key: and the hash of it */
        { "JSON.parse(jk)", ROPE_STEP + 5, 200006 + 200000 + 200000 },
        /* the text, then the bytes of each key and value, and for the wide text the characters before the wide one when the buffer is widened (the totals of the engine before keys and plain strings were read from the text, less the 2 narrow strings of the rope jy, which the setup now makes flat) */
        { "JSON.parse(jt).length", ROPE_STEP + 5, 96001 + 3000 * 18 },
        { "JSON.parse(jl).length", ROPE_STEP + 5, 2 * 96001 + 3000 * 19 },
        { "JSON.parse(jy).length", ROPE_STEP + 5, 177001 },
        /* a rope made flat (String): each string in steps, and one more for it; the strings of 8 bits of a wide rope are written into its wide buffer */
        { "String(rf1 + rf2).length", ROPE_STEP, 270000 + 2 },
        { "String(rf1 + rf3 + rf2).length", ROPE_STEP, 270600 + 3 },
        { "String(rf2 + rf1 + rf2).length", ROPE_STEP, 340000 + 3 },
        { "String(rf1 + rfw + rf2).length", ROPE_STEP, 370000 + 3 },
        /* repeat in steps: of one character, of two, of a string longer than a step (pieces that end at the end of the string or of the step), of a wide one */
        { "'x'.repeat(200000).length", ROPE_STEP, 200000 },
        { "'ab'.repeat(100000).length", ROPE_STEP, 200000 },
        { "rf2.repeat(3).length", ROPE_STEP, 210000 },
        { "rfw.repeat(2).length", ROPE_STEP, 200000 },
        { "'\\u4e2d'.repeat(70000).length", ROPE_STEP, 70000 },
        /* a rope appended to a string buffer (String.raw), which is widened at the wide string: the 200000 characters before it once more */
        { "String.raw(rraw, rf1 + rf2).length", ROPE_STEP, 270000 + 2 },
        { "String.raw(rraw, rf1 + rfw).length", ROPE_STEP, 300000 + 2 + 200000 },
        /* the same with fewer than a step before the wide string, which is widened at once with no reading of the flag */
        { "String.raw(rraw, rs1 + rsw).length", ROPE_STEP, 80000 + 2 + 40000 },
        /* JSON.stringify: runs, escapes and runs of wide characters */
        { "JSON.stringify(qa).length", ROPE_STEP, 200000 },
        { "JSON.stringify(qe).length", ROPE_STEP, 70000 },
        { "JSON.stringify(qm).length", ROPE_STEP, 3 * (32768 + 1) },
        /* the quote that opens the string is widened by the first run: 1 more */
        { "JSON.stringify(qw).length", ROPE_STEP + 1, 70000 + 1 },
        /* runs of narrow characters into a buffer that is wide already: each counts twice, once for the scan and once for the copy */
        { "JSON.parse(jn)", ROPE_STEP + 5, 262160 + 8 * 2 * 32768 + 6 + 16 },
        { "JSON.stringify(qn).length", ROPE_STEP + 1, 2 + 1 + 8 * 2 * 32768 + 8 },
        /* compiling a long regexp: the pattern as UTF-8, the code copied as it grows, the code scanned for its registers, for an atom with a quantifier and moved for an alternative or a quantifier, the pattern scanned for a name used before its group, and the code copied into its string */
        { "new RegExp(xa), 0", SUB_STEP, ANY },
        { "new RegExp(xg), 0", SUB_STEP, ANY },
        { "new RegExp(xo), 0", SUB_STEP, ANY },
        { "new RegExp(xk), 0", SUB_STEP, ANY },
        { "new RegExp(xw), 0", SUB_STEP, ANY },
        { "new RegExp(xd), 0", SUB_STEP, 4194312 },
        { "new RegExp(xu, 'u'), 0", SUB_STEP, 4194312 },
        { "new RegExp(xn), 0", SUB_STEP, 4194318 },
        /* the lookup in the cache: the hash of the pattern, and the compare with the one kept (one more for each leaf) */
        { "new RegExp(yc2), 0", ROPE_STEP, 900002 + 900002 + 14 },
        /* each string compared with the ones before it in their chain (the bytes compared and the strings passed, read every 16,384), and the pattern as UTF-8 */
        { "new RegExp(yq, 'v'), 0", ROPE_STEP, ANY },
        /* the characters of a class not in order merged into its ranges (the intervals passed, read every 16,384 bytes of them), the ranges turned into code, and the strings of a class put in a table that grows (the strings moved and the table cleared) */
        { "new RegExp(zd, 'u'), 0", SUB_STEP, ANY },
        { "new RegExp(zl, 'v'), 0", SUB_STEP, ANY },
        /* BigInts: the limbs of each row of a product or a division, each digit of a radix that is a power of two, and the limbs of each division of a conversion to or from radix 10, in steps of JS_BIGINT_MAX_SIZE (32768 limbs of 32 bits), and the string of BigInt as a C string first (in steps of 65536) */
        { "ba * bb, 0", BIG_STEP, 5001 * 10001 },
        { "ba / bb, 0", BIG_STEP, 5000 * 5001 },
        { "yb.toString(2).length", BIG_STEP, 200000 },
        /* the rope of sb2 made flat, its conversion to a C string, then the digits */
        { "BigInt(sb2), 0", ROPE_STEP, 200004 + 200002 + 200000 },
        { "String(xw).length", BIG_STEP, ANY },
        { "BigInt(sd), 0", BIG_STEP, ANY },
    };
    VM v = vm_new();
    size_t i;

    CHECK(eval_ok(v.ctx, setup), "the script threw");
    for (i = 0; i < countof(cases); i++) {
        unsigned long longest, total;

        white_work_reset();
        CHECK(eval_ok(v.ctx, cases[i].call), "%s: the script threw", cases[i].call);
        longest = white_work_longest();
        total = white_work_sum();
        CHECK(longest <= cases[i].step, "%s: %lu units of work between two readings of the flag, at most %lu", cases[i].call,
              longest, cases[i].step);
        CHECK(cases[i].total == ANY || total == cases[i].total, "%s: %lu units of work, want %lu", cases[i].call, total, cases[i].total);
    }
    vm_free(&v);
}

/* startTag and endTag read the interrupt flag before each step of a loop over
   a long string: JS_ESCAPE_STEP (4096) units for the escape, the hyphens of a
   style name and the white space before a value or a URL, JS_EMIT_COPY_STEP
   (524288) units for the scan of a tag or a style name and for a copy. A
   string of a step or less is done at once and not counted. The strings here
   keep every buffer under 1 MiB, so that no growth of a buffer is counted
   (test_buffer_copy_work has that). The keys of the props are made in the
   setup: the hash of a long key is counted too. */
static void test_tag_work_gaps(void)
{
    enum { E = 4096, C = 524288 };
    static const char setup[] =
        "var hop = Object.prototype.hasOwnProperty, d = {}, fc = { viewTransition: null }\n"
        "function st(p, type) { var t = []; if (startTag(t, p, type || 'div', fc, hop, d) === d || t.length !== 1) throw new Error('declined'); return t[0].length }\n"
        "var tg = 'a'.repeat(600000), tg2 = 'c'.repeat(600000), tg4 = 'e'.repeat(524286), tg5 = 'f'.repeat(524288)\n"
        "var pc1 = { style: { ['--' + '&'.repeat(100000)]: 1 } }, pc2 = { style: { ['--' + '<'.repeat(150000)]: 1 } }\n"
        "var ps1 = { style: { ['aB'.repeat(100000)]: 1 } }, ps2 = { style: { ['cD'.repeat(300000)]: 1 } }\n"
        "var c1 = '<'.repeat(100000), c2 = '>'.repeat(200000), c3 = '\\u0100&'.repeat(50000), a1 = '\"'.repeat(80000)\n"
        "var sp = [' '.repeat(10000), 'x', ' '.repeat(10000)].join(''), se = [' '.repeat(5000), '<'.repeat(5000)].join('')\n"
        "var ul = [' '.repeat(10000), 'x'].join(''), ut = ['j', '\\t'.repeat(10000), 'x'].join(''), uj = ['j', '\\t'.repeat(10000), 'avascript:x'].join('')\n"
        "var x8 = 'x'.repeat(8192), s8 = ' '.repeat(8192), u8 = [' '.repeat(8192), 'x'].join(''), uv = [' '.repeat(8192), 'javascript:x'].join(''), u4 = [' '.repeat(4096), 'x'].join('')\n"
        "0";
    static const struct {
        const char *call;
        unsigned long step, total;
        const char *want;
    } cases[] = {
        /* the scan of the tag, its copy and the copy of "<" + tag + ">" */
        { "st({}, tg)", C, 600000 + 600000 + 600002, "600002" },
        /* a tag of a step is scanned and copied at once, and so is a text of a step */
        { "st({}, tg4)", C, 0, "524288" },
        { "st({}, tg5)", C, 524290, "524290" },
        { "endTag(tg).length", C, 600000, "600003" },
        { "endTag({ toString() { return tg2 } }).length", C, 600000, "600003" },
        /* a custom style name: the escape; the name of 500002 units and the tag are copied at once */
        { "st(pc1)", E, 100002, "500018" },
        /* the escape, then the name of 600002 units copied out of its buffer, into the tag and the tag out of it */
        { "st(pc2)", C, 150002 + 600002 + 600002 + 600018, "600018" },
        /* a style name: the hyphens; the scan of 200000 is done at once */
        { "st(ps1)", E, 200000, "300018" },
        { "st(ps2)", C, 600000 + 600000 + 900000 + 900000 + 900018, "900018" },
        /* the text: the escape, and the tag copied out of its buffer when it is long */
        { "st({ children: c1 })", E, 100000, "400005" },
        { "st({ children: c2 })", C, 200000 + 800005, "800005" },
        /* a wide text: the entities are written into a wide buffer, which counts their units (5 for &amp;), and "<div>" is widened */
        { "st({ children: c3 })", 7 * E, 100000 + 5 + 50000 * 5, "300005" },
        { "st({ title: a1 })", E, 80000, "480014" },
        /* the white space around a style value, both ends together, and then the escape of what is left */
        { "st({ style: { color: sp } })", E, 20000, "21" },
        { "st({ style: { width: se } })", E, 5000 + 5000, "20020" },
        /* the white space and tabs isJavaScriptProtocol skips, and the escape of the URL */
        { "st({ href: ul }, 'a')", E, 10000 + 10001, "10012" },
        { "st({ href: ut }, 'a')", E, 10000 + 10002, "10013" },
        { "startTag([], { href: uj }, 'a', fc, hop, d) === d", E, 10000, "true" },
        /* a run of white space after a long escape: the escape reads the flag after its last block, so the two are not one stretch */
        { "st({ style: { color: x8, width: s8 } })", E, 8192 + 8192, "8219" },
        { "st({ title: x8, href: u8 }, 'a')", E, 8192 + 8192 + 8193, "16405" },
        /* a run of a step is skipped at once */
        { "st({ href: u4 }, 'a')", E, 4097, "4108" },
        /* a short wide value makes the buffer wide, which counts "<a title=\"" and the quote after it, and the flag is read before the white space of a long URL */
        { "startTag([], { title: '\\u0100', href: uv }, 'a', fc, hop, d) === d", E, 10 + 1 + 8192, "true" },
    };
    VM v = vm_new();
    JSValue g = JS_GetGlobalObject(v.ctx);
    size_t i;

    JS_SetPropertyStr(v.ctx, g, "startTag", JS_NewStartTagFunction(v.ctx, "startTag", 8));
    JS_SetPropertyStr(v.ctx, g, "endTag", JS_NewEndTagFunction(v.ctx, "endTag", 6));
    JS_FreeValue(v.ctx, g);
    CHECK(eval_ok(v.ctx, setup), "the setup threw");
    for (i = 0; i < countof(cases); i++) {
        unsigned long longest, total;
        char *got;

        white_work_reset();
        got = eval_text(v.ctx, cases[i].call);
        longest = white_work_longest();
        total = white_work_sum();
        CHECK(got != NULL, "%s: the script threw", cases[i].call);
        if (got && cases[i].want)
            CHECK(strcmp(got, cases[i].want) == 0, "%s: gave %s, want %s", cases[i].call, got, cases[i].want);
        free(got);
        CHECK(longest <= cases[i].step, "%s: %lu units of work between two readings of the flag, at most %lu", cases[i].call,
              longest, cases[i].step);
        CHECK(total == cases[i].total, "%s: %lu units of work, want %lu", cases[i].call, total, cases[i].total);
    }
    vm_free(&v);
}

/* A literal of 28 or 29 plain keys keeps its properties in its block (the
   header of 40 bytes and 29 properties of 16 are 504 bytes, the most a small
   block of the allocator holds), one of 30 does not. */
static void test_object_inline_limit(void)
{
    int n;

    for (n = 27; n <= 31; n++) {
        VM v = vm_new();
        char src[2048] = "var o = { ";
        int room = 0, k, inline_;
        char want[32];

        for (k = 0; k < n; k++)
            snprintf(src + strlen(src), sizeof(src) - strlen(src), "q%d: %d, ", k, k);
        strcat(src, "}; void 0");
        CHECK(eval_ok(v.ctx, src), "the script threw");
        inline_ = props_inline(&v, "o", &room);
        CHECK(inline_ == (n <= 29), "%d keys: in the block %d, room for %d", n, inline_, room);
        CHECK(room == n, "%d keys: room for %d", n, room);
        snprintf(want, sizeof(want), "%d:26", n);
        CHECK(eval_is(v.ctx, "Object.keys(o).length + ':' + o.q26", want), "values");
        vm_free(&v);
    }
}

static uintptr_t shape_of(VM *v, const char *expr)
{
    JSValue o = JS_Eval(v->ctx, expr, strlen(expr), "<unit>", JS_EVAL_TYPE_GLOBAL);
    uintptr_t r = white_shape_of(o);

    JS_FreeValue(v->ctx, o);
    return r;
}

/* the cache of the first object rest or spread of the global function fn,
   with the shapes of the targets of a spread in tgt (engine/regvm-ops-copy.h) */
static int copy_site2(VM *v, const char *fn, uintptr_t *src, uintptr_t *tgt, int max,
                      int *swaps)
{
    JSValue f = JS_Eval(v->ctx, fn, strlen(fn), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int n = white_copy_site(f, 0, src, tgt, max, swaps);

    JS_FreeValue(v->ctx, f);
    return n;
}

/* the same for a rest, which has no target */
static int copy_site(VM *v, const char *fn, uintptr_t *src, int max, int *swaps)
{
    JSValue f = JS_Eval(v->ctx, fn, strlen(fn), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int n = white_copy_site(f, 0, src, NULL, max, swaps);

    JS_FreeValue(v->ctx, f);
    return n;
}

/* An object rest keeps, at its place, the shapes of its sources and of
   their results: the result of a kept shape takes the shape an object built
   key by key gets, sources of another prototype than Object.prototype or
   null, with a symbol key or of a shape out of the shape hash table are not
   kept, a place that sees many shapes in turn keeps four and replaces them
   RV_COPY_SWAPS times, and the shapes kept stay through a collection and go
   with their function (the leak check of vm_free). */
static void test_object_rest_cache(void)
{
    int cache;

    for (cache = 0; cache <= 1; cache++) {
        VM v = vm_new();
        uintptr_t src[4];
        int swaps = -1, n;

        JS_SetShapeCache(v.ctx, cache ? 2048 : 0);
        CHECK(eval_ok(v.ctx, "function f(s) { const { a, ...r } = s; return r }\n"
                             "var o1 = { a: 1, b: 2, c: 3 }, r1 = f(o1), r2 = f({ a: 4, b: 5, c: 6 });\n"
                             "var k = {}; k.b = 0; k.c = 0; void 0"), "the script threw");
        n = copy_site(&v, "f", src, 4, &swaps);
        CHECK(n == 1 && src[0] == shape_of(&v, "o1"), "cache %d: one entry, the shape of o1: %d", cache, n);
        CHECK(shape_of(&v, "r1") == shape_of(&v, "r2") && shape_of(&v, "r1") == shape_of(&v, "k"),
              "cache %d: the result has the shape of an object built key by key", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify([r1, r2])", "[{\"b\":2,\"c\":3},{\"b\":5,\"c\":6}]"),
              "values of the results");

        /* not kept: another prototype, a symbol key, a shape out of the
           table */
        CHECK(eval_ok(v.ctx, "class C { constructor() { this.a = 1; this.b = 2 } }\n"
                             "var y = { a: 1, b: 2 }; Object.defineProperty(y, Symbol('s'), { value: 1 });\n"
                             "var d = { a: 1, x: 0, b: 2 }; delete d.x;\n"
                             "var r3 = [f(new C()), f(y), f(d)].map(x => JSON.stringify(x)).join(); void 0"),
              "the script threw");
        n = copy_site(&v, "f", src, 4, &swaps);
        CHECK(n == 1, "cache %d: a class instance, a symbol key or a deleted key kept: %d entries", cache, n);
        CHECK(eval_is(v.ctx, "r3", "{\"b\":2},{\"b\":2},{\"b\":2}"), "values of the sources not kept");

        /* a null prototype is kept */
        CHECK(eval_ok(v.ctx, "var np = Object.create(null); np.a = 1; np.b = 2; var r4 = f(np); void 0"),
              "the script threw");
        n = copy_site(&v, "f", src, 4, &swaps);
        CHECK(n == 2 && src[1] == shape_of(&v, "np"), "cache %d: a null prototype: %d entries", cache, n);
        CHECK(eval_is(v.ctx, "JSON.stringify(r4) + Object.getPrototypeOf(r4).constructor.name", "{\"b\":2}Object"),
              "the result of a null prototype source");

        /* seven shapes in turn */
        CHECK(eval_ok(v.ctx, "function g(s) { const { k0, ...r } = s; return r }\n"
                             "var os = []; for (let n = 1; n <= 7; n++) { const o = {}; for (let i = 0; i < n; i++) o['k' + i] = i; os.push(o) }\n"
                             "var sum = 0; for (let t = 0; t < 30; t++) for (const o of os) sum += Object.keys(g(o)).length; void 0"),
              "the script threw");
        n = copy_site(&v, "g", src, 4, &swaps);
        CHECK(n == 4 && swaps == 16, "cache %d: seven shapes: %d entries, %d swaps", cache, n, swaps);
        CHECK(eval_is(v.ctx, "String(sum)", "630"), "the keys of the results of seven shapes");

        JS_RunGC(v.rt);
        CHECK(eval_is(v.ctx, "JSON.stringify(f({ a: 1, b: 2, c: 3 }))", "{\"b\":2,\"c\":3}"),
              "a kept shape after a collection");
        CHECK(eval_ok(v.ctx, "f = g = null; void 0"), "the script threw");
        JS_RunGC(v.rt);
        vm_free(&v);
    }
}

/* The spread of an object literal keeps, at its place, the shapes of its
   target and its source and of the result: a target with keys before the
   spread, a key of the source that the target has stays in its place with
   the value of the source, the result takes the shape an object built key
   by key gets, sources and targets of another prototype, with a symbol or a
   getter on either side are not kept, a place that sees many
   shapes in turn keeps four, and the shapes kept go with their function
   (the leak check of vm_free). */
static void test_object_spread_cache(void)
{
    int cache;

    for (cache = 0; cache <= 1; cache++) {
        VM v = vm_new();
        uintptr_t src[4], tgt[4];
        int swaps = -1, n;

        JS_SetShapeCache(v.ctx, cache ? 2048 : 0);
        CHECK(eval_ok(v.ctx, "function f(s) { return { a: 0, z: 0, ...s, w: 1 } }\n"
                             "var o1 = { x: 1, a: 2 }, r1 = f(o1), r2 = f({ x: 3, a: 4 });\n"
                             "var k = {}; k.a = 0; k.z = 0; k.x = 0; k.w = 0; var tz = { a: 9, z: 9 }; void 0"),
              "the script threw");
        n = copy_site2(&v, "f", src, tgt, 4, &swaps);
        CHECK(n == 1 && src[0] == shape_of(&v, "o1") && tgt[0] == shape_of(&v, "tz"),
              "cache %d: one entry, the shapes of o1 and of the target: %d", cache, n);
        CHECK(shape_of(&v, "r1") == shape_of(&v, "r2") && shape_of(&v, "r1") == shape_of(&v, "k"),
              "cache %d: the result has the shape of an object built key by key", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify([r1, r2])",
                      "[{\"a\":2,\"z\":0,\"x\":1,\"w\":1},{\"a\":4,\"z\":0,\"x\":3,\"w\":1}]"),
              "values of the results");

        /* an empty target takes the shape of a plain source */
        CHECK(eval_ok(v.ctx, "function e(s) { return { ...s } }\n"
                             "var o2 = { p: 1, q: 2 }, r3 = e(o2), r4 = e(o2); void 0"), "the script threw");
        n = copy_site2(&v, "e", src, tgt, 4, &swaps);
        CHECK(n == 1 && shape_of(&v, "r3") == shape_of(&v, "o2") && shape_of(&v, "r4") == shape_of(&v, "o2"),
              "cache %d: an empty target: %d entries", cache, n);

        /* not kept: a class instance, a symbol, a getter of the source or
           of the target */
        CHECK(eval_ok(v.ctx, "class C { constructor() { this.x = 1 } }\n"
                             "var y = { x: 1 }; y[Symbol('s')] = 2;\n"
                             "var g = { get x() { return 5 } };\n"
                             "function t(s) { return { a: 1, get b() { return 2 }, ...s } }\n"
                             "var r5 = [f(new C()), f(y), f(g), t({ b: 3 }), t({ b: 3 })].map(x => JSON.stringify(x)).join(); void 0"),
              "the script threw");
        n = copy_site2(&v, "f", src, tgt, 4, &swaps);
        CHECK(n == 1, "cache %d: a class instance, a symbol or a getter kept: %d entries", cache, n);
        CHECK(eval_is(v.ctx, "r5", "{\"a\":0,\"z\":0,\"x\":1,\"w\":1},{\"a\":0,\"z\":0,\"x\":1,\"w\":1},"
                                   "{\"a\":0,\"z\":0,\"x\":5,\"w\":1},{\"a\":1,\"b\":3},{\"a\":1,\"b\":3}"),
              "values of the spreads not kept");

        /* seven shapes in turn */
        CHECK(eval_ok(v.ctx, "function h(s) { return { k0: -1, ...s } }\n"
                             "var os = []; for (let n = 1; n <= 7; n++) { const o = {}; for (let i = 0; i < n; i++) o['k' + i] = i; os.push(o) }\n"
                             "var sum = 0; for (let t = 0; t < 30; t++) for (const o of os) { const r = h(o); sum += Object.keys(r).length + r.k0 } void 0"),
              "the script threw");
        n = copy_site2(&v, "h", src, tgt, 4, &swaps);
        CHECK(n == 4 && swaps == 16, "cache %d: seven shapes: %d entries, %d swaps", cache, n, swaps);
        CHECK(eval_is(v.ctx, "String(sum)", "840"), "the keys of the results of seven shapes");

        JS_RunGC(v.rt);
        CHECK(eval_is(v.ctx, "JSON.stringify(f({ x: 1, a: 2 }))", "{\"a\":2,\"z\":0,\"x\":1,\"w\":1}"),
              "a kept shape after a collection");
        CHECK(eval_ok(v.ctx, "f = e = h = t = null; void 0"), "the script threw");
        JS_RunGC(v.rt);
        vm_free(&v);
    }
}

/* Object literals of the same keys in the same order share one shape, made
   at whichever ran first, wherever they are written and whether or not the
   empty shape of Object.prototype and the shapes in between exist: with the
   shape cache on and off. A shape an object got key by key is taken too, also
   when it has room for more properties than the literal has keys. Keys in
   another order or with other flags are another shape. */
static void test_object_lit_shared_shape(void)
{
    int cache;

    for (cache = 0; cache <= 1; cache++) {
        VM v = vm_new();
        int room = -1;

        JS_SetShapeCache(v.ctx, cache ? 2048 : 0);
        CHECK(eval_ok(v.ctx, "function f1() { return { zz1: 1 } }\n"
                             "function f2() { return { zz1: 2 } }\n"
                             "var a = f1(), b = f2(), c = f1(); void 0"), "the script threw");
        CHECK(shape_of(&v, "a") != 0 && shape_of(&v, "a") == shape_of(&v, "b"),
              "cache %d: {zz1} in two functions has two shapes", cache);
        CHECK(shape_of(&v, "a") == shape_of(&v, "c"), "cache %d: one literal, two shapes", cache);
        CHECK(eval_is(v.ctx, "a.zz1 + ',' + b.zz1 + ',' + Object.keys(b)", "1,2,zz1"), "values of {zz1}");

        /* three keys, two places, and a literal written by eval */
        CHECK(eval_ok(v.ctx, "function g1(x) { return { yy1: x, yy2: 2, yy3: 3 } }\n"
                             "function g2(x) { return { yy1: x, yy2: 'b', yy3: [3] } }\n"
                             "var d = g1(1), e = g2(2), f = eval('({ yy1: 0, yy2: 0, yy3: 0 })'); void 0"),
              "the script threw");
        CHECK(shape_of(&v, "d") == shape_of(&v, "e") && shape_of(&v, "d") == shape_of(&v, "f"),
              "cache %d: {yy1, yy2, yy3} in three places", cache);

        /* the order of the keys and their flags are part of the shape */
        CHECK(eval_ok(v.ctx, "var p = (() => ({ pp1: 1, pp2: 2 }))(), q = (() => ({ pp2: 1, pp1: 2 }))();\n"
                             "var fl = {}; Object.defineProperty(fl, 'vv1', { value: 1, enumerable: true });\n"
                             "var lit = (() => ({ vv1: 2 }))(); void 0"), "the script threw");
        CHECK(shape_of(&v, "p") != shape_of(&v, "q"), "cache %d: keys in another order share a shape", cache);
        CHECK(shape_of(&v, "fl") != shape_of(&v, "lit"), "cache %d: a key with other flags shares a shape", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify(Object.getOwnPropertyDescriptor(lit, 'vv1'))",
                      "{\"value\":2,\"writable\":true,\"enumerable\":true,\"configurable\":true}"),
              "the key of the literal");
        CHECK(eval_is(v.ctx, "lit.vv1 = 5; delete lit.vv1; JSON.stringify(lit) + JSON.stringify(fl)", "{}{\"vv1\":1}"),
              "the literal after a write and a delete");

        /* a shape built key by key, with room for 6 properties, kept by its
           object: the literal of 5 keys takes it, room and all, and keeps it
           when the object grows on */
        CHECK(eval_ok(v.ctx, "var kk = {}; kk.kk1 = 1; kk.kk2 = 2; kk.kk3 = 3; kk.kk4 = 4; kk.kk5 = 5;\n"
                             "function lit5() { return { kk1: 'a', kk2: 'b', kk3: 'c', kk4: 'd', kk5: 'e' } }\n"
                             "var l1 = lit5(); kk.kk6 = 6; kk.kk7 = 7; var l2 = lit5(); void 0"),
              "the script threw");
        CHECK(props_inline(&v, "kk", &room) == 0 && room > 6, "cache %d: kk grown: room %d", cache, room);
        CHECK(props_inline(&v, "l1", &room) == 1 && room == 6,
              "cache %d: the literal of the shape of kk: room %d", cache, room);
        CHECK(shape_of(&v, "l1") == shape_of(&v, "l2") && shape_of(&v, "l1") != shape_of(&v, "kk"),
              "cache %d: the literal kept its shape when kk grew", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify([l1, l2, kk])",
                      "[{\"kk1\":\"a\",\"kk2\":\"b\",\"kk3\":\"c\",\"kk4\":\"d\",\"kk5\":\"e\"},"
                      "{\"kk1\":\"a\",\"kk2\":\"b\",\"kk3\":\"c\",\"kk4\":\"d\",\"kk5\":\"e\"},"
                      "{\"kk1\":1,\"kk2\":2,\"kk3\":3,\"kk4\":4,\"kk5\":5,\"kk6\":6,\"kk7\":7}]"),
              "values of the objects");
        CHECK(eval_is(v.ctx, "l1.kk6 = 'f'; l1.kk7 = 'g'; Object.keys(l1) + ' ' + Object.keys(l2) + ' ' + l1.kk7",
                      "kk1,kk2,kk3,kk4,kk5,kk6,kk7 kk1,kk2,kk3,kk4,kk5 g"), "the literal grows on its own");

        /* an object of one literal that got a key turns off the room in the
           block for the next objects of every literal of its shape */
        CHECK(eval_ok(v.ctx, "function h1() { return { ww1: 1, ww2: 2 } }\n"
                             "function h2() { return { ww1: 3, ww2: 4 } }\n"
                             "var x1 = h1(), x2 = h2(); x1.ww3 = 3; var x3 = h2(); void 0"), "the script threw");
        CHECK(props_inline(&v, "x2", &room) == 1, "cache %d: an object made before the key", cache);
        CHECK(props_inline(&v, "x3", &room) == 0, "cache %d: an object of the other literal made after the key", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify([x1, x2, x3])",
                      "[{\"ww1\":1,\"ww2\":2,\"ww3\":3},{\"ww1\":3,\"ww2\":4},{\"ww1\":3,\"ww2\":4}]"), "values of the objects");
        vm_free(&v);
    }
}

/* o[k] = v with a key that is its own atom adds the key as the generic path
   does: objects that get the same keys in the same order by o[k] with keys
   from Object.keys, by o[k] with the same keys made again (not atoms), by
   o.x = v, by Reflect.set and by Object.assign share one shape, empty and
   from a literal, with the shape cache on and off, past the room of the
   first property array and of the block of a literal. A key on the
   prototype (a writable data property) is added as an own key of the same
   shape, and keys in another order are another shape. */
static void test_putel_atom_shape(void)
{
    int cache;

    for (cache = 0; cache <= 1; cache++) {
        VM v = vm_new();

        JS_SetShapeCache(v.ctx, cache ? 2048 : 0);
        CHECK(eval_ok(v.ctx,
                      "var names = Object.keys({ ka: 0, kb: 0, kc: 0, kd: 0, ke: 0, kf: 0, kg: 0, kh: 0, ki: 0, kj: 0 });\n"
                      "function fresh(k) { return (' ' + k).slice(1) }\n"
                      "function put(o, k, v) { o[k] = v }\n"
                      "function byAtom(o) { for (var i = 0; i < names.length; i++) put(o, names[i], i); return o }\n"
                      "function byFresh(o) { for (var i = 0; i < names.length; i++) put(o, fresh(names[i]), i); return o }\n"
                      "function byField(o) { o.ka = 0; o.kb = 1; o.kc = 2; o.kd = 3; o.ke = 4; o.kf = 5; o.kg = 6;\n"
                      "  o.kh = 7; o.ki = 8; o.kj = 9; return o }\n"
                      "function byReflect(o) { for (var i = 0; i < names.length; i++) Reflect.set(o, names[i], i); return o }\n"
                      "function lit() { return { la: 1, lb: 2 } }\n"
                      "var a1 = byAtom({}), a2 = byFresh({}), a3 = byField({}), a4 = byReflect({});\n"
                      "var a5 = Object.assign({}, a1), a6 = byAtom({});\n"
                      "var l1 = byAtom(lit()), l2 = byFresh(lit()), l3 = byField(lit());\n"
                      "var inh = { ka: 'p', kb: 'p' }, i1 = byAtom(Object.create(inh)), i2 = byFresh(Object.create(inh));\n"
                      "var r1 = {}; for (var j = names.length - 1; j >= 0; j--) put(r1, names[j], j);\n"
                      "void 0"), "the script threw");
        CHECK(shape_of(&v, "a1") != 0 && shape_of(&v, "a1") == shape_of(&v, "a2"),
              "cache %d: atom keys and keys made again: two shapes", cache);
        CHECK(shape_of(&v, "a1") == shape_of(&v, "a3"), "cache %d: atom keys and o.x: two shapes", cache);
        CHECK(shape_of(&v, "a1") == shape_of(&v, "a4"), "cache %d: atom keys and Reflect.set: two shapes", cache);
        CHECK(shape_of(&v, "a1") == shape_of(&v, "a5"), "cache %d: atom keys and Object.assign: two shapes", cache);
        CHECK(shape_of(&v, "a1") == shape_of(&v, "a6"), "cache %d: atom keys twice: two shapes", cache);
        CHECK(shape_of(&v, "l1") == shape_of(&v, "l2") && shape_of(&v, "l1") == shape_of(&v, "l3"),
              "cache %d: a literal grown three ways: more than one shape", cache);
        CHECK(shape_of(&v, "i1") == shape_of(&v, "i2"), "cache %d: keys of the prototype added: two shapes", cache);
        CHECK(shape_of(&v, "r1") != shape_of(&v, "a1"), "cache %d: keys in another order share a shape", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify([a1, l1, i1]) + Object.keys(r1)",
                      "[{\"ka\":0,\"kb\":1,\"kc\":2,\"kd\":3,\"ke\":4,\"kf\":5,\"kg\":6,\"kh\":7,\"ki\":8,\"kj\":9},"
                      "{\"la\":1,\"lb\":2,\"ka\":0,\"kb\":1,\"kc\":2,\"kd\":3,\"ke\":4,\"kf\":5,\"kg\":6,\"kh\":7,\"ki\":8,\"kj\":9},"
                      "{\"ka\":0,\"kb\":1,\"kc\":2,\"kd\":3,\"ke\":4,\"kf\":5,\"kg\":6,\"kh\":7,\"ki\":8,\"kj\":9}]"
                      "kj,ki,kh,kg,kf,ke,kd,kc,kb,ka"),
              "values of the objects");
        vm_free(&v);
    }
}

/* ---- the kept shapes let go on a refused request ---- */

#define SLOTS_MAX 4096

/* A block whose size has no free slot left needs a new arena. Under the
   memory limit the heap refuses it, and js_heap_drain lets the kept shapes
   go: a kept shape in an arena that still holds other blocks leaves a free
   slot there, which gives no memory back to the heap, and the block is
   taken from that slot. The keys are atoms of the engine, so that the
   shapes are all that goes. */
static void test_shape_drop_slot(void)
{
    static void *slots[SLOTS_MAX];
    VM v;
    void *shape, *p;
    size_t size;
    int n, kept;

    if (white_every_block_is_a_request())
        return;
    v = vm_new();
    JS_SetShapeCache(v.ctx, 64);
    CHECK(eval_ok(v.ctx, "function mk() { var o = {}; o.length = 1; o.name = 2; return 0 }\nmk(); mk();"),
          "the script threw");
    JS_RunGC(v.rt);
    kept = white_kept_shapes(v.rt);
    shape = white_kept_shape(v.rt, &size);
    CHECK(kept > 0 && shape, "no shape kept");
    n = white_take_slots(v.rt, size, slots, SLOTS_MAX);
    CHECK(n >= 0, "more than %d free blocks of %lu bytes", SLOTS_MAX, (unsigned long)size);
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt));
    p = js_malloc_rt(v.rt, size);
    JS_SetMemoryLimit(v.rt, (size_t)-1);
    CHECK(white_kept_shapes(v.rt) == 0, "%d shapes kept after the refused request, %d before", white_kept_shapes(v.rt), kept);
    CHECK(p == shape, "a block of %lu bytes: %p, want the slot the kept shape left, %p", (unsigned long)size, p, shape);
    js_free_rt(v.rt, p);
    while (n > 0)
        js_free_rt(v.rt, slots[--n]);
    vm_free(&v);
}

/* add_property takes a shape it finds in the shape hash table that the
   shape cache alone holds, and grows the property array of the object to its
   room. That block needs a new arena, whose request runs js_heap_drain first,
   as a refused one does: the drain lets the kept shapes go, the one found
   among them, while add_property holds it, and the object gets it whole (the sanitizer builds see any read of it after it
   was freed, and the runtime frees with no block left). */
static void test_shape_drop_in_resize(void)
{
    static void *slots[SLOTS_MAX];
    VM v;
    JSValue g, o;
    size_t size;
    int n, kept, r;

    if (white_every_block_is_a_request())
        return;
    v = vm_new();
    JS_SetShapeCache(v.ctx, 64);
    CHECK(eval_ok(v.ctx,
                  "function mk() { var o = {}; o.length = 1; o.name = 2; o.value = 3; o.done = 4; return 0 }\n"
                  "mk(); mk();\n"
                  "var o = {}; o.length = 1; o.name = 2;"),
          "the script threw");
    JS_RunGC(v.rt);
    g = JS_GetGlobalObject(v.ctx);
    o = JS_GetPropertyStr(v.ctx, g, "o");
    size = white_found_resize(v.ctx, o, "value");
    kept = white_kept_shapes(v.rt);
    CHECK(size > 0 && kept > 0, "adding value to o finds no shape held by the cache alone with more room");
    n = white_take_slots(v.rt, size, slots, SLOTS_MAX);
    CHECK(n >= 0, "more than %d free blocks of %lu bytes", SLOTS_MAX, (unsigned long)size);
    white_drain_once();
    r = JS_SetPropertyStr(v.ctx, o, "value", JS_NewInt32(v.ctx, 3));
    CHECK(r == 1, "the property was not added: %d", r);
    /* the shape o left once it had the new one is the only one kept */
    CHECK(kept > 1 && white_kept_shapes(v.rt) == 1, "%d shapes kept after the refused request, %d before", white_kept_shapes(v.rt), kept);
    while (n > 0)
        js_free_rt(v.rt, slots[--n]);
    JS_FreeValue(v.ctx, o);
    JS_FreeValue(v.ctx, g);
    CHECK(eval_is(v.ctx, "Object.keys(o) + ':' + o.value + ':' + mk() + ':' + JSON.stringify(o)",
                  "length,name,value:3:0:{\"length\":1,\"name\":2,\"value\":3}"),
          "o after the property was added");
    vm_free(&v);
}

/* The write of a name o.done = v keeps the step of the key it added: the
   shape it gave the object, with no reference (RV_PUTF,
   engine/regvm-ops-basic.h). The next object of the shape before takes that
   shape at once, and when its property array must grow for it, rv_putf_step
   asks for the new array first. Once the objects of the shape are gone, the
   shape cache alone holds it, and that request runs js_heap_drain, which
   lets it go: the step must hold it before the request, as add_property
   holds the shape it finds (test_shape_drop_in_resize), or the object takes
   a shape that is freed, and whatever takes its block next has a count one
   short (the sanitizer builds see the read of the freed block, and the
   others the shapes made after it, and the runtime that frees with no block
   left). */
static void test_shape_drop_in_step(void)
{
    static void *slots[SLOTS_MAX];
    VM v;
    JSValue g, o, s, r;
    uint32_t w[4] = { 0, 0, 0, 0 };
    size_t size;
    int n, kept;

    if (white_every_block_is_a_request())
        return;
    v = vm_new();
    JS_SetShapeCache(v.ctx, 64);
    CHECK(eval_ok(v.ctx,
                  "function s(o) { o.done = 4; return 0 }\n"
                  "function mk() { var o = {}; o.length = 1; o.name = 2; o.value = 3; return o }\n"
                  "s(mk()); s(mk());\n"
                  "var o = mk();"),
          "the script threw");
    JS_RunGC(v.rt);
    g = JS_GetGlobalObject(v.ctx);
    o = JS_GetPropertyStr(v.ctx, g, "o");
    s = JS_GetPropertyStr(v.ctx, g, "s");
    CHECK(white_regvm_putf_sites(s, w, 1) == 1 && w[3] == white_shape_step_epoch(v.rt),
          "the write of s keeps no step of this epoch (ids %u %u, epoch %u of %u)",
          w[1], w[2], w[3], white_shape_step_epoch(v.rt));
    size = white_found_resize(v.ctx, o, "done");
    kept = white_kept_shapes(v.rt);
    CHECK(size > 0 && kept > 0, "adding done to o finds no shape held by the cache alone with more room");
    n = white_take_slots(v.rt, size, slots, SLOTS_MAX);
    CHECK(n >= 0, "more than %d free blocks of %lu bytes", SLOTS_MAX, (unsigned long)size);
    white_drain_once();
    r = JS_Call(v.ctx, s, JS_UNDEFINED, 1, &o);
    CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_INT && JS_VALUE_GET_INT(r) == 0, "s(o) threw");
    JS_FreeValue(v.ctx, r);
    /* the shape o left once it had the new one is the only one kept */
    CHECK(kept > 1 && white_kept_shapes(v.rt) == 1, "%d shapes kept after the refused request, %d before",
          white_kept_shapes(v.rt), kept);
    while (n > 0)
        js_free_rt(v.rt, slots[--n]);
    JS_FreeValue(v.ctx, s);
    JS_FreeValue(v.ctx, o);
    JS_FreeValue(v.ctx, g);
    /* shapes of the size of the one o has, made after it */
    CHECK(eval_is(v.ctx,
                  "var more = []; for (var i = 0; i < 64; i++) { var m = mk(); m.done = i; more.push(m, { p: i, q: 1, r: 2, t: 3 }) }\n"
                  "Object.keys(o) + ':' + o.done + ':' + JSON.stringify(o) + ':' + more.length",
                  "length,name,value,done:4:{\"length\":1,\"name\":2,\"value\":3,\"done\":4}:128"),
          "o after the property was added");
    vm_free(&v);
}

/* The same write, when the heap refuses the request of the step again after
   the drain: the drain lets go the reference of the cache to the shape the
   step found, so the step holds the only one, and gives it back on the way
   out. The call throws out of memory, the object keeps the shape it had, and
   the shape found goes to the cache again whole: the next write of the key
   gives it to the object (the sanitizer builds see any read of a freed block,
   and the runtime that frees with no block left any reference kept or
   lost). */
static void test_shape_drop_in_step_refused(void)
{
    static void *slots[SLOTS_MAX];
    VM v;
    JSValue g, o, s, r;
    uint32_t w[4] = { 0, 0, 0, 0 };
    uintptr_t before, back;
    size_t size, back_size;
    int n, kept;

    if (white_every_block_is_a_request())
        return;
    v = vm_new();
    JS_SetShapeCache(v.ctx, 64);
    CHECK(eval_ok(v.ctx,
                  "function s(o) { o.done = 4; return 0 }\n"
                  "function mk() { var o = {}; o.length = 1; o.name = 2; o.value = 3; return o }\n"
                  "s(mk()); s(mk());\n"
                  "var o = mk();"),
          "the script threw");
    JS_RunGC(v.rt);
    g = JS_GetGlobalObject(v.ctx);
    o = JS_GetPropertyStr(v.ctx, g, "o");
    s = JS_GetPropertyStr(v.ctx, g, "s");
    CHECK(white_regvm_putf_sites(s, w, 1) == 1 && w[3] == white_shape_step_epoch(v.rt),
          "the write of s keeps no step of this epoch (ids %u %u, epoch %u of %u)",
          w[1], w[2], w[3], white_shape_step_epoch(v.rt));
    before = shape_of(&v, "o");
    size = white_found_resize(v.ctx, o, "done");
    kept = white_kept_shapes(v.rt);
    CHECK(size > 0 && kept > 1, "adding done to o finds no shape held by the cache alone with more room");
    n = white_take_slots(v.rt, size, slots, SLOTS_MAX);
    CHECK(n >= 0, "more than %d free blocks of %lu bytes", SLOTS_MAX, (unsigned long)size);
    white_drain_once_refused();
    r = JS_Call(v.ctx, s, JS_UNDEFINED, 1, &o);
    CHECK(JS_IsException(r) && JS_IsUncatchableException(v.ctx), "s(o) with the request refused: not out of memory");
    JS_FreeValue(v.ctx, JS_GetException(v.ctx));
    /* the drain let the others go, and the step gave back the one it found */
    back = (uintptr_t)white_kept_shape(v.rt, &back_size);
    CHECK(white_kept_shapes(v.rt) == 1 && back != 0, "%d shapes kept after the refused request, %d before",
          white_kept_shapes(v.rt), kept);
    CHECK(shape_of(&v, "o") == before, "o took another shape in the write that failed");
    while (n > 0)
        js_free_rt(v.rt, slots[--n]);
    JS_FreeValue(v.ctx, s);
    JS_FreeValue(v.ctx, o);
    JS_FreeValue(v.ctx, g);
    CHECK(eval_is(v.ctx, "Object.keys(o) + ':' + o.done + ':' + JSON.stringify(o)",
                  "length,name,value:undefined:{\"length\":1,\"name\":2,\"value\":3}"),
          "o after the refused write");
    CHECK(eval_ok(v.ctx, "s(o)") && shape_of(&v, "o") == back, "the write again: o does not take the shape the step gave back");
    /* shapes of the size of the one o has, made after it */
    CHECK(eval_is(v.ctx,
                  "var more = []; for (var i = 0; i < 64; i++) { var m = mk(); s(m); m.done = i; more.push(m, { p: i, q: 1, r: 2, t: 3 }) }\n"
                  "Object.keys(o) + ':' + o.done + ':' + JSON.stringify(o) + ':' + more.length",
                  "length,name,value,done:4:{\"length\":1,\"name\":2,\"value\":3,\"done\":4}:128"),
          "o after the write was made again");
    vm_free(&v);
}

/* ---- the shape cache and the prototype it holds ---- */

/* The cache holds the prototype of the shapes it keeps. Letting them go in a
   drain of the heap never frees it, so no finalizer runs inside an
   allocation. JS_SetShapeCache lets the prototype go after the shapes, once
   the cache is off, and the finalizer of the prototype may set the cache
   again: the call that let it go sets its own, and the one of the finalizer
   goes. */
static int reenter_finalized;
static JSContext *reenter_ctx;
static int reenter_n;

static void reenter_finalizer(JSRuntime *rt, JSValue val)
{
    reenter_finalized++;
    if (reenter_ctx)
        JS_SetShapeCache(reenter_ctx, reenter_n);
}

/* the finalizer of put_back, which puts put_back_proto back as the
   Object.prototype of put_back_ctx while the collector frees objects */
static JSContext *put_back_ctx;
static JSValue put_back_proto;

static void put_back_finalizer(JSRuntime *rt, JSValue val)
{
    JS_SetClassProto(put_back_ctx, white_class_object(), put_back_proto);
}

/* reenter_setup makes p, of a class of the host whose finalizer is
   reenter_finalizer, the Object.prototype of v, turns the shape cache on
   with n shapes and makes plain objects. Then it lets go of an object whose
   finalizer puts the Object.prototype back, while the collector frees
   objects, and of p: the cache keeps its shapes and p, which nothing else
   holds. It returns the number of kept shapes. */
static int reenter_setup(VM *v, int n)
{
    JSClassDef def = { "Reenter", reenter_finalizer };
    JSClassDef put_back = { "PutBack", put_back_finalizer };
    JSClassID class_id = 0, put_back_id = 0;
    JSValue p, x;
    int kept;

    reenter_finalized = 0;
    reenter_ctx = NULL;
    JS_NewClassID(v->rt, &class_id);
    CHECK(JS_NewClass(v->rt, class_id, &def) == 0, "JS_NewClass");
    JS_NewClassID(v->rt, &put_back_id);
    CHECK(JS_NewClass(v->rt, put_back_id, &put_back) == 0, "JS_NewClass");
    put_back_ctx = v->ctx;
    put_back_proto = JS_GetClassProto(v->ctx, white_class_object());
    p = JS_NewObjectClass(v->ctx, class_id);
    CHECK(JS_IsObject(p), "the prototype");
    JS_SetClassProto(v->ctx, white_class_object(), JS_DupValue(v->ctx, p));
    CHECK(JS_SetShapeCache(v->ctx, n) == 0, "a shape cache of %d shapes", n);
    CHECK(eval_ok(v->ctx, "function mk() { var o = {}; o.qa = 1; o.qb = 2; return 0 }\nmk(); mk(); void 0"), "the objects");
    x = JS_NewObjectClass(v->ctx, put_back_id);
    CHECK(JS_IsObject(x), "the object that puts the prototype back");
    JS_FreeValue(v->ctx, x);
    CHECK(eval_is(v->ctx, "String(Object.getPrototypeOf({}) === Object.prototype)", "true"),
          "the Object.prototype was not put back");
    /* the shapes of p stay, and the empty one the context held for p may
       join them */
    kept = white_kept_shapes(v->rt);
    CHECK(kept > 0, "no shape kept after the prototype was replaced in a collection");
    JS_FreeValue(v->ctx, p);
    CHECK(reenter_finalized == 0, "the prototype the cache holds was finalized");
    return kept;
}

/* a drain of the heap lets the shapes go, and not the prototype */
static void test_shape_cache_set_in_drop(void)
{
    VM v = vm_new();
    void *p;
    int kept;

    kept = reenter_setup(&v, 1);
    CHECK(kept == 1, "%d shapes kept in a ring of 1", kept);
    reenter_ctx = v.ctx;
    reenter_n = 128;
    white_drain_once();
    /* a block too large for an arena is a request to the heap */
    p = js_malloc_rt(v.rt, 1 << 20);
    CHECK(p != NULL, "a block of 1 MiB");
    js_free_rt(v.rt, p);
    reenter_ctx = NULL;
    CHECK(reenter_finalized == 0, "the prototype was finalized %d times in a drain of the heap", reenter_finalized);
    CHECK(white_kept_shapes(v.rt) == 0 && white_shape_cache_size(v.rt) == 1,
          "%d shapes kept in a cache of %d after the drain, want 0 in 1", white_kept_shapes(v.rt), white_shape_cache_size(v.rt));
    /* the new objects have the Object.prototype put back, not the one the
       cache holds */
    CHECK(eval_ok(v.ctx, "mk(); mk(); void 0") && white_kept_shapes(v.rt) == 0, "the cache keeps a shape of another prototype");
    vm_free(&v);
    CHECK(reenter_finalized == 1, "the prototype was finalized %d times in all", reenter_finalized);
}

/* JS_SetShapeCache lets the shapes go, then the prototype, and the finalizer
   of the prototype sets a cache of 128 shapes: the call that let it go sets
   its own, and the one of the finalizer goes */
static void test_shape_cache_set_in_off(void)
{
    VM v = vm_new();
    int kept, r;

    kept = reenter_setup(&v, 4);
    CHECK(kept > 0, "no shape kept");
    reenter_ctx = v.ctx;
    reenter_n = 128;
    r = JS_SetShapeCache(v.ctx, 16);
    reenter_ctx = NULL;
    CHECK(r == 0, "JS_SetShapeCache: %d", r);
    CHECK(reenter_finalized == 1, "the prototype was finalized %d times", reenter_finalized);
    CHECK(white_shape_cache_size(v.rt) == 16, "a cache of %d shapes, after a call that set one of 16", white_shape_cache_size(v.rt));
    CHECK(eval_ok(v.ctx, "mk(); mk(); void 0") && white_kept_shapes(v.rt) > 0, "the cache keeps no shape");
    /* the runtime frees with no block left: the ring of the finalizer went */
    vm_free(&v);
}

/* JS_SetClassProto replaces the Object.prototype of the context of the
   cache: the cache lets go of the shapes of the old one, then of the old one,
   and keeps the shapes of the new one from then on */
static void test_shape_cache_proto_replaced(void)
{
    JSClassDef def = { "Replaced", reenter_finalizer };
    JSClassID class_id = 0;
    JSValue o, p;
    VM v;
    int kept;

    v = vm_new();
    reenter_finalized = 0;
    reenter_ctx = NULL;
    JS_NewClassID(v.rt, &class_id);
    CHECK(JS_NewClass(v.rt, class_id, &def) == 0, "JS_NewClass");
    o = JS_GetClassProto(v.ctx, white_class_object());
    p = JS_NewObjectClass(v.ctx, class_id);
    CHECK(JS_IsObject(p), "the prototype");
    JS_SetClassProto(v.ctx, white_class_object(), JS_DupValue(v.ctx, p));
    JS_SetShapeCache(v.ctx, 64);
    CHECK(eval_ok(v.ctx, "function mk() { var o = {}; o.qa = 1; o.qb = 2; return 0 }\nmk(); mk(); void 0"), "the objects");
    kept = white_kept_shapes(v.rt);
    CHECK(kept > 0, "no shape of the prototype of the host kept");
    JS_SetClassProto(v.ctx, white_class_object(), o);
    JS_FreeValue(v.ctx, p);
    CHECK(white_kept_shapes(v.rt) == 0, "%d shapes kept after the prototype was replaced, %d before", white_kept_shapes(v.rt), kept);
    CHECK(reenter_finalized == 1, "the replaced prototype was finalized %d times once the host let it go", reenter_finalized);
    CHECK(eval_is(v.ctx, "mk(); mk(); String(Object.getPrototypeOf((function () { var o = {}; o.qa = 1; return o })()) === Object.prototype)", "true"),
          "a plain object after the prototype was replaced");
    CHECK(white_kept_shapes(v.rt) > 0, "the cache keeps no shape of the new prototype");
    vm_free(&v);
    CHECK(reenter_finalized == 1, "the replaced prototype was finalized %d times in all", reenter_finalized);
}

/* A second context that shares the Object.prototype of the context of the
   cache, then replaces its own, leaves the cache as it is: only the context
   given to JS_SetShapeCache moves it */
static void test_shape_cache_two_contexts(void)
{
    JSClassDef def = { "Other", reenter_finalizer };
    JSClassID class_id = 0;
    JSContext *b;
    JSValue oa, ob, q;
    VM v;
    void *p;
    int kept;

    v = vm_new();
    reenter_finalized = 0;
    reenter_ctx = NULL;
    JS_NewClassID(v.rt, &class_id);
    CHECK(JS_NewClass(v.rt, class_id, &def) == 0, "JS_NewClass");
    JS_SetShapeCache(v.ctx, 64);
    CHECK(eval_ok(v.ctx, "function mk() { var o = {}; o.qa = 1; o.qb = 2; return 0 }\nmk(); mk(); void 0"), "the objects");
    kept = white_kept_shapes(v.rt);
    CHECK(kept > 0, "no shape kept");
    b = JS_NewContext(v.rt);
    CHECK(b != NULL, "the second context");
    oa = JS_GetClassProto(v.ctx, white_class_object());
    ob = JS_GetClassProto(b, white_class_object());
    JS_SetClassProto(b, white_class_object(), oa);
    q = JS_NewObjectClass(b, class_id);
    CHECK(JS_IsObject(q), "the prototype of the second context");
    JS_SetClassProto(b, white_class_object(), JS_DupValue(b, q));
    CHECK(white_kept_shapes(v.rt) == kept, "%d shapes kept after the second context replaced its prototype, %d before",
          white_kept_shapes(v.rt), kept);
    JS_SetClassProto(b, white_class_object(), ob);
    JS_FreeValue(b, q);
    CHECK(reenter_finalized == 1, "the prototype of the second context was finalized %d times once it let it go", reenter_finalized);
    JS_FreeContext(b);
    JS_RunGC(v.rt);
    white_drain_once();
    p = js_malloc_rt(v.rt, 1 << 20);
    CHECK(p != NULL, "a block of 1 MiB");
    js_free_rt(v.rt, p);
    CHECK(white_kept_shapes(v.rt) == 0, "%d shapes kept after the drain", white_kept_shapes(v.rt));
    CHECK(eval_ok(v.ctx, "mk(); mk(); void 0") && white_kept_shapes(v.rt) > 0, "the cache keeps no shape of its context");
    vm_free(&v);
    CHECK(reenter_finalized == 1, "the prototype of the second context was finalized %d times in all", reenter_finalized);
}

/* each_vm_failing calls the global f of setup with each call of the
   allocator failing in turn, as each_call_failing does, but each call in a VM
   of its own, made and set up again, so that what only a first call
   allocates (the hash table of shapes growing) fails in turn too. The shape
   cache is off when nocache is set. Each call either throws the out of
   memory error or returns want, and leaves as many objects, strings and
   shapes as there were before it, after a collection. The VM is freed after
   each call, which fails the test (status 87) when a block is left. At least
   min_ooms of the calls fail. */
static void each_vm_failing(const char *what, int nocache, const char *setup, const char *want, int min_ooms)
{
    int fail, n = 0, ooms = 0;

    for (fail = 0; fail == 0 || fail <= n; fail++) {
        VM v = vm_new();
        JSMemoryUsage before, after;
        JSValue g, f, r;
        int made;

        if (nocache)
            JS_SetShapeCache(v.ctx, 0);
        CHECK(eval_ok(v.ctx, setup), "%s: the setup threw", what);
        g = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, g, "f");
        JS_FreeValue(v.ctx, g);
        JS_RunGC(v.rt);
        JS_ComputeMemoryUsage(v.rt, &before);
        r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);
        if (fail == 0)
            n = made;
        if (JS_IsException(r)) {
            CHECK(fail > 0 && JS_IsUncatchableException(v.ctx), "%s, call %d failing: an exception that is not out of memory", what, fail);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            ooms++;
        } else {
            const char *s = JS_ToCString(v.ctx, r);

            CHECK(s && !strcmp(s, want), "%s, call %d failing: %s", what, fail, s ? s : "(no string)");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, r);
        }
        CHECK(!JS_HasException(v.ctx), "%s, call %d failing: an exception is pending", what, fail);
        JS_RunGC(v.rt);
        JS_ComputeMemoryUsage(v.rt, &after);
        CHECK(after.obj_count == before.obj_count && after.str_count == before.str_count &&
              after.shape_count == before.shape_count,
              "%s, call %d failing: %lld objects, %lld strings and %lld shapes, before it %lld, %lld and %lld", what, fail,
              (long long)after.obj_count, (long long)after.str_count, (long long)after.shape_count,
              (long long)before.obj_count, (long long)before.str_count, (long long)before.shape_count);
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
    CHECK(ooms >= min_ooms, "%s: only %d of %d calls failed", what, ooms, n);
}

/* The stores of rv_putel_atom (o[k] = v, k a key that is its own atom) and of
   rv_putf_atom (o.x = v) with each allocation failing in turn: the property
   array of the object growing, the shape cloned when another object shares
   it or grown in place, the hash table of shapes growing (300 new shapes in
   a new VM), with the shape cache on and off. The value is a new object, so
   that one freed twice or not at all shows (in the counts, in the leak check
   of the runtime, and under AddressSanitizer). The object, the key and the
   value are in arguments, in locals and in temps. The functions run once on
   other keys first, so that they are register code. */
static void test_putel_atom_each_failing(void)
{
    static const char setup[] =
        "var K = ['w'], U = ['w'];\n"
        "function mk() { return {} }\n"
        "function args(o, k, v) { o[k] = v }\n"
        "function locals() { var o = {}, n = K.length; for (var i = 0; i < n; i++) { var k = K[i], v = {}; o[k] = v } return o }\n"
        "function temps() { var o = mk(); for (var i = 0; i < K.length; i++) mk()[K[i]] = {}; return o }\n"
        "function shared() { var a = { s0: 1 }, b = { s0: 1 }; b[K[0]] = {}; return a.s0 + b.s0 }\n"
        "function many() { var keep = []; for (var i = 0; i < U.length; i++) { var o = {}; o[U[i]] = {}; keep.push(o) } return keep.length }\n"
        "function fargs(o) { o.x0 = {}; o.x1 = {}; o.x2 = {}; o.x3 = {}; o.x4 = {}; o.x5 = {}; o.x6 = {}; o.x7 = {}; o.x8 = {}; o.x9 = {}; return o }\n"
        "function flocals() { var o = {}, v = {}; o.y0 = v; o.y1 = {}; o.y2 = {}; o.y3 = {}; o.y4 = {}; o.y5 = {}; return o }\n"
        "function ftemps() { mk().z0 = {}; mk().z1 = {}; return mk() }\n"
        "function fshared() { var a = { s0: 1 }, b = { s0: 1 }; b.zs = {}; return a.s0 + b.s0 }\n"
        "function fmany() { var keep = []; for (var i = 0; i < 300; i++) { var o = {}; o.zm = {}; keep.push(o) } return keep.length }\n"
        "args(mk(), 'w', 1); locals(); temps(); shared(); many(); fargs({}); flocals(); ftemps(); fshared(); fmany();\n"
        "var src = {}; for (var i = 0; i < 300; i++) src['u' + i] = 0; U = Object.keys(src);\n"
        "K = Object.keys({ ka: 0, kb: 0, kc: 0, kd: 0, ke: 0, kf: 0, kg: 0, kh: 0, ki: 0, kj: 0 }); src = null;\n";
    static const struct { const char *what, *f, *want; } cases[] = {
        { "o[k] = v in arguments", "function f() { var o = {}; for (var i = 0; i < K.length; i++) args(o, K[i], {}); return Object.keys(o).length }", "10" },
        { "o[k] = v in locals", "function f() { return Object.keys(locals()).length }", "10" },
        { "o[k] = v in temps", "function f() { return Object.keys(temps()).length }", "0" },
        { "o[k] = v on a shared shape", "function f() { return shared() }", "2" },
        { "o[k] = v making 300 shapes", "function f() { return many() }", "300" },
        { "o.x = v in arguments", "function f() { return Object.keys(fargs({})).length }", "10" },
        { "o.x = v in locals", "function f() { return Object.keys(flocals()).length }", "6" },
        { "o.x = v in temps", "function f() { return Object.keys(ftemps()).length }", "0" },
        { "o.x = v on a shared shape", "function f() { return fshared() }", "2" },
        { "o.x = v on 300 objects", "function f() { return fmany() }", "300" },
    };
    char buf[4096];
    int i, nocache;

    for (i = 0; i < (int)(sizeof(cases) / sizeof(cases[0])); i++) {
        for (nocache = 0; nocache <= 1; nocache++) {
            snprintf(buf, sizeof(buf), "%s%s\n", setup, cases[i].f);
            each_vm_failing(cases[i].what, nocache, buf, cases[i].want, 3);
        }
    }
}

/* ---- the lists of the collector ---- */

static void lists(VM *v, int *old, int *young, const char *when)
{
    int bad;

    white_gc_lists(v->rt, old, young, &bad);
    CHECK(bad == 0, "%s: a GC object on a list has a count of 0 or less (%d), its mark set (%d) or the young bit of the other list (%d)", when, bad & 1, (bad >> 1) & 1, (bad >> 2) & 1);
}

/* The ids of the shapes in the shape hash table follow their rule
   (white_shape_ids_bad) after each step, with the shape cache on and off:
   objects that get the same keys one at a time, the changes in place that
   take a shape out of the table, a global function declared again, which
   changes the flags of the shape of the global object where it is, a shape
   that is never in the table, and the ids running out, which numbers the
   shapes again from 1. white.c builds the engine with JS_SHAPE_ID_CHECK, so
   a shape found by its parent id that starts with other keys than the shape
   it grew from ends the test. Shapes are found by their parent id
   (white_shape_id_hits) where an object held the shape before, and again
   once the ids were given anew: by the parent id a comparison of the keys
   gave them, and for a new stair. */
static int global_shape_hashed(VM *v)
{
    JSValue g = JS_GetGlobalObject(v->ctx);
    int hashed = white_shape_hashed(g);

    JS_FreeValue(v->ctx, g);
    return hashed;
}

static void shape_ids_ok(VM *v, const char *what, int cache)
{
    int count, parented, bad = white_shape_ids_bad(v->rt, &count, &parented);

    CHECK(bad == 0, "cache %d, %s: %d of %d shapes break the rule of ids", cache, what, bad, count);
    CHECK(count > 0, "cache %d, %s: no shape in the table", cache, what);
}

/* A string that is an atom of the type string, not empty and not starting
   with a digit, keeps its atom in key_atom, which the keyed reads and
   hasOwnProperty.call read in place of the test and the hash chain. Every other
   string has JS_ATOM_NULL there: a string that is not an atom, the empty
   string, a string that starts with a digit, a symbol. The rule holds for
   every atom of the runtime while atoms are made and freed and their
   numbers are given to other strings, and while the hash table of atoms
   grows. */
static void key_atoms_ok(VM *v, const char *what)
{
    int live, keys, bad = white_key_atoms_bad(v->rt, &live, &keys);

    CHECK(bad == 0, "%s: %d of %d atoms have the wrong key_atom", what, bad, live);
    CHECK(keys > 0, "%s: no atom is its own key", what);
}

static uint32_t key_atom_of_atom(VM *v, JSAtom a)
{
    JSValue s = JS_AtomToString(v->ctx, a);
    uint32_t k = white_key_atom(s);

    JS_FreeValue(v->ctx, s);
    return k;
}

/* A call of a C function that js_call_c_counted makes (the answer of push
   in the register loop, when the push must grow its array, after the
   checks it made itself) counts what a direct call counts while the
   function runs: one level of call depth and the debt rt->c_direct_debt
   (JS_RegvmStat 17), the frame of JS_CallInternal it skips less what
   js_call_c_plain takes beyond js_call_c_function. Both are given back
   after the call. A counted call that kept the debt of before the plain
   calls (rt->call_frame) would count JS_RegvmStat 6 where 17 differs. */
static void test_counted_call_debt(void)
{
    VM v = vm_new();
    long want = JS_RegvmStat(v.ctx, 17), counted, direct;
    int depth = 0, direct_depth = 0;

    CHECK(want >= 0, "the direct calls are off: their debt is %ld", want);
    counted = white_call_debt(v.ctx, 0, &depth);
    direct = white_call_debt(v.ctx, 1, &direct_depth);
    CHECK(counted == want, "js_call_c_counted counts %ld bytes as taken, a direct call %ld",
          counted, want);
    CHECK(direct == want, "js_call_c_direct counts %ld bytes as taken, JS_RegvmStat 17 %ld",
          direct, want);
    CHECK(depth == 1 && direct_depth == 1,
          "a counted call takes %d levels of call depth, a direct call %d", depth, direct_depth);
    vm_free(&v);
}

static void test_key_atom(void)
{
    static const struct { const char *s; int key; } kinds[] = {
        { "wkaName", 1 }, { "-1", 1 }, { "-0", 1 }, { "Infinity", 1 }, { "NaN", 1 }, { "_0", 1 },
        { "0", 0 }, { "10", 0 }, { "01", 0 }, { "1e3", 0 }, { "4294967295", 0 }, { "9z", 0 }, { "", 0 },
    };
    VM v = vm_new();
    JSValue s;
    JSAtom a, b, c;
    size_t i;

    key_atoms_ok(&v, "a new context");
    for (i = 0; i < sizeof(kinds) / sizeof(kinds[0]); i++) {
        a = JS_NewAtom(v.ctx, kinds[i].s);
        CHECK(a != JS_ATOM_NULL, "the atom of '%s'", kinds[i].s);
        /* the atoms of "0" to "2147483647" are numbers (JS_ATOM_TAG_INT), with
           no string */
        if (!(a & 0x80000000u))
            CHECK(key_atom_of_atom(&v, a) == (kinds[i].key ? a : JS_ATOM_NULL),
                  "'%s': key_atom %u, atom %u", kinds[i].s, key_atom_of_atom(&v, a), a);
        JS_FreeAtom(v.ctx, a);
    }

    /* a string that is not an atom has none. Made an atom when no atom has
       its characters, it becomes the atom (__JS_NewAtom) and holds the
       number */
    s = JS_NewString(v.ctx, "wkaName");
    CHECK(white_key_atom(s) == JS_ATOM_NULL, "a new string has key_atom %u", white_key_atom(s));
    a = JS_ValueToAtom(v.ctx, s);
    CHECK(a != JS_ATOM_NULL && white_key_atom(s) == a, "the string made an atom: key_atom %u, atom %u",
          white_key_atom(s), a);
    JS_FreeAtom(v.ctx, a);
    JS_FreeValue(v.ctx, s);

    /* a number given back is given to the next atom: the string of the new
       atom has the number, and a new string of the old characters is no
       atom until it is made one */
    a = JS_NewAtom(v.ctx, "wkaFirstOfItsName");
    JS_FreeAtom(v.ctx, a);
    b = JS_NewAtom(v.ctx, "wkaSecondOfItsName");
    CHECK(b == a, "the number %u of the freed atom was not given again (%u)", a, b);
    CHECK(key_atom_of_atom(&v, b) == b, "the new atom: key_atom %u", key_atom_of_atom(&v, b));
    s = JS_NewString(v.ctx, "wkaFirstOfItsName");
    CHECK(white_key_atom(s) == JS_ATOM_NULL, "the old characters: key_atom %u", white_key_atom(s));
    c = JS_ValueToAtom(v.ctx, s);
    CHECK(c != a && white_key_atom(s) == c && key_atom_of_atom(&v, c) == c,
          "the old characters as an atom: %u, key_atom %u", c, white_key_atom(s));
    JS_FreeAtom(v.ctx, c);
    JS_FreeAtom(v.ctx, b);
    JS_FreeValue(v.ctx, s);
    key_atoms_ok(&v, "atoms freed and made again");

    /* a string that is no atom, whose characters are an atom that exists,
       keeps that atom as its key, with a reference of its own, from the
       first time it is made a key, and gives it back when it is freed. A
       string whose atom is no key (an index, a string that starts with a
       digit) keeps none */
    a = JS_NewAtom(v.ctx, "wkaKept");
    {
        int refs = white_atom_refs(v.ctx, "wkaKept");

        s = JS_NewString(v.ctx, "wkaKept");
        b = JS_ValueToAtom(v.ctx, s);
        CHECK(b == a && white_key_atom(s) == a, "the kept atom: %u, key_atom %u, atom %u", b,
              white_key_atom(s), a);
        CHECK(white_atom_refs(v.ctx, "wkaKept") == refs + 2, "the atom kept: %d references, want %d",
              white_atom_refs(v.ctx, "wkaKept"), refs + 2);
        c = JS_ValueToAtom(v.ctx, s);
        CHECK(c == a && white_atom_refs(v.ctx, "wkaKept") == refs + 3, "made a key again: %u, %d references",
              c, white_atom_refs(v.ctx, "wkaKept"));
        JS_FreeAtom(v.ctx, c);
        JS_FreeAtom(v.ctx, b);
        JS_FreeValue(v.ctx, s);
        CHECK(white_atom_refs(v.ctx, "wkaKept") == refs, "the string freed: %d references, want %d",
              white_atom_refs(v.ctx, "wkaKept"), refs);
    }
    JS_FreeAtom(v.ctx, a);
    /* made a registered symbol, which takes the string itself as its
       atom, the string gives back the atom it kept */
    a = JS_NewAtom(v.ctx, "wkaSym");
    {
        int refs = white_atom_refs(v.ctx, "wkaSym");
        JSValue g = JS_GetGlobalObject(v.ctx), sy = JS_GetPropertyStr(v.ctx, g, "Symbol");
        JSValue sfor = JS_GetPropertyStr(v.ctx, sy, "for"), r;

        s = JS_NewString(v.ctx, "wkaSym");
        JS_FreeAtom(v.ctx, JS_ValueToAtom(v.ctx, s));
        CHECK(white_key_atom(s) == a && white_atom_refs(v.ctx, "wkaSym") == refs + 1,
              "the string keeps the atom: key_atom %u, %d references", white_key_atom(s),
              white_atom_refs(v.ctx, "wkaSym"));
        r = JS_Call(v.ctx, sfor, sy, 1, &s);
        CHECK(JS_IsSymbol(r) && white_key_atom(s) == JS_ATOM_NULL, "Symbol.for: key_atom %u",
              white_key_atom(s));
        CHECK(white_atom_refs(v.ctx, "wkaSym") == refs, "the symbol made: %d references, want %d",
              white_atom_refs(v.ctx, "wkaSym"), refs);
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, s);
        JS_FreeValue(v.ctx, sfor);
        JS_FreeValue(v.ctx, sy);
        JS_FreeValue(v.ctx, g);
    }
    JS_FreeAtom(v.ctx, a);
    a = JS_NewAtom(v.ctx, "07");
    s = JS_NewString(v.ctx, "07");
    b = JS_ValueToAtom(v.ctx, s);
    CHECK(b == a && white_key_atom(s) == JS_ATOM_NULL, "\"07\": atom %u, key_atom %u", b, white_key_atom(s));
    JS_FreeAtom(v.ctx, b);
    JS_FreeAtom(v.ctx, a);
    JS_FreeValue(v.ctx, s);
    /* in a script: the string a join makes reads its key from the second
       read on without a search. Appended to, it is copied, though it has
       room at its end (the second append made it with some): in place it
       would keep the atom of its old characters. Made a registered symbol,
       it is read by its key as before (the string is the atom of the symbol now) */
    CHECK(eval_ok(v.ctx,
        "var T = { wkaMonth: 1, wkaMonthx: 2 }, k = ['wka', 'Month'].join(''), r = 0;\n"
        "for (let i = 0; i < 100; i++) r += T[k];\n"
        "var m = (function () { let m = 'wk'; m += 'a'; m += 'Month'; r += T[m];\n"
        "  m += 'x'; r += T[m]; return m })();\n"
        "var g = ['wka', 'Month'].join(''); r += T[g]; var sym = Symbol.for(g); r += T[g];\n"
        "r += T[k] + T[m] + T[g]\n"), "the script threw");
    CHECK(eval_is(v.ctx, "r + ' ' + m + ' ' + String(sym.description)", "109 wkaMonthx wkaMonth"),
          "reads by kept atoms");
    {
        JSValue k = JS_Eval(v.ctx, "k", 1, "<k>", 0);
        JSAtom month = JS_NewAtom(v.ctx, "wkaMonth");

        CHECK(white_key_atom(k) == month, "the joined key: key_atom %u, atom %u", white_key_atom(k), month);
        JS_FreeAtom(v.ctx, month);
        JS_FreeValue(v.ctx, k);
    }
    key_atoms_ok(&v, "strings that keep atoms");

    /* thousands of atoms made, read by key, freed and made again, through
       the growth of the hash table of atoms and the reuse of numbers */
    CHECK(eval_ok(v.ctx,
        "var o = {}, bad = 0;\n"
        "for (let r = 0; r < 3; r++) {\n"
        "  let ks = [];\n"
        "  for (let i = 0; i < 5000; i++) ks.push(Object.keys({ ['wka' + r + '_' + i]: i })[0]);\n"
        "  for (let i = 0; i < 5000; i++) { o[ks[i]] = i; if (o[ks[i]] !== i || !(ks[i] in o)) bad++ }\n"
        "  for (let i = 0; i < 5000; i++) delete o[ks[i]];\n"
        "  ks = null }\n"), "the script threw");
    CHECK(eval_is(v.ctx, "bad", "0"), "keyed reads of the made atoms");
    key_atoms_ok(&v, "thousands of atoms");
    JS_RunGC(v.rt);
    key_atoms_ok(&v, "after a collection");
    vm_free(&v);
}

/* white_string_plain of the value of the expression src, -1 when it is no
   flat string */
static int plain_of(JSContext *ctx, const char *src)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), "<plain>", 0);
    int r = JS_VALUE_GET_TAG(v) == JS_TAG_STRING ? white_string_plain(ctx, v) : -1;

    JS_FreeValue(ctx, v);
    return r;
}

/* The mark "needs no escape" (JSString.plain): JS_ToQuotedString sets it on
   a string it wrote with no escape, an atom or not, and copies a marked
   string with no look at its characters. A string with a quote, a control
   character or a lone surrogate never gets it, a marked string is not
   appended to in place, and the number of a freed atom comes back with no
   mark. */
static void test_json_plain(void)
{
    VM v = vm_new();
    JSValue o, j;
    JSAtom a, b;

    /* strings that are no atoms: marked by a write with no escape */
    CHECK(eval_ok(v.ctx,
        "var a = ['wjp', 'Plain'].join(''), q = ['wjp', '\"q'].join(''),\n"
        "  n = ['wjp', '\\n'].join(''), u = ['wjp', '\\ud800'].join(''),\n"
        "  pr = ['wjp', '\\ud83d\\ude00'].join(''), w = ['wjp', '\\u4e2d'].join('');\n"), "the script threw");
    CHECK(plain_of(v.ctx, "a") == 0, "a new string is marked");
    CHECK(eval_is(v.ctx, "JSON.stringify([a, q, n, u, pr, w])",
                  "[\"wjpPlain\",\"wjp\\\"q\",\"wjp\\n\",\"wjp\\ud800\",\"wjp\xf0\x9f\x98\x80\",\"wjp\xe4\xb8\xad\"]"),
          "the first write");
    CHECK(plain_of(v.ctx, "a") == 1 && plain_of(v.ctx, "w") == 1 && plain_of(v.ctx, "pr") == 1,
          "a string written with no escape: %d %d %d", plain_of(v.ctx, "a"), plain_of(v.ctx, "w"),
          plain_of(v.ctx, "pr"));
    CHECK(plain_of(v.ctx, "q") == 0 && plain_of(v.ctx, "n") == 0 && plain_of(v.ctx, "u") == 0,
          "a string with an escape or a lone surrogate is marked");
    CHECK(eval_is(v.ctx, "JSON.stringify([a, q, n, u, pr, w])",
                  "[\"wjpPlain\",\"wjp\\\"q\",\"wjp\\n\",\"wjp\\ud800\",\"wjp\xf0\x9f\x98\x80\",\"wjp\xe4\xb8\xad\"]"),
          "the second write");

    /* a marked string with room at its end is copied to append to: appended
       to in place, it would keep the mark with a quote in it */
    CHECK(eval_is(v.ctx,
        "(function () { let m = 'wjp'; for (let i = 0; i < 20; i++) m += 'Mark';\n"
        "  const j1 = JSON.stringify(m); m += '\"'; const j2 = JSON.stringify(m);\n"
        "  return [j1.length === m.length + 1, j2.length === m.length + 3, j2.endsWith('\\\\\"\"'),\n"
        "    JSON.stringify({ k: m }).endsWith('\\\\\"\"}')].join() })()",
        "true,true,true,true"), "a marked string appended to");

    /* strings of JSON.parse, written twice */
    CHECK(eval_ok(v.ctx, "var pp = JSON.parse('[\"wjpParsedLong\", \"wjp\\\\nx\", \"wjp\\\\u0041\"]')"),
          "the script threw");
    CHECK(eval_is(v.ctx, "JSON.stringify(pp)", "[\"wjpParsedLong\",\"wjp\\nx\",\"wjpA\"]"), "the parsed strings");
    CHECK(plain_of(v.ctx, "pp[0]") == 1 && plain_of(v.ctx, "pp[1]") == 0 && plain_of(v.ctx, "pp[2]") == 1,
          "parsed strings written: %d %d %d", plain_of(v.ctx, "pp[0]"), plain_of(v.ctx, "pp[1]"),
          plain_of(v.ctx, "pp[2]"));
    CHECK(eval_is(v.ctx, "JSON.stringify(pp)", "[\"wjpParsedLong\",\"wjp\\nx\",\"wjpA\"]"), "the parsed strings again");

    /* a string that keeps the atom of its key (the hash 1) and has the
       mark: the mark is no part of the test of js_string_atom_or_key, so
       the string is not appended to in place and gives its key back when
       it is freed (vm_free finds an atom left) */
    CHECK(eval_ok(v.ctx,
        "var ko = { wjpKey: 1 }, kk = ['wjp', 'Key'].join(''); ko[kk]; JSON.stringify([kk]);"),
          "the script threw");
    {
        JSValue s = JS_Eval(v.ctx, "kk", 2, "<plain>", 0);

        CHECK(white_key_atom(s) != JS_ATOM_NULL && white_string_plain(v.ctx, s) == 1,
              "the key %u and the mark %d of kk", white_key_atom(s), white_string_plain(v.ctx, s));
        JS_FreeValue(v.ctx, s);
    }
    CHECK(eval_is(v.ctx, "kk += 'More'; [kk, ko[kk.slice(0, 6)], JSON.stringify(kk)].join()",
                  "wjpKeyMore,1,\"wjpKeyMore\""), "kk appended to");
    CHECK(eval_ok(v.ctx, "kk = undefined;"), "the script threw");

    /* a long string, over several steps of the interrupt check */
    CHECK(eval_is(v.ctx,
        "var big = 'wjp'.repeat(40000) + 'x', s1 = JSON.stringify(big), s2 = JSON.stringify(big);\n"
        "s1 === s2 && s1.length === big.length + 2", "true"), "a long marked string");

    /* atoms: a key is marked by its first write */
    a = JS_NewAtom(v.ctx, "wjpFirst");
    o = JS_NewObject(v.ctx);
    JS_SetProperty(v.ctx, o, a, JS_NewInt32(v.ctx, 1));
    {
        JSValue s = JS_AtomToString(v.ctx, a);

        CHECK(white_string_plain(v.ctx, s) == 0, "a new atom is marked");
        j = JS_JSONStringify(v.ctx, o, JS_UNDEFINED, JS_UNDEFINED);
        CHECK(white_string_plain(v.ctx, s) == 1, "a key written: not marked");
        JS_FreeValue(v.ctx, j);
        JS_FreeValue(v.ctx, s);
    }
    JS_FreeValue(v.ctx, o);
    JS_FreeAtom(v.ctx, a);
    /* its number, given to the next atom, comes with no mark */
    b = JS_NewAtom(v.ctx, "wjp\"Second");
    CHECK(b == a, "the number %u of the freed atom was not given again (%u)", a, b);
    o = JS_NewObject(v.ctx);
    JS_SetProperty(v.ctx, o, b, JS_NewInt32(v.ctx, 2));
    j = JS_JSONStringify(v.ctx, o, JS_UNDEFINED, JS_UNDEFINED);
    {
        const char *t = JS_ToCString(v.ctx, j);

        CHECK(t && strcmp(t, "{\"wjp\\\"Second\":2}") == 0, "the key of the new atom: %s", t ? t : "(null)");
        JS_FreeCString(v.ctx, t);
    }
    JS_FreeValue(v.ctx, j);
    JS_FreeValue(v.ctx, o);
    JS_FreeAtom(v.ctx, b);
    vm_free(&v);
}

static void test_shape_ids(void)
{
    static const char build[] =
        "function build(n, pre) { const o = {}; for (let i = 0; i < n; i++) o[pre + i] = i; return o }\n"
        "function stair(n, pre) { const a = []; for (let i = 1; i <= n; i++) a.push(build(i, pre)); return a }\n"
        "function keys(o) { return Reflect.ownKeys(o).map(String).join() }\n"
        "void 0";
    int cache;

    for (cache = 0; cache <= 1; cache++) {
        VM v = vm_new();
        unsigned long hits;
        uint32_t start;
        int count, parented;

        JS_SetShapeCache(v.ctx, cache ? 2048 : 0);
        CHECK(eval_ok(v.ctx, build), "the script threw");
        shape_ids_ok(&v, "a new context", cache);

        /* every shape of 1 to 10 keys is held: the next object finds each */
        CHECK(eval_ok(v.ctx, "var ks = stair(10, 'k'); void 0"), "the stair threw");
        hits = white_shape_id_hits();
        CHECK(eval_ok(v.ctx, "var k10 = build(10, 'k'); void 0"), "the script threw");
        CHECK(white_shape_id_hits() >= hits + 9, "cache %d: %lu shapes found by their parent id, want 9",
              cache, white_shape_id_hits() - hits);
        CHECK(shape_of(&v, "k10") == shape_of(&v, "ks[9]"), "cache %d: k10 has a shape of its own", cache);
        shape_ids_ok(&v, "a stair", cache);
        white_shape_ids_bad(v.rt, &count, &parented);
        CHECK(parented >= 9, "cache %d: %d shapes with a parent id", cache, parented);

        /* changes in place, then the same keys again */
        CHECK(eval_ok(v.ctx,
                      "var d = build(4, 'k'); delete d.k1; d.k1 = 1; d.k4 = 4;\n"
                      "var f = build(4, 'k'); Object.defineProperty(f, 'k2', { enumerable: false }); f.k4 = 4;\n"
                      "var g = build(4, 'k'); Object.defineProperty(g, 'k0', { get() { return 9 }, configurable: true }); g.k4 = 4;\n"
                      "var p = build(4, 'k'); Object.setPrototypeOf(p, null); p.k4 = 4;\n"
                      "var z = build(4, 'k'); Object.freeze(z);\n"
                      "var arr = [1, 2, 3]; arr.x = 1; arr[9] = 9; delete arr[0]; arr.y = 2;\n"
                      "var k5 = build(5, 'k'); void 0"), "the changes threw");
        CHECK(eval_is(v.ctx, "[d, f, g, p, arr].map(keys).join(' ')",
                      "k0,k2,k3,k1,k4 k0,k1,k2,k3,k4 k0,k1,k2,k3,k4 k0,k1,k2,k3,k4 1,2,9,length,x,y"),
              "cache %d: keys after the changes", cache);
        CHECK(eval_is(v.ctx, "Object.keys(f) + ' ' + g.k0 + ' ' + Object.getPrototypeOf(p) + ' ' +"
                             " Object.isFrozen(z) + ' ' + Object.isFrozen(ks[3])",
                      "k0,k1,k3,k4 9 null true false"), "cache %d: flags after the changes", cache);
        CHECK(shape_of(&v, "k5") == shape_of(&v, "ks[4]"), "cache %d: k5 after the changes", cache);
        shape_ids_ok(&v, "changes in place", cache);

        /* a global function declared again changes the flags of the shape
           of the global object in place, a shape that is never in the
           table */
        CHECK(!global_shape_hashed(&v), "cache %d: the shape of the global object is in the table", cache);
        CHECK(eval_is(v.ctx,
                      "Object.defineProperty(globalThis, 'gf', { get() { return 1 }, configurable: true });\n"
                      "(0, eval)('function gf() { return 2 }'); (0, eval)('function gh() { return 3 }');\n"
                      "(0, eval)('function gh() { return 4 }'); globalThis.gz = 5;\n"
                      "gf() + ' ' + gh() + ' ' + JSON.stringify(Object.getOwnPropertyDescriptor(globalThis, 'gh'))"
                      ".replace(/\"value\":[^,]*,/, '')",
                      "2 4 {\"writable\":true,\"enumerable\":true,\"configurable\":true}"),
              "cache %d: global functions", cache);
        CHECK(!global_shape_hashed(&v), "cache %d: the shape of the global object went in the table", cache);
        shape_ids_ok(&v, "global functions", cache);

        /* the ids run out in the middle of a stair: the shapes are numbered
           again, and the objects keep their keys and their shapes */
        start = white_shape_id_max() - 8;
        white_shape_id_set(v.rt, start);
        CHECK(eval_ok(v.ctx, "var ws = stair(12, 'w'); void 0"), "the stair threw");
        CHECK(white_shape_id_next(v.rt) < start, "cache %d: the ids did not run out (next %u)",
              cache, white_shape_id_next(v.rt));
        shape_ids_ok(&v, "ids given anew", cache);
        white_shape_ids_bad(v.rt, &count, &parented);
        CHECK(white_shape_id_next(v.rt) <= (uint32_t)count + 64, "cache %d: next id %u for %d shapes",
              cache, white_shape_id_next(v.rt), count);
        CHECK(eval_is(v.ctx, "keys(ws[11]) + ' ' + keys(build(3, 'k')) + ' ' +"
                             " ws.every((o, i) => Object.keys(o).length === i + 1)",
                      "w0,w1,w2,w3,w4,w5,w6,w7,w8,w9,w10,w11 k0,k1,k2 true"),
              "cache %d: keys after the ids ran out", cache);
        CHECK(eval_ok(v.ctx, "var w12 = build(12, 'w'), k10b = build(10, 'k'); void 0"), "the script threw");
        CHECK(shape_of(&v, "w12") == shape_of(&v, "ws[11]"), "cache %d: w12 has a shape of its own", cache);
        CHECK(shape_of(&v, "k10b") == shape_of(&v, "ks[9]"), "cache %d: k10b has a shape of its own", cache);

        /* the shapes numbered again have no parent id, so k10b found each
           by comparing its keys, which gave it the id of the shape before
           as its parent id: the next object finds them by it */
        hits = white_shape_id_hits();
        CHECK(eval_ok(v.ctx, "var k10c = build(10, 'k'); void 0"), "the script threw");
        CHECK(white_shape_id_hits() >= hits + 9,
              "cache %d: %lu shapes found by the parent id a comparison of their keys gave, want 9",
              cache, white_shape_id_hits() - hits);
        CHECK(shape_of(&v, "k10c") == shape_of(&v, "ks[9]"), "cache %d: k10c has a shape of its own", cache);
        shape_ids_ok(&v, "parent ids from a comparison", cache);

        /* a new stair after the ids were given anew is found by parent id */
        CHECK(eval_ok(v.ctx, "var vs = stair(8, 'v'); void 0"), "the stair threw");
        hits = white_shape_id_hits();
        CHECK(eval_ok(v.ctx, "var v8 = build(8, 'v'); void 0"), "the script threw");
        CHECK(white_shape_id_hits() >= hits + 7,
              "cache %d: %lu shapes found by their parent id after the ids ran out, want 7",
              cache, white_shape_id_hits() - hits);
        CHECK(shape_of(&v, "v8") == shape_of(&v, "vs[7]"), "cache %d: v8 has a shape of its own", cache);
        shape_ids_ok(&v, "after the ids ran out", cache);
        vm_free(&v);
    }
}

/* A shape held by two objects is copied into the shape hash table when one
   of them gets a key, and the copy keeps the id of the shape it copies until
   the key is in. When adding the key fails for lack of memory, the copy
   stays in the table beside the shape it copies, with its id: two shapes of
   one id with the same prototype, keys and flags, which the rule of the ids
   allows. Each call of the allocator fails in turn, with the shape cache on
   and off, and the objects keep their keys. */
static void test_shape_ids_copy_oom(void)
{
    static const char setup[] =
        "var s1 = {}, s2 = {};\n"
        "for (const k of ['a', 'b', 'c', 'd']) { s1[k] = 1; s2[k] = 2 }\n"
        "function addz(o) { o.zz = 3; return o }\n"
        "void 0";
    long cases = 0, twins = 0;
    int cache;

    for (cache = 0; cache <= 1; cache++) {
        int n = 0, fail;

        for (fail = 0; fail <= 1 || fail <= n; fail++) {
            VM v = vm_new();
            JSValue global, addz, s1, r;
            int made;

            JS_SetShapeCache(v.ctx, cache ? 2048 : 0);
            CHECK(eval_ok(v.ctx, setup), "the script threw");
            CHECK(shape_of(&v, "s1") == shape_of(&v, "s2"), "cache %d: s1 and s2 share no shape", cache);
            global = JS_GetGlobalObject(v.ctx);
            addz = JS_GetPropertyStr(v.ctx, global, "addz");
            s1 = JS_GetPropertyStr(v.ctx, global, "s1");
            r = white_call_failing(v.ctx, addz, JS_UNDEFINED, 1, &s1, fail, &made);
            if (fail == 0)
                n = made;
            cases++;
            if (JS_IsException(r))
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            JS_FreeValue(v.ctx, r);
            twins += white_shape_id_twins(v.rt);
            shape_ids_ok(&v, "a copy that could not take its key", cache);
            CHECK(eval_is(v.ctx, "Object.keys(s2) + ' ' + Object.keys(s1).slice(0, 4) + ' ' + s1.d + s2.d",
                          "a,b,c,d a,b,c,d 12"),
                  "cache %d, call %d failing: the keys of the objects", cache, fail);
            JS_FreeValue(v.ctx, s1);
            JS_FreeValue(v.ctx, addz);
            JS_FreeValue(v.ctx, global);
            vm_free(&v);
        }
        CHECK(n > 0, "cache %d: adding the key makes no call of the allocator", cache);
    }
    CHECK(twins > 0, "no failing call of %ld left two shapes of one id", cases);
}

/* The shapes of the prototype the global expression proto names: *count of
   them, *parented of those that hold a parent, and no shape of the runtime
   breaks the rule of parents (white_shape_parents_bad). */
static void shape_parents_ok(VM *v, const char *proto, const char *what, int cache,
                             int *count, int *parented)
{
    JSValue p = JS_Eval(v->ctx, proto, strlen(proto), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int bad;

    CHECK(JS_IsObject(p), "cache %d, %s: %s is no object", cache, what, proto);
    bad = white_shape_parents_bad(v->rt, p, count, parented);
    CHECK(bad == 0, "cache %d, %s: %d shapes break the rule of parents", cache, what, bad);
    JS_FreeValue(v->ctx, p);
    shape_ids_ok(v, what, cache);
}

static int64_t shape_count(VM *v)
{
    JSMemoryUsage u;

    JS_RunGC(v->rt);
    JS_ComputeMemoryUsage(v->rt, &u);
    return u.shape_count;
}

/* new F() of a function whose body sets four keys on this, with the shape
   cache on and off (js_shape_leave): once a few objects were made and kept,
   the shapes of F.prototype are the empty one and one for each key, each but
   the empty one holding the one before it, and a hundred more objects make no
   shape. Objects that leave the shape of the others (a delete, a new
   prototype, a freeze, a key of their own) keep the rule, and once every
   object is gone, so is every shape of F.prototype. A class with a field and
   a derived class do the same (B.prototype also has the shape of
   D.prototype, which has no parent). A constructor of 32 keys keeps a chain
   of 33 shapes, one of 33 keys none (a parent past 32 keys is not taken, and
   a chain is found from its last shape back), and a constructor whose
   objects all go one by one, none kept, makes no chain (the first one makes
   T.prototype, which has a shape of its own). */
static void test_new_shapes(void)
{
    static const char setup[] =
        "function F(v) { this.a = v; this.b = v; this.c = v; this.d = v }\n"
        "class B { x = 1; constructor(v) { this.y = v } }\n"
        "class D extends B { constructor(v) { super(v); this.z = v } }\n"
        "function L(n) { for (let i = 0; i < n; i++) this['k' + i] = i }\n"
        "function M(n) { for (let i = 0; i < n; i++) this['k' + i] = i }\n"
        "function T(v) { this.p = v; this.q = v }\n"
        "var keep = [], bs = [], ls = [], ms = [];\n"
        "function make(n) { for (let i = 0; i < n; i++) keep.push(new F(i)) }\n"
        "void 0";
    int cache;

    for (cache = 0; cache <= 1; cache++) {
        VM v = vm_new();
        int count, parented;
        int64_t before;

        JS_SetShapeCache(v.ctx, cache ? 2048 : 0);
        CHECK(eval_ok(v.ctx, setup), "the setup threw");
        CHECK(eval_ok(v.ctx, "make(10); void 0"), "make threw");
        shape_parents_ok(&v, "F.prototype", "ten objects", cache, &count, &parented);
        CHECK(count == 5 && parented == 4, "cache %d, ten objects: %d shapes of F.prototype, %d with a parent, want 5 and 4",
              cache, count, parented);
        before = shape_count(&v);
        CHECK(eval_ok(v.ctx, "make(100); void 0"), "make threw");
        CHECK(shape_count(&v) == before, "cache %d: 100 more objects made %lld shapes", cache,
              (long long)(shape_count(&v) - before));
        CHECK(eval_is(v.ctx, "keep.length + ' ' + Object.keys(keep[109]) + ' ' + keep[109].d", "110 a,b,c,d 99"),
              "cache %d: the keys of the objects", cache);

        CHECK(eval_ok(v.ctx,
                      "delete keep[0].b; Object.setPrototypeOf(keep[1], {}); Object.freeze(keep[2]);\n"
                      "keep[3].e = 1; keep[4].e = 1; keep[5].a = 'x'; make(5); void 0"), "the changes threw");
        shape_parents_ok(&v, "F.prototype", "objects that left the shape", cache, &count, &parented);
        CHECK(eval_is(v.ctx, "[0, 1, 2, 3, 114].map(i => Object.keys(keep[i])).join(' ')",
                      "a,c,d a,b,c,d a,b,c,d a,b,c,d,e a,b,c,d"), "cache %d: keys after the changes", cache);

        CHECK(eval_ok(v.ctx, "for (let i = 0; i < 20; i++) { bs.push(new B(i)); bs.push(new D(i)) } void 0"),
              "the classes threw");
        shape_parents_ok(&v, "B.prototype", "class B", cache, &count, &parented);
        CHECK(count == 4 && parented == 2, "cache %d, class B: %d shapes, %d with a parent, want 4 and 2",
              cache, count, parented);
        shape_parents_ok(&v, "D.prototype", "class D", cache, &count, &parented);
        CHECK(count == 4 && parented == 3, "cache %d, class D: %d shapes, %d with a parent, want 4 and 3",
              cache, count, parented);

        CHECK(eval_ok(v.ctx, "for (let i = 0; i < 40; i++) { ls.push(new L(32)); ms.push(new M(33)) } void 0"), "L threw");
        shape_parents_ok(&v, "L.prototype", "32 keys", cache, &count, &parented);
        CHECK(count == 33 && parented == 32, "cache %d, 32 keys: %d shapes, %d with a parent, want 33 and 32",
              cache, count, parented);
        shape_parents_ok(&v, "M.prototype", "33 keys", cache, &count, &parented);
        CHECK(count == 1 && parented == 0, "cache %d, 33 keys: %d shapes, %d with a parent, want 1 and 0",
              cache, count, parented);

        CHECK(eval_ok(v.ctx, "new T(0); void 0"), "T threw");
        before = shape_count(&v);
        CHECK(eval_ok(v.ctx, "for (let i = 0; i < 50; i++) new T(i); void 0"), "T threw");
        shape_parents_ok(&v, "T.prototype", "objects that go at once", cache, &count, &parented);
        CHECK(count == 0 && shape_count(&v) == before, "cache %d, objects that go at once: %d shapes left, %lld more in all",
              cache, count, (long long)(shape_count(&v) - before));

        CHECK(eval_ok(v.ctx, "keep.length = 0; bs.length = 0; ls.length = 0; ms.length = 0; void 0"), "the drop threw");
        JS_RunGC(v.rt);
        shape_parents_ok(&v, "F.prototype", "no object", cache, &count, &parented);
        CHECK(count == 0, "cache %d: %d shapes of F.prototype with no object", cache, count);
        shape_parents_ok(&v, "D.prototype", "no object", cache, &count, &parented);
        CHECK(count == 0, "cache %d: %d shapes of D.prototype with no object", cache, count);
        shape_parents_ok(&v, "L.prototype", "no object", cache, &count, &parented);
        CHECK(count == 0, "cache %d: %d shapes of L.prototype with no object", cache, count);
        vm_free(&v);
    }
}

/* new F() and new of a class with each allocation failing in turn, in a new
   VM each time (each_vm_failing): the object, its property array, the empty
   shape, a shape grown or copied, the property array growing when the object
   takes a shape found with more room. Objects are kept and dropped, so that
   the shapes of earlier ones are found and given a parent, and some of them
   go before the next ones. The setup makes enough objects of F, C and W
   that their chains are whole before the call, so that the call leaves as
   many shapes as it found, and none of G, whose chain the call makes and
   frees with its objects. The setup calls f once, so that what only a
   first call leaves is there before. With the shape cache on and off. Each
   call throws the out of memory error or gives the right answer, and leaves
   as many objects, strings and shapes as before it. */
static void test_new_shapes_each_failing(void)
{
    static const char setup[] =
        "function F(v) { this.a = v; this.b = {}; this.c = v }\n"
        "class C { x = {}; constructor(v) { this.y = v; this.z = {} } }\n"
        "function W(v) { this.p = v; this.q = v; this.r = v; this.s = v; this.t = {} }\n"
        "function G(v) { this.g = v; this.h = {}; this.i = v; this.j = v }\n"
        "G.prototype;\n"
        "var held = []; for (let i = 0; i < 8; i++) held.push(new F(i), new C(i), new W(i));\n";
    static const struct { const char *what, *f, *want; } cases[] = {
        { "new F, kept", "function f() { var k = []; for (var i = 0; i < 8; i++) k.push(new F(i)); return k.map(o => Object.keys(o).length).join('') }", "33333333" },
        { "new F, dropped", "function f() { var s = 0; for (var i = 0; i < 8; i++) s += new F(i).c; return s }", "28" },
        { "new class", "function f() { var k = []; for (var i = 0; i < 6; i++) k.push(new C(i)); return k.map(o => Object.keys(o).join('')).join(' ') }", "xyz xyz xyz xyz xyz xyz" },
        { "new W, five keys", "function f() { var k = []; for (var i = 0; i < 6; i++) k.push(new W(i)); held = null; return k.map(o => Object.keys(o).length).join('') }", "555555" },
        { "new G, the chain made in the call", "function f() { var k = []; for (var i = 0; i < 6; i++) k.push(new G(i)); return k.map(o => Object.keys(o).length).join('') }", "444444" },
    };
    char buf[4096];
    int i, nocache;

    for (i = 0; i < (int)(sizeof(cases) / sizeof(cases[0])); i++) {
        for (nocache = 0; nocache <= 1; nocache++) {
            snprintf(buf, sizeof(buf), "%s%s\nf();\n", setup, cases[i].f);
            each_vm_failing(cases[i].what, nocache, buf, cases[i].want, 3);
        }
    }
}

/* New GC objects are young, a young collection makes what it keeps old, and a
   full collection and JS_ComputeMemoryUsage see the young objects. */
static void test_young_lists(void)
{
    VM v = vm_new();
    int old, young, old2, young2;
    JSMemoryUsage u;

    JS_SetGCThreshold(v.rt, (size_t)-1);
    JS_RunGC(v.rt);
    lists(&v, &old, &young, "after a full collection");
    CHECK(young == 0, "%d young objects after a full collection", young);
    eval_ok(v.ctx, "globalThis.kept = Array.from({length: 100}, (_, i) => ({i})); void 0");
    lists(&v, &old, &young, "after making 100 objects");
    CHECK(young >= 100, "%d young objects after making 100", young);
    JS_RunYoungGC(v.rt);
    lists(&v, &old2, &young2, "after a young collection");
    CHECK(young2 == 0 && old2 >= old + 100, "after a young collection: %d old, %d young, before %d old, %d young", old2, young2, old, young);
    eval_ok(v.ctx, "kept.push({}); void 0");
    JS_ComputeMemoryUsage(v.rt, &u);
    lists(&v, &old, &young, "after JS_ComputeMemoryUsage");
    CHECK(young == 0, "%d young objects after JS_ComputeMemoryUsage", young);
    vm_free(&v);
}

/* After a young collection, no object that only the garbage held is left on a
   list with a count of 0, where the weak pass of the next collection would
   free it while still on the list. */
static void test_young_leaves_no_zero_count(void)
{
    static const char *const observe[] = {
        "", "wm.set(old, 1);", "wr = new WeakRef(old);", "fr.register(old, 1);",
        "old.f = async function () {};", "old.g = (function* () { yield 1 })(); old.g.next();",
    };
    size_t i;

    for (i = 0; i < countof(observe); i++) {
        VM v = vm_new();
        char src[400];
        int old, young;

        JS_SetGCThreshold(v.rt, (size_t)-1);
        snprintf(src, sizeof(src),
                 "globalThis.wm = new WeakMap(); globalThis.wr = null; globalThis.fr = new FinalizationRegistry(() => {});\n"
                 "globalThis.old = {inner: {deep: [1, 2, 3]}}; %s void 0", observe[i]);
        eval_ok(v.ctx, src);
        /* the job of the setup ends, and with it the hold of the target of
           a new WeakRef (docs/internals/changes/weak-collections.md) */
        JS_ClearKeptObjects(v.rt);
        JS_RunGC(v.rt);
        eval_ok(v.ctx, "(() => { const a = {old}, b = {a}; a.b = b; old = null })(); void 0");
        JS_RunYoungGC(v.rt);
        lists(&v, &old, &young, "after a young collection");
        JS_RunYoungGC(v.rt);
        JS_RunGC(v.rt);
        lists(&v, &old, &young, "after a full collection");
        CHECK(eval_is(v.ctx, "String(wr === null || wr.deref() === undefined)", "true"), "observer %zu: the weak reference holds the object", i);
        vm_free(&v);
    }
}

/* A young collection treats the record of a weak map whose key and value are
   young as a reference that holds only once the key is kept: it takes the
   reference to the value away before the scan and gives it back once, when
   the key is kept, or before it frees the garbage when the key is not and it
   deletes the record. Every object it keeps must then have the references
   from outside the GC objects it had before (white_gc_outside): for records
   in an old map and in a young one, a key kept before or after its map, a
   chain of records, a value kept another way and the same key in two maps.
   The young collection is also run with each of its calls of the allocator
   failing in turn: when the table of records cannot be made, every record
   holds its value as before the change (today), and the counts are as before
   all the same. key is what the WeakRef wr watches, and dead and today say
   whether the young collection frees it with and without its table. */
static void young_weak_case(size_t i, int fail, int *nallocs);

static const struct {
    const char *what, *body, *check;
    int dead, today; /* the young collection frees the key, with and without the table */
} young_weak_cases[] = {
    { "old map, the value holds its key", "let key = {}; wm.set(key, {key, p: [1, 2, 3]});", "true", 1, 0 },
    { "old map, a key in a cycle, the value kept another way", "let key = {}; key.self = key; hold = {p: [1]}; wm.set(key, hold);", "hold.p.length === 1", 1, 1 },
    { "old map, the value kept another way holds its key", "let key = {}; hold = {key}; wm.set(key, hold);", "wm.get(hold.key) === hold", 0, 0 },
    { "old map, a chain kept from its first key", "let a = {}, key = {}; hold = a; wm.set(key, {a, p: [1]}); wm.set(a, {key});", "wm.get(wm.get(hold).key).a === hold", 0, 0 },
    { "old map, a chain that is garbage", "let a = {}, key = {}; a.self = a; wm.set(a, {key}); wm.set(key, {a});", "true", 1, 0 },
    { "the same key in two old maps", "let key = {}; wm.set(key, {key}); wm2.set(key, {key});", "true", 1, 0 },
    { "young map kept, its key garbage", "const m = new WeakMap(); ym = m; let key = {}; m.set(key, {key});", "true", 1, 0 },
    { "young map kept, its key kept and made before it", "let key = {}; hold = key; const m = new WeakMap(); ym = m; m.set(key, {key, p: [1]});", "ym.get(hold).p.length === 1", 0, 0 },
    { "young map kept, its key kept and made after it", "const m = new WeakMap(); ym = m; let key = {}; hold = key; m.set(key, {key, p: [1]});", "ym.get(hold).p.length === 1", 0, 0 },
    { "young map that is garbage", "const m = new WeakMap(); let key = {}; hold = key; m.set(key, {key, m});", "true", 0, 0 },
    { "key held by an old object", "let key = {}; hold = key; wm.set(key, {key});", "wm.get(hold).key === hold", 0, 0 },
    { "old key", "let key = oldKey; wm.set(key, {key, p: [1]});", "wm.get(oldKey).p.length === 1", 0, 0 },
    { "the value is its key", "let key = {}; wm.set(key, key);", "true", 1, 0 },
    { "a closure of the value holds its key", "let r = {}; const rw = {attach(x) { r = x }}; rw.attach(r); wm.set(r, rw); let key = r;", "true", 1, 0 },
};

static void young_weak_case(size_t i, int fail, int *nallocs)
{
    VM v = vm_new();
    char src[1024], when[200];
    int old, young, n, bad;

    JS_SetGCThreshold(v.rt, (size_t)-1);
    CHECK(eval_ok(v.ctx, "globalThis.wm = new WeakMap(); globalThis.wm2 = new WeakMap(); globalThis.oldKey = {};"
                         "globalThis.wr = null; globalThis.hold = null; globalThis.ym = null; void 0"),
          "%s: the setup", young_weak_cases[i].what);
    JS_ClearKeptObjects(v.rt);
    JS_RunGC(v.rt);
    snprintf(src, sizeof(src), "(() => { %s wr = new WeakRef(key); })(); void 0", young_weak_cases[i].body);
    CHECK(eval_ok(v.ctx, src), "%s: the case", young_weak_cases[i].what);
    /* the job ends, and with it the hold of the target of the new WeakRef */
    JS_ClearKeptObjects(v.rt);
    snprintf(when, sizeof(when), "%s, call %d failing", young_weak_cases[i].what, fail);
    CHECK(white_gc_outside(v.rt, 1, &n) == 0, "%s: the counts before", when);
    white_young_gc_failing(v.rt, fail, nallocs);
    bad = white_gc_outside(v.rt, 0, &n);
    CHECK(bad == 0 && n > 0, "%s: %d of %d objects kept have another count from outside after the young collection", when, bad, n);
    lists(&v, &old, &young, when);
    CHECK(eval_is(v.ctx, young_weak_cases[i].check ? young_weak_cases[i].check : "true", "true"), "%s: the check after the young collection", when);
    if (fail == 0)
        CHECK(eval_is(v.ctx, "String(wr.deref() === undefined)", young_weak_cases[i].dead ? "true" : "false"),
              "%s: the young collection frees the key: want %d", when, young_weak_cases[i].dead);
    else if (fail == 1)
        CHECK(eval_is(v.ctx, "String(wr.deref() === undefined)", young_weak_cases[i].today ? "true" : "false"),
              "%s: without its table the young collection frees the key: want %d", when, young_weak_cases[i].today);
    JS_ClearKeptObjects(v.rt);
    CHECK(white_gc_outside(v.rt, 1, &n) == 0, "%s: the counts before the full collection", when);
    JS_RunGC(v.rt);
    bad = white_gc_outside(v.rt, 0, &n);
    CHECK(bad == 0, "%s: %d of %d objects kept have another count from outside after the full collection", when, bad, n);
    lists(&v, &old, &young, when);
    CHECK(eval_is(v.ctx, young_weak_cases[i].check, "true"), "%s: the check after the full collection", when);
    CHECK(eval_is(v.ctx, "String(wr.deref() === undefined)", young_weak_cases[i].dead ? "true" : "false"),
          "%s: the full collection frees the key: want %d", when, young_weak_cases[i].dead);
    vm_free(&v);
}

static void test_young_weak_counts(void)
{
    size_t i;
    int nallocs, fail, n;

    for (i = 0; i < countof(young_weak_cases); i++) {
        young_weak_case(i, 0, &nallocs);
        /* the cases with a young key make the table of records */
        if (strstr(young_weak_cases[i].what, "old key") == NULL)
            CHECK(nallocs >= 1, "%s: the young collection made no table", young_weak_cases[i].what);
        for (fail = 1; fail <= nallocs; fail++)
            young_weak_case(i, fail, &n);
    }
}

/* A finalizer that a collection runs as it frees its garbage can drop the
   last reference to an object outside the garbage: here the host holds the
   only strong reference to q, which no gc_mark reports, and the finalizer of
   a dead self cycle d lets it go. q is then freed after the garbage, as
   outside a collection (gc_defer), with its children, and leaves no object
   with a count of 0 on a list. The cases:
   RF_WEAK     a WeakRef to q: the weak pass after the garbage must not free
               the block of q while q is still on a list, and clears the
               WeakRef in that same collection
   RF_OLD      q made old by a full collection before d is made
   RF_YOUNG_GC the collection that frees d is JS_RunYoungGC
   RF_CHAIN    q is of a class whose finalizer drops the only reference to
               r, another root of the host, with a child of its own
   RF_FR_Q     a FinalizationRegistry watches q itself: its callback is
               queued once
   RF_MAP      a WeakMap with an entry whose key lives, so that a full
               collection judges its entries (gc_ephemeron), where without
               it the weak pass leaves none
   What a program sees, the callbacks of the registry (in the order of the
   registrations) and the WeakRef, is checked right after the collection
   that freed d, and the lists after it and after a full collection more.
   The leak check of these tests sees what is not freed. */
enum {
    RF_WEAK = 1, RF_OLD = 2, RF_YOUNG_GC = 4, RF_CHAIN = 8, RF_FR_Q = 16, RF_MAP = 32, RF_CASES = 64,
};

static struct {
    JSClassID class_id, chain_id;
    JSValue q, r;
    int finalized, chained;
} root_freed;

static void root_freed_finalizer(JSRuntime *rt, JSValue val)
{
    JS_FreeValueRT(rt, root_freed.q);
    root_freed.q = JS_UNDEFINED;
    root_freed.finalized++;
}

/* the finalizer of q in RF_CHAIN */
static void root_freed_chain_finalizer(JSRuntime *rt, JSValue val)
{
    JS_FreeValueRT(rt, root_freed.r);
    root_freed.r = JS_UNDEFINED;
    root_freed.chained++;
}

static JSValue root_freed_new(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    return JS_NewObjectClass(ctx, root_freed.class_id);
}

static JSValue root_freed_get(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    return JS_DupValue(ctx, argc > 0 && JS_ToBool(ctx, argv[0]) ? root_freed.r : root_freed.q);
}

static void root_freed_case(int f)
{
    JSClassDef def = { "RootFreed", root_freed_finalizer };
    JSClassDef chain = { "RootFreedChain", root_freed_chain_finalizer };
    VM v = vm_new();
    JSContext *jctx;
    JSValue g;
    char want[16];
    int old, nyoung;

    memset(&root_freed, 0, sizeof(root_freed));
    root_freed.q = root_freed.r = JS_UNDEFINED;
    JS_SetGCThreshold(v.rt, (size_t)-1);
    JS_NewClassID(v.rt, &root_freed.class_id);
    CHECK(JS_NewClass(v.rt, root_freed.class_id, &def) == 0, "JS_NewClass");
    JS_NewClassID(v.rt, &root_freed.chain_id);
    CHECK(JS_NewClass(v.rt, root_freed.chain_id, &chain) == 0, "JS_NewClass");
    g = JS_GetGlobalObject(v.ctx);
    JS_SetPropertyStr(v.ctx, g, "rootFreed", JS_NewCFunction(v.ctx, root_freed_new, "rootFreed", 0));
    JS_SetPropertyStr(v.ctx, g, "rootGet", JS_NewCFunction(v.ctx, root_freed_get, "rootGet", 1));
    JS_FreeValue(v.ctx, g);
    CHECK(eval_ok(v.ctx, "var got = [], fr = new FinalizationRegistry((v) => got.push(v)), wr = null, keep = {}, wm = new WeakMap(); void 0"), "the setup");
    if (f & RF_MAP)
        CHECK(eval_ok(v.ctx, "wm.set(keep, {}); void 0"), "the entry of a live key");
    root_freed.q = f & RF_CHAIN ? JS_NewObjectClass(v.ctx, root_freed.chain_id) : JS_NewObject(v.ctx);
    CHECK(!JS_IsException(root_freed.q), "q");
    CHECK(eval_ok(v.ctx, "(() => { const q = rootGet(); q.c = {}; fr.register(q.c, 'c') })(); void 0"), "q's child");
    snprintf(want, sizeof(want), "c");
    if (f & RF_CHAIN) {
        root_freed.r = JS_NewObject(v.ctx);
        CHECK(!JS_IsException(root_freed.r), "r");
        CHECK(eval_ok(v.ctx, "(() => { const r = rootGet(true); r.c = {}; fr.register(r.c, 'r') })(); void 0"), "r's child");
        strcat(want, ",r");
    }
    if (f & RF_FR_Q) {
        CHECK(eval_ok(v.ctx, "fr.register(rootGet(), 'q'); void 0"), "q in the registry");
        strcat(want, ",q");
    }
    if (f & RF_WEAK)
        CHECK(eval_ok(v.ctx, "wr = new WeakRef(rootGet()); void 0"), "the WeakRef");
    /* the job ends, and with it the hold of the target of a new WeakRef */
    JS_ClearKeptObjects(v.rt);
    if (f & RF_OLD)
        JS_RunGC(v.rt);
    CHECK(eval_ok(v.ctx, "(() => { const d = rootFreed(); d.self = d })(); void 0"), "the cycle");
    if (f & RF_YOUNG_GC)
        JS_RunYoungGC(v.rt);
    else
        JS_RunGC(v.rt);
    CHECK(root_freed.finalized == 1, "case %d: the cycle was finalized %d times", f, root_freed.finalized);
    CHECK(root_freed.chained == !!(f & RF_CHAIN), "case %d: q was finalized %d times", f, root_freed.chained);
    lists(&v, &old, &nyoung, "after the collection whose finalizer dropped q");
    while (JS_ExecutePendingJob(v.rt, &jctx) > 0)
        ;
    CHECK(eval_is(v.ctx, "got.join()", want), "case %d: the callbacks after the collection that freed q are not %s", f, want);
    if (f & RF_WEAK)
        CHECK(eval_is(v.ctx, "String(wr.deref())", "undefined"), "case %d: the WeakRef still gives q", f);
    if (f & RF_MAP)
        CHECK(eval_is(v.ctx, "String(wm.has(keep))", "true"), "case %d: the entry of the live key is gone", f);
    JS_RunGC(v.rt);
    lists(&v, &old, &nyoung, "after the next full collection");
    while (JS_ExecutePendingJob(v.rt, &jctx) > 0)
        ;
    CHECK(eval_is(v.ctx, "got.join()", want), "case %d: the callbacks after the next collection are not %s", f, want);
    vm_free(&v);
}

static void test_finalizer_frees_a_root(void)
{
    int f;

    for (f = 0; f < RF_CASES; f++) {
        /* the weak map only changes the path of a full collection */
        if ((f & RF_MAP) && (f & RF_YOUNG_GC))
            continue;
        root_freed_case(f);
    }
}


/* An object that a finalizer makes while a full collection deletes the
   entries of dead WeakMap keys (gc_remove_weak_objects) goes to the young
   list. The collection splices the young list after that pass, so gc_decref
   walks the object like any other: held by an old object of the host, it
   stays, with counts that add up, and the runtime is freed whole at vm_free
   (the leak check of these tests). Spliced before the pass, the object stayed
   on the young list, the scan reached it through its holder, moved it to
   gc_obj_list and added a count to each of its children, its prototype among
   them, that gc_decref never took away. Then a child given to it and dropped
   with it, after a young collection, is freed. The holder (holder()) is an
   object of the host whose gc_mark reports the object it holds, held() gives
   that object and drop() lets it go. */
static struct {
    JSContext *ctx;
    JSClassID holder_id, maker_id;
    JSValue held;
    int made;
} made_in_gc;

static void made_in_gc_mark(JSRuntime *rt, JSValueConst val, JS_MarkFunc *mark_func)
{
    JS_MarkValue(rt, made_in_gc.held, mark_func);
}

static void made_in_gc_holder_finalizer(JSRuntime *rt, JSValue val)
{
    JS_FreeValueRT(rt, made_in_gc.held);
    made_in_gc.held = JS_UNDEFINED;
}

/* a maker, freed with the entry of its WeakMap, gives the holder a new object */
static void made_in_gc_maker_finalizer(JSRuntime *rt, JSValue val)
{
    JSValue x = JS_NewObject(made_in_gc.ctx);

    CHECK(!JS_IsException(x), "no object made in the finalizer");
    JS_FreeValueRT(rt, made_in_gc.held);
    made_in_gc.held = x;
    made_in_gc.made++;
}

static JSValue made_in_gc_holder(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    return JS_NewObjectClass(ctx, made_in_gc.holder_id);
}

static JSValue made_in_gc_maker(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    return JS_NewObjectClass(ctx, made_in_gc.maker_id);
}

static JSValue made_in_gc_held(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    return JS_DupValue(ctx, made_in_gc.held);
}

static JSValue made_in_gc_drop(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    JS_FreeValue(ctx, made_in_gc.held);
    made_in_gc.held = JS_UNDEFINED;
    return JS_UNDEFINED;
}

/* other = 1 keeps a second entry, whose key lives, in the weak map: the
   full collection then takes the path of gc_ephemeron with entries, and
   with other = 0 the one where the weak pass left none. */
static void made_in_gc_case(int other)
{
    JSClassDef holder = { "MadeInGCHolder", made_in_gc_holder_finalizer, made_in_gc_mark };
    JSClassDef maker = { "MadeInGCMaker", made_in_gc_maker_finalizer };
    VM v = vm_new();
    JSContext *jctx;
    JSValue g;
    int old, young;

    memset(&made_in_gc, 0, sizeof(made_in_gc));
    made_in_gc.ctx = v.ctx;
    made_in_gc.held = JS_UNDEFINED;
    JS_SetGCThreshold(v.rt, (size_t)-1);
    JS_NewClassID(v.rt, &made_in_gc.holder_id);
    CHECK(JS_NewClass(v.rt, made_in_gc.holder_id, &holder) == 0, "JS_NewClass");
    JS_NewClassID(v.rt, &made_in_gc.maker_id);
    CHECK(JS_NewClass(v.rt, made_in_gc.maker_id, &maker) == 0, "JS_NewClass");
    g = JS_GetGlobalObject(v.ctx);
    JS_SetPropertyStr(v.ctx, g, "holder", JS_NewCFunction(v.ctx, made_in_gc_holder, "holder", 0));
    JS_SetPropertyStr(v.ctx, g, "maker", JS_NewCFunction(v.ctx, made_in_gc_maker, "maker", 0));
    JS_SetPropertyStr(v.ctx, g, "held", JS_NewCFunction(v.ctx, made_in_gc_held, "held", 0));
    JS_SetPropertyStr(v.ctx, g, "drop", JS_NewCFunction(v.ctx, made_in_gc_drop, "drop", 0));
    JS_FreeValue(v.ctx, g);
    CHECK(eval_ok(v.ctx, "var h = holder(), got = [], fr = new FinalizationRegistry((v) => got.push(v)), y = null; void 0"), "the setup");
    JS_RunGC(v.rt);
    /* the key dies at once, and its entry waits for the next collection */
    CHECK(eval_ok(v.ctx, "var wm = new WeakMap(); (function () { wm.set({}, maker()) })(); void 0"), "a weak map");
    if (other)
        CHECK(eval_ok(v.ctx, "var keep = {}; wm.set(keep, {}); void 0"), "a second entry");
    JS_RunGC(v.rt);
    CHECK(made_in_gc.made == 1, "other %d: the maker made %d objects in the collection", other, made_in_gc.made);
    lists(&v, &old, &young, "after the full collection whose weak pass made an object");
    /* the object gets a child only it holds, and its only reference moves from
       the old holder to a young object the global holds */
    CHECK(eval_ok(v.ctx, "(function () { const x = held(); x.c = {}; fr.register(x.c, 'c'); y = { x }; drop() })(); void 0"),
          "the child");
    JS_RunYoungGC(v.rt);
    lists(&v, &old, &young, "after the young collection");
    CHECK(eval_ok(v.ctx, "y = null; void 0"), "the drop");
    JS_RunGC(v.rt);
    while (JS_ExecutePendingJob(v.rt, &jctx) > 0)
        ;
    CHECK(eval_is(v.ctx, "got.join()", "c"), "other %d: the child of the object made in the collection is still held", other);
    if (other)
        CHECK(eval_is(v.ctx, "String(wm.has(keep))", "true"), "other %d: the entry of the live key is gone", other);
    vm_free(&v);
}

static void test_object_made_in_the_weak_pass(void)
{
    made_in_gc_case(0);
    made_in_gc_case(1);
}

/* A finalizer of a class of the host that runs javascript while a collection
   frees its garbage never gets an object of that garbage from a WeakRef:
   the collection clears every weak reference to its garbage before it frees
   any of it, as the spec clears the targets at once. Before, a WeakRef was
   cleared only after the free, and deref returned a target whose count was
   not 0, which the garbage gives itself: the kept set and the global then
   held an object that the collection went on to free. Two such objects run
   wr.deref(), one made before the target and one after, so that one of them
   is freed before it whatever the order of the free. */
static struct {
    JSContext *ctx;
    JSClassID class_id;
    int calls, errors, got, freed;
} deref_in_gc;

static void deref_in_gc_finalizer(JSRuntime *rt, JSValue val)
{
    JSContext *ctx = deref_in_gc.ctx;
    JSValue r = JS_Eval(ctx, "saved = wr.deref()", 18, "<finalizer>", JS_EVAL_TYPE_GLOBAL);

    deref_in_gc.calls++;
    if (JS_IsException(r)) {
        deref_in_gc.errors++;
        JS_FreeValue(ctx, JS_GetException(ctx));
    } else if (JS_IsObject(r)) {
        deref_in_gc.got++;
        if (white_being_freed(r))
            deref_in_gc.freed++;
    }
    JS_FreeValue(ctx, r);
}

static JSValue deref_in_gc_new(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    return JS_NewObjectClass(ctx, deref_in_gc.class_id);
}

static const struct {
    const char *what, *body;
    int young_frees, registered;
} deref_in_gc_cases[] = {
    { "a cycle", "k.self = k;", 1, 0 },
    { "a cycle in a registry", "k.self = k; fr.register(k, 'k');", 1, 1 },
    { "the key of an old weak map whose value holds it", "wm.set(k, {k});", 1, 0 },
};

static void deref_in_gc_case(int i, int young)
{
    JSClassDef def = { "DerefInGC", deref_in_gc_finalizer };
    VM v = vm_new();
    JSContext *jctx;
    JSValue g;
    char src[256], *t;
    int freed;

    memset(&deref_in_gc, 0, sizeof(deref_in_gc));
    deref_in_gc.ctx = v.ctx;
    JS_SetGCThreshold(v.rt, (size_t)-1);
    JS_NewClassID(v.rt, &deref_in_gc.class_id);
    CHECK(JS_NewClass(v.rt, deref_in_gc.class_id, &def) == 0, "JS_NewClass");
    g = JS_GetGlobalObject(v.ctx);
    JS_SetPropertyStr(v.ctx, g, "derefInGC", JS_NewCFunction(v.ctx, deref_in_gc_new, "derefInGC", 0));
    JS_FreeValue(v.ctx, g);
    CHECK(eval_ok(v.ctx, "var wr, saved, got = [], fr = new FinalizationRegistry((v) => got.push(v)), wm = new WeakMap(); void 0"), "the setup");
    JS_RunGC(v.rt);
    snprintf(src, sizeof(src), "(() => { const f = derefInGC(); f.self = f; const k = {x: 1}; %s"
             " wr = new WeakRef(k); const f2 = derefInGC(); f2.self = f2 })(); void 0", deref_in_gc_cases[i].body);
    CHECK(eval_ok(v.ctx, src), "%s: the objects", deref_in_gc_cases[i].what);
    /* the job ends, and with it the hold of the target of the new WeakRef */
    JS_ClearKeptObjects(v.rt);
    if (young)
        JS_RunYoungGC(v.rt);
    else
        JS_RunGC(v.rt);
    CHECK(deref_in_gc.calls == 2 && deref_in_gc.errors == 0, "%s, young %d: %d finalizers ran, %d threw",
          deref_in_gc_cases[i].what, young, deref_in_gc.calls, deref_in_gc.errors);
    CHECK(deref_in_gc.freed == 0, "%s, young %d: a finalizer got the target from the WeakRef %d times while the collection freed it",
          deref_in_gc_cases[i].what, young, deref_in_gc.freed);
    if (deref_in_gc.freed)
        return; /* the global and the kept set hold a freed object */
    JS_ClearKeptObjects(v.rt);
    t = eval_text(v.ctx, "String(wr.deref())");
    CHECK(t != NULL, "%s, young %d: wr.deref() threw", deref_in_gc_cases[i].what, young);
    freed = t && strcmp(t, "undefined") == 0;
    free(t);
    if (young)
        CHECK(freed == deref_in_gc_cases[i].young_frees, "%s, young %d: the collection freed the target: %d",
              deref_in_gc_cases[i].what, young, freed);
    if (!young)
        CHECK(freed, "%s: the full collection kept the target", deref_in_gc_cases[i].what);
    if (freed)
        CHECK(deref_in_gc.got == 0 && eval_is(v.ctx, "String(saved)", "undefined"),
              "%s, young %d: a finalizer got the freed target %d times", deref_in_gc_cases[i].what, young, deref_in_gc.got);
    else
        CHECK(eval_is(v.ctx, "String(saved.x)", "1"), "%s, young %d: the kept target lost its property",
              deref_in_gc_cases[i].what, young);
    CHECK(eval_ok(v.ctx, "saved = undefined; void 0"), "the drop");
    /* a deref above keeps its target to the end of the job */
    JS_ClearKeptObjects(v.rt);
    JS_RunGC(v.rt);
    while (JS_ExecutePendingJob(v.rt, &jctx) > 0)
        ;
    CHECK(eval_is(v.ctx, "String(wr.deref())", "undefined"), "%s, young %d: the next full collection kept the target",
          deref_in_gc_cases[i].what, young);
    if (deref_in_gc_cases[i].registered)
        CHECK(eval_is(v.ctx, "got.join()", "k"), "%s, young %d: the callback of the registry", deref_in_gc_cases[i].what, young);
    vm_free(&v);
}

static void test_deref_in_gc(void)
{
    int i;

    for (i = 0; i < (int)(sizeof(deref_in_gc_cases) / sizeof(deref_in_gc_cases[0])); i++) {
        deref_in_gc_case(i, 1);
        deref_in_gc_case(i, 0);
    }
}

/* A collection makes the job of a FinalizationRegistry after it has freed its
   garbage, so the memory of a target is free by then: a runtime at its limit
   (the run with no failing call) gets the callback of a target that held a
   lot. Each of the allocations of the collection fails in turn as well:
   the garbage is freed whatever fails, and the callback is
   lost only when its own job is the call that fails, as a collection cannot
   throw. */
static void test_registry_job_after_free(void)
{
    VM v;
    JSContext *jctx;
    size_t base;
    int young, fail, nallocs, early, total, lost;
    char *t;

    for (young = 1; young >= 0; young--) {
        total = 1;
        lost = 0;
        for (fail = 0; fail <= total; fail++) {
            v = vm_new();
            JS_SetGCThreshold(v.rt, (size_t)-1);
            CHECK(eval_ok(v.ctx, "var calls = [], fr = new FinalizationRegistry((h) => calls.push(h)); void 0"), "the setup");
            JS_RunGC(v.rt);
            base = JS_GetMemorySize(v.rt);
            CHECK(eval_ok(v.ctx, "(() => { const t = { p: 'x'.repeat(1 << 21) }; t.self = t; fr.register(t, 'big') })(); void 0"),
                  "the target");
            if (fail == 0)
                JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt));
            white_gc_failing(v.rt, young, fail, &nallocs, &early);
            unlimit(&v);
            if (fail == 0)
                total = nallocs;
            CHECK(early == 0, "young %d, call %d failing: %d allocations before the garbage was freed", young, fail, early);
            CHECK(JS_GetMemorySize(v.rt) < base + (1 << 20), "young %d, call %d failing: the target was not freed", young, fail);
            while (JS_ExecutePendingJob(v.rt, &jctx) > 0)
                ;
            t = eval_text(v.ctx, "calls.join()");
            CHECK(t != NULL, "calls");
            if (fail == 0)
                CHECK(t && strcmp(t, "big") == 0, "young %d: the callbacks are %s", young, t ? t : "?");
            else if (t && strcmp(t, "big") != 0)
                lost++;
            free(t);
            vm_free(&v);
        }
        CHECK(total >= 1 && lost <= 1, "young %d: %d allocations, the callback lost %d times", young, total, lost);
    }
}

/* An array that grows within JS_GC_BLOCK_SLACK of the memory limit runs a
   full collection first (js_trigger_gc_block), and one that leaves the heap
   near the limit makes the next ones wait (js_trigger_gc_block_run). A render whose memory
   stays in that band, its garbage freed by the counts, collected at every
   growth: here 1000 calls that each grow an array a few times and free it,
   with room for it under the limit, ran 6000 full collections. */
static void test_block_gc_near_limit(void)
{
    VM v = vm_new();
    JSValue g, f, r;
    unsigned long gcs;
    struct timespec t0, t1;
    int i, ok = 0;

    CHECK(eval_ok(v.ctx, "function step() { var a = []; for (var i = 0; i < 64; i++) a.push(i); return a.length }\nstep();"),
          "the script threw");
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "step");
    JS_RunGC(v.rt);
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + 6000);
    gcs = white_gc_run_count(0);
    clock_gettime(CLOCK_MONOTONIC, &t0);
    for (i = 0; i < 1000; i++) {
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
        if (JS_IsException(r))
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
        else
            ok++;
        JS_FreeValue(v.ctx, r);
    }
    clock_gettime(CLOCK_MONOTONIC, &t1);
    gcs = white_gc_run_count(0) - gcs;
    JS_SetMemoryLimit(v.rt, (size_t)-1);
    printf("unit-white: test_block_gc_near_limit: %lu full collections in %.1f ms\n", gcs,
           (t1.tv_sec - t0.tv_sec) * 1e3 + (t1.tv_nsec - t0.tv_nsec) / 1e6);
    CHECK(ok == 1000 && gcs <= 32, "1000 calls near the limit: %d gave their result, with %lu full collections", ok, gcs);
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, g);
    vm_free(&v);
}

/* Garbage in cycles of about n bytes in v, the collections of the
   threshold out of reach */
static void make_cycles(VM *v, size_t n)
{
    size_t base = JS_GetMemorySize(v->rt), k;

    JS_SetGCThreshold(v->rt, (size_t)1 << 40);
    for (k = 0; JS_GetMemorySize(v->rt) - base < n; k++) {
        if (!eval_ok(v->ctx, "cyc(16)") || k == 1000000) {
            CHECK(0, "the cycles threw or did not grow the heap after %lu calls: %lu bytes, %lu wanted",
                  (unsigned long)k, (unsigned long)(JS_GetMemorySize(v->rt) - base), (unsigned long)n);
            return;
        }
    }
}

/* 'x'.repeat(n) in v: 1 when made, 0 for out of memory */
static int repeat_block(VM *v, size_t n)
{
    char src[64];
    JSValue r;
    int ok;

    snprintf(src, sizeof(src), "globalThis.b = 'x'.repeat(%lu); 0", (unsigned long)n);
    r = JS_Eval(v->ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    ok = !JS_IsException(r);
    if (!ok) {
        char *t = exception_text(v->ctx);

        CHECK(t && strstr(t, "out of memory"), "the block threw %s, not out of memory", t ? t : "nothing");
        free(t);
    }
    JS_FreeValue(v->ctx, r);
    return ok;
}

/* The wait the collections near the limit start keeps the meaning of the
   limit: a block that fits only once the garbage in cycles is collected
   collects first and is made, as before, unless a wait is on, which a
   collection that leaves room, a full collection of the host, or the end of
   the wait (at most JS_GC_BLOCK_WAIT_MAX requests) ends. */
static void test_block_gc_wait(void)
{
    VM v;
    JSValue g, f, r;
    unsigned long gcs;
    size_t base;
    int i, tries;

    /* no wait: 200 KiB of cycles, and a block of 150 KiB under a limit
       300 KiB above what is live */
    v = vm_new();
    CHECK(eval_ok(v.ctx, "function cyc(n) { for (var i = 0; i < n; i++) { var a = { i: i }; a.self = a } }"),
          "the setup threw");
    JS_RunGC(v.rt);
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + (300 << 10));
    make_cycles(&v, 200 << 10);
    gcs = white_gc_run_count(0);
    CHECK(repeat_block(&v, 150 << 10), "with no wait, the block that fits once the cycles are collected was refused");
    CHECK(white_gc_run_count(0) - gcs == 1, "the block ran %lu collections", white_gc_run_count(0) - gcs);
    JS_SetMemoryLimit(v.rt, (size_t)-1);
    vm_free(&v);

    /* a wait: arrays that grow within the slack of the limit, until the
       wait is the longest, then the same cycles and block. The block fails
       while the wait lasts, and is made at its end. */
    v = vm_new();
    CHECK(eval_ok(v.ctx,
                  "function cyc(n) { for (var i = 0; i < n; i++) { var a = { i: i }; a.self = a } }\n"
                  "function step() { var a = []; for (var i = 0; i < 64; i++) a.push(i); return a.length }\nstep();"),
          "the setup threw");
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "step");
    JS_RunGC(v.rt);
    base = JS_GetMemorySize(v.rt);
    JS_SetMemoryLimit(v.rt, base + 6000);
    for (i = 0; i < 1000; i++) {
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
        CHECK(!JS_IsException(r), "step %d near the limit threw", i);
        JS_FreeValue(v.ctx, r);
    }
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, g);
    JS_SetMemoryLimit(v.rt, base + (300 << 10));
    make_cycles(&v, 200 << 10);
    gcs = white_gc_run_count(0);
    for (tries = 1; tries <= 2 * 1024 && !repeat_block(&v, 150 << 10); tries++)
        ;
    CHECK(tries > 1 && tries <= 1024 + 1, "in a wait, the block was made at try %d", tries);
    CHECK(white_gc_run_count(0) - gcs == 1, "the tries ran %lu collections", white_gc_run_count(0) - gcs);
    /* a collection of the host ends a wait: the same again */
    CHECK(eval_ok(v.ctx, "globalThis.b = undefined"), "the reset threw");
    JS_RunGC(v.rt);
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + 6000);
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "step");
    for (i = 0; i < 1000; i++) {
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
        JS_FreeValue(v.ctx, r);
    }
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, g);
    JS_RunGC(v.rt);
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + (300 << 10));
    make_cycles(&v, 200 << 10);
    CHECK(repeat_block(&v, 150 << 10), "after a collection of the host, the block was refused");
    JS_SetMemoryLimit(v.rt, (size_t)-1);
    vm_free(&v);
}

/* The lengths of the waits: a block request that collects and still finds
   no room starts a wait of 1, then 2, 4 and up to JS_GC_BLOCK_WAIT_MAX
   (1024) requests with no collection, which stays the length after that. A
   collection of the threshold ends the wait, and so does the next call of
   the host (JS_StartCall): the next wait starts again at 1. */
static void test_block_gc_wait_steps(void)
{
    VM v = vm_new();
    unsigned skip, wait, want = 1, k, i;
    unsigned long gcs;

    JS_RunGC(v.rt);
    /* a block of 1 byte comes within the slack of the limit, and a
       collection frees nothing */
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + 4096);
    for (k = 1; k <= 14; k++) {
        CHECK(white_block_gc(v.rt, 1), "wait %u: the request after the wait did not collect", k);
        wait = white_block_wait(v.rt, &skip);
        CHECK(wait == want && skip == want, "wait %u is %u requests (%u left), %u wanted", k, wait, skip, want);
        for (i = 0; i < wait; i++)
            if (white_block_gc(v.rt, 1)) {
                CHECK(0, "wait %u: request %u of %u collected", k, i + 1, wait);
                break;
            }
        CHECK(white_block_wait(v.rt, &skip) == wait && skip == 0, "wait %u: %u requests left at its end", k, skip);
        want = want * 2 > 1024 ? 1024 : want * 2;
    }
    /* in a wait of 8, the threshold collects at an object made */
    JS_RunGC(v.rt);
    for (k = 1; k <= 4; k++)
        while (!white_block_gc(v.rt, 1))
            ;
    CHECK(white_block_wait(v.rt, &skip) == 8 && skip == 8, "the wait is %u requests (%u left), 8 wanted",
          white_block_wait(v.rt, &skip), skip);
    JS_SetMemoryLimit(v.rt, (size_t)-1);
    JS_SetGCThreshold(v.rt, 0);
    gcs = white_gc_run_count(0);
    CHECK(eval_ok(v.ctx, "globalThis.o = {}"), "the object threw");
    CHECK(white_gc_run_count(0) - gcs >= 1, "the threshold ran no collection");
    CHECK(white_block_wait(v.rt, &skip) == 0 && skip == 0, "after the threshold the wait is %u requests (%u left)",
          white_block_wait(v.rt, &skip), skip);
    /* what the eval left, freed now, so that the next collection frees
       nothing */
    JS_RunGC(v.rt);
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + 4096);
    CHECK(white_block_gc(v.rt, 1), "the request after the threshold did not collect");
    CHECK(white_block_wait(v.rt, &skip) == 1 && skip == 1, "after the threshold the next wait is %u requests (%u left)",
          white_block_wait(v.rt, &skip), skip);
    /* the next call of the host ends a wait of 8, and its first request
       collects and starts one of 1 */
    while (white_block_wait(v.rt, &skip) != 8 || skip != 8)
        white_block_gc(v.rt, 1);
    JS_StartCall(v.rt);
    CHECK(white_block_wait(v.rt, &skip) == 0 && skip == 0, "after JS_StartCall the wait is %u requests (%u left)",
          white_block_wait(v.rt, &skip), skip);
    CHECK(white_block_gc(v.rt, 1), "the first request of the call did not collect");
    CHECK(white_block_wait(v.rt, &skip) == 1 && skip == 1, "the first wait of the call is %u requests (%u left)",
          white_block_wait(v.rt, &skip), skip);
    JS_SetMemoryLimit(v.rt, (size_t)-1);
    vm_free(&v);
}

/* The edge of the wait: a collection that leaves just JS_GC_BLOCK_SLACK
   (8 KiB) of room above the block starts none, one that leaves a byte less
   starts one. Here the collection frees cycles and leaves what was live. */
static void test_block_gc_wait_edge(void)
{
    VM v = vm_new();
    size_t base, size = 64 << 10, after;
    unsigned skip, wait;
    int d;

    CHECK(eval_ok(v.ctx, "function cyc(n) { for (var i = 0; i < n; i++) { var a = { i: i }; a.self = a } }"),
          "the setup threw");
    make_cycles(&v, 32 << 10);
    JS_RunGC(v.rt);
    for (d = 0; d <= 1; d++) {
        base = JS_GetMemorySize(v.rt);
        make_cycles(&v, 32 << 10);
        JS_SetMemoryLimit(v.rt, base + size + 8192 - d);
        CHECK(white_block_gc(v.rt, size), "%d below the edge: no collection", d);
        after = JS_GetMemorySize(v.rt);
        CHECK(after == base, "%d below the edge: the collection left %lu bytes, %lu before the cycles", d,
              (unsigned long)after, (unsigned long)base);
        wait = white_block_wait(v.rt, &skip);
        CHECK(wait == (unsigned)d && skip == (unsigned)d, "%d below the edge: a wait of %u requests (%u left)", d,
              wait, skip);
        JS_SetMemoryLimit(v.rt, (size_t)-1);
        JS_RunGC(v.rt);
    }
    vm_free(&v);
}

/* A collection allocates nothing while it has counts taken away (JS_GC_WINDOW
   in quickjs.c), here at the memory limit, where a request the heap refuses
   runs js_heap_drain and the kept shapes go. A young shape the shape cache
   keeps that a live young object uses has, after the decref pass, the count
   of the cache only: freed by a drain then, it is read again by the scan of
   the object (the sanitizer builds see the read). The young collection made
   the table of its weak map records (young_weak_start) after its decref pass,
   and 128 records make it a request to the heap. */
static void test_gc_alloc_at_limit(void)
{
    VM v;
    JSValue live, g;
    long before;
    int young, count;

    for (young = 1; young >= 0; young--) {
        v = vm_new();
        JS_SetGCThreshold(v.rt, (size_t)-1);
        JS_SetShapeCache(v.ctx, 64);
        CHECK(eval_ok(v.ctx,
                      "var wm = new WeakMap();\n"
                      "(function () { for (let i = 0; i < 128; i++) { const k = {}; k.self = k; wm.set(k, {k}) } })();\n"
                      "function mk(v) { var o = {}; o.zq = v; return o }\n"
                      "mk(1); void 0"),
              "the setup");
        live = JS_Eval(v.ctx, "mk(2)", 5, "<live>", JS_EVAL_TYPE_GLOBAL);
        CHECK(JS_IsObject(live), "mk(2)");
        count = JS_IsObject(live) ? white_kept_young_shape(v.rt, live) : 0;
        CHECK(count == 2, "young %d: the shape of the live object has a count of %d, want the cache's and its own", young, count);
        before = white_gc_window_count();
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt));
        if (young)
            JS_RunYoungGC(v.rt);
        else
            JS_RunGC(v.rt);
        unlimit(&v);
        CHECK(white_gc_window_count() == before, "young %d: %ld allocations while the collection had counts taken away",
              young, white_gc_window_count() - before);
        g = JS_GetGlobalObject(v.ctx);
        JS_SetPropertyStr(v.ctx, g, "live", live);
        JS_FreeValue(v.ctx, g);
        CHECK(eval_is(v.ctx, "JSON.stringify(live) + ':' + JSON.stringify(mk(3))", "{\"zq\":2}:{\"zq\":3}"),
              "young %d: the live object after the collection", young);
        vm_free(&v);
    }
}

/* The request for the table of the young weak records (young_weak_alloc) can
   run a drain of the heap, and the drain can free an object whose class has a
   finalizer of the host, which can run javascript: here it makes the record
   of a young key with the value 0, which the table was not counted for, a
   record whose young value waits for its key. The records are counted again
   after the request: with one more, the collection goes without the table,
   as when there is no memory for it, where it made room for 128 and queued
   129 (an assert, or a write past the table). */
static int young_weak_drain_ran;

static void young_weak_set_in_drain(void *arg)
{
    VM *v = arg;

    young_weak_drain_ran = eval_ok(v->ctx, "wm.set(k0, v0); void 0") ? 1 : -1;
}

static void test_young_weak_table_recount(void)
{
    VM v = vm_new();
    long before;

    young_weak_drain_ran = 0;
    JS_SetGCThreshold(v.rt, (size_t)-1);
    CHECK(eval_ok(v.ctx, "var wm = new WeakMap(); void 0"), "the map");
    JS_RunGC(v.rt);
    CHECK(eval_ok(v.ctx,
                  "(function () { for (let i = 0; i < 128; i++) { const k = {}; k.self = k; wm.set(k, {k}) } })();\n"
                  "var k0 = {}, v0 = { x: 1 }; wm.set(k0, 0); void 0"),
          "the records");
    before = white_gc_window_count();
    white_drain_once_then(young_weak_set_in_drain, &v);
    JS_RunYoungGC(v.rt);
    CHECK(young_weak_drain_ran == 1, "the code of the drain: %d", young_weak_drain_ran);
    CHECK(white_gc_window_count() == before, "%ld allocations while the collection had counts taken away",
          white_gc_window_count() - before);
    CHECK(eval_is(v.ctx, "String(wm.get(k0) === v0 && v0.x)", "1"), "the record the drain set");
    JS_RunGC(v.rt);
    CHECK(eval_is(v.ctx, "String(wm.get(k0) === v0 && v0.x)", "1"), "the record the drain set, after a full collection");
    vm_free(&v);
}

/* ---- a collection asked for while objects are freed ----

   The host runs code while the engine frees objects: the finalizer of a class
   of the host, and host_release for a function of JS_NewHostFunctionRelease.
   A collection started from there, by the host (JS_RunGC, JS_RunYoungGC) or by
   the threshold when the host makes an object, does nothing: it would free
   what the engine is in the middle of freeing, the object whose finalizer runs
   among them. white_gc_run_count counts the collections that ran. Each case
   runs in a process of its own, the test run again with the case as its
   arguments, so that a case that crashes is one failed case, and the process
   starts with no lock held and its own watchdog. */

enum {
    IN_FREE_RUN_GC,     /* JS_RunGC */
    IN_FREE_YOUNG_GC,   /* JS_RunYoungGC */
    IN_FREE_NEW_OBJECT, /* JS_NewObject with the heap past the threshold */
    IN_FREE_USAGE,      /* JS_ComputeMemoryUsage and JS_DumpMemoryUsage */
    IN_FREE_ACTIONS,
};

static const char *const in_free_actions[IN_FREE_ACTIONS] = {
    "JS_RunGC", "JS_RunYoungGC", "an object past the threshold", "JS_ComputeMemoryUsage",
};

/* How the objects are freed. */
enum {
    IN_FREE_COUNT,      /* the host drops the last reference to an array of
                           them: the array goes at once
                           (free_zero_refcount_first), and what it held in
                           free_zero_refcount */
    IN_FREE_EACH_YOUNG, /* the host drops the last reference to each object of
                           the array, made since the last collection: each goes
                           at once, a probe too (free_zero_refcount_first) */
    IN_FREE_EACH_OLD,   /* the same after a collection made them old */
    IN_FREE_CYCLE,      /* JS_RunGC frees a cycle: gc_free_cycles */
    IN_FREE_YOUNG,      /* JS_RunYoungGC frees a young cycle: gc_free_cycles */
    IN_FREE_DEFERRED,   /* JS_RunYoungGC frees a young cycle that held the last
                           reference to an old array: the array waits on
                           gc_dead_list and goes in free_zero_refcount */
    IN_FREE_WEAK,       /* JS_RunGC deletes the entry of a WeakMap whose key
                           died: gc_remove_weak_objects */
    IN_FREE_EPHEMERON,  /* JS_RunGC deletes the entry of a WeakMap whose key is
                           in a cycle: gc_ephemeron_prune, then gc_free_cycles */
    IN_FREE_RUNTIME,    /* the runtime is freed: a pending exception
                           (free_zero_refcount), then the global object
                           (gc_free_cycles) */
    IN_FREE_WAYS,
};

static const char *const in_free_ways[IN_FREE_WAYS] = {
    "the last reference dropped",
    "the last reference to each young object dropped",
    "the last reference to each old object dropped",
    "a cycle in JS_RunGC",
    "a cycle in JS_RunYoungGC",
    "an old array that a young cycle held",
    "the value of a WeakMap whose key died",
    "the value of a WeakMap whose key is in a cycle",
    "the runtime freed",
};

#define IN_FREE_PROBES 8

static struct {
    JSRuntime *rt;
    JSContext *ctx;
    JSClassID class_id;
    uint32_t vm;
    int action;
    int finalized, released, made, releasers;
    unsigned long runs; /* collections that ran in calls from the host */
} in_free;

static unsigned long in_free_run_count(void)
{
    return white_gc_run_count(0) + white_gc_run_count(1);
}

static void in_free_act(void)
{
    JSRuntime *rt = in_free.rt;
    unsigned long runs = in_free_run_count();
    JSMemoryUsage u;
    char buf[128];
    JSValue o;

    switch (in_free.action) {
    case IN_FREE_RUN_GC:
        JS_RunGC(rt);
        break;
    case IN_FREE_YOUNG_GC:
        JS_RunYoungGC(rt);
        break;
    case IN_FREE_NEW_OBJECT:
        o = JS_NewObject(in_free.ctx);
        CHECK(!JS_IsException(o), "no object made in a finalizer");
        JS_FreeValue(in_free.ctx, o);
        break;
    case IN_FREE_USAGE:
        JS_ComputeMemoryUsage(rt, &u);
        CHECK(JS_DumpMemoryUsage(buf, sizeof(buf), &u, rt) > 0, "an empty report");
        break;
    }
    in_free.runs += in_free_run_count() - runs;
}

static void in_free_finalizer(JSRuntime *rt, JSValue val)
{
    CHECK(rt == in_free.rt, "the finalizer of another runtime");
    in_free.finalized++;
    in_free_act();
}

static void in_free_release(uint32_t vm, uint32_t handle)
{
    CHECK(vm == in_free.vm, "a release of another VM");
    in_free.released++;
    in_free_act();
}

/* probe() makes an object of the class, release() a function whose release
   reaches the host. */
static JSValue in_free_probe(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    in_free.made++;
    return JS_NewObjectClass(ctx, in_free.class_id);
}

static JSValue in_free_releaser(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    JSValue f = JS_NewHostFunctionRelease(ctx, "r", 1, 0, ++in_free.releasers);

    if (JS_IsException(f))
        in_free.releasers--;
    return f;
}

/* make() gives an array of probes and functions to release, with plain
   objects between them, that the engine frees after the first finalizer ran. */
#define IN_FREE_MAKE \
    "function make() {\n" \
    "  var a = [];\n" \
    "  for (var i = 0; i < 8; i++) a.push({ i }, probe(), [i], release(), { o: { i } });\n" \
    "  return a;\n" \
    "}\n"

static void in_free_case(int way, int action)
{
    JSClassDef def = { "InFree", in_free_finalizer };
    VM v = vm_new();
    JSValue g, a, each[40];
    unsigned long full, young;
    int i, n = 0;

    memset(&in_free, 0, sizeof(in_free));
    in_free.rt = v.rt;
    in_free.ctx = v.ctx;
    in_free.vm = host_vm_id(v.rt);
    in_free.action = -1;
    host_release_call = in_free_release;
    JS_NewClassID(v.rt, &in_free.class_id);
    CHECK(JS_NewClass(v.rt, in_free.class_id, &def) == 0, "JS_NewClass");
    g = JS_GetGlobalObject(v.ctx);
    JS_SetPropertyStr(v.ctx, g, "probe", JS_NewCFunction(v.ctx, in_free_probe, "probe", 0));
    JS_SetPropertyStr(v.ctx, g, "release", JS_NewCFunction(v.ctx, in_free_releaser, "release", 0));
    JS_FreeValue(v.ctx, g);
    /* no collection by the threshold until the case asks for one */
    JS_SetGCThreshold(v.rt, (size_t)1 << 40);
    CHECK(eval_ok(v.ctx, IN_FREE_MAKE), "make");

    switch (way) {
    case IN_FREE_COUNT:
        a = JS_Eval(v.ctx, "make()", 6, "<unit>", JS_EVAL_TYPE_GLOBAL);
        CHECK(JS_IsObject(a), "make()");
        break;
    case IN_FREE_EACH_YOUNG:
    case IN_FREE_EACH_OLD:
        a = JS_Eval(v.ctx, "make()", 6, "<unit>", JS_EVAL_TYPE_GLOBAL);
        CHECK(JS_IsObject(a), "make()");
        for (n = 0; n < (int)countof(each); n++)
            each[n] = JS_GetPropertyUint32(v.ctx, a, n);
        JS_FreeValue(v.ctx, a);
        if (way == IN_FREE_EACH_OLD)
            JS_RunGC(v.rt);
        break;
    case IN_FREE_CYCLE:
    case IN_FREE_YOUNG:
        CHECK(eval_ok(v.ctx, "(function () { var a = make(); a.push(a) })()"), "a cycle");
        break;
    case IN_FREE_DEFERRED:
        CHECK(eval_ok(v.ctx, "var old = make()"), "old");
        JS_RunGC(v.rt);
        CHECK(eval_ok(v.ctx, "(function () { var a = { old }, b = { a }; a.b = b; old = null })()"), "a young cycle");
        break;
    case IN_FREE_WEAK:
        /* the key dies at once, and its entry waits for the next collection */
        CHECK(eval_ok(v.ctx, "var wm = new WeakMap(); (function () { wm.set({}, make()) })()"), "a weak map");
        break;
    case IN_FREE_EPHEMERON:
        CHECK(eval_ok(v.ctx, "var wm = new WeakMap(); (function () { var k = {}; k.k = k; wm.set(k, make()) })()"),
              "a weak map");
        break;
    case IN_FREE_RUNTIME:
        /* a pending exception the runtime frees first, and a cycle through
           the global object it frees in its collection */
        CHECK(eval_ok(v.ctx, "var kept = make()"), "kept");
        JS_Throw(v.ctx, JS_Eval(v.ctx, "make()", 6, "<unit>", JS_EVAL_TYPE_GLOBAL));
        break;
    }
    CHECK(in_free.finalized == 0 && in_free.released == 0, "%d finalized and %d released before the free",
          in_free.finalized, in_free.released);

    in_free.action = action;
    if (action == IN_FREE_NEW_OBJECT)
        JS_SetGCThreshold(v.rt, 1);
    full = white_gc_run_count(0);
    young = white_gc_run_count(1);
    switch (way) {
    case IN_FREE_COUNT:
        JS_FreeValue(v.ctx, a);
        break;
    case IN_FREE_EACH_YOUNG:
    case IN_FREE_EACH_OLD:
        for (i = 0; i < n; i++)
            JS_FreeValue(v.ctx, each[i]);
        break;
    case IN_FREE_CYCLE:
    case IN_FREE_WEAK:
    case IN_FREE_EPHEMERON:
        JS_RunGC(v.rt);
        full++;
        break;
    case IN_FREE_YOUNG:
    case IN_FREE_DEFERRED:
        JS_RunYoungGC(v.rt);
        young++;
        break;
    case IN_FREE_RUNTIME:
        vm_free(&v);
        break;
    }
    CHECK(in_free.made == (way == IN_FREE_RUNTIME ? 2 : 1) * IN_FREE_PROBES, "%d probes made", in_free.made);
    CHECK(in_free.finalized == in_free.made, "%d of %d probes finalized", in_free.finalized, in_free.made);
    CHECK(in_free.released == in_free.releasers && in_free.releasers == in_free.made, "%d of %d functions released",
          in_free.released, in_free.releasers);
    CHECK(in_free.runs == 0, "%lu collections ran in the calls from the host", in_free.runs);
    CHECK(white_gc_run_count(0) == full && white_gc_run_count(1) == young,
          "%ld full and %ld young collections ran besides the one the case started",
          (long)(white_gc_run_count(0) - full), (long)(white_gc_run_count(1) - young));
    in_free.action = -1;
    if (way == IN_FREE_RUNTIME) {
        host_release_call = NULL;
        return;
    }

    /* the threshold is checked again at the next object, and it is still
       past unless the collection that freed the objects set it again */
    if (action == IN_FREE_NEW_OBJECT && way != IN_FREE_CYCLE && way != IN_FREE_WEAK && way != IN_FREE_EPHEMERON) {
        CHECK(JS_GetGCThreshold(v.rt) == 1, "the threshold moved to %zu during the free", JS_GetGCThreshold(v.rt));
        full = white_gc_run_count(0);
        a = JS_NewObject(v.ctx);
        JS_FreeValue(v.ctx, a);
        CHECK(white_gc_run_count(0) == full + 1, "the next object started %lu collections", white_gc_run_count(0) - full);
    }
    /* the runtime still works, and frees with no block left */
    JS_SetGCThreshold(v.rt, (size_t)1 << 40);
    full = white_gc_run_count(0);
    young = white_gc_run_count(1);
    JS_RunGC(v.rt);
    JS_RunYoungGC(v.rt);
    CHECK(white_gc_run_count(0) == full + 1 && white_gc_run_count(1) == young + 1,
          "after the free, JS_RunGC and JS_RunYoungGC ran %lu and %lu collections", white_gc_run_count(0) - full,
          white_gc_run_count(1) - young);
    CHECK(eval_is(v.ctx, "var b = make(); b.length + ':' + typeof b[1]", "40:object"), "make after the free");
    for (i = 0; i < 2; i++)
        JS_RunGC(v.rt);
    vm_free(&v);
    host_release_call = NULL;
}

static void test_gc_in_free(void)
{
    int way, action, st;
    char way_arg[16], action_arg[16];
    char *args[5];

    for (way = 0; way < IN_FREE_WAYS; way++) {
        for (action = 0; action < IN_FREE_ACTIONS; action++) {
            /* a finalizer must not make an object as the runtime is freed
               (see docs/internals/changes/c-engine.md) */
            if (way == IN_FREE_RUNTIME && action == IN_FREE_NEW_OBJECT)
                continue;
            snprintf(way_arg, sizeof(way_arg), "%d", way);
            snprintf(action_arg, sizeof(action_arg), "%d", action);
            args[1] = "in-free";
            args[2] = way_arg;
            args[3] = action_arg;
            args[4] = NULL;
            st = run_self(args);
            CHECK(WIFEXITED(st) && WEXITSTATUS(st) == 0, "%s from a finalizer, %s: %s %d", in_free_actions[action],
                  in_free_ways[way], WIFSIGNALED(st) ? "stopped by signal" : "exit status",
                  WIFSIGNALED(st) ? WTERMSIG(st) : WEXITSTATUS(st));
        }
    }
}

/* JS_ThrowError2 makes at most two errors at once: one, and the out of
   memory error that making it can throw. Inside two it throws null,
   uncatchable, as it does when there is no memory for an error, and it does
   not abort. Inside one it makes the error as usual. Each leaves the count
   as it found it. */
static void test_error_nest(void)
{
    VM v = vm_new();
    int n;

    for (n = 0; n <= 3; n++) {
        JSValue e;
        int uncatchable;

        CHECK(white_error_nest(v.rt, n) == 0, "%d: the count of errors was not 0", n);
        JS_ThrowTypeError(v.ctx, "made inside %d", n);
        CHECK(white_error_nest(v.rt, 0) == n, "%d: the count of errors moved", n);
        uncatchable = JS_IsUncatchableException(v.ctx);
        e = JS_GetException(v.ctx);
        JS_SetUncatchableException(v.ctx, 0);
        if (n < 2) {
            JSValue m = JS_GetPropertyStr(v.ctx, e, "message");
            const char *s = JS_ToCString(v.ctx, m);
            char want[32];

            snprintf(want, sizeof want, "made inside %d", n);
            CHECK(JS_IsError(v.ctx, e) && !uncatchable, "%d: want a catchable error", n);
            CHECK(s && strcmp(s, want) == 0, "%d: message %s", n, s ? s : "(none)");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, m);
        } else {
            CHECK(JS_IsNull(e) && uncatchable, "%d: want null, uncatchable, got tag %d", n, JS_VALUE_GET_TAG(e));
        }
        JS_FreeValue(v.ctx, e);
    }
    CHECK(eval_is(v.ctx, "try { null.x } catch (e) { String(e instanceof TypeError) }", "true"), "the runtime does not throw after");
    vm_free(&v);
}

/* js_function_set_properties gives a new function its length and name. With
   no memory for the string of a name that is an index atom (which holds no
   string) or for a property, it returns -1 with the out of memory error
   pending, and the caller frees the function. It used to say nothing, and
   JS_NewCFunction2, JS_NewHostFunction and the others returned the function
   with the error still pending and its name the exception marker. Both
   paths: a function whose prototype is Function.prototype takes the shape
   the context keeps for it and gets only the string, and one with another
   prototype defines both properties. With memory, both get them. */
static void test_function_properties_oom(void)
{
    int other, full;

    for (other = 0; other < 2; other++) {
        for (full = 0; full < 2; full++) {
            VM v = vm_new();
            JSValue g = JS_GetGlobalObject(v.ctx), fp = JS_GetPropertyStr(v.ctx, g, "Function"), proto, f;
            JSAtom name = JS_NewAtomUInt32(v.ctx, 12345);
            const char *path = other ? "another prototype" : "Function.prototype";
            int r, pending, unc;

            proto = JS_GetPropertyStr(v.ctx, fp, "prototype");
            JS_FreeValue(v.ctx, fp);
            JS_FreeValue(v.ctx, g);
            if (other) {
                JSValue p2 = JS_NewObjectProto(v.ctx, proto);

                JS_FreeValue(v.ctx, proto);
                proto = p2;
            }
            f = white_bare_function(v.ctx, proto);
            CHECK(!JS_IsException(f), "%s: no function", path);
            if (full)
                fill_memory(&v);
            r = white_function_set_properties(v.ctx, f, name, 2);
            pending = JS_HasException(v.ctx);
            unc = JS_IsUncatchableException(v.ctx);
            if (full)
                release_memory(&v);
            if (full) {
                CHECK(r == -1 && pending && unc, "%s, full heap: returned %d, pending %d, uncatchable %d", path, r, pending, unc);
                free(exception_text(v.ctx));
            } else {
                JSValue n = JS_GetPropertyStr(v.ctx, f, "name"), l = JS_GetPropertyStr(v.ctx, f, "length");
                const char *s = JS_ToCString(v.ctx, n);

                CHECK(r == 0 && !pending, "%s: returned %d, pending %d", path, r, pending);
                CHECK(s && !strcmp(s, "12345") && JS_VALUE_GET_TAG(l) == JS_TAG_INT && JS_VALUE_GET_INT(l) == 2,
                      "%s: name %s, length tag %d", path, s ? s : "?", JS_VALUE_GET_TAG(l));
                JS_FreeCString(v.ctx, s);
                JS_FreeValue(v.ctx, n);
                JS_FreeValue(v.ctx, l);
            }
            JS_FreeValue(v.ctx, f);
            JS_FreeValue(v.ctx, proto);
            JS_FreeAtom(v.ctx, name);
            fflush(stdout);
            vm_free(&v);
        }
    }
}

/* ---- the polls of the bytecode ---- */

/* The opcodes of white_op, in its order. */
enum {
    W_INVALID, W_NOP, W_PUSH_0, W_DROP, W_GOTO, W_GOTO16, W_GOTO8,
    W_IF_FALSE, W_IF_TRUE, W_IF_TRUE8, W_IF_FALSE8, W_RETURN_UNDEF, W_CATCH,
    W_GOSUB, W_RET, W_WITH_GET_VAR, W_PUSH_I32, W_PUSH_I16, W_OBJECT_LIT,
    W_TAIL_CALL, W_TAIL_CALL_METHOD,
    W_GET_FIELD, W_GET_FIELD2, W_PUSH_ATOM_VALUE, W_STRICT_EQ,
    W_GET_ARG0_GET_FIELD, W_GET_ARG1_GET_FIELD, W_GET_LOC0_GET_FIELD,
    W_GET_LOC8_GET_FIELD, W_DUP_CASE_ATOM,
    W_TYPEOF, W_TYPEOF_IS_UNDEFINED, W_TYPEOF_IS_FUNCTION, W_TYPEOF_IS, W_LNOT,
    W_GET_LOC_CHECK, W_PUT_LOC_CHECK, W_SET_LOC_CHECK, W_SET_LOC_UNINITIALIZED,
    W_RETURN,
};

/* Code written for a test, one instruction at a time. A jump is written
   with no target, which set_target puts in once its place is known. */
typedef struct {
    uint8_t b[4096];
    int n;
} Code;

static void op_n(Code *c, int w, int times)
{
    while (times-- > 0)
        c->b[c->n++] = white_op(w);
}

static int jump(Code *c, int w)
{
    int at = c->n, size = 5;

    switch (w) {
    case W_GOTO16: size = 3; break;
    case W_GOTO8: case W_IF_TRUE8: case W_IF_FALSE8: size = 2; break;
    case W_WITH_GET_VAR: size = 10; break;
    }
    memset(c->b + at, 0, size);
    c->b[at] = white_op(w);
    c->n += size;
    return at;
}

static void set_target(Code *c, int at, int target)
{
    int op = c->b[at], d = target - (at + 1);
    int32_t d32;
    int16_t d16;

    if (op == white_op(W_WITH_GET_VAR)) {
        d32 = target - (at + 5);
        memcpy(c->b + at + 5, &d32, 4);
    } else if (op == white_op(W_GOTO16)) {
        d16 = d;
        memcpy(c->b + at + 1, &d16, 2);
    } else if (op == white_op(W_GOTO8) || op == white_op(W_IF_TRUE8) || op == white_op(W_IF_FALSE8)) {
        c->b[at + 1] = (uint8_t)d;
    } else {
        d32 = d;
        memcpy(c->b + at + 1, &d32, 4);
    }
}

/* scan_ok: c keeps to the rule of the polls. scan_fails: it does not, and
   what poll_scan says has want in it. */
static int scan_ok(const Code *c)
{
    const char *why = white_poll_scan(c->b, c->n);

    if (why)
        printf("poll_scan: %s\n", why);
    return why == NULL;
}

static int scan_fails(const Code *c, const char *want)
{
    const char *why = white_poll_scan(c->b, c->n);

    return why && strstr(why, want);
}

/* Code with a run of n instructions and a return. */
static Code straight(int n)
{
    Code c = {{0}, 0};

    op_n(&c, W_PUSH_0, n);
    op_n(&c, W_RETURN_UNDEF, 1);
    return c;
}

/* The count of the work of a compile stays below 0 once it is there, up to
   the next reading of the flag (js_compile_count): counts that add up past an
   int took it around to a large number, and the reading came that much
   later. */
static void test_compile_count(void)
{
    VM v = vm_new();
    int left;

    left = white_compile_count(v.ctx, 100, 60, 1);
    CHECK(left == 40, "100 less 60 is %d", left);
    left = white_compile_count(v.ctx, 100, INT_MAX, 2);
    CHECK(left < 0, "100 less twice INT_MAX is %d", left);
    left = white_compile_count(v.ctx, 100, 1 << 20, 1 << 12);
    CHECK(left < 0, "100 less 2 to the 32 is %d", left);
    vm_free(&v);
}

/* poll_scan, which check_polls runs on all code before it runs, takes a run
   of POLL_RUN instructions without a count and no more: in straight code, on
   a jump forward, which carries the run on, and into a finally block. A jump
   back, an OP_nop and the catch of an exception count. It also stops on code
   it cannot follow: an opcode the interpreter does not have, an instruction
   past the end, a jump into an instruction or out of the code, and a last
   instruction that runs off the end. */
static void test_poll_scan(void)
{
    int run = white_poll_run(), at, x;
    Code c;
    static const uint8_t past_end[] = { 0, 0, 0 };
    uint8_t bad[8];

    c = straight(run - 1);
    CHECK(scan_ok(&c), "a run of %d", run);
    c = straight(run);
    CHECK(scan_fails(&c, "without a count"), "a run of %d", run + 1);
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, 100);
    op_n(&c, W_NOP, 1);
    op_n(&c, W_PUSH_0, 100);
    op_n(&c, W_NOP, 1);
    op_n(&c, W_PUSH_0, 100);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "runs of 100 between OP_nop");

    /* a loop: the jump back counts */
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, run - 10);
    set_target(&c, jump(&c, W_GOTO), 0);
    CHECK(scan_ok(&c), "a loop");
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, run - 10);
    set_target(&c, jump(&c, W_IF_TRUE8), 0);
    op_n(&c, W_PUSH_0, 9);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_fails(&c, "without a count"), "a run on past a jump back not taken");

    /* a jump forward carries the run on past code it skips */
    for (x = 20; x <= 30; x += 10) {
        int w;
        for (w = 0; w < 2; w++) {
            c = (Code){{0}, 0};
            op_n(&c, W_PUSH_0, 100);
            at = jump(&c, w ? W_WITH_GET_VAR : W_GOTO16);
            op_n(&c, W_RETURN_UNDEF, 1);
            set_target(&c, at, c.n);
            op_n(&c, W_PUSH_0, x);
            op_n(&c, W_RETURN_UNDEF, 1);
            if (x == 20)
                CHECK(scan_ok(&c), "%d after a jump at 101", x);
            else
                CHECK(scan_fails(&c, "without a count"), "%d after a jump at 101 (%d)", x, w);
        }
    }

    /* an exception reaches the catch, which counts: a jump would not */
    for (x = 0; x < 2; x++) {
        c = (Code){{0}, 0};
        op_n(&c, W_PUSH_0, 120);
        at = jump(&c, x ? W_IF_FALSE : W_CATCH);
        op_n(&c, W_RETURN_UNDEF, 1);
        set_target(&c, at, c.n);
        op_n(&c, W_PUSH_0, 20);
        op_n(&c, W_RETURN_UNDEF, 1);
        if (x)
            CHECK(scan_fails(&c, "without a count"), "a jump carries the run");
        else
            CHECK(scan_ok(&c), "a catch counts");
    }

    /* a finally block: OP_gosub goes on with the run, OP_ret counts */
    for (x = 120; x <= 130; x += 10) {
        c = (Code){{0}, 0};
        at = jump(&c, W_GOSUB);
        op_n(&c, W_RETURN_UNDEF, 1);
        set_target(&c, at, c.n);
        op_n(&c, W_PUSH_0, x);
        op_n(&c, W_RET, 1);
        if (x == 120)
            CHECK(scan_ok(&c), "a finally block of %d", x);
        else
            CHECK(scan_fails(&c, "without a count"), "a finally block of %d", x);
    }

    /* code it cannot follow */
    c = (Code){{0}, 0};
    op_n(&c, W_INVALID, 1);
    CHECK(scan_fails(&c, "an opcode the interpreter does not have"), "OP_invalid");
    bad[0] = white_op_count();
    bad[1] = white_op(W_RETURN_UNDEF);
    CHECK(white_poll_scan(bad, 2) && strstr(white_poll_scan(bad, 2), "does not have"), "an opcode past the last");
    memcpy(bad, past_end, sizeof(past_end));
    bad[0] = white_op(W_GOTO);
    CHECK(white_poll_scan(bad, 3) && strstr(white_poll_scan(bad, 3), "past the end"), "a cut instruction");
    c = (Code){{0}, 0};
    at = jump(&c, W_GOTO8);
    x = jump(&c, W_GOTO);
    set_target(&c, at, x + 2);
    set_target(&c, x, 0);
    CHECK(scan_fails(&c, "does not land"), "a jump into an instruction");
    c = (Code){{0}, 0};
    at = jump(&c, W_IF_FALSE8);
    op_n(&c, W_RETURN_UNDEF, 1);
    set_target(&c, at, -4);
    CHECK(scan_fails(&c, "does not land"), "a jump out of the code");
    /* a target of -1, the byte before the code, as a short and a long jump
       and a catch: [OP_goto8, 0xfe] is the whole code of the first */
    for (x = 0; x < 3; x++) {
        static const int w[] = { W_GOTO8, W_GOTO, W_CATCH };
        c = (Code){{0}, 0};
        at = jump(&c, w[x]);
        if (x == 2)
            op_n(&c, W_RETURN_UNDEF, 1);
        set_target(&c, at, -1);
        CHECK(scan_fails(&c, "does not land"), "a jump to -1 (%d)", x);
    }
    c = (Code){{0}, 0};
    at = jump(&c, W_WITH_GET_VAR);
    op_n(&c, W_RETURN_UNDEF, 1);
    set_target(&c, at, c.n);
    CHECK(scan_fails(&c, "does not land"), "a with_ jump past the end");
    c = straight(3);
    c.n--;
    CHECK(scan_fails(&c, "runs off the end"), "no return at the end");
    CHECK(white_poll_scan(bad, 0) && strstr(white_poll_scan(bad, 0), "runs off the end"), "no code");
}

/* insert_c puts the polls into c (insert_polls), checks that the result
   keeps to the rule and has the shape of c, and returns it. */
static Code insert_c(JSContext *ctx, const Code *c, const char *what)
{
    static int s1[8192], s2[8192];
    Code out = {{0}, 0};
    int n1, n2;

    out.n = white_poll_insert(ctx, c->b, c->n, out.b, sizeof(out.b));
    CHECK(out.n > 0, "%s: insert_polls failed", what);
    if (out.n <= 0)
        return out;
    CHECK(scan_ok(&out), "%s: the code with the polls", what);
    n1 = white_code_shape(c->b, c->n, s1, 4096);
    n2 = white_code_shape(out.b, out.n, s2, 4096);
    CHECK(n1 > 0 && n1 == n2 && memcmp(s1, s2, sizeof(int) * 2 * n1) == 0,
          "%s: the code changed shape (%d and %d instructions)", what, n1, n2);
    return out;
}

static int count_nops(const Code *c)
{
    int places[256];

    return white_nops(c->b, c->n, places, countof(places));
}

/* insert_polls puts an OP_nop in where a path reaches POLL_RUN - 1
   instructions, unless the instruction there ends it, and moves the jumps:
   one into a run, a short one that no longer reaches, which takes its long
   form, a jump of with_, of a catch and of a finally block. */
static void test_poll_insert(void)
{
    VM v = vm_new();
    int run = white_poll_run(), at, at2, at3;
    Code c, out;

    c = straight(2 * run);
    out = insert_c(v.ctx, &c, "straight");
    CHECK(count_nops(&out) == 2, "%d OP_nop in a run of %d", count_nops(&out), 2 * run + 1);
    c = straight(run - 1);
    out = insert_c(v.ctx, &c, "a run that fits");
    CHECK(out.n == c.n, "OP_nop in a run that fits");

    /* a short jump forward over the place of an OP_nop */
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, 100);
    at = jump(&c, W_IF_FALSE8);
    op_n(&c, W_PUSH_0, 126);
    set_target(&c, at, c.n);
    op_n(&c, W_RETURN_UNDEF, 1);
    out = insert_c(v.ctx, &c, "a short jump forward");
    CHECK(count_nops(&out) == 1 && out.b[at] == white_op(W_IF_FALSE),
          "%d OP_nop and opcode %d, want 1 and if_false", count_nops(&out), out.b[at]);

    /* a short jump back, with an OP_nop before it */
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, run - 1);
    at = jump(&c, W_IF_TRUE8);
    set_target(&c, at, 0);
    op_n(&c, W_RETURN_UNDEF, 1);
    out = insert_c(v.ctx, &c, "a short jump back");
    CHECK(count_nops(&out) == 1 && out.b[at + 1] == white_op(W_IF_TRUE),
          "%d OP_nop and opcode %d, want 1 and if_true", count_nops(&out), out.b[at + 1]);

    /* a goto back that ends a path of POLL_RUN instructions needs none */
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, run - 1);
    set_target(&c, jump(&c, W_GOTO), 0);
    out = insert_c(v.ctx, &c, "a loop that fits");
    CHECK(count_nops(&out) == 0, "%d OP_nop in a loop that fits", count_nops(&out));

    /* with_, catch and a finally block, over long runs */
    c = (Code){{0}, 0};
    at = jump(&c, W_CATCH);
    at2 = jump(&c, W_WITH_GET_VAR);
    op_n(&c, W_PUSH_0, 200);
    at3 = jump(&c, W_GOSUB);
    set_target(&c, at2, c.n);
    op_n(&c, W_RETURN_UNDEF, 1);
    set_target(&c, at3, c.n);
    op_n(&c, W_PUSH_0, 200);
    op_n(&c, W_RET, 1);
    set_target(&c, at, c.n);
    op_n(&c, W_PUSH_0, 200);
    op_n(&c, W_RETURN_UNDEF, 1);
    out = insert_c(v.ctx, &c, "with_, catch and a finally block");
    CHECK(count_nops(&out) >= 4, "%d OP_nop in three runs of 200", count_nops(&out));
    vm_free(&v);
}

/* long_func: the source of a function of n lines, one assignment each, that
   returns n - 1, or with throws set throws on its last line. */
static char *long_func(int n, int throws)
{
    size_t cap = 32 * (size_t)n + 100, len;
    char *s = malloc(cap);
    int i;

    len = snprintf(s, cap, "(function () { var a = 0;\n");
    for (i = 1; i < n; i++)
        len += snprintf(s + len, cap - len, "a = %d;\n", i);
    snprintf(s + len, cap - len, throws ? "throw new Error('end') })" : "return a })");
    return s;
}

/* code_of: the code of the function of src, compiled only, in *top (the
   script), *code and *len. */
static int code_of(JSContext *ctx, const char *src, JSValue *top, const uint8_t **code, int *len)
{
    *top = JS_Eval(ctx, src, strlen(src), "<poll>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(*top))
        return -1;
    *code = white_child_code(*top, 0, len);
    return *code ? 0 : -1;
}

/* run_read: read buf back with JS_ReadObject, run the script and call the
   function it gives. "ok" with the number it returned, or the exception. */
/* read_extra_flags: flags added to the ones run_read reads with. */
static int read_extra_flags;

static void run_read(JSContext *ctx, const uint8_t *buf, size_t size, char *out, size_t cap)
{
    JSValue top, fun, r;
    char *t;
    int32_t n = -1;

    top = JS_ReadObject(ctx, buf, size, JS_READ_OBJ_BYTECODE | read_extra_flags);
    if (JS_IsException(top))
        goto exception;
    fun = JS_EvalFunction(ctx, top);
    if (JS_IsException(fun))
        goto exception;
    r = JS_Call(ctx, fun, JS_UNDEFINED, 0, NULL);
    JS_FreeValue(ctx, fun);
    if (JS_IsException(r))
        goto exception;
    JS_ToInt32(ctx, &n, r);
    JS_FreeValue(ctx, r);
    snprintf(out, cap, "ok %d", (int)n);
    return;
 exception:
    t = exception_text(ctx);
    snprintf(out, cap, "%s", t ? t : "?");
    free(t);
}

/* The compiler puts the polls into a long function, which runs and keeps
   its line numbers. Without them (white_poll_skip) check_polls stops the
   compiler, and JS_ReadObject reads the function back, but not with its
   OP_nop taken out or with an opcode the interpreter does not have. */
static void test_poll_compile(void)
{
    VM v = vm_new();
    char *src = long_func(400, 0), *src2 = long_func(400, 1), *t, got[256];
    const char *stack;
    JSValue top, fun, r;
    const uint8_t *code;
    uint8_t *buf, *copy;
    size_t size;
    int len, places[64], n, i;
    const uint8_t *at;

    CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile a long function");
    CHECK(white_poll_scan(code, len) == NULL, "the code of a long function");
    n = white_nops(code, len, places, countof(places));
    CHECK(n >= 4, "%d OP_nop in a long function", n);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    CHECK(buf != NULL, "write the long function");
    at = buf ? memmem(buf, size, code, len) : NULL;
    CHECK(at != NULL, "the code of the long function in what JS_WriteObject wrote");
    run_read(v.ctx, buf, size, got, sizeof(got));
    CHECK(strcmp(got, "ok 399") == 0, "read back: %s", got);
    if (at) {
        copy = malloc(size);
        memcpy(copy, buf, size);
        for (i = 0; i < n && i < (int)countof(places); i++)
            copy[at - buf + places[i]] = white_op(W_PUSH_0);
        run_read(v.ctx, copy, size, got, sizeof(got));
        CHECK(strstr(got, "SyntaxError: invalid bytecode: a run of more instructions than POLL_RUN"), "read without the polls: %s", got);
        memcpy(copy, buf, size);
        copy[at - buf] = white_op(W_INVALID);
        run_read(v.ctx, copy, size, got, sizeof(got));
        CHECK(strstr(got, "SyntaxError: invalid bytecode: an opcode the interpreter does not have"), "read with OP_invalid: %s", got);
        free(copy);
    }
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);

    /* the line of an error after the OP_nop: the throw is on line 401 */
    fun = JS_Eval(v.ctx, src2, strlen(src2), "<poll>", JS_EVAL_TYPE_GLOBAL);
    r = JS_Call(v.ctx, fun, JS_UNDEFINED, 0, NULL);
    CHECK(JS_IsException(r), "the long function that throws");
    JS_FreeValue(v.ctx, r);
    JS_FreeValue(v.ctx, fun);
    r = JS_GetException(v.ctx);
    fun = JS_GetPropertyStr(v.ctx, r, "stack");
    stack = JS_ToCString(v.ctx, fun);
    CHECK(stack && strstr(stack, "<poll>:401"), "the stack of the long function: %s", stack ? stack : "(none)");
    JS_FreeCString(v.ctx, stack);
    JS_FreeValue(v.ctx, fun);
    JS_FreeValue(v.ctx, r);

    white_poll_skip(1);
    top = JS_Eval(v.ctx, src, strlen(src), "<poll>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    white_poll_skip(0);
    CHECK(JS_IsException(top), "a long function compiled without the polls");
    t = exception_text(v.ctx);
    CHECK(t && strstr(t, "InternalError: compiled bytecode: a run of more instructions than POLL_RUN"), "without the polls: %s", t ? t : "(none)");
    free(t);
    JS_FreeValue(v.ctx, top);
    free(src);
    free(src2);
    vm_free(&v);
}

/* push_i32: an OP_push_i32 of v at the end of c. */
static void push_i32(Code *c, int32_t v)
{
    op_n(c, W_PUSH_I32, 1);
    memcpy(c->b + c->n, &v, 4);
    c->n += 4;
}

/* read_code: c in place of the code of a function, written with
   JS_WriteObject, read back and run (run_read). The function calls its
   argument with 300 numbers, which gives c a stack and a length of 1505
   bytes, and holds no atom, so that its code is in what JS_WriteObject
   writes as it is in memory. The code past c is OP_return_undef. */
static void read_code(JSContext *ctx, const Code *c, char *got, size_t cap)
{
    size_t scap = 16 * 300 + 100, n, size;
    char *src = malloc(scap);
    JSValue top;
    const uint8_t *code, *at;
    uint8_t *buf;
    int len, i;

    n = snprintf(src, scap, "(function (g) { return g(");
    for (i = 0; i < 300; i++)
        n += snprintf(src + n, scap - n, "%s%d", i ? ", " : "", 100000 + i);
    snprintf(src + n, scap - n, ") })");
    snprintf(got, cap, "no code");
    if (code_of(ctx, src, &top, &code, &len) == 0) {
        buf = JS_WriteObject(ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
        at = buf ? memmem(buf, size, code, len) : NULL;
        if (at && c->n <= len) {
            memcpy(buf + (at - buf), c->b, c->n);
            memset(buf + (at - buf) + c->n, white_op(W_RETURN_UNDEF), len - c->n);
            run_read(ctx, buf, size, got, cap);
        }
        js_free(ctx, buf);
    }
    JS_FreeValue(ctx, top);
    free(src);
}

/* OP_ret goes only where an OP_gosub returns to: code read with
   JS_ReadObject gives it any number, which must not take the interpreter
   into the middle of an instruction or to one that does not follow an
   OP_gosub, where it would run code check_polls did not follow. The two
   cases: OP_ret to 7, inside an OP_push_i32 whose operand holds an
   OP_return_undef, and a path of 255 instructions without a count, which
   runs inside the operands of two runs of 127 that check_polls takes, one
   ending with an OP_nop. And OP_ret to an instruction that follows no
   OP_gosub. A finally block read back runs. */
static void test_poll_ret(void)
{
    VM v = vm_new();
    static Code c;
    char got[256];
    const char *src = "(function () { var t = 0; for (var i = 0; i < 3; i++) { try { t += i } finally { t += 10 } } return t })";
    JSValue top;
    uint8_t *buf;
    size_t size;
    int i;

    /* OP_ret to 7 */
    c = (Code){{0}, 0};
    push_i32(&c, 7);
    op_n(&c, W_RET, 1);
    push_i32(&c, 0);
    c.b[7] = white_op(W_RETURN_UNDEF);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "OP_ret to 7: check_polls takes the code");
    read_code(v.ctx, &c, got, sizeof(got));
    CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret into an instruction: %s", got);

    /* 255 instructions without a count */
    c = (Code){{0}, 0};
    push_i32(&c, 7);
    op_n(&c, W_RET, 1);
    for (i = 0; i < 127; i++)
        push_i32(&c, 0);
    op_n(&c, W_NOP, 1);
    for (i = 0; i < 127; i++)
        push_i32(&c, 0);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(c.n == 1278, "the code of 255 instructions is %d bytes", c.n);
    for (i = 0; i < 126; i++) {
        c.b[6 + 5 * i + 1] = white_op(W_PUSH_I32);
        c.b[642 + 5 * i + 1] = white_op(W_PUSH_I32);
    }
    c.b[637] = white_op(W_PUSH_I16);
    c.b[640] = white_op(W_PUSH_I16);
    c.b[1273] = white_op(W_RETURN_UNDEF);
    CHECK(scan_ok(&c), "255 instructions: check_polls takes the code");
    read_code(v.ctx, &c, got, sizeof(got));
    CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret to a path of 255: %s", got);

    /* OP_ret to the start of an instruction that follows no OP_gosub */
    c = (Code){{0}, 0};
    push_i32(&c, 6);
    op_n(&c, W_RET, 1);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "OP_ret to 6: check_polls takes the code");
    read_code(v.ctx, &c, got, sizeof(got));
    CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret to no OP_gosub: %s", got);

    /* a finally block, compiled and read back */
    top = JS_Eval(v.ctx, src, strlen(src), "<poll>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    CHECK(buf != NULL, "write a finally block");
    if (buf)
        run_read(v.ctx, buf, size, got, sizeof(got));
    CHECK(buf && strcmp(got, "ok 33") == 0, "a finally block read back: %s", buf ? got : "(none)");
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);
    vm_free(&v);
}

/* A read with JS_READ_OBJ_POLLS_CHECKED does not run poll_scan again: the
   bytes were read once without the flag, which holds every function to the
   rule of the polls. So code that breaks the rule is read, where without the
   flag it is a SyntaxError (nothing runs it here). The rest of what
   check_polls makes stays: the places OP_ret may go, so OP_ret to a place
   that follows no OP_gosub is still an InternalError, and a finally block
   still runs. */
#define POLLS_CHECKED (1 << 5) /* JS_READ_OBJ_POLLS_CHECKED */

static void test_poll_checked_read(void)
{
    VM v = vm_new();
    char *src = long_func(400, 0), got[256];
    const char *fin;
    const uint8_t *code;
    uint8_t *buf, *copy;
    JSValue top, r;
    size_t size;
    int len, places[64], n, i;
    const uint8_t *at;

    CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile a long function");
    n = white_nops(code, len, places, countof(places));
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    at = buf ? memmem(buf, size, code, len) : NULL;
    CHECK(at != NULL, "the code of the long function in what JS_WriteObject wrote");
    if (at) {
        copy = malloc(size);
        memcpy(copy, buf, size);
        for (i = 0; i < n && i < (int)countof(places); i++)
            copy[at - buf + places[i]] = white_op(W_PUSH_0);
        r = JS_ReadObject(v.ctx, copy, size, JS_READ_OBJ_BYTECODE);
        CHECK(JS_IsException(r), "code without its polls is read without the flag");
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, JS_GetException(v.ctx));
        r = JS_ReadObject(v.ctx, copy, size, JS_READ_OBJ_BYTECODE | POLLS_CHECKED);
        CHECK(!JS_IsException(r), "code without its polls is read with the flag");
        JS_FreeValue(v.ctx, r);
        free(copy);
    }
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);
    free(src);

    read_extra_flags = POLLS_CHECKED;
    {
        Code c = {{0}, 0};

        push_i32(&c, 7);
        op_n(&c, W_RET, 1);
        push_i32(&c, 0);
        c.b[7] = white_op(W_RETURN_UNDEF);
        op_n(&c, W_RETURN_UNDEF, 1);
        read_code(v.ctx, &c, got, sizeof(got));
        CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret into an instruction with the flag: %s", got);
        c = (Code){{0}, 0};
        push_i32(&c, 6);
        op_n(&c, W_RET, 1);
        op_n(&c, W_RETURN_UNDEF, 1);
        read_code(v.ctx, &c, got, sizeof(got));
        CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret to no OP_gosub with the flag: %s", got);
    }
    fin = "(function () { var t = 0; for (var i = 0; i < 3; i++) { try { t += i } finally { t += 10 } } return t })";
    top = JS_Eval(v.ctx, fin, strlen(fin), "<poll>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    if (buf)
        run_read(v.ctx, buf, size, got, sizeof(got));
    CHECK(buf && strcmp(got, "ok 33") == 0, "a finally block read back with the flag: %s", buf ? got : "(none)");
    read_extra_flags = 0;
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);
    vm_free(&v);
}

/* An object literal of plain keys is one OP_object_lit, whose operands are
   the number of its values and the index of its keys and shape in the
   function. Its bytecode reads back and runs, but not with either operand
   changed so that they no longer match the keys the function holds
   (SyntaxError: invalid object literal). The function holds no atom in its
   code, so that the code is in what JS_WriteObject writes as it is in
   memory. */
/* A call of a C function in the place of a return compiles to a tail call,
   which the interpreter makes without JS_CallInternal too (the recursions of
   TestCCallDepth that end with these return are made of them). */
static void test_tail_c_calls(void)
{
    static const struct { const char *src; int op; } cases[] = {
        { "function f() { return [1].map(f) }", W_TAIL_CALL_METHOD },
        { "const apply = Reflect.apply; function f() { return apply(f, null, []) }", W_TAIL_CALL },
        { "const abs = Math.abs; function f(x) { return abs(x) }", W_TAIL_CALL },
    };
    VM v = vm_new();
    int shape[2 * 64];

    for (size_t i = 0; i < countof(cases); i++) {
        const uint8_t *code;
        JSValue top;
        int len, n, found = 0;

        CHECK(code_of(v.ctx, cases[i].src, &top, &code, &len) == 0, "compile %s", cases[i].src);
        n = code ? white_code_shape(code, len, shape, 64) : -1;
        for (int k = 0; k < n; k++)
            found |= shape[2 * k] == white_op(cases[i].op);
        CHECK(found, "%s: no tail call in %d instructions", cases[i].src, n);
        JS_FreeValue(v.ctx, top);
    }
    vm_free(&v);
}


/* The compiler joins each pair of fused_runs into one opcode, unless a jump
   lands inside the pair or JS_SetFusedOps turned it off. poll_scan holds
   code read by JS_ReadObject to the rest of each run, and code written with
   the opcodes reads back and runs. */
static void test_fused_ops(void)
{
    static const char five[] =
        "(function () { function f(o, p) { var a = o.k1, b = p.k2, c = 1, d = 2, e = p;\n"
        "  var s = a.k4 + e.k3 + (b, c, d, ''); switch (s) { case 'aa': return 1; case 'bb': return 2 } return 3 }\n"
        "  return f({ k1: { k4: 'a' } }, { k2: 0, k3: 'a' }) + 10 * f({ k1: { k4: 'b' } }, { k3: 'b' }) +\n"
        "    100 * f({ k1: {} }, { k3: 'x' }) })";
    static const char *names[8] = { "get_arg0 get_field", "get_arg1 get_field", "get_loc0 get_field",
                                    "get_loc8 get_field", "dup push_atom_value strict_eq if_false8",
                                    "dup push_atom_value strict_eq if_true8",
                                    "dup push_atom_value strict_eq if_false",
                                    "dup push_atom_value strict_eq if_true" };
    /* the four forms of a case: a body each (if_false8), cases that share a
       body (if_true8), a body too long for a jump of one byte after its case
       (if_false), and the first of 16 cases that share a body, which jumps
       over the 15 others (if_true) */
#define W_CASES16 "case 'd': case 'd2': case 'd3': case 'd4': case 'd5': case 'd6': case 'd7': case 'd8':\n" \
        "  case 'd9': case 'd10': case 'd11': case 'd12': case 'd13': case 'd14': case 'd15': case 'e':\n"
#define W_LONG_BODY "    n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1;\n" \
        "    n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1;\n" \
        "    n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1;\n" \
        "    n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1; n = n * 3 + 1;\n" \
        "    n = +n; n = +n; n = +n; n = +n; n = +n; n = +n; n = +n; n = +n; n = +n; n = +n; n = +n; n = +n;\n"
    static const char cases[] =
        "function f(s) { var n = 0; switch (s) { case 'a': return 1; case 'b': case 'c': return 2;\n"
        W_CASES16 W_LONG_BODY "    return n % 7; case 'f': return 3 } return 4 }";
    VM v = vm_new();
    JSValue top;
    const uint8_t *code;
    uint8_t *buf, *at;
    size_t size;
    int len, count[8], n, on, k;
    char got[256];
    Code c;

    /* the inner function f, compiled alone, has each of the five; with the
       pairs off, none. (b, c, d, '') reads b, c and d after e is written,
       so that e keeps a slot of its own, past get_loc3 */
    for (on = 1; on >= 0; on--) {
        static const char inner[] =
            "function f(o, p) { var a = o.k1, b = p.k2, c = 1, d = 2, e = p;\n"
            "  var s = a.k4 + e.k3 + (b, c, d, ''); switch (s) { case 'aa': return 1; case 'bb': return 2 } return 3 }";
        /* a case for each of the two strings */
        JS_SetFusedOps(v.ctx, on);
        CHECK(code_of(v.ctx, inner, &top, &code, &len) == 0, "compile f");
        n = code ? white_fused_ops(code, len, count, countof(count)) : -1;
        for (k = 0; k < 8; k++)
            CHECK(count[k] == (on ? (k == 4 ? 2 : k < 4) : 0), "on %d: %s: %d", on, names[k], count[k]);
        JS_FreeValue(v.ctx, top);
        CHECK(code_of(v.ctx, cases, &top, &code, &len) == 0, "compile the cases");
        n = code ? white_fused_ops(code, len, count, countof(count)) : -1;
        CHECK(n == (on ? 20 : 0), "cases, on %d: %d fused opcodes, want 20", on, n);
        for (k = 0; k < 8; k++)
            CHECK(on ? (k < 4 ? count[k] == 0 : count[k] > 0) : count[k] == 0,
                  "cases, on %d: %s: %d", on, names[k], count[k]);
        JS_FreeValue(v.ctx, top);
    }
    CHECK(eval_is(v.ctx, "(function () { var r = ''; "
                  "for (var s of ['a', 'b', 'c', 'd', 'd9', 'e', 'f', 'g', 'a' + 'b'.repeat(0), 'd1', 1]) {\n"
                  "  r += (function f(s) { var n = 0; switch (s) { case 'a': return 1; case 'b': case 'c': return 2;\n"
                  W_CASES16 W_LONG_BODY "    return n % 11; case 'f': return 3 } return 4 })(s) } return r })()",
                  "12277734144"),
          "the four forms of a case run");
#undef W_CASES16
#undef W_LONG_BODY
    JS_SetFusedOps(v.ctx, 1);

    /* a jump to the get_field of a pair: the pair stays apart */
    CHECK(code_of(v.ctx, "function f(o, c) { return (c ? o : o).x }", &top, &code, &len) == 0, "compile");
    n = code ? white_fused_ops(code, len, count, countof(count)) : -1;
    CHECK(n == 0, "a jump into the pair: %d fused opcodes", n);
    JS_FreeValue(v.ctx, top);
    CHECK(code_of(v.ctx, "function f(o, c) { return o.x }", &top, &code, &len) == 0, "compile");
    n = code ? white_fused_ops(code, len, count, countof(count)) : -1;
    CHECK(n == 1 && count[0] == 1, "no jump: %d fused opcodes", n);
    JS_FreeValue(v.ctx, top);
    CHECK(eval_is(v.ctx, "(function f(o, c) { return (c ? o : o).x })({ x: 'jumped' }, 1)", "jumped"), "a jump into the pair runs");

    /* written and read back, it runs, and not with the rest of a run changed */
    top = JS_Eval(v.ctx, five, strlen(five), "<fused>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    JS_FreeValue(v.ctx, top);
    CHECK(buf != NULL, "write");
    if (buf) {
        run_read(v.ctx, buf, size, got, sizeof(got));
        CHECK(strcmp(got, "ok 321") == 0, "read back: %s", got);
        for (at = buf; at + 1 < buf + size; at++) {
            if (at[0] == white_op(W_GET_ARG1_GET_FIELD) && at[1] == white_op(W_GET_FIELD))
                break;
        }
        CHECK(at + 1 < buf + size, "no get_arg1 get_field in the bytecode written");
        if (at + 1 < buf + size) {
            at[1] = white_op(W_GET_FIELD2);
            run_read(v.ctx, buf, size, got, sizeof(got));
            CHECK(strstr(got, "invalid bytecode: a fused opcode without the instructions it reads") != NULL,
                  "read with the get_field changed: %s", got);
        }
        js_free(v.ctx, buf);
    }

    /* poll_scan: each fused opcode followed by the rest of its run */
    c = (Code){{0}, 0};
    op_n(&c, W_GET_ARG0_GET_FIELD, 1);
    jump(&c, W_GOTO); /* 5 bytes of nothing, to be the get_field */
    c.b[c.n - 5] = white_op(W_GET_FIELD);
    op_n(&c, W_DROP, 1);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "get_arg0 get_field");
    c.b[1] = white_op(W_GET_FIELD2);
    CHECK(scan_fails(&c, "a fused opcode without"), "get_arg0 then get_field2");
    c.n = 1;
    CHECK(scan_fails(&c, "a fused opcode without"), "get_arg0 get_field at the end");
    c = (Code){{0}, 0};
    op_n(&c, W_GET_LOC8_GET_FIELD, 2); /* the index is a second byte of the opcode */
    c.b[1] = 0;
    jump(&c, W_GOTO);
    c.b[c.n - 5] = white_op(W_GET_FIELD);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "get_loc8 get_field");
    c.b[2] = white_op(W_DROP);
    CHECK(scan_fails(&c, "a fused opcode without"), "get_loc8 then something else");
    for (int w = 0; w < 4; w++) {
        static const int jumps[4] = { W_IF_FALSE8, W_IF_TRUE8, W_IF_FALSE, W_IF_TRUE };
        for (k = 0; k < 4; k++) {
            int iff;
            c = (Code){{0}, 0};
            op_n(&c, W_PUSH_0, 1);
            op_n(&c, W_DUP_CASE_ATOM, 1);
            jump(&c, W_GOTO);
            c.b[c.n - 5] = white_op(W_PUSH_ATOM_VALUE);
            op_n(&c, W_STRICT_EQ, 1);
            iff = jump(&c, jumps[w]);
            op_n(&c, W_DROP, 1);
            set_target(&c, iff, c.n);
            op_n(&c, W_RETURN_UNDEF, 1);
            if (k == 0) {
                CHECK(scan_ok(&c), "dup push_atom_value strict_eq %s", names[4 + w] + 30);
            } else {
                /* the push_atom_value, the strict_eq or the jump changed */
                c.b[k == 1 ? 2 : k == 2 ? 7 : 8] = white_op(k == 1 ? W_GET_FIELD : k == 2 ? W_DROP :
                                                             w < 2 ? W_GOTO8 : W_GOTO);
                CHECK(scan_fails(&c, "a fused opcode without"), "the case %d with instruction %d changed", w, k + 1);
            }
        }
    }
    vm_free(&v);
}
/* typeof x compared with a string is one opcode: typeof_is with the string,
   typeof_is_undefined and typeof_is_function for those two, a negated test
   takes the other branch or lnot, and typeof x < "u" or > "u" is
   typeof_is_undefined. With JS_SetFusedOps off, only the forms of before
   typeof_is are joined. Written and read back, the string comes along. */
static void test_typeof_is(void)
{
    static const struct {
        const char *body;
        int op, lnot; /* with the opcodes on */
        int off_op;   /* off: the opcode, or W_TYPEOF when none */
    } cases[] = {
        { "return typeof x == 'string'", W_TYPEOF_IS, 0, W_TYPEOF },
        { "return typeof x === `number`", W_TYPEOF_IS, 0, W_TYPEOF },
        { "return typeof x !== 'object'", W_TYPEOF_IS, 1, W_TYPEOF },
        { "if (typeof x != 'symbol') return 1", W_TYPEOF_IS, 0, W_TYPEOF },
        { "if (typeof x == 'bigint') return 1", W_TYPEOF_IS, 0, W_TYPEOF },
        { "return typeof x < 'u'", W_TYPEOF_IS_UNDEFINED, 1, W_TYPEOF },
        { "return typeof x > `u`", W_TYPEOF_IS_UNDEFINED, 0, W_TYPEOF },
        { "if (typeof x < 'u') return 1", W_TYPEOF_IS_UNDEFINED, 0, W_TYPEOF },
        { "return typeof x === 'undefined'", W_TYPEOF_IS_UNDEFINED, 0, W_TYPEOF_IS_UNDEFINED },
        { "return typeof x != 'function'", W_TYPEOF_IS_FUNCTION, 1, W_TYPEOF },
        { "if (typeof x !== 'function') return 1", W_TYPEOF_IS_FUNCTION, 0, W_TYPEOF_IS_FUNCTION },
        { "return typeof x <= 'u'", W_TYPEOF, 0, W_TYPEOF },
        { "return typeof x < 'v'", W_TYPEOF, 0, W_TYPEOF },
        { "return typeof x == 1", W_TYPEOF, 0, W_TYPEOF },
    };
    static const char script[] =
        "(function () { function f(x) { return typeof x == 'string' ? 1 : 0 }\n"
        "  function g(x) { return typeof x !== 'object' && typeof x < 'u' ? 1 : 0 }\n"
        "  return f('a') * 100 + f(1) * 10 + g(1) + g(null) * 2 + g(undefined) * 4 })";
    VM v = vm_new();
    int shape[2 * 64];
    char src[256], got[256];
    uint8_t *buf;
    size_t size;

    for (int on = 1; on >= 0; on--) {
        JS_SetFusedOps(v.ctx, on);
        for (size_t i = 0; i < countof(cases); i++) {
            const uint8_t *code;
            JSValue top;
            int len, n, ops = 0, lnot = 0, left = 0;
            int want = on ? cases[i].op : cases[i].off_op;

            snprintf(src, sizeof(src), "function f(x) { %s }", cases[i].body);
            CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile %s", src);
            n = code ? white_code_shape(code, len, shape, 64) : -1;
            for (int k = 0; k < n; k++) {
                ops += shape[2 * k] == white_op(want);
                lnot += shape[2 * k] == white_op(W_LNOT);
                left += shape[2 * k] == white_op(W_TYPEOF);
            }
            CHECK(ops == 1, "on %d: %s: %d of the opcode", on, cases[i].body, ops);
            CHECK(want == W_TYPEOF || left == 0, "on %d: %s: typeof left", on, cases[i].body);
            CHECK(lnot == (on && cases[i].lnot), "on %d: %s: %d lnot", on, cases[i].body, lnot);
            JS_FreeValue(v.ctx, top);
        }
    }
    JS_SetFusedOps(v.ctx, 1);

    {
        JSValue top = JS_Eval(v.ctx, script, strlen(script), "<typeof>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
        buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
        JS_FreeValue(v.ctx, top);
    }
    CHECK(buf != NULL, "write");
    if (buf) {
        run_read(v.ctx, buf, size, got, sizeof(got));
        CHECK(strcmp(got, "ok 101") == 0, "read back: %s", got);
        js_free(v.ctx, buf);
    }
    vm_free(&v);
}

/* An exception thrown out of the body of a for...of loop, or out of a
   default of an array pattern, closes the iterator: its return method runs
   with the exception pending, then the catch takes the exception. Running
   out of memory in the return method is uncatchable: the catch of the same
   function does not take it. Each call of the allocator fails in turn: the
   run gives the closes and the exceptions in order, or fails with the out of
   memory error. */
static void test_iterator_close_oom(void)
{
    static const char setup[] =
        "globalThis.run = function () {\n"
        "  var out = [];\n"
        "  var it = { [Symbol.iterator]() { return it }, next() { return { value: undefined } },\n"
        "    return() { out.push('r' + out.length); return {} } };\n"
        "  try { for (var v of it) throw 't' } catch (e) { out.push(e) }\n"
        "  try { var [a = (() => { throw 'd' })()] = it } catch (e) { out.push(e) }\n"
        "  function* gen() { try { for (var v of it) throw 'g' } catch (e) { out.push(e) } }\n"
        "  gen().next();\n"
        "  try { try { for (var v of it) throw 'u' } finally { out.push('f') } } catch (e) { out.push(e) }\n"
        "  (async function () { try { for (var v of it) throw 'a' } catch (e) { out.push(e) } })();\n"
        "  return out.join(' ');\n"
        "}; 0";
    VM v = vm_new();
    int fail, n = 0, ooms = 0;
    JSValue f, g;

    CHECK(eval_ok(v.ctx, setup), "the setup threw");
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "run");
    JS_FreeValue(v.ctx, g);
    for (fail = 0; fail == 0 || fail <= n; fail++) {
        int made;
        JSValue r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);

        if (fail == 0)
            n = made;
        if (JS_IsException(r)) {
            CHECK(fail > 0 && JS_IsUncatchableException(v.ctx), "call %d failing: an exception that is not out of memory", fail);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            ooms++;
        } else {
            char *t = value_text(v.ctx, r);

            CHECK(t && strcmp(t, "r0 t r2 d r4 g r6 f u r9 a") == 0, "call %d failing: %s", fail, t ? t : "(no text)");
            free(t);
        }
        JS_FreeValue(v.ctx, r);
    }
    CHECK(n > 4 && ooms > 4, "only %d of %d calls failed the run", ooms, n);
    JS_FreeValue(v.ctx, f);
    vm_free(&v);
}

/* The captures that never change (capture_by_value): for each captured
   argument or local of the function f, whether no closure can see it
   change. A write that a path from the place a closure is made reaches
   keeps a JSVarRef: in either arm of an if, in a switch, around a loop,
   through a catch or a finally, after a yield or an await, in a function
   below that captures it, by a direct eval or through a mapped arguments
   object. A close_loc on the way (the per-round binding of a for-let) ends
   the capture. The verdict is the same on code read back with
   JS_ReadObject, which works it out again. */
static void test_capture_by_value(void)
{
    static const struct {
        const char *src, *want;
    } cases[] = {
        /* destructuring: the initializer runs after the pattern's code */
        { "function f(g) { const [a, b] = g(() => a); return () => b }", "a=0 b=1" },
        { "function f(g) { const { a, b } = g(); return () => a + b }", "a=1 b=1" },
        /* the arms of an if */
        { "function f(c) { let x; const h = () => x; if (c) x = 1; else x = 2; return h }", "x=0" },
        { "function f(c) { let x; if (c) x = 1; else x = 2; return () => x }", "x=1" },
        { "function f(c) { let x = 0; if (c) return () => x; x = 1; return null }", "x=1" },
        /* switch */
        { "function f(v) { let x = 0; switch (v) { case 0: x = 1; break; case 1: return () => x } return null }", "x=1" },
        { "function f(v) { let x = 0, g; switch (v) { case 0: g = () => x; case 1: x = 2 } return g }", "x=0" },
        /* loops, break and continue to labels */
        { "function f() { let x = 0; const fs = []; while (x < 3) { fs.push(() => x); x++ } return fs }", "x=0" },
        { "function f() { var fs = []; for (var i = 0; i < 3; i++) fs.push(() => i); return fs }", "i=0" },
        { "function f(a) { const r = []; outer: for (let i = 0; i < 3; i++) { let y = i * 2; for (;;) { r.push(() => y); if (a) continue outer; break outer } } return r }", "y=1" },
        { "function f(a) { const r = []; let y = 0; outer: for (let i = 0; i < 3; i++) { for (;;) { r.push(() => y); if (a) continue outer; break outer } } y = 1; return r }", "y=0" },
        { "function f() { let x = 0, g; lab: { g = () => x; if (g.length) break lab; x = 1 } return g }", "x=0" },
        { "function f() { let x = 0, g; lab: { x = 1; break lab } g = () => x; return g }", "x=1" },
        /* try, catch and finally */
        { "function f(h) { let x = 0; const g = () => x; try { h() } catch (e) { x = 1 } return g }", "x=0" },
        /* a write in a catch counts wherever the closure is made, after the
           try too: an exception can only go to some catch of the function,
           but which one the stack holds is not looked at */
        { "function f(h) { let x = 0; try { h() } catch (e) { x = 1 } return () => x }", "x=0" },
        { "function f() { let x = 0, g; try { g = () => x } finally { x = 2 } return g }", "x=0" },
        { "function f(h) { let x = 0, g; try { x = 1 } finally { g = () => x } return g }", "x=1" },
        { "function f(h) { let x = 0, g; try { try { g = () => x } finally { h() } } catch (e) { x = 3 } return g }", "x=0" },
        /* a write that only the handler reaches */
        { "function f(h) { let x = 0, g; try { g = () => x; h() } catch (e) { x = 1 } return g }", "x=0" },
        { "function f(h) { let x = 0, g; try { h() } catch (e) { g = () => x; return g } x = 1; return g }", "x=1" },
        /* a capture in a finally and a write after it */
        { "function f() { let x = 0, g; try {} finally { g = () => x } x = 1; return g }", "x=0" },
        /* the binding of each round of a for-let is closed before the next */
        { "function f() { const fs = []; for (let i = 0; i < 3; i++) fs.push(() => i); return fs }", "i=1" },
        { "function f() { const fs = []; for (let i = 0; i < 3; i++) { fs.push(() => i); i += 0 } return fs }", "i=0" },
        { "function f(a) { const fs = []; for (const x of a) fs.push(() => x); return fs }", "x=1" },
        /* default parameters: in QuickJS the body writes a binding of its own
           when the parameters have expressions, which the closure never sees
           (it gives 1 where Node gives 2) */
        { "function f(a, g = () => a) { a = 2; return g }", "a=1" },
        { "function f(a, g = () => a, h = () => { a = 3 }) { return g }", "a=0" },
        { "function f(a, g = () => a) { return g }", "a=1" },
        /* the name of a function expression, a catch parameter */
        { "var h = function f() { return () => f }", "f=1" },
        /* the catch writes e as it starts: a closure of e keeps a JSVarRef
           (above) */
        { "function f(h) { try { h() } catch (e) { return () => e } }", "e=0" },
        { "function f(h) { try { h() } catch (e) { const g = () => e; e = 1; return g } }", "e=0" },
        /* this and new.target in an arrow */
        { "function f() { return () => this }", "this=1" },
        /* function declarations that call each other */
        { "function f() { function a() { return b } function b() { return a } return a }", "a=1 b=0" },
        /* class fields and static blocks */
        { "function f() { let x = 1; class A { y = x; static { x = 2 } } return A }", "x=0 <class_fields_init>=0" },
        { "function f() { const x = 1; class A { y = () => x; static z = x } return A }", "x=1 <class_fields_init>=0" },
        /* a generator and an async function go on after a yield and an await */
        { "function* f() { let x = 1; const g = () => x; yield 0; x = 2; yield g }", "x=0" },
        { "function* f() { let x = 1; yield 0; x = 2; yield () => x }", "x=1" },
        { "async function f(h) { let x = 1; const g = () => x; await 0; x = 2; return g }", "x=0" },
        { "async function f(h) { let x = await h(); return () => x }", "x=1" },
        /* with: an assignment inside it takes a reference to the slot
           (make_loc_ref), which keeps it a JSVarRef */
        { "function f(o) { let x = 1; with (o) { x = 2 } return () => x }", "x=0" },
        { "function f(o) { let x = 1; const g = () => x; with (o) { x = 2 } return g }", "x=0" },
        { "function f(o) { let x = 1; with (o) { return () => x } }", "x=1 <with>=1" },
        /* eval: a direct one can write any of them, an indirect one none */
        { "function f() { let x = 1; const g = () => x; eval('x = 2'); return g }", "x=0 g=0 <var>=0 this=0 new.target=0 arguments=0" },
        { "function f() { let x = 1; const g = () => x; (0, eval)('x = 2'); return g }", "x=1" },
        { "function f() { let x = 1; const g = () => () => eval(''); return g }", "x=0 g=0 this=0 new.target=0 arguments=0" },
        /* arguments: mapped in sloppy code with simple parameters */
        { "function f(a) { const g = () => a; arguments[0] = 2; return g }", "a=0" },
        { "function f(a) { 'use strict'; const g = () => a; arguments[0] = 2; return g }", "a=1" },
        { "function f(a, b = 1) { const g = () => a; arguments[0] = 2; return g }", "a=1" },
        /* a function two levels down */
        { "function f() { let x = 1; return () => () => { x = 2 } }", "x=0" },
        { "function f() { let x = 1; return () => () => x }", "x=1" },
        { "function f() { let x = 1; return () => () => () => x++ }", "x=0" },
        /* the dead zone: a capture before the declaration, and one no write follows */
        { "function f() { const g = () => x; let x = 1; return g }", "x=0" },
        { "function f(v) { switch (v) { case 0: let x = 1; break; case 1: return () => x } }", "x=1" },
    };
    VM v = vm_new();
    size_t i;
    int read, r;
    char got[256];

    for (i = 0; i < countof(cases); i++) {
        for (read = 0; read < 2; read++) {
            r = white_by_value(v.ctx, cases[i].src, "f", read, got, sizeof(got));
            CHECK(r == 0 && !strcmp(got, cases[i].want), "%s%s: %d \"%s\", want \"%s\"",
                  cases[i].src, read ? " (read back)" : "", r, got, cases[i].want);
        }
    }
    /* 257 captured locals read two functions down, one written after the
       capture: past 256 closure variables and over five chunks of 64 */
    {
        char *src = malloc(16384), *want = malloc(16384);
        size_t at = 0, wt = 0, k;
        char *got2 = malloc(16384);
        at += snprintf(src + at, 16384 - at, "function f() { let ");
        for (k = 0; k < 257; k++)
            at += snprintf(src + at, 16384 - at, "%sa%zu = %zu", k ? ", " : "", k, k);
        at += snprintf(src + at, 16384 - at, "; const g = () => () => a0");
        for (k = 1; k < 257; k++)
            at += snprintf(src + at, 16384 - at, " + a%zu", k);
        snprintf(src + at, 16384 - at, "; a200 = 0; return g }");
        for (k = 0; k < 257; k++)
            wt += snprintf(want + wt, 16384 - wt, "%sa%zu=%d", k ? " " : "", k, k != 200);
        for (read = 0; read < 2; read++) {
            r = white_by_value(v.ctx, src, "f", read, got2, 16384);
            CHECK(r == 0 && !strcmp(got2, want), "257 locals%s: %d", read ? " (read back)" : "", r);
        }
        free(src);
        free(want);
        free(got2);
    }
    /* 2000 try and finally blocks: OP_ret goes to after each of the 4000
       OP_gosub through one node, not by an edge for each pair */
    {
        size_t cap2 = 64 * 2000 + 256, at = 0, k;
        char *src = malloc(cap2);
        at += snprintf(src + at, cap2 - at, "function f(h) { let x = 1, y = 0;\n");
        for (k = 0; k < 2000; k++)
            at += snprintf(src + at, cap2 - at, "try { h() } finally { h(y) }\n");
        snprintf(src + at, cap2 - at, "const g = () => x + y; y = 1; return g }");
        for (read = 0; read < 2; read++) {
            r = white_by_value(v.ctx, src, "f", read, got, sizeof(got));
            CHECK(r == 0 && !strcmp(got, "x=1 y=0"), "2000 finally blocks%s: %d \"%s\"",
                  read ? " (read back)" : "", r, got);
        }
        free(src);
    }
    /* a chain of REFs through 32 functions holds the value, one through 33
       keeps the JSVarRef (CBV_DEPTH) */
    for (i = 32; i <= 33; i++) {
        char src[512];
        size_t at = 0, k;
        at += snprintf(src + at, sizeof(src) - at, "function f() { let x = 1; return ");
        for (k = 0; k < i; k++)
            at += snprintf(src + at, sizeof(src) - at, "() => ");
        snprintf(src + at, sizeof(src) - at, "x }");
        for (read = 0; read < 2; read++) {
            r = white_by_value(v.ctx, src, "f", read, got, sizeof(got));
            CHECK(r == 0 && !strcmp(got, i == 32 ? "x=1" : "x=0"), "%zu functions deep%s: %d \"%s\"",
                  i, read ? " (read back)" : "", r, got);
        }
    }
    /* code a file can hold and no compiler makes: a define_class whose
       constructor is not the function of the push_const right before it
       (1), or that a jump lands on (2), keeps every slot a JSVarRef. The
       same function as compiled holds x by value. */
    {
        static const char src[] =
            "function f(a) { let x = 0; const g = () => x; if (a) a(); class C {} return g }";
        int craft;

        r = white_by_value(v.ctx, src, "f", 0, got, sizeof(got));
        CHECK(r == 0 && !strcmp(got, "x=1 <class_fields_init>=0"), "a class: %d \"%s\"", r, got);
        for (craft = 1; craft <= 2; craft++) {
            r = white_by_value_crafted(v.ctx, src, "f", craft, got, sizeof(got));
            CHECK(r == 0 && !strcmp(got, "x=0 <class_fields_init>=0"), "a class, changed %d: %d \"%s\"", craft, r, got);
        }
    }
    /* insert_polls puts an OP_nop after a run of POLL_RUN - 1 instructions
       with no count, which can fall between the push_const of a class and
       its define_class: the class is still one the pass knows. Runs of
       straight code of each length find the one. */
    {
        int a, m, found = 0;
        char src[4096];

        for (a = 0; a < 150 && !found; a++) {
            for (m = 0; m < 2 && !found; m++) {
                size_t at = 0;
                int k;
                at += snprintf(src + at, sizeof(src) - at, "function f(q) { let x = 0, p; const g = () => x; ");
                for (k = 0; k < a; k++)
                    at += snprintf(src + at, sizeof(src) - at, "p = q; ");
                for (k = 0; k < m; k++)
                    at += snprintf(src + at, sizeof(src) - at, "p = -q; ");
                snprintf(src + at, sizeof(src) - at, "class C {} return g }");
                if (white_class_after_nop(v.ctx, src, "f") != 1)
                    continue;
                found = 1;
                for (read = 0; read < 2; read++) {
                    r = white_by_value(v.ctx, src, "f", read, got, sizeof(got));
                    CHECK(r == 0 && !strcmp(got, "x=1 <class_fields_init>=0"), "a class after an OP_nop%s: %d \"%s\"",
                          read ? " (read back)" : "", r, got);
                }
            }
        }
        CHECK(found, "no run of code puts an OP_nop between the push_const of a class and its define_class");
    }
    vm_free(&v);
}

/* JS_ReadObject reads a function of as many closure variables as the
   compiler makes, 65534, and refuses more: by_value_count, 16 bits, counts
   the cells of a closure, one for each closure variable that holds its
   value. The function g has three, a and b that hold their values and w
   that the frame writes, and the copies of a hold theirs too: 65538 of
   them would give 65537 cells. */
static void test_closure_var_limit(void)
{
    static const char src[] =
        "function f(a) { let b = a + 1, w = 0;"
        " const g = function g() { const h = function h() { return a + b }; return h() + w }; w = 5; return g }\n"
        "f(1)()";
    static const int counts[] = { 65534, 65535, 65538 };
    VM v = vm_new(), w;
    JSValue c, obj, r;
    uint8_t *buf;
    size_t len;
    char *t, want[64];
    int i, n;

    c = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(c), "the script threw");
    for (i = 0; i < (int)countof(counts); i++) {
        n = counts[i];
        CHECK(white_wide_closure_vars(v.ctx, c, "g", n, 1) == 0, "%d closure variables: not made", n);
        buf = JS_WriteObject(v.ctx, &len, c, JS_WRITE_OBJ_BYTECODE);
        white_wide_closure_vars(v.ctx, c, "g", n, 0);
        CHECK(buf != NULL, "%d closure variables: cannot be written", n);
        if (!buf)
            continue;
        w = vm_new();
        obj = JS_ReadObject(w.ctx, buf, len, JS_READ_OBJ_BYTECODE);
        if (n == 65534) {
            CHECK(!JS_IsException(obj), "%d closure variables: the read threw", n);
            r = JS_EvalFunction(w.ctx, obj);
            CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_INT && JS_VALUE_GET_INT(r) == 8,
                  "%d closure variables: the script did not give 8", n);
            JS_FreeValue(w.ctx, r);
        } else {
            snprintf(want, sizeof(want), "SyntaxError: invalid bytecode: %d closure variables", n);
            t = exception_text(w.ctx);
            CHECK(JS_IsException(obj) && t && strstr(t, want), "%d closure variables: read (%s)",
                  n, t ? t : "no exception");
            free(t);
            JS_FreeValue(w.ctx, obj);
        }
        vm_free(&w);
        js_free(v.ctx, buf);
    }
    JS_FreeValue(v.ctx, c);
    vm_free(&v);
}

/* js_trigger_gc_block, before one large block of a built-in: with the block
   under the memory limit it collects nothing (a page that fits runs the
   collections it ran before), with garbage in cycles in the way it collects
   and the block is made, and a threshold of -1 turns it off as it does the
   collections of js_trigger_gc. The setup of a case runs before the garbage
   is made, so that the collection counted is the one of the operation. */
static void test_gc_block(void)
{
    static const struct {
        const char *setup, *op;
    } ops[] = {
        { NULL, "globalThis.r = 'x'.repeat(4 << 20); 0" },
        { NULL, "globalThis.r = ''.padStart(4 << 20, 'x'); 0" },
        /* a filler of one wide character: the pad is wide */
        { NULL, "globalThis.r = ''.padStart(2 << 20, '\\u0100'); 0" },
        { NULL, "globalThis.r = new ArrayBuffer(4 << 20); 0" },
        { NULL, "globalThis.r = new Float64Array(1 << 19); 0" },
        { "globalThis.s = new ArrayBuffer(16, { maxByteLength: 8 << 20 }); 0", "s.resize(4 << 20); 0" },
        { "globalThis.s = new ArrayBuffer(16); 0", "globalThis.r = s.transfer(4 << 20); 0" },
        { NULL, "{ const a = []; for (let i = 0; i < 1 << 18; i++) a.push(i * 0.5); globalThis.r = a } 0" },
        { NULL, "{ const a = []; for (let i = 0; i < 1 << 18; i++) a[i] = i * 0.5; globalThis.r = a } 0" },
        { "{ let p = 'y'.repeat(1 << 16), t = ''; for (let i = 0; i < 64; i++) t += p; globalThis.s = t } 0",
          "globalThis.r = s.indexOf('z'); 0" },
        { "{ const p = 'j'.repeat(1 << 16); globalThis.s = []; for (let i = 0; i < 64; i++) s.push(p + i) } 0",
          "globalThis.r = s.join(''); 0" },
        /* each element a join of its own, which ends before the outer
           buffer grows */
        { "{ const p = 'j'.repeat(1 << 16); globalThis.s = []; for (let i = 0; i < 64; i++) s.push([p, i]) } 0",
          "globalThis.r = s.join(''); 0" },
        { "{ globalThis.s = []; for (let i = 0; i < 1 << 18; i++) s.push(i) } 0", "globalThis.r = s.toSorted((a, b) => a - b); 0" },
        { "{ globalThis.s = []; for (let i = 0; i < 1 << 18; i++) s.push(i) } 0", "globalThis.r = s.slice(); 0" },
        { "{ globalThis.s = []; for (let i = 0; i < 1 << 18; i++) s.push(i) } 0", "globalThis.r = s.splice(0); 0" },
    };
    const char *garbage = "for (let i = 0; i < 120000; i++) { const a = { x: [1, 2, 3] }, b = { a }; a.b = b } 0";
    size_t i;

    for (i = 0; i < countof(ops); i++) {
        VM v = vm_new();
        unsigned long full;
        size_t room;
        int ok;

        /* no limit: the block fits, and no collection runs */
        JS_SetGCThreshold(v.rt, (size_t)1 << 40);
        if (ops[i].setup)
            CHECK(eval_ok(v.ctx, ops[i].setup), "the setup of op %zu threw", i);
        full = white_gc_run_count(0);
        CHECK(eval_ok(v.ctx, ops[i].op), "op %zu threw with no limit", i);
        CHECK(white_gc_run_count(0) == full, "op %zu with no limit ran %lu collections", i,
              white_gc_run_count(0) - full);
        vm_free(&v);

        /* garbage in the way: the room fits the block with what the
           garbage holds given back, and not without it */
        v = vm_new();
        if (ops[i].setup)
            CHECK(eval_ok(v.ctx, ops[i].setup), "the setup of op %zu threw", i);
        JS_RunGC(v.rt);
        JS_SetGCThreshold(v.rt, (size_t)1 << 40);
        room = JS_GetMemorySize(v.rt);
        CHECK(eval_ok(v.ctx, ops[i].op), "op %zu threw", i);
        room = JS_GetMemorySize(v.rt) - room + (1 << 20);
        vm_free(&v);

        v = vm_new();
        if (ops[i].setup)
            CHECK(eval_ok(v.ctx, ops[i].setup), "the setup of op %zu threw", i);
        JS_RunGC(v.rt);
        JS_SetGCThreshold(v.rt, (size_t)1 << 40);
        CHECK(eval_ok(v.ctx, garbage), "the garbage threw");
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + room / 2);
        full = white_gc_run_count(0);
        ok = eval_ok(v.ctx, ops[i].op);
        CHECK(ok, "op %zu ran out of memory with the garbage in the way", i);
        CHECK(white_gc_run_count(0) > full, "op %zu ran no collection with the garbage in the way", i);
        unlimit(&v);
        vm_free(&v);

        /* a threshold of -1: no collection, and out of memory */
        v = vm_new();
        JS_SetGCThreshold(v.rt, (size_t)-1);
        if (ops[i].setup)
            CHECK(eval_ok(v.ctx, ops[i].setup), "the setup of op %zu threw", i);
        CHECK(eval_ok(v.ctx, garbage), "the garbage threw");
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + room / 2);
        full = white_gc_run_count(0);
        {
            JSValue r = JS_Eval(v.ctx, ops[i].op, strlen(ops[i].op), "<unit>", JS_EVAL_TYPE_GLOBAL);
            char *t = exception_text(v.ctx);

            CHECK(JS_IsException(r) && t && (strstr(t, "out of memory") || strcmp(t, "null") == 0), "op %zu with the collections off: %s", i,
                  t ? t : "no exception");
            free(t);
            JS_FreeValue(v.ctx, r);
        }
        CHECK(white_gc_run_count(0) == full, "op %zu collected with a threshold of -1", i);
        unlimit(&v);
        JS_SetGCThreshold(v.rt, (size_t)1 << 40);
        vm_free(&v);
    }
}

/* js_trigger_gc_block counts what the heap gains, as the allocator does: a
   block that grows counts only its growth, and a rope already made flat
   counts nothing. Near the limit, with no garbage, steps that fit run no
   collection. */
static void test_gc_block_fits(void)
{
    static const struct {
        const char *setup, *op;
    } ops[] = {
        { "globalThis.s = new ArrayBuffer(4 << 20, { maxByteLength: 8 << 20 }); 0",
          "for (let i = 1; i <= 16; i++) s.resize((4 << 20) + i * 4096); 0" },
        { "{ let p = 'y'.repeat(1 << 16), t = ''; for (let i = 0; i < 64; i++) t += p; globalThis.s = t; t.indexOf('z') } 0",
          "for (let i = 0; i < 16; i++) s.indexOf('z'); 0" },
    };
    size_t i;

    for (i = 0; i < countof(ops); i++) {
        VM v = vm_new();
        unsigned long full;

        CHECK(eval_ok(v.ctx, ops[i].setup), "the setup of op %zu threw", i);
        JS_RunGC(v.rt);
        JS_SetGCThreshold(v.rt, (size_t)1 << 40);
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + (1 << 20));
        full = white_gc_run_count(0);
        CHECK(eval_ok(v.ctx, ops[i].op), "op %zu threw", i);
        CHECK(white_gc_run_count(0) == full, "op %zu ran %lu collections with no garbage", i,
              white_gc_run_count(0) - full);
        unlimit(&v);
        vm_free(&v);
    }
}

/* js_trigger_gc_block when the host holds memory for the VM (held in
   engine/heap.h): a block that fits once the host gives it back has it given
   back, and runs no collection. When the host keeps it, or when the block
   does not fit even without it, the collection runs. room is what the op
   leaves the heap holding, 1 MiB more, as test_gc_block measures it. A block
   that grows takes up to half again what it keeps on the way, so the case
   with no garbage has twice the room under the limit, all but half the room
   of it held by the host. */
static size_t gc_held_pressures, gc_held_after;

static size_t gc_held_pressure(uint32_t vm, size_t request)
{
    (void)vm;
    (void)request;
    gc_held_pressures++;
    return gc_held_after;
}

/* the host holds n bytes: it writes held, then the limit less held */
static void gc_held_hold(size_t *words, size_t n)
{
    words[2] = n;
    words[0] = words[1] == (size_t)-1 ? (size_t)-1 : words[1] > n ? words[1] - n : 0;
}

/* a new VM with the setup of an op made and collected, and no collection
   by the threshold */
static VM gc_held_vm(const char *setup)
{
    VM v = vm_new();

    if (setup)
        CHECK(eval_ok(v.ctx, setup), "the setup threw: %s", setup);
    JS_RunGC(v.rt);
    JS_SetGCThreshold(v.rt, (size_t)1 << 40);
    return v;
}

static void test_gc_block_held(void)
{
    static const struct {
        const char *setup, *op;
    } ops[] = {
        { NULL, "globalThis.r = 'x'.repeat(4 << 20); 0" },
        { NULL, "globalThis.r = new ArrayBuffer(4 << 20); 0" },
        { NULL, "{ const a = []; for (let i = 0; i < 1 << 18; i++) a.push(i * 0.5); globalThis.r = a } 0" },
        { "{ const p = 'j'.repeat(1 << 16); globalThis.s = []; for (let i = 0; i < 64; i++) s.push(p + i) } 0",
          "globalThis.r = s.join(''); 0" },
    };
    const char *garbage = "for (let i = 0; i < 120000; i++) { const a = { x: [1, 2, 3] }, b = { a }; a.b = b } 0";
    size_t i, room, held, *words;

    host_pressure_call = gc_held_pressure;
    for (i = 0; i < countof(ops); i++) {
        VM v = gc_held_vm(ops[i].setup);
        unsigned long full;

        room = JS_GetMemorySize(v.rt);
        CHECK(eval_ok(v.ctx, ops[i].op), "op %zu threw", i);
        room = JS_GetMemorySize(v.rt) - room + (1 << 20);
        held = room + room / 2;
        vm_free(&v);

        /* no garbage, and the block fits once the host gives back what it
           holds: the host is asked once, and no collection runs */
        v = gc_held_vm(ops[i].setup);
        words = JS_GetHostLimit(v.rt);
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + 2 * room);
        gc_held_hold(words, held);
        gc_held_pressures = 0;
        gc_held_after = 0;
        full = white_gc_run_count(0);
        CHECK(eval_ok(v.ctx, ops[i].op), "op %zu ran out of memory with the host holding %zu bytes", i, held);
        CHECK(white_gc_run_count(0) == full, "op %zu ran %lu collections where the host could give back what it held", i,
              white_gc_run_count(0) - full);
        CHECK(gc_held_pressures == 1 && words[2] == 0, "op %zu: %zu requests to the host, %zu still held, want 1 and 0", i,
              gc_held_pressures, words[2]);
        gc_held_hold(words, 0);
        unlimit(&v);
        vm_free(&v);

        /* the host keeps what it holds, and garbage is in the way: the
           collection runs */
        v = gc_held_vm(ops[i].setup);
        words = JS_GetHostLimit(v.rt);
        CHECK(eval_ok(v.ctx, garbage), "the garbage threw");
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + room / 2 + held);
        gc_held_hold(words, held);
        gc_held_after = held;
        full = white_gc_run_count(0);
        CHECK(eval_ok(v.ctx, ops[i].op), "op %zu ran out of memory with the host keeping what it holds and the garbage in the way", i);
        CHECK(white_gc_run_count(0) > full, "op %zu ran no collection with the host keeping what it holds", i);
        CHECK(words[2] == held, "op %zu: the host holds %zu, want %zu", i, words[2], held);
        gc_held_hold(words, 0);
        unlimit(&v);
        vm_free(&v);

        /* the block does not fit even with what the host holds given back,
           and garbage is in the way: the collection runs */
        v = gc_held_vm(ops[i].setup);
        words = JS_GetHostLimit(v.rt);
        CHECK(eval_ok(v.ctx, garbage), "the garbage threw");
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + room / 2);
        gc_held_hold(words, 64 << 10);
        gc_held_after = 0;
        full = white_gc_run_count(0);
        CHECK(eval_ok(v.ctx, ops[i].op), "op %zu ran out of memory with the garbage in the way and the host holding a little", i);
        CHECK(white_gc_run_count(0) > full, "op %zu ran no collection with the garbage in the way and the host holding a little", i);
        gc_held_hold(words, 0);
        unlimit(&v);
        vm_free(&v);
    }
    host_pressure_call = NULL;
}

/* The buffer of join made 16 bits wide where it is, with room left
   (string_buffer_widen): the narrow buffer grows to 1.5 Mi characters for
   1.25 Mi of them, which takes 1.5 MiB of room, and its wide copy takes
   3 MiB, 1.5 MiB more than the narrow one. With 2.25 MiB of room and the
   garbage in the way, the narrow buffer fits and the wide one collects
   first. That is the second join, which takes the loop that reads one
   element at a time. The first, of strings only, takes one block of 16 bits
   for its 1.25 Mi characters (js_join_plan), 2.5 MiB, which collects first
   too. */
static void test_gc_block_widen(void)
{
    static const char *const setups[] = {
        "globalThis.s = ['j'.repeat(1 << 20), 'j'.repeat(1 << 18), '\\u0100']; 0",
        "globalThis.s = ['j'.repeat(1 << 20), 'j'.repeat(1 << 18), 0, '\\u0100']; 0",
    };
    const char *garbage = "for (let i = 0; i < 120000; i++) { const a = { x: [1, 2, 3] }, b = { a }; a.b = b } 0";
    size_t i;

    for (i = 0; i < countof(setups); i++) {
        VM v = vm_new();
        unsigned long full;

        CHECK(eval_ok(v.ctx, setups[i]), "the setup threw");
        JS_RunGC(v.rt);
        JS_SetGCThreshold(v.rt, (size_t)1 << 40);
        CHECK(eval_ok(v.ctx, garbage), "the garbage threw");
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + (9 << 18));
        full = white_gc_run_count(0);
        CHECK(eval_ok(v.ctx, "globalThis.r = s.join(''); r.length"), "join %zu ran out of memory", i);
        CHECK(white_gc_run_count(0) > full, "join %zu ran no collection", i);
        unlimit(&v);
        vm_free(&v);
    }
}

/* A join makes room under the memory limit where the loop that reads one
   element at a time made it, and nowhere else (js_join_plan). Under a limit
   1 KiB short of the characters of the result, with cycles of garbage that
   the threshold kept, a join whose loop's buffer grows to 64 Ki characters
   collects and gives the string, and one whose buffer stays under runs out
   of memory with no collection, as the loop did: of 8 and of 16 bits, just
   under 43,692 characters (the fast join, no room made), between it and
   64 Ki (the loop: 45,000 and 5,000 characters make room, 30,000 and 15,000
   and one element of 65,535 do not) and at 64 Ki (the fast join, room
   made). */
static void test_join_gc_block_small(void)
{
    static const struct {
        const char *arr;
        size_t bytes;
        int room;
    } cases[] = {
        { "['a'.repeat(45000), 'b'.repeat(5000)]", 50000, 1 },
        { "['\\u4e00'.repeat(45000), 'b'.repeat(5000)]", 100000, 1 },
        { "['a'.repeat(4000), 'b'.repeat(1000)]", 5000, 0 },
        { "['\\u4e00'.repeat(4000), 'b'.repeat(1000)]", 10000, 0 },
        { "['a'.repeat(43000), 'b'.repeat(691)]", 43691, 0 },
        { "['\\u4e00'.repeat(43000), 'b'.repeat(691)]", 87382, 0 },
        { "['a'.repeat(30000), 'b'.repeat(15000)]", 45000, 0 },
        { "['\\u4e00'.repeat(30000), 'b'.repeat(15000)]", 90000, 0 },
        { "['a'.repeat(65535)]", 65535, 0 },
        { "['\\u4e00'.repeat(65535)]", 131070, 0 },
        { "['a'.repeat(65000), 'b'.repeat(536)]", 65536, 1 },
        { "['\\u4e00'.repeat(65000), 'b'.repeat(536)]", 131072, 1 },
    };
    const char *garbage = "for (let i = 0; i < 20000; i++) { const a = { x: [1, 2, 3] }, b = { a }; a.b = b } 0";
    size_t i;

    for (i = 0; i < countof(cases); i++) {
        VM v = vm_new();
        JSValue glob, f, r;
        char src[200];
        unsigned long full;

        /* f runs once here, so the call below makes nothing but the join */
        snprintf(src, sizeof(src), "globalThis.s = %s; globalThis.f = () => s.join(''); f().length",
                 cases[i].arr);
        CHECK(eval_ok(v.ctx, src), "case %zu: the setup threw", i);
        glob = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, glob, "f");
        JS_FreeValue(v.ctx, glob);
        JS_RunGC(v.rt);
        JS_SetGCThreshold(v.rt, (size_t)1 << 40);
        CHECK(eval_ok(v.ctx, garbage), "case %zu: the garbage threw", i);
        /* the scratch of the register pass and the blocks the front keeps
           would be given back when a request fails, room the limit does not
           count on */
        white_regvm_scratch_drain(v.ctx, 0);
        /* 1 KiB short of the characters: the block of the result does not
           fit without a collection */
        JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + cases[i].bytes - 1024);
        full = white_gc_run_count(0);
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
        if (cases[i].room) {
            CHECK(!JS_IsException(r), "case %zu: the join ran out of memory", i);
            CHECK(white_gc_run_count(0) > full, "case %zu: the join ran no collection", i);
        } else {
            CHECK(JS_IsException(r) && JS_IsUncatchableException(v.ctx),
                  "case %zu: the join did not run out of memory", i);
            CHECK(white_gc_run_count(0) == full, "case %zu: the join ran a collection", i);
        }
        if (JS_IsException(r))
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, f);
        unlimit(&v);
        vm_free(&v);
    }
}

/* Short joins near the memory limit with nothing to free run no
   collection, as the loop's buffer made none under 64 Ki characters: 10,000
   joins of two characters, with 10,000 live objects and 6 KiB of room, less
   than the margin of js_trigger_gc_block. */
static void test_join_gc_block_short(void)
{
    VM v = vm_new();
    JSValue glob, a, join, sep;
    unsigned long full;
    int i, failed = 0;

    CHECK(eval_ok(v.ctx, "globalThis.keep = Array.from({ length: 10000 }, () => ({})); globalThis.a = ['a', 'b']; 0"),
          "the setup threw");
    glob = JS_GetGlobalObject(v.ctx);
    a = JS_GetPropertyStr(v.ctx, glob, "a");
    join = JS_GetPropertyStr(v.ctx, a, "join");
    sep = JS_NewString(v.ctx, "");
    JS_FreeValue(v.ctx, glob);
    JS_RunGC(v.rt);
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + 6144);
    full = white_gc_run_count(0);
    for (i = 0; i < 10000; i++) {
        JSValue r = JS_Call(v.ctx, join, a, 1, &sep);

        if (JS_IsException(r)) {
            failed++;
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
        }
        JS_FreeValue(v.ctx, r);
    }
    CHECK(failed == 0, "%d of the 10,000 joins failed", failed);
    CHECK(white_gc_run_count(0) == full, "10,000 joins of two characters ran %lu collections",
          white_gc_run_count(0) - full);
    JS_FreeValue(v.ctx, sep);
    JS_FreeValue(v.ctx, join);
    JS_FreeValue(v.ctx, a);
    unlimit(&v);
    vm_free(&v);
}

/* Array.prototype.join of a fast array of strings, ropes, undefined and null
   takes one block of the length and width of the result (js_join_plan) and
   reads the elements in place. It gives the same string in the same form, 8
   or 16 bits, as the loop that reads the elements one at a time gives for an
   object with the same elements: W and RW hold 16-bit units that are all
   under 0x100, as a wide buffer can leave them, and a result made of them is
   8 bits. The one call of the allocator failing gives the out of memory
   error and leaves nothing behind. An array of anything else, with holes,
   whose result would be too long, or whose result has 43,692 to 65,535
   characters, at either width (js_join_plan), takes the loop. */
static void test_join_fast(void)
{
    static const struct {
        const char *what, *arr, *sep;
    } joins[] = {
        { "8 bits", "['a', 'bc', 'def'.repeat(20)]", "undefined" },
        { "undefined and null", "['a', undefined, 'b'.repeat(40), null, 'c', undefined]", "'-'" },
        { "a separator of three", "['ab', 'cd'.repeat(20), 'ef']", "'<->'" },
        { "an empty separator", "['ab', 'cd'.repeat(20), 'ef']", "''" },
        { "a wide separator", "['ab'.repeat(20), 'cd', 'ef']", "'\\u4e2d'" },
        { "a wide separator, one element", "['ab'.repeat(30)]", "'\\u4e2d\\u6587'" },
        { "a wide element", "['ab'.repeat(20), '\\u4e2d', 'ef']", "undefined" },
        { "a wide element last", "['ab'.repeat(20), 'ef', 'x\\u4e2d']", "'\\u00e9'" },
        { "16-bit units under 0x100", "['ab'.repeat(20), W, 'ef']", "undefined" },
        { "a separator of 16-bit units under 0x100", "['ab', 'cd'.repeat(20)]", "W" },
        { "ropes", "[R8, 'x', R16]", "'+'" },
        { "a rope of 16-bit units under 0x100", "[R8, RW]", "undefined" },
        { "one element", "['abc'.repeat(20)]", "undefined" },
        { "one rope", "[R16]", "undefined" },
        { "empty strings", "['', '', '']", "''" },
        { "only undefined and null", "[undefined, null]", "undefined" },
    };
    static const struct {
        const char *what, *arr, *sep;
        int fast;
        int64_t len;
        int wide;
    } plans[] = {
        { "a number", "['a', 1, 'b']", "undefined", 0, 0 },
        { "an object", "['a', {}, 'b']", "undefined", 0, 0 },
        { "a hole at the end", "(() => { const x = ['a', 'b']; x.length = 3; return x })()", "undefined", 0, 0 },
        { "itself", "(() => { const x = ['a']; x.push(x); return x })()", "undefined", 0, 0 },
        { "empty", "[]", "undefined", 0, 0 },
        { "not an array", "({ length: 2, 0: 'a', 1: 'b' })", "undefined", 0, 0 },
        { "2^29 characters", "[R29]", "undefined", 1, 1 << 29 },
        { "2^30 - 1 characters", "[R29, R29m, 'q'.repeat(511)]", "''", 1, (1 << 30) - 1 },
        { "2^30 characters", "[R29, R29m, 'q'.repeat(512)]", "''", 0, 0 },
        { "2^30 - 1 characters with commas", "[R29, R29m, 'q'.repeat(509)]", "undefined", 1, (1 << 30) - 1 },
        { "2^30 characters with commas", "[R29, R29m, 'q'.repeat(510)]", "undefined", 0, 0 },
        { "2^30 - 1 characters of separators", "Array(1024).fill('')", "S", 1, (1 << 30) - 1 },
        { "2^30 characters of separators", "Array(1025).fill('')", "S", 0, 0 },
        { "43,691 characters", "['a'.repeat(43000), 'b'.repeat(690)]", "undefined", 1, 43691 },
        { "43,692 characters", "['a'.repeat(43000), 'b'.repeat(691)]", "undefined", 0, 0 },
        { "65,535 characters", "['a'.repeat(65000), 'b'.repeat(534)]", "undefined", 0, 0 },
        { "65,536 characters", "['a'.repeat(65000), 'b'.repeat(535)]", "undefined", 1, 65536 },
        { "43,691 characters of 16 bits", "['\\u4e00'.repeat(43000), 'b'.repeat(690)]", "undefined", 1, 43691, 1 },
        { "43,692 characters of 16 bits", "['\\u4e00'.repeat(43000), 'b'.repeat(691)]", "undefined", 0, 0 },
        { "65,535 characters of 16 bits", "['\\u4e00'.repeat(65000), 'b'.repeat(534)]", "undefined", 0, 0 },
        { "65,536 characters of 16 bits", "['\\u4e00'.repeat(65000), 'b'.repeat(535)]", "undefined", 1, 65536, 1 },
    };
    VM v = vm_new();
    JSValue glob, f, g;
    char src[256];
    size_t i;

    glob = JS_GetGlobalObject(v.ctx);
    JS_SetPropertyStr(v.ctx, glob, "W", white_wide_latin1(v.ctx, "16 bits, latin-1 \xe9"));
    CHECK(eval_ok(v.ctx,
                  "globalThis.R8 = 'x'.repeat(600) + 'y'.repeat(600)\n"
                  "globalThis.R16 = '\\u4e2d'.repeat(300) + 'y'.repeat(600)\n"
                  "globalThis.RW = W + 'z'.repeat(600)\n"
                  "globalThis.R29 = 'q'.repeat(1 << 20), R29m = 'q'.repeat((1 << 20) - 1)\n"
                  "for (let i = 0; i < 9; i++) R29 = R29 + R29, R29m = R29m + R29m\n"
                  "globalThis.S = 'q'.repeat((1 << 20) + (1 << 10) + 1)\n"
                  "globalThis.f = () => a.join(s)\n"
                  "globalThis.g = () => Array.prototype.join.call(Object.assign({ length: a.length }, a), s)\n"
                  "0"), "the setup threw");
    {
        static const char *const ropes[] = { "R8", "R16", "RW", "R29", "R29m" };
        static const int wide[] = { 0, 1, 1, 0, 0 };
        JSValue w = JS_GetPropertyStr(v.ctx, glob, "W");

        CHECK(white_string_is_rope(w) == 0 && white_string_wide(w) == 1, "W is no flat string of 16 bits");
        JS_FreeValue(v.ctx, w);
        for (i = 0; i < countof(ropes); i++) {
            JSValue r = JS_GetPropertyStr(v.ctx, glob, ropes[i]);

            CHECK(white_string_is_rope(r) == 1 && white_string_wide(r) == wide[i], "%s is no rope of %d bits",
                  ropes[i], wide[i] ? 16 : 8);
            JS_FreeValue(v.ctx, r);
        }
    }
    f = JS_GetPropertyStr(v.ctx, glob, "f");
    g = JS_GetPropertyStr(v.ctx, glob, "g");
    for (i = 0; i < countof(joins); i++) {
        JSValue a, s, want, r;
        int64_t len = -1;
        int wide = -1, fail, made, n = 0;

        snprintf(src, sizeof(src), "globalThis.a = %s; globalThis.s = %s; 0", joins[i].arr, joins[i].sep);
        CHECK(eval_ok(v.ctx, src), "%s: the setup threw", joins[i].what);
        a = JS_GetPropertyStr(v.ctx, glob, "a");
        s = JS_GetPropertyStr(v.ctx, glob, "s");
        CHECK(white_join_plan(v.ctx, a, s, &len, &wide) == 1, "%s: no fast join", joins[i].what);
        JS_FreeValue(v.ctx, a);
        JS_FreeValue(v.ctx, s);
        want = JS_Call(v.ctx, g, JS_UNDEFINED, 0, NULL);
        CHECK(!JS_IsException(want), "%s: the join of the object threw", joins[i].what);
        /* a first call of f sets f up (its code and caches), which is not
           the join's to count */
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
        CHECK(!JS_IsException(r), "%s: the join threw", joins[i].what);
        JS_FreeValue(v.ctx, r);
        for (fail = 0; fail == 0 || fail <= n; fail++) {
            JSMemoryUsage before, after;

            JS_RunGC(v.rt);
            JS_ComputeMemoryUsage(v.rt, &before);
            r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);
            if (fail == 0) {
                JSValue rl = JS_GetPropertyStr(v.ctx, r, "length");
                int64_t got = -1;

                JS_ToInt64(v.ctx, &got, rl);
                n = made;
                CHECK(made == 1, "%s: %d calls of the allocator, want 1", joins[i].what, made);
                CHECK(JS_StrictEq(v.ctx, r, want), "%s: another string than the join of the object", joins[i].what);
                CHECK(white_string_wide(r) == white_string_wide(want), "%s: %d bits, the join of the object %d",
                      joins[i].what, white_string_wide(r) ? 16 : 8, white_string_wide(want) ? 16 : 8);
                CHECK(wide == white_string_wide(r) && len == got,
                      "%s: planned %lld characters of %d bits", joins[i].what, (long long)len, wide ? 16 : 8);
            } else {
                CHECK(JS_IsException(r) && JS_IsUncatchableException(v.ctx),
                      "%s: call %d failing: no out of memory error", joins[i].what, fail);
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            }
            JS_FreeValue(v.ctx, r);
            JS_RunGC(v.rt);
            JS_ComputeMemoryUsage(v.rt, &after);
            CHECK(after.obj_count == before.obj_count && after.str_count == before.str_count &&
                  after.malloc_size == before.malloc_size,
                  "%s: call %d failing: %lld objects, %lld strings and %lld bytes, before it %lld, %lld and %lld",
                  joins[i].what, fail, (long long)after.obj_count, (long long)after.str_count,
                  (long long)after.malloc_size, (long long)before.obj_count, (long long)before.str_count,
                  (long long)before.malloc_size);
        }
        JS_FreeValue(v.ctx, want);
    }
    for (i = 0; i < countof(plans); i++) {
        JSValue a, s;
        int64_t len = -1;
        int wide = -1, fast;

        snprintf(src, sizeof(src), "globalThis.a = %s; globalThis.s = %s; 0", plans[i].arr, plans[i].sep);
        CHECK(eval_ok(v.ctx, src), "%s: the setup threw", plans[i].what);
        a = JS_GetPropertyStr(v.ctx, glob, "a");
        s = JS_GetPropertyStr(v.ctx, glob, "s");
        fast = white_join_plan(v.ctx, a, s, &len, &wide);
        CHECK(fast == plans[i].fast && (!fast || (len == plans[i].len && wide == plans[i].wide)),
              "%s: plan %d of %lld characters, want %d of %lld", plans[i].what, fast, (long long)len,
              plans[i].fast, (long long)plans[i].len);
        JS_FreeValue(v.ctx, a);
        JS_FreeValue(v.ctx, s);
    }
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, g);
    JS_FreeValue(v.ctx, glob);
    vm_free(&v);
}

/* js_closure2 gives a function its block of closure variables once every
   entry is written, as it does not zero the block: a collection in the
   middle reads none of it. A script of 17 global definitions, the last of
   which a global let already holds, throws in the first pass of
   js_closure2, and the error it makes may run a collection. The threshold
   goes over the bytes around the count, 8 at a time, so that some round
   puts the collection there and not at the making of the function just
   before (a collection sets the threshold far above the count). Before
   each round, an array of the size of the block (136 bytes) leaves its
   bytes in the allocator. */
static void test_closure_gc_in_build(void)
{
    static const char src[] =
        "var a0, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15, x;";
    VM v = vm_new();
    JSValue c, r;
    char *t;
    long d;

    CHECK(eval_ok(v.ctx, "let x = 1"), "the global let threw");
    for (d = -512; d <= 4096; d += 8) {
        c = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
        CHECK(!JS_IsException(c), "the script threw");
        CHECK(eval_ok(v.ctx, "new Uint8Array(136).fill(165), 0"), "the array threw");
        JS_SetGCThreshold(v.rt, white_gc_count(v.rt) + d);
        r = JS_EvalFunction(v.ctx, c);
        JS_SetGCThreshold(v.rt, (size_t)1 << 40);
        t = exception_text(v.ctx);
        CHECK(JS_IsException(r) && t && strstr(t, "SyntaxError"), "at %ld: %s", d, t ? t : "no exception");
        free(t);
        JS_FreeValue(v.ctx, r);
    }
    vm_free(&v);
}

/* The marks of capture_by_value go into the file with the code. A read
   without JS_READ_OBJ_POLLS_CHECKED makes them again and refuses a file
   whose marks are not those, with a mark turned either way. A read with
   the flag takes them as they are, and runs the same. */
static void test_by_value_file(void)
{
    static const char src[] =
        "function f(a) { let b = a + 1, w = 0;"
        " const g = function g() { const h = function h() { return a + b }; return h() + w }; w = 5; return g }\n"
        "f(1)()";
    static const char marks[] = "<eval>:0/0 f:/0 g:110/2 h:11/2";
    static const struct {
        const char *what, *name;
        int was;
    } flips[] = {
        { "a closure variable that holds its value made a JSVarRef", "h", 1 },
        { "a closure variable the frame writes made to hold its value", "g", 0 },
    };
    VM v = vm_new(), w;
    JSValue c, obj, r;
    char got[256];
    uint8_t *buf;
    size_t len;
    char *t;
    int checked, i, k;

    c = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(c), "the script threw");
    white_marks(v.ctx, c, got, sizeof(got));
    CHECK(!strcmp(got, marks), "compiled: \"%s\", want \"%s\"", got, marks);
    buf = JS_WriteObject(v.ctx, &len, c, JS_WRITE_OBJ_BYTECODE);
    CHECK(buf != NULL, "the script cannot be written");
    for (checked = 0; buf && checked < 2; checked++) {
        w = vm_new();
        obj = JS_ReadObject(w.ctx, buf, len, JS_READ_OBJ_BYTECODE | (checked ? JS_READ_OBJ_POLLS_CHECKED : 0));
        CHECK(!JS_IsException(obj), "read%s: threw", checked ? " with the flag" : "");
        white_marks(w.ctx, obj, got, sizeof(got));
        CHECK(!strcmp(got, marks), "read%s: \"%s\", want \"%s\"", checked ? " with the flag" : "", got, marks);
        r = JS_EvalFunction(w.ctx, obj);
        CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_INT && JS_VALUE_GET_INT(r) == 8, "read%s: the script did not give 8",
              checked ? " with the flag" : "");
        JS_FreeValue(w.ctx, r);
        vm_free(&w);
    }
    js_free(v.ctx, buf);

    for (i = 0; i < (int)countof(flips); i++) {
        k = white_flip_by_value(v.ctx, c, flips[i].name, flips[i].was, -1);
        CHECK(k >= 0, "%s: no such closure variable", flips[i].what);
        buf = JS_WriteObject(v.ctx, &len, c, JS_WRITE_OBJ_BYTECODE);
        if (k >= 0)
            white_flip_by_value(v.ctx, c, flips[i].name, 0, k);
        CHECK(buf != NULL, "%s: cannot be written", flips[i].what);
        if (!buf)
            continue;
        w = vm_new();
        obj = JS_ReadObject(w.ctx, buf, len, JS_READ_OBJ_BYTECODE);
        t = exception_text(w.ctx);
        CHECK(JS_IsException(obj) && t && strstr(t, "SyntaxError: invalid bytecode: code no compiler makes"),
              "%s: read (%s)", flips[i].what, t ? t : "no exception");
        free(t);
        JS_FreeValue(w.ctx, obj);
        vm_free(&w);
        js_free(v.ctx, buf);
    }
    white_marks(v.ctx, c, got, sizeof(got));
    CHECK(!strcmp(got, marks), "the marks put back: \"%s\"", got);
    JS_FreeValue(v.ctx, c);
    vm_free(&v);
}

/* capture_by_value gets no memory: the compile, or the read without
   JS_READ_OBJ_POLLS_CHECKED, fails with out of memory, and never gives
   other marks (which a read would refuse as code no compiler makes). Each
   call of js_malloc_rt and js_realloc_rt fails in turn, for the script of
   test_by_value_file, whose analysis follows the flow. */
static void test_by_value_oom(void)
{
    static const char src[] =
        "function f(a) { let b = a + 1, w = 0;"
        " const g = function g() { const h = function h() { return a + b }; return h() + w }; w = 5; return g }\n"
        "f(1)()";
    static const char marks[] = "<eval>:0/0 f:/0 g:110/2 h:11/2";
    VM v = vm_new(), w;
    JSValue c, obj;
    char got[256];
    uint8_t *buf;
    size_t len;
    char *t;
    int fail, n, read, failed[2] = { 0, 0 };

    c = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(c), "the script threw");
    buf = JS_WriteObject(v.ctx, &len, c, JS_WRITE_OBJ_BYTECODE);
    CHECK(buf != NULL, "the script cannot be written");
    JS_FreeValue(v.ctx, c);
    for (read = 0; buf && read < 2; read++) {
        for (fail = 1;; fail++) {
            w = vm_new();
            if (read)
                obj = white_read_failing(w.ctx, buf, len, fail, &n);
            else
                obj = white_compile_failing(w.ctx, src, fail, 1, &n);
            if (JS_IsException(obj)) {
                t = exception_text(w.ctx);
                CHECK(t && strstr(t, "out of memory"), "%s, call %d failing: %s",
                      read ? "read" : "compile", fail, t ? t : "no exception");
                free(t);
                failed[read]++;
            } else {
                white_marks(w.ctx, obj, got, sizeof(got));
                CHECK(!strcmp(got, marks), "%s, call %d failing: \"%s\"", read ? "read" : "compile", fail, got);
                JS_FreeValue(w.ctx, obj);
            }
            vm_free(&w);
            if (fail > n)
                break;
        }
        CHECK(failed[read] > 0, "%s: no call failed", read ? "read" : "compile");
    }
    js_free(v.ctx, buf);
    printf("unit-white: test_by_value_oom: %d compiles and %d reads failed\n", failed[0], failed[1]);
    vm_free(&v);
}

/* The checks of the temporal dead zone the compiler keeps (tdz_checks): a
   read or a write of a let, a const or a class known to come after its
   declaration has none, and a variable left with no check, no capture and
   no other read is not set uninitialized on entering its block, when that
   block runs once: in a loop or a finally it stays, since it frees the value
   of the run before (a buffer of each run of a loop under a memory limit
   that holds one). The checks
   stay where a jump can reach the use without the declaration: a case of a
   switch, a read before, in its initializer, in a closure, at the head of a
   loop it is declared in, after a catch or a finally. Each function also runs
   and gives what Node gives. With JS_SetFusedOps off, every check stays. */
static int count_ops(JSContext *ctx, const char *src, int w);

static void test_tdz_checks(void)
{
    static const struct {
        const char *src, *call, *want;
        int checks, uninit; /* the checks left (get, put, set) and the set_loc_uninitialized */
    } cases[] = {
        { "function f() { let a = 1; const b = a + 1; return a + b }", "f()", "3", 0, 0 },
        { "function f(c) { const a = 2; if (c) { a + 1 } else { a - 1 } return a * 3 }", "f(1)", "6", 0, 0 },
        { "function f(c) { let a = 2; if (c) a = 5; else a = 7; return a }", "f(0)", "7", 0, 0 },
        { "function f(n) { const k = 3; let s = 0; for (let i = 0; i < n; i++) s += i * k; return s }", "f(4)", "18", 0, 0 },
        { "function f(n) { let s = 0; let i = 0; while (i < n) { const d = i * 2; s += d; i++ } return s }", "f(4)", "12", 0, 1 },
        { "function f(o) { const a = o && o.x; const b = a || 3; return a + b }", "f({ x: 1 })", "2", 0, 0 },
        { "function f() { let a = 1; a = a + 1; a += 2; a++; return a }", "f()", "5", 0, 0 },
        /* a jump into the stretch from before the declaration */
        { "function f() { const r = a; let a = 1; return r }", "typeof f", "function", 1, 1 },
        { "function f() { a = 2; let a = 1; return a }", "typeof f", "function", 1, 1 },
        { "function f() { let a = a + 1; return a }", "typeof f", "function", 1, 1 },
        { "function f(v) { switch (v) { case 0: let a = 'a'; return a; case 1: return a } }", "f(0)", "a", 1, 1 },
        { "function f(v) { switch (v) { case 0: let a = 'x'; case 1: return a } }", "f(0)", "x", 1, 1 },
        { "function f(v) { switch (v) { case 0: { let a = 'x'; return a } } }", "f(0)", "x", 0, 0 },
        /* captured: no check is needed in f, but the closure still checks */
        { "function f() { const g = () => a; let a = 1; return a + g() }", "f()", "2", 0, 1 },
        { "function f() { let a = 2; function g() { return a } return g() + a }", "f()", "4", 0, 1 },
        { "function f() { const fs = []; for (let i = 0; i < 3; i++) fs.push(() => i); return fs.map(g => g()).join() }",
          "f()", "0,1,2", 0, 1 },
        /* a catch and a finally inside the stretch */
        { "function f() { let a = 1; try { a = 2; throw 0 } catch (e) { a += 3 } return a }", "f()", "5", 2, 1 },
        { "function f() { let a = 1; try { a = 2 } finally { a = 3 } return a }", "f()", "3", 2, 1 },
        { "function f() { try { throw 4 } catch (e) { const a = e * 2; return a } }", "f()", "8", 0, 0 },
        /* generators and async functions go on after each yield and await */
        { "function* f() { let a = 1; yield a; a++; yield a; const b = a * 10; yield b }", "[...f()].join()", "1,2,20", 0, 0 },
        { "async function f() { let a = 1; await null; a += 1; return a }", "typeof f", "function", 0, 0 },
        /* a block that a loop or a finally runs again keeps its set_loc_uninitialized,
           which frees the value of the run before */
        { "function f(n) { let s = 0; for (let i = 0; i < n; i++) { let b = [i]; s += b[0] } return s }", "f(4)", "6", 0, 1 },
        { "function* f() { for (let i = 0; i < 2; i++) { yield i; let b = [i]; yield b[0] } }", "[...f()].join()", "0,0,1,1", 0, 1 },
        { "function f(c) { let r = 0; try { if (c) return 1 } finally { let b = [2]; r = b[0] } return r }", "f(0)", "2", 2, 2 },
        { "function f() { let r = 0; try { r = 1 } finally { r += 1 } let b = [r]; return b[0] }", "f()", "2", 2, 1 },
        /* eval and with */
        { "function f() { let a = 3; return eval('a') + a }", "f()", "6", 0, 1 },
        { "function f() { let a = 1; with ({ b: 2 }) { a = a + b } return a }", "f()", "3", 0, 2 },
        { "function f() { let a = 1; with ({ a: 5 }) { a = a + 1 } return a }", "f()", "1", 0, 2 },
    };
    VM v = vm_new();
    size_t i;
    int left, uninit;
    char src[512];

    for (i = 0; i < countof(cases); i++) {
        /* CHECK counts in checks */
        left = count_ops(v.ctx, cases[i].src, W_GET_LOC_CHECK) + count_ops(v.ctx, cases[i].src, W_PUT_LOC_CHECK) +
                 count_ops(v.ctx, cases[i].src, W_SET_LOC_CHECK);
        uninit = count_ops(v.ctx, cases[i].src, W_SET_LOC_UNINITIALIZED);
        CHECK(left == cases[i].checks && uninit == cases[i].uninit, "%s: %d checks and %d set_loc_uninitialized, want %d and %d",
              cases[i].src, left, uninit, cases[i].checks, cases[i].uninit);
        snprintf(src, sizeof(src), "%s; %s", cases[i].src, cases[i].call);
        CHECK(eval_is(v.ctx, src, cases[i].want), "%s: %s", cases[i].src, cases[i].call);
    }
    /* with the switch off, every check stays */
    JS_SetFusedOps(v.ctx, 0);
    left = count_ops(v.ctx, cases[0].src, W_GET_LOC_CHECK);
    uninit = count_ops(v.ctx, cases[0].src, W_SET_LOC_UNINITIALIZED);
    CHECK(left == 3 && uninit == 2, "off: %d get_loc_check and %d set_loc_uninitialized, want 3 and 2", left, uninit);
    JS_SetFusedOps(v.ctx, 1);
    /* the value of the run before is freed as the block of a loop starts
       again: two buffers of 8 MiB never live at once under a limit of 12 */
    JS_SetMemoryLimit(v.rt, JS_GetMemorySize(v.rt) + 12 * 1024 * 1024);
    CHECK(eval_is(v.ctx, "(function () { let s = 0; for (let i = 0; i < 2; i++) { "
                  "let b = new ArrayBuffer(8 * 1024 * 1024); s += b.byteLength } return s })()", "16777216"),
          "a buffer of each run of a loop");
    JS_SetMemoryLimit(v.rt, (size_t)-1);
    vm_free(&v);
}

/* The index the disassembler prints for an opcode that has it in the opcode,
   the fused reads included (they are apart from the groups of four of
   upstream), against the name of the opcode. */
static void test_short_opcode_index(void)
{
    char bad[96] = "";
    int n = white_short_index_check(bad, sizeof(bad));

    CHECK(n >= 3 * 4 + 3, "the index of %s", bad);
}

/* count_ops: how many of the opcode w the code of the first function of src
   has, or -1 when it does not compile. */
static int count_ops(JSContext *ctx, const char *src, int w)
{
    int shape[2 * 128], n, k, c = 0;
    const uint8_t *code;
    JSValue top;
    int len;

    if (code_of(ctx, src, &top, &code, &len) != 0)
        return -1;
    n = white_code_shape(code, len, shape, 128);
    for (k = 0; k < n; k++)
        c += shape[2 * k] == white_op(w);
    JS_FreeValue(ctx, top);
    return n < 0 ? -1 : c;
}

/* The locals of a function share slots when no run of its code needs two of
   them at once (share_local_slots): the slots each function gets with the
   pass on and off, and what it gives, against Node. A local captured by a
   closure, live where a catch or finally block starts, or in a function
   with a mapped arguments object, a direct eval or a with block keeps its
   own slot. A function with the slots shared and a closure that reads a
   local past them reads back with JS_ReadObject and runs. */
static void test_share_local_slots(void)
{
    static const struct {
        const char *src, *call, *want;
        int on, off; /* the locals of f with the pass on and off */
    } cases[] = {
        /* one block after another */
        { "function f(o) { { const a = o.x; o.r = a } { const b = o.y; o.r += b } { const c = o.z; o.r += c } return o.r }",
          "f({ x: 1, y: 2, z: 3 })", "6", 1, 3 },
        { "function f(o) { const a = o.x, b = o.y; return a + b }", "f({ x: 1, y: 2 })", "3", 2, 2 },
        /* a write of a that nothing reads, where b is live */
        { "function f(o) { let a = o.x; let b = o.y; a = 3; return b }", "f({ x: 1, y: 2 })", "2", 2, 2 },
        /* a read before any write finds undefined: on the path that reads a,
           b is never written */
        { "function f(c) { var a; if (c) { var b = 1; return b } return a }", "f(0) + ',' + f(1)", "undefined,1", 1, 2 },
        /* a read before the write in each turn of a loop */
        { "function f(n) { var r = []; for (var i = 0; i < n; i++) { var a; r.push(a); var b = i; a = b } return r.join() }",
          "f(3)", ",0,1", 3, 4 },
        { "function f(n) { let s = ''; for (let i = 0; i < n; i++) { var t; s += t + ','; t = i } { const u = s.length; s += u } return s }",
          "f(3)", "undefined,0,1,14", 3, 4 },
        /* r is live where the catch starts, and keeps its slot */
        { "function f(o) { var r = 0; try { const t = o.x.y; r = t } catch (e) { return 'caught ' + r } return r }",
          "f({}) + ',' + f({ x: { y: 5 } })", "caught 0,5", 2, 3 },
        /* the finally reads x after the return took it */
        { "function f(o) { let x = o.x; try { return x } finally { o.seen = x; x = 9 } }",
          "(() => { const o = { x: 1 }; return f(o) + ',' + o.seen })()", "1,1", 1, 1 },
        /* captured: a and b keep their slots, c and d share one */
        { "function f(o) { const a = o.x; { const b = o.y; o.g = () => a + b } { const c = o.z; o.h = c } { const d = o.w; o.h += d } return o.g() + o.h }",
          "f({ x: 1, y: 2, z: 3, w: 4 })", "10", 3, 4 },
        /* a closure reads c, whose index goes down */
        { "function f(o) { { const a = o.x; o.r = a } { const b = o.y; o.r += b } const c = o.z; return () => c + o.r }",
          "f({ x: 1, y: 2, z: 3 })()", "6", 2, 3 },
        { "function f(n) { const fs = []; for (let i = 0; i < n; i++) { const k = i * 2; fs.push(() => i + k) } { const z = fs.length; fs.push(z) } return fs.slice(0, n).map(h => h()).join() + ',' + fs[n] }",
          "f(3)", "0,3,6,3", 4, 4 },
        /* a switch whose value is on the stack where a case returns: a is
           not read after b or c is made, so the three share a slot */
        { "function f(v) { const a = 'A' + v; switch (v) { case 1: { const b = a + 'b'; return b } case 2: { const c = a + 'c'; return c } } return a }",
          "f(1) + f(2) + f(3)", "A1bA2cA3", 1, 3 },
        /* a yield and an await in the middle of a range */
        { "function* f(o) { const a = o.x; yield 1; const b = o.y; yield a + b; const c = o.z; yield c }",
          "[...f({ x: 1, y: 2, z: 3 })].join()", "1,3,3", 2, 3 },
        { "async function f(o) { const a = await o.x; const b = a + 1; await null; const c = b * 2; return c }",
          "typeof f", "function", 1, 3 },
        /* the arguments object: mapped, nothing is shared; unmapped, it keeps
           its own slot */
        { "function f(o) { { const a = o.x; o.r = a } { const b = o.y; o.r += b } return arguments.length + o.r }",
          "f({ x: 1, y: 2 })", "4", 3, 3 },
        { "function f(o) { 'use strict'; { const a = o.x; o.r = a } { const b = o.y; o.r += b } return arguments.length + o.r }",
          "f({ x: 1, y: 2 })", "4", 2, 3 },
        /* a direct eval and a with block see the locals */
        { "function f(o) { { const a = o.x; o.r = a } { const b = o.y; o.r += b } return eval('o.r') }",
          "f({ x: 1, y: 2 })", "3", 6, 6 },
        { "function f(o) { { const a = o.x; o.r = a } { const b = o.y; o.r += b } with (o) { return r } }",
          "f({ x: 1, y: 2 })", "3", 3, 3 },
    };
    static const char script[] =
        "(function () { function f(o) { { const a = o.x; o.r = a } { const b = o.y; o.r += b } const c = o.z;\n"
        "    return () => c * 100 + o.r }\n"
        "  return f({ x: 1, y: 2, z: 7 })() })";
    char src[8192], *p;
    VM v = vm_new();
    JSValue top;
    uint8_t *buf;
    size_t size;
    char got[256];
    int i, k, n, on;

    for (on = 1; on >= 0; on--) {
        JS_SetFusedOps(v.ctx, on);
        for (i = 0; i < (int)countof(cases); i++) {
            top = JS_Eval(v.ctx, cases[i].src, strlen(cases[i].src), "<share>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
            n = white_child_var_count(top, 0);
            JS_FreeValue(v.ctx, top);
            CHECK(n == (on ? cases[i].on : cases[i].off), "on %d: %s: %d locals", on, cases[i].src, n);
            snprintf(src, sizeof(src), "%s; %s", cases[i].src, cases[i].call);
            CHECK(eval_is(v.ctx, src, cases[i].want), "on %d: %s: %s", on, cases[i].src, cases[i].call);
        }
    }
    JS_SetFusedOps(v.ctx, 1);
    /* a switch of 40 cases, each with a var of its own, as a renderer of
       React has: the vars take one slot */
    p = src;
    p += sprintf(p, "function f(o, k) { const out = []; switch (k) {");
    for (k = 0; k < 40; k++)
        p += sprintf(p, " case %d: var v%d = o.p%d; out.push(v%d, k); break;", k, k, k, k);
    sprintf(p, " } return out.join() }");
    top = JS_Eval(v.ctx, src, strlen(src), "<share>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    n = white_child_var_count(top, 0);
    JS_FreeValue(v.ctx, top);
    CHECK(n == 2, "a switch of 40 vars: %d locals", n);
    strcat(src, "; f({ p7: 'x', p39: 'y' }, 7) + f({ p7: 'x', p39: 'y' }, 39)");
    CHECK(eval_is(v.ctx, src, "x,7y,39"), "a switch of 40 vars runs");
    /* a check of the temporal dead zone left next to locals that share a
       slot names its own local */
    CHECK(eval_is(v.ctx, "(function (o) { { const a = o.x; o.r = a } { const b = o.y; o.r += b }\n"
                  "  try { if (o.r) return z } catch (e) { return e.message } let z = 1; return z })({ x: 1, y: 2 })",
                  "z is not initialized"), "the name of a local in its dead zone");
    /* written and read back */
    top = JS_Eval(v.ctx, script, strlen(script), "<share>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    JS_FreeValue(v.ctx, top);
    CHECK(buf != NULL, "write");
    if (buf) {
        run_read(v.ctx, buf, size, got, sizeof(got));
        CHECK(strcmp(got, "ok 703") == 0, "read back: %s", got);
        js_free(v.ctx, buf);
    }
    vm_free(&v);
}

/* fused_count: the fused opcodes of the first function of src, by kind
   into count, and in all; -1 when it does not compile. */
static int fused_count(JSContext *ctx, const char *src, int *count, size_t cap)
{
    const uint8_t *code;
    JSValue top;
    int len, n;

    if (code_of(ctx, src, &top, &code, &len) != 0)
        return -1;
    n = white_fused_ops(code, len, count, cap);
    JS_FreeValue(ctx, top);
    return n;
}

/* The places where the fused opcodes meet what is not a plain object or a
   plain binding: an object with [[IsHTMLDDA]] in every typeof test, a module
   namespace, whose exports are references to the variables of the module,
   and an argument or a local that a direct eval or a with block sets just
   before the fused read. Each runs with the opcodes on and off, and on, the
   code must have the opcodes, so that the test does not pass on the path of
   before alone. */
static void test_fused_edges(void)
{
    static const char dda_tests[] =
        "function f(x) { return [typeof x == 'undefined', typeof x === 'object', typeof x != 'undefined',"
        " typeof x !== 'function', typeof x < 'u', typeof x > 'u', typeof x == 'function',"
        " typeof x === 'string', typeof x == 'object', typeof x !== 'undefined'].join() }";
    static const char dda_branches[] =
        "function f(x) { var r = ''; if (typeof x < 'u') r += 'a'; if (typeof x > 'u') r += 'b';"
        " if (typeof x === 'undefined') r += 'c'; if (typeof x !== 'object') r += 'd';"
        " if (typeof x != 'function') r += 'e'; return r }";
    static const char ns_read[] =
        "function f(o) { var r = o.x; o.bump(); return [r, o.x, o.y, typeof o.nope].join() }";
    static const struct {
        const char *src, *call, *want;
        int kind, n; /* the kind of fused read after the eval or the with (in the order of white_fused_ops), and the fused reads in all */
    } bound[] = {
        { "function f(a) { eval('a = { x: 2 }'); return a.x }", "({ x: 1 })", "2", 0, 1 },
        { "function f(a) { eval('var a = { x: 5 }'); return a.x }", "({ x: 1 })", "5", 0, 1 },
        { "function f(a, b) { eval('b = { y: 6 }'); return a.x + b.y }", "({ x: 1 }, { y: 2 })", "7", 1, 2 },
        { "function f() { var o = { x: 1 }; eval('o = { x: 4 }'); return o.x }", "()", "4", 2, 1 },
        { "function f(s) { var a = 0, b = 1, c = 2, d = 3, e = { x: 1 }; eval('e = { x: 8 }'); return e.x }",
          "()", "8", 3, 1 },
        { "function f(a, s) { with (s) { a = { x: 3 } } return a.x }", "({ x: 1 }, {})", "3", 0, 1 },
        { "function f(a, s) { with (s) { a = { x: 3 } } return a.x }", "({ x: 1 }, { a: 0 })", "1", 0, 1 },
        { "function f(a, s) { with (s) { eval('a = { x: 9 }') } return a.x }", "({ x: 1 }, {})", "9", 0, 1 },
    };
    VM v = vm_new();
    JSValue g, dda, mod, ns, r;
    JSModuleDef *m;
    int on, count[8], n;
    size_t i;
    char src[512];

    dda = JS_NewObject(v.ctx);
    JS_SetIsHTMLDDA(v.ctx, dda);
    g = JS_GetGlobalObject(v.ctx);
    JS_SetPropertyStr(v.ctx, g, "dda", dda);

    /* a module of its own, whose namespace the reads get */
    {
        static const char dep[] = "export let x = 7; export const y = 'y'; export function bump() { x++ }";
        mod = JS_Eval(v.ctx, dep, strlen(dep), "dep", JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
        CHECK(!JS_IsException(mod), "compile the module");
        m = JS_VALUE_GET_PTR(mod);
        r = JS_EvalFunction(v.ctx, mod);
        CHECK(!JS_IsException(r), "run the module");
        JS_FreeValue(v.ctx, r);
        ns = JS_GetModuleNamespace(v.ctx, m);
        CHECK(JS_IsObject(ns), "the namespace");
        JS_SetPropertyStr(v.ctx, g, "ns", ns);
    }
    JS_FreeValue(v.ctx, g);

    for (on = 1; on >= 0; on--) {
        JS_SetFusedOps(v.ctx, on);

        /* typeof of an object with [[IsHTMLDDA]] is "undefined" */
        n = count_ops(v.ctx, dda_tests, W_TYPEOF);
        CHECK(on ? n == 0 : n == 8, "on %d: %d typeof left in the tests of dda", on, n);
        n = count_ops(v.ctx, dda_tests, W_TYPEOF_IS) + count_ops(v.ctx, dda_tests, W_TYPEOF_IS_UNDEFINED) +
            count_ops(v.ctx, dda_tests, W_TYPEOF_IS_FUNCTION);
        CHECK(on ? n == 10 : n == 2, "on %d: %d typeof opcodes in the tests of dda", on, n);
        snprintf(src, sizeof(src), "(%s)(dda)", dda_tests);
        CHECK(eval_is(v.ctx, src, "true,false,false,true,false,true,false,false,false,false"), "on %d: dda", on);
        n = count_ops(v.ctx, dda_branches, W_TYPEOF);
        CHECK(on ? n == 0 : n == 3, "on %d: %d typeof left in the branches of dda", on, n);
        snprintf(src, sizeof(src), "(%s)(dda)", dda_branches);
        CHECK(eval_is(v.ctx, src, "bcde"), "on %d: the branches of dda", on);
        CHECK(eval_is(v.ctx, "typeof dda + (dda == undefined) + (dda === undefined)", "undefinedtruefalse"),
              "on %d: dda itself", on);
        /* its truth in the branches and ! of the register loop, from a
           local and from a temp that is not its last reference */
        CHECK(eval_is(v.ctx, "(function (x) { var o = { d: x }, r = []; if (x) r.push('if'); if (o.d) r.push('temp');"
                      " while (o.d) { r.push('loop'); break } r.push(!x, !o.d, x ? 1 : 0, o.d ? 1 : 0,"
                      " (function () { return x })() ? 1 : 0, !{}, ![], !(function () { return {} })()); return r.join() })(dda)",
                      "true,true,0,0,0,false,false,false"),
              "on %d: the truth of dda", on);

        /* the exports of a namespace are read through their references:
           the read after bump() sees the new value */
        n = fused_count(v.ctx, ns_read, count, countof(count));
        CHECK(on ? n == 4 && count[0] == 4 : n == 0, "on %d: %d fused reads of the namespace", on, n);
        snprintf(src, sizeof(src), "(%s)(ns)", ns_read);
        CHECK(eval_is(v.ctx, src, on ? "7,8,y,undefined" : "8,9,y,undefined"), "on %d: the namespace", on);

        /* a direct eval or a with block sets the binding, then the fused read */
        for (i = 0; i < countof(bound); i++) {
            n = fused_count(v.ctx, bound[i].src, count, countof(count));
            CHECK(on ? n == bound[i].n && count[bound[i].kind] == 1 : n == 0,
                  "on %d: %s: %d fused", on, bound[i].src, n);
            snprintf(src, sizeof(src), "(%s)%s", bound[i].src, bound[i].call);
            CHECK(eval_is(v.ctx, src, bound[i].want), "on %d: %s", on, src);
        }
    }
    JS_SetFusedOps(v.ctx, 1);
    vm_free(&v);
}

static void test_object_lit_read(void)
{
    VM v = vm_new();
    const char *src = "(function () { var n = 0; for (var k in { a: 1, b: 2 }) n++; return n })";
    const uint8_t op[5] = { white_op(W_OBJECT_LIT), 2, 0, 0, 0 };
    const uint8_t *code, *at, *lit;
    uint8_t *buf, *copy;
    size_t size;
    JSValue top;
    char got[256];
    int len;

    CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile an object literal");
    lit = memmem(code, len, op, sizeof(op));
    CHECK(lit != NULL, "OP_object_lit 2 0 in the code of an object literal");
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    at = buf ? memmem(buf, size, code, len) : NULL;
    CHECK(at != NULL, "the code of the object literal in what JS_WriteObject wrote");
    run_read(v.ctx, buf, size, got, sizeof(got));
    CHECK(strcmp(got, "ok 2") == 0, "read back: %s", got);
    if (at && lit) {
        copy = malloc(size);
        memcpy(copy, buf, size);
        copy[at - buf + (lit - code) + 1] = 1;
        run_read(v.ctx, copy, size, got, sizeof(got));
        CHECK(strstr(got, "SyntaxError: invalid object literal"), "one value for two keys: %s", got);
        memcpy(copy, buf, size);
        copy[at - buf + (lit - code) + 3] = 1;
        run_read(v.ctx, copy, size, got, sizeof(got));
        CHECK(strstr(got, "SyntaxError: invalid object literal"), "the keys of a second literal: %s", got);
        free(copy);
    }
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);
    vm_free(&v);
}

/* The compiler adds the variable of this, new.target, the home object, the
   active function of a derived class constructor, arguments and the name of
   a function expression the first time code of the function, or of a
   function in it, uses one. When adding it runs out of memory, resolving the
   name fails with the out of memory error. It used to take the failure for a
   function without that variable and look further out: the name became a
   variable of the script, or a global, with the error left pending, and from
   an arrow function, arguments and the name of the function read the
   variable at index -1. The variables a direct eval can use fail the same
   way. With memory, each gets its variable. */
static void test_added_vars_oom(void)
{
    static const char *const kinds[] = {
        "this", "new.target", "home object", "active function", "arguments", "function name",
    };
    int kind, in_arrow, full;

    for (in_arrow = 0; in_arrow < 2; in_arrow++) {
        for (full = 0; full < 2; full++) {
            for (kind = 0; kind <= (int)countof(kinds); kind++) {
                VM v = vm_new();
                const char *what = kind < (int)countof(kinds) ? kinds[kind] : "eval";
                const char *where = in_arrow ? "in an arrow function" : "in the function";
                size_t room = full ? 4096 : (size_t)-1;
                int r, pending, made, unc;

                if (kind < (int)countof(kinds))
                    r = white_resolve_var(v.ctx, kind, in_arrow, room, &pending, &made);
                else
                    r = white_eval_vars(v.ctx, in_arrow, room, &pending, &made);
                unc = JS_IsUncatchableException(v.ctx);
                CHECK(r != -2, "%s %s: the functions could not be made", what, where);
                if (full)
                    CHECK(r == -1 && pending && unc, "%s %s, no memory: returned %d, pending %d, uncatchable %d", what, where, r, pending, unc);
                else
                    CHECK(r >= 0 && !pending && made, "%s %s: returned %d, pending %d, made %d", what, where, r, pending, made);
                free(exception_text(v.ctx));
                fflush(stdout);
                vm_free(&v);
            }
        }
    }
}

/* ---- strings built by appending ---- */

/* A string built by appending to a variable grows in place, in the room its
   copies get: it stays a flat string past JS_STRING_ROPE_SHORT2_LEN, where
   QuickJS makes a rope of it, whatever kind of variable it is in. */
static void test_string_append_flat(void)
{
    static const struct {
        const char *what, *src;
    } ways[] = {
        { "closure variable", "(() => { let s = ''; const add = x => { s += x }; for (let i = 0; i < 20000; i++) add(i % 7 ? 'ab' : '\u00e9'); return s })()" },
        { "argument", "((n, s) => { s = ''; for (let i = 0; i < n; i++) s += i % 7 ? 'ab' : '\u4e2d'; return s })(20000)" },
        { "local", "(() => { const f = i => i % 7 ? 'ab' : 'c'; let s = ''; for (let i = 0; i < 20000; i++) s = s + f(i); return s })()" },
        { "local by add_loc", "(() => { let s = ''; for (let i = 0; i < 20000; i++) s += 'ab'; return s })()" },
    };
    size_t i;

    for (i = 0; i < countof(ways); i++) {
        VM v = vm_new();
        JSValue r = JS_Eval(v.ctx, ways[i].src, strlen(ways[i].src), "<unit>", JS_EVAL_TYPE_GLOBAL);
        JSValue len;

        CHECK(!JS_IsException(r), "%s: threw", ways[i].what);
        CHECK(white_string_is_rope(r) == 0, "%s: built %s", ways[i].what,
              white_string_is_rope(r) == 1 ? "a rope" : "no string");
        len = JS_GetPropertyStr(v.ctx, r, "length");
        CHECK(JS_VALUE_GET_TAG(len) == JS_TAG_INT && JS_VALUE_GET_INT(len) > 30000,
              "%s: built a string of length %d", ways[i].what, JS_VALUE_GET_INT(len));
        JS_FreeValue(v.ctx, len);
        JS_FreeValue(v.ctx, r);
        vm_free(&v);
    }
}

/* A string a variable holds can be an atom too, whose table holds the same
   string without counting it. Appending to it makes a new string: grown in
   place, the atom would change under its hash, and the key it names would no
   longer be the key of its text. Both ways a variable can be appended to in
   place: a closure variable that the add stores into (the stack holds the
   second reference), and a local that add_loc grows (the only reference). */
#define ATOM_TAIL ", with a tail that makes it longer than 64 characters"

static void test_string_append_atom(void)
{
    static const struct {
        const char *what, *holder;
    } ways[] = {
        { "closure variable", "var holder = (() => { let s; return [v => { s = v }, x => (s += x)] })()" },
        { "let local", "var holder = (() => { let held; return [v => { held = v }, x => { let t = held; held = undefined; t += x; return t }] })()" },
        { "var local, add_loc", "var holder = (() => { let held; return [v => { held = v }, x => { var t = held; held = undefined; t += x; return t }] })()" },
    };
    size_t i;

    /* An atom grows in place only into the room at the end of its block.
       Where every block is a request to the system malloc (asan), a block
       has no more room than was asked for, and nothing would be checked. */
    if (white_every_block_is_a_request()) {
        printf("unit-white: test_string_append_atom skipped, every block is a request in this build\n");
        return;
    }
    for (i = 0; i < countof(ways); i++) {
        VM v = vm_new();
        /* longer than JS_STRING_APPEND_MIN, so that an add takes the path of
           JS_ConcatStringAppend */
        JSAtom a = JS_NewAtom(v.ctx, "appended atom" ATOM_TAIL);
        JSValue s = JS_AtomToString(v.ctx, a), g, holder, set, r;

        CHECK(eval_ok(v.ctx, ways[i].holder), "%s: no holder", ways[i].what);
        g = JS_GetGlobalObject(v.ctx);
        holder = JS_GetPropertyStr(v.ctx, g, "holder");
        set = JS_GetPropertyUint32(v.ctx, holder, 0);
        r = JS_Call(v.ctx, set, JS_UNDEFINED, 1, &s);
        JS_FreeValue(v.ctx, r);
        CHECK(white_string_room(v.ctx, s) > 0, "%s: the atom has no room to grow into, the test checks nothing", ways[i].what);
        /* then the variable holds the only reference to the atom's string */
        JS_FreeValue(v.ctx, s);
        JS_FreeAtom(v.ctx, a);
        /* no literal of the atom's text in the scripts: it would hold the atom */
        CHECK(eval_ok(v.ctx, "var k = holder[1]('!')"), "%s: the append threw", ways[i].what);
        CHECK(eval_is(v.ctx, "var o = {}; o[k] = 1; String(o['appended ' + 'atom" ATOM_TAIL "!'])", "1"),
              "%s: the atom was changed in place", ways[i].what);
        JS_FreeValue(v.ctx, set);
        JS_FreeValue(v.ctx, holder);
        JS_FreeValue(v.ctx, g);
        vm_free(&v);
    }
}

/* An empty string on the left of + gives the string on the right, not a copy
   of it, whichever way the add is made. The string is then shared, so an
   append to the result must leave the operand as it was. */
static void test_string_empty_left(void)
{
    static const struct {
        const char *what, *src;
    } ways[] = {
        { "add", "s => '' + s" },
        { "add of a local", "s => { let e = ''; return e + s }" },
        { "add into a local", "s => { let t = ''; t = t + s; return t }" },
        { "concat", "s => ''.concat(s)" },
        { "template", "s => `${s}`" },
    };
    static const char *appends[] = {
        /* add_loc, in place when the local holds the only reference */
        "(s => { let t = '' + s; t += '!'; return [s, t] })",
        /* an add into a local, in place when the string has two references */
        "(s => { let t = '' + s; for (let i = 0; i < 3; i++) t = t + '!'; return [s, t] })",
    };
    /* longer than JS_STRING_APPEND_MIN and in a block with room */
    static const char text[] = "a string of more than sixty-four characters, kept in a variable of the test ";
    size_t i;

    /* JS_STRING_APPEND_MIN of quickjs.c */
    CHECK(strlen(text) >= 64, "the text is too short to be appended to in place");
    for (i = 0; i < countof(ways); i++) {
        VM v = vm_new();
        JSValue f = JS_Eval(v.ctx, ways[i].src, strlen(ways[i].src), "<unit>", JS_EVAL_TYPE_GLOBAL);
        JSValue s = JS_NewString(v.ctx, text), r;

        r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &s);
        CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_STRING, "%s: gave no string", ways[i].what);
        if (JS_VALUE_GET_TAG(r) == JS_TAG_STRING)
            CHECK(JS_VALUE_GET_PTR(r) == JS_VALUE_GET_PTR(s), "%s: copied the string", ways[i].what);
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, s);
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
    for (i = 0; i < countof(appends); i++) {
        VM v = vm_new();
        char src[512], want[256];

        snprintf(src, sizeof src, "var r = %s(['%s'].join('')); r[0] + '|' + r[1]", appends[i], text);
        snprintf(want, sizeof want, "%s|%s%s", text, text, i == 0 ? "!" : "!!!");
        CHECK(eval_is(v.ctx, src, want), "append %zu: the operand changed", i);
        vm_free(&v);
    }
}

/* a + b of two strings in each form of the register loop's add (ADD, ADDL,
   ADDK, ADDKL, KADD, rv_add_str in regvm-ops-basic.h) gives the string the
   generic add gives, JS_ConcatString of the two (white_concat): a rope where
   it makes a rope, a flat string where it makes one, of the same width,
   length and characters. Operands of every kind the engine has: the empty
   string, an atom, flat strings of 8 and 16 bits, short and long, a rope and
   the string of a property key, made again for each add. */
static void test_string_add_kinds(void)
{
    static const char *const kinds[] = {
        "''", "'a'", "'ab'.repeat(2)", "'\\u4e2d'.repeat(2)", "'x'.repeat(600)",
        "'\\u00e9'.repeat(600)", "'y'.repeat(9000)", "'r'.repeat(9000) + 's'.repeat(9000)",
        "Object.keys({ keyName: 1 })[0]", "'p'.repeat(100) + 'q'",
    };
    /* kl and kr: the constant on the left or the right, in place of a or b */
    static const struct {
        const char *what, *add, *kl, *kr;
    } forms[] = {
        { "ADD", "(a, b) => a + b", NULL, NULL },
        { "ADDL", "(a, b) => { let t = 0; t = a + b; return t }", NULL, NULL },
        { "ADDK", "(a, b) => a + '<k>'", NULL, "'<k>'" },
        { "ADDKL", "(a, b) => { let t = 0; t = a + '\\u4e2d'; return t }", NULL, "'\\u4e2d'" },
        { "KADD", "(a, b) => '<k>' + b", "'<k>'", NULL },
        { "KADD into a local", "(a, b) => { let t = 0; t = '\\u4e2d' + b; return t }", "'\\u4e2d'", NULL },
    };
    size_t f, i, j;

    for (f = 0; f < countof(forms); f++) {
        VM v = vm_new();
        JSValue add = JS_Eval(v.ctx, forms[f].add, strlen(forms[f].add), "<unit>", JS_EVAL_TYPE_GLOBAL);

        for (i = 0; i < countof(kinds); i++) {
            for (j = 0; j < countof(kinds); j++) {
                const char *l = forms[f].kl ? forms[f].kl : kinds[i];
                const char *r = forms[f].kr ? forms[f].kr : kinds[j];
                int k;

                /* twice: the first call converts the function to register
                   code, the second runs it there */
                for (k = 0; k < 2; k++) {
                    JSValue args[2], res, want;

                    args[0] = JS_Eval(v.ctx, kinds[i], strlen(kinds[i]), "<unit>", JS_EVAL_TYPE_GLOBAL);
                    args[1] = JS_Eval(v.ctx, kinds[j], strlen(kinds[j]), "<unit>", JS_EVAL_TYPE_GLOBAL);
                    res = JS_Call(v.ctx, add, JS_UNDEFINED, 2, args);
                    JS_FreeValue(v.ctx, args[0]);
                    JS_FreeValue(v.ctx, args[1]);
                    args[0] = JS_Eval(v.ctx, l, strlen(l), "<unit>", JS_EVAL_TYPE_GLOBAL);
                    args[1] = JS_Eval(v.ctx, r, strlen(r), "<unit>", JS_EVAL_TYPE_GLOBAL);
                    want = white_concat(v.ctx, args[0], args[1]);
                    JS_FreeValue(v.ctx, args[0]);
                    JS_FreeValue(v.ctx, args[1]);
                    CHECK(white_string_is_rope(res) >= 0 && white_string_is_rope(want) >= 0,
                          "%s of %s and %s: no string", forms[f].what, l, r);
                    CHECK(white_string_is_rope(res) == white_string_is_rope(want),
                          "%s of %s and %s: a rope is %d, the generic add's %d", forms[f].what, l, r,
                          white_string_is_rope(res), white_string_is_rope(want));
                    CHECK(white_string_wide(res) == white_string_wide(want),
                          "%s of %s and %s: wide is %d, the generic add's %d", forms[f].what, l, r,
                          white_string_wide(res), white_string_wide(want));
                    CHECK(JS_StrictEq(v.ctx, res, want), "%s of %s and %s: not the string of the generic add",
                          forms[f].what, l, r);
                    JS_FreeValue(v.ctx, res);
                    JS_FreeValue(v.ctx, want);
                }
            }
        }
        JS_FreeValue(v.ctx, add);
        vm_free(&v);
    }
}

/* ---- the own keys of an object ---- */

/* objects of every kind of key, with the shapes the listing handles itself,
   keeps in the key cache, and leaves to the generic path */
static const struct { const char *name, *js; } own_keys_objects[] = {
    { "empty", "({})" },
    { "strings", "({a: 1, b: 2, c: 3})" },
    { "indexes", "({b: 1, 10: 2, 2: 3, a: 4, 4294967294: 5, 4294967295: 6, '01': 7})" },
    { "symbols", "({[Symbol.iterator]: 1, a: 2, [Symbol('s')]: 3, 1: 4, [Symbol.for('g')]: 5})" },
    { "hidden", "Object.defineProperties({a: 1}, {h: {value: 2}, [Symbol('h')]: {value: 3}, 3: {value: 4}})" },
    { "accessors", "({get a() { return 1 }, set b(v) {}, c: 3})" },
    { "deleted", "(() => { const o = {a: 1, b: 2, 1: 3, c: 4}; delete o.b; delete o[1]; return o })()" },
    { "private", "(() => { const s = Symbol('s'); class C { #p = 1; x = 2; #q = 3; constructor() { this[s] = 4; this[0] = 5 } }; return new C })()" },
    { "null proto", "Object.assign(Object.create(null), {z: 1, 0: 2, y: 3})" },
    { "frozen", "Object.freeze({a: 1, 2: 2})" },
    { "wide", "({'\\u00e9': 1, '\\u4e2d': 2, '1\\u4e2d': 3})" },
    { "many strings", "Object.fromEntries(Array.from({length: 40}, (_, i) => ['k' + i, i]))" },
    { "many indexes", "Object.fromEntries(Array.from({length: 20}, (_, i) => [String(19 - i), i]))" },
    { "function", "(function f(a, b) {})" },
    { "class", "(class K { static s = 1; static m() {} })" },
    { "error", "new Error('m')" },
    { "array", "Object.assign([1, 2], {x: 3})" },
    { "arguments", "(function () { return arguments })(1, 2)" },
    { "string object", "Object.assign(new String('ab'), {x: 1})" },
    { "typed array", "new Uint8Array(3)" },
    { "proxy", "new Proxy({a: 1}, {})" },
};

/* The flags of JS_GetOwnPropertyNames. */
#define OWN_KEYS_ALL_FLAGS (JS_GPN_STRING_MASK | JS_GPN_SYMBOL_MASK | JS_GPN_PRIVATE_MASK | JS_GPN_ENUM_ONLY | JS_GPN_SET_ENUM)

/* For every combination of flags, JS_GetOwnPropertyNames lists the own keys
   of an ordinary object as the engine does from its shape, also the second
   time, when the kinds come from the key cache, and lists those of exotic
   objects without failing. */
static void test_own_keys_flags(void)
{
    VM v = vm_new();
    static char got[8192], want[8192];
    size_t i;

    for (i = 0; i < countof(own_keys_objects); i++) {
        JSValue obj = JS_Eval(v.ctx, own_keys_objects[i].js, strlen(own_keys_objects[i].js),
                              "<unit>", JS_EVAL_TYPE_GLOBAL);
        int flags, pass, exotic = 0;

        CHECK(JS_IsObject(obj), "%s: %s is no object", own_keys_objects[i].name, own_keys_objects[i].js);
        if (!JS_IsObject(obj)) {
            free(exception_text(v.ctx));
            continue;
        }
        for (flags = 0; flags <= OWN_KEYS_ALL_FLAGS; flags++) {
            if (flags & ~OWN_KEYS_ALL_FLAGS)
                continue;
            for (pass = 0; pass < 2; pass++) {
                int ng = white_own_keys(v.ctx, obj, flags, 1, got, sizeof(got));
                int nw = white_own_keys(v.ctx, obj, flags, 0, want, sizeof(want));

                exotic = nw == -2;
                CHECK(ng >= 0, "%s, flags %#x: JS_GetOwnPropertyNames throws", own_keys_objects[i].name, flags);
                if (!exotic)
                    CHECK(ng == nw && strcmp(got, want) == 0,
                          "%s, flags %#x:\ngot  %s\nwant %s", own_keys_objects[i].name, flags, got, want);
            }
        }
        JS_FreeValue(v.ctx, obj);
    }
    vm_free(&v);
}

/* ---- source text and line tables shared with a buffer ---- */

/* With JS_READ_OBJ_SHARED_DEBUG the source text and line table of a function
   point into the buffer read, the function says so, the runtime neither
   counts nor frees them, and they hold what a copy holds. */
static void test_shared_debug_read(void)
{
    enum { PAD = 64 << 10 };
    static const char head[] = "function f(a) {\n\t/*", tail[] = "*/\n\treturn new Error(a).stack\n}\n";
    char *js = malloc(sizeof(head) + PAD + sizeof(tail));
    VM c = vm_new();
    JSValue compiled;
    uint8_t *out, *buf;
    size_t n;
    char *sources[2] = { NULL, NULL };
    JSMemoryUsage usage[2];
    int i;

    memcpy(js, head, sizeof(head) - 1);
    memset(js + sizeof(head) - 1, 'y', PAD);
    strcpy(js + sizeof(head) - 1 + PAD, tail);
    compiled = JS_Eval(c.ctx, js, strlen(js), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(compiled), "the script did not compile");
    out = JS_WriteObject(c.ctx, &n, compiled, JS_WRITE_OBJ_BYTECODE);
    CHECK(out != NULL, "the bytecode was not written");
    buf = malloc(n);
    memcpy(buf, out, n);
    js_free(c.ctx, out);
    JS_FreeValue(c.ctx, compiled);
    vm_free(&c);
    free(js);

    for (i = 0; i < 2; i++) {
        int flags = JS_READ_OBJ_BYTECODE | (i ? JS_READ_OBJ_SHARED_DEBUG : 0);
        int shared = i, pooled, f_shared;
        const uint8_t *src, *pc;
        size_t src_len;
        VM v = vm_new();
        JSValue script = JS_ReadObject(v.ctx, buf, n, flags);
        int in_src, in_pc;

        CHECK(JS_VALUE_GET_TAG(script) == JS_TAG_FUNCTION_BYTECODE, "the bytecode was not read");
        pooled = white_function_debug(script, 0, &src, &src_len, &pc, &f_shared);
        CHECK(pooled == 1, "the script has %d constants, want f alone", pooled);
        in_src = src >= buf && src < buf + n;
        in_pc = pc >= buf && pc < buf + n;
        CHECK(in_src == shared && in_pc == shared && f_shared == shared,
              "flags %d: source in the buffer %d, line table %d, bit %d", flags, in_src, in_pc, f_shared);
        sources[i] = strndup((const char *)src, src_len);
        JS_ComputeMemoryUsage(v.rt, &usage[i]);
        /* freeing the functions must leave the buffer to its owner */
        JS_FreeValue(v.ctx, script);
        vm_free(&v);
    }
    CHECK(strcmp(sources[0], sources[1]) == 0 && strncmp(sources[0], "function f(a) {", 15) == 0,
          "source %.40s, shared %.40s", sources[0], sources[1]);
    CHECK(usage[1].js_func_pc2line_size == 0, "the runtime counts %lld bytes of shared line tables",
          (long long)usage[1].js_func_pc2line_size);
    CHECK(usage[0].malloc_size - usage[1].malloc_size >= PAD,
          "shared, the runtime takes %lld bytes less, want at least %d",
          (long long)(usage[0].malloc_size - usage[1].malloc_size), PAD);
    CHECK(usage[0].js_func_size - usage[1].js_func_size >= PAD,
          "shared, the runtime counts %lld bytes less in its functions, want at least %d",
          (long long)(usage[0].js_func_size - usage[1].js_func_size), PAD);
    free(sources[0]);
    free(sources[1]);
    free(buf);
}

/* ---- the regexp cache ---- */

/* The entries the cache of the runtime keeps, newest first (see
   white_regexp_cache), with its lists checked. */
static char *cache_entries(VM *v)
{
    static char out[4096];
    size_t bytes;
    uint32_t buckets;
    int n = white_regexp_cache(v->rt, out, sizeof(out), &bytes, &buckets);

    CHECK(n >= 0, "the lists of the regexp cache are not whole");
    return out;
}

static int cache_count(VM *v)
{
    char out[4096];
    size_t bytes;
    uint32_t buckets;

    return white_regexp_cache(v->rt, out, sizeof(out), &bytes, &buckets);
}

/* Only a pattern that is a string with flags that are undefined or a string
   is kept: anything else is converted again each time, which can run
   javascript. A compile that fails keeps nothing. */
static void test_regexp_cache_keeps(void)
{
    static const char *const fails[] = {
        "new RegExp('(')", "new RegExp('a', 'gg')", "new RegExp('a', 'uv')",
        "new RegExp('a', 'q')", "new RegExp('\\\\p{Nope}', 'u')",
    };
    VM v = vm_new();
    char src[256];
    int before, i, again;

    eval_ok(v.ctx, "var n = 0, f = {toString() { n++; return 'g' }}, p = {toString() { n++; return 'x' }}; 0");
    for (i = 0; i < 3; i++)
        eval_ok(v.ctx, "new RegExp('a', f); new RegExp(p, 'g'); new RegExp(new String('b')); new RegExp('c', new String('i')); 0");
    CHECK(eval_is(v.ctx, "String(n)", "6"), "toString did not run 6 times");
    /* The constructor converts a pattern that is not a regexp to a string
       before it compiles: that string is kept. Flags it hands on as they are. */
    CHECK(strcmp(cache_entries(&v), "b undefined,x g") == 0, "entries %s, want b undefined,x g", cache_entries(&v));
    before = cache_count(&v);
    for (i = 0; i < (int)countof(fails); i++) {
        for (again = 0; again < 2; again++) {
            snprintf(src, sizeof(src), "try { %s; 'made' } catch (e) { e.name }", fails[i]);
            CHECK(eval_is(v.ctx, src, "SyntaxError"), "%s did not throw a SyntaxError", fails[i]);
        }
    }
    CHECK(cache_count(&v) == before, "%d entries after compiles that fail, want %d", cache_count(&v), before);
    vm_free(&v);
}

/* The entries, their table and the strings they hold are the memory the cache
   takes: turning it off gives all of it back, and freeing a runtime with
   entries leaves no block (the host ends the process if one is left). */
static void test_regexp_cache_memory(void)
{
    static const char js[] = "for (var i = 0; i < 300; i++) new RegExp('(a|b)+c' + i, 'g'); 0";
    VM v = vm_new();
    VM w = vm_new();
    char out[64];
    size_t off, on, kept, bytes;
    uint32_t buckets;
    int n;

    JS_SetRegExpCache(v.rt, 0, 0);
    eval_ok(v.ctx, js);
    JS_RunGC(v.rt);
    off = JS_GetMemorySize(v.rt);
    JS_SetRegExpCache(v.rt, 256, 512 << 10);
    eval_ok(v.ctx, js);
    JS_RunGC(v.rt);
    on = JS_GetMemorySize(v.rt);
    n = white_regexp_cache(v.rt, out, sizeof(out), &bytes, &buckets);
    CHECK(n == 256 && on > off, "%d entries, %zu bytes with the cache, %zu without", n, on, off);
    /* The cache counts the usable size of each block: the allocator rounds
       up and adds a header, which the memory size has too. */
    kept = on - off;
    CHECK(kept >= bytes / 2 && kept <= bytes * 2 + (16 << 10), "the cache takes %zu bytes and counts %zu", kept, bytes);
    JS_SetRegExpCache(v.rt, 0, 0);
    JS_RunGC(v.rt);
    CHECK(JS_GetMemorySize(v.rt) == off, "%zu bytes after the cache let go, %zu before it", JS_GetMemorySize(v.rt), off);
    vm_free(&v);
    /* the default cache, freed with its entries */
    eval_ok(w.ctx, js);
    vm_free(&w);
}

/* ---- regexps that nothing changed ---- */

/* js_is_standard_regexp says yes for a regexp of the realm as the constructor
   made it, with exec of RegExp.prototype not made yet and made, and no after
   each change a script can make to what replace and match read: the getter of
   flags or of each of the 8 flags, exec, the getter or exec of another realm,
   the getter of another flag, lastIndex, the prototype of the regexp, a
   property of its own. Once the built-in is put back at its place, yes again.
   A property deleted and added again sits at another place, and the answer
   stays no. */
static void test_regexp_standard(void)
{
    static const char *const flags[] = {
        "flags", "global", "ignoreCase", "multiline", "dotAll", "unicode", "unicodeSets", "sticky", "hasIndices",
    };
    static const struct { const char *change, *undo; } changes[] = {
        { "P.exec = function (s) { return d.exec.value.call(this, s) }", "P.exec = d.exec.value" },
        { "Object.defineProperty(P, 'exec', { get: function () { return d.exec.value } })", "Object.defineProperty(P, 'exec', d.exec)" },
        { "P.exec = exec2", "P.exec = d.exec.value" },
        { "Object.defineProperty(P, 'sticky', { get: sticky2 })", "Object.defineProperty(P, 'sticky', d.sticky)" },
        { "Object.defineProperty(P, 'sticky', d.global)", "Object.defineProperty(P, 'sticky', d.sticky)" },
        { "Object.defineProperty(P, 'global', { value: true })", "Object.defineProperty(P, 'global', d.global)" },
        { "re.lastIndex = { valueOf() { return 0 } }", "re.lastIndex = 0" },
        { "re.lastIndex = '1'", "re.lastIndex = 1.5" },
        { "re.lastIndex = undefined", "re.lastIndex = -1" },
    };
    static const char *const others[] = {
        "Object.setPrototypeOf(/a/, Object.create(RegExp.prototype))", "Object.assign(/a/, { x: 1 })",
        "Object.defineProperty(/a/, 'lastIndex', { writable: false })", "Object.freeze(/a/g)",
        "Object.defineProperty(Object.create(RegExp.prototype), 'lastIndex', { value: 0, writable: true })",
        "new (class extends RegExp {})('a')", "re2", "new Proxy(/a/, {})", "({})",
    };
    VM v = vm_new();
    JSContext *c2 = JS_NewContext(v.rt);
    JSValue g = JS_GetGlobalObject(v.ctx);
    char src[256];
    size_t i;

    CHECK(c2 != NULL, "no second context");
    eval_ok(v.ctx, "var P = RegExp.prototype, re = /a/g, re0 = new RegExp('a', 'dgimsuy'); 0");
    CHECK(white_regexp_standard(v.ctx, "re") == 1, "a regexp before exec is made");
    CHECK(white_regexp_standard(v.ctx, "re0") == 1, "a regexp of every flag before exec is made");
    eval_ok(v.ctx, "var d = {}; for (var k of ['exec', 'flags', 'global', 'ignoreCase', 'multiline', 'dotAll', 'unicode',"
            " 'unicodeSets', 'sticky', 'hasIndices']) d[k] = Object.getOwnPropertyDescriptor(P, k); 0");
    CHECK(white_regexp_standard(v.ctx, "re") == 1, "a regexp once exec is made");
    JS_SetPropertyStr(v.ctx, g, "exec2", JS_Eval(c2, "RegExp.prototype.exec", strlen("RegExp.prototype.exec"), "<c2>", JS_EVAL_TYPE_GLOBAL));
    JS_SetPropertyStr(v.ctx, g, "sticky2", JS_Eval(c2, "Object.getOwnPropertyDescriptor(RegExp.prototype, 'sticky').get",
                                                    strlen("Object.getOwnPropertyDescriptor(RegExp.prototype, 'sticky').get"), "<c2>", JS_EVAL_TYPE_GLOBAL));
    JS_SetPropertyStr(v.ctx, g, "re2", JS_Eval(c2, "/a/g", strlen("/a/g"), "<c2>", JS_EVAL_TYPE_GLOBAL));
    for (i = 0; i < countof(flags); i++) {
        snprintf(src, sizeof(src), "Object.defineProperty(P, '%s', { get: function () { return d.%s.get.call(this) } }); 0",
                 flags[i], flags[i]);
        eval_ok(v.ctx, src);
        CHECK(white_regexp_standard(v.ctx, "re") == 0, "the getter of %s replaced", flags[i]);
        snprintf(src, sizeof(src), "Object.defineProperty(P, '%s', d.%s); 0", flags[i], flags[i]);
        eval_ok(v.ctx, src);
        CHECK(white_regexp_standard(v.ctx, "re") == 1, "the getter of %s put back", flags[i]);
    }
    for (i = 0; i < countof(changes); i++) {
        snprintf(src, sizeof(src), "%s; 0", changes[i].change);
        eval_ok(v.ctx, src);
        CHECK(white_regexp_standard(v.ctx, "re") == 0, "after %s", changes[i].change);
        snprintf(src, sizeof(src), "%s; 0", changes[i].undo);
        eval_ok(v.ctx, src);
        CHECK(white_regexp_standard(v.ctx, "re") == 1, "after %s", changes[i].undo);
    }
    for (i = 0; i < countof(others); i++) {
        snprintf(src, sizeof(src), "var o = %s; 0", others[i]);
        eval_ok(v.ctx, src);
        CHECK(white_regexp_standard(v.ctx, "o") == 0, "%s", others[i]);
    }
    eval_ok(v.ctx, "delete P.exec; P.exec = d.exec.value; 0");
    CHECK(white_regexp_standard(v.ctx, "re") == 0, "exec deleted and added again");
    JS_FreeValue(v.ctx, g);
    JS_FreeContext(c2);
    vm_free(&v);
}

/* ---- the split ops of the regexp machine ---- */

/* lre_split steps over at most LRE_SPLIT_BUDGET split ops in one call, and
   then gives back the one it is at to be pushed, so a chain of splits that
   leads back to itself, which lre_compile does not write, cannot hold it. An
   alarm ends the test if it does. */
static void test_regexp_split_budget(void)
{
    int ok;

    alarm(20);
    ok = white_lre_split_budget();
    alarm(0);
    CHECK(ok, "lre_split gave other than the split at 0 and its char at 5");
}

/* alts writes n alternatives of two letters at p, between bars, each first
   written as pre (a string of UTF-8), and returns where it ended. Several of
   them share a first letter. */
static char *alts(char *p, int n, const char *pre)
{
    for (int k = 0; k < n; k++)
        p += sprintf(p, "%s%s%c%c", k ? "|" : "", pre, 'a' + k % 9, 'a' + k % 26);
    return p;
}

/* lre_chains keeps a chain for a run of at least LRE_CHAIN_MIN split ops in
   a row whose ways start with a literal of one kind, 16 alternatives and up.
   A way that starts with anything else, or with a literal of the other kind,
   ends the run, the ways of a lookbehind start with REOP_prev and make none,
   and it keeps at most LRE_CHAIN_MAX chains, the first in the code. */
static void test_regexp_chains(void)
{
    static const struct {
        int n1;            /* alternatives, then */
        const char *mid;   /* this alternative when not NULL, then */
        int n2;            /* these many more */
        const char *pre;   /* the start of each, (?:...) around all of them */
        int flags, groups; /* LRE_FLAG_ bits, and how many groups of these */
        int count;
        const char *want;
    } cases[] = {
        {15, NULL, 0, "", 0, 1, 0, "-"},
        {16, NULL, 0, "", 0, 1, 1, "15c"},
        {17, NULL, 0, "", 0, 1, 1, "16c"},
        {149, NULL, 0, "", 0, 1, 1, "148c"},
        {16, NULL, 0, "", 2, 1, 1, "15f"},        /* i */
        {16, NULL, 0, "", 2 | 16, 1, 1, "15f"},   /* i and u */
        {16, NULL, 0, "", 16, 1, 1, "15c"},       /* u */
        {16, NULL, 0, "\xc4\x80", 0, 1, 1, "15c"},
        {16, NULL, 0, "\xf0\x9f\x98\x80", 16, 1, 1, "15c"},
        {10, "[x]", 10, "", 0, 1, 0, "-"},
        {16, "[x]", 16, "", 0, 1, 2, "16c 15c"},
        {16, "", 16, "", 0, 1, 2, "16c 15c"},
        {16, ".", 16, "", 0, 1, 2, "16c 15c"},
        {16, "\\b", 16, "", 0, 1, 2, "16c 15c"},
        {16, "(ab)", 16, "", 0, 1, 1, "32c"},
        {16, NULL, 0, "", 0, 5, 4, "15c 15c 15c 15c"},
        {16, NULL, 0, "(?<=", 0, 1, 0, "-"},
    };
    VM v = vm_new();
    char re[2048], out[64];

    for (size_t i = 0; i < countof(cases); i++) {
        char *p = re;
        int lookbehind = !strcmp(cases[i].pre, "(?<=");
        for (int g = 0; g < cases[i].groups; g++) {
            p += sprintf(p, lookbehind ? "(?<=" : "(?:");
            p = alts(p, cases[i].n1, lookbehind ? "" : cases[i].pre);
            if (cases[i].mid) {
                p += sprintf(p, "|%s|", cases[i].mid);
                p = alts(p, cases[i].n2, cases[i].pre);
            }
            p += sprintf(p, ")");
        }
        int n = white_lre_chains(re, cases[i].flags, v.ctx, out, sizeof(out));
        CHECK(n == cases[i].count && !strcmp(out, cases[i].want),
              "/%.40s.../ flags %d: %d chains \"%s\", want %d \"%s\"", re, cases[i].flags, n, out,
              cases[i].count, cases[i].want);
    }
    vm_free(&v);
}

/* The literal every match of a regexp holds (lre_plan_literal, see "A literal
   every match holds" in engine/libregexp.c), for each rule of what makes it
   required (white_lre_literal): the run of char ops that no jump from before
   it crosses, the longest, the first of the longest, at most 16 units, with
   the saves of captures inside it, never inside an alternative, an optional
   group, a quantifier of 0, a lookahead or a lookbehind, ended by a back
   reference, a char32, a class, a dot or a loop, under i only the ASCII
   characters and not k and s under u and v, none for a sticky regexp, one
   that starts with ^ or the literal prefix lre_skip compares, and the most
   units a match reads before it. */
static void test_regexp_literal(void)
{
    enum { I = 2, M = 4, S = 8, U = 16, Y = 32, V = 256 };
    static const struct {
        const char *re;
        int flags;
        const char *want;
    } cases[] = {
        /* the regexp of the V8 RegExp benchmark */
        { "(^|[^\\\\])\"\\\\\\/Qngr\\((-?[0-9]+)\\)\\\\\\/\"", 0, "\"\\/Qngr( key 3 before 1" },
        /* the literal prefix of LRE_PLAN_CHAR is not taken, a longer run is */
        { "abc", 0, "-" },
        { "abc", U, "abc key 1 before ?" },
        { "a(b)c", 0, "abc key 1 before 0" },
        { "x(?:abc)+d", 0, "d key 0 before ?" },
        { "ab{0}c", 0, "-" },
        { "abc$", 0, "-" },
        /* alternatives, optional groups, quantifiers */
        { "xa|ya", 0, "-" },
        { "(?:foo|foo)", 0, "-" },
        { "(?:xa|ya)b", 0, "b key 0 before 2" },
        { "(?:abc)?d", 0, "d key 0 before 3" },
        { "(?:abc)*d", 0, "d key 0 before ?" },
        { "(?:abc)+d", 0, "d key 0 before ?" },
        { ".(?:abc){2}d", 0, "abc key 1 before 1" },
        { "a{2}bc", 0, "bc key 0 before ?" },
        { "ab?c", 0, "c key 0 before 2" },
        { ".{0,3}abc", 0, "abc key 1 before ?" },
        { ".?.?abc", 0, "abc key 1 before 2" },
        { "a(?:b|c)*d", 0, "d key 0 before ?" },
        { "(?:a|b)xy(?:c|d)xyz", 0, "xyz key 2 before 4" },
        { "(?:a|b)xyz(?:c|d)xyz", 0, "xyz key 2 before 1" },
        /* after a chain of 15 split ops, which the plan holds first */
        { "(?:a1|b1|c1|d1|e1|f1|g1|h1|i1|j1|k1|l1|m1|n1|o1|p1)xyz", 0, "xyz key 2 before 2" },
        /* lookarounds */
        { "a(?=bc)", 0, "-" },
        { "(?=abc)", 0, "-" },
        { ".(?=bc)d", 0, "d key 0 before 1" },
        { ".(?!bc)d", 0, "d key 0 before 1" },
        { ".(?<=bc)d", 0, "d key 0 before 1" },
        { ".(?<!bc)d", 0, "d key 0 before 1" },
        { "(?<=abc)", 0, "-" },
        { "(?!foo)bar", 0, "bar key 0 before 0" },
        { "(?<=foo)bar", 0, "bar key 0 before 0" },
        /* back references and named groups */
        { ".(a)\\1b", 0, "a key 0 before 1" },
        { "(?<n>ab)c\\k<n>", 0, "abc key 1 before 0" },
        /* ignore case */
        { "abc", I, "~a~b~c key 1 before 0" },
        { "a-b", I, "~a-~b key 2 before 0" },
        { "k1", I, "~k1 key 0 before 0" },
        { "k1", I | U, "1 key 0 before ?" },
        { "s1", I | V, "1 key 0 before ?" },
        { "K1", I | U, "1 key 0 before ?" },
        { ".x\xc3\xa9y", I, "~x key 0 before 1" },
        { "x(?i:ab)y", 0, "x~a~by key 0 before 0" },
        { "x(?-i:ab)y", I, "~xab~y key 0 before 0" },
        /* unicode, lone surrogates, characters from 256 up */
        { "\xf0\x9f\x98\x80x", U, "x key 0 before ?" },
        { "\\ud83dx", U, "\\ud83dx key 0 before ? wide" },
        { "a\xe4\xb8\xad" "b", 0, "-" },
        { ".a\xe4\xb8\xad" "b", 0, "a\\u4e2db key 1 before 1 wide" },
        /* sticky, ^, other assertions */
        { ".abc", Y, "-" },
        { "^abc", 0, "-" },
        { "^abc", M, "abc key 1 before 0" },
        { "\\babc", 0, "abc key 1 before 0" },
        { ".a\\nb", S, "a\\u000ab key 1 before 1" },
        /* at most 16 units */
        { ".abcdefghijklmnopqrstuvwxyz", 0, "abcdefghijklmnop key 9 before 1" },
    };
    VM v = vm_new();
    char out[256];
    static char big[60000];

    for (size_t i = 0; i < countof(cases); i++) {
        int n = white_lre_literal(cases[i].re, cases[i].flags, v.ctx, out, sizeof(out));
        CHECK(n >= 0 && !strcmp(out, cases[i].want), "/%s/ flags %d: \"%s\", want \"%s\"",
              cases[i].re, cases[i].flags, out, cases[i].want);
    }
    /* a literal past the bytes of code the walk reads (LRE_SCAN_STEP) is not
       found, and one before it is */
    for (int k = 0; k < 6000; k++)
        memcpy(big + 4 * k, "[ab]", 4);
    strcpy(big + 4 * 6000, "zz");
    white_lre_literal(big, 0, v.ctx, out, sizeof(out));
    CHECK(!strcmp(out, "-"), "6000 classes then zz: \"%s\", want \"-\"", out);
    memmove(big + 3, big, 4 * 6000 + 3);
    memcpy(big, ".zq", 3);
    white_lre_literal(big, 0, v.ctx, out, sizeof(out));
    CHECK(!strcmp(out, "zq key 1 before 1"), "zq then 6000 classes: \"%s\"", out);
    vm_free(&v);
}

/* The plan after the bytecode of a regexp (lre_plan) is 36 bytes when the
   regexp has no literal every match holds, as before there were literals,
   and only one with a literal carries it, 6 bytes and 2 for each unit, after
   the chains (white_lre_plan_size). The group names follow it. */
static void test_regexp_plan_size(void)
{
    enum { Y = 32 };
    static const struct {
        const char *re;
        int flags, size;
        const char *names;
    } cases[] = {
        { "[a-z]+", 0, 36, "" },
        { "abc", 0, 36, "" },
        { ".abc", Y, 36, "" },
        { "xa|ya", 0, 36, "" },
        { "[a-z](?<n>[0-9])", 0, 36, "n" },
        { ".xyz", 0, 36 + 6 + 2 * 3, "" },
        { "[a-z](?<n>x)yz", 0, 36 + 6 + 2 * 3, "n" },
        { ".abcdefghijklmnopqrstuvwxyz", 0, 36 + 6 + 2 * 16, "" },
        /* a chain of 15 split ops, then the literal */
        { "(?:a1|b1|c1|d1|e1|f1|g1|h1|i1|j1|k1|l1|m1|n1|o1|p1)xyz", 0,
          36 + 8 + 2 * (257 + 15) + 6 + 2 * 3, "" },
        { "(?:a1|b1|c1|d1|e1|f1|g1|h1|i1|j1|k1|l1|m1|n1|o1|p1)(?<n>[0-9])", 0,
          36 + 8 + 2 * (257 + 15), "n" },
    };
    VM v = vm_new();
    char names[16];

    for (size_t i = 0; i < countof(cases); i++) {
        int rest, size = white_lre_plan_size(cases[i].re, cases[i].flags, v.ctx, &rest, names, sizeof(names));
        CHECK(size == cases[i].size && (rest == 0) == !cases[i].names[0] && !strcmp(names, cases[i].names),
              "/%s/ flags %d: plan %d, %d bytes after it, names \"%s\", want plan %d, names \"%s\"",
              cases[i].re, cases[i].flags, size, rest, names, cases[i].size, cases[i].names);
    }
    vm_free(&v);
}

/* The units a match can start with (lre_first_set), as the plan of lre_skip
   keeps them (white_lre_plan): \s and \S start a match at the units the
   machine takes for them (lre_is_space: 9 to 13, 32 and 160 below 256, and
   units from 256 up), alone, after an optional part, in alternatives and
   after a lookbehind, with i, and not with u or y, which keep the loop. A
   class, alternatives of characters and a dot are as before. */
static void test_regexp_first_set(void)
{
    enum { I = 2, U = 16, Y = 32 };
    static const struct {
        const char *re;
        int flags;
        const char *want;
    } cases[] = {
        { "\\s+", 0, "set high 9-13 32 160" },
        { "\\s", I, "set high 9-13 32 160" },
        { "\\S", 0, "set high 0-8 14-31 33-159 161-255" },
        { "\\S|a", 0, "set high 0-8 14-31 33-159 161-255" },
        /* the regexp of a split of the V8 RegExp benchmark */
        { "\\s?;\\s?", 0, "set high 9-13 32 59 160" },
        { "\\s*x", 0, "set high 9-13 32 120 160" },
        { "(?:^|\\s+)x(?:\\s+|$)", 0, "set high 9-13 32 120 160" },
        { "(?<=a)\\s", 0, "set high 9-13 32 160" },
        { "\\s", U, "loop" },
        { "\\s", Y, "loop" },
        { "[ab]", 0, "range 97-98" },
        { "a|b", 0, "set 97-98" },
        { ".x", 0, "loop" },
    };
    VM v = vm_new();
    char out[256];

    for (size_t i = 0; i < countof(cases); i++) {
        int n = white_lre_plan(cases[i].re, cases[i].flags, v.ctx, out, sizeof(out));
        CHECK(n >= 0 && !strcmp(out, cases[i].want), "/%s/ flags %d: \"%s\", want \"%s\"",
              cases[i].re, cases[i].flags, out, cases[i].want);
    }
    vm_free(&v);
}

/* lre_split with the chains gives the machine what the walk over the split
   ops one at a time gives it (white_lre_split_same), at every split op it can
   be called at, on each character of a list and at the end of the string:
   chains of each kind under the flags i, u, iu and v, first characters that
   fold to one another and from 256 up, a chain in a loop, runs that a way of
   another kind ends, more chains than are kept, and chains of 4095 to 4097
   split ops followed by a split op or not, past the steps of one call. */
static void test_regexp_split_same(void)
{
    static const uint32_t units[] = {
        'a', 'b', 'k', 'K', 's', 'S', 'z', 'Z', 'x', 'p', 0xb5, 0xff, 0x100,
        0x178, 0x17f, 0x39c, 0x212a, 0x4e2d, 0x1f600,
    }, few[] = { 'a', 'z' };
    static const char fold[] = "(?:k1|K2|\xe2\x84\xaa" "3|\xc5\xbf" "4|s5|S6|\xc3\xbf" "7|\xc5\xb8" "8|"
        "\xc2\xb5" "9|\xce\x9c" "0|ka|kb|sa|sb|\xc4\x80" "c|zz)";
    /* the same with one more, above U+FFFF, which a pattern of UTF-8 holds only under u and v */
    static const char fold_u[] = "(?:k1|K2|\xe2\x84\xaa" "3|\xc5\xbf" "4|s5|S6|\xc3\xbf" "7|\xc5\xb8" "8|"
        "\xc2\xb5" "9|\xce\x9c" "0|\xf0\x9f\x98\x80" "a|ka|kb|sa|sb|\xc4\x80" "c|zz)";
    static const int flags[] = { 0, 2, 16, 2 | 16, 256 };
    static char re[10000];
    VM v = vm_new();
    long calls = 0;
    int bad = 0, d;
    char *p;

    for (size_t f = 0; f < countof(flags); f++) {
        const char *pre[] = { "", "\xc4\x80" };
        for (size_t k = 0; k < countof(pre); k++) {
            for (int n = 15; n <= 17; n++) {
                p = re + sprintf(re, "(?:");
                p = alts(p, n, pre[k]);
                sprintf(p, ")");
                d = white_lre_split_same(re, flags[f], v.ctx, units, countof(units), &calls);
                CHECK(d == 0, "/%.40s/ flags %d: %d calls differ", re, flags[f], d);
                bad += d != 0;
            }
        }
        d = white_lre_split_same(flags[f] & (16 | 256) ? fold_u : fold, flags[f], v.ctx, units, countof(units), &calls);
        CHECK(d == 0, "/%s/ flags %d: %d calls differ", flags[f] & (16 | 256) ? fold_u : fold, flags[f], d);
        bad += d != 0;
    }
    {
        static const struct {
            int n1;
            const char *mid;
            int n2, groups, flags;
        } shapes[] = {
            {149, NULL, 0, 1, 0},
            {149, NULL, 0, 1, 2},
            {16, "[x]", 16, 1, 0},
            {16, "", 16, 1, 0},
            {16, "(?i:ka|kb|kc|kd|ke|kf|kg|kh|ki|kj|kk|kl|km|kn|ko|kp)", 16, 1, 0},
            {16, NULL, 0, 5, 0},
        };
        for (size_t i = 0; i < countof(shapes); i++) {
            p = re;
            for (int g = 0; g < shapes[i].groups; g++) {
                p += sprintf(p, "(?:");
                p = alts(p, shapes[i].n1, "");
                if (shapes[i].mid) {
                    p += sprintf(p, "|%s|", shapes[i].mid);
                    p = alts(p, shapes[i].n2, "");
                }
                p += sprintf(p, ")");
            }
            d = white_lre_split_same(re, shapes[i].flags, v.ctx, units, countof(units), &calls);
            CHECK(d == 0, "/%.40s/ flags %d: %d calls differ", re, shapes[i].flags, d);
            bad += d != 0;
        }
    }
    d = white_lre_split_same("(?:a|b|c|d|e|f|g|h|i|j|k|l|m|n|o|p|aZ)+", 0, v.ctx, units, countof(units), &calls);
    CHECK(d == 0, "the loop of a chain: %d calls differ", d);
    bad += d != 0;
    /* (?:(?:z|[q])|z|...|z): its split ops are a chain, and past it is the
       split op of z|[q], whose way does not start with a literal; then
       z|...|z, past which is a char */
    for (int n = 4094; n <= 4097; n++) {
        p = re + sprintf(re, n < 4097 ? "(?:(?:z|[q])" : "(?:z");
        for (int k = 0; k <= n; k++)
            p += sprintf(p, "|z");
        sprintf(p, ")");
        d = white_lre_split_same(re, 0, v.ctx, few, countof(few), &calls);
        CHECK(d == 0, "%d z: %d calls differ", n, d);
        bad += d != 0;
    }
    printf("unit-white: test_regexp_split_same: %ld calls, %d patterns differ\n", calls, bad);
    vm_free(&v);
}

/* ---- compiling with one request to the heap failing ---- */

/* The bits of the mode of a compile: any call of js_malloc_rt or js_realloc_rt
   is a request (small blocks too), and js is a module. */
#define MODE_CALLS 1
#define MODE_MODULE 2

/* compile_failing compiles js in a new runtime where request number fail
   fails, 0 for none, and runs the code it compiled with every request
   working. A request is a request to the heap, or with MODE_CALLS a call of
   js_malloc_rt or js_realloc_rt (small blocks too). It puts the result of the
   code as a string in result (room cap), or "exception" when it threw, and
   returns 1 when the compile failed, else 0. *n is the number of requests of
   the compile. A compile may only fail with the out of memory error. */
static int compile_failing(const char *what, const char *js, int fail, int mode, int *n, char *result, size_t cap)
{
    VM v = vm_new();
    JSValue c = white_compile_failing(v.ctx, js, fail, mode, n);
    int failed = JS_IsException(c);

    result[0] = 0;
    if (failed) {
        int oom = JS_HasException(v.ctx) && JS_IsUncatchableException(v.ctx);
        char *t = exception_text(v.ctx);

        CHECK(oom, "%s, request %d failing: the compile fails with %s, not the out of memory error", what, fail, t ? t : "no exception");
        free(t);
    } else {
        JSValue r;
        char *t;

        CHECK(!JS_HasException(v.ctx), "%s, request %d failing: the compile works, but leaves an exception", what, fail);
        r = JS_EvalFunction(v.ctx, c);
        t = value_text(v.ctx, r);
        snprintf(result, cap, "%s", t ? t : "exception");
        free(t);
        free(exception_text(v.ctx));
        JS_FreeValue(v.ctx, r);
    }
    if (failed)
        JS_FreeValue(v.ctx, c);
    CHECK(!JS_HasException(v.ctx), "%s, request %d failing: an exception is pending", what, fail);
    vm_free(&v);
    return failed;
}

/* compile_failing_alone is compile_failing in a process of its own
   (run_self), so that a compile that aborts is one failed case with its
   script and request, not the end of the test. The process gets the sweep,
   the number of the script, the request and want, and makes the list of the
   scripts again (compile_one). It returns 1 when the compile failed with the
   out of memory error, 0 when it worked and gave want, and -1 for any other
   outcome. */
static int compile_failing_alone(const char *what, int calls, size_t script, int fail, const char *want)
{
    char calls_arg[16], script_arg[32], fail_arg[16];
    char *args[7];
    int st;

    snprintf(calls_arg, sizeof(calls_arg), "%d", calls);
    snprintf(script_arg, sizeof(script_arg), "%zu", script);
    snprintf(fail_arg, sizeof(fail_arg), "%d", fail);
    args[1] = "compile-one";
    args[2] = calls_arg;
    args[3] = script_arg;
    args[4] = fail_arg;
    args[5] = (char *)want;
    args[6] = NULL;
    st = run_self(args);
    if (WIFEXITED(st) && (WEXITSTATUS(st) == 0 || WEXITSTATUS(st) == 10))
        return WEXITSTATUS(st) == 10;
    CHECK(0, "%s, request %d failing: the case %s", what, fail,
          WIFSIGNALED(st) ? "was stopped by a signal (the engine aborted)" : "failed");
    return -1;
}

/* More scripts for the sweep where any block may fail: each uses a place of
   the parser that looks ahead, patches code it wrote before, or adds to a
   table (the constant pool, the closure variables, the labels). */
static const char *const compile_alloc_scripts[] = {
    /* a destructuring pattern that is not an arrow function or a declaration:
       the parser looks ahead to the token after its closing bracket */
    "[aa, bb] = [1, 2]; ({cc, dd} = {cc: 1, dd: 2}); (ee, ff); [(gg), hh] = [3, 4]; aa + hh",
    "var a, b; for ([a, b] of [[1, 2]]) ; for ({a, b} of [{a: 1, b: 2}]) ; for ([a, b] in {x: 1}) ; typeof a",
    "var {x: {y}, z: [w]} = {x: {y: 1}, z: [2]}; ({p: [q]} = {p: [3]}); var q; y + w + q",
    /* parentheses and templates the lookahead reads through */
    "var o = {a: 1}; var t = (o.a, (o.a + 1) * 2, [o.a, (o)]); `x${(1, 2)}y${`z${(3)}`}` + (1) + t.length",
    /* class fields: the function that sets them, computed keys, static blocks */
    "class A { a = 1; ['b' + 1] = 2; static c = 3; static { A.d = 4 } #e = 5; e() { return this.#e } }; const o = new A(); o.a + o.b1 + A.c + A.d + o.e()",
    /* private methods, accessors and static ones, with a brand check */
    "class A { #m() { return 1 } get #g() { return 2 } set #g(v) {} static #s() { return 3 } static t(o) { return #m in o } u() { return this.#m() + this.#g + A.#s() } }; new A().u() + A.t(new A())",
    /* derived class with fields, an arrow in the constructor and super */
    "class B { constructor(x) { this.x = x } } class A extends B { f = () => this.x; constructor(a = 1, ...r) { super(a); this.r = r } }; new A(2, 3).f() + new A(4).r.length",
    /* a class with only static fields, one with only a brand, an empty one */
    "class A { static a = 1; static b = A.a + 1 } class B { #p = 1; static has(o) { return #p in o } } class C {} A.b + B.has(new B()) + typeof C",
    /* arrow functions: parameters that need a lookahead to tell from parens */
    "const f = (a, b = (1, 2), {c, d = 3} = {}, [e, ...g] = [5]) => a + b + c + d + e + g.length; const h = async (x) => await x; (a => a)(1) + f(1, undefined, {c: 4}) + ((x, y) => x * y)(2, 3)",
    "var v = 1; (v, (v), ((v))); ((a, b) => (a, b))(1, 2) + (typeof (() => {}))",
    /* regexp literals and flags, with a group name read before a class escape */
    "const a = /[a-z]+(?<n>\\d)\\k<n>/giu, b = /(?:x|y)+?z$/m, c = /\\p{L}+/v; 'ab11'.match(a)[0] + b.test('xyz') + c.test('ab') + /[/]\\//.source",
    "const r = /(?<year>\\d{4})-(?<month>\\d\\d)|[\\w&&[^\\d]]\\s\\S\\W\\D/gs; 'x 2024-05'.replace(r, '$<month>') + /\\u{1F600}/u.test('x')",
    /* a class of strings, with an empty one and a range: each goto of the
       strings is patched after code that may have failed to grow */
    "/[a-c\\q{abc|xy|}]/v.test('xy') + /[\\q{abc|}x]/vi.source + /[\\q{ab|cd}--\\q{cd}]/v.test('cd')",
    /* templates, tagged and nested, with escapes (and a bad escape in a tag) */
    "const t = (s, ...v) => s.raw.length + v.length; `a${`b${1 + 1}c`}d` + t`x\\n${1}y${2}z` + `\\u{41}${'q'}`",
    "const t = (s, ...v) => String(s[0]) + s.raw[0] + v.length; t`\\unicode and \\xerxes ${1} \\u{110000}`",
    /* destructuring with defaults in every place */
    "var {a = 1, b: {c = 2, ...d} = {}, ...e} = {z: 1}; var [f = 3, [g = 4] = [], ...h] = []; function k({p = 5} = {}, [q = 6] = []) { return p + q } let l, m; ({l = 7, m = 8} = {}); a + c + f + g + k() + l + m + Object.keys(e).length",
    "var a, b, c; [a, [b = 2], ...c] = [1, [], 3, 4]; ({x: a, y: [b], ...c} = {x: 1, y: [2], z: 3}); for ([a, b] of [[1, 2]]) ; for ({a, b} of [{a: 1, b: 2}]) ; for (var {x, y = 2} of [{x: 1}]) ; a + b + x + y",
    "function f({a, b} = {}, [c, d] = [], ...[e, f]) { return a + b + c + d + e + f } const g = async ({a}, [b]) => a + b; f({a: 1, b: 2}, [3, 4], 5, 6)",
    /* generators, async generators, for await and labels */
    "async function* g() { yield 1; yield* [2, 3] } async function f() { let s = 0; outer: for await (const x of g()) { for (const y of [1, 2]) { if (y > 1) continue outer; s += x } } return s } f().then(v => globalThis.r = v); typeof g",
    /* getters and setters, computed names, spread, optional chaining */
    "const k = 'p'; const o = { get [k]() { return 1 }, set [k](v) {}, ['q' + 1]: 2, ...{r: 3}, m() { return super.toString }, async *n() {} }; o?.p + o.q1 + o.r + (o.zz?.[0] ?? 4) + (o.m?.() ? 1 : 0)",
    /* switch, try/finally and closures that capture loop variables */
    "var fs = []; for (let i = 0; i < 3; i++) { switch (i) { case 0: fs.push(() => i); break; default: try { fs.push(() => i * 2) } finally { i += 0 } } } fs.map(f => f()).join()",
    /* eval and with, which keep variables by name */
    "function f(s) { with ({w: 2}) { return eval(s) + w } } f('var u = 1; u + 1')",
    /* let as a name, async as a name, yield and await as names */
    "var let_ = 1; var async = (x) => x; async\n(1); var yield_ = function* () { var yield2 = yield 1 }; var o = { async, get: 1, set: 2, static: 3 }; async(2) + o.get",
    /* new.target, super calls and properties, arguments, home objects */
    "function F() { return new.target === F } class B { m() { return 1 } } class A extends B { m() { return super.m() + (() => super.m())() + arguments.length } static s() { return super.name } }; new F() instanceof F",
    /* strict mode: directives, names that are checked, duplicate parameters */
    "function f(a, b) { 'use strict'; var eval_ = 1; return (function g(c) { return c })(a + b) } (function () { 'use strict'; return [typeof f, this === undefined] })()",
    /* catch with a pattern, optional catch, finally with a return, labels */
    "function f() { try { throw {message: 'm', code: 1} } catch ({message, code = 2}) { try { throw 1 } catch { return message + code } finally { var z = 1 } } } a: b: for (;;) { break a } f()",
    /* class heritage expression, computed members and arrows in fields */
    "const mix = (B) => class extends B { ['x' + 1] = () => this; static ['y' + 2] = (a = 1) => a; get [Symbol.toStringTag]() { return 'M' } }; class A {} const M = mix(A); new M().x1() instanceof A",
    /* spread in calls and arrays, comma, in operator in a for head, delete, void */
    "var o = {a: 1}, arr = [1, 2]; for (var i = ('a' in o) ? 0 : 1, j = (1, 2); i < 1; i++) ; Math.max(...arr, ...[3], 4) + (delete o.a ? 1 : 0) + (void 0 === undefined) + [...arr, ...'ab'].length",
    /* async arrows and methods with destructuring and defaults */
    "const f = async ({a = 1, ...r} = {}, [b] = [2], c = async () => 3) => a + b + await c(); const o = { async m({x}) { return x }, async *g(a = 1) { yield a }, get v() { return 1 } }; f().then(() => {}); o.v",
    /* try/finally with break, continue and return across loops and a switch */
    "function f(n) { for (var i = 0; i < n; i++) { try { if (i == 1) continue; if (i == 3) break; switch (i) { case 2: return i; default: } } finally { n += 0 } } return -1 } f(5)",
    /* getters and setters on classes with super, static blocks, new.target in arrows */
    "class B { get a() { return 1 } set a(v) {} static m() { return 2 } } class A extends B { get a() { return super.a + 1 } set a(v) { super.a = v } static m() { return super.m() + (() => new.target)() } static { this.k = super.m() } }; new A().a + A.k",
    /* nested functions that close over arguments, this, eval-free vars */
    "function outer(a) { var x = arguments; return function () { return () => [a, x.length, this, new.target, arguments.length] } } outer(1, 2)()()",
    /* object literals: shorthand, methods, __proto__, numeric and string keys */
    "var a = 1, __proto__ = 2; var o = { a, b() {}, 1: 'n', 'c d': 3, [a + 1]: 4, __proto__: null, get 5() { return 5 }, set 'e'(v) {}, async f() {}, *g() {}, async *h() {} }; Object.keys(o).length",
    /* very long expression and a long chain of calls and properties */
    "var x = 1 + 2 * 3 - 4 / 2 % 3 << 1 >> 1 >>> 1 & 7 | 8 ^ 9; var y = x.toString().length.toFixed(1).split('').join('-').toUpperCase(); var z = x < 1 ? y : x > 2 ? 'a' : 'b'; (x && y || z) ?? x",
    /* numbers, bigints, strings with escapes, comments, labels, exponent */
    "var a = 0x1f + 0o7 + 0b11 + 1_000 + 1e3 + .5 + 5. + 08; var b = 10n ** 3n + 0xffn; var c = '\\x41\\u0042\\u{43}\\101\\\n' + \"\\0\"; /* c */ // d\nvar e = 2 ** 3 ** 2, f = (-2) ** 2; l: { break l } a + c.length + e + f + typeof b",
};

/* The modules of the sweep (compiled with MODE_MODULE): exports of every
   form, top level await and import.meta. None has an import: the compile
   loads the modules it imports, and nothing here loads one. */
static const char *const compile_module_scripts[] = {
    "export const a = 1, b = [2]; export let c; export var d = 4; export function f() { return a } export class K { static s = 1 } export default 5; export { a as e, b as default2 }; export { f as 'string name' }",
    "var r = import.meta.url, s = import.meta; import('lazy').then(() => {}, () => {}); export { r, s as meta }; export { r as 'quoted name' }",
    "export default function () { return 1 } export const g = async () => { await 1 }; await Promise.resolve(2); for await (const v of [Promise.resolve(1)]) ; export { g as h }",
    "export default class { static #p = 1; static q() { return #p in this } } export const { a, b: [c = 1] } = { a: 1, b: [] }; export const [d, ...e] = [1, 2, 3]",
};

/* startTag and endTag keep how they write each attribute name, style name and
   end tag in tables of the context that are made at their first entry and
   grow, so that a name a page uses is made once and stays: two names never
   take each other's place. The tables are emptied only when they would pass
   512 names or 256 end tags. */
static void emit_tables(VM *v, int size[3], int count[3])
{
    int r = white_emit_tables(v->ctx, size, count);

    CHECK(r == 0, "the tables of the tags are not as made (%d)", r);
}

static void test_emit_tables(void)
{
    static const char setup[] =
        "var hop = Object.prototype.hasOwnProperty, d = {}, fc = { viewTransition: null };\n"
        "function names(from, n) {\n"
        "  for (var i = from; i < from + n; i++) {\n"
        "    var p = { ['data-n' + i]: i, style: { ['w' + i]: i } }, t = [];\n"
        "    if (startTag(t, p, 'div', fc, hop, d) !== null || t[0] !== '<div data-n' + i + '=\"' + i + '\" style=\"w' + i + ':' + i + (i ? 'px' : '') + '\">')\n"
        "      throw new Error('wrote ' + t[0]);\n"
        "  }\n"
        "}\n"
        "var tags = [];\n"
        "for (var i = 0; i < 300; i++) tags.push('t' + i);\n"
        "function ends(from, n) {\n"
        "  for (var i = from; i < from + n; i++)\n"
        "    if (endTag(tags[i]) !== '</t' + i + '>') throw new Error('end ' + i);\n"
        "}\n";
    VM v = vm_new();
    JSValue g = JS_GetGlobalObject(v.ctx);
    int size[3], count[3];

    JS_SetPropertyStr(v.ctx, g, "startTag", JS_NewStartTagFunction(v.ctx, "startTag", 8));
    JS_SetPropertyStr(v.ctx, g, "endTag", JS_NewEndTagFunction(v.ctx, "endTag", 6));
    JS_FreeValue(v.ctx, g);
    /* nothing is made before the first call */
    emit_tables(&v, size, count);
    CHECK(size[0] == 0 && size[1] == 0 && size[2] == 0, "tables of %d, %d and %d before a call", size[0], size[1], size[2]);
    CHECK(eval_ok(v.ctx, setup), "the setup failed");
    CHECK(eval_ok(v.ctx, "names(0, 1); 0"), "names failed");
    emit_tables(&v, size, count);
    /* data-n0 and style, and w0 */
    CHECK(size[0] == 16 && count[0] == 2 && size[1] == 16 && count[1] == 1 && size[2] == 0,
          "after one call: %d of %d names, %d of %d styles, %d end tags", count[0], size[0], count[1], size[1], size[2]);
    /* 200 names and 100 end tags, twice: the second time makes nothing */
    CHECK(eval_ok(v.ctx, "names(1, 199); ends(0, 100); 0"), "names failed");
    emit_tables(&v, size, count);
    CHECK(size[0] == 512 && count[0] == 201 && size[1] == 512 && count[1] == 200 && size[2] == 256 && count[2] == 100,
          "after 200 names: %d of %d names, %d of %d styles, %d of %d end tags",
          count[0], size[0], count[1], size[1], count[2], size[2]);
    CHECK(eval_ok(v.ctx, "names(0, 200); ends(0, 100); 0"), "names failed");
    emit_tables(&v, size, count);
    CHECK(count[0] == 201 && count[1] == 200 && count[2] == 100,
          "the same names again: %d names, %d styles, %d end tags", count[0], count[1], count[2]);
    /* past 256 names (half of 512) a table is emptied and fills again, with
       style again, and the end tags past 128 */
    CHECK(eval_ok(v.ctx, "names(200, 100); ends(100, 200); 0"), "names failed");
    emit_tables(&v, size, count);
    CHECK(size[0] == 512 && count[0] == 301 - 256 + 1 && size[1] == 512 && count[1] == 300 - 256 &&
          size[2] == 256 && count[2] == 300 - 2 * 128,
          "past the most: %d of %d names, %d of %d styles, %d of %d end tags",
          count[0], size[0], count[1], size[1], count[2], size[2]);
    CHECK(eval_ok(v.ctx, "names(0, 300); ends(0, 300); 0"), "names failed after the tables were emptied");
    emit_tables(&v, size, count);
    vm_free(&v);
}

/* the slack of the string the script src gives (see white_string_slack) */
static int emit_slack(VM *v, const char *src)
{
    JSValue r = JS_Eval(v->ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int s;

    CHECK(!JS_IsException(r), "%s threw", src);
    s = white_string_slack(r);
    JS_FreeValue(v->ctx, r);
    return s;
}

/* A start tag is written into a buffer the cache keeps, then copied into a
   string of its length, and the names, style names and end tags the cache
   keeps have their length too: an arena block never shrinks, and a string
   left in a block of the size its buffer grew to would hold an arena with
   the long-lived objects of a render once the render is freed. A buffer that
   grew past 1024 units, or became wide, is freed after the call. A call
   made while the buffer is out (none can be today: nothing between the take
   and the end runs a script) makes one of its own, and of the two the cache
   keeps one and frees the other. */
static void test_emit_buffer(void)
{
    static const char setup[] =
        "var hop = Object.prototype.hasOwnProperty, d = {}, fc = { viewTransition: null };\n"
        "function tag(p, type) {\n"
        "  var t = [];\n"
        "  if (startTag(t, p, type || 'div', fc, hop, d) === d || t.length !== 1) throw new Error('declined');\n"
        "  return t[0];\n"
        "}\n"
        "var many = {};\n"
        "for (var i = 0; i < 12; i++) many['data-attribute-' + i] = 'value ' + i;\n";
    VM v = vm_new();
    JSValue g = JS_GetGlobalObject(v.ctx);
    int s, size;

    JS_SetPropertyStr(v.ctx, g, "startTag", JS_NewStartTagFunction(v.ctx, "startTag", 8));
    JS_SetPropertyStr(v.ctx, g, "endTag", JS_NewEndTagFunction(v.ctx, "endTag", 6));
    JS_FreeValue(v.ctx, g);
    CHECK(eval_ok(v.ctx, setup), "the setup failed");
    CHECK(white_emit_buf(v.ctx, 0) == 0, "a buffer before the first call");

    /* a short tag: a buffer of 128 units is kept, the string has its length */
    s = emit_slack(&v, "tag({ id: 'a' })");
    CHECK(s == 0 || s == -3, "the string of a short tag is %d classes above its length", s);
    size = white_emit_buf(v.ctx, 0);
    CHECK(size >= 128 && size <= 1024, "a buffer of %d units after a short tag", size);

    /* a tag of 300 units grows the buffer, which is kept */
    s = emit_slack(&v, "var m = tag(many); if (m.length < 300) throw new Error(m); m");
    CHECK(s == 0 || s == -3, "the string of a long tag is %d classes above its length", s);
    size = white_emit_buf(v.ctx, 0);
    CHECK(size >= 300 && size <= 1024, "a buffer of %d units after a tag of 300", size);

    /* a text of 2000 units takes the buffer past 1024: it is freed */
    CHECK(eval_ok(v.ctx, "var x = 'y'.repeat(2000); if (tag({ children: x }, 'p') !== '<p>' + x) throw new Error('long'); 0"),
          "a long text failed");
    CHECK(white_emit_buf(v.ctx, 0) == 0, "the buffer of a long text was kept");
    s = emit_slack(&v, "tag({ id: 'b' })");
    CHECK(s == 0 || s == -3, "the string after a long text is %d classes above its length", s);
    CHECK(white_emit_buf(v.ctx, 0) > 0, "no buffer after a short tag again");

    /* a wide text makes the buffer wide: the string is wide, the buffer freed */
    CHECK(eval_ok(v.ctx, "if (tag({ children: 'a\\u4e2d' }, 'b') !== '<b>a\\u4e2d') throw new Error('wide'); 0"),
          "a wide text failed");
    CHECK(white_emit_buf(v.ctx, 0) == 0, "a wide buffer was kept");
    s = emit_slack(&v, "tag({ title: '\\u4e2d'.repeat(40) })");
    CHECK(s == 0 || s == -3, "a wide string is %d classes above its length", s);

    /* a call while the buffer is out makes its own, which the cache keeps;
       the one given back after it is freed (the host checks for a leak) */
    CHECK(eval_ok(v.ctx, "tag({ id: 'c' }); 0"), "a tag failed");
    size = white_emit_buf(v.ctx, 1);
    CHECK(size >= 128 && white_emit_buf(v.ctx, 0) == 0, "the buffer was not taken (%d)", size);
    CHECK(eval_ok(v.ctx, "if (tag({ id: 'd', className: 'e' }) !== '<div id=\"d\" class=\"e\">') throw new Error('inner'); 0"),
          "a call while the buffer was out failed");
    CHECK(white_emit_buf(v.ctx, 0) > 0, "the call while the buffer was out kept none");
    CHECK(white_emit_buf(v.ctx, 2) > 0, "no buffer after the one taken came back");
    CHECK(eval_ok(v.ctx, "if (tag({ id: 'f' }) !== '<div id=\"f\">') throw new Error('after'); 0"),
          "a call after the buffer came back failed");

    /* names, style names that grow as they are written, and end tags */
    CHECK(eval_ok(v.ctx,
                  "tag({ className: 'a', 'aria-label': 'b', hidden: true, style: { aBCDEFGHIJKLMNOP: 1, '--x<y>&': 2, marginTop: 3 } });\n"
                  "if (endTag('section') !== '</section>' || endTag('\\u4e2d\\u6587') !== '</\\u4e2d\\u6587>') throw new Error('end');\n"
                  "0"), "names failed");
    s = white_emit_chunks_slack(v.ctx);
    CHECK(s == 0, "a name of the cache is %d classes above its length", s);
    s = emit_slack(&v, "endTag('section')");
    CHECK(s == 0 || s == -3, "an end tag is %d classes above its length", s);
    s = emit_slack(&v, "endTag('\\u4e2d\\u6587')");
    CHECK(s == 0 || s == -3, "a wide end tag is %d classes above its length", s);
    vm_free(&v);
}

/* Compiling with one request failing, and the others working, fails with the
   out of memory error or gives code that works as it should. A request is a
   request to the heap (what a large block failing under the memory limit
   looks like, as the small blocks still fit in their arenas), or with calls
   set any call of js_malloc_rt or js_realloc_rt, small blocks too (what every
   block failing alone looks like, as it does in the builds that take each
   block from the system malloc). */
struct compile_script {
    const char *js;
    int mode;
};

#define COMPILE_SCRIPTS \
    (countof(compile_scripts) + countof(compile_fine_scripts) + countof(compile_alloc_scripts) + countof(compile_module_scripts) + 5)

/* compile_sweep_scripts puts the scripts of the sweep in scripts (room for
   COMPILE_SCRIPTS), any call a request when calls is set, and gives their
   number. The test and the process of each case make the same list. */
static size_t compile_sweep_scripts(int calls, struct compile_script *scripts)
{
    static char big_scope[16384], deep_scope[8192], fields[8192], cases[8192];
    size_t i, k = 0;
    int j;

    nested_scopes();
    strcpy(big_scope, "function f() { let n = 0; ");
    for (j = 0; j < 400; j++)
        strcat(big_scope, "{ let a = 1; n += a } ");
    strcat(big_scope, "return n } f()");
    deep_scope[0] = 0;
    for (j = 0; j < 300; j++)
        strcat(deep_scope, "{ let a = 1; ");
    for (j = 0; j < 300; j++)
        strcat(deep_scope, "} ");
    strcat(deep_scope, "1");
    strcpy(fields, "class A { #m() { return 1 } ");
    for (j = 0; j < 200; j++)
        strcat(fields, "x = 1; ");
    strcat(fields, "y() { return this.#m() } }; new A().y()");
    strcpy(cases, "switch (2) { ");
    for (j = 0; j < 300; j++)
        strcat(cases, "case 1: break; ");
    strcat(cases, "default: 3 }");
    for (i = 0; i < countof(compile_scripts); i++)
        scripts[k++].js = compile_scripts[i];
    for (i = 0; i < countof(compile_fine_scripts); i++)
        scripts[k++].js = compile_fine_scripts[i];
    for (i = 0; i < countof(compile_alloc_scripts); i++)
        scripts[k++].js = compile_alloc_scripts[i];
    scripts[k++].js = big_scope;
    scripts[k++].js = deep_scope;
    scripts[k++].js = fields;
    scripts[k++].js = cases;
    /* Private methods and no fields: the code that adds the brand is made
       and patched at the end of the class, with no token between. */
    scripts[k++].js = "class A { #m() { return 1 } y() { return this.#m() } }; new A().y()";
    for (i = 0; i < k; i++)
        scripts[i].mode = calls ? MODE_CALLS : 0;
    for (i = 0; i < countof(compile_module_scripts); i++) {
        scripts[k].js = compile_module_scripts[i];
        scripts[k++].mode = (calls ? MODE_CALLS : 0) | MODE_MODULE;
    }
    return k;
}

/* compile_one is the process of one case of compile_failing_alone. */
static void compile_one(int calls, size_t script, int fail, const char *want)
{
    struct compile_script scripts[COMPILE_SCRIPTS];
    size_t k = compile_sweep_scripts(calls, scripts);
    char what[32], got[256];
    int n, failed;

    failures = 0;
    snprintf(what, sizeof(what), "script %zu", script);
    if (script >= k) {
        CHECK(0, "%s: the sweep has %zu scripts", what, k);
        host_leave(1);
    }
    failed = compile_failing(what, scripts[script].js, fail, scripts[script].mode, &n, got, sizeof(got));
    if (!failed)
        CHECK(strcmp(got, want) == 0, "%s, request %d failing: the code compiled gives %s, want %s", what, fail, got, want);
    fflush(stdout);
    host_leave(failures ? 1 : failed ? 10 : 0);
}

static void test_compile_one_failure(int calls)
{
    struct compile_script scripts[COMPILE_SCRIPTS];
    size_t i, k = compile_sweep_scripts(calls, scripts);
    long cases_run = 0, bad = 0;

    for (i = 0; i < k; i++) {
        char what[32], want[256];
        int n, fail, failures_of_script = 0, bad_of_script = 0;

        snprintf(what, sizeof(what), "script %zu", i);
        CHECK(!compile_failing(what, scripts[i].js, 0, scripts[i].mode, &n, want, sizeof(want)), "%s does not compile", what);
        for (fail = 1; fail <= n; fail++) {
            int r = compile_failing_alone(what, calls, i, fail, want);

            cases_run++;
            if (r < 0) {
                bad++;
                bad_of_script++;
            } else {
                failures_of_script += r;
            }
        }
        if (bad_of_script)
            printf("unit-white: %s (%.40s): %d of %d requests bad\n", what, scripts[i].js, bad_of_script, n);
        CHECK(failures_of_script > 0, "%s: no compile of its %d requests failed", what, n);
    }
    printf("unit-white: test_compile_one_failure (%s): %ld cases, %ld bad\n", calls ? "every call" : "heap requests", cases_run, bad);
}

/* ---- the register loop ---- */

/* Functions the pass of the register loop converts (engine/regvm-pass.h),
   with what f(3) gives: runs of instructions that can throw of every length,
   so that each buffer of the pass grows at the first byte of one of them
   somewhere, a catch and a finally (the pcmap entries of CATCH and GOSUB), a
   closure, a loop, a for-of and a switch on strings. */
static const struct {
    const char *src, *want;
} regvm_scripts[] = {
    { "globalThis.f = function (x) { 'use strict'; return x + 1 + 2; }; 0", "6" },
    { "globalThis.f = function (x) { 'use strict'; return x.a; }; 0", "undefined" },
    { "globalThis.f = function (x) { 'use strict'; var o = { a: x }; return o.a + o.a + o.a + o.a + o.a + o.a + o.a; }; 0", "21" },
    { "globalThis.f = function (x) { 'use strict'; var r = 0;\n"
      "  try { try { r = x + 1; if (r > 2) throw r; } catch (e) { r = e + 10; } finally { r = r + 100; } } catch (e2) { r = -1; }\n"
      "  return r; }; 0", "114" },
    { "globalThis.f = function (x) { 'use strict'; var a = x + 1; var g = function () { return a + x; }; a = a + 1; return g() + 1; }; 0", "9" },
    { "globalThis.f = function (x) { 'use strict'; var s = 0, i; for (i = 0; i < x; i++) { if (i == 1) continue; s = s + i * 2; } return s; }; 0", "4" },
    { "globalThis.f = function (x) { 'use strict'; var s = 0, o = { a: 1, b: [x, 2] }, v;\n"
      "  for (v of o.b) s = s + v;\n"
      "  switch (typeof x) { case 'number': s = s + 1; break; case 'string': s = s + 2; break; default: s = s - 1; }\n"
      "  return s + o.a; }; 0", "7" },
    /* the shapes the pass has tables for: a generator, an async function
       and an async generator (g, converted, and f, which runs it), new.target
       and the mapped arguments, constructors (check_ctor, and init_ctor of
       the default constructor of a derived class), with and a direct eval */
    { "globalThis.g = function* (x) { 'use strict'; var y = yield x + 1; return x + y; };\n"
      "globalThis.f = function (x) { var it = g(x); it.next(); return it.next(10).value; }; 0", "13" },
    { "globalThis.g = async function (x) { 'use strict'; var y = await x; return y + 1; };\n"
      "globalThis.f = function (x) { g(x).then(function (v) { globalThis.out = v; }); return 0; }; 0", "4" },
    { "globalThis.g = async function* (x) { 'use strict'; var y = await x; yield y + 1; };\n"
      "globalThis.f = function (x) { g(x).next().then(function (r) { globalThis.out = r.value; }); return 0; }; 0", "4" },
    { "globalThis.f = function (x) { arguments[0] = x + 1; return new.target === undefined ? x + arguments.length : 0; }; 0", "5" },
    { "globalThis.g = class { constructor(x) { this.a = x; } };\n"
      "globalThis.f = function (x) { return new g(x).a + 1; }; 0", "4" },
    { "globalThis.g = class extends class { constructor(x) { this.a = x; } } {};\n"
      "globalThis.f = function (x) { return new g(x).a + 1; }; 0", "4" },
    { "globalThis.f = function (x) { var o = { y: 1 }; with (o) { x = x + y; } return eval('x + 1'); }; 0", "5" },
};

/* the shapes too long to write out: a stack of 130 entries, which folds
   (engine/regvm-fold.h), in a function and in a generator that suspends in
   it, 130 captured locals, which leave no room for the temps (their
   var_refs on the heap, regvm-ops-wide.h), and 140 locals, whose slots are
   past -128 (MOVW). 0 when there is no shape i. */
static int regvm_convert_shape(int i, char *src, size_t cap, char *want, size_t want_cap)
{
    size_t len;
    int k;

    switch (i) {
    case 0:
    case 1:
        if (i == 0)
            len = snprintf(src, cap, "globalThis.f = function (x) { 'use strict'; return ");
        else
            len = snprintf(src, cap, "globalThis.g = function* (x) { 'use strict'; return ");
        for (k = 0; k < 129; k++)
            len += snprintf(src + len, cap - len, k == 64 && i == 1 ? "(yield x) + (" : "x + (");
        len += snprintf(src + len, cap - len, "x");
        for (k = 0; k < 129; k++)
            len += snprintf(src + len, cap - len, ")");
        if (i == 0)
            snprintf(src + len, cap - len, "; }; 0");
        else
            snprintf(src + len, cap - len, "; };\nglobalThis.f = function (x) { var it = g(x); it.next(); return it.next(x).value; }; 0");
        snprintf(want, want_cap, "390");
        return 1;
    case 2:
    case 3:
        len = snprintf(src, cap, "globalThis.f = function (x) { 'use strict'; var v0 = x");
        for (k = 1; k < (i == 2 ? 130 : 140); k++)
            len += snprintf(src + len, cap - len, ", v%d = x", k);
        if (i == 2) {
            len += snprintf(src + len, cap - len, "; return (function () { return v0");
            for (k = 1; k < 130; k++)
                len += snprintf(src + len, cap - len, " + v%d", k);
            snprintf(src + len, cap - len, "; })(); }; 0");
            snprintf(want, want_cap, "390");
        } else {
            snprintf(src + len, cap - len, "; return v0 + v139; }; 0");
            snprintf(want, want_cap, "6");
        }
        return 1;
    default:
        return 0;
    }
}

/* the rounds of the pass made again in the last conversion of
   regvm_convert_case with fail 0 and the bytes of the code it made */
static long case_shorts;
static int case_code_len;

/* regvm_convert_case converts g of src, or f when src sets no g, with call
   number fail of the allocator failing, 0 for none, and with small set each
   buffer of the pass starting at 16 bytes. A conversion that could not get
   a block never ends the process, and the function is not converted in
   part: it stays unconverted and the next call converts it. f(3) then gives
   want, or globalThis.out does once the jobs have run, when src sets it.
   Returns the calls of the allocator the conversion made, and in *hash
   (fail 0) the hash of the register code it made. */
static int regvm_convert_case(const char *src, const char *want, int fail, int small, uint32_t *hash)
{
    VM v = vm_new();
    JSValue g, f, c, arg, r, out;
    JSContext *jctx;
    char *t;
    int made = 0, conv;

    CHECK(eval_ok(v.ctx, src), "the setup threw: %s", src);
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "f");
    c = JS_GetPropertyStr(v.ctx, g, "g");
    if (JS_IsUndefined(c))
        c = JS_DupValue(v.ctx, f);
    conv = white_regvm_convert_failing(v.ctx, c, fail, small ? 3 : 1, &made);
    if (fail == 0) {
        CHECK(conv == 1, "the pass does not convert it (%d), so the case tests nothing: %s", conv, src);
        *hash = white_regvm_code_hash(c, &case_code_len);
        case_shorts = white_regvm_shorts();
    } else {
        CHECK(conv == 0 && JS_RegvmFuncStat(v.ctx, c, 0) == 0, "call %d of %d failing: the conversion gives %d, not a function left unconverted for a later call: %s", fail, made, conv, src);
    }
    CHECK(!JS_HasException(v.ctx), "call %d failing: the conversion leaves an exception", fail);
    arg = JS_NewInt32(v.ctx, 3);
    r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &arg);
    while (JS_ExecutePendingJob(v.rt, &jctx) > 0)
        ;
    out = JS_GetPropertyStr(v.ctx, g, "out");
    if (!JS_IsUndefined(out) && !JS_IsException(r)) {
        JS_FreeValue(v.ctx, r);
        r = out;
    } else {
        JS_FreeValue(v.ctx, out);
    }
    t = value_text(v.ctx, r);
    CHECK(t && strcmp(t, want) == 0, "call %d failing: f(3) gives %s, not %s: %s", fail, t ? t : "(exception)", want, src);
    CHECK(JS_RegvmFuncStat(v.ctx, c, 0) == 1, "call %d failing: the function has no register code after it ran (%d): %s", fail, JS_RegvmFuncStat(v.ctx, c, 0), src);
    free(t);
    free(exception_text(v.ctx));
    JS_FreeValue(v.ctx, r);
    JS_FreeValue(v.ctx, c);
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, g);
    vm_free(&v);
    return made;
}

/* The scripts of the sweep by number: those of regvm_scripts, the shapes
   of regvm_convert_shape, then the chains, for each lead a sum of 1 to 60
   terms, each term an instruction that can throw. 0 when there is no
   script i. */
static int regvm_convert_script(int i, char *src, size_t cap, char *want, size_t want_cap)
{
    static const char *const lead[] = { "", "x = x + 0;", "var o = { a: x };", "try { x = x + 0; } finally { x = x - 0; }" };
    static const int terms[] = { 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 19, 26, 33, 40, 47, 54, 60 };
    int fixed = sizeof(regvm_scripts) / sizeof(regvm_scripts[0]), nterms = sizeof(terms) / sizeof(terms[0]), n, k;
    size_t len;

    if (i < fixed) {
        snprintf(src, cap, "%s", regvm_scripts[i].src);
        snprintf(want, want_cap, "%s", regvm_scripts[i].want);
        return 1;
    }
    i -= fixed;
    if (regvm_convert_shape(i, src, cap, want, want_cap))
        return 1;
    for (n = 0; regvm_convert_shape(n, src, cap, want, want_cap); n++)
        ;
    i -= n;
    if (i >= (int)(sizeof(lead) / sizeof(lead[0])) * nterms)
        return 0;
    n = terms[i % nterms];
    len = snprintf(src, cap, "globalThis.f = function (x) { 'use strict'; %s return x", lead[i / nterms]);
    for (k = 1; k <= n; k++)
        len += snprintf(src + len, cap - len, " + %d", k);
    snprintf(src + len, cap - len, "; }; 0");
    snprintf(want, want_cap, "%d", 3 + n * (n + 1) / 2);
    return 1;
}

/* regvm_convert_one is the sweep of script i in a process of its own
   (run_self), so that a conversion that ends the process is one failed case:
   every call of the allocator failing in turn, or with fail not negative
   that call alone */
static void regvm_convert_one(int i, int fail, int small)
{
    char src[8192], want[32];
    int n, n_small;
    uint32_t h, h_small;

    failures = 0;
    if (!regvm_convert_script(i, src, sizeof(src), want, sizeof(want))) {
        CHECK(0, "the sweep has no script %d", i);
    } else if (fail >= 0) {
        regvm_convert_case(src, want, fail, small, &h);
    } else {
        /* the scratch of the pass, then the register code */
        n = regvm_convert_case(src, want, 0, 0, &h);
        CHECK(n >= 2, "script %d: only %d calls of the allocator", i, n);
        for (fail = 1; fail <= n; fail++)
            regvm_convert_case(src, want, fail, 0, &h_small);
        /* with buffers that run short, made again in a larger scratch: the
           same code, and each growth of the scratch failing in turn */
        n_small = regvm_convert_case(src, want, 0, 1, &h_small);
        CHECK(h_small == h, "script %d: buffers that run short make other code (%08x, not %08x)", i, h_small, h);
        CHECK(case_shorts > 0 || case_code_len <= 16, "script %d: with buffers of 16 bytes no round was made again", i);
        for (fail = 1; fail <= n_small; fail++)
            regvm_convert_case(src, want, fail, 1, &h);
    }
    fflush(stdout);
    host_leave(failures ? 1 : 0);
}

static int regvm_convert_alone(int i, int fail, int small)
{
    char i_arg[16], fail_arg[16];
    char *args[6];

    snprintf(i_arg, sizeof(i_arg), "%d", i);
    snprintf(fail_arg, sizeof(fail_arg), "%d", fail);
    args[1] = "regvm-convert";
    args[2] = i_arg;
    args[3] = fail_arg;
    args[4] = small ? "small" : "plain";
    args[5] = NULL;
    return run_self(args);
}

static void test_regvm_convert_alloc(void)
{
    char src[8192], want[32];
    int i, st, fail, small, bad = 0;

    for (i = 0; regvm_convert_script(i, src, sizeof(src), want, sizeof(want)); i++) {
        st = regvm_convert_alone(i, -1, 0);
        if (WIFEXITED(st) && WEXITSTATUS(st) == 0)
            continue;
        bad++;
        if (!WIFSIGNALED(st)) {
            CHECK(0, "script %d: the sweep failed", i);
            continue;
        }
        /* which call it was */
        for (small = 0; small < 2; small++) {
            for (fail = 0; fail < 1000; fail++) {
                st = regvm_convert_alone(i, fail, small);
                if (WIFSIGNALED(st))
                    break;
            }
            if (fail < 1000)
                break;
        }
        CHECK(0, "script %d, call %d of the allocator failing%s: the conversion ends the process (the engine aborted): %s", i, fail, small ? " (buffers of 16 bytes)" : "", src);
    }
    printf("unit-white: test_regvm_convert_alloc: %d scripts, %d bad\n", i, bad);
}

/* The heap that fails a request gives the scratch of the pass back, as it
   gives the front cache, but not while a conversion works in it
   (rt->rv_in_pass): the pass would go on in a block given back. A
   conversion runs no script and makes no object (rv_build has the runtime
   and no context), so no other conversion and no collection starts in one,
   and the test builds abort if one does (rv_build, rv_scratch_free). */
static void test_regvm_scratch_drain(void)
{
    VM v = vm_new();
    int kept;

    CHECK(eval_is(v.ctx, "var f = function (x) { 'use strict'; return x + 1; }; f(1)", "2"), "f(1)");
    kept = JS_RegvmStat(v.ctx, 11);
    CHECK(kept > 0, "a conversion keeps no scratch");
    CHECK(white_regvm_scratch_drain(v.ctx, 1) == kept, "the heap gave the scratch back while a conversion works in it");
    CHECK(white_regvm_scratch_drain(v.ctx, 0) == 0, "the heap that failed a request kept %d bytes of scratch", JS_RegvmStat(v.ctx, 11));
    vm_free(&v);
}

/* A first call whose conversion runs out of memory: the call ends with the uncatchable out of memory error,
   on every entry, with no second try in that call, and the function stays
   unconverted, so that the next call converts it. Each allocation of the
   conversion fails in turn, for a call from C, a call of the register loop,
   one through Function.prototype.call and one from a function with a with
   statement. */
static const char regvm_entry_setup[] =
    "globalThis.f = function (x) { 'use strict'; return x + 39; };\n"
    "globalThis.inLoop = function (t) { 'use strict'; return t ? f(3) : 0; };\n"
    "globalThis.viaCall = function (t) { 'use strict'; return t ? f.call(null, 3) : 0; };\n"
    "globalThis.viaWith = function (t) { with ({}) { return t ? f(3) : 0; } };\n"
    "0";

static void regvm_entry_vm(VM *v, const char *entry, JSValue *f, JSValue *e)
{
    JSValue g, arg = JS_FALSE;

    *v = vm_new();
    CHECK(eval_ok(v->ctx, regvm_entry_setup), "the setup of the entries threw");
    g = JS_GetGlobalObject(v->ctx);
    *f = JS_GetPropertyStr(v->ctx, g, "f");
    *e = JS_GetPropertyStr(v->ctx, g, entry);
    JS_FreeValue(v->ctx, g);
    if (strcmp(entry, "f") != 0)
        JS_FreeValue(v->ctx, JS_Call(v->ctx, *e, JS_UNDEFINED, 1, &arg));
    /* the scratch of the pass back, for the conversion of f to make */
    JS_RunGC(v->rt);
}

static void test_regvm_convert_oom_entries(void)
{
    static const char *const entries[] = { "f", "inLoop", "viaCall", "viaWith" };
    int i, k, conv, n, cases = 0;
    VM v;
    JSValue f, e, r, arg;
    char *t;

    /* the allocations of the conversion of f */
    regvm_entry_vm(&v, "f", &f, &e);
    white_regvm_convert_failing(v.ctx, f, 0, 1, &conv);
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, e);
    vm_free(&v);
    CHECK(conv >= 2, "the conversion of f makes only %d calls of the allocator", conv);
    for (i = 0; i < (int)(sizeof(entries) / sizeof(entries[0])); i++) {
        for (k = 1; k <= conv; k++) {
            regvm_entry_vm(&v, entries[i], &f, &e);
            arg = strcmp(entries[i], "f") == 0 ? JS_NewInt32(v.ctx, 3) : JS_TRUE;
            r = white_call_failing(v.ctx, e, JS_UNDEFINED, 1, &arg, k, &n);
            CHECK(JS_IsException(r) && JS_IsUncatchableException(v.ctx),
                  "%s, call %d of the conversion failing: the call does not end with the uncatchable error", entries[i], k);
            t = exception_text(v.ctx);
            CHECK(t && strstr(t, "out of memory"), "%s, call %d failing: it throws %s, not out of memory", entries[i], k, t ? t : "nothing");
            free(t);
            CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 0, "%s, call %d failing: f is not left unconverted (%d)", entries[i], k, JS_RegvmFuncStat(v.ctx, f, 0));
            r = JS_Call(v.ctx, e, JS_UNDEFINED, 1, &arg);
            t = value_text(v.ctx, r);
            CHECK(t && strcmp(t, "42") == 0, "%s, call %d failing: the next call gives %s, not 42", entries[i], k, t ? t : "(exception)");
            free(t);
            free(exception_text(v.ctx));
            JS_FreeValue(v.ctx, r);
            CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 1, "%s, call %d failing: the next call does not convert f", entries[i], k);
            JS_FreeValue(v.ctx, f);
            JS_FreeValue(v.ctx, e);
            vm_free(&v);
            cases++;
        }
    }
    /* the call of f ends before it runs (its depth is over the limit)
       after its conversion ran out of memory: the next call of f
       converts it, with no stale out of memory error */
    /* the depth where inLoop runs and the call of f is one too deep: f is
       converted there before the depth stops it */
    arg = JS_TRUE;
    for (k = 1; k < 16; k++) {
        regvm_entry_vm(&v, "inLoop", &f, &e);
        JS_SetMaxCallDepth(v.rt, k);
        r = JS_Call(v.ctx, e, JS_UNDEFINED, 1, &arg);
        free(exception_text(v.ctx));
        JS_FreeValue(v.ctx, r);
        conv = JS_RegvmFuncStat(v.ctx, f, 0);
        JS_FreeValue(v.ctx, f);
        JS_FreeValue(v.ctx, e);
        vm_free(&v);
        if (!JS_IsException(r))
            break;
        if (conv == 1)
            break;
    }
    CHECK(k < 16 && JS_IsException(r), "no depth where f is converted and its call is too deep (%d)", k);
    regvm_entry_vm(&v, "inLoop", &f, &e);
    CHECK(JS_SetMaxCallDepth(v.rt, k) == 0, "the call depth cannot be set");
    r = white_call_failing(v.ctx, e, JS_UNDEFINED, 1, &arg, 1, &n);
    t = exception_text(v.ctx);
    CHECK(JS_IsException(r) && t && strstr(t, "RangeError"), "inLoop over the depth: it throws %s, not a RangeError", t ? t : "nothing");
    free(t);
    CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 0, "f is not left unconverted after the RangeError");
    JS_SetMaxCallDepth(v.rt, JS_DEFAULT_MAX_CALL_DEPTH);
    r = JS_Call(v.ctx, e, JS_UNDEFINED, 1, &arg);
    t = value_text(v.ctx, r);
    CHECK(t && strcmp(t, "42") == 0, "after the RangeError inLoop(true) gives %s, not 42", t ? t : "(exception)");
    free(t);
    free(exception_text(v.ctx));
    JS_FreeValue(v.ctx, r);
    CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 1, "after the RangeError the next call does not convert f");
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, e);
    vm_free(&v);
    /* the first resume of a generator converts its body (rv_gen_ok of
       engine/regvm-ops-gen.h), and a call of a generator function resumes
       it up to its first yield (js_generator_function_call): out of memory
       there ends the call with the error, as at the entry of a call, and
       does not run the body. Each allocation of g(3) fails in turn. */
    {
        int all = 0, gcases = 0, d;
        for (k = 0; k == 0 || k <= all; k++) {
            JSValue g, three = JS_NewInt32(NULL, 3);
            v = vm_new();
            CHECK(eval_ok(v.ctx, "globalThis.g = function* (x) { 'use strict'; yield x + 39; }; 0"),
                  "the setup of the generator threw");
            g = JS_GetGlobalObject(v.ctx);
            f = JS_GetPropertyStr(v.ctx, g, "g");
            JS_FreeValue(v.ctx, g);
            CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 0, "g is converted before its first call");
            /* the scratch of the pass back, for the conversion of g to make */
            JS_RunGC(v.rt);
            r = white_call_failing(v.ctx, f, JS_UNDEFINED, 1, &three, k, &n);
            if (k == 0)
                all = n;
            conv = JS_RegvmFuncStat(v.ctx, f, 0);
            if (!JS_IsException(r)) {
                /* the allocation that failed was not the conversion's */
                CHECK(conv == 1, "g, allocation %d failing: g(3) runs with g not converted", k);
                JS_FreeValue(v.ctx, r);
            } else {
                CHECK(JS_IsUncatchableException(v.ctx),
                      "g, allocation %d failing: g(3) does not end with the uncatchable error", k);
                t = exception_text(v.ctx);
                CHECK(t && strstr(t, "out of memory"), "g, allocation %d failing: g(3) throws %s, not out of memory", k, t ? t : "nothing");
                free(t);
            }
            if (conv == 0) {
                /* a new generator runs and converts g, with no stale error */
                r = JS_Eval(v.ctx, "g(3).next().value", 17, "<gen>", JS_EVAL_TYPE_GLOBAL);
                t = value_text(v.ctx, r);
                CHECK(t && strcmp(t, "42") == 0, "g, allocation %d failing: a new generator gives %s, not 42", k, t ? t : "(exception)");
                free(t);
                free(exception_text(v.ctx));
                JS_FreeValue(v.ctx, r);
                CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 1, "g, allocation %d failing: the next generator does not convert g", k);
                gcases++;
            }
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
        }
        CHECK(gcases >= 2, "g: only %d of the %d allocations of g(3) leave g unconverted", gcases, all);
        cases += gcases;
        /* the resume of g(3) one call too deep: its conversion runs, then
           the depth stops it before it runs. When the conversion
           ran out of memory, no error is left waiting for a later call */
        for (d = 1; d < 16; d++) {
            JSValue g, three = JS_NewInt32(NULL, 3);
            v = vm_new();
            CHECK(eval_ok(v.ctx, "globalThis.g = function* (x) { 'use strict'; yield x + 39; }; 0"),
                  "the setup of the generator threw");
            g = JS_GetGlobalObject(v.ctx);
            f = JS_GetPropertyStr(v.ctx, g, "g");
            JS_FreeValue(v.ctx, g);
            JS_SetMaxCallDepth(v.rt, d);
            r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &three);
            free(exception_text(v.ctx));
            JS_FreeValue(v.ctx, r);
            conv = JS_RegvmFuncStat(v.ctx, f, 0);
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
            if (!JS_IsException(r) || conv == 1)
                break;
        }
        CHECK(d < 16 && JS_IsException(r) && conv == 1, "no depth where g is converted and its resume is too deep (%d)", d);
        for (k = 1; k <= all; k++) {
            JSValue g, three = JS_NewInt32(NULL, 3);
            v = vm_new();
            CHECK(eval_ok(v.ctx, "globalThis.g = function* (x) { 'use strict'; yield x + 39; }; 0"),
                  "the setup of the generator threw");
            g = JS_GetGlobalObject(v.ctx);
            f = JS_GetPropertyStr(v.ctx, g, "g");
            JS_FreeValue(v.ctx, g);
            JS_SetMaxCallDepth(v.rt, d);
            r = white_call_failing(v.ctx, f, JS_UNDEFINED, 1, &three, k, &n);
            CHECK(JS_IsException(r), "g(3) at depth %d, allocation %d failing: it does not throw", d, k);
            free(exception_text(v.ctx));
            CHECK(!white_regvm_oom_pending(v.ctx),
                  "g(3) at depth %d, allocation %d failing: an out of memory error is left for a later call", d, k);
            JS_SetMaxCallDepth(v.rt, JS_DEFAULT_MAX_CALL_DEPTH);
            r = JS_Eval(v.ctx, "g(3).next().value", 17, "<gen>", JS_EVAL_TYPE_GLOBAL);
            t = value_text(v.ctx, r);
            CHECK(t && strcmp(t, "42") == 0, "g(3) at depth %d, allocation %d failing: the next generator gives %s, not 42", d, k, t ? t : "(exception)");
            free(t);
            free(exception_text(v.ctx));
            JS_FreeValue(v.ctx, r);
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
        }
    }
    printf("unit-white: test_regvm_convert_oom_entries: %d cases\n", cases);
}

/* Operands of code no compiler makes that index the tables of the function
   out of their bounds, or a stack deeper than stack_size after a gosub: the
   read check (rv_read_check) rejects each without reading out of a table,
   and a read whose variable definitions do not match the arguments and
   locals fails. */
static void test_regvm_operands(void)
{
    enum { GOSUB, RET_UNDEF, DROP, SET_LOC_UNINIT, PUSH_CONST8, RET, FCLOSURE8, NOP, PUSH_ATOM };
    struct { const char *what, *src; uint8_t code[16]; int len, stack_size; } cases[] = {
        { "gosub past stack_size", "globalThis.f = function (x) { 'use strict'; return x + x + x + x + x + x + x + x; }; 0",
          { 0, 5, 0, 0, 0, 0, 0, 0 }, 8, 0 },
        { "a local past var_count", "globalThis.f = function (x) { 'use strict'; return x + 1; }; 0",
          { 0, 127, 0, 0 }, 4, 1 },
        { "a constant past cpool_count", "globalThis.f = function (x) { 'use strict'; return x + 1; }; 0",
          { 0, 0, 0, 0 }, 4, 1 },
        { "fclosure of a constant that is no function", "globalThis.f = function () { 'use strict'; return 1.5; }; 0",
          { 0, 0, 0 }, 3, 1 },
        { "push_atom_value of an index atom", "globalThis.f = function (x) { 'use strict'; return x + x + x + x; }; 0",
          { 0, 0, 0, 0, 0, 0 }, 6, 1 },
    };
    static const char vsrc[] = "(function (a) { 'use strict'; return () => a; })";
    int k, r;
    VM v, w;
    JSValue g, f, obj;
    JSAtom atom;
    uint8_t *buf;
    size_t len;
    char *t;

    /* the code of each case, by the opcodes of the build */
    cases[0].code[0] = white_opcode(GOSUB);
    cases[0].code[5] = white_opcode(RET_UNDEF);
    cases[0].code[6] = white_opcode(DROP);
    cases[0].code[7] = white_opcode(RET_UNDEF);
    cases[1].code[0] = white_opcode(SET_LOC_UNINIT);
    cases[1].code[3] = white_opcode(RET_UNDEF);
    cases[2].code[0] = white_opcode(PUSH_CONST8);
    cases[2].code[2] = white_opcode(RET);
    cases[2].code[3] = white_opcode(NOP);
    cases[3].code[0] = white_opcode(FCLOSURE8);
    cases[3].code[2] = white_opcode(RET);
    cases[4].code[0] = white_opcode(PUSH_ATOM);
    cases[4].code[5] = white_opcode(RET);
    for (k = 0; k < (int)countof(cases); k++) {
        v = vm_new();
        CHECK(eval_ok(v.ctx, cases[k].src), "%s: the setup threw", cases[k].what);
        g = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, g, "f");
        JS_FreeValue(v.ctx, g);
        if (k == 4) {
            atom = JS_NewAtomUInt32(v.ctx, 5);
            memcpy(cases[k].code + 1, &atom, 4);
        }
        if (white_set_code(v.ctx, f, cases[k].code, cases[k].len, cases[k].stack_size, 0) != 0) {
            CHECK(0, "%s: the function is too short for the code", cases[k].what);
        } else {
            r = white_regvm_read_check(v.ctx, f);
            t = exception_text(v.ctx);
            CHECK(r == -1 && t && strstr(t, "SyntaxError: invalid bytecode: code no compiler makes"),
                  "%s: the read lets it through (%d, %s)", cases[k].what, r, t ? t : "no exception");
            free(t);
            white_set_code(v.ctx, f, NULL, 0, 0, 1);
        }
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }

    /* variable definitions that do not match: none for an argument that a
       closure captures */
    v = vm_new();
    f = JS_Eval(v.ctx, vsrc, strlen(vsrc), "<vardefs>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(f), "the script of the variable definitions threw");
    CHECK(white_drop_vardefs(f, 1) == 0, "the script of the variable definitions has no function");
    buf = JS_WriteObject(v.ctx, &len, f, JS_WRITE_OBJ_BYTECODE);
    white_drop_vardefs(f, 0);
    CHECK(buf != NULL, "the function cannot be written");
    JS_FreeValue(v.ctx, f);
    if (buf) {
        w = vm_new();
        obj = JS_ReadObject(w.ctx, buf, len, JS_READ_OBJ_BYTECODE);
        t = exception_text(w.ctx);
        CHECK(JS_IsException(obj) && t && strstr(t, "invalid bytecode: 0 variable definitions for 1 arguments and locals"),
              "JS_ReadObject reads a function with no variable definitions (%s)", t ? t : "no exception");
        free(t);
        JS_FreeValue(w.ctx, obj);
        vm_free(&w);
        js_free(v.ctx, buf);
    }
    vm_free(&v);
}

/* The register stack settles (engine/regvm-entry.h): its first segment is
   about 32 KiB, a deeper run steps onto next segments of about twice the
   size, and when the outermost run returns the first segment becomes one
   that holds what the run needed, up to about 256 KiB, so that the same
   run again steps onto no next segment. The stack counts in the memory
   size but not toward the threshold of the collector. */
static const char regvm_stack_setup[] =
    "globalThis.rec = function rec(n) { 'use strict';\n"
    "  var a = n + 1, b = n + 2, c = n + 3, d = n + 4, e = n + 5, f = n + 6, g = n + 7, h = n + 8;\n"
    "  if (n === 0) return a + b + c + d + e + f + g + h;\n"
    "  return rec(n - 1) + 1;\n"
    "}; 0";

/* rec(n) of the VM v, and its answer checked */
static void regvm_stack_rec(VM *v, JSValueConst rec, int n)
{
    JSValue arg = JS_NewInt32(v->ctx, n), r = JS_Call(v->ctx, rec, JS_UNDEFINED, 1, &arg);
    char *t = value_text(v->ctx, r), want[32];

    snprintf(want, sizeof(want), "%d", 36 + n);
    CHECK(t && strcmp(t, want) == 0, "rec(%d) gives %s, not %s", n, t ? t : "(exception)", want);
    free(t);
    free(exception_text(v->ctx));
    JS_FreeValue(v->ctx, r);
}

static JSValue regvm_stack_vm(VM *v)
{
    JSValue g, rec;

    *v = vm_new();
    CHECK(eval_ok(v->ctx, regvm_stack_setup), "the setup of rec threw");
    g = JS_GetGlobalObject(v->ctx);
    rec = JS_GetPropertyStr(v->ctx, g, "rec");
    JS_FreeValue(v->ctx, g);
    return rec;
}

/* the depth of rec whose frames first need a next segment, and the slots
   of the first segment that run settles at */
static int regvm_stack_edge(long *settled)
{
    VM v;
    JSValue rec = regvm_stack_vm(&v);
    long st[7];
    int n, made;

    for (n = 1; n < 4000; n++) {
        made = JS_RegvmStat(v.ctx, 7);
        regvm_stack_rec(&v, rec, n);
        if (JS_RegvmStat(v.ctx, 7) != made)
            break;
    }
    white_regvm_stack(v.ctx, st);
    *settled = st[0];
    JS_FreeValue(v.ctx, rec);
    vm_free(&v);
    return n;
}

static void test_regvm_stack(void)
{
    VM v;
    JSValue rec;
    long st[7], settled;
    int mode, edge, made;
    size_t th, n;

    /* a new VM: a first segment of about 32 KiB, counted in the memory
       size and not toward the threshold of the collector. It is in the
       shared chunks: a chunk of its own would be one more chunk the host
       maps and unmaps for each VM. */
    rec = regvm_stack_vm(&v);
    CHECK(white_regvm_stack(v.ctx, st) == 0 && st[0] == st[5] && st[1] == 1 && st[3] == 0,
          "a new VM has a stack of %ld slots in %ld segments", st[0], st[1]);
    CHECK(st[4] <= 0, "the first segment of %ld slots asks the heap for %ld bytes past what a new shared chunk takes", st[0], st[4]);
    CHECK(st[2] >= st[5] * 16 && JS_GetMemorySize(v.rt) > (size_t)st[2],
          "the stack takes %ld bytes of the %zu counted", st[2], JS_GetMemorySize(v.rt));
    th = JS_GetGCThreshold(v.rt);
    /* grow and settle: a run that needs more than the first segment steps
       onto a next segment once, then the first segment holds it */
    regvm_stack_rec(&v, rec, 300);
    white_regvm_stack(v.ctx, st);
    CHECK(st[0] > st[5] && st[0] <= st[6] && st[1] == 1 && st[3] == 0,
          "after rec(300) the stack is %ld slots in %ld segments, hw %ld", st[0], st[1], st[3]);
    made = JS_RegvmStat(v.ctx, 7);
    regvm_stack_rec(&v, rec, 300);
    CHECK(JS_RegvmStat(v.ctx, 7) == made, "rec(300) again steps onto a next segment");
    /* past RV_SEG_MAX: next segments on the way and freed on the way
       back, the first one settled at RV_SEG_MAX, one spare at most */
    regvm_stack_rec(&v, rec, 4000);
    white_regvm_stack(v.ctx, st);
    CHECK(st[0] == st[6] && st[1] <= 2 && st[3] == 0,
          "after rec(4000) the stack is %ld slots in %ld segments, hw %ld", st[0], st[1], st[3]);
    regvm_stack_rec(&v, rec, 4000);
    white_regvm_stack(v.ctx, st);
    CHECK(st[0] == st[6] && st[1] <= 2, "rec(4000) again leaves %ld slots in %ld segments", st[0], st[1]);
    /* the stack of RV_SEG_MAX and more ran no collection of its own */
    CHECK(JS_GetGCThreshold(v.rt) == th, "the stack moved the threshold of the collector from %zu to %zu",
          th, JS_GetGCThreshold(v.rt));
    JS_RunGC(v.rt);
    white_regvm_stack(v.ctx, st);
    n = JS_GetMemorySize(v.rt) - st[2];
    CHECK(JS_GetGCThreshold(v.rt) == n + (n >> 1), "after a collection the threshold is %zu, not 1.5 times the %zu that is not the stack",
          JS_GetGCThreshold(v.rt), n);
    JS_FreeValue(v.ctx, rec);
    vm_free(&v);

    /* a settle whose segment cannot be allocated: each allocation of a run
       that steps onto a next segment fails in turn. The run gives its
       answer or the uncatchable out of memory error, the stack stays
       whole, and the next run settles it. */
    edge = regvm_stack_edge(&settled);
    CHECK(edge > 10 && edge < 4000, "rec never steps onto a next segment (%d)", edge);
    rec = regvm_stack_vm(&v);
    regvm_stack_rec(&v, rec, 1);
    {
        JSValue arg = JS_NewInt32(v.ctx, edge), r;
        r = white_call_failing(v.ctx, rec, JS_UNDEFINED, 1, &arg, 0, &made);
        JS_FreeValue(v.ctx, r);
    }
    JS_FreeValue(v.ctx, rec);
    vm_free(&v);
    CHECK(made >= 2, "rec(%d) makes %d calls of the allocator", edge, made);
    for (mode = 1; mode <= made; mode++) {
        JSValue arg, r;
        int k;
        rec = regvm_stack_vm(&v);
        regvm_stack_rec(&v, rec, 1); /* converted, so that only the run fails */
        arg = JS_NewInt32(v.ctx, edge);
        r = white_call_failing(v.ctx, rec, JS_UNDEFINED, 1, &arg, mode, &k);
        if (JS_IsException(r)) {
            char *t = exception_text(v.ctx);
            CHECK(t && strstr(t, "out of memory"), "rec(%d), call %d failing: it throws %s", edge, mode, t ? t : "nothing");
            free(t);
        }
        JS_FreeValue(v.ctx, r);
        CHECK(white_regvm_stack(v.ctx, st) == 0 && st[1] >= 1 && (st[0] == st[5] || st[3] == 0),
              "rec(%d), call %d failing: the stack is %ld slots in %ld segments, hw %ld", edge, mode, st[0], st[1], st[3]);
        regvm_stack_rec(&v, rec, edge);
        white_regvm_stack(v.ctx, st);
        CHECK(st[0] == settled && st[1] == 1 && st[3] == 0,
              "rec(%d), call %d failing: the next run leaves %ld slots in %ld segments, hw %ld, not %ld slots", edge, mode, st[0], st[1], st[3], settled);
        JS_FreeValue(v.ctx, rec);
        vm_free(&v);
    }
    printf("unit-white: test_regvm_stack: rec steps onto a next segment at %d, %d allocations swept\n", edge, made);
}

/* Code that no compiler makes, which makes a catch offset the pass does not
   know of (white_crafted_code): an iterator_get_value_done with no
   iterator under it, which the read refuses, and a finally that changes the
   catches under its return address, whose ret throws "invalid ret value"
   (which its own catch returns) where the code after the gosub would have
   returned the catch offset. A finally that leaves them as they were
   returns as before. */
static void test_regvm_crafted_code(void)
{
    static const char gen_src[] =
        "globalThis.f = async function (a) { 'use strict'; return [a, a, a, a, a, a]; }; 0";
    static const char fin_src[] =
        "globalThis.f = function (a) { 'use strict'; try { a.b; } finally { a.c; a.d; } return 1; }; 0";
    VM v;
    JSValue g, f, r;
    char *t;
    int kind, rc;

    for (kind = 0; kind < 3; kind++) {
        v = vm_new();
        CHECK(eval_ok(v.ctx, kind ? fin_src : gen_src), "kind %d: the setup threw", kind);
        g = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, g, "f");
        JS_FreeValue(v.ctx, g);
        if (white_crafted_code(v.ctx, f, kind, 0) != 0) {
            CHECK(0, "kind %d: the function has no room for the code", kind);
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
            continue;
        }
        rc = white_regvm_read_check(v.ctx, f);
        t = exception_text(v.ctx);
        if (kind == 0) {
            CHECK(rc == -1 && t && strstr(t, "SyntaxError: invalid bytecode: code no compiler makes"),
                  "an iterator_get_value_done with no iterator: the read lets it through (%d, %s)",
                  rc, t ? t : "no exception");
        } else {
            CHECK(rc == 0, "kind %d: the read refuses it (%s)", kind, t ? t : "no exception");
        }
        free(t);
        if (kind > 0 && rc == 0) {
            r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
            t = exception_text(v.ctx);
            if (kind == 1) {
                /* the ret throws inside the catch the finally made, whose
                   handler returns the error */
                const char *s = JS_IsObject(r) ? JS_ToCString(v.ctx, r) : NULL;
                CHECK(s && strstr(s, "InternalError: invalid ret value"),
                      "a finally that makes a catch under its return address: the call gives tag %d (%s)",
                      (int)JS_VALUE_GET_TAG(r), s ? s : t ? t : "no exception");
                JS_FreeCString(v.ctx, s);
            } else
                CHECK(JS_IsUndefined(r), "a finally that returns at once: the call gives tag %d (%s)",
                      (int)JS_VALUE_GET_TAG(r), t ? t : "no exception");
            free(t);
            if (!JS_IsException(r) && JS_VALUE_GET_TAG(r) != JS_TAG_CATCH_OFFSET)
                JS_FreeValue(v.ctx, r);
        }
        white_crafted_code(v.ctx, f, kind, 1);
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
}

/* Code that no compiler makes, read by JS_ReadObject (rv_read_check of
   engine/regvm-entry.h): the read rejects it with a SyntaxError, one kind
   at a time (white_crafted), and lets the code the compiler made through.
   Then the whole way: the bytes written and read back in another VM, the
   check skipped with JS_READ_OBJ_POLLS_CHECKED. */
static void test_regvm_crafted(void)
{
    static const struct { const char *src; const char *what; } kinds[] = {
        { "globalThis.f = function () { 'use strict'; return 1; }; 0", "pops more than the stack holds" },
        { "globalThis.f = function () { 'use strict'; return 1; }; 0", "a stack over stack_size" },
        { "globalThis.f = function (x) { 'use strict'; return x ? 1 : 2; }; 0", "paths that meet with different depths" },
        { "globalThis.f = function (x, y) { 'use strict'; return x + y; }; 0", "nip_catch with no catch" },
        { "globalThis.f = function (a) { return arguments[0] + a; }; 0", "mapped arguments with no closure state" },
        { "globalThis.f = function (a) { 'use strict'; try { a.b; } catch (e) { return 2; } return 1; }; 0",
          "a catch offset returned" },
        { "globalThis.f = function (x) { 'use strict'; return x ? 1 : 2; }; 0", "a jump before the start of the code" },
        { "globalThis.f = function (a) { 'use strict'; try { a.b; } catch (e) { return 2; } return 1; }; 0",
          "a catch before the start of the code" },
        { "globalThis.f = function (a) { 'use strict'; try { for (const v of a) {} } catch (e) {} return 1; }; 0",
          "a for_of_next that takes a catch offset for its iterator" },
    };
    static const char script[] = "'use strict'; var x = 3; x ? 1 : 2;";
    int k, r;
    VM v, w;
    JSValue g, f, obj;
    uint8_t *buf;
    size_t len;
    char *t;

    for (k = 0; k < (int)(sizeof(kinds) / sizeof(kinds[0])); k++) {
        v = vm_new();
        CHECK(eval_ok(v.ctx, kinds[k].src), "kind %d: the setup threw", k);
        g = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, g, "f");
        JS_FreeValue(v.ctx, g);
        CHECK(white_regvm_read_check(v.ctx, f) == 0, "%s: the code the compiler made does not pass", kinds[k].what);
        free(exception_text(v.ctx));
        if (white_crafted(v.ctx, f, k, 0) != 0) {
            CHECK(0, "%s: the function has no place for it", kinds[k].what);
        } else {
            r = white_regvm_read_check(v.ctx, f);
            t = exception_text(v.ctx);
            CHECK(r == -1 && t && strstr(t, "SyntaxError: invalid bytecode: code no compiler makes"),
                  "%s: the read lets it through (%d, %s)", kinds[k].what, r, t ? t : "no exception");
            free(t);
            white_crafted(v.ctx, f, k, 1);
        }
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }

    /* the whole way: written, then read in another VM */
    v = vm_new();
    obj = JS_Eval(v.ctx, script, strlen(script), "<crafted>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(obj), "the crafted script does not compile");
    CHECK(white_crafted(v.ctx, obj, 2, 0) == 0, "the crafted script has no push_2");
    buf = JS_WriteObject(v.ctx, &len, obj, JS_WRITE_OBJ_BYTECODE);
    white_crafted(v.ctx, obj, 2, 1);
    CHECK(buf != NULL, "the crafted script cannot be written");
    JS_FreeValue(v.ctx, obj);
    if (buf) {
        w = vm_new();
        obj = JS_ReadObject(w.ctx, buf, len, JS_READ_OBJ_BYTECODE);
        t = exception_text(w.ctx);
        CHECK(JS_IsException(obj) && t && strstr(t, "invalid bytecode: code no compiler makes"),
              "JS_ReadObject reads the crafted script (%s)", t ? t : "no exception");
        free(t);
        JS_FreeValue(w.ctx, obj);
        obj = JS_ReadObject(w.ctx, buf, len, JS_READ_OBJ_BYTECODE | JS_READ_OBJ_POLLS_CHECKED);
        CHECK(!JS_IsException(obj), "JS_ReadObject with JS_READ_OBJ_POLLS_CHECKED checks the code again");
        free(exception_text(w.ctx));
        JS_FreeValue(w.ctx, obj);
        vm_free(&w);
        js_free(v.ctx, buf);
    }
    vm_free(&v);
}

/* The functions of fns run in the register loop, with a pcmap entry at
   every instruction that can fail for want of memory: run() is called with
   each call of the allocator failing in turn, and gives its answer or the
   out of memory error. An instruction that failed with no entry would end
   the process in the unwinding (rv_unwind). */
static void test_regvm_run_alloc(void)
{
    static const char setup[] =
        "(function () { 'use strict';\n"
        "  function mk(i) { return { a: i, b: [i, i + 1], s: 'k' + i }; }\n"
        "  function sum(o) { var t = 0, v; for (v of o.b) t = t + v; return t + o.a; }\n"
        "  function clos(i) { var c = i * 2; return function () { return c + i; }; }\n"
        "  function thr(i) { try { if (i > 1) throw new Error('e' + i); return i; } catch (e) { return e.message.length; } finally { i = i + 1; } }\n"
        "  function keys(o) { var n = 0, k; for (k in o) n = n + k.length; return n; }\n"
        "  function num(i) { return (i / 2 + 0.25) * 3 - i % 2 + (i << 2) + -i + 10000000000 * i; }\n"
        "  function str(o, i) { return (o.s + i + typeof i).length + (o.s === 'k1' ? 1 : 0); }\n"
        "  function run() { var out = 0, i, o; for (i = 0; i < 4; i++) { o = mk(i); out = out + sum(o) + clos(i)() + thr(i) + keys(o) + num(i) + str(o, i); } return out; }\n"
        "  globalThis.run = run;\n"
        "  globalThis.fns = [mk, sum, clos, thr, keys, num, str, run];\n"
        "})(); 0";
    VM v = vm_new();
    int fail, n = 0, ooms = 0, i;
    JSValue f, g, fns;
    char *want = NULL;

    CHECK(eval_ok(v.ctx, setup), "the setup threw");
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "run");
    fns = JS_GetPropertyStr(v.ctx, g, "fns");
    JS_FreeValue(v.ctx, g);
    /* the first call converts them, the second runs them in the register loop */
    for (i = 0; i < 2; i++)
        JS_FreeValue(v.ctx, JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL));
    for (i = 0; i < 8; i++) {
        JSValue fn = JS_GetPropertyUint32(v.ctx, fns, i);

        CHECK(JS_RegvmFuncStat(v.ctx, fn, 0) == 1, "function %d of fns has no register code (%d, op %d at %d), so the sweep does not test it",
              i, JS_RegvmFuncStat(v.ctx, fn, 0), JS_RegvmFuncStat(v.ctx, fn, 2), JS_RegvmFuncStat(v.ctx, fn, 3));
        JS_FreeValue(v.ctx, fn);
    }
    JS_FreeValue(v.ctx, fns);
    for (fail = 0; fail == 0 || fail <= n; fail++) {
        int made;
        JSValue r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);

        if (fail == 0) {
            n = made;
            want = value_text(v.ctx, r);
            CHECK(want != NULL, "run() threw");
        } else if (JS_IsException(r)) {
            CHECK(JS_IsUncatchableException(v.ctx), "call %d failing: an exception that is not out of memory", fail);
            free(exception_text(v.ctx));
            ooms++;
        } else {
            char *t = value_text(v.ctx, r);

            CHECK(t && want && strcmp(t, want) == 0, "call %d failing: %s, not %s", fail, t ? t : "(no text)", want ? want : "?");
            free(t);
        }
        JS_FreeValue(v.ctx, r);
    }
    CHECK(n > 40 && ooms > 40, "only %d of %d calls failed the run", ooms, n);
    free(want);
    JS_FreeValue(v.ctx, f);
    vm_free(&v);
}

/* An OP_ret whose value is no return address (code read with JS_ReadObject
   can have one anywhere) throws and leaves the value where it is, in its
   temp of the register loop, for the unwinding to free. The function here
   makes an object and ends with OP_ret in place of its OP_return: every call
   throws, the first, which converts the function, and the others, and none
   keeps the object. */
static void test_regvm_bad_ret(void)
{
    static const char src[] = "(function (x) { 'use strict'; var a = x + 1; a = a + 2; a = a + 3; return {}; })";
    VM v = vm_new();
    JSValue top, fun = JS_UNDEFINED, r;
    JSMemoryUsage mu;
    const uint8_t *code, *at;
    uint8_t *buf;
    size_t size;
    int64_t objs[4];
    char *t;
    int len, i;

    CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile the function");
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    at = buf ? memmem(buf, size, code, len) : NULL;
    CHECK(at != NULL, "the code of the function in what JS_WriteObject wrote");
    JS_FreeValue(v.ctx, top);
    if (at) {
        buf[at - buf + len - 1] = white_op(W_RET);
        top = JS_ReadObject(v.ctx, buf, size, JS_READ_OBJ_BYTECODE);
        CHECK(!JS_IsException(top), "read the function with OP_ret");
        fun = JS_EvalFunction(v.ctx, top);
        CHECK(JS_IsFunction(v.ctx, fun), "the script gives the function");
        for (i = 0; i < 4; i++) {
            r = JS_Call(v.ctx, fun, JS_UNDEFINED, 0, NULL);
            CHECK(JS_IsException(r), "call %d: no exception", i);
            JS_FreeValue(v.ctx, r);
            t = exception_text(v.ctx);
            CHECK(t && strstr(t, "InternalError: invalid ret value"), "call %d: %s", i, t ? t : "(none)");
            free(t);
            JS_RunGC(v.rt);
            JS_ComputeMemoryUsage(v.rt, &mu);
            objs[i] = mu.obj_count;
            if (i == 0)
                CHECK(JS_RegvmFuncStat(v.ctx, fun, 0) == 1, "the function has no register code (%d), so the calls after test nothing",
                      JS_RegvmFuncStat(v.ctx, fun, 0));
            else
                CHECK(objs[i] == objs[0], "call %d, in the register loop: %lld objects, %lld after the first call: the object of the OP_ret was kept",
                      i, (long long)objs[i], (long long)objs[0]);
        }
    }
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, fun);
    /* a leak ends the process when the runtime is freed: the checks first */
    fflush(stdout);
    vm_free(&v);
}

/* the result of a call as text: the value, or the exception */
static char *call_text(JSContext *ctx, JSValueConst f)
{
    JSValue r = JS_Call(ctx, f, JS_UNDEFINED, 0, NULL);
    char *t = JS_IsException(r) ? exception_text(ctx) : value_text(ctx, r);

    JS_FreeValue(ctx, r);
    return t;
}

/* A return address is an int that code read with JS_ReadObject can make
   and read as any other: OP_ret takes the ints QuickJS's interpreter takes
   (the end of an old gosub) and refuses the others, and a return address
   read as a value is the one QuickJS's interpreter makes. Each function is
   the code of src with nops in front of one of the codes below: the first
   call converts it, and each call after it must give what the first gave.

     gosub L; push_i16 7; return; L: drop; push_i16 val; ret
     gosub L; push_i16 7; return; L: return */
/* the call of the register code that copies its last arguments itself
   (CALLS, CALLSD, rv_call_copies of engine/regvm-ops-basic.h): which
   arguments it copies, read from the code of f. A pattern byte of -1 takes
   any byte (the window, a constant's index), and a case with no pattern
   wants neither op in the code. The slots of f(a, b): a is -3, b is -2. */
static void test_regvm_call_copies(void)
{
    static const struct { const char *what, *src; int op, pat[8], n; } cases[] = {
        { "two arguments, the last first", "return h(a, b);", 0, { -1, 2, 2, 0xfe, 0xfd }, 5 },
        { "the result dropped", "h(a, b); return 0;", 1, { -1, 2, 2, 0xfe, 0xfd }, 5 },
        { "a local before a constant", "return h(a, 's');", -1, { 0 }, 0 },
        { "a constant before a local", "return h('s', a);", 0, { -1, 2, 1, 0xfd }, 4 },
        { "the same slot twice", "return h(b, b);", 0, { -1, 2, 2, 0xfe, 0xfe }, 5 },
        { "five arguments, the last four", "return h(a, b, a, b, a);", 0, { -1, 5, 4, 0xfd, 0xfe, 0xfd, 0xfe }, 7 },
        { "after a temp", "return h(h(), a);", 0, { -1, 2, 1, 0xfd }, 4 },
        { "an argument set by the one after it", "return h(a, a = b);", 0, { -1, 2, 1, 0xfd }, 4 },
        { "before a temp", "return h(a, h());", -1, { 0 }, 0 },
        { "a method call", "return b.m(a);", -1, { 0 }, 0 },
        { "a captured argument", "var g = function () { return a; }; return h(a);", -1, { 0 }, 0 },
    };
    int k, i, j, op[2] = { -1, -1 }, len, found, other;
    const uint8_t *code;
    char src[256];
    VM v;
    JSValue g, f;

    for (i = 0; i < 256; i++) {
        const char *name = JS_RegvmOpName(i);
        if (name && !strcmp(name, "CALLS"))
            op[0] = i;
        if (name && !strcmp(name, "CALLSD"))
            op[1] = i;
    }
    CHECK(op[0] >= 0 && op[1] >= 0, "the names of CALLS and CALLSD");
    for (k = 0; k < (int)countof(cases); k++) {
        v = vm_new();
        snprintf(src, sizeof(src), "function h() { return 1; } globalThis.f = function (a, b) { 'use strict'; %s }; 0", cases[k].src);
        CHECK(eval_ok(v.ctx, src), "%s: the setup threw", cases[k].what);
        g = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, g, "f");
        JS_FreeValue(v.ctx, g);
        CHECK(eval_ok(v.ctx, "f(1, { m() { return 2; } }); 0"), "%s: the call threw", cases[k].what);
        code = white_regvm_code(f, &len);
        CHECK(code != NULL, "%s: f has no register code", cases[k].what);
        found = other = 0;
        for (i = 0; code && i < len; i++) {
            if (code[i] != op[0] && code[i] != op[1])
                continue;
            if (cases[k].op < 0 || code[i] != op[cases[k].op] || i + cases[k].n >= len) {
                other = 1;
                continue;
            }
            for (j = 0; j < cases[k].n; j++)
                if (cases[k].pat[j] >= 0 && code[i + 1 + j] != cases[k].pat[j])
                    break;
            if (j == cases[k].n)
                found = 1;
            else
                other = 1;
        }
        if (cases[k].op < 0)
            CHECK(!other, "%s: a CALLS or CALLSD in the code", cases[k].what);
        else
            CHECK(found, "%s: no %s with the copies wanted", cases[k].what, cases[k].op ? "CALLSD" : "CALLS");
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
}

/* the number of the register op named name */
static int regvm_op(const char *name)
{
    int i;
    for (i = 0; i < 256; i++) {
        const char *n = JS_RegvmOpName(i);
        if (n && !strcmp(n, name))
            return i;
    }
    return -1;
}

/* globalThis.f = function (a, b) { 'use strict'; body }, called once with
   (1, 2): its register code, or NULL */
static const uint8_t *regvm_code_of_body(VM *v, const char *body, JSValue *f, int *len)
{
    char src[512];
    JSValue g;
    snprintf(src, sizeof(src), "function h() { return { x: 1, y: 2, a: { x: 3 } }; }"
             " globalThis.f = function (a, b) { 'use strict'; %s }; 0", body);
    if (!eval_ok(v->ctx, src))
        return NULL;
    g = JS_GetGlobalObject(v->ctx);
    *f = JS_GetPropertyStr(v->ctx, g, "f");
    JS_FreeValue(v->ctx, g);
    if (!eval_ok(v->ctx, "f(1, 2); 0"))
        return NULL;
    return white_regvm_code(*f, len);
}

/* dup of a temp then get_field (a destructuring): GETF reads the temp, with
   no MOV of it into the temp above and no GETFT that frees the copy
   (rv_dup_get_field of engine/regvm-ops-basic.h) */
static void test_regvm_dup_get_field(void)
{
    static const struct { const char *what, *src; int getfl; } cases[] = {
        { "renamed fields of a call", "var { x: p, y: q } = h(); return p + q;", 2 },
        { "an assignment pattern", "var p, q; ({ x: p, y: q } = h()); return p + q;", 2 },
        { "a nested pattern", "var { a: { x: p }, y: q } = h(); return p + q;", 2 },
    };
    int k, i, len, mov = regvm_op("MOV"), getft = regvm_op("GETFT"), getfl = regvm_op("GETFL");
    int copies, n, m;
    uint32_t slots[8], ids[8];
    const uint8_t *code;
    VM v;
    JSValue f;

    CHECK(mov >= 0 && getft >= 0 && getfl >= 0, "the names of MOV, GETFT and GETFL");
    for (k = 0; k < (int)countof(cases); k++) {
        v = vm_new();
        f = JS_UNDEFINED;
        code = regvm_code_of_body(&v, cases[k].src, &f, &len);
        CHECK(code != NULL, "%s: f has no register code", cases[k].what);
        copies = n = 0;
        for (i = 0; code && i < len; i++) {
            if (code[i] == mov && i + 5 < len && code[i + 3] == getft && code[i + 5] == code[i + 1])
                copies++;
            if (code[i] == getfl)
                n++;
        }
        CHECK(copies == 0, "%s: %d MOV then GETFT of the copy", cases[k].what, copies);
        CHECK(n >= cases[k].getfl, "%s: %d GETFL, want %d", cases[k].what, n, cases[k].getfl);
        /* the GETF of a destructured temp carries the slot and the id of a
           read of a name as the others do: after a second call each read
           keeps the slot of the own field it found */
        CHECK(eval_ok(v.ctx, "f(1, 2); 0"), "%s: the second call failed", cases[k].what);
        m = white_regvm_getf_slots(f, slots, ids, 8);
        CHECK(m >= cases[k].getfl, "%s: %d reads of a name, want %d", cases[k].what, m, cases[k].getfl);
        for (i = 0; i < m && i < 8; i++)
            CHECK(slots[i] != 0xffffffffu, "%s: read %d of %d keeps no slot", cases[k].what, i, m);
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
}

/* ++k or --k of a local read lazily below it (x[k] = x[++k]): the read below
   is copied before the INC, which then writes k itself, INC k k, with no
   SETL after it (rv_incdec_store of engine/regvm-ops-arith.h) */
static void test_regvm_incdec_store(void)
{
    static const struct { const char *what, *src, *op; } cases[] = {
        { "x[k] = x[++k]", "var x = [1, 2, 3], k = a - 1; x[k] = x[++k]; return x[0] + k;", "INC" },
        { "x[k] = x[--k]", "var x = [1, 2, 3], k = a + 1; x[k] = x[--k]; return x[0] + k;", "DEC" },
        { "k + ++k", "var k = a; var s = k + ++k; return s + k;", "INC" },
    };
    int k, i, len, op, self;
    const uint8_t *code;
    VM v;
    JSValue f;

    for (k = 0; k < (int)countof(cases); k++) {
        op = regvm_op(cases[k].op);
        CHECK(op >= 0, "the name of %s", cases[k].op);
        v = vm_new();
        f = JS_UNDEFINED;
        code = regvm_code_of_body(&v, cases[k].src, &f, &len);
        CHECK(code != NULL, "%s: f has no register code", cases[k].what);
        self = 0;
        for (i = 0; code && i + 2 < len; i++) {
            if (code[i] == op && code[i + 1] == code[i + 2] && (int8_t)code[i + 1] < 0)
                self++;
        }
        CHECK(self >= 1, "%s: no %s of the local into itself", cases[k].what, cases[k].op);
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
}

/* l = x[k] = v and l = o.f = v into a local l: the value goes into l first
   and the put reads l, so no MOVT, SWAP or MOV of the value is left
   (rv_chain_store of engine/regvm-ops-basic.h). Where a closure reads l or
   a catch is around, the copies stay, in the order of QuickJS. */
static void test_regvm_chain_store(void)
{
    static const struct { const char *what, *src; int moves; } cases[] = {
        { "l = x[k] = v", "var x = [0, 0], l; l = x[a] = b * 2; return l + x[1];", 0 },
        { "l = o.f = v", "var o = {}, l; l = o.f = b * 2; return l + o.f;", 0 },
        { "(l = x[k] = v) kept", "var x = [0, 0], l; var r = (l = x[a] = b * 2); return r + l;", 0 },
        { "a closure reads l", "var x = [0, 0], l, g = function () { return l; }; l = x[a] = b * 2; return g();", 1 },
        { "a catch around", "var x = [0, 0], l; try { l = x[a] = b * 2; } catch (e) { return l; } return l;", 1 },
    };
    int k, i, len, moves, op[3];
    const uint8_t *code;
    VM v;
    JSValue f;

    op[0] = regvm_op("MOV");
    op[1] = regvm_op("MOVT");
    op[2] = regvm_op("SWAP");
    CHECK(op[0] >= 0 && op[1] >= 0 && op[2] >= 0, "the names of MOV, MOVT and SWAP");
    for (k = 0; k < (int)countof(cases); k++) {
        v = vm_new();
        f = JS_UNDEFINED;
        code = regvm_code_of_body(&v, cases[k].src, &f, &len);
        CHECK(code != NULL, "%s: f has no register code", cases[k].what);
        moves = 0;
        for (i = 0; code && i + 2 < len; i++) {
            if ((code[i] == op[1] || code[i] == op[2]) && (int8_t)code[i + 1] >= 0 && (int8_t)code[i + 2] >= 0)
                moves++;
        }
        if (cases[k].moves)
            CHECK(moves > 0, "%s: no MOVT or SWAP of temps, the old order wants them", cases[k].what);
        else
            CHECK(moves == 0, "%s: %d MOVT or SWAP of temps", cases[k].what, moves);
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
}

/* the test of a loop read again at its goto (rv_rotate of
   engine/regvm-ops-cmp.h): no GOTO is left in a loop whose test reads
   locals, var refs, constants and fields, and one is left when the test
   calls */
static void test_regvm_loop_rotate(void)
{
    static const struct { const char *what, *src; int gotos; } cases[] = {
        { "i < a", "var s = 0, i; for (i = 0; i < a; i++) s += i; return s;", 0 },
        { "i <= a", "var s = 0; for (var i = 1; i <= a; i++) s += i; return s;", 0 },
        { "while (i !== x.length)", "var x = [1, 2], i = 0, s = 0; while (i !== x.length) s += x[i++]; return s;", 0 },
        { "a test that calls", "var s = 0, i; for (i = 0; i < Math.min(a, 3); i++) s += i; return s;", 1 },
    };
    int k, i, len, n, op = regvm_op("GOTO");
    const uint8_t *code;
    VM v;
    JSValue f;

    CHECK(op >= 0, "the name of GOTO");
    for (k = 0; k < (int)countof(cases); k++) {
        v = vm_new();
        f = JS_UNDEFINED;
        code = regvm_code_of_body(&v, cases[k].src, &f, &len);
        CHECK(code != NULL, "%s: f has no register code", cases[k].what);
        n = 0;
        for (i = 0; code && i < len; i++)
            n += code[i] == op;
        if (cases[k].gotos)
            CHECK(n > 0, "%s: no GOTO, the loop is not rotated here", cases[k].what);
        else
            CHECK(n == 0, "%s: %d GOTO", cases[k].what, n);
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
}

static void test_regvm_ret_value(void)
{
    static const char src[] = "(function () { return [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]; })";
    VM v = vm_new();
    JSValue top, fun;
    const uint8_t *code, *at;
    uint8_t *buf;
    size_t size;
    int len, val, i, g, tail, taken = 0;
    int16_t v16;
    Code c;
    char *first, *t;

    CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile the function");
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    at = buf ? memmem(buf, size, code, len) : NULL;
    CHECK(at != NULL && len >= 14, "the code of the function in what JS_WriteObject wrote (%d bytes)", len);
    JS_FreeValue(v.ctx, top);
    for (val = -2; at && val <= len + 12; val++) {
        /* val len + 12 is the second code */
        tail = val == len + 12 ? 10 : 14;
        c = (Code){{0}, 0};
        op_n(&c, W_NOP, len - tail);
        g = jump(&c, W_GOSUB);
        c.b[c.n++] = white_op(W_PUSH_I16);
        v16 = 7;
        memcpy(c.b + c.n, &v16, 2);
        c.n += 2;
        op_n(&c, W_RETURN, 1);
        set_target(&c, g, c.n);
        if (tail == 14) {
            op_n(&c, W_DROP, 1);
            c.b[c.n++] = white_op(W_PUSH_I16);
            v16 = val;
            memcpy(c.b + c.n, &v16, 2);
            c.n += 2;
            op_n(&c, W_RET, 1);
        } else {
            op_n(&c, W_RETURN, 1);
        }
        CHECK(c.n == len, "the code of val %d has %d bytes, not %d", val, c.n, len);
        memcpy(buf + (at - buf), c.b, len);
        top = JS_ReadObject(v.ctx, buf, size, JS_READ_OBJ_BYTECODE);
        CHECK(!JS_IsException(top), "val %d: read the function", val);
        fun = JS_EvalFunction(v.ctx, top);
        CHECK(JS_IsFunction(v.ctx, fun), "val %d: the script gives the function", val);
        first = call_text(v.ctx, fun);
        CHECK(JS_RegvmFuncStat(v.ctx, fun, 0) == 1, "val %d: the function has no register code (%d), so the calls after test nothing",
              val, JS_RegvmFuncStat(v.ctx, fun, 0));
        if (first && strcmp(first, "7") == 0)
            taken++;
        for (i = 0; i < 2; i++) {
            t = call_text(v.ctx, fun);
            CHECK(t && first && strcmp(t, first) == 0, "val %d, call %d after the first: %s, the first %s",
                  val, i, t ? t : "(none)", first ? first : "(none)");
            free(t);
        }
        free(first);
        JS_FreeValue(v.ctx, fun);
    }
    /* the one val of the old return address, and no other */
    CHECK(taken == 1, "OP_ret took %d values", taken);
    js_free(v.ctx, buf);
    fflush(stdout);
    vm_free(&v);
}

/* the global function name of v.ctx called with the array [7, 8]: its
   result, the exception thrown, or NULL with *fun the function */
static JSValue dst_call(VM *v, const char *name, JSValue *fun)
{
    JSValue g = JS_GetGlobalObject(v->ctx), arr, r;

    *fun = JS_GetPropertyStr(v->ctx, g, name);
    JS_FreeValue(v->ctx, g);
    arr = JS_NewArray(v->ctx);
    JS_SetPropertyUint32(v->ctx, arr, 0, JS_NewInt32(v->ctx, 7));
    JS_SetPropertyUint32(v->ctx, arr, 1, JS_NewInt32(v->ctx, 8));
    r = JS_Call(v->ctx, *fun, JS_UNDEFINED, 1, (JSValueConst *)&arr);
    JS_FreeValue(v->ctx, arr);
    return r;
}

/* The next key and the read of it of a for in loop (engine/regvm-ops-window.h,
   engine/regvm-ops-object.h): the pass makes FINEXT for the for_in_next
   followed by its if_false, whatever stores the key, and GETELE or GETELET
   for the read o[k] inside the loop whose key k is the local or argument the
   loop stores, not for a read with another key, a key in a property, a let
   a closure captures (which the pass does not take as the slot of the key)
   or a read outside the loop. The read checks the key it is given, so a
   miss of the pass costs only the fast path. Each function is run once
   first, so that it has its register code, and its result is checked. */
static void forin_case(VM *v, const char *name, const char *want, int fin, int win,
                       int ele, int el)
{
    char expr[64];
    JSValue g, fun;
    int n[4];

    snprintf(expr, sizeof(expr), "%s(src)", name);
    CHECK(eval_is(v->ctx, expr, want), "%s: the result is not %s", name, want);
    g = JS_GetGlobalObject(v->ctx);
    fun = JS_GetPropertyStr(v->ctx, g, name);
    JS_FreeValue(v->ctx, g);
    CHECK(white_regvm_forin_sites(fun, n) == 0, "%s: no register code", name);
    CHECK(n[0] == fin && n[1] == win && n[2] == ele && n[3] == el,
          "%s: FINEXT %d, W_FOR_IN_NEXT %d, GETELE %d, GETEL %d, not %d %d %d %d",
          name, n[0], n[1], n[2], n[3], fin, win, ele, el);
    JS_FreeValue(v->ctx, fun);
}

static void test_regvm_forin(void)
{
    static const char src[] =
        "globalThis.src = { a: 1, b: { x: 2, y: 3 }, c: 4 };"
        "globalThis.plain = function (o) { var s = ''; for (var k in o) s += k + o[k]; return s; };"
        "globalThis.arg = function (o, k) { var s = ''; for (k in o) s += o[k]; return s; };"
        "globalThis.prop = function (o) { var t = {}, s = ''; for (t.p in o) s += o[t.p]; return s; };"
        "globalThis.other = function (o) { var s = '', j = 'a'; for (var k in o) s += o[j]; return s; };"
        "globalThis.after = function (o) { var k; for (k in o); return o[k]; };"
        "globalThis.nest = function (o) { var s = ''; for (var a in o) if (typeof o[a] == 'object')"
        " for (var b in o[a]) s += a + b + o[a][b]; return s; };"
        "globalThis.letk = function (o) { var s = ''; for (let k in o) s += o[k]; return s; };"
        "globalThis.cap = function (o) { var s = '', fs = []; for (let k in o) { fs.push(() => k);"
        " s += o[k]; } return s + fs.map(f => f()).join(''); };"
        "globalThis.gen = function* (o) { for (var k in o) yield o[k]; };"
        "globalThis.genr = function (o) { return [...gen(o)].join(); }; 0";
    VM v;

    v = vm_new();
    CHECK(eval_ok(v.ctx, src), "the setup threw");
    forin_case(&v, "plain", "a1b[object Object]c4", 1, 0, 1, 0);
    forin_case(&v, "arg", "1[object Object]4", 1, 0, 1, 0);
    forin_case(&v, "prop", "1[object Object]4", 1, 0, 0, 1);
    forin_case(&v, "other", "111", 1, 0, 0, 1);
    forin_case(&v, "after", "4", 1, 0, 0, 1);
    forin_case(&v, "nest", "bx2by3", 2, 0, 4, 0);
    forin_case(&v, "letk", "1[object Object]4", 1, 0, 1, 0);
    forin_case(&v, "cap", "1[object Object]4abc", 1, 0, 0, 1);
    CHECK(eval_is(v.ctx, "genr(src)", "1,[object Object],4"), "gen: the values differ");
    forin_case(&v, "gen", "[object Generator]", 1, 0, 1, 0);
    vm_free(&v);
}

/* the slot the one read of a name of the function name keeps after src ran
   (engine/regvm-ops-basic.h), 0xffffffff for none, and whether it keeps
   the id of a shape with it (proto) */
static void getf_slot_case(VM *v, const char *name, const char *src, const char *want, uint32_t slot,
                           int proto)
{
    JSValue g = JS_GetGlobalObject(v->ctx), f = JS_GetPropertyStr(v->ctx, g, name);
    uint32_t s[2] = { 0, 0 }, id[2] = { 0, 0 };
    int n;

    CHECK(eval_is(v->ctx, src, want), "%s: %s does not give %s", name, src, want);
    n = white_regvm_getf_slots(f, s, id, 2);
    CHECK(n == 1, "%s: %d reads of a name, want 1", name, n);
    CHECK(n != 1 || s[0] == slot, "%s after %s: the site keeps slot %u, want %u", name, src, s[0], slot);
    CHECK(n != 1 || (id[0] != 0) == proto, "%s after %s: the site keeps the id %u", name, src, id[0]);
    JS_FreeValue(v->ctx, f);
    JS_FreeValue(v->ctx, g);
}

/* The slot a read of a name keeps (GETF, GETFL, GETFT and the read of a
   method): the slot of the data property it last found, own or of the
   prototype with the id of the shape of the object, none before the first
   run, not changed by an accessor, a key no object has or a primitive with
   no such key. The key "stack" is never kept. */
static void test_regvm_getf_slot(void)
{
    VM v = vm_new();

    CHECK(eval_ok(v.ctx,
        "globalThis.f = function (o) { return o.b; };"
        "globalThis.fl = function (o) { var x; x = o.b; return x; };"
        "globalThis.ft = function (o) { return [o][0].b; };"
        "globalThis.fm = function (o) { return o.b(); };"
        "globalThis.fs = function (o) { return typeof o.stack; }; 0"), "the setup threw");
    getf_slot_case(&v, "f", "f(1)", "undefined", 0xffffffffu, 0);
    getf_slot_case(&v, "f", "f({ a: 1, b: 2 })", "2", 1, 0);
    getf_slot_case(&v, "f", "f({ b: 3 })", "3", 0, 0);
    getf_slot_case(&v, "f", "f(Object.create({ b: 4 }))", "4", 0, 1);
    getf_slot_case(&v, "f", "f({ a: 1, get b() { return 5; } })", "5", 0, 1);
    getf_slot_case(&v, "f", "f({ x: 1 })", "undefined", 0, 1);
    getf_slot_case(&v, "f", "f({ x: 1, y: 2, z: 3, b: 6 })", "6", 3, 0);
    getf_slot_case(&v, "f", "var d = { a: 1, b: 2, c: 3 }; delete d.a; f(d)", "2", 1, 0);
    getf_slot_case(&v, "fl", "fl({ a: 1, b: 7 })", "7", 1, 0);
    getf_slot_case(&v, "ft", "ft({ a: 1, b: 8 })", "8", 1, 0);
    getf_slot_case(&v, "fm", "fm({ a: 1, b() { return 9; } })", "9", 1, 0);
    getf_slot_case(&v, "fm", "class K { b() { return 10; } }; fm(new K())", "10", 1, 1);
    getf_slot_case(&v, "fs", "fs(new Error('e'))", "string", 0xffffffffu, 0);
    getf_slot_case(&v, "fs", "fs({ stack: 1 })", "number", 0xffffffffu, 0);
    vm_free(&v);

    /* the ids run out and are given again: an id a site keeps may now name
       a shape with the key of its own or another prototype, so no read
       takes the slot of a prototype any more, and none keeps one */
    v = vm_new();
    CHECK(eval_ok(v.ctx,
        "globalThis.fm = function (o) { return o.b(); };"
        "class K { b() { return 1; } }; globalThis.K = K; fm(new K()); fm(new K()); 0"), "the setup threw");
    getf_slot_case(&v, "fm", "fm(new K())", "1", 1, 1);
    white_shape_id_set(v.rt, white_shape_id_max() - 8);
    CHECK(eval_ok(v.ctx, "var objs = []; for (var i = 0; i < 64; i++) { var o = {}; o['r' + i] = i; objs.push(o); } 0"),
          "the shapes threw");
    CHECK(white_shape_id_next(v.rt) < white_shape_id_max() - 8, "the ids did not run out (next %u)",
          white_shape_id_next(v.rt));
    /* every id from 1 up again, the old one of K's objects among them */
    CHECK(eval_ok(v.ctx, "var many = []; for (var i = 0; i < 4096; i++) { var o = { b: function () { return 2; } };"
                         " o['s' + i] = i; many.push(o); } 0"), "the shapes threw");
    getf_slot_case(&v, "fm", "fm(new K()) + fm(many[7]) + fm(new K()) + fm(many[4095])", "6", 0, 0);
    getf_slot_case(&v, "fm", "class L { b() { return 3; } }; fm(new L()) + fm(new K())", "4", 0, 0);
    vm_free(&v);
}

/* what the one write of a name of the function s keeps after src ran
   (engine/regvm-ops-basic.h): the slot, and whether it keeps a step and the
   epoch of the step is the one of the runtime (step 2), is another (1) or
   it keeps none (0) */
static void putf_case(VM *v, const char *src, const char *want, uint32_t slot, int step)
{
    JSValue g = JS_GetGlobalObject(v->ctx), f = JS_GetPropertyStr(v->ctx, g, "s");
    uint32_t w[4] = { 0, 0, 0, 0 };
    int n, got;

    CHECK(eval_is(v->ctx, src, want), "%s does not give %s", src, want);
    n = white_regvm_putf_sites(f, w, 1);
    CHECK(n == 1, "%s: %d writes of a name, want 1", src, n);
    got = w[3] == 0xffffffffu ? 0 : w[3] == white_shape_step_epoch(v->rt) ? 2 : 1;
    CHECK(n != 1 || w[0] == slot, "after %s: the site keeps slot %u, want %u", src, w[0], slot);
    CHECK(n != 1 || got == step, "after %s: the step is %d (ids %u %u, epoch %u of %u), want %d",
          src, got, w[1], w[2], w[3], white_shape_step_epoch(v->rt), step);
    CHECK(n != 1 || got == 0 || (w[1] != 0 && w[2] != 0 && w[1] != w[2]),
          "after %s: the step keeps the ids %u %u", src, w[1], w[2]);
    JS_FreeValue(v->ctx, f);
    JS_FreeValue(v->ctx, g);
}

/* The slot and the step a write of a name keeps (PUTF): none after a
   store to an array, the slot of the own key it set, the step of the key
   it added to an ordinary object (the objects of C stay alive, so that an
   add copies the shape of the one before). The epoch moves when a
   prototype gets a setter or loses it and when the ids run out and are
   given again (also inside the add the site makes), and it stops at its
   last value, after which no site takes a step. */
static void test_regvm_putf_step(void)
{
    VM v = vm_new();

    CHECK(eval_ok(v.ctx,
        "globalThis.s = function (o, v) { o.x = v; return o; };"
        "globalThis.keep = []; globalThis.C = function () { this.a = 1; keep.push(this); }; s(new C(), 0); 0"), "the setup threw");
    putf_case(&v, "s([1], 1).x", "1", 0xffffffffu, 0);
    putf_case(&v, "s(new C(), 2).x", "2", 0xffffffffu, 2);
    putf_case(&v, "s(new C(), 3).x", "3", 0xffffffffu, 2);
    putf_case(&v, "s({ b: 1, x: 2 }, 4).x", "4", 1, 2);
    putf_case(&v, "Object.defineProperty(C.prototype, 'x', { set: function (v) { this.y = v; }, configurable: true });"
                  "s(new C(), 5).y", "5", 1, 1);
    putf_case(&v, "var o = s(new C(), 6); o.x + ',' + o.y + ',' + Object.keys(o)", "undefined,6,a,y", 1, 1);
    putf_case(&v, "delete C.prototype.x; s(new C(), 7).x", "7", 1, 2);
    vm_free(&v);

    /* the ids run out and are given again: the epoch moves, so a step kept
       with the old ids is not taken, whatever shape has its ids now */
    v = vm_new();
    CHECK(eval_ok(v.ctx,
        "globalThis.s = function (o, v) { o.x = v; return o; };"
        "globalThis.keep = []; globalThis.C = function () { this.a = 1; keep.push(this); }; s(new C(), 1); 0"), "the setup threw");
    putf_case(&v, "s(new C(), 2).x", "2", 0xffffffffu, 2);
    white_shape_id_set(v.rt, white_shape_id_max() - 8);
    CHECK(eval_ok(v.ctx, "var objs = []; for (var i = 0; i < 64; i++) { var o = {}; o['r' + i] = i; objs.push(o); } 0"),
          "the shapes threw");
    CHECK(white_shape_id_next(v.rt) < white_shape_id_max() - 8, "the ids did not run out (next %u)",
          white_shape_id_next(v.rt));
    putf_case(&v, "objs.length", "64", 0xffffffffu, 1);
    CHECK(eval_ok(v.ctx, "var S = { set x(v) { this.y = v; } }, many = [];"
                         " for (var i = 0; i < 4096; i++) { var o = Object.create(S); o['s' + i] = i; many.push(o); } 0"),
          "the shapes threw");
    putf_case(&v, "var n = 0; for (var i = 0; i < 4096; i++) if (s(many[i], i).y === i &&"
                  " !Object.hasOwn(many[i], 'x')) n++; n", "4096", 0xffffffffu, 1);
    putf_case(&v, "s(new C(), 3).x", "3", 0xffffffffu, 2);
    vm_free(&v);

    /* the epoch stops at its last value: no site takes a step after */
    v = vm_new();
    CHECK(eval_ok(v.ctx,
        "globalThis.s = function (o, v) { o.x = v; return o; };"
        "globalThis.keep = []; globalThis.C = function () { this.a = 1; keep.push(this); }; s(new C(), 0); 0"), "the setup threw");
    white_shape_step_epoch_set(v.rt, 0xfffffffdu);
    putf_case(&v, "s(new C(), 1).x", "1", 0xffffffffu, 2);
    putf_case(&v, "Object.defineProperty(C.prototype, 'z', { get: function () { return 0; } }); s(new C(), 2).x",
              "2", 0xffffffffu, 1);
    CHECK(white_shape_step_epoch(v.rt) == 0xfffffffeu, "the epoch is %u", white_shape_step_epoch(v.rt));
    putf_case(&v, "Object.defineProperty(C.prototype, 'w', { get: function () { return 0; } }); s(new C(), 3).x",
              "3", 0xffffffffu, 1);
    CHECK(white_shape_step_epoch(v.rt) == 0xfffffffeu, "the epoch is %u", white_shape_step_epoch(v.rt));
    vm_free(&v);

    /* the ids run out inside the add itself: the id the object had is one
       of the old numbering, so the site takes no step from it */
    v = vm_new();
    CHECK(eval_ok(v.ctx,
        "globalThis.s = function (o, v) { o.x = v; return o; };"
        "var P = {}, o1 = Object.create(P), o2 = Object.create(P); s([1], 0); 0"), "the setup threw");
    white_shape_id_set(v.rt, white_shape_id_max());
    putf_case(&v, "s(o1, 1).x", "1", 0xffffffffu, 0);
    CHECK(white_shape_id_next(v.rt) < white_shape_id_max() / 2, "the ids did not run out (next %u)",
          white_shape_id_next(v.rt));
    putf_case(&v, "s(o2, 2).x", "2", 0xffffffffu, 2);
    vm_free(&v);
}

static int call_record_of(JSContext *ctx, const char *name)
{
    JSValue g = JS_GetGlobalObject(ctx), f = JS_GetPropertyStr(ctx, g, name);
    int r = white_regvm_call_record(f);

    JS_FreeValue(ctx, f);
    JS_FreeValue(ctx, g);
    return r;
}

/* The record of register code a function object keeps for the call ops of
   the loop (JSFuncObject.rv, engine/regvm-ops-basic.h): a closure made
   before its bytecode is converted keeps the mark of none until a call op
   finds the record of its bytecode, also when another closure of the same
   bytecode was converted first, and a closure made after has it at once.
   Every call gives what it gave before: with fewer and more arguments than
   parameters, to a function that reads its arguments, and to a function
   of another realm, which the call op leaves to JS_CallInternal, so that
   it runs in its own realm. */
static void test_regvm_call_record(void)
{
    VM v = vm_new();
    JSContext *ctx2;
    JSValue g, g2, f2, arr2;

    CHECK(eval_ok(v.ctx,
        "function mk(k) { return function (x, y) { return x + k; }; }"
        "function mka() { return function () { return arguments.length; }; }"
        "globalThis.call1 = function (f, x) { return f(x); };"
        "globalThis.call3 = function (f) { return f(1, 2, 3); };"
        "globalThis.a = mk(1); globalThis.b = mk(2); globalThis.d = mka(); globalThis.e = mka(); 0"),
          "the setup threw");
    CHECK(call_record_of(v.ctx, "a") == 1, "a: %d, want not converted", call_record_of(v.ctx, "a"));
    CHECK(call_record_of(v.ctx, "b") == 1, "b: %d, want not converted", call_record_of(v.ctx, "b"));
    CHECK(eval_is(v.ctx, "call1(b, 40)", "42"), "the first call of b");
    CHECK(call_record_of(v.ctx, "b") == 0, "b after its call: %d", call_record_of(v.ctx, "b"));
    CHECK(call_record_of(v.ctx, "a") == 2, "a after the call of b: %d", call_record_of(v.ctx, "a"));
    CHECK(eval_is(v.ctx, "call1(a, 40)", "41"), "the first call of a");
    CHECK(call_record_of(v.ctx, "a") == 0, "a after its call: %d", call_record_of(v.ctx, "a"));
    CHECK(eval_is(v.ctx, "globalThis.c = mk(3); 0", "0"), "c");
    CHECK(call_record_of(v.ctx, "c") == 0, "c, made after: %d", call_record_of(v.ctx, "c"));
    CHECK(eval_is(v.ctx, "call1(c, 1) + call3(c) + String(call1(c))", "8NaN"), "the calls of c");
    CHECK(eval_is(v.ctx, "call3(d) + call1(e, 1) + call1(d)", "5"), "the calls of d and e");
    CHECK(call_record_of(v.ctx, "e") == 0, "e after its call: %d", call_record_of(v.ctx, "e"));

    /* a function of another realm runs in its realm: the array it makes
       has the Array.prototype of that realm, not of the caller's */
    ctx2 = JS_NewContext(v.rt);
    CHECK(ctx2 != NULL, "no second context");
    CHECK(eval_ok(ctx2, "globalThis.f2 = function (x) { return [x * 2]; }; 0"), "the second realm threw");
    g2 = JS_GetGlobalObject(ctx2);
    f2 = JS_GetPropertyStr(ctx2, g2, "f2");
    g = JS_GetGlobalObject(v.ctx);
    JS_SetPropertyStr(v.ctx, g, "f2", f2);
    arr2 = JS_GetPropertyStr(ctx2, g2, "Array");
    JS_SetPropertyStr(v.ctx, g, "proto2", JS_GetPropertyStr(ctx2, arr2, "prototype"));
    JS_FreeValue(ctx2, arr2);
    CHECK(eval_is(v.ctx,
        "var r = [call1(f2, 21), call1(f2, 1)];"
        "r[0][0] + r[1][0] + ':' + (Object.getPrototypeOf(r[0]) === proto2) +"
        "(Object.getPrototypeOf(r[1]) === proto2) + (r[0] instanceof Array)",
        "44:truetruefalse"), "the calls of f2");
    CHECK(call_record_of(v.ctx, "f2") == 0, "f2 after its calls: %d", call_record_of(v.ctx, "f2"));
    CHECK(eval_ok(v.ctx, "delete globalThis.f2; delete globalThis.proto2; 0"), "the delete threw");
    JS_FreeValue(v.ctx, g);
    JS_FreeValue(ctx2, g2);
    JS_FreeContext(ctx2);

    /* a function object an embedder makes with JS_NewObjectProtoClass has
       the block of one js_new_function_object makes (under AddressSanitizer
       its free writes nothing past it) */
    CHECK(white_func_class_objects(v.ctx) == 0, "the objects of JS_NewObjectProtoClass: %d",
          white_func_class_objects(v.ctx));
    vm_free(&v);
}

/* Array destructuring with no iterator (engine/regvm-ops-dst.h): the pass
   makes DSTART for the for_of_start of a destructuring, and not for a
   for-of loop, yield* or a destructuring with a yield in a default (its
   return takes the catch offset off with nip_catch). The record of a DSTART
   holds the array in place of the iterator, which no instruction but
   for_of_next and iterator_close may read (rv_dst_scan): code read by
   JS_ReadObject that drops the catch offset of the record, or takes it off
   with nip_catch, and returns the iterator, gets the Array Iterator, from
   the old W_FOR_OF_START. So does nip_catch then rot3r, undefined and
   iterator_close, which closes the record with the return of
   %ArrayIteratorPrototype% when it has one. */
/* A record of DSTART keeps the built-in next with a reference, and the
   close, the unwinding and a read at a hole make the Array Iterator when
   they need it: each call of js_malloc_rt failing in turn, also the two of
   making it, ends in the out of memory error or the result, and leaves no
   reference behind, which vm_free would find as a block left in the
   runtime (status 87). */
static void test_regvm_dst_alloc(void)
{
    static const char setup[] =
        "var AIP = Object.getPrototypeOf([][Symbol.iterator]());"
        "AIP.return = function () { return {}; };"
        "globalThis.close = function () { 'use strict'; const [x] = [1, 2, 3]; return 'c' + x; };"
        "globalThis.unwind = function () { 'use strict'; try { const [x, y = (() => { throw 'e'; })()] ="
        " [1, undefined]; } catch (e) { return 'u' + e; } };"
        "globalThis.hole = function () { 'use strict'; var a = [1, 2]; a.length = 3;"
        " const [x, y, z = 'z'] = a; return x + y + z; }; 0";
    static const char *const names[] = { "close", "unwind", "hole" };
    static const char *const want[] = { "c1", "ue", "3z" };
    int c, fail, n, made, ooms;

    for (c = 0; c < (int)countof(names); c++) {
        n = 0;
        ooms = 0;
        for (fail = 0; fail == 0 || fail <= n; fail++) {
            VM v = vm_new();
            JSValue g, f, r;
            const char *s;

            CHECK(eval_ok(v.ctx, setup), "%s: the setup threw", names[c]);
            g = JS_GetGlobalObject(v.ctx);
            f = JS_GetPropertyStr(v.ctx, g, names[c]);
            JS_FreeValue(v.ctx, g);
            /* converted, and the built-in next made */
            r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
            JS_FreeValue(v.ctx, r);
            r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);
            if (fail == 0)
                n = made;
            if (JS_IsException(r)) {
                CHECK(fail > 0 && JS_IsUncatchableException(v.ctx),
                      "%s, call %d failing: an exception that is not out of memory", names[c], fail);
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
                ooms++;
            } else {
                s = JS_ToCString(v.ctx, r);
                CHECK(s && strcmp(s, want[c]) == 0, "%s, call %d failing: %s, not %s", names[c], fail,
                      s ? s : "?", want[c]);
                JS_FreeCString(v.ctx, s);
            }
            JS_FreeValue(v.ctx, r);
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
        }
        CHECK(n >= 2 && ooms >= 2, "%s: %d calls, %d failed", names[c], n, ooms);
    }
}

/* The spread of a plain array makes no Array Iterator (js_append_array,
   engine/quickjs.c): it allocates two blocks less than the spread of an
   array with an own property more, which takes the iterator, in an array
   literal and in a call. Each allocation of every spread fails in turn,
   those of the built-in next of the array iterator, which the first spread
   of a VM makes, too, and each run ends with out of memory or the result
   and leaves nothing. */
static void test_spread_alloc(void)
{
    static const char setup[] =
        "var p = [1, 2, 3], q = [1, 2, 3]; q.x = 0;"
        "globalThis.lp = function (k) { 'use strict'; if (k === 0) return 0; return [0, ...p].join(); };"
        "globalThis.lq = function (k) { 'use strict'; if (k === 0) return 0; return [0, ...q].join(); };"
        "globalThis.cp = function (k) { 'use strict'; if (k === 0) return 0; return Math.max(...p, 0); };"
        "globalThis.cq = function (k) { 'use strict'; if (k === 0) return 0; return Math.max(...q, 0); };"
        "globalThis.tp = function (k) { 'use strict'; if (k === 0) return 0; return [...p, ...p, ...p, ...p].length; }; 0";
    static const char *const names[] = { "lp", "lq", "cp", "cq", "tp" };
    static const char *const want[] = { "0,1,2,3", "0,1,2,3", "3", "3", "12" };
    int c, fail, n, made, ooms, allocs[countof(names)];
    JSValue zero = JS_MKVAL(JS_TAG_INT, 0);

    for (c = 0; c < (int)countof(names); c++) {
        n = 0;
        ooms = 0;
        for (fail = 0; fail == 0 || fail <= n; fail++) {
            VM v = vm_new();
            JSValue g, f, r;
            const char *s;

            CHECK(eval_ok(v.ctx, setup), "%s: the setup threw", names[c]);
            g = JS_GetGlobalObject(v.ctx);
            f = JS_GetPropertyStr(v.ctx, g, names[c]);
            JS_FreeValue(v.ctx, g);
            /* converted, with no spread run: the built-in next of the
               array iterator is made by the first spread, in the sweep */
            r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &zero);
            JS_FreeValue(v.ctx, r);
            r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);
            if (fail == 0)
                n = made;
            if (JS_IsException(r)) {
                CHECK(fail > 0 && JS_IsUncatchableException(v.ctx),
                      "%s, call %d failing: an exception that is not out of memory", names[c], fail);
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
                ooms++;
            } else {
                s = JS_ToCString(v.ctx, r);
                CHECK(s && strcmp(s, want[c]) == 0, "%s, call %d failing: %s, not %s", names[c], fail,
                      s ? s : "?", want[c]);
                JS_FreeCString(v.ctx, s);
            }
            JS_FreeValue(v.ctx, r);
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
        }
        allocs[c] = n;
        CHECK(n >= 1 && ooms >= 1, "%s: %d calls, %d failed", names[c], n, ooms);
    }
    CHECK(allocs[0] + 2 <= allocs[1], "a spread in a literal: %d allocations, %d with the iterator",
          allocs[0], allocs[1]);
    CHECK(allocs[2] + 2 <= allocs[3], "a spread in a call: %d allocations, %d with the iterator",
          allocs[2], allocs[3]);
}

/* Elements with a reference count (an object, a string of 80 characters, a
   symbol), spread three times into one array, of an array the copy takes
   (e) and of one it refuses (f, an own key): each allocation fails in turn,
   the growth of the array for the third spread among them, and after each
   run every element has the references it had before, none kept by the
   elements already appended to the array that the error frees. */
static void test_spread_refs(void)
{
    static const char setup[] =
        "var e = [{}, 'e'.repeat(80), Symbol('e')], f = [{}, 'f'.repeat(80), Symbol('f')]; f.x = 0;"
        "function k() { return arguments.length; }"
        "globalThis.le = function () { 'use strict'; return [...e, ...e, ...e].length; };"
        "globalThis.lf = function () { 'use strict'; return [...f, ...f, ...f].length; };"
        "globalThis.ce = function () { 'use strict'; return k(...e, ...e, ...e); };"
        "globalThis.cf = function () { 'use strict'; return k(...f, ...f, ...f); }; 0";
    static const char *const names[] = { "le", "lf", "ce", "cf" };
    int c, fail, n, made, ooms, i, before[3];

    for (c = 0; c < (int)countof(names); c++) {
        n = 0;
        ooms = 0;
        for (fail = 0; fail == 0 || fail <= n; fail++) {
            VM v = vm_new();
            JSValue g, f, r, src, el[3];
            int32_t got = -1;

            CHECK(eval_ok(v.ctx, setup), "%s: the setup threw", names[c]);
            g = JS_GetGlobalObject(v.ctx);
            f = JS_GetPropertyStr(v.ctx, g, names[c]);
            src = JS_GetPropertyStr(v.ctx, g, names[c][1] == 'e' ? "e" : "f");
            JS_FreeValue(v.ctx, g);
            /* converted, and what the first spread of a VM makes made */
            r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
            JS_FreeValue(v.ctx, r);
            JS_RunGC(v.rt);
            for (i = 0; i < 3; i++) {
                el[i] = JS_GetPropertyUint32(v.ctx, src, i);
                before[i] = white_ref_count(el[i]);
            }
            r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);
            if (fail == 0)
                n = made;
            if (JS_IsException(r)) {
                CHECK(fail > 0 && JS_IsUncatchableException(v.ctx),
                      "%s, call %d failing: an exception that is not out of memory", names[c], fail);
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
                ooms++;
            } else {
                CHECK(JS_ToInt32(v.ctx, &got, r) == 0 && got == 9, "%s, call %d failing: %d elements, not 9",
                      names[c], fail, got);
            }
            JS_FreeValue(v.ctx, r);
            JS_RunGC(v.rt);
            for (i = 0; i < 3; i++) {
                CHECK(white_ref_count(el[i]) == before[i] && before[i] > 0,
                      "%s, call %d failing: element %d has %d references, had %d", names[c], fail, i,
                      white_ref_count(el[i]), before[i]);
                JS_FreeValue(v.ctx, el[i]);
            }
            JS_FreeValue(v.ctx, src);
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
        }
        CHECK(n >= 1 && ooms >= 1, "%s: %d calls, %d failed", names[c], n, ooms);
    }
}

static void test_regvm_dst(void)
{
    static const char src[] =
        "globalThis.f = function (a) { 'use strict'; const [x, y] = a; let s = 0;"
        " for (const v of a) s += v; return x + y + s; };"
        "globalThis.g = function* (a) { 'use strict'; yield* a; const [p = yield 1] = []; return p; };"
        "globalThis.h = function (a) { 'use strict'; let x = a; const [p, q, r] = [x, x, x];"
        " return [p, q, r, x, a]; }; 0";
    static const char ret_src[] =
        "globalThis.closed = 0; Object.getPrototypeOf([][Symbol.iterator]()).return ="
        " function () { globalThis.closed++; return {}; }; 0";
    VM v;
    JSValue fun, r;
    char *t;
    int kind, rc, dst, win;

    v = vm_new();
    CHECK(eval_ok(v.ctx, src), "the setup threw");
    r = dst_call(&v, "f", &fun);
    CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_INT && JS_VALUE_GET_INT(r) == 30, "f([7, 8]) is not 30");
    CHECK(white_regvm_dst_sites(fun, &dst, &win) == 0 && dst == 1 && win == 1,
          "f: %d DSTART and %d W_FOR_OF_START, not 1 and 1", dst, win);
    JS_FreeValue(v.ctx, r);
    JS_FreeValue(v.ctx, fun);
    CHECK(eval_is(v.ctx, "var it = g([1]); [it.next().value, it.next().value, it.return(5).value,"
                  " it.next().done].join()", "1,1,5,true"),
          "g: the generator gives other values");
    {
        JSValue gl = JS_GetGlobalObject(v.ctx);
        fun = JS_GetPropertyStr(v.ctx, gl, "g");
        JS_FreeValue(v.ctx, gl);
        CHECK(white_regvm_dst_sites(fun, &dst, &win) == 0 && dst == 0 && win == 2,
              "g: %d DSTART and %d W_FOR_OF_START, not 0 and 2", dst, win);
        JS_FreeValue(v.ctx, fun);
    }
    vm_free(&v);

    for (kind = 3; kind <= 5; kind++) {
        v = vm_new();
        CHECK(eval_ok(v.ctx, src), "kind %d: the setup threw", kind);
        {
            JSValue gl = JS_GetGlobalObject(v.ctx);
            fun = JS_GetPropertyStr(v.ctx, gl, "h");
            JS_FreeValue(v.ctx, gl);
        }
        if (white_crafted_code(v.ctx, fun, kind, 0) != 0) {
            CHECK(0, "kind %d: the function has no room for the code", kind);
            JS_FreeValue(v.ctx, fun);
            vm_free(&v);
            continue;
        }
        rc = white_regvm_read_check(v.ctx, fun);
        t = exception_text(v.ctx);
        CHECK(rc == 0, "kind %d: the read refuses it (%s)", kind, t ? t : "no exception");
        free(t);
        JS_FreeValue(v.ctx, fun);
        if (kind == 5)
            CHECK(eval_ok(v.ctx, ret_src), "kind 5: the return could not be set");
        r = dst_call(&v, "h", &fun);
        t = JS_IsException(r) ? exception_text(v.ctx) : NULL;
        CHECK(white_regvm_dst_sites(fun, &dst, &win) == 0, "kind %d: no register code", kind);
        CHECK(dst == 0 && win == 1, "kind %d: %d DSTART and %d W_FOR_OF_START, not 0 and 1",
              kind, dst, win);
        if (kind < 5) {
            JSValue gl = JS_GetGlobalObject(v.ctx);
            JS_SetPropertyStr(v.ctx, gl, "got", JS_DupValue(v.ctx, r));
            JS_FreeValue(v.ctx, gl);
            CHECK(eval_is(v.ctx, "Object.prototype.toString.call(got)", "[object Array Iterator]"),
                  "kind %d: the iterator returned is no Array Iterator (tag %d, %s)",
                  kind, (int)JS_VALUE_GET_TAG(r), t ? t : "no exception");
        } else {
            CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_INT && JS_VALUE_GET_INT(r) == 7,
                  "kind 5: the first element is not 7 (tag %d, %s)", (int)JS_VALUE_GET_TAG(r),
                  t ? t : "no exception");
            CHECK(eval_is(v.ctx, "globalThis.closed", "1"), "kind 5: the return ran other than once");
        }
        free(t);
        JS_FreeValue(v.ctx, r);
        white_crafted_code(v.ctx, fun, kind, 1);
        JS_FreeValue(v.ctx, fun);
        vm_free(&v);
    }

    /* a record no for_of_start made, of the JS_UNINITIALIZED that get_loc
       reads from a local in its dead zone as its next method and undefined
       as its catch offset, is the record it is: for_of_next calls that
       next (TypeError) with null (6) or the array (7) as the iterator, and
       iterator_close calls the return of the array (8). */
    for (kind = 6; kind <= 8; kind++) {
        v = vm_new();
        CHECK(eval_ok(v.ctx, src), "kind %d: the setup threw", kind);
        CHECK(eval_ok(v.ctx, "globalThis.closed = 0; Array.prototype.return ="
                      " function () { globalThis.closed++; return {}; }; 0"),
              "kind %d: the return could not be set", kind);
        {
            JSValue gl = JS_GetGlobalObject(v.ctx);
            fun = JS_GetPropertyStr(v.ctx, gl, "h");
            JS_FreeValue(v.ctx, gl);
        }
        if (white_crafted_code(v.ctx, fun, kind, 0) != 0) {
            CHECK(0, "kind %d: the function has no room for the code", kind);
            JS_FreeValue(v.ctx, fun);
            vm_free(&v);
            continue;
        }
        rc = white_regvm_read_check(v.ctx, fun);
        t = exception_text(v.ctx);
        CHECK(rc == 0, "kind %d: the read refuses it (%s)", kind, t ? t : "no exception");
        free(t);
        JS_FreeValue(v.ctx, fun);
        r = dst_call(&v, "h", &fun);
        t = JS_IsException(r) ? exception_text(v.ctx) : NULL;
        if (kind < 8)
            CHECK(t && strstr(t, "TypeError"), "kind %d: not a TypeError but tag %d (%s)",
                  kind, (int)JS_VALUE_GET_TAG(r), t ? t : "no exception");
        else
            CHECK(JS_IsUndefined(r) && eval_is(v.ctx, "globalThis.closed", "1"),
                  "kind 8: the return of the array ran other than once (tag %d, %s)",
                  (int)JS_VALUE_GET_TAG(r), t ? t : "no exception");
        free(t);
        JS_FreeValue(v.ctx, r);
        white_crafted_code(v.ctx, fun, kind, 1);
        JS_FreeValue(v.ctx, fun);
        vm_free(&v);
    }

    /* the catch offset of a record of DSTART is the index of its next
       element, below 0, which only for_of_next and iterator_close of the
       record read: a finally run with the record under its return address
       leaves it, and the ret goes back by the depth and the catch the pass
       found (9: DSTART, 7). iterator_get_value_done, which would write the
       catch offset 0 back, gets the old W_FOR_OF_START (10: 7, where DSTART
       would leave the array as the iterator, and the built-in next would
       throw a TypeError on it), and a read refuses swap (11). */
    for (kind = 9; kind <= 11; kind++) {
        v = vm_new();
        CHECK(eval_ok(v.ctx, src), "kind %d: the setup threw", kind);
        CHECK(eval_ok(v.ctx, "globalThis.k = function (a) { 'use strict'; let x = a;"
                      " try { const [p, q, r] = [x, x, x]; return [p, q, r, x, a]; }"
                      " finally { x = 0; } }; 0"), "kind %d: k could not be made", kind);
        {
            JSValue gl = JS_GetGlobalObject(v.ctx);
            fun = JS_GetPropertyStr(v.ctx, gl, "k");
            JS_FreeValue(v.ctx, gl);
        }
        if (white_crafted_code(v.ctx, fun, kind, 0) != 0) {
            CHECK(0, "kind %d: the function has no room for the code", kind);
            JS_FreeValue(v.ctx, fun);
            vm_free(&v);
            continue;
        }
        rc = white_regvm_read_check(v.ctx, fun);
        t = exception_text(v.ctx);
        free(t);
        JS_FreeValue(v.ctx, fun);
        if (kind == 11) {
            CHECK(rc != 0, "kind 11: the read lets swap read a catch offset");
        } else {
            CHECK(rc == 0, "kind %d: the read refuses it", kind);
            r = dst_call(&v, "k", &fun);
            t = JS_IsException(r) ? exception_text(v.ctx) : NULL;
            CHECK(white_regvm_dst_sites(fun, &dst, &win) == 0, "kind %d: no register code", kind);
            CHECK(kind == 9 ? dst == 1 && win == 0 : dst == 0 && win == 1,
                  "kind %d: %d DSTART and %d W_FOR_OF_START", kind, dst, win);
            CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_INT && JS_VALUE_GET_INT(r) == 7,
                  "kind %d: the first element is not 7 (tag %d, %s)", kind,
                  (int)JS_VALUE_GET_TAG(r), t ? t : "no exception");
            free(t);
            JS_FreeValue(v.ctx, r);
            white_crafted_code(v.ctx, fun, kind, 1);
            JS_FreeValue(v.ctx, fun);
            vm_free(&v);
            continue;
        }
        {
            JSValue gl = JS_GetGlobalObject(v.ctx);
            fun = JS_GetPropertyStr(v.ctx, gl, "k");
            JS_FreeValue(v.ctx, gl);
        }
        white_crafted_code(v.ctx, fun, kind, 1);
        JS_FreeValue(v.ctx, fun);
        vm_free(&v);
    }
}

/* ---- the date of a count of days ---- */

/* The year, month and day as get_date_fields found them before
   days_to_date: an estimate of the year stepped by whole years, then the
   months counted off. */
static int64_t ref_floor_div(int64_t a, int64_t b)
{
    int64_t m = a % b;
    return (a - (m + (m < 0) * b)) / b;
}

static int64_t ref_days_from_year(int64_t y)
{
    return 365 * (y - 1970) + ref_floor_div(y - 1969, 4) -
        ref_floor_div(y - 1901, 100) + ref_floor_div(y - 1601, 400);
}

static int64_t ref_days_in_year(int64_t y)
{
    return 365 + !(y % 4) - !(y % 100) + !(y % 400);
}

static void ref_days_to_date(int64_t days, int *py, int *pm, int *pd)
{
    static const int month_days[] = { 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31 };
    int64_t y = ref_floor_div(days * 10000, 3652425) + 1970, d1, nd, md;
    int i;

    for (;;) {
        d1 = days - ref_days_from_year(y);
        if (d1 < 0) {
            y--;
            d1 += ref_days_in_year(y);
        } else {
            nd = ref_days_in_year(y);
            if (d1 < nd)
                break;
            d1 -= nd;
            y++;
        }
    }
    for (i = 0; i < 11; i++) {
        md = month_days[i];
        if (i == 1)
            md += ref_days_in_year(y) - 365;
        if (d1 < md)
            break;
        d1 -= md;
    }
    *py = (int)y;
    *pm = i;
    *pd = (int)d1 + 1;
}

/* days_to_date gives the date the loops gave, for every day from 1148 to
   2791, every day of two cycles of 400 years at each end of the days a time
   value can name, and two days past each end (a day of time zone), and every
   997th day between. A run over every day of that range, 200,000,005 days,
   found no difference when it was written. */
static void test_days_to_date(void)
{
    static const int64_t ranges[][3] = {
        { -300000, 300000, 1 },
        { -100000002, -100000002 + 2 * 146097, 1 },
        { 100000002 - 2 * 146097, 100000002, 1 },
        { -100000002, 100000002, 997 },
    };
    long bad = 0, n = 0;
    int i;

    for (i = 0; i < (int)countof(ranges); i++) {
        int64_t x;
        for (x = ranges[i][0]; x <= ranges[i][1]; x += ranges[i][2]) {
            int y, m, d, ry, rm, rd;
            white_days_to_date(x, &y, &m, &d);
            ref_days_to_date(x, &ry, &rm, &rd);
            if (y != ry || m != rm || d != rd) {
                if (bad++ < 5)
                    printf("day %lld: %d-%d-%d, the loops give %d-%d-%d\n",
                           (long long)x, y, m + 1, d, ry, rm + 1, rd);
            }
            n++;
        }
    }
    CHECK(bad == 0, "%ld of %ld days give another date", bad, n);
}

/* ---- an element added at 2^31 - 1 ----

   A fast array holds at most 2^31 - 1 elements: an element added after them
   leaves it as it was and throws, by each way to add one. Such an array takes
   32 GiB, so the array of a case has 8 elements and is given a count and a
   length of 2^31 - 1 (white_array_fake_count), with no collection while it
   is so. Before, the element was written 16 GiB past the block, which was
   made smaller. Each case runs in a process of its own, where a crash is one
   failed case. */
static const char *const max_index_ways[] = {
    "JS_SetPropertyUint32", "JS_SetPropertyStr", "JS_DefinePropertyValueUint32",
    "a[i] = v", "push", "splice",
};

static void max_index_case(int way)
{
    static const char setup[] =
        "function put(a, i, v) { a[i] = v; }\n"
        "function push(a, v) { return a.push(v); }\n"
        "function splice(a, v) { return a.splice(a.length, 0, v); }\n"
        /* once before, so that their register code is made */
        "put([], 0, 0); push([], 0); splice([], 0);\n";
    const char *want = "RangeError: invalid array length";
    VM v = vm_new();
    JSValue a, g, f, r = JS_UNDEFINED, args[3];
    int64_t room;
    size_t threshold;
    uint32_t old, count;
    int ret = 0;
    char *t;

    CHECK(eval_ok(v.ctx, setup), "%s: the setup", max_index_ways[way]);
    a = JS_Eval(v.ctx, "[0, 1, 2, 3, 4, 5, 6, 7]", 24, "<unit>", JS_EVAL_TYPE_GLOBAL);
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, way == 3 ? "put" : way == 4 ? "push" : "splice");
    room = white_array_room(a);
    threshold = JS_GetGCThreshold(v.rt);
    JS_SetGCThreshold(v.rt, (size_t)-1);
    old = white_array_fake_count(v.ctx, a, INT32_MAX);
    args[0] = a;
    args[1] = JS_NewInt32(v.ctx, INT32_MAX);
    args[2] = JS_NewInt32(v.ctx, 1);
    switch (way) {
    case 0: ret = JS_SetPropertyUint32(v.ctx, a, INT32_MAX, JS_NewInt32(v.ctx, 1)); break;
    case 1: ret = JS_SetPropertyStr(v.ctx, a, "2147483647", JS_NewInt32(v.ctx, 1)); break;
    case 2: ret = JS_DefinePropertyValueUint32(v.ctx, a, INT32_MAX, JS_NewInt32(v.ctx, 1), JS_PROP_C_W_E | JS_PROP_THROW); break;
    case 3: r = JS_Call(v.ctx, f, JS_UNDEFINED, 3, args); break;
    default: args[1] = args[2]; r = JS_Call(v.ctx, f, JS_UNDEFINED, 2, args); break;
    }
    t = exception_text(v.ctx);
    CHECK((ret < 0 || JS_IsException(r)) && t && strcmp(t, want) == 0, "%s at 2^31 - 1: %s, want %s",
          max_index_ways[way], t ? t : "no exception", want);
    free(t);
    JS_FreeValue(v.ctx, r);
    CHECK(eval_is(v.ctx, "this.x = 1; 'ok'", "ok"), "%s: the context after", max_index_ways[way]);
    r = JS_GetPropertyStr(v.ctx, a, "length");
    CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_INT && JS_VALUE_GET_INT(r) == INT32_MAX, "%s: the length is no longer 2^31 - 1",
          max_index_ways[way]);
    JS_FreeValue(v.ctx, r);
    CHECK(white_array_room(a) == room, "%s: the room was %lld, is %lld", max_index_ways[way], (long long)room,
          (long long)white_array_room(a));
    count = white_array_fake_count(v.ctx, a, old);
    CHECK(count == INT32_MAX, "%s: the count is %u", max_index_ways[way], count);
    JS_SetGCThreshold(v.rt, threshold);
    CHECK(eval_is(v.ctx, "put.length", "3"), "%s: a script after", max_index_ways[way]);
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, g);
    JS_FreeValue(v.ctx, a);
    vm_free(&v);
}

static void test_max_index(void)
{
    char way_arg[16];
    char *args[4];
    int way, st;

    for (way = 0; way < (int)countof(max_index_ways); way++) {
        snprintf(way_arg, sizeof(way_arg), "%d", way);
        args[1] = "max-index";
        args[2] = way_arg;
        args[3] = NULL;
        st = run_self(args);
        CHECK(WIFEXITED(st) && WEXITSTATUS(st) == 0, "an element at 2^31 - 1 by %s: %s %d", max_index_ways[way],
              WIFSIGNALED(st) ? "stopped by signal" : "exit status", WIFSIGNALED(st) ? WTERMSIG(st) : WEXITSTATUS(st));
    }
}

/* ---- a property added to a shape at its limit ----

   A shape holds at most 2^26 - 1 properties (JS_SHAPE_PROP_MAX): the index
   of a property plus one is kept in the 26 bits of hash_next, and past it a
   lookup would miss a property that is there. One more throws and leaves the
   object as it was, and a store that the object refuses anyway is refused as
   before, by the length that cannot be written or the array that cannot be
   extended, with no RangeError. Such an object takes gigabytes, so a small
   one is given a count at the limit (white_shape_fake_count for its shape,
   white_array_fake_count for a fast array), with no collection while it is
   so. Before, the shape was copied with that count, past its block, an
   element added to a normal array at the limit left its length one more, and
   a store refused by the length or the array threw a RangeError. Each case
   runs in a process of its own. */
static const char *const max_props_ways[] = {
    "a new key",
    "an array made a normal one",
    "an element at the end of a normal array",
    "a store past a length that cannot be written",
    "a store past the end of an array that cannot be extended",
};

static void max_props_case(int way)
{
    static const char *const setup[] = {
        /* a delete takes the shape out of the shape table, as its own */
        "var o = { a: 1, b: 2, c: 3 }; delete o.c; o",
        /* a fast array of 2^26 - 1 elements and its length would be 2^26
           properties as a normal array: an element far past its end makes it
           one */
        "var o = [1, 2, 3]; o",
        /* the delete makes the array a normal one, with a shape of its own */
        "var o = [1, 2, 3]; delete o[0]; o",
        "var o = Object.defineProperty([1, 2, 3], 'length', { writable: false }); o",
        "var o = Object.preventExtensions([1, 2, 3]); o",
    };
    /* what is left of o once its count is back */
    static const char *const after[] = {
        "o.a + o.b + ':' + ('z' in o)",
        "o.join() + ':' + o.length + ':' + (1073741824 in o)",
        "o.join() + ':' + o.length + ':' + (3 in o)",
        "o.join() + ':' + o.length + ':' + (67108864 in o)",
        "o.join() + ':' + o.length + ':' + (67108864 in o)",
    };
    static const char *const after_want[] = {
        "3:false", "1,2,3:3:false", ",2,3:3:false", "1,2,3:3:false", "1,2,3:3:false",
    };
    VM v = vm_new();
    JSValue o;
    size_t threshold;
    int64_t old = 0;
    uint32_t max = white_shape_prop_max();
    int ret, refused;
    char *t;

    CHECK(max == (1u << 26) - 1, "the limit is %u", max);
    threshold = JS_GetGCThreshold(v.rt);
    JS_SetGCThreshold(v.rt, (size_t)-1);
    o = JS_Eval(v.ctx, setup[way], strlen(setup[way]), "<unit>", JS_EVAL_TYPE_GLOBAL);
    CHECK(!JS_IsException(o), "%s: the setup threw", max_props_ways[way]);
    refused = way >= 3;
    switch (way) {
    case 0:
        old = white_shape_fake_count(o, max);
        CHECK(old == 3, "%s: the shape was not its own (%lld)", max_props_ways[way], (long long)old);
        ret = JS_SetPropertyStr(v.ctx, o, "z", JS_NewInt32(v.ctx, 1));
        white_shape_fake_count(o, (uint32_t)old);
        break;
    case 2:
        /* length, a deleted 0, 1 and 2 */
        old = white_shape_fake_count(o, max);
        CHECK(old == 4, "%s: the shape was not its own (%lld)", max_props_ways[way], (long long)old);
        ret = JS_SetPropertyUint32(v.ctx, o, 3, JS_NewInt32(v.ctx, 4));
        white_shape_fake_count(o, (uint32_t)old);
        break;
    default:
        old = white_array_fake_count(v.ctx, o, max);
        ret = JS_SetPropertyUint32(v.ctx, o, way == 1 ? 1u << 30 : max + 1, JS_NewInt32(v.ctx, 1));
        if (refused) {
            t = exception_text(v.ctx);
            CHECK(ret < 0 && t && strncmp(t, "TypeError: ", 11) == 0, "%s: %s", max_props_ways[way],
                  t ? t : "no exception");
            free(t);
            /* in sloppy code the store does nothing, and Reflect.set gives
               false */
            CHECK(eval_is(v.ctx, "[Reflect.set(o, 67108864, 1), (function () { o[67108864] = 1; return 7 })()].join()",
                          "false,7"),
                  "%s: Reflect.set and a sloppy store", max_props_ways[way]);
        }
        white_array_fake_count(v.ctx, o, (uint32_t)old);
        break;
    }
    if (!refused) {
        t = exception_text(v.ctx);
        CHECK(ret < 0 && t && strcmp(t, "RangeError: too many properties") == 0, "%s at the limit: %s",
              max_props_ways[way], t ? t : "no exception");
        free(t);
    }
    JS_SetGCThreshold(v.rt, threshold);
    CHECK(eval_is(v.ctx, after[way], after_want[way]), "%s: the object after", max_props_ways[way]);
    JS_FreeValue(v.ctx, o);
    vm_free(&v);
}

static void test_max_props(void)
{
    char way_arg[16];
    char *args[4];
    int way, st;

    for (way = 0; way < (int)countof(max_props_ways); way++) {
        snprintf(way_arg, sizeof(way_arg), "%d", way);
        args[1] = "max-props";
        args[2] = way_arg;
        args[3] = NULL;
        st = run_self(args);
        CHECK(WIFEXITED(st) && WEXITSTATUS(st) == 0, "a property at 2^26 - 1 by %s: %s %d", max_props_ways[way],
              WIFSIGNALED(st) ? "stopped by signal" : "exit status", WIFSIGNALED(st) ? WTERMSIG(st) : WEXITSTATUS(st));
    }
}

/* ---- Array.from past 32 bits ----

   Array.from on a constructor of the program passes the mapping function an
   index of 64 bits and sets the length it ends at, which a constructor whose
   object keeps nothing can take past 2^32. Reaching that index would take
   2^32 calls, so Array.from starts there (white_set_array_from_first). Before,
   the index was cut to an int32 and the length to a uint32. */
static void test_array_from_past_32_bits(void)
{
    VM v = vm_new();

    CHECK(eval_is(v.ctx,
                  "var log = [];"
                  "function Sink() {"
                  "  return new Proxy({}, {"
                  "    defineProperty(t, k, d) { log.push(k + ':' + d.value); return true },"
                  "    set(t, k, x) { log.push(k + '=' + x); return true },"
                  "  })"
                  "}"
                  "function* two() { yield 'a'; yield 'b' }"
                  "'' + log",
                  ""),
          "Array.from past 32 bits: the setup");
    white_set_array_from_first(4294967295LL);
    CHECK(eval_is(v.ctx,
                  "Array.from.call(Sink, { length: 4294967297 }, (x, i) => i); log.push('|');"
                  "Array.from.call(Sink, two(), (x, i) => x + i); log.push('|');"
                  "Array.from.call(Sink, two()); '' + log",
                  "4294967295:4294967295,4294967296:4294967296,length=4294967297,|,"
                  "4294967295:a4294967295,4294967296:b4294967296,length=4294967297,|,"
                  "4294967295:a,4294967296:b,length=4294967297"),
          "Array.from past 32 bits");
    white_set_array_from_first(0);
    vm_free(&v);
}

/* ---- Object.defineProperty on a property that is there, out of memory ----

   A define that changes the flags of a property that is there copies the
   shape of the object when another object shares it, which can run out of
   memory. Each call of the allocator fails in turn in a define of the
   length of a new array, of a data property and of an accessor of a new
   literal, each sharing its shape (with the context, or with a twin o2),
   and after each the property is as it was (the define threw out of
   memory) or as the define makes it.
   Before, the length or the value was set first, and a failed copy left it
   set with the flags as they were.
   A define that is refused changes nothing and copies nothing, so it gives
   false and never runs out of memory: a read-only export of a module
   namespace, whose shape the namespace of a twin module shares.
   Shortening an array past index 2^31 - 1 looks the names of the elements
   up and makes none, so it cannot fail once it has deleted one. Before, it
   made the name of each index, and when the second failed the first element
   was gone, the define threw and still made the length read-only. */

/* reset of test_define_existing_oom: "NS" makes two new modules with the same
   export, whose namespaces share a shape, as ns1 and ns2 (a namespace whose
   define copied its shape shares it no more) */
static int define_reset(JSContext *ctx, const char *reset, int k)
{
    static const char mod_src[] = "export let x = 1;";
    JSValue g, mod, r;
    char name[32];
    int i, ok = 1;

    if (strcmp(reset, "NS") != 0)
        return eval_ok(ctx, reset);
    g = JS_GetGlobalObject(ctx);
    for (i = 0; i < 2 && ok; i++) {
        snprintf(name, sizeof(name), "ns%d-%d", i + 1, k);
        mod = JS_Eval(ctx, mod_src, strlen(mod_src), name, JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
        if (JS_IsException(mod)) {
            ok = 0;
            break;
        }
        r = JS_EvalFunction(ctx, JS_DupValue(ctx, mod));
        ok = !JS_IsException(r);
        JS_FreeValue(ctx, r);
        if (ok)
            JS_SetPropertyStr(ctx, g, i ? "ns2" : "ns1", JS_GetModuleNamespace(ctx, JS_VALUE_GET_PTR(mod)));
        JS_FreeValue(ctx, mod);
    }
    JS_FreeValue(ctx, g);
    return ok && eval_ok(ctx, "o = ns1; o2 = ns2; r = 0");
}

static void test_define_existing_oom(void)
{
    static const struct {
        const char *name, *reset, *change, *state, *before, *after;
        int oom; /* 1: some call fails and changes nothing, 0: none fails */
    } cases[] = {
        { "the length and writable of an array", "o = []",
          "Object.defineProperty(o, 'length', { value: 2, writable: false })",
          "o.length + ':' + Object.getOwnPropertyDescriptor(o, 'length').writable", "0:true", "2:false", 1 },
        { "the length of an array and writable after a shorter one", "o = [1, 2, 3, 4]",
          "Object.defineProperty(o, 'length', { value: 2, writable: false })",
          "o.length + ':' + Object.getOwnPropertyDescriptor(o, 'length').writable + ':' + o.join()",
          "4:true:1,2,3,4", "2:false:1,2", 1 },
        { "a value and writable", "o = { x: 1 }; o2 = { x: 1 }",
          "Object.defineProperty(o, 'x', { value: 2, writable: false })",
          "o.x + ':' + Object.getOwnPropertyDescriptor(o, 'x').writable", "1:true", "2:false", 1 },
        { "a value and enumerable", "o = { x: 1 }; o2 = { x: 1 }",
          "Object.defineProperty(o, 'x', { value: 2, enumerable: false })", "o.x + ':' + Object.keys(o).length",
          "1:1", "2:0", 1 },
        { "a getter and enumerable", "o = mk(); o2 = mk()",
          "Object.defineProperty(o, 'x', { get: function () { return 2 }, enumerable: false })",
          "o.x + ':' + Object.keys(o).length", "1:1", "2:0", 1 },
        { "a read-only export of a module namespace", "NS",
          "r = Reflect.defineProperty(o, 'x', readOnly)",
          "r + ':' + o.x + ':' + Object.getOwnPropertyDescriptor(o, 'x').writable", "0:1:true", "false:1:true", 0 },
        { "the length and writable of an array past 2^31 - 1", "o = []; o[2147483650] = 7; o.extra = 1",
          "Object.defineProperty(o, 'length', { value: 2147483648, writable: false })",
          "o.length + ':' + o[2147483650] + ':' + Object.getOwnPropertyDescriptor(o, 'length').writable",
          "2147483651:7:true", "2147483648:undefined:false", 1 },
        { "the length of an array past 2^31 - 1", "o = []; o[2147483650] = 7; o.extra = 1",
          "o.length = 2147483648",
          "o.length + ':' + o[2147483650] + ':' + Object.getOwnPropertyDescriptor(o, 'length').writable",
          "2147483651:7:true", "2147483648:undefined:true", 0 },
    };
    VM v = vm_new();
    JSValue g;
    size_t c;
    int k = 0;
    char src[320];

    for (c = 0; c < countof(cases); c++) {
        JSValue f;
        int fail, n = 0, ooms = 0;

        snprintf(src, sizeof(src), "var o, o2, r, readOnly = { writable: false };"
                 " function mk() { return { get x() { return 1 } } }"
                 " function change() { %s }", cases[c].change);
        CHECK(eval_ok(v.ctx, src), "%s: the setup threw", cases[c].name);
        g = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, g, "change");
        JS_FreeValue(v.ctx, g);
        /* once, so that the code of change is compiled and its atoms made */
        CHECK(define_reset(v.ctx, cases[c].reset, k++) && eval_ok(v.ctx, "change()"), "%s: the first change threw",
              cases[c].name);
        for (fail = 0; fail == 0 || fail <= n; fail++) {
            int made;
            JSValue r;

            CHECK(define_reset(v.ctx, cases[c].reset, k++), "%s: the reset threw", cases[c].name);
            CHECK(eval_is(v.ctx, cases[c].state, cases[c].before), "%s: before call %d", cases[c].name, fail);
            r = white_call_failing(v.ctx, f, JS_UNDEFINED, 0, NULL, fail, &made);
            if (fail == 0)
                n = made;
            if (JS_IsException(r)) {
                CHECK(cases[c].oom && fail > 0 && JS_IsUncatchableException(v.ctx),
                      "%s: call %d failing: threw", cases[c].name, fail);
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
                ooms++;
                CHECK(eval_is(v.ctx, cases[c].state, cases[c].before), "%s: call %d failing: the define changed it",
                      cases[c].name, fail);
            } else {
                JS_FreeValue(v.ctx, r);
                CHECK(eval_is(v.ctx, cases[c].state, cases[c].after), "%s: call %d failing: not defined",
                      cases[c].name, fail);
            }
        }
        if (cases[c].oom)
            CHECK(n > 0 && ooms > 0, "%s: %d calls of the allocator, %d failed the define", cases[c].name, n, ooms);
        JS_FreeValue(v.ctx, f);
    }
    vm_free(&v);
}

/* The growth of the block of a fast array that moves reads the interrupt
   flag before each MiB it copies, and a stop throws the interrupt and
   leaves the array as it was. The block of 4M elements (64 MiB) was copied
   in one go with no read: a stop asked while split filled its result of 2M
   elements waited for the copy, 11 to 14 polls of the generated engine. The
   host is told it cannot remap, so the block moves on every system. */
#define GROW_LEN 4194304
static void test_array_grow_stops(void)
{
    static const char src[] = "var a = []; for (let i = 0; i < 4194304; i++) a.push(i); a";
    VM v = vm_new();
    JSValue a, e;
    long reads;
    int ret;

    JS_SetHostRemap(v.rt, 0);
    a = JS_Eval(v.ctx, src, strlen(src), "<grow>", JS_EVAL_TYPE_GLOBAL);
    CHECK(!JS_IsException(a), "the array threw");
    host_poll_reads = 0;
    ret = white_array_grow(v.ctx, a, 2 * GROW_LEN);
    reads = host_poll_reads;
    CHECK(ret == 0, "the growth failed");
    CHECK(reads >= (long)(GROW_LEN * sizeof(JSValue) >> 20) - 1,
          "the copy of %d elements read the flag %ld times", GROW_LEN, reads);
    JS_FreeValue(v.ctx, a);

    a = JS_Eval(v.ctx, src, strlen(src), "<grow>", JS_EVAL_TYPE_GLOBAL);
    CHECK(!JS_IsException(a), "the array threw");
    host_poll_reads = 0;
    host_poll_stop_after(2);
    ret = white_array_grow(v.ctx, a, 2 * GROW_LEN);
    host_poll_stop_after(0);
    CHECK(ret < 0, "a stop in the copy did not end the growth");
    *JS_GetInterruptFlag(v.rt) = 0;
    if (ret < 0) {
        e = JS_GetException(v.ctx);
        JS_FreeValue(v.ctx, e);
    }
    JS_FreeValue(v.ctx, a);
    CHECK(eval_is(v.ctx, "a.length + ',' + a[0] + ',' + a[4194303] + ',' + (a.push(7), a[4194304])",
                  "4194304,0,4194303,7"), "the array after the stop");
    vm_free(&v);
}

/* Each allocation of the evaluation of module u.js failing in turn, which
   imports t.js, which has a top-level await it runs or not
   (globalThis.runAwait), and each allocation of the jobs that go on with it
   once it awaited, then the error taken as the host takes it. A failure
   in making the function of u.js left u.js with no code, and the import
   again crashed (js_create_module_bytecode_function). A failure that lost
   a job of t.js, or of the handlers it put on the promise of its function,
   left t.js and u.js evaluating for good, an import of either never
   settling: running out of memory ends the call as a stop does, and the
   host taking the error records them as failed (js_fail_async_modules). An
   import of u.js after the failure gives the result or the error out of
   memory, and so does one of t.js, which fails only when u.js does, and
   nothing leaks. */
static void test_module_jobs_oom(void)
{
    static const char t[] = "if (globalThis.runAwait) await 0; globalThis.n = 1;";
    static const char u[] = "import \"t.js\"; globalThis.x = n + 1;";
    static const char *const modes[] = { "no await", "await", "await, jobs" };
    static const char oom[] = "InternalError: out of memory";
    char buf[128], buf2[128];
    int mode, fail, n, made, failed, leaked;
    JSContext *ctx;
    JSValue m, r;

    for (mode = 0; mode < 3; mode++) {
        n = 0;
        failed = 0;
        for (fail = 0; fail == 0 || fail <= n; fail++) {
            VM v = vm_new();

            CHECK(eval_ok(v.ctx, mode ? "globalThis.runAwait = true" : "globalThis.runAwait = false"), "the setup threw");
            compile_module(&v, "t.js", t);
            m = JS_Eval(v.ctx, u, strlen(u), "u.js", JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
            CHECK(!JS_IsException(m), "u.js does not compile");
            if (mode < 2) {
                r = white_eval_function_failing(v.ctx, m, fail, &made);
            } else {
                r = JS_EvalFunction(v.ctx, m);
                CHECK(!JS_IsException(r), "the evaluation of u.js threw");
                white_jobs_failing(v.rt, &ctx, fail, &made);
            }
            if (fail == 0)
                n = made;
            JS_FreeValue(v.ctx, r);
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            drain_jobs(&v);
            import_state(&v, "u.js", buf, sizeof(buf));
            import_state(&v, "t.js", buf2, sizeof(buf2));
            if (strcmp(buf, "ok") == 0) {
                CHECK(strcmp(buf2, "ok") == 0 && eval_is(v.ctx, "x", "2"),
                      "%s, allocation %d of %d failing: u.js ran, and the import of t.js gave %s", modes[mode], fail, n, buf2);
            } else {
                CHECK(fail > 0 && strcmp(buf, oom) == 0 && (strcmp(buf2, "ok") == 0 || strcmp(buf2, oom) == 0),
                      "%s, allocation %d of %d failing: the imports after it gave %s and %s", modes[mode], fail, n, buf, buf2);
                failed++;
            }
            leaked = host_leaked_runtimes();
            vm_free(&v);
            CHECK(host_leaked_runtimes() == leaked, "%s, allocation %d of %d failing: blocks left in the runtime",
                  modes[mode], fail, n);
        }
        CHECK(n > 10 && failed > 0, "%s: %d allocations, %d failures recorded", modes[mode], n, failed);
    }
}

/* The modules of test_module_waiters: module t awaits, and each module w<i>
   imports it and writes down that it ran. */
static JSModuleDef *waiter_loader(JSContext *ctx, const char *name, void *opaque)
{
    char src[64];
    JSValue v;
    JSModuleDef *m;

    (void)opaque;
    if (strcmp(name, "t") == 0)
        snprintf(src, sizeof(src), "globalThis.order = []; await 0;");
    else
        snprintf(src, sizeof(src), "import \"t\"; order.push(%d);", atoi(name + 1));
    v = JS_Eval(ctx, src, strlen(src), name, JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(v))
        return NULL;
    m = JS_VALUE_GET_PTR(v);
    JS_FreeValue(ctx, v);
    return m;
}

/* When the await of a module ends, the modules that waited for it run in the
   order they were reached, and the work of finding them is linear in their
   number: the gathering looks at each parent once and clears each mark
   once. A module imports WAITERS modules that each import the one module
   that awaits: 2 * WAITERS parents and WAITERS + 1 marks. Looking each one
   up in the list of those found so far, as QuickJS did, takes about
   WAITERS^2 / 2 steps. The steps are counted (JS_MODULE_STEP), so that no
   clock decides. */
#define WAITERS 5000
static void test_module_waiters(void)
{
    VM v = vm_new();
    size_t size = (size_t)WAITERS * 24 + 64, len = 0;
    char *src = malloc(size), want[128];
    unsigned long steps;
    JSValue r;
    int i;

    if (!src) {
        CHECK(0, "no memory");
        vm_free(&v);
        return;
    }
    for (i = 0; i < WAITERS; i++)
        len += snprintf(src + len, size - len, "import \"w%d\";\n", i);
    len += snprintf(src + len, size - len, "order.push(-1);");
    JS_SetModuleLoaderFunc(v.rt, NULL, waiter_loader, NULL);
    r = JS_Eval(v.ctx, src, len, "top", JS_EVAL_TYPE_MODULE);
    CHECK(!JS_IsException(r), "top threw");
    JS_FreeValue(v.ctx, r);
    white_module_steps_take();
    drain_jobs(&v);
    steps = white_module_steps_take();
    CHECK(steps == 3 * WAITERS + 1, "the gathering took %lu steps for %d waiting modules",
          steps, WAITERS);
    snprintf(want, sizeof(want),
             "order.length === %d && order.every((v, i) => v === (i < %d ? i : -1))",
             WAITERS + 1, WAITERS);
    CHECK(eval_is(v.ctx, want, "true"), "the order");
    free(src);
    vm_free(&v);
}

/* The modules of test_module_lookup: t starts a count, and each module w<i>
   imports t and adds one to it. The loader refuses w<lookup_refuse>. */
static int lookup_refuse = -1;

static JSModuleDef *lookup_loader(JSContext *ctx, const char *name, void *opaque)
{
    char src[64];
    JSValue v;
    JSModuleDef *m;

    (void)opaque;
    if (strcmp(name, "t") == 0) {
        snprintf(src, sizeof(src), "globalThis.n = 0;");
    } else if (atoi(name + 1) == lookup_refuse) {
        JS_ThrowReferenceError(ctx, "no module %s", name);
        return NULL;
    } else {
        snprintf(src, sizeof(src), "import \"t\"; n++;");
    }
    v = JS_Eval(ctx, src, strlen(src), name, JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(v))
        return NULL;
    m = JS_VALUE_GET_PTR(v);
    JS_FreeValue(ctx, v);
    return m;
}

/* lookup_top makes the source of a module that imports w0 to w<count - 1>
   (malloc'd). */
static char *lookup_top(int count)
{
    size_t size = (size_t)count * 24 + 64, len = 0;
    char *src = malloc(size);
    int i;

    if (!src) {
        fprintf(stderr, "no memory\n");
        exit(2);
    }
    for (i = 0; i < count; i++)
        len += snprintf(src + len, size - len, "import \"w%d\";\n", i);
    src[len] = '\0';
    return src;
}

/* The look for a module by its name among the loaded modules takes a few
   steps whatever their number (JS_MODULE_STEP counts the modules it goes
   past). A walk of the list of them, as QuickJS did, made the load of
   LOOKUPS modules that each import one more take about LOOKUPS^2 / 2 steps,
   and each import.meta as many as there were modules. A module the host
   refused leaves the modules loaded with it freed, and they load again; of
   two modules of one name, the first loaded is the one found, as in the
   list. */
#define LOOKUPS 2000
static void test_module_lookup(void)
{
    static const char meta[] =
        "globalThis.metas = 0;\n"
        "for (let i = 0; i < 1000; i++) metas += typeof import.meta === \"object\";";
    VM v = vm_new();
    char *src = lookup_top(LOOKUPS), buf[64];
    unsigned long steps;
    JSValue r;

    JS_SetModuleLoaderFunc(v.rt, NULL, lookup_loader, NULL);
    lookup_refuse = LOOKUPS / 2;
    r = JS_Eval(v.ctx, src, strlen(src), "top", JS_EVAL_TYPE_MODULE);
    CHECK(JS_IsException(r), "top ran with w%d refused", LOOKUPS / 2);
    JS_FreeValue(v.ctx, JS_GetException(v.ctx));
    lookup_refuse = -1;

    white_module_steps_take();
    r = JS_Eval(v.ctx, src, strlen(src), "top", JS_EVAL_TYPE_MODULE);
    steps = white_module_steps_take();
    CHECK(!JS_IsException(r), "top threw");
    JS_FreeValue(v.ctx, r);
    drain_jobs(&v);
    CHECK(steps <= 4 * LOOKUPS, "the looks for %d modules went past %lu", LOOKUPS, steps);
    snprintf(buf, sizeof(buf), "%d", LOOKUPS);
    CHECK(eval_is(v.ctx, "n", buf), "the modules that ran");

    r = JS_Eval(v.ctx, meta, strlen(meta), "meta", JS_EVAL_TYPE_MODULE);
    steps = white_module_steps_take();
    CHECK(!JS_IsException(r), "meta threw");
    JS_FreeValue(v.ctx, r);
    drain_jobs(&v);
    CHECK(steps <= 4 * 1000, "1000 import.meta went past %lu modules", steps);
    CHECK(eval_is(v.ctx, "metas", "1000"), "import.meta");

    compile_module(&v, "d", "export var d = 1;");
    compile_module(&v, "d", "export var d = 2;");
    {
        static const char e[] = "import { d } from \"d\"; globalThis.dv = d;";

        r = JS_Eval(v.ctx, e, strlen(e), "e", JS_EVAL_TYPE_MODULE);
    }
    CHECK(!JS_IsException(r), "e threw");
    JS_FreeValue(v.ctx, r);
    drain_jobs(&v);
    CHECK(eval_is(v.ctx, "dv", "1"), "the module d found");
    free(src);
    vm_free(&v);
}


/* A module whose compile fails, or whose bytecode cannot be read, is freed
   with its name taken out of the table of the loaded modules and of their
   count: the count of names stayed, with the module gone, and the table
   doubled for names no module had. 1,000 failed compiles of `export {` and
   1,000 failed reads of a module leave the one module read whole. */
#define FAILED_LOADS 1000
static void test_module_failed_names(void)
{
    VM v = vm_new();
    JSValue r;
    uint8_t *buf;
    size_t size;
    int i, bits;
    uint32_t names;

    for (i = 0; i < FAILED_LOADS; i++) {
        r = JS_Eval(v.ctx, "export {", 8, "bad", JS_EVAL_TYPE_MODULE);
        CHECK(JS_IsException(r), "export { compiled");
        JS_FreeValue(v.ctx, JS_GetException(v.ctx));
    }
    names = white_module_names(v.ctx, &bits);
    CHECK(names == 0 && bits == 4, "after %d failed compiles: %u names, a table of 2^%d",
          FAILED_LOADS, names, bits);

    r = JS_Eval(v.ctx, "export var v = 1;", 17, "rb", JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(r), "rb threw");
    buf = JS_WriteObject(v.ctx, &size, r, JS_WRITE_OBJ_BYTECODE);
    JS_FreeValue(v.ctx, r);
    CHECK(buf != NULL && size > 8, "no bytecode");
    for (i = 0; buf && i < FAILED_LOADS; i++) {
        r = JS_ReadObject(v.ctx, buf, size - 1, JS_READ_OBJ_BYTECODE);
        CHECK(JS_IsException(r), "a short read of rb worked");
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, JS_GetException(v.ctx));
    }
    js_free(v.ctx, buf);
    names = white_module_names(v.ctx, &bits);
    CHECK(names == 1 && bits == 4, "after %d failed reads: %u names, a table of 2^%d",
          FAILED_LOADS, names, bits);
    vm_free(&v);
}

/* The modules m imports, for test_module_read_oom. */
static JSModuleDef *read_loader(JSContext *ctx, const char *name, void *opaque)
{
    static const char src[] = "export var a = 1, q = 2;";
    JSValue v = JS_Eval(ctx, src, strlen(src), name, JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    JSModuleDef *m;

    (void)opaque;
    if (JS_IsException(v))
        return NULL;
    m = JS_VALUE_GET_PTR(v);
    JS_FreeValue(ctx, v);
    return m;
}

/* A module read from bytecode that gets no memory for its list of the
   modules it requests, of its exports or of its imports fails with out of
   memory. The count of each list was set before its array was made, and
   the free of the module read the entries of the array it did not get.
   Each allocation of the read of a module with every list fails in turn. */
static void test_module_read_oom(void)
{
    static const char src[] =
        "import { a } from \"x\"; export * from \"y\"; export { q as r } from \"z\";\n"
        "export var v = a; export function f() { return v; }";
    VM v = vm_new(), w;
    JSValue c, obj;
    uint8_t *buf;
    size_t len;
    char *t;
    int fail, n, failed = 0;

    JS_SetModuleLoaderFunc(v.rt, NULL, read_loader, NULL);
    c = JS_Eval(v.ctx, src, strlen(src), "m", JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(c), "m threw");
    buf = JS_WriteObject(v.ctx, &len, c, JS_WRITE_OBJ_BYTECODE);
    CHECK(buf != NULL, "m cannot be written");
    JS_FreeValue(v.ctx, c);
    for (fail = 1; buf; fail++) {
        w = vm_new();
        obj = white_read_failing(w.ctx, buf, len, fail, &n);
        if (JS_IsException(obj)) {
            t = exception_text(w.ctx);
            CHECK(t && strstr(t, "out of memory"), "allocation %d failing: %s", fail, t ? t : "no exception");
            free(t);
            failed++;
        }
        JS_FreeValue(w.ctx, obj);
        vm_free(&w);
        if (fail > n)
            break;
    }
    CHECK(failed >= 3, "%d reads failed", failed);
    js_free(v.ctx, buf);
    vm_free(&v);
}

/* The modules of test_module_same_name: the load of t makes a module named
   s too, loaded after the one being compiled, and missing is refused. */
static JSModuleDef *same_loader(JSContext *ctx, const char *name, void *opaque)
{
    static const char s2[] = "export var v = 2;", t[] = "export var w = 1;";
    JSValue v;
    JSModuleDef *m;

    (void)opaque;
    if (strcmp(name, "t") != 0) {
        JS_ThrowReferenceError(ctx, "no module %s", name);
        return NULL;
    }
    v = JS_Eval(ctx, s2, strlen(s2), "s", JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(v))
        return NULL;
    JS_FreeValue(ctx, v);
    v = JS_Eval(ctx, t, strlen(t), "t", JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(v))
        return NULL;
    m = JS_VALUE_GET_PTR(v);
    JS_FreeValue(ctx, v);
    return m;
}

/* Many modules of one name, as an <eval> made a module again and again,
   are added to the table of the loaded modules in a few steps each: only
   the first of a name is in its list, and the others follow it. Each was
   put at the end of the list of its bucket, which all of them shared, and
   a grow did that again for each: about SAME^2 / 2 steps. The first loaded
   of a name is the one found, and when it goes, the next loaded of its
   name takes its place: s fails in its resolution after its load of t
   made another s, which is then the one found. */
#define SAME 4000
static void test_module_same_name(void)
{
    static const char first[] = "import \"t\"; import \"missing\"; export var v = 1;";
    static const char e[] = "import { v } from \"s\"; globalThis.sv = v;";
    VM v = vm_new();
    unsigned long steps;
    char src[64];
    JSValue r;
    int i;

    JS_SetModuleLoaderFunc(v.rt, NULL, same_loader, NULL);
    r = JS_Eval(v.ctx, first, strlen(first), "s", JS_EVAL_TYPE_MODULE);
    CHECK(JS_IsException(r), "the first s ran");
    JS_FreeValue(v.ctx, JS_GetException(v.ctx));
    white_module_steps_take();
    for (i = 3; i < SAME + 3; i++) {
        snprintf(src, sizeof(src), "export var v = %d;", i);
        r = JS_Eval(v.ctx, src, strlen(src), "s", JS_EVAL_TYPE_MODULE);
        CHECK(!JS_IsException(r), "s %d threw", i);
        JS_FreeValue(v.ctx, r);
    }
    steps = white_module_steps_take();
    CHECK(steps <= 2 * SAME, "%d modules of one name took %lu steps", SAME, steps);
    drain_jobs(&v);
    r = JS_Eval(v.ctx, e, strlen(e), "e", JS_EVAL_TYPE_MODULE);
    CHECK(!JS_IsException(r), "e threw");
    JS_FreeValue(v.ctx, r);
    drain_jobs(&v);
    CHECK(eval_is(v.ctx, "sv", "2"), "the s found");
    vm_free(&v);
}

/* The modules of test_module_load_stops: those of lookup_loader, or with
   load_c modules of C that the host makes with no compile and that count
   their runs in load_ran. The load of w<load_stop_at> asks the host to stop
   the runtime, as a host does from another thread (host_interrupt), and the
   flag is set at its next reading. load_after counts the modules loaded
   after that one. */
static int load_c, load_ran, load_stop_at = -1, load_stopped, load_after;

static int load_c_init(JSContext *ctx, JSModuleDef *m)
{
    (void)ctx;
    (void)m;
    load_ran++;
    return 0;
}

static JSModuleDef *stop_loader(JSContext *ctx, const char *name, void *opaque)
{
    if (load_stopped)
        load_after++;
    else if (strcmp(name, "t") != 0 && atoi(name + 1) == load_stop_at) {
        host_interrupt(JS_GetRuntime(ctx));
        load_stopped = 1;
    }
    if (load_c)
        return JS_NewCModule(ctx, name, load_c_init);
    return lookup_loader(ctx, name, opaque);
}

/* A stop asked during the load of a module ends the load before the host
   loads another one: the engine reads the flag before it asks the host for
   each module, so that the wait is that of one module, not of the modules
   left. A module of source reads it in its compile, but one the host makes
   with no compile, as a module of C or of bytecode, did not, and the look
   for each import reads it only after a count of its work: a stop waited
   for the load of every module left. The modules loaded before the stop
   are freed with the failed resolution, and the same import then loads
   and runs each of them once. */
#define LOADS 2000
static void test_module_load_stops(void)
{
    static const int at[] = { 0, 1, LOADS / 2, LOADS - 2 };
    char *src = lookup_top(LOADS), buf[64];
    size_t i;

    snprintf(buf, sizeof(buf), "%d", LOADS);
    for (load_c = 0; load_c < 2; load_c++) {
        for (i = 0; i < countof(at); i++) {
            VM v = vm_new();
            JSValue r;
            char *e;

            JS_SetModuleLoaderFunc(v.rt, NULL, stop_loader, NULL);
            load_stop_at = at[i];
            load_stopped = 0;
            load_after = 0;
            load_ran = 0;
            r = JS_Eval(v.ctx, src, strlen(src), "top", JS_EVAL_TYPE_MODULE);
            CHECK(JS_IsException(r), "%s, stop at w%d: top ran", load_c ? "C" : "source", at[i]);
            JS_FreeValue(v.ctx, r);
            *JS_GetInterruptFlag(v.rt) = 0;
            e = exception_text(v.ctx);
            CHECK(e && strcmp(e, "InternalError: interrupted") == 0, "%s, stop at w%d: %s",
                  load_c ? "C" : "source", at[i], e ? e : "no exception");
            free(e);
            CHECK(load_stopped && load_after == 0, "%s, stop at w%d: %d modules loaded after it",
                  load_c ? "C" : "source", at[i], load_after);
            load_stop_at = -1;
            load_stopped = 0;

            r = JS_Eval(v.ctx, src, strlen(src), "top", JS_EVAL_TYPE_MODULE);
            CHECK(!JS_IsException(r), "%s, stop at w%d: top threw after the stop",
                  load_c ? "C" : "source", at[i]);
            JS_FreeValue(v.ctx, r);
            drain_jobs(&v);
            if (load_c)
                CHECK(load_ran == LOADS, "C, stop at w%d: %d modules ran", at[i], load_ran);
            else
                CHECK(eval_is(v.ctx, "n", buf), "source, stop at w%d: the modules that ran", at[i]);
            vm_free(&v);
        }
    }
    free(src);
}

int main(int argc, char **argv)
{
    /* one case of test_gc_in_free, in a process of its own */
    if (argc == 4 && strcmp(argv[1], "in-free") == 0) {
        failures = 0;
        in_free_case(atoi(argv[2]), atoi(argv[3]));
        fflush(stdout);
        host_leave(failures ? 1 : 0);
    }
    /* one case of test_compile_one_failure, in a process of its own */
    if (argc == 6 && strcmp(argv[1], "compile-one") == 0)
        compile_one(atoi(argv[2]), (size_t)strtoul(argv[3], NULL, 10), atoi(argv[4]), argv[5]);
    /* the sweep of one script of test_regvm_convert_alloc, in a process of
       its own */
    if (argc == 5 && strcmp(argv[1], "regvm-convert") == 0)
        regvm_convert_one(atoi(argv[2]), atoi(argv[3]), strcmp(argv[4], "small") == 0);
    /* one case of test_max_index, in a process of its own */
    if (argc == 3 && strcmp(argv[1], "max-index") == 0) {
        failures = 0;
        max_index_case(atoi(argv[2]));
        fflush(stdout);
        host_leave(failures ? 1 : 0);
    }
    /* one case of test_max_props, in a process of its own */
    if (argc == 3 && strcmp(argv[1], "max-props") == 0) {
        failures = 0;
        max_props_case(atoi(argv[2]));
        fflush(stdout);
        host_leave(failures ? 1 : 0);
    }
    self_path = argv[0];
    test_json_string_run();
    test_json_quote_run();
    test_json_runs_oom();
    test_json_run_too_long();
    test_json_run_limit();
    test_json_plain_run();
    test_json_count_high_bytes();
    test_json_key_cache();
    test_json_stringify_oom();
    test_shared_shape_oom();
    test_json_stringify_list_oom();
    test_json_walk();
    test_backtrace_attribute_checks();
    test_backtrace_class();
    test_backtrace_needs_data_property();
    test_backtrace_memory_usage();
    test_object_block();
    test_object_inline_props();
    test_object_inline_oom();
    test_poll_work_gaps();
    test_tag_work_gaps();
    test_buffer_copy_work();
    test_copy_poll_work();
    test_buffer_limit();
    test_object_inline_limit();
    test_object_lit_shared_shape();
    test_putel_atom_shape();
    test_shape_drop_slot();
    test_shape_drop_in_resize();
    test_shape_drop_in_step();
    test_shape_drop_in_step_refused();
    test_shape_cache_set_in_drop();
    test_shape_cache_set_in_off();
    test_shape_cache_proto_replaced();
    test_shape_cache_two_contexts();
    test_putel_atom_each_failing();
    test_object_rest_cache();
    test_object_spread_cache();
    test_shape_ids();
    test_shape_ids_copy_oom();
    test_new_shapes();
    test_new_shapes_each_failing();
    test_counted_call_debt();
    test_key_atom();
    test_json_plain();
    test_young_lists();
    test_young_leaves_no_zero_count();
    test_young_weak_counts();
    test_finalizer_frees_a_root();
    test_object_made_in_the_weak_pass();
    test_deref_in_gc();
    test_registry_job_after_free();
    test_block_gc_near_limit();
    test_block_gc_wait();
    test_block_gc_wait_steps();
    test_block_gc_wait_edge();
    test_gc_alloc_at_limit();
    test_young_weak_table_recount();
    test_gc_in_free();
    test_error_nest();
    test_function_properties_oom();
    test_compile_count();
    test_poll_scan();
    test_poll_insert();
    test_poll_compile();
    test_poll_ret();
    test_poll_checked_read();
    test_array_grow_stops();
    test_object_lit_read();
    test_tail_c_calls();
    test_fused_ops();
    test_typeof_is();
    test_share_local_slots();
    test_max_index();
    test_max_props();
    test_array_from_past_32_bits();
    test_define_existing_oom();
    test_iterator_close_oom();
    test_tdz_checks();
    test_capture_by_value();
    test_closure_var_limit();
    test_closure_gc_in_build();
    test_gc_block();
    test_gc_block_fits();
    test_gc_block_widen();
    test_join_gc_block_small();
    test_join_gc_block_short();
    test_join_fast();
    test_gc_block_held();
    test_by_value_file();
    test_by_value_oom();
    test_short_opcode_index();
    test_fused_edges();
    test_added_vars_oom();
    test_string_append_flat();
    test_string_append_atom();
    test_string_empty_left();
    test_string_add_kinds();
    test_own_keys_flags();
    test_shared_debug_read();
    test_regexp_cache_keeps();
    test_regexp_cache_memory();
    test_regexp_standard();
    test_regexp_split_budget();
    test_regexp_chains();
    test_regexp_literal();
    test_regexp_plan_size();
    test_regexp_first_set();
    test_regexp_split_same();
    test_compile_one_failure(0);
    test_compile_one_failure(1);
    test_emit_tables();
    test_emit_buffer();
    test_regvm_convert_alloc();
    test_regvm_convert_oom_entries();
    test_regvm_scratch_drain();
    test_regvm_crafted();
    test_regvm_crafted_code();
    test_regvm_stack();
    test_regvm_operands();
    test_regvm_run_alloc();
    test_regvm_bad_ret();
    test_regvm_ret_value();
    test_regvm_call_copies();
    test_regvm_dup_get_field();
    test_regvm_incdec_store();
    test_regvm_chain_store();
    test_regvm_loop_rotate();
    test_regvm_dst();
    test_regvm_dst_alloc();
    test_regvm_forin();
    test_regvm_getf_slot();
    test_regvm_putf_step();
    test_regvm_call_record();
    test_spread_alloc();
    test_module_jobs_oom();
    test_module_waiters();
    test_module_lookup();
    test_module_load_stops();
    test_module_failed_names();
    test_module_same_name();
    test_module_read_oom();
    test_spread_refs();
    test_days_to_date();
    CHECK(white_gc_window_count() == 0, "%ld allocations while a collection had counts taken away", white_gc_window_count());
    if (failures) {
        printf("unit-white: %d of %ld checks failed\n", failures, checks);
        return 1;
    }
    printf("unit-white: %ld checks passed\n", checks);
    return 0;
}
