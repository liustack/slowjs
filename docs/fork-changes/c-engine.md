---
summary: 'The changes of the C tree in engine/ that the switch to the new engine will bring and that no release has yet: the small libc, time and abort through host callbacks, no blocking Atomics, class IDs per runtime, the report of JS_DumpMemoryUsage in a buffer, recursion bounded by counted depths instead of the stack, and which checks and tests guard them'
read_when:
  - Changing engine/libc, engine/host.h, or what the C tree asks of its host
  - Changing where the C tree counts the call depth or the internal depth, or adding a recursion to it
  - Asking why Atomics.wait throws in the C tree, or why test262.conf in engine/test excludes tests
  - Calling JS_NewClassID, JS_DumpMemoryUsage or abort in the C tree, or writing the Go side of them
  - Looking for what changes for users when the new engine replaces the one of lib/
---

# The C tree of the new engine

`engine/` is the C engine that will replace the ccgo one of `lib/` (see [architecture.md](../architecture.md)). Nothing here is in the Go API or in a release yet: the generated Go and the interpreter of `lib/goasm_*` are made from the pinned archive, `lib/internal/patch/quickjs.diff` and `cpatch.py`, and do not read `engine/`. What follows changes only `engine/`. It is written down here so that the switch has the list of what users will see, and the Go side of each item is known. When the switch is made, each row goes to the changelogs in both languages.

| Change | What changes for a program | Checked by |
| --- | --- | --- |
| The engine builds with `-ffreestanding -nostdinc` against `engine/libc` | `Math.*` gives the same bits on linux/amd64 as the Go engine today. On the other targets it can change in the last digit (see "The small libc") | `check-symbols.sh`, `libc-test.sh`, `math.sh` |
| `Date` and the seed of `Math.random` come from `host_now` | the Go host answers with the clock of the VM | the native host, `HOST_CLOCK` |
| `abort` is a request to the host: `host_call` with `HOST_CALL_ABORT` | the Go host panics and the VM can only be closed, as it is now. A trap would be a signal and would end the process | `api-test.c` |
| `Atomics.wait` throws, `Atomics.notify` wakes nobody | `Atomics.wait` throws a `TypeError` as the main thread of a browser does. There are no threads in the engine | test262 with `engine/test/test262.conf` |
| `JS_NewClassID(rt, &id)` counts per runtime | the Go side keeps its class IDs per VM | `api-test.c` |
| `JS_DumpMemoryUsage(buf, size, ...)` writes to a buffer | no `FILE`. The Go side gets a string | `api-test.c` |
| The tools that print are gone: `JS_Dump*`, `dump_token` and the like | the `DUMP_*` options of the C do not build | the build |
| Recursion is bounded by counted depths, not by the stack: 5000 calls by default, at most 10000, and 2000 levels of C recursion | the Go engine counts 10000 TLS stack slots, so a script that recursed deeper than 5000 calls gets a `RangeError` sooner. `JS_SetMaxCallDepth` replaces `JS_SetMaxStackSize`. Chains of bound functions, `finally` and iterator helpers that crashed now throw | `unit.c`, test262 |

## The small libc

`engine/libc` is musl's string and math functions at commit `7ada6dd` (the one `modernc.org/libc` v1.77.1 is translated from), a `snprintf` that does the formats the engine uses, and the headers. See its [README](../../engine/libc/README.md).

The math is the same C that the Go engine runs on linux/amd64, built with `-ffp-contract=off -U__FP_FAST_FMA`, so that `log`, `log2` and `pow` take the same branch on every machine. What is tested, and what is not:

- linux/amd64: `engine/test/math.sh` compares, bit for bit, 100000 inputs for each of 33 functions of libc (against `modernc.org/libc`, the musl the Go engine runs) and for 33 `Math` operations as JavaScript sees them (against the Go engine). Both are the same on every result. The JavaScript comparison writes a NaN as the one NaN of JavaScript, so it does not compare the sign and payload of a NaN. The libc comparison does.
- macOS arm64: compared once with the linux/amd64 results, by hand and outside the tests. The JavaScript results are the same. The libc results are the same except for the sign of a NaN, which the processor chooses (positive on arm64, negative on amd64). This is not a test that runs.
- The other targets (linux/arm64, darwin/amd64, windows/amd64) are not compared. So the same last digit on all five targets is the aim of the flags and of the shared C, and is shown for two of them.

On the targets where the Go engine takes `Math` from Go's own `math` package today (darwin and windows) the results change in the last digit from what it gives, which is what the switch is for.


`snprintf` reads a whole conversion, flags to conversion letter, and checks it before it takes any argument for it, a `*` included. It calls `abort` on a floating point conversion, `%n`, a positional argument, a length on `c`, `s`, `p` or `%`, any flag, width or precision written on `%` (a `*` there counts, whatever it would be given), a format that ends inside a conversion, and a width or precision that does not fit an int. A width of INT_MIN given for a `*`, and an output longer than INT_MAX, abort too, but only after the arguments are taken, since they depend on them. A new format in the engine cannot print the wrong thing. It is the `abort` of the small libc, which asks the host like every other abort, and not a trap, which in Go would end the process. `libc-test.c` lists the formats that abort and counts the arguments taken before each one does, and `api-test.c` checks that one reaches the host. The only trap left in `engine/` is the one after the `host_call` in `abort`, for a host that returns.

`check-symbols.sh` proves the symbol level only. The engine files, with the libc, leave undefined nothing but the callbacks of `engine/host.h` and `host_memory_grow` of `engine/heap.h`. The allocator ([allocator.md](../allocator.md)) is among the engine files, and the engine calls no malloc: `dbuf` and `CharRange` have no default allocator, and `stdlib.h` of the libc declares none. It does not prove that the machine code uses no thread local storage, no system call instruction and no fixed address. Those checks come with the generator, not before.

## Time, abort and the host

`engine/host.h` declares what the engine asks of its host. `host_now(vm, kind)` gives the wall clock or a monotonic one in nanoseconds. `host_call(vm, this, argc, argv, magic)` calls a function of the host, and a magic below 0 is a request of the engine: `HOST_CALL_ABORT` for `abort()`. The `vm` is the one `JS_NewRuntime` was given, and `Date` asks the clock of the VM of its runtime. `Math.random` asks no clock: it starts from the seed `JS_NewRuntime` was given, the same in every context of the runtime. `abort` does not know its VM and passes 0. `host_memory_grow` is in `engine/heap.h` and comes with the allocator. Two kinds of `host_call` below 0 are the allocator's: `HOST_CALL_MEMORY_RELEASE` gives a dedicated chunk back and `HOST_CALL_MEMORY_REMAP` grows one (linux, when the host said so with `JS_SetHostRemap`). They run no user code and do not call back into the engine. The other callbacks (`host_poll`, `host_release`) come with the interrupt.

## Depth of calls and of C recursion

The engine never reads the stack pointer. The generated assembly runs on a Go stack, which moves and grows, and keeps the locals whose address is taken on a shadow stack of its own, so an address on the stack says nothing about how deep the engine is. It counts instead, with two counts in the runtime, and throws `RangeError: Maximum call stack size exceeded` past either. Neither is ever reset: each level that is entered is left on every path, and a level that cannot be entered counts nothing, so the error path recurses no further.

- **The call depth** counts each call of a function while it runs. `JS_CallInternal` counts before it sends a call to a class, so a built-in function, a bound function and a proxy count a level as a JavaScript function does. A constructor that `JS_CallConstructorInternal` sends to a class counts there. The fast path of `JS_IteratorNext2`, which calls the C `next` of a built-in iterator directly, counts a level too: a chain of iterator helpers recursed through it with no other count. The exported functions that run code (`JS_Call`, `JS_CallConstructor`, `JS_CallConstructor2`, `JS_Invoke`, `JS_Eval`, `JS_EvalThis`, `JS_EvalFunction`, `JS_ExecutePendingJob`) count one level for the entry of the host, so that a host function that calls back into the engine adds a level even when the path to the next call is long, as in the parser of `JS_Eval`. The engine itself calls forms of them that do not count (`js_call` and the like). The limit is 5000 by default (`JS_DEFAULT_MAX_CALL_DEPTH`) and at most 10000 (`JS_MAX_CALL_DEPTH_LIMIT`). `JS_SetMaxCallDepth(rt, n)` sets it, and returns -1 and changes nothing when `n` is out of that range or the runtime is running code. A plain recursive function gets 2 levels short of the limit when the host started it with `JS_Eval`: one for the entry and one for the global code.
- **The internal depth** counts the recursions of C that make no call: the parser (`js_parse_unary`, `js_parse_postfix_expr`, `js_parse_assign_expr2`, `js_parse_statement_or_decl`, `js_parse_destructuring_element`, `js_parse_function_decl2`, `js_parse_expr_binary`, `js_parse_logical_and_or`, which cut every cycle of it), `json_parse_value`, `internalize_json_property`, `js_json_to_str`, the parser of regexps (`re_parse_disjunction` and `re_parse_nested_class`, through `lre_depth_enter`), module linking and evaluation, `JS_ReadObjectRec` and `JS_WriteObjectRec`, `JS_FlattenIntoArray`, the methods of a proxy that go on to the target (all but call and construct, which count as calls), `JS_OrdinaryIsInstanceOf` on a bound function and `JS_GetFunctionRealm`. Each is a wrapper that counts around the original function, renamed with a `1` at the end. The limit is fixed, `JS_MAX_INTERNAL_DEPTH` (2000), so that the stack the engine can use has a bound for any call depth. A regexp too deep throws `SyntaxError: stack overflow`, as it did.

How deep real code goes: the parser counts about 14 levels for each nested parenthesis, call or array and about 30 for each nested function expression. Rendering the pages of tifa's example shop takes the parser 484 levels deep, its MUI pages 440, the bench pages 238 (with the Intl polyfill), and all of test262 960. Everything else stays under 10 on those pages: JSON 6, regexps 189 counted on top of the parser that compiles them, `JS_GetFunctionRealm` 2. The call depth of those renders stays under 600.

Each count also checks the stack left. `__shadow_remaining()` gives the bytes left: the generator turns it into the room left on the shadow stack, and the native test host gives the room left on its thread's stack. Entering either depth needs `JS_STACK_MARGIN` (64 KiB) left, and an `alloca` of a call (the locals of `JS_CallInternal`, the arguments of a bound function) needs its size on top, or it throws the same `RangeError`. So a dynamic alloca never crosses the end of the shadow stack, whatever the depths allow.

What the counts do not cover yet: the recursions that end by the shape of the data (rope strings, Unicode tables, the printer) and the property layer, whose cycles exist only in the static call graph. They are bounded without a count, and the generator's proof that no cycle is left uncounted needs them rewritten first.

The checks: `unit.c` (see [testing.md](../testing.md#the-c-tree-and-its-native-test-host)) and test262, whose 58 known errors stay the same.

## Atomics without threads

`Atomics.wait` converts its arguments in the order the specification gives and then throws `TypeError: cannot block in this thread`. `Atomics.notify` converts its arguments and returns 0. test262 gives the same 58 known errors as before with 66 tests excluded in `engine/test/test262.conf`, all of `built-ins/Atomics/wait` and `notify`: 7 wait on the main thread for a time out, and 59 start agents with `$262.agent.safeBroadcast`. That function begins with an `Atomics.wait` on a value that differs, which must return at once and now throws. In 53 of the 59 an agent also waits for a wake up or a time out. In 6 nobody waits and only that probe keeps them out. `api-test.c` keeps what those 6 say that still holds: `Atomics.notify` gives 0 when nobody waits, and `Atomics.wait` converts its arguments in the order of the specification (the array and its shared buffer, the index, the value, the timeout) and throws what a conversion throws before it throws that it cannot block. It also checks that a wait with a value that differs, or with no timeout, throws the same.

## When upstream changes it

Nothing here follows upstream: `engine/` is a copy that changes on its own. A release of QuickJS is taken into `lib/` as [upgrading.md](../upgrading.md) says, and into `engine/` by hand.
