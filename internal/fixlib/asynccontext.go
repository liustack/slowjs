// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var asyncContextF = flag.Bool("asynccontext", false, "print the fingerprints of the functions fixAsyncContext changes or relies on, after the earlier final rewrites, in the form of asyncContextSites, to update it after a review")

const (
	// asyncContextHandler is the line of perform_promise_then that stores the
	// handler of a reaction, which it holds a reference to.
	asyncContextHandler = "\t\t(*TJSPromiseReactionData)(unsafe.Pointer(rd)).Fhandler = v6\n"
	// asyncContextThenable is the first line of the arguments of the job that
	// js_promise_resolve_function_call queues to call the then of a thenable,
	// which it holds a reference to and frees after.
	asyncContextThenable = "\t(*(*[3]TJSValue)(unsafe.Pointer(bp + 32)))[0] = (*TJSPromiseFunctionData)(unsafe.Pointer(s)).Fpromise\n"
	asyncContextComment  = "// Changed by internal/fixlib: the handler keeps the async context (lib/asynccontext.go).\n"
)

// asyncContextEdits are the changes of fixAsyncContext, each of the text old,
// once in the function fn, into new. Each new is undone into its old.
var asyncContextEdits = []struct{ fn, old, new string }{
	{"_perform_promise_then", asyncContextHandler, "\t\t" + asyncContextComment + "\t\t(*TJSPromiseReactionData)(unsafe.Pointer(rd)).Fhandler = asyncContextWrap(tls, ctx2, v6)\n"},
	{"_js_promise_resolve_function_call", asyncContextThenable, "\t" + asyncContextComment + "\tthen = asyncContextWrap(tls, ctx2, then)\n" + asyncContextThenable},
}

var (
	// asyncContextFuncs are the functions fixAsyncContext changes or relies
	// on: perform_promise_then registers the reactions, promise_reaction_job
	// calls their handlers, js_promise_resolve_function_call queues the job
	// that calls the then of a thenable, and js_promise_resolve_thenable_job
	// is that job.
	asyncContextFuncs = []string{"_perform_promise_then", "_promise_reaction_job", "_js_promise_resolve_function_call", "_js_promise_resolve_thenable_job"}

	// A store into the handler of a reaction, and a job that calls the then
	// of a thenable: fixAsyncContext changes the one of each it knows.
	reAsyncContextStore = regexp.MustCompile(`\(\*TJSPromiseReactionData\)\(unsafe\.Pointer\([A-Za-z0-9_]+\)\)\.Fhandler = `)
	reAsyncContextJob   = regexp.MustCompile(`__ccgo_fp\(_js_promise_resolve_thenable_job\)`)
)

// asyncContextFingerprint is the fingerprint of asyncContextFuncs in b, which
// has none of the change.
func asyncContextFingerprint(b []byte) (string, error) {
	h := sha256.New()
	for _, name := range asyncContextFuncs {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return "", err
		}
		h.Write(body)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}

// fixAsyncContext makes perform_promise_then pass the handler of each
// reaction through asyncContextWrap of lib/asynccontext.go, so the handler
// runs with the async context the runtime had when it was registered, and
// js_promise_resolve_function_call pass the then of a thenable through it,
// so the job that calls it runs with the context of the resolve, as in Node.
//
// The text of asyncContextFuncs is pinned in asyncContextSites, and the two
// sites it changes must be the only store into the handler of a reaction and
// the only queueing of the thenable job: when a release changes any of that,
// fixAsyncContext stops, and the new code must be read against
// lib/asynccontext.go before the sites are updated (go run ./internal/fixlib
// -asynccontext prints the fingerprints).
//
// It is the last final change, and its exact inverse runs first.
func fixAsyncContext(b []byte, name string) ([]byte, error) {
	bare, err := restoreAsyncContext(b, name)
	if err != nil {
		return nil, err
	}
	got, err := asyncContextFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := asyncContextSites[name]; got != want {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/asynccontext.go and docs/fork-changes/go-api.md, then update asyncContextSites (go run ./internal/fixlib -asynccontext)", name, strings.Join(asyncContextFuncs, ", "), got, want)
	}
	if n := len(reAsyncContextStore.FindAll(bare, -1)); n != 1 {
		return nil, fmt.Errorf("%s: %d stores into the handler of a promise reaction, want 1: read the new ones against lib/asynccontext.go, each handler must keep the async context", name, n)
	}
	if n := len(reAsyncContextJob.FindAll(bare, -1)); n != 1 {
		return nil, fmt.Errorf("%s: the thenable job is queued %d times, want once: read the new ones against lib/asynccontext.go, each then must keep the async context", name, n)
	}
	out := bare
	for _, e := range asyncContextEdits {
		body, err := funcBody(out, funcHead(e.fn), e.fn)
		if err != nil {
			return nil, err
		}
		if n := bytes.Count(body, []byte(e.old)); n != 1 {
			return nil, fmt.Errorf("fixAsyncContext: %s: %q %d times, want once", e.fn, firstLine(e.old), n)
		}
		out = bytes.Replace(out, body, bytes.Replace(body, []byte(e.old), []byte(e.new), 1), 1)
	}
	if back, err := restoreAsyncContext(out, name); err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("fixAsyncContext: restoreAsyncContext does not give the input back (%v)", err)
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("fixAsyncContext: the file has a part of the change")
	}
	return out, nil
}

// restoreAsyncContext undoes fixAsyncContext.
func restoreAsyncContext(b []byte, _ string) ([]byte, error) {
	for i := len(asyncContextEdits) - 1; i >= 0; i-- {
		e := asyncContextEdits[i]
		switch n := bytes.Count(b, []byte(e.new)); n {
		case 0:
		case 1:
			body, err := funcBody(b, funcHead(e.fn), e.fn)
			if err != nil {
				return nil, err
			}
			if !bytes.Contains(body, []byte(e.new)) {
				return nil, fmt.Errorf("restoreAsyncContext: %q is outside %s", firstLine(e.new), e.fn)
			}
			b = bytes.Replace(b, body, bytes.Replace(body, []byte(e.new), []byte(e.old), 1), 1)
		default:
			return nil, fmt.Errorf("restoreAsyncContext: %d copies of %q", n, firstLine(e.new))
		}
	}
	if bytes.Contains(b, []byte("asyncContextWrap(")) {
		return nil, fmt.Errorf("restoreAsyncContext: asyncContextWrap is called where fixAsyncContext does not put it")
	}
	return b, nil
}

func asyncContextReport(read func(string) ([]byte, error)) error {
	files, err := filepath.Glob("lib/ccgo_*.go")
	if err != nil {
		return err
	}
	fmt.Println("var asyncContextSites = map[string]string{")
	for _, fn := range files {
		b, err := read(fn)
		if err != nil {
			return err
		}
		name := filepath.Base(fn)
		// The final changes before fixAsyncContext.
		for _, fix := range []func([]byte, string) ([]byte, error){fixExpect, fixFastFrames, fixEphemeron, fixYoungGC, fixSharedDebug, fixGoasm} {
			if b, err = fix(b, name); err != nil {
				return err
			}
		}
		sum, err := asyncContextFingerprint(b)
		if err != nil {
			return err
		}
		fmt.Printf("\t%q: %q,\n", name, sum)
	}
	fmt.Println("}")
	return nil
}
