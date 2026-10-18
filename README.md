# quickjs (liustack fork)

This repository is a fork of two modernc.org modules, merged into one Go module:

| Upstream module | Upstream repository | Here |
|---|---|---|
| `modernc.org/quickjs` (the Go API) | https://gitlab.com/cznic/quickjs | repository root, package `github.com/liustack/quickjs` |
| `modernc.org/libquickjs` (QuickJS transpiled to Go by ccgo) | https://gitlab.com/cznic/libquickjs | `lib/`, package `github.com/liustack/quickjs/lib` |

It starts from quickjs v0.25.0 and libquickjs v0.13.1 (QuickJS release
2026-06-04, commit `3d5e064`), with these changes:

- A VM that runs out of memory inside `JSON.parse`, or anything else that
  converts a string with non-ASCII characters to UTF-8, no longer leaks that
  string. `JS_ToCStringLen2` kept a reference to its argument when allocating
  the UTF-8 copy failed, so the string outlived `VM.Close`. QuickJS, quickjs-ng
  and libquickjs still had this bug when the fork was made. Test:
  `lib/tocstring_oom_test.go`.
- `VM.Call` and `VM.CallValue` report the exception thrown while evaluating
  the function expression (for example out of memory, or a name that is not
  defined) instead of `cannot call a non-function`.
- A string that cannot be converted to Go comes back as the error, not as
  `""`. Converting needs memory for a string with non-ASCII characters and
  for one built by concatenation, so near the memory limit `Eval`, `Call`,
  the JSON of an `Object`, `Value.MarshalJSON` and host function arguments
  used to get an empty string, and the out of memory exception stayed in
  the context. A BigInt whose `toString` throws now reports that exception
  instead of a made-up error. `ErrorFromValue` has no error result: a field
  it cannot read stays empty, and the exception is dropped. Reading the
  name and message of a thrown value that is not an object (`throw null`,
  which the engine throws near the memory limit, `throw undefined`, a
  symbol) no longer leaves a TypeError in the context. Test:
  `gostring_test.go`.
- A Go string made from a JS string is valid UTF-8. A JS string can hold a
  lone surrogate, half of a surrogate pair without the other half, which
  code makes when it cuts a string inside an emoji. `JS_ToCStringLen2`
  wrote it as the three bytes of its code point, which is not UTF-8, so a
  page rendered to Go had bytes a browser shows as three replacement
  characters. Now each lone surrogate becomes U+FFFD, as Go's
  `utf16.Decode`, `TextEncoder` and Node's `Buffer` make it. Test:
  `surrogate_test.go`.
- A Go string that is not valid UTF-8 becomes a JS string with U+FFFD for
  each byte that does not start a valid sequence, as Go decodes it
  (`for range`, `[]rune`, `utf16.Encode`) and as `encoding/json` writes it,
  so a string passed as it is and one inside a struct, which goes as JSON,
  arrive the same. This covers arguments of `Call`, results of Go
  functions, map keys and values, `NewString`, `NewAtom`, the message of an
  error a Go function returns, function names and module names. The engine
  used to skip the bytes after an invalid one, ASCII included: `"\x80a"`
  became a single U+FFFD and lost the `a`, `"\xc0\x80"` gave one U+FFFD
  instead of two, and the three-byte form of a surrogate (CESU-8, `0xED 0xA0`
  and on) became that surrogate, so two of them made an emoji. A sequence cut
  short, such as `"\xe4\xb8"`, now gives one U+FFFD per byte, where a
  browser's `TextDecoder` gives one for the sequence. Source code is not
  changed: the parser still reports invalid UTF-8 as a `SyntaxError`, and
  still takes a surrogate in the three-byte form. Test:
  `invalidutf8_test.go`.
- Page code can no longer catch an interrupt (`VM.Interrupt` or the eval
  timeout) through a promise or an iterator. The engine already skipped
  every `catch` and `finally` block for an interrupt, but its own handlers
  of an exception went on with it: an async function and a promise job
  turned it into a rejection, which an `await`, a `.catch` or a `.then`
  handler then received, and so did the `Promise` constructor,
  `Promise.try`, `Promise.all` and the other combinators, async generators
  and modules. A built-in that runs an iterator's `return` method on an
  error (`Array.from`, ...) ran it, then threw the interrupt again as an
  error a `catch` could take, and any later error replaced it. Now an
  uncatchable exception stays pending until the Go call that ran the script
  takes it, and while it is pending every call into javascript fails at
  once. So no `catch`, `finally`, iterator `return`, generator `finally` or
  rejection handler runs, and the call returns the interrupt, as it does
  for a script without promises. Where a built-in only records the error,
  such as a resolve function that rejects its promise when reading `then`
  throws, the code after it runs on until its next function call, which
  fails. `ExecutePendingJobs` stops at the job that failed with an
  interrupt and leaves the rest queued, where it used to run them all.
  Tests: `uncatchable_test.go`, `internal/fixlib/uncatchable_test.go`.
- Running out of memory is uncatchable too, like an interrupt, and the call
  returns `*Error{Name: "InternalError", Message: "out of memory"}`. Page
  code used to catch it, so a framework catching every error to show a
  fallback reported it as an error of the page, and a `finally` block or a
  rejection handler ran with no memory left. Every way the engine runs out
  of memory now ends this way: the engine's own out of memory error, the
  regexp engine running out of memory (it used to throw
  `InternalError: out of memory in regexp execution`, or a `SyntaxError`
  when compiling a pattern), and an error the engine could not build. That
  last case used to throw a catchable `null`, and an error whose message
  could not be allocated got the exception marker as its message, which
  read back as nonsense. Now the engine builds the message first, and if
  the message, the error object or setting the message fails, it throws
  `null`, uncatchable, which the VM reports as the same out of memory
  error. A script that throws `null` itself still gets `null`. Page code
  cannot fill memory on purpose and catch the failure any more: allocating
  until it fails ends the call. The VM stays usable after the call, and
  `Close` frees everything. Tests: `uncatchable_test.go`,
  `lib/outofmemory_test.go`, `internal/fixlib/outofmemory_test.go`.
- An interrupt or running out of memory inside a Go function that
  javascript called ends the whole call too. That covers converting the
  function's arguments or result, and javascript the function evaluates on
  the same VM (`Eval`, `Call` and the like), from a function registered
  with `RegisterFunc` or `RegisterHostFunc`, a `Then` callback, or the
  module loader and normalizer. The Go function still gets the error, but
  the exception stays pending until it returns, so further calls into
  javascript fail at once. Whatever the function returns, the script that
  called it gets the original uncatchable exception. It used to get a
  catchable `TypeError` wrapping the message (for example
  `calling take: argument conversion error: InternalError: out of memory`),
  or the function's own result when it dropped the error, so a timeout in
  nested javascript could be caught or lost and the script ran on. The
  `intl` package now takes its host functions off the global object before
  its install changes anything, so that an install cut short this way
  leaves none of them in sight once the next use has installed.
  Tests: `hostuncatchable_test.go`, `intl/install_test.go`.
- Recursing without end throws `RangeError: Maximum call stack size
  exceeded`, worded as in V8, which a script can catch, with the default
  settings. It used to kill the
  process with a Go stack overflow: the engine sets a new runtime's stack
  size to 1 MB, meant as bytes, but the Go port's stack check counts TLS
  stack slots, so the limit was a million slots, far more than a
  goroutine's stack holds. A new runtime now starts with the port's own
  default, `lib.MaxStackSlots`, as `SetMaxStackSize` already said, and that
  default is 10000 slots rather than 1000: a React render of a page of
  tifa's example shop takes about 470. At the default, recursing without
  end takes up to 128 MB of goroutine stack before it throws. Running out
  of stack used to be `InternalError: stack overflow`. The parser and
  `JSON.parse` now report nesting too deep for the stack with the same
  `RangeError`, as V8's parser does, rather than `SyntaxError: stack
  overflow`. A regular expression nested too deep still fails to compile
  with `SyntaxError: stack overflow`. Tests:
  `stackoverflow_test.go`, `internal/fixlib/stack_test.go`.
- A panic in a Go function that javascript called (`RegisterFunc`,
  `RegisterHostFunc`, a `Then` callback, the module loader or normalizer)
  no longer unwinds through the engine. The VM recovers it, ends the
  javascript with an uncatchable `InternalError` (`panic in Go function
  <name>: <value>`) as for an interrupt, and panics again with the same
  value in the Go code that called javascript: in `Eval` or the like, or in
  the Go function one level up when a Go function called the javascript.
  Page code sees no catch or finally run for it. The panic used
  to unwind through the engine, which kept pointing at stack frames that
  were gone. The VM looked fine after a recover, but the next error it
  made walked those frames, forever when a new frame took the place of an
  old one, so the eval timeout of that VM never fired again. The panic
  value is the same, but its stack trace now starts where the VM panics
  again. Tests: `hostpanic_test.go`.
- Compiling a script, a function body, `eval` code or a regexp with little
  memory left fails with the same out of memory error. The engine used to
  crash there (a Go panic on a write before or past the code buffer, or a
  double free), report a `SyntaxError` the script does not have, or, in
  `Compile`, return bytecode cut short, bytecode with the out of memory
  error left pending, or `errors.New("OOM")`. The parser went on after its
  code buffer failed to grow and patched code in place at positions it had
  noted, past what the buffer held (a Go panic on an empty buffer, or a
  write past the end of a full one), and the passes after it assumed the
  buffer was whole. Now the parser stops at the next token once the code
  buffer of the function it parses failed, and checks it before each patch
  that does not wait for a token, the buffer takes no more bytes once it
  failed, and each pass that reads it back stops with the out of memory
  error. A scope the parser fails to push marks the function's code lost
  too, and its scopes then stay as they are, each later push failing with
  the out of memory error: the pop after a failed push used to take off the
  scope around it, until a pop read before the scope array. The pass that
  resolves jumps frees, when it fails, the jump records of the labels it has
  not reached yet, which used to leak. When shrinking the line table of a
  function fails, the compile keeps the table and leaves no error behind: it
  used to work and leave the out of memory error pending, for the next call
  to report. Tests: `compileoom_test.go`, `internal/fixlib/compile_test.go`.
- Making an error no longer formats its `stack`. The engine records the
  frames (function name, bytecode and position) and formats the text the
  first time `stack` is read, with the same result as before. Code that makes
  many errors and reads few of their stacks, such as the development build of
  React (one error per JSX element), runs much faster. On darwin/arm64, a
  server render of a 100-item React component took 11.2 ms at the median
  with the development build, against 42.1 ms before. The production build
  of the same component, the production build of a four-route app and a
  JSON and array benchmark stayed within 1% of before. Top-level code that
  makes 500 errors per run took 1.8 ms instead of 3.2 ms.
  `stack` is an ordinary writable, configurable data property, so the error
  has the same shape as one whose stack was formatted at once. Once read, it
  is the string. What is different:
  - While the stack is pending, the value of `stack` is an internal object
    that holds the recorded frames. A pending error so carries three extra
    small allocations: that object, its property list and the frame record
    (about 150 bytes for a short stack). Assigning, defining or deleting
    `stack` frees them like any other value. The internal object belongs to
    a private class. The C patch makes it a built-in class of every runtime.
    In Go, its class ID is taken when the package loads, and a runtime
    registers the class the first time it records a lazy stack, a one-time
    cost of about a kilobyte.
  - No code sees the internal object. Everything that reads the value of
    `stack` formats it first: property reads in scripts and through the C
    API, `Object.getOwnPropertyDescriptor(s)` and `JS_GetOwnProperty`, and
    redefining a frozen `stack` with a value (the engine compares the old
    and the new value). What only looks at keys or attributes leaves it
    pending: `Object.keys`, `Object.freeze`, `Object.seal`,
    `Object.isFrozen`, `Object.isSealed`, `propertyIsEnumerable`,
    `__lookupGetter__`, `__lookupSetter__`, and the checks a proxy whose
    target is the error makes after a `has`, `deleteProperty` or `ownKeys`
    trap. `stack` is not enumerable, so
    spreading, `Object.assign`, `Object.entries` and `JSON.stringify` skip
    it.
  - Through a proxy, apart from those three checks, `stack` is formatted
    as before. A lookup of the attributes through a proxy (`Object.keys`,
    `Object.freeze` or `propertyIsEnumerable` on the proxy, for example)
    and the checks a proxy makes after a `get`, `set`,
    `defineProperty` or `getOwnPropertyDescriptor` trap read the error's
    full descriptor, which formats the stack. Results, trap calls and their
    order are the same as when the stack was formatted at once. The cost is
    memory: with almost none left, such an operation can fail with out of
    memory, where the upstream engine had already formatted the stack when
    the error was made.
  - Reading a property named `stack` whose value is an object takes the
    slow path of the interpreter, which is where the stack gets formatted.
    Other property reads do not change. In a loop of 160,000 reads,
    `o.x` took 1% less time than before, `o.stack` holding a number 5%
    more, `o[k]` 1% more, and an inherited `o.x` 9% less. A read takes
    about 19 ns there, so no change is more than 2 ns per read.
  - Until its stack is read or the error is freed, an error keeps the
    bytecode of every function on its stack alive, and with it the realm
    (global object) each function was compiled in. Most of these stay alive
    anyway. A function that runs once and makes an error that something
    keeps is the exception, together with every function it defines: one
    kept error from a function that defines 2,000 others kept 1.30 MB where
    0.30 MB stayed before. An error made in one realm and kept by another
    keeps the first realm and everything its globals hold: with a 4 MiB
    `ArrayBuffer` in the first realm, the runtime used 4.31 MB where
    67 KB stayed before, until the stack was read. Keeping only the file
    name and the line table would need either a copy of the line table for
    every frame of every error, which adds work and memory to every error
    made, the case this change is for, or a reference count of its own for
    the line table, which changes a struct of the engine and so needs a
    regeneration of `lib/`.
  - Reading a stack frees its three allocations, but the holes they leave
    between objects that are still alive are reused only by later
    allocations of the same sizes. After making 10,000 errors, keeping them
    and reading every stack, the allocator held 3.60 MB, against 2.06 MB
    when the stacks were formatted at once.
  - Errors made while top-level code (a script, an eval or a module body)
    is on the stack are still formatted at once, so that an error a library
    keeps from when it loaded does not keep that code alive. So are errors
    that cannot take new properties and errors that already have a `stack`.
  - If there is no memory left when `stack` is first read, the read throws
    the out of memory error, which ends the call, and the stack stays
    pending, so a later read can give the text. Reading the property descriptor formats the stack the
    same way and so can throw too. When an error whose stack is still
    pending is printed (`console.log`, `JS_PrintValue`), formatting the
    stack never throws and never leaves a pending exception: out of memory
    there prints the error without its stack, and a later read can still
    give it. This covers the stack only. The rest of the print, such as the
    message, can run out of memory and leave an exception, as in the
    upstream engine. Printing an error whose stack is already a string works
    as it did before.
  - If the text cannot be formatted when a stack is built at once (any
    allocation of the formatting can fail: growing the text buffer,
    converting a function or file name to UTF-8, making the final string),
    `stack` becomes `null` and no exception is left, in Go and in the C
    patch alike. The upstream engine also gives `null` when the text buffer
    cannot grow. When converting a function name fails, it instead keeps a
    half-formatted stack with the name shown as `<anonymous>` and leaves an
    exception pending.

  Tests: `errorstack_test.go`, `errorstack_unix_test.go`,
  `lib/backtrace_test.go`, `lib/backtrace_round2_test.go`,
  `lib/backtrace_round3_test.go`, `lib/backtrace_proxy_test.go`,
  `internal/fixlib/main_test.go`.
- A script's local time is UTC in every VM, whatever zone the process has.
  The engine used to take the zone from libc's `localtime`, which reads
  `TZ`. The process shares that libc and that variable with Go's
  `time.Local`, its logs and SQLite (through modernc.org/libc), so setting
  `TZ=UTC` to make server rendered pages the same on every machine changed
  all of those too. Now the engine's offset from UTC is always 0 and the
  process's zone is left alone. `Intl`, from package `intl`, defaults to UTC
  as well. What this changes for scripts:
  - `getTimezoneOffset()` is 0, and `getHours`, `getDate`, `getDay` and the
    other local getters give the same values as their `getUTC*`
    counterparts. The local setters (`setHours`, ...) set UTC fields.
  - `new Date(2026, 0, 1)`, `Date.parse` of a date and time without an
    offset (`'2026-01-01T13:05:00'`) and the other local forms read the
    fields as UTC.
  - `toString`, `toDateString` and `toTimeString` print UTC with
    `GMT+0000`. The engine's own `toLocaleString`, `toLocaleDateString` and
    `toLocaleTimeString` print UTC too. Package `intl` replaces those three,
    and its versions print UTC unless given a `timeZone`.

  A script that needs another zone names it to `Intl`, with package
  `intl` registered: `d.toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})`
  or `new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York',
  hour: 'numeric'}).formatToParts(d)`. Without `intl` the engine has no way
  to show another zone. Tests: `dateutc_unix_test.go`,
  `internal/fixlib/main_test.go`.
- Package `intl` formats in every locale Chromium has, from CLDR 48.2
  (cldr-json 48.2.3), where upstream formats en-US only. It covers
  `Intl.DateTimeFormat`, `Intl.NumberFormat`, `Intl.PluralRules` and the
  `toLocale*` methods, and its output is compared with Chromium 156 on about
  60,000 expressions (`intl/golden_test.go`). Without a locale argument the
  locale is en-US, and without a `timeZone` option the zone is UTC: `Intl`
  and the `toLocale*` methods never read the host's settings, and neither
  does the engine's `Date` (see above). `Register` only puts stand-ins on the VM.
  The first use of `Intl` or a `toLocale*` method installs the rest, from
  bytecode compiled once per process. The package documentation lists what
  is not implemented and where the output differs from Chromium.

  What it costs, measured on darwin/arm64 against the en-US version: a
  program that uses `intl` is 1.7 MB larger (1.3 MB of it is the locale
  data). `Register` takes 0.09 ms and 9 KB of the VM's memory, against
  0.85 ms and 46 KB before. The first use in a VM then takes 0.15 ms and
  42 KB more. A language is decoded into the Go heap the first time it is
  used, once per process, in about 0.3 ms and 100 to 180 KB. The first use
  of any language also decodes about 1 MB of shared data. Formatting a date
  takes 3.3 µs in en-US (4.3 µs before) and 3.7 µs in zh-CN, a number
  2.4 µs in en-US (2.4 µs before) and 2.7 µs in de.
- Package `intl` tells daylight saving time from standard time as ICU does,
  whichever form of the tz data Go reads. In the main form, which Linux
  distributions and Go's `time/tzdata` ship, Europe/Dublin's summer time
  is its standard time and its winter time is daylight saving time, an
  hour behind. Africa/Casablanca, Africa/El_Aaiun, Africa/Windhoek and
  Europe/Prague (1946 to 1947) have such negative daylight saving time
  too. ICU builds from the rearguard form, where daylight saving time is
  always ahead of standard time, and macOS ships that form. Go's `IsDST`
  says what the data says, so on Linux the names of these zones came out
  swapped: Dublin in winter was `Irish Standard Time`, the name Chromium
  gives its summer. For these zones and their links the flag now comes
  from a table of the rearguard form of tzdb 2026d, the release in
  Chromium 156's ICU (`intl/tzdst_table.go`, see "Updating the intl locale
  data"). When Go reads another release and the table has another offset
  for a time, or the time is past 2037, the flag is the one of the zone's
  last period with the same offset. Tests: `intl/tzdst_test.go`.
- The eval timeout (`SetEvalTimeout`) runs on the monotonic clock. It used
  to run on the wall clock, so setting the system clock while a script ran
  (NTP, a person, a machine waking from sleep) moved the deadline: set
  back, the timeout fired that much later or never, and set forward, at
  once. Even without a jump the two clocks drift apart by a few
  microseconds per 100 ms, so a server that keeps its own deadline on Go's
  monotonic clock could see the engine interrupt before that deadline. A
  timeout too long for the clock, up to `math.MaxInt64`, never fires: the
  deadline used to wrap around to the past and interrupt the call at once.
  Test: `clock_test.go`.
- `VM.Interrupt` can be called from any goroutine while the VM closes and
  after `Close`, where it does nothing. It used to read the address of the
  interrupt flag, which `Close` frees and then clears, so a watchdog firing
  just as the script finished could write to freed memory or panic on a nil
  pointer. The race detector also reported a race when `Interrupt` stopped a
  script and the VM was closed after the call returned. That one was not
  real: the two goroutines are ordered by the atomic flag, but the flag is in
  C memory, outside Go's heap, where the race detector ignores atomic
  operations. `Interrupt` and `Close` now meet at a mutex in Go memory that
  `Close` never clears. Test: `interrupt_test.go`.
- `*Error` says whether the call ended with an exception no script could
  catch, and why, so that an embedder need not compare names and messages,
  which a script can copy. `Uncatchable` is true when the VM ran out of
  memory (`OutOfMemory`, also when the engine could not build the error and
  threw `null`), when `Interrupt` or the eval timeout stopped the script
  (`Interrupted`), and when a Go function that javascript called panicked
  (neither of the two). The same holds for the error a Go function gets
  from javascript it evaluates. What a script throws is never uncatchable,
  even `new InternalError('interrupted')` or `{name: 'InternalError',
  message: 'out of memory'}`, which used to look exactly like the real
  thing. Taking an uncatchable error runs no page code: `Name` is
  `InternalError`, `Message` and `Stack` are the error's own properties,
  and `Error()` is `InternalError: <message>`. It used to call an
  `Error.prototype.toString` or a `name` getter that page code installed,
  which could rewrite the message. An interrupt that runs out of memory
  building its error counts as `OutOfMemory`. Tests: `uncatchable_test.go`,
  `hostuncatchable_test.go`, `hostpanic_test.go`.
- `RegisterFunc` and `RegisterHostFunc` no longer abort the process when
  the global object already has the name, for example after `globalThis.g =
  1`, or for `Array` or `undefined`. They installed the function with
  `JS_SetPropertyFunctionList`, which is meant for new objects and aborts
  when the property exists. Now the function replaces a configurable
  property, as `Object.defineProperty` would, and stays writable,
  configurable and not enumerable. A property that is not configurable
  (`undefined`, `NaN`, `Infinity`, a script's `var` or `function`) is kept,
  and so is a global object that is not extensible: the call returns an
  error. A name is now taken once per VM as javascript sees it, after
  invalid UTF-8 is replaced, so `"\xc0\x80"` after `"\xe4\xb8"` (both two
  U+FFFD) is `func already registered` rather than an abort, and a name
  with a NUL byte, which ended the name, is an error. A name in brackets
  is a string too: the function lists read `"[Symbol.iterator]"` as that
  symbol and aborted on other names in brackets. Test:
  `register_test.go`.
- Calls cost less. A call of a javascript function, of a native function,
  of a bound function or of a native function with data used to take the
  room for its arguments, locals and stack with `alloca`, which the Go
  port implements with malloc, and on darwin, freebsd and windows also
  with Go memory: a closure and a slice entry per call, 1 to 4 Go
  allocations. Up to 768 bytes, that room is now the end of the
  function's TLS frame. A larger one, and any in `js_c_function_data_call`
  and, on 32-bit targets, in `js_call_bound_function`, which have no
  frame, is malloc'd and freed on return. Either way a call allocates no
  Go memory. And 14 hot small functions, such as `set_value`,
  `free_property` and `__JS_FreeValueRT`, used their TLS frame only to
  read a `JSValue` as a pointer or a float as bits: they now keep those
  bytes in a Go array, and skip `tls.Alloc`, `tls.Free` and the `defer`.
  The stack check counts the same TLS stack slots as before at every
  point, so a script recurses exactly as deep as it used to. A server
  render of a page of tifa's example shop takes about 11% less time on
  linux/amd64 (AMD EPYC) and 14% less on darwin/arm64 (Apple M4), with
  the same HTML, and loading the shop's bytecode into a VM about 11% less
  on linux.

  The 768 bytes bound what this costs in TLS memory on linux. Linux's
  libc keeps the TLS block of every call depth a VM has reached until the
  VM is closed, grown to the largest frame used at that depth and rounded
  up to a power of two, and neither `JS_ComputeMemoryUsage` nor the VM's
  memory limit counts it. The frame of `JS_CallInternal` is 1280 bytes on
  64-bit targets, so it takes a 2 KB block with or without the room, and
  no block grows past 2 KB. The worst case is the same as before: a
  recursion to the default limit of 10000 slots holds about 20.7 MB of
  TLS blocks on linux/amd64, whatever its functions (without the limit, a
  function with 500 locals made that 164 MB). Smaller frames can grow up
  to 1 KB: those of `js_call_bound_function` (16 bytes) and
  `js_call_c_function` (160 bytes). A recursion through bound functions
  can so hold up to about 1 KB more per level: 1000 levels through a
  function bound to 44 arguments hold 5.13 MB rather than 4.14 MB. On
  32-bit targets a block can take up to 1 KB where it took 256 bytes, so
  up to 10 MB in the worst case. Elsewhere the TLS stack frees its pages
  as before. These are
  changes to the generated Go only (`internal/fixlib/alloca.go`,
  `internal/fixlib/frames.go`), with no C hunk. Tests:
  `calldepth_test.go`, `callbench_test.go`,
  `internal/fixlib/alloca_test.go`, `internal/fixlib/frames_test.go`.
- The interpreter loop does less work with the stack. Three changes to the
  generated Go, with no C hunk:
  - ccgo writes every use of an address held in a `uintptr` as
    `*(**T)(__ccgo_up(x))`, where `__ccgo_up` returns the address of a
    copy of `x`. It is there only so that `go vet` does not report the
    conversion. Once inlined, each use stores `x` into a stack slot and
    loads it back, which arm64 does not optimize away, and the slots made
    the Go stack frame of `JS_CallInternal` 7744 bytes on darwin/arm64 and
    2608 on linux/amd64. Every use is now `*(*T)(unsafe.Pointer(x))`, the
    same pointer, in all 13 files. Without the call, Go no longer reads `x`
    before the other calls in the same statement. C does not fix that
    order either, and no statement in the 13 files depends on it: `fixlib`
    lists the 23 statements of each file where the order goes away, and
    stops at a new one. With the
    next two changes, the frame of `JS_CallInternal` is now 944 and 896
    bytes. `go vet`'s `unsafeptr`
    check reports more lines of `lib/` than before. Nothing runs it there.
  - The interpreter inlines `JS_DupValue` and `JS_FreeValue`, and each
    expansion stored the value into a cell of the TLS frame to read its
    pointer back. On the 11 files of 64-bit targets the two cells are Go
    variables. The TLS frame keeps its size, so the stack check counts the
    same slots. The 32-bit targets keep the cells: a value is a NaN-boxed
    `uint64` there.
  - On amd64, Go's register allocator makes every opcode handler put back
    the registers of the values the loop keeps for the whole call, mostly
    as loads from the stack. In the files of darwin, freebsd and linux on
    amd64 and of windows, the interpreter takes the address of 15 of them
    (the arguments, the runtime, the context, the bytecode, the frame and
    its buffers), so they stay in their stack slots and are read where they
    are used. Nothing escapes. The windows file also serves windows/arm64:
    the same change, tried on darwin/arm64, was not slower.

  A server render of a page of tifa's example shop takes about 12% less
  time on linux/amd64 (AMD EPYC) and 17% less on darwin/arm64 (Apple M4),
  with the same HTML. Tests: `internal/fixlib/up_test.go`,
  `internal/fixlib/cells_test.go`, `internal/fixlib/pin_test.go`.
- Defining a new property on a plain object costs less. Object literals,
  spread, class fields, `JSON.parse` and every new function go through
  `JS_DefinePropertyValue`, which handed each property on to
  `JS_DefineProperty` and `JS_CreateProperty` before `add_property` did the
  work, with a TLS frame and a deferred free at each step. Now
  `JS_DefinePropertyValue` first calls `objlitDefine` (`lib/objlit.go`),
  which calls `add_property` itself when the object is extensible and not
  exotic, not a fast array and not the global object, the property has no
  getter or setter, and the object does not have it yet. Every other case
  takes the old path, and so does assignment (`o.x = v`,
  `Object.assign`). Nothing is cached, so memory use is the same. A
  property of an object literal takes about 55 ns rather than 95 on
  linux/amd64 (AMD EPYC), and a server render of a page of tifa's example
  shop about 6% less, with the same HTML. `fixlib` puts the call into all
  13 files and takes the object bit fields `objlitDefine` checks from each
  file's `JS_NewObjectFromShape`, checked against the checks of the same
  bits in the old path, so 32-bit and big-endian targets get their own
  layout. This changes only the generated Go, with no C hunk
  (`internal/fixlib/define.go`). Tests: `lib/objlit_test.go`,
  `lib/objlit_memgrind_test.go`, `internal/fixlib/define_test.go`.

The engine lives in the same module so that a fix to it ships together with
the Go API, without waiting for an upstream release of either module.

```sh
go get github.com/liustack/quickjs
```

The license files of both projects are kept: [LICENSE](LICENSE) and
[LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) at the root, and
[lib/LICENSE](lib/LICENSE), [lib/LICENSE-LIBQUICKJS](lib/LICENSE-LIBQUICKJS)
and [lib/LICENSE_TEST262](lib/LICENSE_TEST262) for the engine.
[intl/LICENSE-DATA](intl/LICENSE-DATA) holds the notices of the locale data
`intl` embeds, from Unicode CLDR and Chromium's ICU data: a program that
imports `intl` distributes that data and has to include those notices in its
documentation or with its copies. Maintenance notes are at the
[end of this file](#maintaining-the-fork).

The rest of this section is the upstream README.

---

[![Go Reference](https://pkg.go.dev/badge/github.com/liustack/quickjs.svg)](https://pkg.go.dev/github.com/liustack/quickjs)
[![LiberaPay](https://liberapay.com/assets/widgets/donate.svg)](https://liberapay.com/jnml/donate)
[![receives](https://img.shields.io/liberapay/receives/jnml.svg?logo=liberapay)](https://liberapay.com/jnml/donate)
[![patrons](https://img.shields.io/liberapay/patrons/jnml.svg?logo=liberapay)](https://liberapay.com/jnml/donate)

![logo_png](logo.png)

# quickjs

Package quickjs is a pure Go embeddable Javascript engine. It supports the
ECMA script 14 ([ES2023]) specification including modules, asynchronous
generators, proxies and BigInt.

See also the original [C QuickJS] library.

The engine has no `Intl`: ECMA-402 is not implemented by the C library this
package wraps. Package [quickjs/intl] adds `Intl.DateTimeFormat`,
`Intl.NumberFormat` and `Intl.PluralRules`, in every locale Chromium has and
over Go's time zone database, for a VM that opts in by calling its `Register`
— once per VM, including one created to replace another, because `Intl` lives
on that VM's global object.

Resolving IANA time zone names needs a zoneinfo database. Go reads the
operating system's, or the copy `time/tzdata` embeds in the binary. That import
belongs in a program's main package rather than in a library, so neither
package makes it for you.

[C QuickJS]: https://bellard.org/quickjs
[quickjs/intl]: https://pkg.go.dev/github.com/liustack/quickjs/intl
[ES2023]: https://tc39.es/ecma262/2023/

## Maintaining the fork

Branches: `main` carries the fork. `master` and the other upstream branches
mirror gitlab.com/cznic/quickjs and get no commits of their own.

Remotes, set up once in a fresh clone. The engine remotes must not fetch
tags, because libquickjs tags (v0.13.1, ...) would clash with the tags of
this repository:

```sh
git remote add upstream https://gitlab.com/cznic/quickjs.git
git remote add upstream-lib https://gitlab.com/cznic/libquickjs.git
git config remote.upstream-lib.tagOpt --no-tags
git fetch upstream
git fetch upstream-lib 'refs/tags/*:refs/tags/lib/*'   # engine tags as lib/v0.13.2, ...
```

### Taking a new engine release

`lib/` was added with `git subtree`, so an upstream libquickjs release merges
in with its history:

```sh
git subtree pull --prefix lib upstream-lib <libquickjs tag, e.g. v0.13.2>
```

Conflicts to expect:

- `lib/ccgo_*.go`: every upstream release regenerates these files, so the
  two lines added to `XJS_ToCStringLen2`, the calls into
  `lib/backtrace.go` (lazy error stacks), the `getTimezoneOffset` that
  answers 0 (local time is UTC) and the calls into `lib/uncatchable.go`
  (uncatchable exceptions) and `lib/outofmemory.go` (out of memory), the
  compiler's out of memory checks, the stack limit of a new runtime, the
  call paths without `alloca`, the functions without a TLS frame, the
  changes to the interpreter loop and the call into `lib/objlit.go`
  conflict. Take the upstream version of every generated file, then put
  the changes back:

  ```sh
  git checkout --theirs -- 'lib/ccgo_*.go' 'lib/internal/run-test262/ccgo_*.go'
  go run ./internal/fixlib
  ```

  `fixlib` also rewrites `modernc.org/libquickjs` imports in hand-written
  files under `lib/`. `go run ./internal/fixlib -check` lists files that still
  need changes. If `fixlib` reports that `XJS_ToCStringLen2` changed shape,
  read the new upstream code: QuickJS may have fixed the leak itself. If
  the fail label now frees `val`, drop `fixToCString` and the
  `JS_ToCStringLen2` hunk, or the added calls free it twice. If there is a
  new jump to the label, check that `val` is freed on that path. If it
  reports that `_getTimezoneOffset` changed shape, or that the engine calls
  libc's local time functions (`localtime`, `mktime`, `tzset`, ...)
  somewhere else, upstream reads the local zone some new way: make that
  answer UTC too and teach `fixTimezone` about it. If it
  reports that `_build_backtrace`, `_get_prop_string`,
  `JS_GetPropertyInternal`, `JS_GetOwnPropertyInternal`, the interpreter's
  `get_field`, `js_print_error`, `js_print_object` or `JS_ComputeMemoryUsage`
  changed shape, compare the new upstream `build_backtrace` with
  `lib/backtrace.go`, which formats the same text from the recorded frames
  and must change with it. If it reports that `XJS_Throw`,
  `_JS_CallInternal` or `_JS_IteratorClose` changed shape, put the
  uncatchable check of `lib/uncatchable.go` back where the C change has it.
  If it reports that `XJS_ThrowOutOfMemory`, `_JS_ThrowError2`, the regexp
  functions or the string table changed shape, compare the new upstream
  code with `lib/outofmemory.go`: `_js_throw_error2` replaces the end of
  `JS_ThrowError2` and must build the same error. That includes the end
  not matching one of the calls `fixThrowError2` expects (the property
  flags of `message`, the backtrace flags, a call made more than once).
  If it reports that the uses of the regexp out of memory error, of the
  `"out of memory"` message, of `lre_exec` or of `lre_compile` differ,
  read the new function: running out of memory there must throw with
  `XJS_ThrowOutOfMemory`, as in `_js_regexp_exec`. Then wire it in
  `fixOutOfMemory` and add it to `checkOutOfMemoryUses`.
  If it reports that `Xdbuf_claim`, `_push_scope`, `_pop_scope`,
  `_js_create_function`, `_resolve_variables`, `_resolve_labels`, the
  loops whose code the parser moves, `_js_parse_error_v`, `_next_token`,
  `_js_parse_class`, `_js_parse_destructuring_element`, the bytecode
  writer or `Xlre_compile` changed shape, read the new upstream code
  against the C change: the parser must stop once a code buffer failed and
  patch no code in place after that, and every pass that reads back a code
  buffer must stop when that buffer failed (`internal/fixlib/compile.go`
  lists each place).
  If it reports that `XJS_NewRuntime2` or `_JS_ThrowStackOverflow` changed
  shape, a new runtime must still start with a stack size of 0 (the port's
  `MaxStackSlots`), and running out of stack must still throw a
  `RangeError` (`internal/fixlib/stack.go`). The same holds when it reports
  that the stack check of `_next_token` or `_json_next_token` changed
  shape: the tokenizers must throw that `RangeError` too. If it reports
  that the uses of `"stack overflow"` differ from `stackOverflowUses`, a
  new place throws that message: make it call `_JS_ThrowStackOverflow`
  (teach `fixStack` to), or add it to the list if it is the regexp parser.
  If it reports a call of `XJS_GetException` that is not in
  `exceptionHandlers` or `exceptionReaders` (`internal/fixlib/uncatchable.go`),
  read the new caller: if it hands the exception to a script (a rejection,
  a handler call, a new throw), add it to `exceptionHandlers`, so that it
  gets `_js_get_exception`. If it reports a store of the runtime's
  exception (`current_exception`) in a function that is not in
  `exceptionWriters`, read it the same way: code that takes the exception
  out by hand must leave an uncatchable one pending, as `_JS_IteratorClose`
  does. If it reports that `XJS_Throw` or `_JS_IteratorClose` uses the
  exception before the uncatchable check, move the check up to before that
  use.
  `fixlib` also audits how the engine uses property slots (the `JSProperty`
  that holds a value), because a pending `stack` must be formatted before
  anything hands the value on. It parses each generated function and
  follows slot pointers from where they are made (an object's property
  array, the slot `find_own_property` returns) through variables (`=`,
  `:=` and `var`), stack cells, pointer arithmetic, call arguments and
  return values, whatever their names. A slot pointer that goes anywhere
  else (stored into a structure, in an assignment of several results,
  arithmetic on a stack cell other than a step to the next cell) is
  listed as a use there. For every function they reach it lists each use: reads and
  writes of the value or of another field, calls the pointer is passed to,
  stores, and which uses come before the function's first call into
  `lib/backtrace.go`. It also counts each function's descriptor lookups.
  `internal/fixlib/slotsites.go` holds the expected list with the reason
  each function is safe. When a release adds a function that uses slots,
  or changes how one does, `fixlib` stops and prints the difference: check
  whether the change can see a pending `stack`, wire it to
  `lib/backtrace.go` if so, and update the list
  (`go run ./internal/fixlib -slots` prints it). It also requires every
  property lookup in the interpreter loop (`JS_CallInternal`) to be a
  known guarded read or the `put_field` write.
  What the audit does not see: a slot pointer stored into a structure is
  listed as a use where it is stored, but not followed to where it is
  loaded again. Indirect calls are not followed. A function that no one
  passes a slot pointer to is not audited, so a new helper only shows up
  once something calls it with one. The allocator functions that resize
  or free the property array are listed as uses but not looked into. The
  order check only compares positions in the source with the first guard
  of the function. And slot addresses made some other way than from the
  property array, such as a raw offset into the object, are not
  recognized. So a change in these places needs a review by hand.
  If it reports that the `alloca` of `_JS_CallInternal`,
  `_js_call_c_function`, `_js_call_bound_function` or
  `_js_c_function_data_call` changed shape, or that there are no longer
  exactly four, read the new function (`internal/fixlib/alloca.go`
  describes the rewrite). The room must still come from the end of the
  frame, allocated again right before the stack check, so that the TLS
  stack slots stay what they were at every point: `calldepth_test.go` pins
  the depth each kind of recursion reaches and that the slots balance.
  An alloca larger than `allocaMax` (768 bytes) stays a malloc, so that no
  TLS block linux keeps grows past the 2 KB the frame of
  `JS_CallInternal` takes. If that frame changes size, set the constant
  again (`TestTLSStackResidentNearLimit` checks both, and the blocks on
  linux).
  If it reports that a function of `internal/fixlib/framesites.go` changed
  its TLS frame (size, number of cell uses), uses `bp` some other way, or
  can now reach a stack check, read the new function. Its frame may only
  hold values read back where they are written, and the function must not
  be on a call chain that checks the stack, or it would take a slot out of
  the count. Update the entry or remove the function from the list.
  `go run ./internal/fixlib -frames` prints every TLS frame of
  `lib/ccgo_darwin_arm64.go` and why it must stay: a line without a reason
  is a function that could join the list, after a review.
  `fixlib` writes every `*(**T)(__ccgo_up(x))` as
  `*(*T)(unsafe.Pointer(x))` (`fixUp`). The changes before it take both
  forms and the ones after it (`fixCells`, `fixPin`, `fixDefine`) only the
  new one, so a match that only knows the upstream form must learn the other
  (`reDeref` in `internal/fixlib/main.go` gives both). If it reports that
  `__ccgo_up` is used some other way or that its definition changed, read
  what ccgo now writes (`internal/fixlib/up.go`).
  The call made Go read the memory in `x` before the calls to the right of
  it in the same statement, and the new form does not. `upSites`
  (`internal/fixlib/upsites.go`) lists each statement where that order
  goes away, by its function, the addresses that read memory and the
  functions called after them. If it reports a statement that is not
  listed, or a count that differs, read the statement and the functions it
  names. If one of them can change what the address reads, as `f` can in
  `s->buf[i] = f(s)` when it grows `buf`, `fixUp` must leave that
  statement as ccgo wrote it, which it cannot do yet. Otherwise run
  `go run ./internal/fixlib -up` on the files as taken from upstream, put
  its output into `upsites.go` and say why each new statement is safe. On
  files `fixlib` has changed, `-up` fails: no `__ccgo_up` is left to find
  the statements by.
  If it reports that `_JS_CallInternal` has other cells than the two of
  `JS_DupValue` and `JS_FreeValue`, a use of one other than a store of a
  whole value or a load of its pointer, or that `JSValue` has another
  layout, read the new interpreter against `internal/fixlib/cells.go`: a
  cell may only become a Go variable if nothing else reads or writes its
  bytes. `TestCellsEveryFile` counts the stores and loads of each.
  Every other use of `bp` there must be a read or a write through
  `(*T)(unsafe.Pointer(bp + K))` that stays out of the cells, or a use
  listed in `cellsFrameUses`. If it reports an address in the frame that is
  not `bp + K` (such as `bp + 32 + 16` or `bp + uintptr(48)`), a read or
  write that reaches into a cell, a type whose size it does not know, a use
  of `bp` that is not listed (such as `libc.Xmemset(tls, bp, 0, 80)`) or a
  listed one that is gone, read that line. A new local in the interpreter
  moves the offsets and is the usual cause. If the use cannot touch the
  bytes of a cell, add it to `cellsFrameUses` with the reason, or fix the
  count, or add the type to `valueSizes`. If it can, the cells must stay in
  the frame. In a fixed file, any use of the bytes where the cells were is
  an error too: nothing writes them any more.
  If it reports that a value of `pinVars` (`internal/fixlib/pin.go`) is
  gone, find what the interpreter now calls it and update the list.
  If it reports that `JS_DefinePropertyValue`, `JS_DefineProperty`,
  `JS_CreateProperty`, `add_property`, `find_own_property` or
  `find_own_property1` changed (a fingerprint other than the one in
  `internal/fixlib/definesites.go`),
  read the new code against `objlitDefine` in `lib/objlit.go`. For a new
  data property of an extensible object that is not exotic, not a fast
  array and not the global object, the old path must still end in
  `add_property` with the `C_W_E` bits of the flags and then store the
  value, and every other case must still fail one of the checks of
  `objlitDefine`. Change `objlitDefine` if it no longer matches, then put
  the new fingerprints into `definesites.go`: `go run ./internal/fixlib
  -define` prints them, for the files with all of `fixlib`'s other changes
  made. Any edit to those functions changes the fingerprint, even one that
  does not matter here, and so does a change to another `fixlib` step that
  touches them (`fixUp` does). If it reports that `JS_DefinePropertyValue`
  has a new signature, update the call (`defineCall`) and
  `reDefineValueHead` in `internal/fixlib/define.go`. `objlitDefine` reads
  `extensible`, `is_exotic` and `fast_array` through constants that
  `fixlib` takes from the bit fields `JS_NewObjectFromShape` sets (the
  second, fourth and fifth, all in one byte), and checks against the
  checks of those bits in `JS_CreateProperty` and `JS_DefineProperty`. If
  it reports that `JS_NewObjectFromShape` changed shape, that one of those
  checks is gone, or that a check disagrees with `JS_NewObjectFromShape`,
  then the layout of `JSObject` moved or `JS_NewObjectFromShape` sets the
  fields in another order: find where the three bits are now and update
  `objectBits` and its patterns in `internal/fixlib/define.go`.
- `lib/internal/patch/quickjs.diff`: holds the same changes in C, so that a
  regeneration keeps them. Keep the `JS_ToCStringLen2` hunk unless upstream
  QuickJS has fixed the bug, the `getTimezoneOffset` hunk, and the lazy stack hunks (`JSBacktrace`, the
  built-in `JS_CLASS_BACKTRACE` class, `js_backtrace_build` and the
  `js_backtrace_*` calls in `JS_GetPropertyInternal`,
  `JS_GetOwnPropertyInternal`, `JS_DefineProperty`, the interpreter's
  `GET_FIELD_INLINE`, `get_prop_string`, `js_print_error`,
  `js_print_object`, `JS_ComputeMemoryUsage`, the callers that only need
  the attributes of a property (`js_get_own_property_flags`)), and the
  uncatchable exception hunks (`js_uncatchable_pending`, `js_get_exception`
  and its callers, `JS_IsUncatchableException`, and the checks in
  `JS_Throw`, `JS_CallInternal` and `JS_IteratorClose`), and the out of
  memory hunks (`JS_ThrowOutOfMemory`, `JS_ThrowError2`, the regexp
  callers of `JS_ThrowOutOfMemory`), and the compiler hunks (`dbuf_claim`
  in `cutils.c`, `push_scope`, `pop_scope`, `js_create_function`,
  `resolve_variables`, `resolve_labels`, the two code moves of the parser,
  `js_parse_error_v`, `next_token`, the two patches in `js_parse_class`,
  the test removal in `js_parse_destructuring_element`,
  `JS_WriteObjectAtoms`, `JS_WriteObject2` and the error label of
  `lre_compile` in `libregexp.c`), and the stack hunks (the stack size
  `JS_NewRuntime2` sets, and `JS_ThrowStackOverflow`). After a
  regeneration that includes them, `lib/backtrace.go`, `lib/uncatchable.go`, `lib/outofmemory.go` and
  their wiring in `fixlib` are no longer needed. The calls without
  `alloca`, the functions without a TLS frame, the changes to the
  interpreter loop and the call into `lib/objlit.go` have no C form: they
  are changes to what ccgo generates, so `fixlib` applies them after every
  regeneration.
- `lib/go.mod`, `lib/go.sum`: they do not exist here. When upstream changed
  them, delete them again (`git rm lib/go.mod lib/go.sum`) and bring any newer
  requirement into the root `go.mod`.
- `lib/internal/run-test262/go.mod` is ours. Upstream has none there.
- `lib/internal/run-test262/ccgo_*.go`: ccgo links a whole copy of the
  engine into the test262 runner, so as generated the runner tests that copy
  and not `lib/`. `fixlib` turns each engine API function of the runner into
  a call of the same function in `lib/` (`lib/test262_runner.go` exports the
  one ccgo leaves unexported). Take the upstream files and run `fixlib`, as
  for `lib/ccgo_*.go`.

This fork does not regenerate `lib/ccgo_*.go` itself. `lib/generator.go` still
assumes it is the root of its own module.

### Taking a new release of the Go API

```sh
git merge <upstream tag>
```

Upstream commits import `modernc.org/libquickjs` and `modernc.org/quickjs`.
After the merge, drop `modernc.org/libquickjs` from `go.mod`, run
`grep -rn 'modernc.org/quickjs\|modernc.org/libquickjs' --include='*.go' .`
and switch what it finds to `github.com/liustack/quickjs`. The only expected
leftover is the Symbol key `"modernc.org/quickjs/intl"` in `intl/stub.js`,
which is a runtime marker, not an import path.

`intl/` has been rewritten here, so an upstream change to it conflicts.
Keep this fork's version and bring over only what the upstream change fixes,
checked with the golden test below.

### Updating the intl locale data

`intl/cldr.bin` is generated from cldr-json and `intl/tzdst_table.go` from
tzdb, and both are checked in, so neither a build nor `go test` needs the
network or Node. To generate them again:

```sh
go run ./intl/internal/gen -fetch    # downloads what is missing into a cache directory
go test ./intl                       # includes the golden test against Chromium
```

The tzdb step needs `make`, `awk` and `zic` (`-zic` names another zic). It
unpacks the release in `tzdbVersion` in `intl/internal/gen/tzdb.go`,
after checking its SHA-256 against `tzdbSHA256`, makes `main.zi` and
`rearguard.zi` with the release's Makefile and compiles both. The table
lists the zones and links whose daylight saving flag differs between the
two forms before 2038, with their periods in the rearguard form, which is
what ICU builds from. The generator also copies a few zones in both forms
to `intl/testdata/tzdata` for the tests. To move to a new release, take
the one in the ICU data of the Chromium the golden test records with: the
`TZVersion` of `zoneinfo64.res` in its `icudtl.dat` (see below for where to
get that file). Set `tzdbVersion` and `tzdbSHA256` (the SHA-256 of
`tzdata<version>.tar.gz` from https://data.iana.org/time-zones/releases/,
whose signature is the `.asc` next to it) and run the two commands.

To move to a new CLDR release, set `cldrTag` in `intl/internal/gen/main.go`
to the cldr-json tag and run the two commands. The golden test then shows
every expression whose output changed. Where the new CLDR changed a value
Chromium does not have yet, add the value Chromium has to `browserOverrides`
in `intl/internal/gen/overrides.go`, with where it comes from. The generator
stops on an override CLDR has caught up with, so the list only ever holds
live differences.

To compare with a newer Chromium, record its answers again. This needs Node
and a Chromium that Playwright drives. Set it up once:

```sh
npm install -g playwright@1.64.0                # Chromium 156.0.8078.4, as recorded here
npx playwright@1.64.0 install chromium
export PLAYWRIGHT="$(npm root -g)/playwright"   # what record.cjs loads
```

Each Playwright release drives one Chromium build, so a newer Chromium needs a
newer Playwright: its `browsers.json` names the build.

Then, from the repository root:

```sh
CLDR="$HOME/cldr-json"                                         # any directory for the downloads
go run ./intl/internal/gen -fetch -cldr "$CLDR"                # the list of CLDR locales
node intl/testdata/chromium/record.cjs availability "$CLDR"    # the locales Chromium has
go run ./intl/internal/gen -icudtl "$(node intl/testdata/chromium/record.cjs icudtl)"
go run ./intl/internal/gen -fetch -cldr "$CLDR"                # data for those locales
go test ./intl -run TestGolden -args -golden.corpus            # writes corpus.json
node intl/testdata/chromium/record.cjs golden                  # writes golden.json.gz
go test ./intl
rm -rf "$CLDR"
```

`record.cjs icudtl` prints where the browser keeps `icudtl.dat`, its ICU
data file. The same file, byte for byte, is `common/icudtl.dat` in Chromium's
copy of ICU, at the commit Chromium's `DEPS` pins for `src/third_party/icu`
(5aa52620 for 156.0.8078.4), so summarizing it needs no browser:

```sh
curl -s 'https://chromium.googlesource.com/chromium/deps/icu/+/5aa526207171ecadf31597f0980a7b7ad8872f33/common/icudtl.dat?format=TEXT' |
	base64 -d > icudtl.dat
go run ./intl/internal/gen -icudtl icudtl.dat
```

The generator writes a summary of it to
`intl/testdata/chromium/icu.json.gz` and makes every locale take each
resource from where Chromium's ICU finds it, so a language whose data
Chromium leaves out formats with root's or English data as it does in
Chromium. To record another browser, set `CHROMIUM` to its executable for
every `record.cjs` command.

Update the Chromium and ICU versions in the package documentation and here,
and look at each new difference: fix the code when Chromium is right, or add
the cause to `goldenDifferences` when the difference is in the data.

### Checks

```sh
go test -short ./...                                     # all packages
go test -vet=off -run 'TestToCString' ./lib              # the leak reproduction
go test -run 'TestErrorStack' .                          # stack text and memory of lazy error stacks
go test -vet=off -run 'TestBacktrace' ./lib              # shapes, retries and writes of pending stacks
go run ./internal/fixlib -check                          # fork changes present in lib/
go run ./intl/internal/gen -fetch -check                 # intl data is what the generator makes
go test ./internal/fixlib                                # fixlib notices new reads of property values and new shapes of what it changes
go test -run 'CallDepth|TLSStackBalance|CallAllocs' .    # depth limits, TLS balance, no Go allocation per call
go test -run '^$' -bench Call .                          # the cost of each kind of call
for t in linux/amd64 linux/arm64 darwin/arm64 windows/amd64; do
	GOOS=${t%/*} GOARCH=${t#*/} go build ./...
done
```

`lib/internal/run-test262` is a module of its own, so its 69 MiB of
transpiled test runner stay out of the module zip. `Test262` in `lib/` builds
it with `go build -C`. The runner runs the engine in `lib/` (see `fixlib`),
so it checks the changes made there.
