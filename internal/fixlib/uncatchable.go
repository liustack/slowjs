// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The engine's own handlers of an exception: each takes it with
// JS_GetException to reject a promise, record a module error or throw it
// again, and how many times. They call _js_get_exception instead, which
// leaves an uncatchable exception pending (lib/uncatchable.go).
var exceptionHandlers = map[string]int{
	"_js_async_function_resume":            1, // rejects the async function's promise
	"_js_async_generator_completed_return": 1, // rejects the return promise
	"_js_async_generator_resume_next":      1, // rejects the next request
	"_js_async_generator_next":             1, // rejects a call on a non-generator
	"_js_load_module_fulfilled":            1, // rejects import()
	"_JS_LoadModuleInternal":               1, // rejects import()
	"_js_dynamic_import_job":               1, // rejects import()
	"_js_dynamic_import":                   1, // rejects import()
	"_js_execute_sync_module":              1, // records the module error
	"_js_inner_module_evaluation":          2, // records the module error
	"_promise_reaction_job":                1, // passes a throwing handler's error on
	"_js_promise_resolve_thenable_job":     1, // rejects when 'then' throws
	"_js_promise_resolve_function_call":    1, // rejects when reading 'then' throws
	"_js_promise_constructor":              1, // rejects when the executor throws
	"_js_promise_try":                      1, // rejects when the callback throws
	"_js_promise_all":                      1, // rejects on an iteration error
	"_js_promise_race":                     1, // rejects on an iteration error
	"_js_async_from_sync_iterator_next":    1, // rejects on an iteration error
}

// exceptionReaders are the other callers of JS_GetException, which keep it:
// they report the exception and are not reached from the Go API.
var exceptionReaders = map[string]int{
	"Xjs_std_dump_error": 1,
}

var (
	reThrowHead       = regexp.MustCompile(`\nfunc XJS_Throw\(tls \*libc\.TLS, (\w+) uintptr, (\w+) TJSValue\) \(r TJSValue\) \{\n`)
	reIteratorClose   = regexp.MustCompile(`\nfunc _JS_IteratorClose\(tls \*libc\.TLS, (\w+) uintptr, \w+ TJSValue, is_exception_pending TBOOL\) \(r int32\) \{\n`)
	reCallInternalCtx = regexp.MustCompile(`\nfunc _JS_CallInternal\(tls \*libc\.TLS, (\w+) uintptr, [^\n]*\{\n`)
	reEntryPoll       = regexp.MustCompile(`\n\tif _js_poll_interrupts\(tls, (\w+)\) != 0 \{\n(?:\t\t[^\n]*\n)+?\t\}\n`)
	reAnyFunc         = regexp.MustCompile(`\nfunc (\w+)\(`)
)

// fixUncatchable keeps an uncatchable exception (an interrupt) pending until
// the embedder takes it, the Go form of the same change in
// lib/internal/patch/quickjs.diff (see lib/uncatchable.go):
//
//   - XJS_Throw returns at once, freeing the new value, while one is
//     pending.
//   - The functions of exceptionHandlers call _js_get_exception instead of
//     XJS_GetException. A new caller of XJS_GetException stops fixlib, as it
//     may turn an uncatchable exception into something a script can catch.
//   - _JS_CallInternal fails right after its interrupt check while one is
//     pending.
//   - _JS_IteratorClose returns at once for one, without calling 'return'.
func fixUncatchable(b []byte) ([]byte, error) {
	// JS_Throw: keep a pending uncatchable exception.
	body, err := funcBody(b, reThrowHead, "XJS_Throw")
	if err != nil {
		return nil, err
	}
	m := reThrowHead.FindSubmatch(b)
	ctx, obj := string(m[1]), string(m[2])
	guard := fmt.Sprintf("\tif _js_uncatchable_pending(tls, %s) {\n\t\tXFreeValue(tls, %s, %s)\n\t\treturn jsvException()\n\t}\n", ctx, ctx, obj)
	if !bytes.Contains(body, []byte(guard)) {
		anchor := fmt.Sprintf("\trt = (*TJSContext)(unsafe.Pointer(%s)).Frt\n", ctx)
		if bytes.Count(body, []byte(anchor)) != 1 {
			return nil, fmt.Errorf("XJS_Throw: want 1 read of the runtime")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, []byte(anchor), []byte(anchor+guard), 1), 1)
	}

	// The engine's handlers of an exception.
	if b, err = fixExceptionHandlers(b); err != nil {
		return nil, err
	}

	// JS_CallInternal: no call while one is pending.
	if body, err = funcBody(b, reCallInternalCtx, "_JS_CallInternal"); err != nil {
		return nil, err
	}
	ctx = string(reCallInternalCtx.FindSubmatch(b)[1])
	guard = fmt.Sprintf("\tif _js_uncatchable_pending(tls, %s) {\n\t\treturn jsvException()\n\t}\n", ctx)
	if !bytes.Contains(body, []byte(guard)) {
		loc := reEntryPoll.FindSubmatchIndex(body)
		if loc == nil || string(body[loc[2]:loc[3]]) != ctx {
			return nil, fmt.Errorf("_JS_CallInternal: cannot find the interrupt check")
		}
		if before := body[:loc[0]]; bytes.Contains(before, []byte("\n\tfor {")) || !bytes.Contains(before, []byte("tls.Alloc(")) {
			return nil, fmt.Errorf("_JS_CallInternal: the first interrupt check is not at the entry")
		}
		fixed := append(append(append([]byte{}, body[:loc[1]]...), guard...), body[loc[1]:]...)
		b = bytes.Replace(b, body, fixed, 1)
	}

	// JS_IteratorClose: no 'return' for one.
	if body, err = funcBody(b, reIteratorClose, "_JS_IteratorClose"); err != nil {
		return nil, err
	}
	ctx = string(reIteratorClose.FindSubmatch(b)[1])
	guard = fmt.Sprintf("\t\tif _js_uncatchable_pending(tls, %s) {\n\t\t\treturn -int32(1)\n\t\t}\n", ctx)
	if !bytes.Contains(body, []byte(guard)) {
		// The test that takes the exception out, not the one that throws
		// it again at the end.
		anchor := "\n\tif is_exception_pending != 0 {\n"
		take := anchor + "\t\tex_obj = "
		if bytes.Count(body, []byte(take)) != 1 {
			return nil, fmt.Errorf("_JS_IteratorClose: want 1 test of is_exception_pending that takes the exception")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, []byte(take), []byte(anchor+guard+"\t\tex_obj = "), 1), 1)
	}
	return b, nil
}

// fixExceptionHandlers makes the functions of exceptionHandlers call
// _js_get_exception, and checks that every other caller of XJS_GetException
// is one of exceptionReaders.
func fixExceptionHandlers(b []byte) ([]byte, error) {
	const get, internal = "XJS_GetException(tls, ", "_js_get_exception(tls, "
	for name, n := range exceptionHandlers {
		re := regexp.MustCompile(`\nfunc ` + regexp.QuoteMeta(name) + `\(tls \*libc\.TLS, [^\n]*\{\n`)
		body, err := funcBody(b, re, name)
		if err != nil {
			return nil, err
		}
		fixed := bytes.ReplaceAll(body, []byte(get), []byte(internal))
		if got := bytes.Count(fixed, []byte(internal)); got != n {
			return nil, fmt.Errorf("%s: want %d calls of JS_GetException, found %d", name, n, got)
		}
		b = bytes.Replace(b, body, fixed, 1)
	}

	callers := map[string]int{}
	heads := reAnyFunc.FindAllSubmatchIndex(b, -1)
	for i, h := range heads {
		end := len(b)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		if n := bytes.Count(b[h[1]:end], []byte(get)); n != 0 {
			callers[string(b[h[2]:h[3]])] = n
		}
	}
	var unknown []string
	for name, n := range callers {
		if exceptionReaders[name] != n {
			unknown = append(unknown, fmt.Sprintf("%s (%d)", name, n))
		}
	}
	for name, n := range exceptionReaders {
		if callers[name] != n {
			unknown = append(unknown, fmt.Sprintf("%s (%d, want %d)", name, callers[name], n))
		}
	}
	if len(unknown) != 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("calls of JS_GetException not in exceptionHandlers or exceptionReaders: %s: check whether each one hands an uncatchable exception to a script", strings.Join(unknown, ", "))
	}
	return b, nil
}
