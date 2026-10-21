---
summary: 'The fork changes to the interpreter loop JS_CallInternal: plain pointer conversions instead of __ccgo_up, the cells of Dup and Free as Go variables, values pinned in their stack slots on amd64, set_value written out, the interrupt countdown inline, and likely and unlikely tests that branch on their condition, with what each gains, why it is safe and what to do when upstream changes it'
read_when:
  - Changing fixUp, fixCells, fixPin, fixInline, fixPoll or fixExpect in internal/fixlib
  - fixlib stops on upSites, the cells of JS_CallInternal, pinVars, set_value, inlineCallees, js_poll_interrupts or expectSites after an upstream release
  - About to add a fast path or a helper to the interpreter loop
  - Asking why the Go stack frame of JS_CallInternal has the size it has
---

# The interpreter loop

`JS_CallInternal` runs the bytecode. It is far too large for Go to inline anything into it but the smallest functions (a cost of 20 or less, with or without PGO), so a helper there is one more real call, and the size of its Go stack frame matters. All of these change only the generated Go, through `internal/fixlib`, with no C form.

## Less stack work in the interpreter loop

Release: v0.26.13.

The interpreter loop does less work with the stack. Three changes to the generated Go, with no C hunk:

- ccgo writes every use of an address held in a `uintptr` as `*(**T)(__ccgo_up(x))`, where `__ccgo_up` returns the address of a copy of `x`. It is there only so that `go vet` does not report the conversion. Once inlined, each use stores `x` into a stack slot and loads it back, which arm64 does not optimize away, and the slots made the Go stack frame of `JS_CallInternal` 7744 bytes on darwin/arm64 and 2608 on linux/amd64. Every use is now `*(*T)(unsafe.Pointer(x))`, the same pointer, in all 13 files. Without the call, Go no longer reads `x` before the other calls in the same statement. C does not fix that order either, and no statement in the 13 files depends on it: `fixlib` lists the 23 statements of each file where the order goes away, and stops at a new one. With the next two changes, the frame of `JS_CallInternal` is now 944 and 896 bytes. `go vet`'s `unsafeptr` check reports more lines of `lib/` than before. Nothing runs it there.
- The interpreter inlines `JS_DupValue` and `JS_FreeValue`, and each expansion stored the value into a cell of the TLS frame to read its pointer back. On the 11 files of 64-bit targets the two cells are Go variables. The TLS frame keeps its size, so the stack check counts the same slots. The 32-bit targets keep the cells: a value is a NaN-boxed `uint64` there.
- On amd64, Go's register allocator makes every opcode handler put back the registers of the values the loop keeps for the whole call, mostly as loads from the stack. In the files of darwin, freebsd and linux on amd64 and of windows, the interpreter takes the address of 15 of them (the arguments, the runtime, the context, the bytecode, the frame and its buffers), so they stay in their stack slots and are read where they are used. Nothing escapes. The windows file also serves windows/arm64: the same change, tried on darwin/arm64, was not slower.

A server render of a page of tifa's example shop takes about 12% less time on linux/amd64 (AMD EPYC) and 17% less on darwin/arm64 (Apple M4), with the same HTML.

Tests: `internal/fixlib/up_test.go`, `internal/fixlib/cells_test.go`, `internal/fixlib/pin_test.go`.

### When upstream changes it

What to do when `go run ./internal/fixlib` stops on this change after an upstream release. [upgrading.md](../upgrading.md) has the whole procedure and the order in which to update the lists `fixlib` checks against.

`fixlib` writes every `*(**T)(__ccgo_up(x))` as `*(*T)(unsafe.Pointer(x))` (`fixUp`). The changes before it take both forms and the ones after it (`fixCells`, `fixPin`, `fixDefine`) only the new one, so a match that only knows the upstream form must learn the other (`reDeref` in `internal/fixlib/main.go` gives both). If it reports that `__ccgo_up` is used some other way or that its definition changed, read what ccgo now writes (`internal/fixlib/up.go`). The call made Go read the memory in `x` before the calls to the right of it in the same statement, and the new form does not. `upSites` (`internal/fixlib/upsites.go`) lists each statement where that order goes away, by its function, the addresses that read memory and the functions called after them. If it reports a statement that is not listed, or a count that differs, read the statement and the functions it names. If one of them can change what the address reads, as `f` can in `s->buf[i] = f(s)` when it grows `buf`, `fixUp` must leave that statement as ccgo wrote it, which it cannot do yet. Otherwise run `go run ./internal/fixlib -up` on the files as taken from upstream, put its output into `upsites.go` and say why each new statement is safe. On files `fixlib` has changed, `-up` fails: no `__ccgo_up` is left to find the statements by.

If it reports that `_JS_CallInternal` has other cells than the two of `JS_DupValue` and `JS_FreeValue`, a use of one other than a store of a whole value or a load of its pointer, or that `JSValue` has another layout, read the new interpreter against `internal/fixlib/cells.go`: a cell may only become a Go variable if nothing else reads or writes its bytes. `TestCellsEveryFile` counts the stores and loads of each. Every other use of `bp` there must be a read or a write through `(*T)(unsafe.Pointer(bp + K))` that stays out of the cells, or a use listed in `cellsFrameUses`. If it reports an address in the frame that is not `bp + K` (such as `bp + 32 + 16` or `bp + uintptr(48)`), a read or write that reaches into a cell, a type whose size it does not know, a use of `bp` that is not listed (such as `libc.Xmemset(tls, bp, 0, 80)`) or a listed one that is gone, read that line. A new local in the interpreter moves the offsets and is the usual cause: `go run ./internal/fixlib -cells` prints the uses as the files have them, each with the named local it starts in. If the use cannot touch the bytes of a cell, put it into `cellsFrameUses` with the reason, or fix the count, or add the type to `valueSizes`. If it can, the cells must stay in the frame. In a fixed file, any use of the bytes where the cells were is an error too: nothing writes them any more.

If it reports that a value of `pinVars` (`internal/fixlib/pin.go`) is gone, find what the interpreter now calls it and update the list.

## set_value written out in the interpreter

Release: v0.26.16.

The interpreter no longer calls `set_value`: its 46 calls in `JS_CallInternal` are written out in place. Go inlines into a function as big as the interpreter only callees that cost 20 or less, with or without PGO, so each was a real call, made by every `put_loc`, `set_loc`, `put_arg` and `put_var_ref` of a script. `fixlib` turns a call into a block that gives each argument to a variable of the parameter's type, in order, then runs the callee's own body with its parameters renamed to those variables and its labels renamed to labels of their own. An argument that is a variable of the interpreter, such as `ctx10`, is read where the parameter is, with no variable of its own, when nothing can set it while the copy runs and the callee does not set the parameter: 46 copies of `ctx10` made the interpreter's frame on amd64 288 bytes bigger and undid `fixPin`, which keeps `ctx10` in its stack slot. The Go array that `fixFrames` gives `set_value` holds one JSValue, and becomes a Go JSValue in each copy, read as `fixCells` reads the interpreter's own cells.

Why this is safe: the arguments are evaluated once and in order, as for the call, before the callee's locals are declared, so a local cannot change what an argument reads, and `fixlib` stops when the callee has a result, returns early, takes a TLS frame, uses its Go array other than as one JSValue or uses a name that is a local of the interpreter, when a call is not a statement of its own, or when the number of calls changes. `set_value` takes no TLS frame, so the TLS stack a stack check sees is the same. Its text is pinned by fingerprints (`inlineSites`), so an upstream change stops `fixlib` until it is read again. Other changes of `fixlib` match the interpreter as ccgo writes it, so `fixlib` undoes this change first (`restoreInline`) and makes it again last, and checks that undoing it gives back the text it was given, byte for byte. A server render of a page of tifa's example shop on linux/amd64 (AMD EPYC) takes 1.4% less time with this change, with and without PGO, with the same HTML. With the [TLS frames taken by statement](calls-and-tls-frames.md#tls-frames-taken-per-statement) as well, it takes 2.8% less time than with v0.26.15 with PGO (each with an engine profile taken on its own code) and 2.9% less without. `JS_FreeAtom` and `is_strict_mode`, written out too, gained nothing that could be told from noise, so they stay calls. This changes only the generated Go, with no C hunk (`internal/fixlib/inline.go`).

Tests: `internal/fixlib/inline_test.go`, with test262 and the tests of the root package for what the engine gives.

### When upstream changes it

If it reports that `set_value` changed (a fingerprint other than the one in `internal/fixlib/inlinesites.go`), read it: `fixInline` writes its body out in the interpreter, and must still be able to (no result, no early return, no TLS frame, its Go array used only as one JSValue). Then put the new fingerprints in (`go run ./internal/fixlib -inline` prints them). If it reports another number of calls, a call that is not a statement of its own or a name of the callee that is a local of `JS_CallInternal`, read the new interpreter and update `inlineCallees` in `internal/fixlib/inline.go`.

## The interrupt countdown inside the interpreter

Release: v0.26.16.

The interpreter counts its interrupt counter down in its own body. Each of the eight interrupt polls of `JS_CallInternal` (at its entry and at each backward jump, so a loop polls twice a pass) called `js_poll_interrupts`, which only counts a counter down and calls `__js_poll_interrupts` once the counter runs out, every 10000 polls. `JS_CallInternal` is too large for Go to inline a function of that cost into it, so every poll was a call. `fixlib` writes each poll as `pollDue(ctx+OFFSET) && __js_poll_interrupts(tls, ctx) != 0`: `pollDue` (`lib/poll.go`) is the countdown, small enough to be inlined, and OFFSET is the counter `js_poll_interrupts` reads. Its text is pinned in `pollSites`. On top of every change before it in [the list](README.md), a render of a page of tifa's example shop takes 0.6% less time with PGO and about as long without, on linux/amd64 (AMD EPYC), with the same HTML. This changes only the generated Go, with no C hunk (`internal/fixlib/poll.go`).

Tests: `lib/poll_test.go` (the handler is still called as often, and stops an endless loop) and `internal/fixlib/poll_test.go`.

### When upstream changes it

If it reports that `js_poll_interrupts` changed (a fingerprint other than the one in `internal/fixlib/pollsites.go`), read it against `pollDue` in `lib/poll.go`: it must still count the counter at the offset `fixPoll` reads down by one and call `__js_poll_interrupts` when the counter is at most zero. Then put the new fingerprints in (`go run ./internal/fixlib -poll` prints them).

## Likely and unlikely tests branch on their condition

Release: not released yet.

QuickJS marks its hot tests as likely or unlikely with `__builtin_expect(!!(x), c)`, which returns a `long`. ccgo writes such a test as `libc.BoolInt64(!!(x)) != 0`, or `libc.BoolInt32(!!(x)) != 0` where a `long` has 32 bits (386, arm and windows). When `x` has `&&` or `||`, Go makes the integer of the condition and then tests it, where it could branch on `x` itself: a `SETcc` and a `TEST` more on each such test. `fixlib` writes each of these tests as `expect64(!!(x))` or `expect32(!!(x))`, of `lib/expect.go`, which return their argument and which Go inlines everywhere, also in `JS_CallInternal`. 728 tests in each file change (862 on linux/386 and linux/arm), 144 of them in `JS_CallInternal`.

Why this is safe: `libc.BoolInt64(b) != 0` is `b` for every `b`, so each test takes the same branch as before. `fixlib` changes only a test of that exact form, a call of `libc.BoolInt64` or `libc.BoolInt32` on `!!(x)` compared with `!= 0`, found by Go's parser, nested ones too. A value made of such a call, or a test without the `!!` of `__builtin_expect`, stays as it is. It checks the number of tests it changes in each file against `expectSites`, and checks that undoing the change gives back the text it was given, byte for byte. Other changes of `fixlib` match the text of these tests, so it makes this change after the frames are freed without a defer, and undoes it first.

A render of the pages of tifa's bench app takes 0.976 to 0.987 of the time of v0.26.17 (the medians of two sessions of 8 rounds, single rounds 0.961 to 0.992), and of the shop's pages 0.983 (0.976 to 0.984 over 5 rounds), on linux/amd64 (AMD EPYC, without PGO), with the same HTML: each round compares the mean of six builds (three for the shop) whose code sits at different addresses with the same for v0.26.17. A render runs 1.8% fewer instructions for the bench pages and 1.6% fewer for the shop's (callgrind), most of them in `JS_CallInternal`, where the change also moves which values Go keeps in registers. This changes only the generated Go, with no C hunk (`internal/fixlib/expect.go`).

Tests: `internal/fixlib/expect_test.go`, with test262 and the tests of the root package for what the engine gives.

### When upstream changes it

If it reports a number of tests other than `expectSites` has for a file, read the new or the missing ones: each must still be a test of `__builtin_expect`, `libc.BoolInt64(!!(x)) != 0` or `libc.BoolInt32(!!(x)) != 0`. Then put the new numbers in (`go run ./internal/fixlib -expect` prints them).
