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
  on linux. 22 more functions were added later, from the count of
  `tls.Alloc` per function in renders of tifa's example shop and test
  apps, such as `js_same_value_zero` (`Map` and `Set` keys, `includes`),
  `free_token` (the parser) and the finalizers of functions and
  iterators: the numbers above are from before, and what the later
  changes give together is under the frame of `JS_CallInternal` taken
  once on linux, below.

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
  up to 10 MB in the worst case. Elsewhere the TLS stack keeps one block
  of about 1 MiB (see below) and frees the pages past it as before. These are
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

- Engine functions with a TLS frame no longer free it in a `defer`. ccgo
  frees the frame of a function (`bp := tls.Alloc(N)`) with
  `defer tls.Free(N)`, and Go does not inline a function with a defer, with
  or without PGO, so hundreds of small engine functions stayed calls.
  `fixlib` now frees the frame before each return, after the value is
  computed, and at the end of a function that can run off it, in all 13
  files: 969 functions on darwin/arm64, among them the three call functions
  that `fixAlloca` changed (see below). Go then inlines many of them, and
  a server render of a page of tifa's example shop takes about 10% less
  time on linux/amd64 (AMD EPYC), with or without PGO, with the same HTML.
  A PGO profile taken before this change no longer matches many of the
  call sites: take it again. What changes is a Go panic that goes
  up through the engine: a defer freed the frames on the way, and now they
  stay on the TLS stack. The VM puts the stack back where it was when the
  call into javascript started, and when javascript called a Go function,
  so `Close` frees all of it, and libc's memgrind check of the TLS stack
  holds. A panic of a Go function that javascript called is recovered there
  as before: no engine frame is between them. A panic in the engine itself
  (a bug, such as a nil pointer, or libc running out of memory) goes on to
  the Go code that called javascript, also when the javascript was
  evaluated by a Go function. That Go function used to recover it and
  throw an error, which walked stack frames the panic had dropped and could
  crash the process (it did, with a fatal fault, in
  `TestEnginePanicTLS/nested`). The engine's state after a panic in it is
  not known, so the VM is good only for `Close`, as in tifa: every later
  call into javascript panics. A Go function that recovers such a panic
  itself does not save the VM: the VM panics where the Go function
  returns, as it does when a call returns with engine frames left on the
  TLS stack, which means a frame was not freed (outside linux such a frame
  can break the TLS stack before the call returns). The values held by the
  javascript frames the panic went through are not freed, with or without
  the defer, and neither is the room that `alloca` mallocs for a call with
  more than 768 bytes of it (see `fixAlloca`), which the defer freed. The
  Go code written by hand that calls the engine (`lib/backtrace.go`,
  `quickjs.go`) frees its TLS frames before it returns as well: a defer
  would free the frames a panic left in place of its own. A Go function
  that calls `runtime.Goexit`, as `t.Fatal` does, also leaves the VM good
  only for `Close`, where v0.26.14 kept it usable. The generated Go is
  changed by `internal/fixlib/nodefer.go`, with no C hunk, and the other
  changes of `fixlib` see the defers as ccgo writes them. Tests:
  `enginepanic_test.go`, `memgrind_test.go`,
  `internal/fixlib/nodefer_test.go`.
- A regexp that is not sticky no longer tries every position of the string
  in the backtracking machine. Its bytecode starts with a loop that tries
  the match at one position, steps over one character and tries again.
  `lre_exec` now first calls `lreSkip` (`lib/regexskip.go`): when the
  regexp starts with one character or one class of characters, such as
  `/["&<>]/` or `/\d+/`, it finds the first position that holds one in a
  plain Go loop and starts there, and when there is none it answers no
  match without running the bytecode. A regexp that starts with `^`
  outside multiline mode is run at position 0 only. Unicode regexps (`u`,
  `v`), regexps that ignore case or are multiline (`i`, `m`), and every
  other first op took the old path, until the change for alternations
  below. `lreSkip` checks for an interrupt after every 65,536 characters it looks at, as the backtracking machine
  does every 10,000 steps, so the eval timeout and `Interrupt` stop a
  regexp that runs again and again on a long string as before. A server
  render of a page of tifa's example shop takes about 3% less time on
  linux/amd64 (AMD EPYC), with the same HTML. `fixlib` puts the call into
  all 13 files, and checks the text of
  `lre_compile`, `lre_exec` and `lre_exec_backtrack`, whose bytecode and
  ops `lreSkip` reads, against fingerprints. This changes only the
  generated Go, with no C hunk (`internal/fixlib/regexskip.go`). Tests:
  `lib/regexskip_test.go`, whose `testdata/regex.txt` holds what 61
  regexps give on 47 strings through every API that runs one, from the
  engine before the change, `internal/fixlib/regexskip_test.go`, and
  `TestRegexScanStops` in `timeout_test.go`.
- `===` and the names of keys take no TLS frame on their usual paths.
  `js_strict_eq2` (`===`, `Object.is`, `includes`, `Map` keys) and
  `__JS_AtomToValue` (the name of a key as a value, as `Object.keys` and
  `for in` need it) allocated a TLS frame on every call, though only the
  comparison of two BigInts and the name of an integer key pass the
  address of a part of it on. `fixlib` moves those two paths into
  functions of their own, `_js_strict_eq2_cold` and
  `___JS_AtomToValue_cold`, which take the same frame, and the rest of the
  frame becomes a Go array, as for the functions of `frameSites` (see
  below). The paths are matched as ccgo writes them, every other use of
  the frame must be a load or a store of a cell, and the two functions can
  reach a stack check only through their cold functions, so the TLS stack
  a stack check sees is the same. A server render of a page of tifa's
  example shop takes 0.7% (with PGO) to 2% (without) less time on
  linux/amd64 (AMD EPYC), with the same HTML. This changes only the
  generated Go, with no C hunk (`internal/fixlib/cold.go`). Tests:
  `lib/coldpath_test.go`, whose `testdata/coldpath.txt` is what the engine
  gave before the change (Node gives the same), and
  `internal/fixlib/cold_test.go`.
- Reading a property, `==`, turning a value into a string or an object,
  and freeing an array take no TLS frame on their usual paths.
  `JS_GetPropertyInternal`, `JS_GetOwnPropertyInternal`,
  `JS_AtomIsArrayIndex`, `JS_NewAtomStr`, `js_eq_slow`,
  `JS_ToStringInternal`, `JS_ToObject` and `js_array_finalizer` allocated
  a TLS frame on every call, and so did 12 functions added later from the
  count of `tls.Alloc` per function in renders of tifa's test apps: the
  conversions to numbers (`JS_ToInt32Free`, `JS_ToInt32SatFree`,
  `JS_ToInt64SatFree`, `JS_ToFloat64Free`), `JS_GetPrototype`,
  `JS_ConcatString2`, `js_sub_string`, `add_fast_array_element`,
  `js_object_has_name`, `js_get_regexp`, `js_regexp_get_lastIndex` and
  `js_thisNumberValue`. Only a few of their statements need a frame: those
  that pass the address of a part of the frame to a callee, and those that
  can reach a stack check. `fixlib` makes the frame a Go array,
  as for the functions of `frameSites`, and gives each of those statements
  a TLS frame of the same size for itself: right before the statement it
  allocates the frame and copies the array into it, and right after it
  copies the frame back and frees it. The statement reads and writes the
  same bytes as before, and a stack check it reaches sees the same TLS
  stack: one more slot, with the frame at the same place. The out
  parameter of `find_own_property` and `is_num_string` gets a Go pointer
  into the array instead, through copies of the two functions that take
  one (`_find_own_property_lazy`, `_is_num_string_lazy`), which only load
  and store through it and reach no stack check. Why this is safe: a
  function is changed only when every other use of its frame is a load or
  a store of a cell, every address a statement passes on goes to a callee
  that was read and does not keep it after it returns nor return it
  (`addrCallees` in `internal/fixlib/lazy.go`), and every call that can
  reach a stack check is in such a statement. The text of those callees,
  of the functions the `va_list` goes on to, of the
  `get_own_property` methods of the engine's classes and of
  `JS_GetOwnPropertyInternal`, which the method of a proxy passes the
  descriptor to, is pinned by
  fingerprints (`lazyAddrSites`), so an upstream change to one stops
  `fixlib` until it is read again, and the frame size, cell uses and
  statements of each function are pinned in `lazySites`. A class
  registered through `JS_NewClass` with a `get_own_property` method of its
  own must not keep the descriptor it is given either, as in C (neither
  `quickjs.go` nor `lib/backtrace.go` registers one). Other changes of
  `fixlib` match these functions as ccgo writes them, so `fixlib` undoes
  this change first (`restoreLazy`) and makes it again last, and checks
  that undoing it gives back the text it was given, byte for byte. On
  32-bit targets nine of these functions, such as `JS_ToObject` and
  `js_array_finalizer`, have no TLS frame and stay as they are
  (`lazyFrameless`). Another function of `lazySites`
  without a frame there stops `fixlib`. With the first eight functions, a
  server render of a page of tifa's example shop
  takes 1.6% (with PGO) and 1.1% (without) less time on linux/amd64 (AMD
  EPYC), and 2.6% less on darwin/arm64 (Apple M4, with PGO), with the same
  HTML. What the 12 added later give, together with the other later
  changes, is under the frame of `JS_CallInternal` taken once on linux,
  below. This changes only the generated Go, with no C hunk (`internal/fixlib/lazy.go`). Tests:
  `internal/fixlib/lazy_test.go` and `internal/fixlib/nodefer_test.go`,
  with test262 and the tests of the root package for what the engine
  gives.
- The interpreter no longer calls `set_value`: its 46 calls in
  `JS_CallInternal` are written out in place. Go inlines into a function
  as big as the interpreter only callees that cost 20 or less, with or
  without PGO, so each was a real call, made by every `put_loc`,
  `set_loc`, `put_arg` and `put_var_ref` of a script. `fixlib` turns a
  call into a block that gives each argument to a variable of the
  parameter's type, in order, then runs the callee's own body with its
  parameters renamed to those variables and its labels renamed to labels
  of their own. An argument that is a variable of the interpreter, such
  as `ctx10`, is read where the parameter is, with no variable of its
  own, when nothing can set it while the copy runs and the callee does
  not set the parameter: 46 copies of `ctx10` made the interpreter's
  frame on amd64 288 bytes bigger and undid `fixPin`, which keeps `ctx10`
  in its stack slot. The Go array that `fixFrames` gives `set_value`
  holds one JSValue, and becomes a Go JSValue in each copy, read as
  `fixCells` reads the interpreter's own cells. Why this is safe: the
  arguments are evaluated once and in order, as for the call, before the
  callee's locals are declared, so a local cannot change what an argument
  reads, and `fixlib` stops when the callee has a result, returns early,
  takes a TLS frame, uses its Go array other than as one JSValue or uses
  a name that is a local of the interpreter, when a call is not a
  statement of its own, or when the number of calls changes. `set_value`
  takes no TLS frame, so the TLS stack a stack check sees is the same.
  Its text is pinned by fingerprints (`inlineSites`), so an upstream
  change stops `fixlib` until it is read again. Other changes of `fixlib`
  match the interpreter as ccgo writes it, so `fixlib` undoes this change
  first (`restoreInline`) and makes it again last, and checks that
  undoing it gives back the text it was given, byte for byte. A server
  render of a page of tifa's example shop on linux/amd64 (AMD EPYC) takes
  1.4% less time with this change, with and without PGO, with the same
  HTML. With the TLS frames taken by statement as well, it takes 2.8% less
  time than with v0.26.15 with PGO (each with an engine profile taken on
  its own code) and 2.9% less without. `JS_FreeAtom` and
  `is_strict_mode`, written out too, gained nothing that could be told
  from noise, so they stay calls. This changes only the generated Go,
  with no C hunk (`internal/fixlib/inline.go`).
  Tests: `internal/fixlib/inline_test.go`, with test262 and the tests of
  the root package for what the engine gives.
- A `for in` loop no longer lists the property names of each prototype
  to find out that it has no enumerable property. When a loop runs out of
  the object's own properties, the engine checks the objects of its
  prototype chain for enumerable string keys, to know whether it must walk
  the chain. It did that with `JS_GetOwnPropertyNamesInternal`, which
  mallocs a list, takes a reference to each name and frees it all again,
  in every loop and for every prototype, mostly for `Object.prototype`,
  which has dozens of names and none of them enumerable.
  `js_for_in_prepare_prototype_chain_enum` now first calls `forInNoEnum`
  (`lib/forin.go`), which reads the shape of a prototype that is not
  exotic: when no property of the shape has a name and the enumerable flag,
  it gives the same empty list without allocating, and the loop goes on as
  before. Why this is safe: for an object that is not exotic,
  `JS_GetOwnPropertyNamesInternal` with these flags counts only shape
  properties with a name, the enumerable flag and a string key, calls no
  method of the class and throws for nothing it does not count, so with no
  such property its list is empty. `forInNoEnum` answers no for every
  exotic object (arrays, `String` objects, proxies, ...) and for every
  object with an enumerable property, symbols included, and the engine
  lists the names as before, so the traps of a proxy in the chain run as
  they did. The only difference is out of memory: the empty list needs no
  memory, where the engine mallocs one entry, so with no memory left the
  loop goes on where it used to fail. `fixlib` puts the call into all 13
  files and takes the two bit fields `forInNoEnum` reads, the flags of a
  shape property and the `is_exotic` bit of an object, from each file's
  own `JS_GetOwnPropertyNamesInternal`, so 32-bit and big-endian targets
  get their own layout. It pins the text of
  `js_for_in_prepare_prototype_chain_enum`, `JS_GetOwnPropertyNamesInternal`,
  `get_shape_prop` and `JS_FreePropertyEnum` by fingerprints
  (`forInSites`). This changes only the generated Go, with no C hunk
  (`internal/fixlib/forin.go`). Tests: `lib/forin_test.go`, whose
  `testdata/forin.txt` is what the engine gave before the change, and
  `internal/fixlib/forin_test.go`. On linux/amd64 without PGO, against
  v0.26.16 over 10 rounds, the spiral page of tifa's bench app, which runs
  many for in loops, renders in 0.983 of the time (a wide range, 0.888 to
  1.041) and the table page in 0.971 (0.871 to 1.035). tifa's example shop
  barely changes (0.996, 0.987 to 1.001), nor do the MUI pages. The HTML
  and the memory of a VM stay the same.
- `&`, `|`, `^`, `<<`, `>>` and `>>>` of two numbers no longer take the
  slow path when one of them is a float. The interpreter did the operator
  itself only for two ints and called `js_binary_logic_slow` or
  `js_shr_slow` for everything else, which convert both operands with the
  checks for BigInts and objects and then with the generic `ToInt32`. A
  product past 2^31 is a float, so the string hash of emotion, the CSS-in-JS
  library of MUI, took that path for almost every operator. The two slow
  functions now first call `bitwiseNumbers` (`lib/bitwise.go`), before
  they take their TLS frame. It does the operator for any two ints or
  floats, converting a float as `JS_ToInt32Free` does (the same bits,
  without `fmod`), and stores an int, or for `>>>` a float above
  2^31 - 1, as `JS_NewUint32` does. Any other pair goes on through the rest
  of the slow path as before. Why this is safe: numbers hold no reference,
  so nothing is freed, and the slow paths do nothing else for two numbers,
  since converting a number to a numeric is a no-op and cannot throw or
  call a script. The interpreter is left as it is: calling
  `bitwiseNumbers` from its six handlers instead made the shop 0.8% slower.
  Only the 11 files
  of 64-bit targets change, because `bitwiseNumbers` reads their struct
  `JSValue`: the 32-bit targets keep the slow path. `fixlib` pins the text
  of `js_binary_logic_slow`, `js_shr_slow`, `JS_ToNumberHintFree` and
  `JS_ToInt32Free` by fingerprints (`bitwiseSites`), and stops when the
  interpreter no longer has 5 calls of the first and 1 of the second, or
  when either no longer takes its TLS frame first. This
  changes only the generated Go, with no C hunk
  (`internal/fixlib/bitwise.go`). Tests: `lib/bitwise_test.go`, whose
  `testdata/bitwise.txt` is what the engine gave before the change (Node
  gives the same) and which checks `bitwiseNumbers` on about 41,000 pairs
  of numbers against `ToInt32` of the specification (which the engine's
  `JS_ToInt32Free` must also give) and the operator done in Go, and
  `internal/fixlib/bitwise_test.go`.
  On linux/amd64 without PGO, against v0.26.16 over 10 rounds, the pages
  of tifa's MUI app, which run about 130,000 to 300,000 of these
  operators on floats per render, take 0.952 to 0.960 of the time (0.935
  to 0.969). The
  shop runs the slow paths 3 to 24 times per render and the bench app not
  at all, so they have nothing to gain: the shop measured 1.008 (0.999 to
  1.042), which is code layout, not work. The HTML and the memory of a VM
  stay the same.
- `===`, `!==`, `==` and `!=` no longer call the slow path for the usual
  pairs. The interpreter compared two ints itself and called
  `js_strict_eq_slow` or `js_eq_slow` for everything else, which for `===`
  went on to `js_strict_eq2`. React compares with `null` and `undefined`
  (minified as `null != x` and `void 0 === x`, which the compiler does not
  turn into `is_null` or `is_undefined`), compares types, symbols and
  objects by identity, and switches over strings. The interpreter now
  answers these itself: objects and symbols by pointer, `null`, `undefined`
  and booleans by tag and value, strings when they are the same string,
  have different lengths or are both string atoms (the atom table holds
  each text once), for `===` and `!==` values of different tags when one
  is `null`, `undefined`, a boolean, an object or a symbol, and for `==`
  and `!=` `null` or `undefined` beside anything (equal only to `null`,
  `undefined` or an `IsHTMLDDA` object). Other pairs of different tags
  under `==`, such as `true == 1` or `{} == 'x'`, convert their operands
  and still take the slow path, as do numbers, BigInts, ropes and strings
  that need their characters compared. Why this is safe:
  each answer is the one the slow path gives for that pair, none of them
  can throw or call a script, and both operands lose their reference as
  `JS_FreeValue` does, freed through `eqFree` (`lib/eq.go`) when it was the
  last. Strings that are both atoms are told apart only when both are
  string atoms: the description of a symbol is an atom of another type and
  can hold the text of a string atom. The text is written into the four
  handlers, with no call on the usual path, since Go does not inline into a
  function as big as the interpreter. It passes `eqFree` the stack pointer
  rather than the values, so the frame of the interpreter changes little:
  8 bytes more on amd64 and 16 fewer on arm64, from v0.26.16. Only the 11
  files of 64-bit
  targets change, because the text reads their struct `JSValue`. The bit
  fields it reads (the length and atom type of a string, the `IsHTMLDDA`
  bit) come from each file's `js_string_eq`, `__JS_FreeValueRT` and
  `JS_IsHTMLDDA`, and the layout of a value (how far below its pointer
  the reference count sits, the size of a value on the stack and how a
  boolean is stored) from its `JS_FreeValue` and `js_strict_eq_slow`.
  `fixlib` pins those and `js_strict_eq_slow`,
  `js_strict_eq2`, `js_eq_slow` and `__JS_FreeValue` by fingerprints
  (`eqSites`), and stops when the interpreter no longer has one call of
  each slow path per operator. This changes only the generated Go, with no
  C hunk (`internal/fixlib/eq.go`). Tests: `lib/eq_test.go`, whose
  `testdata/eq.txt` is what the engine gave before the change (Node gives
  the same) and which checks that compared values are freed once, and
  `internal/fixlib/eq_test.go`. On linux/amd64 without PGO, against
  v0.26.16 over 10 rounds, a render of tifa's example shop takes 0.982 of
  the time (0.974 to 0.989), the spiral and table pages of its bench app
  0.940 and 0.942 (with wide ranges, 0.859 to 0.969 and 0.871 to 1.030),
  and its MUI pages 0.982 to 0.985. The HTML and the memory of a VM stay
  the same.
- On linux, a call of a javascript function takes one TLS frame instead
  of two. `JS_CallInternal` took a frame of 1280 bytes at its entry, and
  for a function whose arguments, locals and stack fit in 768 bytes (see
  `fixAlloca` above) freed it and took 1280 bytes plus that room again
  right before the stack check. `fixlib` makes the entry take 1280 + 768
  bytes, and turns the block that freed and took the frame again into a
  comment. Why this is safe: the rest of the function is the path an
  alloca larger than 768 bytes took before, which skips that block too.
  The block only copied a parameter into the new frame and set a pointer
  into it, and `fixlib` stops unless the entry made each of those, once,
  in the same words. Nothing between the entry and the block writes them
  again, except on paths that return or jump past the block, and
  `fixlib` pins that text by fingerprints (`oneFrameSites`), so a new
  write there stops it. The room is still at `bp + 1280`, now inside the
  frame the entry took, and each call holds one slot of the TLS stack at
  every point, as before, so a stack check sees the same count and a
  script recurses exactly as deep as it used to. Linux's libc keeps one
  block per slot, rounded up to a power of two, and 1280 + 768 bytes fill
  the 2 KB block the frame took already, so no block grows. On 32-bit
  linux (a frame of 160 bytes) a block would grow, and elsewhere the TLS
  stack is one run of bytes that a larger frame uses up faster, so those
  files are not changed. Other changes of `fixlib` match
  `JS_CallInternal` as `fixAlloca` writes it, so `fixlib` undoes this
  change first (`restoreOneFrame`) and makes it again right before the
  frames are freed without a defer, and checks that undoing it gives back
  the text it was given, byte for byte. On its own this takes about 9%
  of the TLS frames out of a render of tifa's example shop on linux.
  Together with the 22 functions added to `frameSites` and the 12 added
  to `lazySites` above, it takes out about a fifth: 20.6% for the shop,
  and 20.1% and 25.4% for tifa's test apps bench and mui. A render then
  takes 1.4% (shop), 2.2% (bench) and 3.1% (mui) less time than with
  v0.26.16 on linux/amd64 (AMD EPYC, without PGO), with the same HTML:
  the median of 6 rounds, each comparing the mean of three builds of the
  new code, whose code sits at slightly different addresses, with one
  build of v0.26.16. That placement alone moves the time: moving
  `JS_CallInternal` by 160 or 224 bytes changes the time of a render by
  up to 2.4%, so these numbers are good to about that much, and one build
  is not enough to measure changes this small. This changes only the generated
  Go, with no C hunk (`internal/fixlib/oneframe.go`). Tests:
  `internal/fixlib/oneframe_test.go`, with `calldepth_test.go`, test262
  and the tests of the root package for what the engine gives.
- A regexp of alternatives no longer tries each alternative that cannot
  match at a position. An alternation such as `(red|blue|...)` compiles to
  a chain of split ops, each pushing a state for the next alternative
  before the machine tries one, so at every position of the string the
  machine pushed and popped a state for each alternative, which for the
  148 color names react-spring tests its values against (`new
  RegExp('(' + names.join('|') + ')(?!\\w)', 'g')`) was 147 states per
  position. Two changes, both in `lib/regexskip.go`:
  - The split ops of `lre_exec_backtrack` ask `lreSplit` first whether the
    way they push can start at the position: when it begins with a
    character or a class of characters, after nothing but saves of
    captures, and the character at the position does not match it, the
    way would fail as soon as it is popped, and nothing is pushed. When
    the way the machine takes is another split op, as in an alternation,
    `lreSplit` goes on with that one in the same call, for up to 4096
    split ops, after which it gives back the one it is at to be pushed as
    usual. It reads the
    character as the machine does and tests it as the op does, with the
    same `lre_canonicalize` for a regexp that ignores case, so this holds
    for every regexp and flag, lookbehinds and sticky regexps included.
  - `lreSkip` knows more first ops. `lreFirstSet` follows the bytecode
    from the start of the regexp through both ways of every split, over
    saves and assertions (`^`, `$`, `\b`, lookaheads and lookbehinds, to
    the op after them), up to the first op on each path that reads a
    character, and collects the characters a match can start with: a
    set of the 256 Latin-1 units and one bit for all others. It gives up,
    and the regexp takes the old path, when a path can reach the end
    without reading a character (the regexp can match the empty string),
    at a jump back, at an op it does not know (`.`, `\s`, a back reference,
    a class that ignores case, ...), and after 1024 ops. A character
    that ignores case counts as itself and its lower case when it is
    ASCII: outside unicode mode `lre_canonicalize` gives no other unit an
    ASCII canonical form (`TestCanonicalizeASCII` checks every unit).
    Regexps that ignore case or are multiline now take this path too.
    `lre_exec` then calls `lreRun` in place of the machine: it tries the
    match at the first position that can start one, without the loop over
    positions at the start of the bytecode, and when that fails, finds the
    next such position in Go and tries again, for the regexps of `lreSkip`
    that start with one character or class too. That is what the loop
    does: it tries each position in turn, from the captures as they were
    before the loop, all cleared, which `lreRun` clears again before each
    try. It polls the interrupt counter of the machine after each failed
    try and checks for an interrupt every 65,536 characters it looks at.
  Why this is safe: a state that is not pushed is one that would have
  been popped only to fail at its first character, after saves the
  machine undoes when it fails, and a position that is skipped is one
  where no path of the regexp can read its character. Results, captures,
  `lastIndex` and the errors a regexp throws are the same. The machine
  counts a step toward its interrupt check, and checks, at each `goto`,
  each loop and each failed pop, and a greedy `x+` jumps back by a split
  op, so over a long string it counted only through the pops of the
  states it pushed. A split op that pushes nothing therefore counts a step
  and checks itself, and one long match still stops when the eval timeout
  fires or `Interrupt` is called. What differs is how many steps are
  counted: one for each call of `lreSplit` that pushes nothing, however
  many splits it stepped over, where the machine counted one for each
  state it popped. So an interrupt stops a match after a different number
  of ops. And states not pushed take no room on the backtracking stack, so
  a regexp near the memory limit can go on where it ran out of memory.
  `BenchmarkRegexColors` times a `replace` with the react-spring regexp of
  color names: on linux/amd64 (AMD EPYC, one core) it takes 5.9 µs where
  it took 44.5 on a value with no color name, and 8.5 µs where it took
  20.2 on one with a name (the medians of 10 runs). Without PGO, with the
  same HTML and the same memory of a VM, a render of tifa's example shop
  takes about 1% to 2.5% less time, and the MUI pages of tifa's test apps
  about 3% to 5% less. The medians of 10 rounds against `a84c8ed`, before
  the split ops counted their steps toward the interrupt check, were 0.989
  for the shop (0.981 to 1.015 over the rounds) and 0.946 to 0.954 for the
  MUI pages, and two runs against `80261af` gave 0.983 and 0.987 for the
  shop and 0.947 to 0.965 for the MUI pages. With the steps counted, two
  runs of 3 rounds against `de42e83` gave 0.983 and 0.975 for the shop and
  0.962 to 0.993 for the MUI pages. The bench pages moved by less than
  their noise (table varies by 10% between rounds by itself). Other work
  ran on other cores of the machine during the rounds (a load average of
  up to 6). Each figure is from one build of each side: where the code
  sits moves a render by up to 2.4% (see the frame of `JS_CallInternal`
  above).
  `fixlib` puts the question to `lreSplit` into the handler of the split
  ops in all 13 files, after it finds that the handler pushes the way it
  computes and jumps to the next op, and pins `lre_canonicalize`,
  `lre_case_folding_entry` and `lre_poll_timeout` beside the functions
  `regexSites` pinned already. This changes only the generated Go, with no C hunk
  (`internal/fixlib/regexskip.go`). Tests: `lib/regexskip_test.go`, whose
  `testdata/regexsplit.txt` holds what Node gives for 84 regexps on 73
  strings through every API that runs one (the engine before the change
  gives the same), with `testdata/regex.txt` as before,
  `lib/regexskip_memgrind_test.go`, `internal/fixlib/regexskip_test.go`,
  and `TestRegexScanStops` and `TestRegexMatchStops` in `timeout_test.go`,
  which stop regexps that try every position, and single long matches.
- Outside linux, VMs on several goroutines no longer wait for each other
  on the TLS stack. There libc keeps that stack in segments of about 4 KiB,
  each malloc'd under the one lock of libc's allocator, and frees a segment
  once the stack is two segments below it. A render that goes a few calls
  up and down past the end of a segment so mallocs and frees segments
  again and again. Each runtime now allocates and frees one block of 255
  segments (about 1 MiB) right after it creates its TLS (`warmTLS` in
  `tlsstack_other.go`), and libc keeps the block as the segment after the
  first, so every frame up to that depth goes on it and no call mallocs a
  segment. Memory: each VM keeps that block until it is closed. libc's
  allocator maps it as a region of 1 MiB, and only the pages the TLS stack
  has reached are written, so it holds about as much memory as the deepest
  call of the VM took, at most 1 MiB, where it used to free those
  segments. On windows the allocator commits the whole region when it maps
  it, so each VM takes 1 MiB of the commit limit, though only the pages the
  stack reached are in memory. After `Close` the allocator keeps the region
  to hand out again, as it keeps every region it frees: the 4 MiB of
  regions of one size it freed last keep their written pages in memory,
  and older ones give their pages back to the system. A single frame
  larger than the block, far larger than any frame of the engine, would
  make libc free the block and malloc a segment of that size in its place. A render of tifa's example shop with 10 VMs on darwin/arm64
  (Apple M4, `GOMAXPROCS=10`) went from 215 to 281 renders a second to 331
  to 390, 1.4 to 1.55 times as many in each pair of runs, and the time the
  render goroutines spent waiting for a lock from 3.8% to 5.3% of it to
  about 0.5%. One VM renders as fast as before. Linux is not changed:
  libc keeps one block per slot there and never frees it. Test:
  `tlswarm_other_test.go`.
- The interpreter counts its interrupt counter down in its own body. Each
  of the eight interrupt polls of `JS_CallInternal` (at its entry and at
  each backward jump, so a loop polls twice a pass) called
  `js_poll_interrupts`, which only counts a counter down and calls
  `__js_poll_interrupts` once the counter runs out, every 10000 polls.
  `JS_CallInternal` is too large for Go to inline a function of that cost
  into it, so every poll was a call. `fixlib` writes each poll as
  `pollDue(ctx+OFFSET) && __js_poll_interrupts(tls, ctx) != 0`: `pollDue`
  (`lib/poll.go`) is the countdown, small enough to be inlined, and OFFSET
  is the counter `js_poll_interrupts` reads. Its text is pinned in
  `pollSites`. On top of the changes above, a render of a page of tifa's
  example shop takes 0.6% less time with PGO and about as long without,
  on linux/amd64 (AMD EPYC), with the same HTML. This changes only the generated Go, with no C hunk
  (`internal/fixlib/poll.go`). Tests: `lib/poll_test.go` (the handler is
  still called as often, and stops an endless loop) and
  `internal/fixlib/poll_test.go`.
- Each runtime has a front cache in front of the small block allocator. A
  freed block of 16 to 512 bytes went back to its arena: the free list of
  the arena, its count of used blocks and the lists of arenas changed on
  every free, and again on the next allocation of that size. Now
  `__js_free` puts the block on a list of its size, up to 256 blocks a size
  (1.6 MiB at most), and `__js_malloc` takes it from there
  (`lib/frontcache.go`). A cached block keeps its header, so to the arena
  and to `js_malloc_usable_size` it is a block in use. `JS_RunGC` gives
  every cached block back to its arena, so after a GC the memory a runtime
  holds is what it was without the cache, give or take how the blocks fell
  in the arenas: one VM of the shop holds 48 KiB more after a GC (7.37 MiB
  rather than 7.33). With a memory limit set, an allocation that fails
  gives the cache back and is tried once more, so cached blocks never make
  an allocation of another size fail. This holds for the blocks of this
  cache only: the shape cache below does not give its shapes back. A
  cached block also keeps its arena of 4 KiB from being freed until the
  next GC: with every list full and each block in an arena of its own,
  that is about 31 MiB, which the GC threshold, as it counts those arenas,
  and the retry past the memory limit get back. `JS_FreeRuntime` gives the
  cache back first and turns it off. The cache lives in a field that `fixlib` adds at
  the end of `JSRuntime` (`Ffork`, the state of `lib/runtime.go`), which
  `JS_NewRuntime2` allocates and clears with the C struct, so each runtime
  has its own, used only by the goroutine that runs it, and the fields of
  the C struct keep their offsets (`internal/fixlib/runtimefield.go`).
  Alone it makes a render 0.9% faster with PGO and leaves it about as it
  was without. The shape cache below allocates and frees more: with it,
  the front cache and the interrupt countdown take another 0.9% off a
  render without PGO. Generated Go only, no C hunk
  (`internal/fixlib/frontcache.go`). Tests: `lib/frontcache_test.go`,
  `alloc_test.go` (8 VMs on 8 goroutines, the same results, and memory
  after a GC that does not grow), `internal/fixlib/frontcache_test.go` and
  `internal/fixlib/runtimefield_test.go`.
- A VM keeps the shapes of plain objects once their objects are gone. The
  shape of an object is its keys in order, and an object literal goes
  through one shape for each key. `add_property` grew a shape in place when
  only its object used it, so the shapes in between were gone once the
  object had its keys, and the last one went with the last object: the
  next literal with the same keys copied and grew them all again. Now
  `js_free_shape0` keeps a shape whose count drops to zero in a ring of the
  last n, with a count of one, and `add_property` copies a shape that only
  its object uses instead of growing it, so that the shapes in between are
  kept too (`lib/shapecache.go`). The next literal with the same keys finds
  every shape it needs in the shape hash table, and makes none. The times
  in this entry were all taken without PGO, on linux/amd64 (AMD EPYC). A
  copy costs an allocation and the keys so far, which an object that gets keys
  no other object has, one at a time, pays for each key and never gets
  back: such objects took 22% longer to build. So each shape `add_property`
  finds earns one copy, up to 64, and without credit the shape grows in
  place. The shapes in between are then still kept, only later: the shape
  an object leaves when it finds the next one kept is kept. Objects of
  keys no other object has now take about 6% longer to build than with
  the cache off, and object literals with the same keys 30% less.
  Only shapes whose prototype is the `Object.prototype` of the VM, that
  have at most 32 keys and no key that is a symbol not registered are
  kept, so a kept shape holds nothing alive but that prototype and the
  atoms of its keys, and a `WeakRef` or `FinalizationRegistry` whose
  target is a symbol sees it go as it would without the cache. Shapes the
  cycle collector frees are not kept, and `JS_FreeRuntime` lets every kept
  shape go. The ring is in the state of the runtime, next to the front
  cache. In `lib` it is off until `JS_SetShapeCache(ctx, n)`, which
  refuses a ring whose size does not fit in a `size_t`. A new VM keeps
  `DefaultShapeCache` (2048) shapes, `VM.SetShapeCache` changes it, and
  `VM.Close` turns it off before it frees the context. Memory: one VM of
  the shop holds 647 KiB more after a GC, and with the front cache as well
  722 KiB more (8.03 MiB rather than 7.33). With 1024 shapes it was 190 KiB
  and 461 to 502 KiB more, and with 4096 shapes and the front cache 1.26
  MiB more. How the blocks fall in the arenas moves these by up to 0.5 MiB
  from one run to another, with the same shapes and objects left. Loading
  the shop's bundle alone keeps about 1250 shapes, so a ring of 1024 is
  full before the first render. Kept shapes count against the memory limit
  of the VM, and they are not let go when an allocation fails: an
  allocation in `add_property` can come right before the object takes a
  shape that only the cache holds. A shape of 32 keys of about ten
  characters, that no other object has, takes about 3 KiB with the atoms
  of its keys, so 2048 of them take about 6 MiB, and the atom table keeps the size it grew to after they go.
  A script that runs close to its memory limit can run out of memory with
  the cache on where it would not without it: give such a VM a smaller
  cache, or none. Without PGO, on linux/amd64 (AMD EPYC), with the same
  HTML, the shape cache alone makes a render 3.6% faster, and with the
  front cache and the interrupt countdown above a render takes 4.5% less
  time than without the three (six rounds). Two VMs on two hardware
  threads of one core render 2.9% more pages a second. Generated Go
  only, no C hunk (`internal/fixlib/shapecache.go`). Tests:
  `lib/shapecache_test.go` (reuse, which shapes are kept, symbol keys, the
  ring, the credit for copies, the cycle collector, `JS_FreeRuntime` with
  the cache on, out of memory, a ring too large for a `size_t`, 8 runtimes
  on 8 goroutines), `shapecache_test.go` (the memory limit, and objects
  built in many ways that read the same with the cache on and off) and
  `internal/fixlib/shapecache_test.go`.
- A string key that is already an atom costs less. `o[k]`, `o[k] = v`,
  `k in o`, `hasOwnProperty(k)`, computed keys of literals and every other
  path that turns a value into a property key go through
  `JS_ValueToAtom`, which took a TLS frame, duplicated the string
  (`JS_ToPropertyKey`), checked that it is no index (`is_num_string`) and
  looked its index up (`__JS_NewAtom`), even when the string was the atom
  itself: a name the code has, a key from `for...in`, `Object.keys` or
  `JSON.parse`. On the pages of tifa's example shop, 92% of the strings
  `JS_ValueToAtom` gets are such atoms. Now, for a string, it first calls
  `atomKey` (`lib/atomkey.go`): an atom string that is not empty and does
  not start with a digit, for which `is_num_string` is false, gets its
  index from the atom table and one more reference, none for a constant
  atom, as the generic path gives. Every other string, numbers and
  symbols take the generic path, and a number does not even make the
  call. Nothing is cached, so memory use is the same. On linux/amd64
  without PGO (AMD EPYC 7B13, one core), against v0.26.16, a micro
  benchmark reads with a string key in 13% less time, writes in 30% less,
  runs `k in o` in 26% less and `hasOwnProperty(k)` in 14% less, and a
  server render of a page of the shop takes 1.3% less time (median of 6
  rounds, 0.9% to 1.5% over the rounds), with the same HTML. These were
  measured before the `for in`, bitwise and comparison changes above.
  `fixlib` puts the call into all 13 files and takes the atom type and the
  length of a `JSString` from each file's `__JS_NewAtom` and
  `is_num_string`, so the big-endian target gets its own bit fields.
  Generated Go only, no C hunk (`internal/fixlib/atomkey.go`). Tests:
  `lib/atomkey_test.go` (the atom and the reference counts against the
  generic path, the keys it leaves to that path, an atom freed and made
  again, the description of a symbol and the key of a registered symbol,
  strings that share the symbol's atom), `lib/atomkey_memgrind_test.go`
  (the same tests leave no C memory behind), `atomkey_test.go` (keys that
  look like numbers, index strings, symbols, the text of symbols and
  constant atoms through every way a key goes, against Node, and the atom
  count after keys are dropped) and `internal/fixlib/atomkey_test.go`.
- `JSON.stringify` and `JSON.parse` copy the plain characters of a string
  in runs. Both went through a string one character at a time: `JSON.stringify`
  read each with `string_getc` and wrote it with `string_buffer_putc`, and
  `JSON.parse` put each byte of its text into the new string the same way,
  decoding UTF-8 with `unicode_from_utf8`. Now, at the start of each turn
  of those loops, `jsonQuoteRun` and `jsonStringRun` (`lib/jsonstr.go`)
  copy the run of characters the loop would put as they are, in one Go
  copy, and leave the loop the character that ended the run: one to
  escape, the end of the string, a surrogate (`JSON.stringify` decides how
  to write it) or a byte that is not valid UTF-8 (`JSON.parse` throws the
  same error at the same place). The new string grows and becomes 16-bit
  as with `string_buffer_write8` and `string_buffer_write16`, so it is
  the same string, and running out of memory in a run fails the call as
  before. A run of `JSON.parse` stops at the length of the longest
  string, so a text of `JS_ParseJSON` with a longer string still throws
  "string too long". Reviver, replacer, `toJSON`, `space` and the other
  paths are not touched. On linux/amd64 without PGO (AMD EPYC 7B13, one
  core), against 4a4103b (v0.26.16 and 9 later commits), quoting the JSON
  text of each route's data of tifa's example shop as one string again,
  as tifa does, takes 41% less time, and `JSON.parse` of its render data
  9.5% less, while a document of many short strings and numbers parses in
  the same time and stringifies in 2% less. JSON is about 1% of a render,
  so a render can gain at most about 0.3%. The 8 rounds measured 0.2% to
  1.7% less, inside the spread of a round. The HTML and the memory are the
  same.
  `fixlib` puts the two calls into all 13 files and takes the length, the
  16-bit flag and the offset of the characters of a `JSString` from each
  file's `JS_ToQuotedString` and `string_get`, so the big-endian target
  gets its own bit fields. Generated Go only, no C hunk
  (`internal/fixlib/jsonstr.go`). Tests: `lib/jsonstr_test.go`, whose
  `testdata/jsonstr.txt` is what Node 24 prints for `testdata/jsonstr.js`
  (strings of every kind of character at the lengths around the runs, as
  ropes, substrings and keys, through `stringify`, `parse`, reviver,
  replacer and `toJSON`), the errors and their positions and the invalid
  UTF-8 cases against the engine before the change, both runs called
  directly on buffers of every room, 8-bit and 16-bit (the characters,
  where the run stops, and a buffer never filled past its room), each way
  a run grows or widens a buffer failing for lack of memory, and a run
  past the longest string, `lib/jsonstr_memgrind_test.go` (no C memory
  left behind) and `internal/fixlib/jsonstr_test.go`.
- A new function gets its `length` and `name` without the shapes in
  between. `js_function_set_properties` gave every new function the two
  through `JS_DefinePropertyValue` and `add_property`, one at a time: the
  function went from the empty shape of its prototype, through a shape
  with `length` only, to the shape with both. Nothing held the empty shape
  or the one with `length` only for long, so they were made again for each
  function and freed again a moment later (3,000 arrows, methods and
  constructors made 5,001 shapes). Now `js_function_set_properties` calls
  `funcShape` first (`lib/funcshape.go`). For each of the four prototypes
  of new functions (`Function.prototype` and those of async, generator and
  async generator functions) it remembers the empty shape and the shape
  with both, the first time a function goes through `add_property`, and
  puts the next functions with that empty shape straight onto the shape
  with both, and writes the two values (the same 3,000 functions made
  2,000 shapes, for the object literals and the `prototype` of the
  constructors). Why this is safe: a shape is shared and never changes in
  place while more than one object holds it (`add_property` and
  `js_shape_prepare_update` copy it first), and `funcShape` holds both, so
  they stay as they were and the empty shape cannot be freed and its
  memory taken by another shape. The shape with both is the one
  `add_property` gives, with the same prototype, keys, order, flags and
  size of the property array, and the name is made before the function
  changes, so the cycle collector, which can run there, finds it as it
  is. The shapes are held for the context, as it holds the shapes of its
  arrays and `arguments` objects: `JS_MarkContext` marks them, so the
  cycle collector frees a context that only they hold alive, and
  `JS_FreeContext` lets them go. Everything else, an object that is not
  an ordinary extensible one or has another shape, takes the old path.
  The shapes held add one or two shapes to a VM (88 to 176 bytes in one VM
  of the shop). Without PGO, on linux/amd64 (AMD EPYC, one core), with the
  same HTML, a render of tifa's example shop takes 0.982 of the time it
  took with the regexp change above alone (the median of 10 rounds, 0.967
  to 0.989), the MUI pages 0.993 to 1.000 and the bench pages 0.984 and
  0.995, each from one build of each side. Against `80261af`, with the
  code elsewhere, the shop took 0.972 (0.951 to 1.003), the MUI pages 0.990
  to 1.004, and the bench pages 0.993 and 0.995. On `de42e83`, 3 rounds gave
  the shop 0.988 and the MUI pages 0.983 to 0.997.
  `fixlib` puts the three calls into all 13 files, and checks the text of
  `js_function_set_properties`, `add_property`, `find_hashed_shape_prop`,
  `add_shape_property`, `js_shape_prepare_update`, `js_clone_shape`,
  `JS_NewObjectFromShape`, `JS_AtomToString`, `JS_MarkContext`,
  `JS_FreeContext` and `JSShape` against fingerprints. This changes only
  the generated Go, with no C hunk (`internal/fixlib/funcshape.go`).
  Tests: `lib/funcshape_test.go`, whose `testdata/funcshape.txt` holds
  what Node gives for the own properties of 36 kinds of new functions,
  made many times and changed after they were made (the engine before the
  change gives the same), the shapes it holds and lets go, and a context
  that only they hold alive, `lib/funcshape_memgrind_test.go` and
  `internal/fixlib/funcshape_test.go`.

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
documentation or with its copies. `internal/gointl`, a copy of go-intl, keeps
go-intl's [LICENSE](internal/gointl/LICENSE), the Unicode License V3, which is
the notice for its code and for the data it embeds, generated from Unicode
CLDR 48.2, ICU 78.3 and the IANA tz database (public domain), as
[internal/gointl/SOURCES.md](internal/gointl/SOURCES.md) lists: a program
that imports it includes that notice in the same way. Maintenance
notes are at the [end of this file](#maintaining-the-fork).

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
  changes to the interpreter loop, the call into `lib/objlit.go`, the
  call into `lib/regexskip.go`, the cold paths moved out, the TLS frames
  taken per statement, `set_value` written out in the interpreter, the
  interrupt countdown, the front cache, the shape cache, the call into `lib/forin.go`, the calls into `lib/bitwise.go`, the comparisons written out in the interpreter, the calls into `lib/funcshape.go`, the call into `lib/atomkey.go`, the calls into `lib/jsonstr.go`, the frame of
  `JS_CallInternal` taken once on linux and the frames
  freed without a defer conflict. Take the upstream version of every generated file, then put
  the changes back:

  ```sh
  git checkout --theirs -- 'lib/ccgo_*.go' 'lib/internal/run-test262/ccgo_*.go'
  go run ./internal/fixlib
  ```

  `fixlib` also rewrites `modernc.org/libquickjs` imports in hand-written
  files under `lib/`. `go run ./internal/fixlib -check` lists files that still
  need changes. Some steps check the code against a list kept in
  `internal/fixlib`, and a flag prints the list as the files have it. A
  report fails while a list it depends on is out of date, so update them
  in this order: `-frames`, then `-up` (on the files as taken from
  upstream), then `-cells`, then `-slots`. Then fix `lazySites`
  (`internal/fixlib/lazy.go`) and `inlineCallees`
  (`internal/fixlib/inline.go`) by hand from the numbers `fixlib` reports:
  no flag prints them. Last come the fingerprints, `-define`,
  `-backtrace`, `-regex`, `-lazyaddr`, `-inline`, `-poll`, `-runtime`,
  `-front`, `-shape`, `-forin`, `-bitwise`, `-eq`, `-json`, `-funcshape`, `-atomkey` and
  `-oneframe`. Each of these runs
  every change of `fixlib` and skips only the fingerprint checks, so a
  report that stops on another list, such as "TLS frames taken by
  statement changed", means that list is out of date, not the one the
  flag prints. Read
  every new line before you put it in. If `fixlib` reports that
  `XJS_ToCStringLen2` changed shape, read the new upstream code: QuickJS
  may have fixed the leak itself. If the fail label now frees `val`, drop
  `fixToCString` and the `JS_ToCStringLen2` hunk, or the added calls free
  it twice. If there is a new jump to the label, check that `val` is freed
  on that path. If it reports that `_getTimezoneOffset` changed shape, or
  that the engine calls libc's local time functions (`localtime`,
  `mktime`, `tzset`, ...) somewhere else, upstream reads the local zone
  some new way: make that answer UTC too and teach `fixTimezone` about it.
  If it reports that `_build_backtrace`, `_get_prop_string`,
  `JS_GetPropertyInternal`, `JS_GetOwnPropertyInternal`, the interpreter's
  `get_field`, `js_print_error`, `js_print_object` or `JS_ComputeMemoryUsage`
  changed shape, compare the new upstream `build_backtrace` with
  `lib/backtrace.go`, which formats the same text from the recorded frames
  and must change with it. If it reports that `_build_backtrace`,
  `_get_prop_string` or `_JS_GetFunctionBytecode` changed (a fingerprint
  other than the one in `internal/fixlib/backtracesites.go`), the same
  holds: a pending stack must still come out with the bytes
  `_build_backtrace` writes. Once `lib/backtrace.go` matches, put the new
  fingerprints in (`go run ./internal/fixlib -backtrace` prints them). A
  change to the C strings they use counts, a string moving in the table
  does not. If it reports that `XJS_Throw`,
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
  lists each place). If it reports that growing a buffer in `Xdbuf_claim`
  no longer starts with the test of a failed buffer, read the new
  `Xdbuf_claim`: `fixCompile` marks a buffer that failed as full, and only
  an early return for a failed buffer keeps the next write out. If the
  test only moved and a failed buffer still fails at once, teach
  `fixCompile` the new place. If it is gone, make `fixCompile` add it.
  Do not edit the generated code by hand.
  If it reports a new jump to the fail label of `_resolve_labels`, read
  where it comes from: the end that `fixCompile` writes keeps the new code
  and frees the relocations left, which is right only after a failed
  `_add_reloc`. Make the end right for the new path, then fix the count.
  fixlib takes a change of its own as made only when its whole text is
  where it puts it. If it reports that such a text is somewhere else, is
  there twice, or that `_resolve_labels` has a label `out_of_memory` or
  part of the change, upstream has likely written part of the change
  itself: read the new code, then drop that part of fixlib or teach it the
  new place.
  If it reports that `XJS_NewRuntime2` or `_JS_ThrowStackOverflow` changed
  shape, a new runtime must still start with a stack size of 0 (the port's
  `MaxStackSlots`), and running out of stack must still throw a
  `RangeError` (`internal/fixlib/stack.go`). The same holds when it reports
  that the stack check of `_next_token` or `_json_next_token` changed
  shape: the tokenizers must throw that `RangeError` too. If it reports
  that the uses of `"stack overflow"` differ from `stackOverflowUses`, a
  new place throws that message: make it call `_JS_ThrowStackOverflow`
  (teach `fixStack` to), or add it to the list if it is the regexp parser.
  If it reports that a function of `ownFlagsCallers`
  (`internal/fixlib/main.go`) reads the value of the descriptor it gets, or
  uses it another way, read the function: if it now needs the value, it
  must call `_JS_GetOwnPropertyInternal` again, so take it off the list.
  Otherwise fix the count there.
  If it reports a call of `XJS_GetException` that is not in
  `exceptionHandlers` or `exceptionReaders` (`internal/fixlib/uncatchable.go`),
  read the new caller: if it hands the exception to a script (a rejection,
  a handler call, a new throw), add it to `exceptionHandlers`, so that it
  gets `_js_get_exception`. If it reports a store of the runtime's
  exception (`current_exception`) in a function that is not in
  `exceptionWriters`, read it the same way: code that takes the exception
  out by hand must leave an uncatchable one pending, as `_JS_IteratorClose`
  does. The same goes when it reports a use of the exception's address
  (`rt` plus its offset, or `&` of the field): a store through it does not
  look like one. fixlib sees the address only on a runtime named `rt`,
  `rt1` and so on, or `Frt`, with the offset written out: a store through
  a runtime under another name goes through unseen. If it reports that
  `XJS_Throw` or `_JS_IteratorClose` uses the exception before the
  uncatchable check, move the check up to before that use.
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
  moves the offsets and is the usual cause: `go run ./internal/fixlib
  -cells` prints the uses as the files have them, each with the named
  local it starts in. If the use cannot touch the bytes of a cell, put it
  into `cellsFrameUses` with the reason, or fix the count, or add the type
  to `valueSizes`. If it can, the cells must stay in
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
  If it reports that `Xlre_compile`, `Xlre_exec`,
  `_lre_exec_backtrack`, `Xlre_canonicalize` or `_lre_case_folding_entry`
  changed (a fingerprint other than the one in
  `internal/fixlib/regexsites.go`), read the new code against
  `lib/regexskip.go`: the loop and `save_start 0` at the start of the
  bytecode, the header fields it reads, the size and the meaning of every
  op `lreFirstSet` follows, what the character and class ops match at a
  position and how `GET_CHAR` reads it (`lreSplit`), that the split ops
  only push a state, that a failed match at one position undoes its
  captures down to the loop (`lreRun`), that `lre_canonicalize` gives an
  ASCII result outside unicode mode only for ASCII units, that
  `lre_poll_timeout` counts one step and returns non-zero when the machine
  must stop (`lreRun` and the split ops call it), and `lre_exec` must still
  return `LRE_RET_TIMEOUT` when `lre_check_timeout` asks it to stop. Then put the new fingerprints in (`go run ./internal/fixlib
  -regex` prints them, even while the table is wrong) and run `go test
  -run 'Regex|Canonicalize' ./lib`. If it reports that it cannot find the
  handler of the split ops, its push or the jump to the next op, update
  the patterns `reRegexSplit`, `reRegexSplitPush` and `reRegexNext` in
  `internal/fixlib/regexskip.go` after reading the new handler.
  If it reports that the cold paths of `coldSites` changed, read the new
  `js_strict_eq2` or `__JS_AtomToValue`: the cold path must still be the
  only code that passes the address of a part of the frame on, and it must
  still read only the parameters, from the cells the function stores them
  in at its entry. Then update the shapes in `coldShapes`
  (`internal/fixlib/cold.go`), or the frame size and cell count in
  `internal/fixlib/coldsites.go`, and run `go test -run ColdPath ./lib`.
  If it reports that a function of `lazySites` changed (a frame size, a
  number of cell uses, of statements with a frame of their own or of out
  parameters other than the one in `internal/fixlib/lazy.go`), or a use of
  its frame that `fixLazyFrames` cannot show is safe, read the function:
  each statement that passes an address in the frame on or can reach a
  stack check must still be an expression, an assignment, a return of one
  value or the condition of an if, and each address must still go to a
  callee of `addrCallees`. Then update `lazySites`. If it reports that the
  functions of `lazyAddrFuncs` or a `get_own_property` method changed (a
  fingerprint other than the one in `internal/fixlib/lazyaddrsites.go`),
  read them: none may keep an address it is given after it returns, nor
  return it. Then put the new fingerprints in (`go run ./internal/fixlib
  -lazyaddr` prints them).
  If it reports that `set_value` changed (a fingerprint other than the one
  in `internal/fixlib/inlinesites.go`), read it: `fixInline` writes its
  body out in the interpreter, and must still be able to (no result, no
  early return, no TLS frame, its Go array used only as one JSValue).
  Then put the new fingerprints in (`go run ./internal/fixlib -inline`
  prints them). If it reports another number of calls, a call that is not
  a statement of its own or a name of the callee that is a local of
  `JS_CallInternal`, read the new interpreter and update `inlineCallees`
  in `internal/fixlib/inline.go`.
  If it reports that `js_poll_interrupts` changed (a fingerprint other
  than the one in `internal/fixlib/pollsites.go`), read it against
  `pollDue` in `lib/poll.go`: it must still count the counter at the
  offset `fixPoll` reads down by one and call `__js_poll_interrupts` when
  the counter is at most zero. Then put the new fingerprints in (`go run
  ./internal/fixlib -poll` prints them). If it reports that `JSRuntime` or
  `JS_NewRuntime2` changed (`internal/fixlib/runtimesites.go`), check that
  `malloc_ctx` is still the first field and that `JS_NewRuntime2` still
  allocates and clears the runtime with `sizeof(JSRuntime)`, then run
  `-runtime`. If it reports that the functions of `frontFuncs` or
  `JSMallocBlockHeader` changed (`internal/fixlib/frontsites.go`), read
  them against `lib/frontcache.go`: `__js_malloc` must still size a block
  as `frontPop` does, a large block must still have `MFREE_NIL` in its
  header (`js_malloc_large` writes it), `__js_realloc` must still move a
  small block through `__js_malloc` and `__js_free`, and `js_def_malloc`
  and `js_def_realloc` must still fail only past the memory limit or when
  libc fails. Then run `-front`. If it reports that the
  functions of `shapeFuncs` or `JSShape` changed
  (`internal/fixlib/shapesites.go`), read them against
  `lib/shapecache.go`: `add_property` must still copy a shape whose count
  is not one, link the copy into the hash table, and take a shape it finds
  there on the line `shapeFound` is put before. A shape whose count is not
  one must still never change in place: `js_shape_prepare_update` copies
  it, and `resize_properties` and `compact_properties` change only a shape
  of count one that an object has. Outside the cycle collector a shape must
  still be freed only through `js_free_shape0` (`free_object` frees the
  shape of an object that way, and `JS_NewObjectFromShape` takes the count
  it is given), `js_new_shape2` must still grow the hash table when it is
  half full, `__JS_NewAtom` must still put the index of a symbol that is
  not registered in its `hash_next`, where `js_get_atom_index` and
  `JS_FreeAtomStruct` read it and which `JS_ResizeAtomHash` leaves alone,
  and the cycle collector must still
  free the shapes of a cycle itself while `gc_phase` is
  `JS_GC_PHASE_REMOVE_CYCLES`. Then run `-shape`. If it
  reports that `gc_phase` is not the byte right after `tmp_obj_list`,
  change `gcPhaseOffset` in `lib/shapecache.go` to where it is.
  If it reports that the functions of `funcShapeFuncs` or `JSShape`
  changed (`internal/fixlib/funcshapesites.go`), read them against
  `lib/funcshape.go`: `js_function_set_properties` must still give
  `length` and then `name`, both configurable only, through
  `JS_DefinePropertyValue`, `add_property` must still move an object onto
  a shape it finds in the hash table as `funcShape` does (`shapeFound`, a
  new count, the old shape freed, the property array kept when the sizes
  match), a shape whose count is not one must still never change in
  place, `JS_NewObjectFromShape` must still give an object a property
  array of the size of its shape, and `JS_MarkContext` and
  `JS_FreeContext` must still mark and free the shape of arrays where
  `funcShapesMark` and `funcShapesFree` are put. Then run `-funcshape`.
  If it reports that `js_for_in_prepare_prototype_chain_enum`,
  `JS_GetOwnPropertyNamesInternal`, `get_shape_prop` or
  `JS_FreePropertyEnum` changed (a fingerprint other than the one in
  `internal/fixlib/forinsites.go`), read them against `lib/forin.go`: for
  an object that is not exotic, the names listed with `JS_GPN_STRING_MASK`
  and `JS_GPN_ENUM_ONLY` must still be only shape properties with a name
  and the enumerable flag, the loop must still go on to the next prototype
  when the list is empty, and freeing an empty list must still do nothing.
  If it reports that the reads of the enumerable flag or the test of
  `is_exotic` changed shape, find where the bits are now and update the
  patterns in `internal/fixlib/forin.go`. Then put the new fingerprints in
  (`go run ./internal/fixlib -forin` prints them) and run
  `go test -run ForIn ./lib`.
  If it reports that `js_binary_logic_slow`, `js_shr_slow`,
  `JS_ToNumberHintFree` or `JS_ToInt32Free` changed (a fingerprint other
  than the one in `internal/fixlib/bitwisesites.go`), read them against
  `lib/bitwise.go`: for two ints or floats the slow paths must still give
  `ToInt32` of each, the same operator with the count masked to 5 bits, and
  an int, or a float for a `>>>` above 2^31 - 1. If it reports another
  number of calls in `_JS_CallInternal`, read the new interpreter: each
  call of the slow functions must have the two operands on top of the
  stack, and the opcode in `op`. Then put the new
  fingerprints in (`go run ./internal/fixlib -bitwise` prints them) and run
  `go test -run Bitwise ./lib`.
  If it reports that `js_strict_eq_slow`, `js_strict_eq2`, `js_eq_slow`,
  `js_string_eq`, `__JS_FreeValueRT`, `__JS_FreeValue` or `JS_IsHTMLDDA`
  changed (a fingerprint other than the one in
  `internal/fixlib/eqsites.go`), read them against the cases in the comment
  of `fixEq`: each pair it answers must still get that answer from the
  slow path, with both operands freed, and string atoms of one text must
  still be one atom. If it reports that a bit field, `JS_FreeValue` or
  `js_strict_eq_slow` changed shape, or that a slow call pops another
  size than a value, find where the bits, the reference count or the
  result are now and update the patterns in `internal/fixlib/eq.go`. If it reports another number of calls in
  `_JS_CallInternal`, read the new interpreter: each slow call must still
  have the two operands on top of the stack and pop one. Then put the new
  fingerprints in (`go run ./internal/fixlib -eq` prints them) and run
  `go test -run Eq ./lib`.
  If it reports that `JS_ValueToAtom` or a function of `atomKeyFuncs`
  changed (`internal/fixlib/atomkeysites.go`), read them against
  `lib/atomkey.go`: for a string that is an atom and does not start with
  a digit, `JS_ToPropertyKey` must still return it with one more
  reference, `is_num_string` must still be false for it, and
  `__JS_NewAtom` must still answer with `js_get_atom_index` and drop that
  reference only for a constant atom. Then run `-atomkey`. If it reports
  that `JS_ValueToAtom` has another signature, or that the atom type test
  of `__JS_NewAtom` or the length read of `is_num_string` changed shape,
  update `reAtomKeyHead`, `reAtomKeyType` or `reAtomKeyLen` in
  `internal/fixlib/atomkey.go`: `atomKey` reads the atom type and the
  length through the constants `fixlib` takes from them.
  If it reports that the frame of `_JS_CallInternal` is not taken as
  `fixAlloca` writes it (`fixOneFrame` in `internal/fixlib/oneframe.go`),
  that the block that takes it again runs a statement the entry does not
  run once, or that the function has another `tls.Alloc`, read the new
  function: the block may only make again what the entry made on the
  frame, or dropping it changes what the rest of the function finds
  there. If the frame of 1280 bytes changes size, `fixOneFrame` changes a
  linux file only while the power of two that holds the frame also holds
  the room of `allocaMax`, so check that the files it changes are still
  the ones `oneFrameFiles` lists in `internal/fixlib/oneframe_test.go`.
  If it reports that the text between the entry and that block changed (a
  fingerprint other than the one in `internal/fixlib/oneframesites.go`),
  read it: nothing there may write again the parameter copy or the
  pointers into the frame the block made, except on a path that returns
  or jumps past the block. Then put the new fingerprints in (`go run
  ./internal/fixlib -oneframe` prints them).
  If it reports that a function of `jsonFuncs` changed
  (`internal/fixlib/jsonsites.go`), read them against `lib/jsonstr.go`:
  `JS_ToQuotedString` must still write every character that is not `"`,
  `\`, below 0x20 or a surrogate as it is, `json_parse_string` must
  still put every byte from 0x20 to 0x7f that is not the quote or `\` as
  it is and decode the others with `unicode_from_utf8`, which must still
  take the forms `jsonUTF8` takes, and `string_buffer_write8` and
  `string_buffer_write16` must still grow and widen the buffer as
  `jsonWrite8` and `jsonWrite16` do. Then run `-json`. If it reports that
  a loop or `string_get` changed shape, update `reQuoteLoop`,
  `reParseLoop` or `reJSONWide` in `internal/fixlib/jsonstr.go`, and run
  `go test -run JSONString ./lib`.
  `fixlib` frees every TLS frame before each return instead of in a
  defer, as its last change (`fixNoDefer` in `internal/fixlib/nodefer.go`).
  It first puts the defers back (`restoreDefer`), so every other change,
  check and report (`-frames`, `-slots`, ...) works on the frames as ccgo
  writes them, and it checks that putting them back gives the file it was
  given, byte for byte. If it reports a frame that is not freed by
  `defer tls.Free(N)` right after `tls.Alloc(N)` (or by the closure of
  `fixAlloca`), a `tls.Alloc` or `tls.Free` somewhere else, a `defer`
  that is not in `keptDefers`, a function literal, a bare `return` or a
  local with the name of the result, read the function: a defer that frees
  anything on the TLS stack, or a return the rewrite cannot see, would
  leave the stack wrong on some path. Teach `fixNoDefer` the new form, or
  add a defer that only frees malloc'd memory to `keptDefers`. The VM puts
  the TLS stack back after a panic (`tlsstack_linux.go`,
  `tlsstack_other.go`), which knows how libc keeps that stack: one block per
  slot on linux, bytes in segments elsewhere. After a libc release, run
  `go test -run EnginePanic .` with and without `-tags=libc.memgrind`.
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
  interpreter loop, the calls into `lib/objlit.go` and
  `lib/regexskip.go`, the cold paths moved out, the frames taken by
  statement, the calls written out in the interpreter, the calls into
  `lib/forin.go`, `lib/bitwise.go`, `lib/atomkey.go` and `lib/jsonstr.go`, the comparisons written out in the
  interpreter, the frame of `JS_CallInternal` taken once and the frames
  freed without a defer have no C form: they
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

### Updating go-intl (`internal/gointl`)

`internal/gointl` is a copy of [go-intl](https://github.com/go-quickjs/go-intl),
a pure Go ECMA-402 and Temporal library, at the release and commit in
`internal/gointl/UPSTREAM` (v0.6.1). Nothing in JS uses it yet. It is
internal because its API is go-intl's and changes between releases, and it
sits at the module root so that `intl/` and a later Temporal package can
import it.

The files and directories named `qjs*`, and `UPSTREAM`, are ours. Everything
else is go-intl's, with its import path rewritten, and is changed only by
`internal/gointl/qjs.patch`, which keeps to loading data: `source.go` serves
our packs instead of go-intl's 18.9 MB `data.pack`, `internal/blob` tells the
pack tool which pool parts a locale reads, and `dictbe.go` takes a missing
break dictionary for none.

The data is in `internal/gointl/qjsdata/`, written by
`internal/gointl/internal/qjspack/gen` from go-intl's `data/` directory:

- `core.bin` (7.3 MB), everything but the two below.
- `names.bin` (1.9 MB), the display names. Build with
  `-tags quickjs_intl_nonames` to leave it out: `Intl.DisplayNames` then has
  no data.
- `cjdict.bin` (2.0 MB), the Chinese and Japanese word dictionary. Build with
  `-tags quickjs_intl_nocjdict` to leave it out: a run of Chinese or Japanese
  is then one word, as in an ICU without the dictionary.

The locales are those of Chromium 156. `qjsdata/chromium.json` records which
locale Chromium resolves each of go-intl's available locales to, for every
`Intl` constructor, and each service here has the locales Chromium resolves to
themselves: `TestQJSResolveMatchesChromium` checks that every service resolves
every one of those tags as Chromium does. A language's data is kept when
Chromium has the bare language for a service, with the languages it inherits
from, and its plural rules when Chromium has it for `PluralRules`. Chromium
has some locales whose language it has no data for, such as `ha-GH` without
`ha`: they format with the root's data there, and here. The pack tool prints
the languages it drops and those it keeps the plural rules of only.

The shared pools of strings, the collation root and the break dictionaries are
kept uncompressed, because a formatter reads only a little of each and the
pages of the binary it does not touch stay on disk. The rest is DEFLATE in
chunks, and a chunk is inflated into the heap, once per process, the first
time a formatter reads a file in it. `QJSInflated` reports how much that is.

The bytes a formatter reads are never to be written: an uncompressed chunk is
the binary's read-only data, where a write faults, and an inflated one is
shared by every formatter and VM in the process. Code that hands such bytes
to JS, as an `ArrayBuffer` or the like, copies them first.

`make gointl` runs the checks `go test ./...` does not: the pack tool's tests,
which need the `gointl_walk` build tag, the build without the optional packs,
and that `qjsdata/` is what go-intl's data gives.

To move to a new go-intl release, set `VERSION` and `COMMIT` in
`internal/gointl/UPSTREAM`, then:

```sh
internal/gointl/qjs-sync.sh checkout       # clones go-intl into $GOINTL_UPSTREAM (default ~/.cache/quickjs-gointl/go-intl)
internal/gointl/qjs-sync.sh update         # copies its runtime sources here and applies qjs.patch
internal/gointl/qjs-sync.sh chromium       # records what Chromium resolves the new tags to
internal/gointl/qjs-sync.sh pack           # writes qjsdata/ again
internal/gointl/qjs-sync.sh test -p 2      # go-intl's own tests, on packs of every language
internal/gointl/qjs-sync.sh test-chromium -p 2  # and on the packs built in
go test ./internal/gointl/... && make gointl
```

`chromium` needs Node and the Chromium that Playwright drives, set up as for
the intl golden test above (`PLAYWRIGHT`, and `CHROMIUM` for another browser).
Run it again for a newer Chromium too. Where `qjs.patch` no longer applies,
make the change by hand and run `qjs-sync.sh patch` to write the patch again.
When `pack` fails with parts of a pool that no locale reaches, a data package
has started reading something its `qjs_walk.go` does not: make that walk read
it. `test` leaves out `pack_test.go` and `TestGeneratedTablesAreWellFormed`,
which read go-intl's `data.pack` and `data/`. `test-chromium` takes out of
go-intl's corpora the cases of the locales the packs leave out, and leaves out
the tests that name such a locale in their code or count locales or cases
(`chromium_skips` in the script).

### Checks

```sh
go test -short -timeout 30m ./...                        # all packages (internal/fixlib takes 8 to 13 minutes)
go test -vet=off -run 'TestToCString' ./lib              # the leak reproduction
go test -run 'TestErrorStack' .                          # stack text and memory of lazy error stacks
go test -vet=off -run 'TestBacktrace' ./lib              # shapes, retries and writes of pending stacks
go run ./internal/fixlib -check                          # fork changes present in lib/
go run ./intl/internal/gen -fetch -check                 # intl data is what the generator makes
go test -timeout 30m ./internal/fixlib                   # fixlib notices new reads of property values and new shapes of what it changes
go test -run 'CallDepth|TLSStackBalance|CallAllocs' .    # depth limits, TLS balance, no Go allocation per call
go test -run EnginePanic .                               # the TLS stack after a panic in the engine (also with -tags=libc.memgrind)
go test -run 'Regex|Canonicalize' ./lib                 # regexps give what they gave before lreSkip, and what Node gives
go test -run 'Regex(Scan|Match)Stops' .                  # a timeout stops a regexp on a long string
go test -run ColdPath ./lib                              # ===, Object.is and key names give what they gave before fixColdFrames
go test -run ForIn ./lib                                 # for in gives what it gave before fixForIn
go test -run Bitwise ./lib                               # bitwise operators give what they gave before fixBitwise
go test -run Eq ./lib                                    # comparisons give what they gave before fixEq, and free each value once
go test -run FuncShape ./lib                             # new functions get what Node gives, and the shapes held go with their context
go test -vet=off -run AtomKey ./lib . ./internal/fixlib  # string keys give the atoms and counts of the generic path
go test -run '^$' -bench Call .                          # the cost of each kind of call
go test -run '^$' -bench RegexColors .                   # the cost of a replace with react-spring's color names
for t in linux/amd64 linux/arm64 darwin/arm64 windows/amd64; do
	GOOS=${t%/*} GOARCH=${t#*/} go build ./...
done
```

`lib/internal/run-test262` is a module of its own, so its 69 MiB of
transpiled test runner stay out of the module zip. `Test262` in `lib/` builds
it with `go build -C`. The runner runs the engine in `lib/` (see `fixlib`),
so it checks the changes made there.
