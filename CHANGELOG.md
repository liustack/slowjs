# Changelog

What changed in `github.com/liustack/quickjs` (its name up to v0.27.3 and again from v0.30.0, `github.com/liustack/slowjs` from v0.28.0 to v0.29.11) that programs using it can see. Each release is tagged on `main`: a patch version, or the next minor version when it removes or changes a public API. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). [Fork changes](docs/changes/README.md) has the details of each change, and [Performance](docs/performance.md) how each speedup was measured. Times are for a server render of a page of tifa's example shop, on linux/amd64 (AMD EPYC) unless an entry says otherwise.

## [Unreleased]

### Changed

- The hash table of atoms, the numbers the engine gives property names and the strings it keeps once, now doubles when it holds one atom for two buckets, where it doubled at two atoms a bucket. Making a key from a string walks a shorter chain: in our measurement on linux/amd64 tifa's MUI pages took 1.0% less time and the shop's pages 1.1% less, and the workbench's pages did not change. A VM holds 50 KB to 200 KB more, and a script near its memory limit can run out of memory a little sooner ([strings.md](docs/changes/strings.md#the-hash-table-of-atoms-holds-one-atom-for-two-buckets)).
- `'' + x` with a string `x`, which React writes to turn a value into a string, now gives back `x` itself instead of a new copy of it, and so does the first step of a template literal that starts with `${x}`. On a linux/amd64 machine with an AMD EPYC 7B13 (16 code layouts), React pages take 2.2% less time to render and MUI pages 0.7% less.
- `o[k]` with a key that is a string given by `for in` or `Object.keys` is looked up by the number the engine already has for that property name, not through the generic property path. On a linux/amd64 machine with an AMD EPYC 7B13 (16 code layouts), renders take 0.6% (storefront pages) to 1.4% (MUI pages) less time.
- A string remembers the number of the property name it is, so `o[k]` and `hasOwnProperty.call` with a key from `for in` or `Object.keys` take it in one read. On a linux/amd64 machine with an AMD EPYC 7B13 (16 code layouts), renders take 0.8% (workbench pages) to 2.0% (MUI pages) less time than with only the keyed read of the line above. Every string takes 4 bytes more: a heap of short strings counts up to 10% more in `MemorySize`.
- An object rest, such as `const {a, ...rest} = props`, and the spread of an object into an object literal, such as `{type: x, ...props}`, now copy a source whose layout of properties they have seen before by the layout they wrote down, instead of looking its properties up one at a time. On a linux/amd64 machine with an AMD EPYC 7B13 (16 code layouts), tifa's workbench pages take 3.1% less time, the MUI pages 2.5% less and the store pages 1.3% less ([objects-and-shapes.md](docs/changes/objects-and-shapes.md#an-object-rest-is-made-at-its-final-shape)).
- Bytecode is version 12, because an object rest lists the keys it leaves out (`OP_copy_rest`): bytecode written by v0.30.4 or an earlier release no longer loads (`SyntaxError: invalid version`), and has to be compiled again.

### Fixed

- The spread of a string into an object literal gives its characters, as Node does: `{..."ab"}` is `{0: "a", 1: "b"}`, where it was `{}`. A number, a boolean, a symbol or a bigint still gives no key. This is upstream QuickJS's fix b19b2ec ([upstream-commits.md](docs/changes/upstream-commits.md)). The copy of a spread, a rest or `Object.assign` now reads the interrupt flag every 4,096 keys, so a timeout or `Interrupt` stops the copy of a long string or of an object of many keys before it ends ([uncatchable.md](docs/changes/uncatchable.md#the-copy-of-a-spread-a-rest-and-objectassign-reads-the-interrupt-flag)).

## [v0.30.4] - 2026-11-14

### Changed

- A tag in a list of locales longer than 16,384 characters is now a `RangeError`, as a tag given alone has been since v0.30.3. Chrome and Node accept it. Go put each tag of a list in canonical form whole, with no check of the call: 16 tags of 256 Ki characters ran 60 ms past an eval timeout on an Apple M4 ([uncatchable.md](docs/changes/uncatchable.md#long-text-and-lists-of-locales-are-worked-on-in-checked-pieces)).
- An output of `fmt.sprint` or `fmt.sprintf` longer than the longest string of the engine now ends the call with the uncatchable out of memory error, where it was a `TypeError`. Only a VM with a memory limit above 16 GiB can ask for such an output ([uncatchable.md](docs/changes/uncatchable.md#a-go-function-takes-the-long-arrays-of-a-call-from-the-os)).
- In a VM with no memory limit, a call of Intl or `fmt` on a long text now reuses the memory of the call before it, and does not need new pages at each call: the VM keeps the last block given back, of up to 32 MiB, and the VMs of a process keep 64 MiB at most together, each block given back after 16 calls that took none, when `SetMemoryLimit` gives the VM a limit, and at `Close`. On GitHub's macos-15 runner, the first word of a text of 8 Mi characters takes 3.7 to 6.1 ms a call, where v0.30.3 took 5.2 to 8.9 ms. A VM with a memory limit keeps no block, so that its limit holds: there the same call takes 31 to 34 ms on that runner, and 4.1 ms on an Apple M4 where v0.30.3 took 3.6 ms ([uncatchable.md](docs/changes/uncatchable.md#a-vm-keeps-its-last-block-for-the-next-call)).

### Fixed

- `Interrupt` and the eval timeout now stop `Intl.Segmenter`, `Intl.ListFormat`, `toLocaleUpperCase` and `toLocaleLowerCase` in Turkish, Azeri, Lithuanian and Greek, and `Intl.Collator` and `localeCompare` on long text: the text is worked on a piece at a time, with a check of the call between two pieces, and the room for what Go keeps of it is counted against the memory limit before Go allocates it. Before, the work around the checked runs read no interrupt flag. On an Apple M4, a Segmenter of 8 Mi characters ran 19 to 40 ms past the timeout, a ListFormat of one item of 8 Mi characters about 50 ms, Greek in upper case of 8 Mi characters up to 0.75 s, and a compare of two strings of 4 Mi characters 18 to 36 ms. On the Intel Mac of CI, a Segmenter with a timeout of 100 ms stopped after 215 to 310 ms. The answers are the same ([uncatchable.md](docs/changes/uncatchable.md#long-text-and-lists-of-locales-are-worked-on-in-checked-pieces)).
- When the output of `fmt.sprintf`, `printf`, `sprint` or `println` may pass 64 KiB, it is now made a piece at a time, and `Interrupt` and the eval timeout can end the call between two pieces. Go's fmt made it whole, with no check: on an Apple M4, `%q` of 3.2 MiB ran 22 ms, and the timeout could not stop it. The output is the same byte for byte. One step still cannot stop: the first division of the conversion of a very long BigInt to decimal, at most 5 to 7 ms on an Apple M4 ([uncatchable.md](docs/changes/uncatchable.md#the-functions-of-fmt-make-a-long-output-in-checked-pieces)).
- `Interrupt` and the eval timeout now stop a call of Intl or `fmt` that copies a long text or makes a long output while the OS hands it the memory. Go cleared each array the length of the input in one go, with no check, and when Go had given those pages back to the OS, the OS handed each back again while Go cleared it: on GitHub's macos-15 runner, a virtual Apple M1 under Rosetta, about 30 ms for 16 MiB. There the first word of a text of 8 Mi characters, a call of 6.4 ms, stopped 27.1 ms after a timeout of 4.8 ms. Now such an array of 1 MiB or more is mapped from the OS, and its pages come in the loop that fills it, which checks the call: on the same runner, 148 sweeps of that call at every timeout passed, and the call takes 27 to 106 ms whole, half the time under 32 ms, the cost of the pages the OS now hands over at each call. That is the cost in a VM with a memory limit. A VM with none keeps its last block, see Changed. `fmt` also checks the call in a long format and in a long verb, which ran up to 79 ms and 260 ms with no check on an Apple M4. Three stretches still cannot stop, and are next: `format` of a ListFormat of 8 Mi items, up to 574 ms, `toLocaleUpperCase('tr')` of 8 Mi lone surrogates, up to 751 ms, which gets the VM killed, and a Segmenter on a run of 11 Mi characters of Chinese, up to 235 ms, each on one core of an AMD EPYC ([uncatchable.md](docs/changes/uncatchable.md#a-go-function-takes-the-long-arrays-of-a-call-from-the-os)).
- A long string that a Go function of Intl or `fmt` reads in steps is now written through a buffer of 48 KiB of its own, where it took one of 768 KiB from a pool that Go may empty at any collection: a compare of two long strings could make Go allocate 1.5 MB in buffers alone. The strings read are the same ([url.md](docs/changes/url.md)).
- The line of v0.30.3 on `Intl.Segmenter` said that `containing(0)` on 8 Mi characters of Chinese stops within half a millisecond of a timeout of 100 ms. Measured again on an Apple M4, it stops 1 to 9.5 ms after that timeout. A stop that comes while the dictionary makes its arrays the length of the run waits for them: an interrupt there ends the call 87 to 107 ms later. That stretch is the third of those above ([uncatchable.md](docs/changes/uncatchable.md#a-go-function-takes-the-long-arrays-of-a-call-from-the-os)).

## [v0.30.3] - 2026-11-13

### Added

- `Args.StringLen(i)` returns the length of a string argument of a Builtin, in UTF-16 code units, without converting it or joining a string made by `+`, so a Builtin can refuse a string too long for it before it reads any of it. It takes the same time for a string of any length and allocates nothing ([Go API](docs/changes/go-api.md#argsstringlen-reads-the-length-of-a-string-in-place)).
- `fmt.Register` takes options, and `fmt.Writer(w)` makes `printf` and `println` write to `w` in place of `os.Stdout` ([uncatchable.md](docs/changes/uncatchable.md#fmt-bounds-a-call-with-no-memory-limit-and-writes-in-pieces)).
- Package `url`: `url.Register(vm)` gives a VM `URL` and `URLSearchParams`, the whole standard, with the parsing done in Go (`internal/whatwgurl`, which passes the URL data of web-platform-tests but for 7 cases of IdnaTestV2 that need a Unicode newer than 17). A script that takes a URL with `new URL(path, base)`, as TanStack Router does for each link, takes about 4.7 µs (with a write of `pathname` and the reads of 7 getters) where core-js's polyfill takes 42 to 87 µs. On tifa's pages a render takes 33% less time in the store pages, 14% less in the workbench pages and the same in the MUI pages, and the HTML is the same byte for byte. core-js leaves a native URL that passes its checks alone, so a host registers the package before the bundle and changes nothing else. A string a script gives it is at most 65,536 UTF-16 units: a longer input, base or href is not a URL (so neither is a `data:` URL over 64 KiB, which browsers take), the host of a special scheme such as `http:` that is not ASCII is not a host past 1,024 units, as given or once IDNA maps it (the host of another scheme does not go through IDNA and has no limit but the href's), and a name or a value of a query string, or its encoding, past the limit is a `RangeError`. Within that, a call of Go takes a few milliseconds, and the eval timeout, `Interrupt` and the memory limit hold: a URL is an object of the VM, and Go checks the room for what it takes before it reads a string ([url.md](docs/changes/url.md#url-and-urlsearchparams-on-a-go-parser)).

### Changed

- A closure that captures a variable no code changes once the closure has it, such as a parameter, a `const` or a function declaration, now holds the value itself, where it held a reference shared with the frame of the function. Such closures no longer keep a suspended generator or async function alive, so an object that only its frame holds can be freed earlier: a `WeakRef` to it can empty, and a `FinalizationRegistry` callback for it can run, sooner than before ([c-engine.md](docs/changes/c-engine.md#captures-that-never-change)).
- Bytecode is version 11, because it marks the closure variables that hold their value, which a read takes as they are once it has checked them ([c-engine.md](docs/changes/c-engine.md#captures-that-never-change)): bytecode written by v0.30.2 or an earlier release no longer loads (`SyntaxError: invalid version`), and has to be compiled again.
- A call whose last arguments are local variables, arguments or `this` copies them itself, where each took an instruction of its own. A render of tifa's shop takes 0.9% to 1.3% less time, and React's page 0.8% less, on an Apple M4 ([c-engine.md](docs/changes/c-engine.md#a-call-copies-its-last-locals-into-its-window)).
- On arm64 a generated function that calls no longer saves and loads LR itself, which Go's prologue and epilogue already do. A render runs 0.75% to 1.14% fewer instructions and takes 0.4% to 1.2% less time on an Apple M4, inside the spread of the rounds. Frames and the call depth are the same, and amd64 does not change ([c-engine.md](docs/changes/c-engine.md#lr-is-saved-once-on-arm64)).
- Destructuring a plain array, such as `const [a, b] = arr`, no longer makes an Array Iterator. It no longer polls in a call of `values` and takes no level of the call depth, and a script that makes no iterator otherwise holds one shape less. Under a memory limit, out of memory can come later, where an iterator is made. A render of tifa's pages runs 0.26% (MUI) to 0.84% (the workbench) fewer instructions ([c-engine.md](docs/changes/c-engine.md#array-destructuring-with-no-iterator)).
- `Intl` and `Temporal` now refuse with a `RangeError` some inputs Chrome and Node accept: a locale tag given alone and the options `calendar`, `numberingSystem` and `collation` longer than 16,384 characters, a numeric string longer than 524,288, and a number `Intl.NumberFormat` would write with more than 525,312 digits. Every other option, code and string of Temporal past 16,384 characters is refused too. Go read each of these whole, outside the VM's memory limit and with no check of the interrupt flag: a private use tag of 16 MiB took 0.2 seconds and 289 MiB of Go, and `'1e-1073741000'` to three significant digits made Go write a thousand million zeros. Under the caps, what Go may take for an argument is counted against the memory limit, and the call ends with the out of memory error when it does not fit. Each input Chrome accepts is listed in [uncatchable.md](docs/changes/uncatchable.md#a-short-argument-of-intl-and-temporal-is-measured-before-it-is-read).
- In a VM with no memory limit, `fmt.sprintf`, `printf`, `sprint` and `println` now end the call with the uncatchable out of memory error when what their output may make Go allocate does not fit in room for an output of 16 MiB. Go made the whole output first with no check of the VM: `fmt.sprintf('%[1]q'.repeat(64), s)` of 1 MiB of control characters made 256 MiB and ran 636 ms past an eval timeout of 20 ms. The bound can be far above the output, so a call that writes less than 16 MiB can be refused: `"%[x"` a million times writes 13 MB with a bound of 67 MB ([uncatchable.md](docs/changes/uncatchable.md#fmt-bounds-a-call-with-no-memory-limit-and-writes-in-pieces)).
- `fmt.printf` and `fmt.println` now write 64 KiB at a time, and `Interrupt` and the eval timeout end the call between two writes. A write of 1 MiB to a pipe held the call until the pipe was read. One write that blocks still holds the call until it is done ([uncatchable.md](docs/changes/uncatchable.md#fmt-bounds-a-call-with-no-memory-limit-and-writes-in-pieces)).

### Fixed

- `EvalBytecode` and `EvalSharedBytecode` now refuse bytecode in which a catch offset is stored, returned or passed to a call, or a jump goes before the start of its function, with `SyntaxError: invalid bytecode: code no compiler makes`. In bytecode from a file, a `finally` that changes what lies under its return address now throws `InternalError: invalid ret value` at its end. No compiler makes such code: when an exception passed such an offset, the engine jumped to the place the number named in the code of the function where the exception was ([c-engine.md](docs/changes/c-engine.md#catch-offsets-and-jumps-stay-in-their-function)).
- A timeout or `Interrupt` now ends a call while a Go function converts a very long string or a very large result, and the VM is no longer killed for it. `Args.String`, `Args.WTF8`, `VM.NewString`, `VM.NewStringLatin1`, `VM.NewStringUTF16`, `VM.NewAtom` and the arguments and results of a `RegisterFunc` function read the stop every 256 Ki characters or 16,384 elements: `String` and `WTF8` return ok false, the others return the interrupt. Before, a conversion of 16 Mi characters took 129 to 151 ms on amd64 with no check, past the 50 ms after which a VM is killed. One step is still whole: Go's allocation of the string a read returns, at most 52 ms for 64 Mi characters ([go-api.md](docs/changes/go-api.md#long-strings-and-results-stop-converting-at-the-limits-of-the-call)).
- `Intl.NumberFormat` with `roundingPriority: 'lessPrecision'` now writes a number far below its fraction digits as zero without first making its significant digits: `'1e-100000000'` with `maximumFractionDigits: 100` made Go allocate 190 MiB and take 62 ms on an Apple M4 before it gave `0`. The answers are the same ([uncatchable.md](docs/changes/uncatchable.md#a-short-argument-of-intl-and-temporal-is-measured-before-it-is-read)).
- `format` of a unit or currency name in long form, in scientific or engineering notation, no longer writes out every zero the exponent moves to find the plural of the name: `format('1e-1073741000')` in meters took 4.3 seconds and 3.2 GB of Go, with no check of the call. The answers are the same ([uncatchable.md](docs/changes/uncatchable.md#a-short-argument-of-intl-and-temporal-is-measured-before-it-is-read)).
- `format` and `formatToParts` of `Intl.NumberFormat` now find that a numeric string in base 2, 8 or 16 of more than 1024 bits is an infinity before they turn it into decimal digits, in time of the square of its length: `'0x' + 'f'.repeat(65536)` took 9.6 seconds of Go on an Apple M4, with no check of the call, and a string of 512 Ki characters now takes 4 ms. The answers are the same ([uncatchable.md](docs/changes/uncatchable.md#a-short-argument-of-intl-and-temporal-is-measured-before-it-is-read)).
- `Intl.ListFormat` with empty items left out now joins the literals between them in one pass, where it copied the text so far for each: 20,000 items, all empty but the two at the ends, allocated 434 MB in Go, and now 5 MB at most. `formatToParts` of a long number takes less than half the Go memory it took: 52 MB for `10n ** 315000n` in en, where it took 112 MB. The answers are the same ([uncatchable.md](docs/changes/uncatchable.md#a-short-argument-of-intl-and-temporal-is-measured-before-it-is-read)).
- `format` and `formatToParts` of `Intl.NumberFormat` now read a numeric string as `Number` does: `"1e+-5"` and `"1xe1073741825"` are NaN, where they were Infinity, `"1e--5"` is NaN, where it was zero, and `"0e1073741825"` is 0, where it was Infinity ([time-and-intl.md](docs/changes/time-and-intl.md#a-numeric-string-is-read-as-number-reads-it)).
- `Interrupt` and the eval timeout now stop `add`, `subtract`, `until`, `since`, `round` and `total` of Temporal in a calendar other than ISO 8601, which go a month or a year at a time with no check: a date of the Chinese calendar plus 190 million days ran 64 ms past an eval timeout of 20 ms. The answers are the same ([uncatchable.md](docs/changes/uncatchable.md#the-arithmetic-of-a-calendar-reads-the-interrupt-flag)).
- `Interrupt` and the eval timeout now stop `Intl.Segmenter` in a long run of Thai, Lao, Khmer, Burmese, Chinese or Japanese with no space, and `Intl.Collator` and `localeCompare` in a long run of accents, each of which is one piece for the dictionary or the order of the accents: with a timeout of 100 ms, `containing(0)` on 8 Mi characters of Chinese ran 6.6 s on an Apple M4 and ended with no error, and now stops within half a millisecond of the timeout. A run of Chinese or Japanese now ends the call with the out of memory error when the VM has no room for 288 bytes for each character NFKC makes of it. The segments and the order are the same ([uncatchable.md](docs/changes/uncatchable.md#a-run-no-builtin-can-cut-reads-the-interrupt-flag)).

## [v0.30.2] - 2026-11-13

### Fixed

- `new C(...args)` on an object that is not a function, such as `{}`, now throws `TypeError: not a constructor`, as `new C(a, b)` does and Node says. It threw `TypeError: not a function` ([errors-and-stacks.md](docs/changes/errors-and-stacks.md#new-with-a-spread-says-when-its-callee-is-not-a-constructor)).
- `format` and `formatToParts` of `Intl.NumberFormat`, given a string made by `+` that the VM has no memory to join, now end the call with the uncatchable out of memory error. They panicked with `intl: unexpected number bridge value`, and the panic went on in the Go code that called `Eval`. The other Builtins of `intl` and `temporal` now stop at such an argument too, where they took it for the empty string and went on until the call ended ([uncatchable.md](docs/changes/uncatchable.md#a-string-argument-intl-cannot-read-ends-the-call)).
- `Interrupt` and the eval timeout now stop `Intl.getCanonicalLocales`, `supportedLocalesOf`, the Intl constructors and `toLocaleString` given a list of locales, and `Intl.ListFormat`, however many items the list has. One long tag or item is still worked on in Go with no check. Their work in Go read no interrupt flag, so a long list held off the stop until Go was done. `getCanonicalLocales` also compared each tag with all those before it to find duplicates, 8.5 seconds on 100,000 distinct tags, and now takes 0.32 seconds. With a timeout of 20 ms, each of these calls on a list of about a million tags or items now stops within 60 ms, where the Go work ran 0.2 to 3.5 seconds. Every answer is the same ([uncatchable.md](docs/changes/uncatchable.md#intl-reads-the-interrupt-flag-in-loops-over-a-long-list)).
- `containing` and the iterator of `Intl.Segmenter` now find segments as javascript reads them, a batch at a time, where the first call found every segment of the whole text in Go, outside the memory limit of the VM and with no check of the interrupt flag. `containing(0)` on a text of 8 MiB took 1,260 MiB of allocations in Go and up to 1.5 seconds, and now takes 24 MiB and 17 to 36 ms. Every segment is the same, and reading every segment of a text takes as long as before ([time-and-intl.md](docs/changes/time-and-intl.md#intlsegmenter-finds-segments-as-they-are-read)).
- The formats Intl keeps for Temporal values, shared by every VM of the process, no longer keep what a page asked for at any length after its VM is closed. `toLocaleString` cached each request as the page gave it, a failed one too, and a Temporal date format kept its whole request in the formatter registry: 40 VMs closed after a failed calendar of 1 MiB kept 81 MiB. Now only a request of at most 512 bytes that resolved is cached, and a date format keeps the SHA-256 of its request. The 40 VMs now keep less than 1 MiB, and the formats pages ask for are cached as before ([time-and-intl.md](docs/changes/time-and-intl.md#the-caches-of-temporal-formats-keep-short-requests-only)).
- A number option of Intl such as `fractionalSecondDigits` or `minimumIntegerDigits` is now checked against its range before it is rounded down, as ECMA-402 says, so `fractionalSecondDigits: 3.9` is a RangeError, as in Chromium, where it was taken as 3 ([time-and-intl.md](docs/changes/time-and-intl.md#number-options-are-checked-before-they-are-rounded-down)).
- `fmt.sprintf`, `printf`, `sprint` and `println` of package `fmt` now end the call with the uncatchable out of memory error, before Go makes the output, when it may not fit in what the VM has left under its memory limit. Go makes the whole output first, and a short format can ask for a long one: `fmt.sprintf("%1000000[1]s".repeat(256), "")` made Go allocate 1,210 MiB in a VM limited to 16 MiB. A call whose budget fits runs as before. The budget comes from the format and the arguments and can be far above the output: `fmt.sprintf("%.1s", s)` of a string of 1 MiB writes one character and is refused with 2 MiB left. A write of `printf` or `println` that blocks on stdout still holds the call until it is done, as the doc of `Register` now says ([uncatchable.md](docs/changes/uncatchable.md#the-functions-of-fmt-end-the-call-when-their-output-does-not-fit)).
- `fmt.sprintf` and `printf` now count what `%w` writes of its argument, each word of a big int padded to the width, and a string quoted for `%#w`: `fmt.sprintf("%1000000w", 1n << 16384n)` made Go allocate 1,459 MiB in a VM limited to 16 MiB, and now ends with the out of memory error. Each function now checks what its output may make Go's fmt allocate, 16 times the bound on it and a few KiB for the printer that makes it, against what the VM has left. The bound counts a precision only where Go's fmt pads to it, so `fmt.sprintf("%.1000000t", true)` writes `true` in a VM with little room. A format with many a `[` and no `]` after them, such as `"%[x".repeat(1000000)`, which took Go minutes, now makes the same output in 34 ms ([uncatchable.md](docs/changes/uncatchable.md#the-functions-of-fmt-end-the-call-when-their-output-does-not-fit)).
- An Intl constructor, `supportedLocalesOf` and `toLocaleString` now resolve a long locale tag in time linear in its length, where go-intl looked up every part of the tag as it cut it short, in time that grew with the square of the length: a private use tag of 1 MiB took 6.8 seconds, with no check of the interrupt flag, and now takes 33 ms. Every answer is the same ([time-and-intl.md](docs/changes/time-and-intl.md#a-long-locale-tag-resolves-in-time-linear-in-its-length)).
- `Interrupt` and the eval timeout now stop a long operation on BigInts within a few polls, where they killed the VM. The product, the division, the power and the conversion to and from a string of BigInts of up to a million bits take a time of the square of their length, and none of their loops read the interrupt flag: `String(10n ** 300000n)` runs about 52,000 polls of the engine (1.9 s on an Apple M), so with a 20 ms timeout the call ended 70 ms after it started with `ErrKilled`, and the VM could only be closed. Now these loops read the flag every 32,768 limbs or digits, and such a call stops within 0.25 ms of its deadline with the interrupt. A power whose product is too large no longer leaks a block, and `==` and `<` of a BigInt and a string no longer drop an error of the conversion of the string. The results are the same ([uncatchable.md](docs/changes/uncatchable.md#long-operations-on-bigints-read-the-interrupt-flag)).
- `Interrupt` and the eval timeout now stop the conversion of a long string to a number within a few polls. `Number`, unary `+`, `parseInt`, `parseFloat`, `BigInt`, a comparison of a BigInt with a string, `JSON.parse` of a number or of spaces and `eval` of a number literal went over the characters of the string several times without reading the interrupt flag: on a string of 128 Mi characters each took a few hundred milliseconds, and with a 20 ms timeout the VM was killed. These loops now read the flag every 65,536 characters. The results are the same ([uncatchable.md](docs/changes/uncatchable.md#turning-a-long-string-into-a-number-reads-the-interrupt-flag)).
- `Interrupt` and the eval timeout now stop the parse of a long source within a few polls when they come in a loop that goes over a long token or a long stretch of the source. The parser of `eval`, `Function` and `JSON.parse` went past a long hashbang line, long runs of spaces, newlines and comments, and over long identifiers, strings, templates, regexps and escapes `\u{...}`, their atoms, their lines and columns and the messages that name them, without reading the interrupt flag: one `eval` of an identifier of 64 Mi characters took 436 ms, and with a 20 ms timeout the VM was killed. These loops now read the flag every 65,536 bytes or less. The tokens, values and lines are the same, and so is the text of `toString` of a function. A `ReferenceError` for a name of 65,536 characters or more now quotes its first 58 characters, where it quoted 254 ([uncatchable.md](docs/changes/uncatchable.md#parsing-a-long-source-reads-the-interrupt-flag)).
- `Interrupt` and the eval timeout now stop a compare or a copy of a long string within a few polls. `<` of two long strings, their sort without a comparator, `Symbol.for` of a long key, the copy of a long string that `join`, `padEnd`, `replace`, `String.raw` and many others make, the `$` of a long replacement, a long flags string of a regexp, `String.raw` of a long `raw.length`, `RegExp.prototype[Symbol.replace]` of many matches and the sort of the long export names of a module that a namespace import makes went over all of it without reading the interrupt flag: with strings of 64 Mi characters, a stop asked during `s < s2` came 34 ms late, 5,610 polls after the request. These now read the flag every 65,536 characters, or every MiB of a copy of bytes, and a call whose copy was stopped leaves at once. A replacement with many `$<` and no `>` after them no longer takes a time of the square of its length. The results are the same ([uncatchable.md](docs/changes/uncatchable.md#comparing-and-copying-long-strings-reads-the-interrupt-flag)).
- `Interrupt` and the eval timeout now stop `ArrayBuffer.prototype.resize` and `transfer` of a large buffer within a few polls. When the heap could not resize the block of the buffer where it was, it copied the whole block in one go, and both then cleared the bytes the buffer gains in one go too, without reading the interrupt flag: `resize` of an `ArrayBuffer` of 256 MiB to a quarter and back stopped only after its clear of 192 MiB, up to 767 polls after the request. Now both read the flag every MiB of the copy and of the clear, and a stop leaves the buffer at its old length with its bytes and ends the call with the interrupt, not with the out of memory error. The results are the same ([uncatchable.md](docs/changes/uncatchable.md#resizing-a-large-arraybuffer-reads-the-interrupt-flag)).
- `String.prototype.normalize` that runs out of memory now ends the call with the uncatchable out of memory error and frees its buffer. It failed with no exception, which the Go API could not report as running out of memory, and the runtime kept the buffer for good. A Map or Set whose hash table cannot grow now goes on with the table it has. It left the out of memory error pending under a `set` or an `add` that succeeded, which failed the next call, and a new record has its value before a collection can look at it ([uncatchable.md](docs/changes/uncatchable.md#out-of-memory-in-normalize-and-in-the-growth-of-a-map)).
- A loop with no call in it now ends with the out of memory error, or the panic of a Go function, that a built-in left pending. The resolve function of a promise given a thenable whose `then` getter ran out of memory, an async function or async generator that failed before its first `await`, and `import()` of a specifier that failed to convert record the error and return, and the script went on: `for (;;) {}` after one of them ran for good when no timeout was set, though the memory limit had been hit. Now the engine keeps the interrupt flag set while such an error is pending, and the loop stops at its next backward jump ([uncatchable.md](docs/changes/uncatchable.md#a-loop-with-no-call-ends-at-an-uncatchable-exception-left-pending)).
- `Interrupt` and the eval timeout now stop the compile of a regexp with a long string of flags within a few polls. `new RegExp('a', f)`, and a literal followed by many flags in `eval`, counted the characters of the whole string of flags without reading the interrupt flag before it threw its `SyntaxError`: 16 Mi flags ran 140 polls past a stop on amd64, and a longer string could reach the kill. A string of more than 8 flags is now refused at once, with the same `SyntaxError` ([uncatchable.md](docs/changes/uncatchable.md#the-flags-of-a-regexp-are-checked-without-a-pass-over-a-long-string)).
- `WeakRef` now keeps its target alive to the end of the job, as the specification and Node do. `new WeakRef({}).deref()`, or `deref` after the last other reference to the target was dropped, gave `undefined` in the same job, where it now gives the object. The targets are let go when the job ends: when a call of the Go API (`Eval`, `Call`, `ExecutePendingJobs` and the others) returns to Go, and between the promise jobs it runs. A Go function that javascript calls is inside the job. So a VM used for many renders keeps nothing from one call to the next ([weak-collections.md](docs/changes/weak-collections.md#a-weakref-keeps-its-target-to-the-end-of-the-job)). A loop that waits within one call for `deref` to give `undefined` no longer ends: the target is let go when the call returns to Go or the job ends, so ask for a collection (`RunGC`) between two calls and read `deref` in the next one.
- A symbol made by `Symbol(s)` of a string that an array, a variable or a string literal of digits in a function still holds, or whose `description` the script keeps, is now freed when only a `WeakRef` holds it, as in Node. The string itself became the symbol, and `description` gave the symbol back as a string, so a string the script held kept the symbol alive, and the `WeakRef` never emptied ([strings.md](docs/changes/strings.md#a-symbol-has-a-string-of-its-own)).
- A store in strict code to a proxy with no `set` trap whose `defineProperty` trap returns false, such as `p.x = 1`, now throws a `TypeError`, as the specification says and Node does, whether the target has the property already or not. It went on with no error. A store that throws in any code, as those of `Object.assign` and `Array.prototype.fill`, and `SetProperty` and `SetPropertyValue`, now also throws or returns the error when the target already has the property, where it went on. Sloppy code and `Reflect.set` are unchanged ([objects-and-shapes.md](docs/changes/objects-and-shapes.md#a-proxy-that-refuses-a-store-throws-in-strict-code)).
- On windows/amd64 a memory fault in the engine, which only a bug of the engine makes, becomes its panic on every CPU without first breaking the memory of the process. Windows writes the state of the CPU at a fault below the stack pointer, 12.8 KiB on Intel with AMX, where Go keeps 4 KiB below a goroutine's stack (golang/go#81238), so the write could go past the stack into the heap. Every generated function now makes sure of 12 KiB more below its stack pointer, at least 16.9 KiB in all. Only the code of windows changes: renders there take 0 to 2.4% longer, measured with that code on linux, and a goroutine that runs a tiny script takes 32 KiB of stack in place of 8. A fault in Go code, of the host or of a program, still has only Go's 4 KiB until Go fixes it ([c-engine.md](docs/changes/c-engine.md#room-for-a-fault-on-windows)).
- `EvalBytecode`, `EvalBytecodeValue`, `EvalSharedBytecode` and `EvalSharedBytecodeValue` are now one call into javascript from the read of the bytecode to the end of the conversion of the result, inside the eval timeout. The conversion of the result ran after the call had ended, so a `toJSON` or a getter of the object the bytecode ended with, or a replaced `BigInt.prototype.toString`, ran with no timeout, and `Interrupt` could not stop it ([uncatchable.md](docs/changes/uncatchable.md#the-bytecode-calls-convert-their-result-inside-the-eval-timeout)).
- An interrupt, the eval timeout or running out of memory that stops the read of an exception the script threw (its `toString`, or a getter of its `name`, `message` or `stack`) now ends the call with that error, `Uncatchable` set. The read dropped it with what the script throws, so the call returned an `*Error` with empty fields that was not uncatchable, and when a Go function's argument conversion met such an exception, the script caught a `TypeError` and ran on to its end. `ErrorFromValue` returns the interrupted or out of memory error in that case, where it left the fields the timeout cut off empty ([uncatchable.md](docs/changes/uncatchable.md#an-uncatchable-exception-met-while-an-exception-is-read-ends-the-call)).
- Every method that takes a `Value` now refuses one of another VM before it reaches the engine: `SetPropertyValue` (its value and its receiver), the receivers of `SetProperty`, `GetPropertyValue` and `GetProperty`, the `this` of `EvalThis` and `EvalThisValue`, and a `Value` that a Go function of `RegisterFunc` returns, alone or among several results or in a slice or a map, which throws a `TypeError`. `SetPropertyValue` stored such a value in the object, which then pointed into the memory of the other VM, unmapped once that VM was closed. The refused `Value` is not freed. `Call` and `Value.Call` now refuse a number of another VM too, as `NewArray` and a `Builtin` did ([Go API](docs/changes/go-api.md#a-value-of-another-vm-is-refused-at-every-entry)).
- `NewString`, `NewStringLatin1`, `NewStringUTF16` and `NewObjectValue` now return the out of memory error when the VM runs out of memory. They returned the exception marker of the engine as a `Value` with a nil error, and left the error pending for the next call to find ([Go API](docs/changes/go-api.md#the-constructors-of-values-report-running-out-of-memory)).
- A Go function of `RegisterFunc` or `RegisterHostFunc` whose results fail to convert no longer leaks: the values made for the results before the one that failed, and every `Value` of the VM the results hold, are freed, whether the conversion fails part way, refuses the result (an array, a pointer to a pointer, a `Value` as a map key), meets an `Error` method that panics, or a host function returns an error with its value. A string of 4 KiB before a channel stayed in the VM after every call, for good ([Go API](docs/changes/go-api.md#a-result-that-fails-to-convert-leaves-nothing-behind)).
- `UndefinedValue` passed as an argument of `Call` and the other call methods, or as the value of `SetPropertyValue`, is `undefined`, where it panicked with a nil pointer. The zero `Value` that a Go function of `RegisterFunc` returns is `undefined` too, where it was 0, and so is the zero `Value` as the `this` of `Value.Call`, as it is for `NewArray` and a `Builtin` ([Go API](docs/changes/go-api.md#undefinedvalue-as-an-argument-and-a-property-value)).
- `Close` of a VM while a call into it is in progress, as from a Go function that javascript on it called, now returns an error and changes nothing, and the VM goes on. It freed the engine that was still running below the Go function, and the call then panicked with a nil pointer ([Go API](docs/changes/go-api.md#close-inside-a-call-into-the-vm-is-refused)).
- `Value.Then` returns what a getter of `then` threw, where it returned `TypeError: not a function` ([Go API](docs/changes/go-api.md#then-returns-the-exception-of-a-then-getter)).
- A Go map that a Go function returns becomes an object whose keys are own data properties, each key whole. A key was cut at its first NUL byte, so `"a"` and `"a\x00b"` became one, the key `__proto__` changed the prototype of the object, and a setter on `Object.prototype` under the name of a key ran with its value ([Go API](docs/changes/go-api.md#a-go-map-becomes-an-object-of-its-own-data-properties)).
- A `Value` that a Go function returns as an `any`, from `RegisterFunc` or `RegisterHostFunc`, is now taken as a `Value`: one of the VM hands its reference to the call, one of another VM throws a `TypeError`, and `UndefinedValue` and the zero `Value` are `undefined`. It went through JSON, which panicked on `UndefinedValue` and kept the reference of every `Value` of the VM in it for good. A host function that returns an error with a value now frees the `Value`s in the value ([Go API](docs/changes/go-api.md#a-value-returned-as-an-any-is-taken-as-a-value)).
- A `Value` that the results of a Go function reach twice through the same place, such as one slice of `Value`s returned as `[]any{s, s}`, one map returned twice, under any name of its type, or two slices of one array that overlap, is one reference, freed once. Each time the conversion met it, it took the reference again, so the object was freed while the script still used it. A `Value` and its `Dup` are two references, a copy of a `Value` is not one ([Go API](docs/changes/go-api.md#a-place-a-result-reaches-twice-is-one-reference)).
- A result of a Go function that contains itself, such as a slice `s` with `s[0] = s`, a map that holds itself or a pointer to itself, throws a `TypeError`. The conversion recursed until Go's stack limit, `fatal error: stack overflow`, which kills the process ([Go API](docs/changes/go-api.md#a-result-that-contains-itself-is-an-error)).
- The references that the results of a Go function hand over, and their slices, maps and pointers that can hold a `Value`, are now read before any code of the program runs. Code of the program that the conversion runs, an `Error` method, a `MarshalJSON` or the `toJSON` of a `Value` in a struct, could change the result while it converted: a slice made to hold itself killed the process with a stack overflow, a `Value` of another VM put in had its count changed in a VM that does not own it, and a `Value` taken out stayed in the VM for good. The data with no `Value` in it is a leaf that the walk keeps, and what it points to, such as the elements of an `[]int`, is read when it converts. A value whose type has an `Error` method converts to that string as before, and the `Value`s in it stay the caller's, as those in a struct do ([Go API](docs/changes/go-api.md#the-references-of-a-result-are-taken-before-code-of-the-program-runs)).
- `VM.NewArray` in a Go function that javascript called now ends at `Interrupt`, the eval timeout or an uncatchable error of the call, with that error, where it made the whole array first: an array of 4 Mi elements took 0.14 to 0.2 seconds with no check, which could go past the grace after which a script is killed. `toLocaleUpperCase('tr')` of 2M lone surrogates, each before an `i`, got its VM killed by a timeout that came while the array was made, and now ends with the interrupt ([go-api.md](docs/changes/go-api.md#newarray-stops-at-the-limits-of-the-call)).

### Changed

- JavaScript now runs in a register interpreter. Each function is converted from its bytecode at its first call, and the stack interpreter of QuickJS is gone. A first reading on linux/amd64 puts a render of tifa's pages at 0.70 of the time on the React page, 0.86 on the shop's storefront, 0.89 on its workbench and 0.86 on MUI, against the stack interpreter (an AMD EPYC 7B13 server with 4 cores, warm renders of one VM on one core, three rounds). On darwin/arm64 an Apple M4, measured the same way, reads 0.77, 0.80, 0.83 and 0.83 for the same pages.
- A render of React DOM from a bundle that tifa's build has not rewritten takes about 9.5% less time: the string cases of a `switch` that end with any conditional jump are one opcode, and a chain of them runs in one dispatch ([c-engine.md](docs/changes/c-engine.md#a-read-of-a-local-and-its-get_field-and-the-case-of-a-switch-on-a-string)). Bytecode is version 10, for these opcodes, because a function's bytecode now carries, for each of its regexps, where it starts to look for a match, worked out once when it is compiled, which makes a render of tifa's pages run 0.27% to 0.97% fewer instructions ([regexp.md](docs/changes/regexp.md#where-a-regexp-starts-is-worked-out-once)), and because it carries, for a regexp with a long list of alternatives that start with a literal, which of them start with each character, which makes a render of the shop's workbench run 1.36% fewer ([regexp.md](docs/changes/regexp.md#long-lists-of-alternatives-that-start-with-a-literal)): bytecode written by an earlier release no longer loads (`SyntaxError: invalid version`), and has to be compiled again.
- A tagged template with more than 65,535 substitutions now throws `RangeError` when it is called, where it failed to compile with `InternalError: stack overflow`. The arguments of a call past the first 32 now go in an array, as for a call with a spread ([c-engine.md](docs/changes/c-engine.md#calls-of-more-than-32-arguments-pass-them-in-an-array)).
- A function fails to compile when it has more than 32,765 parameters and local variables together (`InternalError: too many local variables` or `too many arguments`, where the limit was 65,534 of each), or when its stack is deeper than 118 values and its `try` blocks and `for-of` loops nest too far around it (`InternalError: stack overflow`: 118 nested `try` or 39 nested `for-of` around `r += a`, or 49 and 17 around an expression 130 deep). The register loop is the only interpreter now, and these are the functions it cannot run ([c-engine.md](docs/changes/c-engine.md#the-limits-of-a-function-in-the-register-loop)).
- `EvalBytecode`, `EvalBytecodeValue` and the first load of a `SharedBytecode` now refuse bytecode with code no compiler of this version makes, among them a function past the limits above, with `SyntaxError: invalid bytecode: code no compiler makes, at N`, where such code would have run. Bytecode this version writes is read as before.
- A VM holds more memory: the register code of each function that ran, 748 bytes a function on average and at most about 33.8 KB, and a register stack of 31.7 KB that grows in segments. A VM that rendered tifa's shop pages holds 15.86 MB where it held 13.34 MB, the React page 932 KB where it held 776 KB (`MemorySize` of one VM after it rendered each page once and ran a full GC, linux/amd64).
- A recursion with large frames now reaches the call depth limit: one through a constructor of 2,000 local variables now reaches 9,996 levels, where the stack ended it at 1,982 on arm64 and 1,928 on amd64. No recursion stops sooner than before.
- `GCThreshold` and the GC the engine runs count the memory a GC can free from: `MemorySize` without the register stack. After a full GC the threshold is 1.5 times that memory. `MemorySize` and the memory limit still count the stack.
- A call that a script makes of `f.call(t, ...)`, of `call.bind(f)` (as core-js makes it), or of a bound function with no bound arguments is now made as the call it stands for. The stack of an `Error` made in `f` has no `at call (native)` line for it, as in Node 24, a recursion through `f.call` goes about twice as deep before `RangeError`, and a chain of 100,000 bound functions works where it threw `RangeError` ([errors-and-stacks.md](docs/changes/errors-and-stacks.md#calls-through-call-and-bound-functions-leave-no-line-in-the-stack)).
- A `for-of` over an array that is not changed in ways that matter for iteration no longer calls the array iterator's `next` for each element. So at the call depth limit such a loop reads on where the call to `next` threw `RangeError` before. Scripts never stop sooner than before.
- When a loop that does nothing but call `charCodeAt` on a string is stopped (a timeout or `Interrupt`), the stop is reported at the jump back of the loop, not inside the call.

## [v0.30.1] - 2026-11-06

### Fixed

- `Interrupt` and the eval timeout now stop a call while a long string made by `+` (a rope) is being made flat. The copy of each of its strings read no interrupt flag, so a string of tens of millions of characters made by `+` held off the stop for the whole copy, the first time a built-in, a key or the tag functions needed its characters: with a 10 ms timeout, a style value of 64M characters stopped 68 ms after the call started on CI. Now the copy reads the flag every 65,536 characters, and that call stops at once. The string made is the same, byte for byte ([uncatchable.md](docs/changes/uncatchable.md#a-long-rope-is-made-flat-in-steps)).
- `String.prototype.repeat` now reads the interrupt flag every 65,536 characters, where it read it every 1,048,576 and only between two whole copies of its string, and a call of fewer than 1,048,576 characters read it never. `'x'.repeat(1 << 20)` ran 105 polls of the engine (each 10,000 ticks) with no reading, and a loop of `'x'.repeat(1e5)` stopped only at the next call. The result is the same ([uncatchable.md](docs/changes/uncatchable.md#repeat-reads-the-interrupt-flag-every-65536-characters)).
- `Interrupt` and the eval timeout now end the compile of one long regexp with the interrupt, where they could kill the VM. Four stretches of the compile read no interrupt flag: the conversion of the pattern to UTF-8, the copies of the code as it grows, the scan of the code for its registers and the copy of the code into its string. For a pattern of 13.2M characters the scan alone ran about 1,360 polls of the engine (each 10,000 ticks), past the 1000 after which the Go host kills the script, so the call ended with `ErrKilled` and the VM could only be closed. The long digits of a count, of `\u{…}` and of a group name, the operands of `&&` and `--`, the strings of `\q{…}`, the compare of one of them with the strings of the same hash before it, and a lookup of a long pattern in the regexp cache read no flag either. Now each reads it in steps, and that compile stops within a few polls. A regexp compiles to the same code, byte for byte ([uncatchable.md](docs/changes/uncatchable.md#compiling-one-long-regexp-reads-the-interrupt-flag-in-every-long-loop)).
- A regexp class of many characters or many strings now stops within a few polls of `Interrupt` or the eval timeout, and compiles much faster. Each character of a class was merged into the ranges before it by making them all anew, with no interrupt check, so a class of 60,000 characters that are not next to each other took 557,426 polls of the engine (each 10,000 ticks) to compile. Now a class in order takes one step for each character and one in any order is sorted once: that class compiles in 51 to 137 polls. The strings of `\q{…}` were sorted, moved when their table grew, and freed after a stop, each in one pass with no interrupt check (up to 146 polls for 262,144 strings), and the walk of an empty table read none either. All of these now check in steps. A regexp compiles to the same code, byte for byte ([uncatchable.md](docs/changes/uncatchable.md#a-class-of-a-regexp-reads-the-interrupt-flag-however-many-characters-or-strings-it-has)).

## [v0.30.0] - 2026-11-05

### Changed

- **Breaking:** the module is `github.com/liustack/quickjs` again, its name up to v0.27.3, and the root package is named `quickjs` (from v0.28.0 to v0.29.11 it was `github.com/liustack/slowjs`, package `slowjs`). The API is the same. A program changes its import paths (`github.com/liustack/slowjs/intl` becomes `github.com/liustack/quickjs/intl`, and so on), writes `quickjs.` where it wrote `slowjs.`, and passes `-tags quickjs_intl_nonames` or `quickjs_intl_nocjdict` where it passed the `slowjs_intl_` tags, and `-tags quickjs_anygo` where it passed `slowjs_anygo`. The variables of the scripts start with `QUICKJS_` (`QUICKJS_LLVM`, `QUICKJS_REMOTE` and the others), the `Symbol.for` key with which `intl` and `temporal` mark a VM names the new path (`Symbol.for('github.com/liustack/quickjs/intl')`), and the errors of the root package start with `quickjs:`. v0.30.0 is the first release under the new path, and the releases from v0.28.0 to v0.29.11 stay at `github.com/liustack/slowjs` ([upstream.md](docs/upstream.md#moving-from-githubcomliustackslowjs)).

### Fixed

- `startTag` and `endTag` of `RegisterStartTag` and `RegisterEndTag` now read the interrupt flag in every loop over a long string, so `Interrupt` and the eval timeout stop a call on a tag, a style name, a URL or a text of millions of characters within milliseconds. Four of their loops never read it: the check of the tag, the scan of a style name for a character that is not ASCII, the white space before a URL, and the copy of the finished start tag. With a 10 ms timeout, `TestTagsStop` stopped 53 to 119 ms after the call started on CI. On linux/amd64 the longest stop of a tag of 32M characters went from 19.0 ms to 5.9 ms, and of a URL after 32M of white space from 10.5 ms to 3.3 ms. A render of tifa's pages runs 0.002% to 0.018% more instructions, and its time moved by less than the 1% the measurement can tell. The output is the same, byte for byte ([uncatchable.md](docs/changes/uncatchable.md#the-start-and-end-tags-read-the-interrupt-flag-in-every-long-loop)).

## [v0.29.11] - 2026-11-05

### Changed

- This is the last release under `github.com/liustack/slowjs`, and `go.mod` marks the module deprecated. From v0.30.0 the module is `github.com/liustack/quickjs` again, its name up to v0.27.3, with the same API: change the import path, and the package name `slowjs` to `quickjs`.
- The module builds with Go 1.26.6 or later, where it needed Go 1.27.0. The engine was checked with Go 1.26: the Go 1.26 releases before 1.26.6 are left out, as their compiler fails on a test of the module (1.26.0 and 1.26.1) and their runtime can leave a stale frame pointer after a memory fault of the engine ([upgrading.md](docs/upgrading.md#moving-to-a-newer-go)).
- A build with a Go newer than the one the engine was checked with, now Go 1.28 or later, stops with one error that names the Go it was checked with (`slowjs_is_checked_up_to_go1_27_see_docs_upgrading_md_or_build_with_tags_slowjs_anygo`). The Go assembly of the engine leans on things of Go that the Go 1 promise does not cover, and a change in one of them would show only when the program runs. Go 1.26.6 and later and every Go 1.27 release build. When a new Go comes out, slowjs is checked with it and lets it build in a patch release. `-tags slowjs_anygo` builds with any Go ([Go API](docs/changes/go-api.md#a-go-newer-than-the-one-the-engine-was-checked-with-stops-the-build)).
- Every function of the generated engine that calls something now starts with Go's stack check. The generator used to leave it out of about 450 more functions, which only stayed safe while a memory fault in the engine found 64 bytes free below them, a number read from the runtime of Go 1.27.2 that Go does not promise. Now a fault has the same room as a nil pointer in any Go program. A render takes up to 0.6% longer on the pages of tifa's example apps (0.27% to 0.36% more instructions), and the HTML is the same ([c-engine.md](docs/changes/c-engine.md#functions-without-a-stack-check)).

- A build with an option that makes the assembler encode the engine with other lengths now stops with an error, where it built and crashed in the first `NewVM`: `-asmflags=-spectre=ret` and `-spectre=all` on amd64, and `-dynlink`, which `-buildmode=plugin`, `-buildmode=shared` and `-linkshared` pass. The error points to a line of `internal/cengine/goasm_tripwire.<arch>.s`, whose comment says why ([Go API](docs/changes/go-api.md#a-build-option-that-changes-how-the-assembler-encodes-the-engine-stops-the-build)). `-buildmode=pie` and the other options build as before.

### Fixed

- A `catch` no longer takes the out of memory error when the `return` method of an iterator runs out of memory while an exception leaves its `for...of` loop or array pattern. The call ends with the error, as it does for running out of memory anywhere else ([uncatchable.md](docs/changes/uncatchable.md#out-of-memory-in-the-return-method-of-an-iterator-that-an-exception-closes)).
- A fault of the engine below the address 0x1000, as a NULL pointer in the C makes, now leaves the VM broken, so that it refuses every call but `Close`, as a fault at any other address does. Go makes such a fault the panic of a nil dereference, which names no address, and the VM was marked broken only by a panic that names one: a caller that recovered the panic could go on with a VM whose engine had stopped halfway, and in a Go function that javascript called the fault became the uncatchable error of a Go panic while the engine went on. The VM is now broken whenever a panic or a Goexit leaves a call into the engine that has not returned, whatever the panic is. A fault in the Go code of a Go function that javascript called, outside its calls into the engine, is now a panic of that function like any other, and leaves the VM usable ([C engine](docs/changes/c-engine.md#which-panics-break-a-vm)).
- After a panic leaves a call into the engine halfway, the Go API does not call into that engine again, not even to free the values a method frees on its way out (`Call`, `CallValue`, `SetProperty`, `Then`, the conversions of values), nor for a Go function that recovers the panic and goes on: such a call panics as on a broken VM, and a free does nothing. Those frees used to run on the state that a fault, a kill, an abort or a trap had left, where they could fault again, hiding the first panic, or change it further. `Close` gives the memory back ([C engine](docs/changes/c-engine.md#which-panics-break-a-vm)).

## [v0.29.10] - 2026-11-04

### Added

- `VM.RegisterStartTag(name)` and `VM.RegisterEndTag(name)` define functions on the global object that write the start tag of an element as `pushStartGenericElement` does in React DOM's server renderer (react-dom 19.3), with its attributes, style and the escaped text of a string children, and give its end tag as `endChunkForTag` does, written in C inside the engine. The start tag function takes the arguments of `pushStartGenericElement`, React's `hasOwnProperty` and a `decline` object of the caller. It pushes the whole tag onto the target as one string and returns what React's function returns, and where React's functions could write anything else (a getter, a replaced built-in, `dangerouslySetInnerHTML`, a rare attribute, a value with its own `toString`, a `javascript:` URL, and the other cases its doc lists) it returns `decline`, having changed nothing, and the script runs React's function. The engine puts nothing on the global object by itself: the functions are in a VM only when its host registers them. On the 16,310 start tags of one render of tifa's example pages they decline none, write the same text, and run about a fifth of the instructions of React's functions for a start and an end tag. With tifa's React DOM calling them, a render takes 7.3% less time on the workbench pages of its shop, 7.0% less on the storefront pages, 3.8% less on the pages of its MUI test app and 33% less on its bench pages, and a VM in tifa's pool holds about 26 KB more memory. ([Performance](docs/performance.md#react-doms-start-and-end-tags-in-c), [Go API](docs/changes/go-api.md#registerstarttag-and-registerendtag))

### Performance

- On linux/arm64 and darwin/arm64 the generated engine is made from a module the optimizer ran over a second time, after the C files were linked, as it already is on amd64. A render runs 0.56% to 0.99% fewer instructions on the pages of tifa's example apps (linux/arm64), with the same HTML on the 16 pages once their time stamps are masked. On an M4 no slowdown was seen (the sum of the pages took 0.987 of main's time, the median of 8 rounds, with 8% to 16% of noise between rounds), and the generated assembly of arm64 is 12% larger ([Performance](docs/performance.md#on-arm64)).
- Objects of the built-in classes start from the empty shape of their prototype, which the context keeps. The next shape of an object is found by comparing only its last key when it grew from the object's shape, and an object whose count drops to zero outside a collection, with nothing else to free, is freed at once. A render runs 1.84% to 2.13% fewer instructions on the pages of tifa's shop and MUI apps and 0.49% fewer on its bench pages, and the bench pages take 0.73% less time ([Performance](docs/performance.md#shapes-of-built-in-classes-shapes-found-by-their-parent-objects-that-die-alone)).

### Fixed

- `JS_RunGC` and `JS_RunYoungGC` of the C engine do nothing when they are called while the engine frees objects, from the finalizer of a class of the host or from `host_release`, and an object made there starts no collection either: the threshold is checked again when the next object is made after the free. Such a collection freed the objects the engine was freeing, the one whose finalizer ran among them, and ended the process or read freed memory. No Go program could reach it: the Go API registers no class and its `host_release` runs no code of the user. Upstream QuickJS and quickjs-ng have the same code ([C engine](docs/changes/c-engine.md#a-collection-asked-for-while-objects-are-freed)).

## [v0.29.9] - 2026-11-03

### Added

- The module builds and runs on linux/arm64, with the same generated engine as darwin/arm64. CI runs the tests on `ubuntu-24.04-arm`.
- The module builds and runs on darwin/amd64, with the same generated engine as linux/amd64. CI runs the tests under Rosetta on `macos-15`.
- The module builds and runs on windows/amd64, with the same generated engine as linux/amd64. CI runs the tests on `windows-2025`. On windows a memory fault in the engine, which only a bug of the engine makes, may break the memory of the process before it becomes a panic, on CPUs such as Intel with AMX: a bug of the Go runtime, golang/go#81238.

### Fixed

- An array made with the prototype of another class, such as `Reflect.construct(Array, [], F)` with `F.prototype` set to `Map.prototype`, throws the out of memory error when it runs out of memory as it gets its `length`. It wrote through a null pointer and ended the process. Upstream QuickJS has the same bug.
- `Object.setPrototypeOf` on an object whose shape is shared no longer keeps the new prototype alive when it runs out of memory. Upstream QuickJS has the same bug.

## [v0.29.8] - 2026-11-01

### Fixed

- `Iterator.concat` follows IteratorClose when returned, produces a fresh completed result, and stays completed after a failed step or close (QuickJS `aac9bc5`).
- `Iterator.prototype.reduce` on an empty iterator without an initial accumulator throws without reading or calling `return` (QuickJS `de4c9e0`).
- Sparse array truncation removes every eligible element before compacting the property table. Writing an old index after truncation updates the length correctly (QuickJS `3c4c6ed`).
- `JSON.stringify` of an array that runs out of memory while it makes the index of an element a string no longer leaks that element. The element, with the atoms of its keys, stayed until `VM.Close`, which then found blocks still in use. Upstream QuickJS has the same bug, quickjs-ng fixed it in `293ba08`.
- `JSON.stringify` with a long string made by concatenation as its space (a rope) indents with the first 10 characters of that string. It indented with bytes read from the inner fields of the rope, a pointer among them. Upstream QuickJS has the same bug, quickjs-ng fixed it in `35e195c`.
- `JSON.stringify` with an array of names as its replacer fails with the out of memory error when it runs out of memory while it copies those names. It went on without the name it could not copy and returned a string without that property, with the error still pending. Upstream QuickJS and quickjs-ng have the same bug.

## [v0.29.7] - 2026-11-01

### Changed

- A `WeakRef` whose target only a local that is no longer used held can give `undefined` sooner than before, since that value is now freed when the next local that shares its slot is written, not at the end of the call.

### Performance

- On linux/amd64 the generated engine is made from a module the optimizer ran over a second time, after the C files were linked, so the calls from one file to another (`memcpy`, `memset` and `memcmp`, the allocator and the heap into their callers, the collector) are inlined. A render runs 0.6% to 1.2% fewer instructions on the pages of tifa's example apps and 3.5% fewer on a run of `r_react_ssr`, with the same HTML. The time of a page stays inside the spread of the code positions (the React script and the bench pages take 0.98 of the time), the engine is 13% larger in assembly and the stack frame of the interpreter does not change. darwin/arm64 is not changed.
- Locals of a function that are never live at the same time share a slot of the frame, so a call sets and frees fewer of them: the 2021 functions of the bundle of tifa's shop have 9983 slots where they had 12191 locals, and React DOM's `pushStartInstance` 11 where it had 138. Locals that a closure captures, that `eval`, `with` or a mapped `arguments` can reach, that keep a check of the temporal dead zone, or that are live where a `catch` or `finally` starts keep a slot of their own. A render runs 1.43% fewer instructions on the shop workbench, 1.02% on the store, 0.82% on MUI and 1.81% on `r_react_ssr`, with the same HTML. The time of a render on the generated build is within the spread of the code positions (1.002, 1.000, 0.995 and 0.994 of main's). The compile of a bundle takes longer (37 M instructions for that of tifa's shop), which a VM of tifa does not pay, and a run of `r_react_ssr` that compiles its bundle and renders once takes 2.6% more time ([c-engine.md](docs/changes/c-engine.md#locals-that-share-a-slot)).
- `JSON.parse` reads a key of ASCII with no escape straight from the text, and takes its atom from a cache of 64 keys that lives for one parse and gives its atoms back at the end. A string value of ASCII with no escape is made at its length in one copy, the end of such a run is found 8 bytes at a time, an element at the end of a new array goes in without the generic path, and the text is made a C string 8 bytes at a time. A parse runs 60% fewer instructions on a table of 1000 rows (92 KB), 31% fewer on the orders of tifa's shop, 54% fewer on the 1 MB data of a large store page, and 49% and 50% fewer on npm's `package-lock.json` and a list of repositories from the GitHub API. Values, key order, errors and their positions, the reading of the interrupt flag and the nesting limit are as before. A render of the bench app's table page runs 3.91% fewer instructions and of the six workbench pages 0.31% fewer, with the same HTML on the 16 pages of tifa's apps. On linux/amd64 `JSON.parse` took 0.357 of the time on the 1000 rows, 0.685 on the orders, 0.517 on the store data and 0.629 and 0.509 on the two public texts. A render of the table page took 0.968 of the time, `/inject/1` 0.831 and `/mall/en-US` 0.951 (8 code positions, 3 rounds), and the other pages moved less than 3%, with fewer instructions or, on the spiral page, as many ([Performance](docs/performance.md#jsonparse-reads-keys-and-plain-strings-from-the-text)).

## [v0.29.6] - 2026-10-31

### Performance

- A call from the generated engine to another of its functions takes the result from the registers the callee returns it in, where it read it back from the Go stack. Calls through a pointer, calls to Go code (the host's callbacks) and calls to a function that failed to generate keep the read, and the check of the linked binary holds every call that skips it to a callee that stores its result from those registers and then runs only Go's epilogue. A render runs 0.37% (the shop workbench) to 0.65% (MUI) fewer instructions on the pages of tifa's example apps and 0.42% fewer on `r_react_ssr`, with the same HTML on linux/amd64 and darwin/arm64. On linux/amd64 the shop workbench took 0.978 of the time, the store 0.980, MUI 0.984, the bench pages 0.995 and `r_react_ssr` 0.973 (4 code positions, 3 rounds, all under 3%), and on darwin/arm64 0.990 to 0.997. The generated assembly is 1.9% (amd64) and 2.1% (arm64) smaller ([Performance](docs/performance.md#what-the-generated-code-costs-against-native-c)).
- The compiler drops the checks of the temporal dead zone that cannot fail: a read or a write of a `let`, `const` or `class` local after its declaration, in code that no jump can reach without going through the declaration, is a plain `get_loc` or `put_loc`, and a local left with no check, no capture and no `eval` in its function is no longer set uninitialized when its block starts, unless a loop or a `finally` runs that block again. A case of a `switch`, a read in a closure or before the declaration, the head of a loop, and code after a `catch` or a `finally` keep their checks, and every `ReferenceError` is thrown as before. A render runs 0.61% fewer instructions on the shop workbench, 0.34% on the store, 0.77% on MUI, 0.11% on the bench pages and 0.06% on a render of `r_react_ssr`, with the same HTML. The compile of a bundle takes a little longer (0.66 M instructions for that of `r_react_ssr`), which a VM of tifa does not pay. On linux/amd64 the shop workbench took 0.995 of the time, MUI 0.997 and the bench pages 0.990 (4 code positions, 3 rounds, all under 3%) ([Performance](docs/performance.md#checks-of-the-temporal-dead-zone)).

## [v0.29.5] - 2026-10-31

### Changed

- Bytecode is version 7, which has opcodes for a read of a local and the property read after it, for the case of a `switch` on a string and for a `typeof` test: bytecode written by an earlier release no longer loads (`SyntaxError: invalid version`), and has to be compiled again. `JS_ReadObject` rejects a fused opcode that is not followed by the instructions it stands for.
- `===`, `!==`, `==`, `!=` and `switch` on two equal strings longer than 65536 characters, or on a rope, and the lookup of a long string or rope key in a `Map` or `Set` that holds an equal one, read the stop every 65536 characters, so they stop within a few polls of an `Interrupt` or the eval timeout. They compared the whole strings in one call, about 3,300 polls (10 ms) for two strings of 32 million characters, and the script was killed when the strings were long enough. The same string or rope is now equal to itself at once.
- A string of more than 65536 characters used as a property key (`o[s] = 1`, `s in o`, `o[s]`, a key of `JSON.parse`) reads the stop every 65536 characters while its hash is made and while it is compared with the atom of an equal string. `JSON.parse` reads it every 65536 bytes of a string of the text, in the key or in a value. Each of them took one pass over the whole string, about 800 polls for 32 million characters (the hash), 3,300 more for the comparison with an equal atom, and 4,000 for the scan of a JSON string, with no stop in them.
- `indexOf`, `lastIndexOf`, `includes`, `startsWith`, `endsWith`, `replace`, `replaceAll` and `split` with a pattern as long as what is left of the string, `Array.prototype.indexOf`, `lastIndexOf` and `includes` and `Object.is` on two equal strings of more than 65536 characters, and `JSON.stringify` of a long string read the stop every 65536 characters. They compared or copied the whole string in one call, about 3,400 polls for 32 million characters.
- `JSON.parse` of a long string reads the stop every 65536 bytes of the whole string, escapes included: a string of nothing but escapes, or of runs of 32768 characters between escapes, was read to the end before. The text is made a C string in steps, a character above 0xff after a long run of ASCII widens the characters before it in steps (so do `padStart` and `padEnd` with such a filler), and a string that holds more than 1 MiB is copied in steps of 1 MiB when it grows.
- The hash of a long key and the compares with the keys of a Map, a Set or an object that have its hash and its length share one budget, and a compare that ends at a difference counts what it did. 131071 characters of hash and compare read the stop after 65536 each time, not 131071, and a chain of keys that differ from the key asked for late in the string is read as often as one of keys that are equal.
- A string of exactly 65536 characters is now read in steps like a longer one: `===` and `!==`, a key of a `Map` or `Set`, `Array.prototype.indexOf`, `lastIndexOf` and `includes`, and a property key. They counted 65536 as short and compared or hashed it in one call.
- A loop that compares many strings shorter than 65536 characters adds the compares up and reads the stop each time the sum passes 65536 characters: `Array.prototype.indexOf`, `lastIndexOf` and `includes` on an array of thousands of strings of one length, and a `Map` or `Set` lookup among keys of one hash and one length. An array of 16384 strings of 65535 characters, searched for one that differs in the last character, made about a billion character compares before it read the stop once.
- `JSON.parse` and `JSON.stringify` read the stop every 32768 characters of a run of 8-bit characters that is copied into a buffer of 16 bits (after a character above 0xff), since each character is looked at and then copied. A buffer of more than 1 MiB that has to grow reads the stop before the first step of its copy. It is copied into a new block, so two blocks are alive for the time of the copy (the allocator could extend a large block without a second one). The limit of the memory is checked as `realloc` checks it, against the memory less the old block, so a buffer that fit the limit before still grows, unless the footprint limit or the host, which may refuse a second block while the old one is there, stops it first. The memory in use can be higher for the time of the copy by about the old block, and that is not an upper bound.

### Fixed

- `GetProperty`, `GetPropertyValue`, `SetProperty`, `SetPropertyValue` (and the `Value` methods that call them), `Value.MarshalJSON`, `Value.Any` and `ErrorFromValue` run inside the eval timeout, as `Eval` and `Call` do. A getter, a setter, a proxy trap, `toJSON`, a replaced `BigInt.prototype.toString` or a `toString` that reads an error could run for as long as it liked, since these methods started no timeout. With a 10 ms timeout a getter of 200 ms returned 42 and no error. `SetProperty` and `SetPropertyValue` now return the exception that the setter threw, where they returned `failed to set property` and left the exception pending ([details](docs/changes/uncatchable.md#every-method-that-can-run-javascript-has-the-eval-timeout)).
- 23 fixes of upstream QuickJS are in the engine, each with a test that prints what Node 24 prints (the lines that follow the current specification are marked): `BigInt.asUintN` of a negative BigInt, `Iterator.prototype.take` and `drop` limits, `''.repeat(2 ** 31)`, `ArrayBuffer.prototype.resize` and `TypedArray.prototype.at` with a conversion that resizes the buffer, the completion value of `try`, `return()` of iterator helpers, `yield` and `await` as names, `f() = 1` in sloppy code, `Promise.try`, `export *` of a binding imported and exported again, `super.x = v` on a non-extensible receiver, the name of a function in a for-in head, the prototype of a bound function, the getOwnPropertyDescriptor and ownKeys traps of a proxy, the order of `Object.defineProperties`, class elements after `get`, `set` and `static`, `Array.prototype.sort` writing back every element, `for await` waiting for `return()` when it leaves early, and string names in exports. Two leaks are gone too: a failing `Atomics.store` and the read of cut-short bytecode with a BigInt. A sealed or frozen array keeps its length when an index past its end is set or defined. test262 has 25 fewer known failures (33 are left). The list and where the C tree differs from upstream or Node 24 are in [upstream-commits.md](docs/changes/upstream-commits.md).
- A `delete` that packs the properties of an object (after 8 or more keys are deleted, at least half of them) no longer leaves an "out of memory" error pending when the VM has no memory for the smaller property list. The delete had worked, but the uncatchable error stayed in the VM and took the place of the next error a script threw. Packing is optional now: the delete works, and the object is packed by a later delete.

### Performance

- The end of a call of a bytecode function skips the values with no reference count, eight at a time, where it looked at them two at a time. A render runs 0.7% to 2.1% fewer instructions on the pages of tifa's example apps and 1.3% to 2.0% fewer on the render scripts, with the same HTML. The 200 locals of a function, none set, cost 37% fewer instructions to call. The one case found slower is a function where every fourth local holds an object: 2.4% more instructions with 200 locals ([Performance](docs/performance.md#the-end-of-a-call-and-the-start-of-one) has the numbers).
- A call of a bytecode function sets its locals to `undefined` eight at a time, where it set four. A render runs 0.2% to 0.5% fewer instructions on the pages of tifa's example apps and on the render scripts, with the same HTML. The locals of a function with 200 locals cost 14% fewer instructions to set and free than with the change above, and a call of a function with 3 locals or fewer takes 1 to 4 instructions more.
- Object literals of the same keys, written in different places, share one shape, as objects built key by key do. The first object of a literal looked for its shape from the empty shape of `Object.prototype`, which the first object that got a key had usually changed in place, so each place made a shape of its own. A VM of tifa's pages holds 12% to 15% fewer shapes, and 0.7% (the shop and the MUI pages) to 1.7% (the store pages) less memory after a render. Renders run the same instructions ([objects-and-shapes.md](docs/changes/objects-and-shapes.md#object-literals-of-the-same-keys-share-one-shape)).
- A read of the first or second argument, of the first local or of a local past the fourth, followed by a read of a property by name (`o.x`), and the case of a `switch` on a string, take one turn of the interpreter. A render of the six workbench pages of tifa's example shop runs 1.85% fewer instructions, the store pages 1.59%, the MUI pages 1.66% and the bench pages 3.41%. The bench pages take 3.4% less time to render (8 code positions, 3 rounds, [c-engine.md](docs/changes/c-engine.md#opcodes-that-do-a-few-instructions-at-once), [Performance](docs/performance.md#opcodes-that-do-a-few-instructions-at-once)).
- `typeof x == "string"` and the like, with `==`, `===`, `!=` or `!==` and any type name, and `typeof x < "u"` and `typeof x > "u"` as minifiers write them, take one turn of the interpreter. A render of tifa's pages runs 0.63% (the shop) to 1.42% (the bench pages) fewer instructions.

## [v0.29.4] - 2026-10-30

### Performance

- An object literal of plain keys keeps its properties in the block of the object, not in a block of its own. A VM of tifa's pages holds 1.0% to 2.4% less after a render, a render runs 0.1% to 0.8% fewer instructions, and no page renders faster or slower by the rule (the shop 0.991 of the time, the MUI pages 0.998, the bench app 0.985, React 0.995). The memory one render adds with the collector off is 0.7% more on average ([Performance](docs/performance.md#object-literals-with-their-properties-in-the-block-of-the-object) has the numbers).
- The interpreter calls a built-in function written in C itself, without first entering the function that runs bytecode, which did no work of its own for such a call. A render of the six workbench pages of tifa's example shop runs 0.74% fewer instructions, its bench pages 2.3% fewer, and the pages took 0.98 to 0.99 of the time, within the noise of the code position. A script recurses exactly as deep as before, also through built-in functions such as `call`, `map` and `sort` ([Performance](docs/performance.md#c-functions-called-from-the-interpreter)).

## [v0.29.3] - 2026-10-30

### Added

- `VM.RegisterCopyProps(name)` defines a function named `name` on the global object that does the work of the loop with which React's `createElement` and `cloneElement` copy the keys of their config into the props of an element, written in C inside the engine. It copies where its result and the loop's cannot differ in anything a script can see (an ordinary config of at most 32 keys with plain values, props of the same kind, an unchanged `Object.prototype` and `Function.prototype.call`) and answers `false`, having changed nothing, for any other config, which the script then gives to the loop. The engine puts nothing on the global object by itself: the function is in a VM only when its host registers it. In tifa, which uses it for React, a render runs 2.1% to 4.1% fewer instructions, and the pages of its example shop render in 0.988 and 0.989 of the time and its MUI pages in 0.980. ([Performance](docs/performance.md#the-copy-of-the-props-of-a-react-element-in-c), [Go API](docs/changes/go-api.md#registercopyprops))

### Changed

- Loops of the engine that walk the items of a string, an array, a Map or a Set and call nothing else for each one stop within a few polls of an `Interrupt` or the eval timeout: `Object.fromEntries`, the spread of an iterable (`[...set]`), `Iterator.prototype.toArray` and `drop`, `Math.sumPrecise`, `Set.prototype.union`, `symmetricDifference` and `isSupersetOf`, and `Array.from`, the typed array constructors and `from`. The engine read the stop when a call started, and the `next` of these iterators is called without one, so a script that spread a Set of two million items, or took the union of two large Sets, ran on until it was killed. The copy of a Set that its methods make before they add to it stops too.
- `replace` and `replaceAll` with a string for the pattern look for it as `indexOf` does, and read the stop every 1024 matches. They used to compare the needle at each index where its first character was, so a needle that almost matches at every index (a hundred `a` and a `b` in a string of a million `a`) took the product of the two lengths, and a `replaceAll` with millions of matches ran on until it was killed. `trim`, `slice`, `substring`, `substr` and the pieces `split` makes stop within a million characters when the piece they cut out is longer than that, and so do `padStart` and `padEnd` with a filler of tens of millions of characters.
- `Map` and `Set` operations (`get`, `set`, `has`, `delete`, `getOrInsert`, `add` and the methods of `Set`) stop within a few thousand characters of an `Interrupt` or the eval timeout when the key is a long string or a rope. Hashing the key walks all of it, tens of milliseconds for ten million characters, and got the script killed.

## [v0.29.2] - 2026-10-30

### Added

- `VM.RegisterEscapeText(name)` defines a function named `name` on the global object that escapes text for HTML as `escapeTextForBrowser` does in React DOM's string renderer, written in C inside the engine. Its argument becomes a string as in `"" + text` (an object goes through `valueOf` and `toString`, a symbol throws), and each of `&`, `<`, `>`, `"` and `'` becomes `&amp;`, `&lt;`, `&gt;`, `&quot;` or `&#x27;`. A string with none of them comes back as the same string. The engine puts nothing on the global object by itself: the function is in a VM only when its host registers it. In tifa, which uses it for React's escape, a render runs 0.7% to 4.5% fewer instructions, and the pages of its example shop render in 0.974 of the time and its bench pages in 0.938. ([Performance](docs/performance.md#react-doms-text-escape-in-c), [Go API](docs/changes/go-api.md#registerescapetext))

### Changed

- `VM.RunGC` sets the GC threshold to 1.5 times what it left, as the GC the engine starts by itself does. It used to leave the threshold where it was, so when a script had grown the heap past it with no object made (array elements do), the first object of the next call started another full GC as long as the first one. A threshold of `^uintptr(0)`, which turns the engine's own GC off, stays. A program that sets the threshold itself after `RunGC`, as tifa does, sees no change.
- The spread of an object (`{...o}`), the rest of a destructuring and `Object.assign` look up each key of the source when its turn comes, as the specification says. A getter that deletes a later key, or makes it not enumerable, now leaves it out of the copy, where it was copied before: as `undefined`, or with the value the prototype has under that name. A getter that makes a later key enumerable now has it copied, where it was left out before. `Object.keys({ ...{ get a() { delete this.b; return 1 }, b: 2, c: 3 } })` is `["a", "c"]`, as in Node.
- `isWellFormed`, `toWellFormed`, `split`, `padStart`, `padEnd`, `trim`, `trimStart`, `trimEnd`, `anchor`, `fontcolor`, `fontsize`, `link`, `escape`, `unescape`, `encodeURI`, `encodeURIComponent`, `decodeURI`, `decodeURIComponent`, `Array.from`, the typed array constructors and `from`, making a rope (a long string joined from parts without copying them) into one string and `<` on two ropes stop within a few thousand characters or elements of an `Interrupt` or the eval timeout. On a string of tens of millions of characters, or as many elements, they ran on to the end, long enough to get the script killed. `===` on two ropes still reads no flag, as on two other strings.

## [v0.29.1] - 2026-10-30

### Performance

- An ordinary object (an object literal, `{}`, the props of a React element) takes 48 bytes of memory, not 72. The highest `MemorySize` of a render of a long Markdown page is 3.8% lower on average, and the 20 pages of the example shop hold 1.8% less at the end of a render. No page renders faster or slower ([Performance](docs/performance.md#ordinary-objects-of-48-bytes) has the numbers).
- `EvalSharedBytecode` loads a bundle in 0.79 (the React page bundle) to 0.86 (a tifa shop bundle) of the time after the first load of the same `SharedBytecode`: the engine checks the code of the bytecode against its rule of the polls once for each `SharedBytecode`, not once for each VM ([Performance](docs/performance.md#what-a-new-vm-and-loading-a-bundle-cost)). `EvalBytecode` scans every time, as before.
- An object literal of plain keys gets all its keys at once, with the shape its function keeps for it and a property array of its exact size. Without PGO on linux/amd64, at 8 code positions with the same HTML, the workbench pages of tifa's example shop render in 0.956 of the time, the MUI pages of tifa's test app in 0.969 and the table page of its bench app in 0.942, and a React server render does not change (0.991). A VM of the shop holds about 0.17 MiB more, and a long Markdown page takes 1.6% less at its peak. [objects-and-shapes.md](docs/changes/objects-and-shapes.md#object-literals-made-at-their-final-shape) has the details.
- A call of a bytecode function sets its locals to `undefined` four at a time and frees its values two at a time. Without PGO on linux/amd64, at 8 code positions with the same HTML, the table and spiral pages of tifa's bench app render in 0.958 and 0.967 of the time. The workbench pages of tifa's example shop (0.996, 2.5% fewer instructions), the MUI pages of tifa's test app (0.999) and a React server render (0.984, 3.1% fewer instructions) do not change by the rule. A loop of calls to a function with no locals runs 3% more instructions. [c-engine.md](docs/changes/c-engine.md#how-a-call-sets-and-frees-its-values) has the details.
- A `for in` loop over an ordinary object holds the shape of the object instead of a list of its keys, and `Object.prototype.hasOwnProperty.call(obj, key)` is answered in the interpreter without calling `call` and `hasOwnProperty`. On top of the change above, the shop renders in 0.984 of the time (2.5% fewer instructions), the MUI pages in 0.981, the spiral page of the bench app in 0.977 and React in 0.988: faster at every position of the shop and the MUI pages, but by less than the rule asks. With both changes the shop takes 0.982 of the time of before, the MUI pages 0.976, the bench pages 0.952 and React 0.969, faster at every position. Memory does not change. [objects-and-shapes.md](docs/changes/objects-and-shapes.md#a-for-in-loop-holds-the-shape-of-its-object) has the details.

### Changed

- The symbols `intl.Register` and `temporal.Register` put on the global object to mark a VM as set up are `Symbol.for("github.com/liustack/slowjs/intl")` and `Symbol.for("github.com/liustack/slowjs/temporal")`, where they were named after modernc.org/quickjs.
- A script is killed only when it has also kept the engine at work after its eval timeout or an `Interrupt`: 1000 polls of the engine since it was asked to stop, besides the 50 ms. Time in which the engine does no work, in a Go function the script called or while the OS or the Go scheduler runs other threads or goroutines, no longer gets a script killed that would have stopped by itself: the call returns the interrupted error and the VM goes on. On a loaded machine a script that stopped in under a millisecond of work could be killed before.
- `String.prototype.normalize`, `localeCompare`, `toUpperCase` and `toLowerCase` stop within a few thousand characters of an `Interrupt` or the eval timeout. On a long string they ran on until they had turned the whole string into code points, compared it or changed its case.
- Bytecode is version 6, which holds the keys of object literals: bytecode written by an earlier release no longer loads (`SyntaxError: invalid version`), and has to be compiled again.

## [v0.29.0] - 2026-10-29

### Changed

- **Breaking:** the engine is the C engine, the only one. It is QuickJS in C (`engine/`) compiled by LLVM to Go assembly that the module ships, so building needs no C compiler and no cgo. Until v0.28 the engine was QuickJS translated to Go by ccgo, and that engine is gone. Results, error stacks and HTML stay the same, with the differences below.
- **Breaking:** the module builds for linux/amd64 and darwin/arm64 only. A build for any other target, windows/amd64, darwin/amd64 and linux/arm64 included, stops with an error that names these two.
- **Breaking:** a script nests calls by a count, not by the stack. `SetMaxStackSize` takes calls: 5000 by default and 10000 at most, where the Go engine counted 10000 slots of stack. A script that recursed past about 5000 calls gets its `RangeError` sooner. JSON and parentheses nest 2000 and 665 levels at most (tifa's `/json/2001` page answers 500). Chains that crashed the engine, such as 300000 bound functions, now throw a `RangeError`.
- A script that runs on 50 ms past its eval timeout or an `Interrupt`, in a Go function it called for example, is killed. The call returns an interrupted `*Error` that also matches `ErrKilled`, and the VM is good only for `Close`.
- `MemorySize` and the memory limit count what the engine's own allocator holds, 13% to 15% less than the ccgo engine counted for the same heap. A VM takes its memory from the system in chunks, and `MemoryUsage` fills `Footprint`, `FootprintLimit`, `Chunks` and `ReleaseCandidate`, which were 0. `ReleaseMemory` gives free memory back and returns how many bytes.
- `Math.*` gives the same bits as before on both targets (`engine/test/math.sh` checks 100000 inputs for each of 33 operations).
- A compile or a lazy property that runs out of memory ends the call with the out of memory error, where the ccgo engine could leave the error pending or read the wrong value. `\p{Script=...}` that runs out of memory no longer crashes the process.
- A template literal converts each substitution to a string at once, as the specification says, and `-0.5 < -1n` is false. Both come from upstream QuickJS commits ([upstream-commits.md](docs/changes/upstream-commits.md)).

### Removed

- The build tags `slowjs_nogoasm`, `goasm`, `slowjs_cengine` and `libc.memgrind`. The first three do nothing now, and the last one tracked the allocations of the ccgo engine.
- The package `github.com/liustack/slowjs/lib`, the ccgo engine and its API. The root package, `intl`, `temporal` and `fmt` keep every exported name they had in the last release: `SetModuleLoader` and `SetDefaultModuleLoader` were removed in v0.27.0 and stay removed.
- The requirements `modernc.org/libc`, `modernc.org/ccgo/v4` and their dependencies. `go.mod` asks for `golang.org/x/sys` and `golang.org/x/arch` only, and a program that imports the module links `golang.org/x/sys` and nothing from modernc.org.

### Performance

Against the last release with its PGO profile, the C engine without PGO, on linux/amd64 (AMD EPYC 7B13, one core, 8 code positions each).

- The pages of tifa's example shop, MUI test app and bench app render in 0.834 to 0.860 of the time (11 pages, every position faster), with the same HTML except the page of the 2001 level JSON.
- The pause of the Go garbage collector stays as short: the P99 of a stop is 131 to 164 µs with 1 VM (164 before) and 197 µs with 3 VMs (197 before), the longest 3.1 ms (4.2 before) and 0.9 ms (2.1 before).
- A VM of the shop holds 8.3 to 8.6 MiB after loading, 12.8 to 12.9 MiB after 300 renders and 9.7 MiB once the pool gives its free memory back. The shop program is 0.5% larger, and its table of frames 152 KB smaller.
- A soak of 150 minutes, 211,076 renders, ended with no error and no VM replaced, and the memory of the VMs did not move. [History](docs/history.md#acceptance-before-the-switch) has the method and the rest.
- On the same JavaScript, a page of tifa's example shop, MUI test app and bench app takes 0.98 to 1.06 times as long as in C QuickJS 2026-06-04, 0.81 on the MUI pages. With its own `Intl`, where C needs a polyfill, the shop takes 0.82 and MUI 0.52. The other pure Go engines that render these pages take 1.6 to 6.6 times as long as slowjs ([Benchmarks](docs/benchmarks.md#tifa-pages)).

### Fixed

- `Close` of a VM that is already closed does nothing and returns nil. It used to panic on a nil pointer, so a deferred `Close` after an explicit one crashed. The functions of `OnClose` still run once.
- `Close` runs every step even when one panics. A function of `OnClose` that panics no longer keeps the functions after it from running, and nothing in `Close` is skipped for good: the VM leaves the table that finds it, the shared bytecode it held is released, and `Close` then panics with the first panic. A `Close` inside a function of `OnClose` returns nil.
- On the C engine, when the OS refuses to unmap some of the memory of a VM, `Close` returns that error, and `Close` again unmaps what is left. It used to return nil the second time and leave the memory mapped.

## [v0.28.6] - 2026-10-24

### Changed

- On linux/amd64 the engine runs JavaScript in the goasm interpreter by default, the interpreter in Go assembly that the `goasm` tag of v0.28.5 turned on. `-tags slowjs_nogoasm` builds a program with the Go interpreter, which every other target uses. The `goasm` tag is still accepted and changes nothing. Results, error stacks and HTML stay the same, and the build still needs no C toolchain. With one VM the pages of tifa's example shop render in 0.940 of the time and the MUI pages of tifa's test app in 0.850, without PGO. With 4 VMs on 4 cores tifa's pool renders 1.046 times as many shop pages a second and 1.171 times as many MUI pages, and with 8 VMs on 8 hyperthreads 1.085 and 1.211 times. After ten minutes of 8 VMs the process holds 0.93 of the RSS on the shop and 0.88 on the MUI pages. GC stops the world as fast as with the Go interpreter. See [The goasm interpreter](internal/goasm/gen/README.md).

## [v0.28.5] - 2026-10-24

### Added

- The `goasm` build tag (`go build -tags goasm`): on linux/amd64 the engine runs JavaScript in an interpreter compiled by LLVM from QuickJS's C and shipped as Go assembly, in place of the Go interpreter that ccgo makes from the same C. It is off by default, and on every other platform the tag changes nothing. Results, error stacks and HTML stay the same, and the build still needs no C toolchain. The pages of tifa's example shop render in 0.940 of the time and the MUI pages of tifa's test app in 0.850, without PGO. [The goasm interpreter](internal/goasm/gen/README.md) has how it is made and what must hold before it becomes the default.
- `GetAsyncContext` and `SetAsyncContext`, two Builtins that read and set the async context of a VM: an object that promise reactions keep. A `then`, `catch`, `finally` or `await` runs with the context the VM had when it was registered, and the `then` of a thenable with the one of the code that resolved a promise with it, as in Node. An `AsyncLocalStorage` written on them finds its store after `await`. A VM that never sets a context runs promises as before.

### Performance

Without PGO, against v0.28.2.

- Listing the own keys of an ordinary object, for `for in`, `Object.keys`, `values` and `entries`, object spread and rest, `Object.assign` and `JSON.stringify`, reads them from its shape in Go, and spread, rest, `values` and `entries` read plain values straight from the object: 0.990 of the time on the bench pages of tifa's example shop, 0.994 on its store pages and 0.987 on the MUI pages of tifa's test app, with the same HTML. `Object.keys` of six keys takes 134 ns rather than 542 and a spread of six keys 74 ns rather than 250 (Apple M4). A VM holds 8 KiB more.

## [v0.28.4] - 2026-10-24

### Changed

- `EvalBytecode`, `EvalBytecodeValue`, `EvalSharedBytecode` and `EvalSharedBytecodeValue` free the top-level function of a module once it has run to its end, with the variables of the top level that no function of the module uses. A VM that loaded a module held them until it was closed. A VM that loads the SSR bundle of tifa's example shop as a module holds 8.66 MiB rather than 9.44 MiB, about what the same bundle as a script gives (8.63 MiB). A module that is still pending or failed keeps its function.

## [v0.28.3] - 2026-10-24

### Fixed

- Regexps with the v flag no longer throw a SyntaxError for what the specification allows and Node accepts: `\` before one of ``& ! # % , : ; < = > @ ` ~`` in a class, as in `[\&\;]` or `[\!\|]`, a `-`, a `/` or a doubled punctuator such as `&&` outside a class, as in `/-/v`, and `\-` or `\&` in `\q{...}`. shiki's JavaScript engine failed on 204 of the 552 regexps it makes for bash, css, go, html, json, typescript and yaml. All but 3 compile now: those have more than 254 capture groups, the most a regexp holds. Patterns that compiled before compile to the same bytecode.

## [v0.28.2] - 2026-10-24

### Added

- `VM.SetClock` and `VM.Now` give one VM a wall clock that `Date.now()`, `new Date()`, `Date()`, Intl formatting without a date and `Temporal.Now` all read. nil restores the live clock. A non-nil `temporal.Clock` still overrides `Temporal.Now` alone. `performance.now`, the eval timeout, `Atomics.wait` and the `Math.random` seed keep their own clocks.

## [v0.28.1] - 2026-10-23

### Added

- `NewSharedBytecode`, `VM.EvalSharedBytecode` and `VM.EvalSharedBytecodeValue`: VMs that load the same bytecode keep the source text of its functions and the tables that give error stacks their line numbers in one copy for the process, where each VM copied them into its own memory. After loading, a VM of tifa's example shop holds 2.36 MiB less (8.99 MB rather than 11.46) and one of tifa's MUI test app 1.22 MiB less (4.16 MB rather than 5.44). `toString`, error stacks and line numbers stay the same. `MemorySize` and the memory limit no longer count those bytes. The copy is freed when the `SharedBytecode` and every VM that loaded it are closed, so a VM can outlive it. Render time and HTML do not change.

## [v0.28.0] - 2026-10-23

### Changed

- **Breaking:** the module is now `github.com/liustack/slowjs` with package `slowjs`, and it supports linux and darwin on amd64 and arm64 and windows on amd64.

## [v0.27.3] - 2026-10-23

### Added

- `VM.RunYoungGC` (`JS_RunYoungGC` in `lib/`): a garbage collection of only the objects made since the last collection, young or full. It frees the cycles among them and keeps the rest, which become old. A full collection, `VM.RunGC`, visits every object of the VM, about 30,000 for the bundle of tifa's example shop before any garbage. A program that runs one piece of work at a time, such as a page render, can run a young collection after each and a full one now and then. What a full collection frees, it still frees after any number of young ones. Young collections keep cycles through old objects and the values of weak maps until the next full one. Programs that do not call it collect as before.

### Fixed

- Loops of `indexOf`, `lastIndexOf` and `includes` of strings stop within 20 ms of a 10 ms timeout: each search counts the characters it looks through toward the interrupt check. In a string of 1 MB that holds no match they ran on for 170 ms, 6.9 s and 170 ms.

### Performance

- `includes`, `indexOf` and `lastIndexOf` of a string find what they look for with the search of Go's package bytes, where upstream compares at one index after the other: a search through 100 KB of HTML takes 81 µs rather than 320. A render of the shop takes 0.995 of the time and the MUI pages of tifa's test app 1.003, both inside the spread of code layouts. Results do not change.
- With tifa running `RunYoungGC` after each render and a full collection now and then, where it ran a full collection every few renders, the P99 of a render of the shop is 0.748 of what it was with one VM and 0.796 with two VMs, and that of the MUI pages of tifa's test app 0.940 and 0.919. The P50 and total CPU stay within 2.1%, and peak memory is 0.82 to 0.88 of what it was. Without PGO.

## [v0.27.2] - 2026-10-23

### Added

- `Args.Latin1` and `Args.UTF16`: the characters of a string argument of a Builtin as the engine holds them, one byte or one 16-bit unit each, without a copy. `VM.NewStringLatin1` and `VM.NewStringUTF16` make a string from such characters. A Builtin that looks at short strings and often returns its argument as it is (`a.Value(0).Dup()`) converts nothing: React's escape of HTML written this way takes 99 ns a call on a 16-character string, against 200 ns in React's javascript.


### Fixed

- The eval timeout and `Interrupt` stop a loop that compiles regexps within a few milliseconds, and stop one long compile in the middle, with the same uncatchable interrupt. Compiling a regexp was one step of the interrupt check however long the pattern, so with a 10 ms timeout a loop of `new RegExp` on a new long pattern each time stopped 225 ms to 184 s late, depending on the pattern, and one compile of 100000 alternatives ran its full 8.5 s. Now they stop at 10 to 12 ms.
- Loops of `JSON.parse`, `JSON.stringify`, `eval`, `new Function`, `repeat`, `padStart`, `replaceAll`, `join` and the other built-ins that parse source or build a string stop within a few milliseconds of the timeout too, where they ran on for 5 to 37 s with inputs of 60 KB to 1 MB. One call still runs to its end. Loops of built-ins that scan or sort without building a string (`indexOf` and `includes` of arrays, `split`, `fill`, `sort`) still stop late. Render time and memory do not change (0.993 of the time on the shop pages and 0.997 on the MUI pages, without PGO, against v0.27.1). A script that overran its deadline inside one long compile now gets the interrupt right after it, where it could finish.
- Full GC releases unreachable WeakMap entries in one collection, including values that point back to their keys and chains of entries. Live keys keep their values. Symbol keys, WeakSet, WeakRef and FinalizationRegistry follow the same rule. A full GC takes about as long as before on tifa's pages, and longer with large weak maps: 2.9 times as long for a chain of 100,000 live entries, 1.9 times for 100,000 live keys.

### Performance

- `Args.String`, `Args.WTF8` and `VM.NewString` (with every Go string that becomes a javascript string) convert in Go, straight from and into the engine's characters, with the same results. A Builtin that reads a 16-character string and returns a new one takes 134 ns rather than 230, and 521 ns rather than 1338 for 1,024 ASCII characters. `Value.Dup` takes no TLS frame: 81 ns rather than 97 for a Builtin that returns its argument.

## [v0.27.1] - 2026-10-23

### Fixed

- Native C builds using `lib/internal/patch/quickjs.diff` retain upstream's computed goto dispatch. The patch disables it only for ccgo and Emscripten. The generated Go engine is unchanged. Uncatchable exceptions, recursion RangeError and error stack rules are retained. Native recursion depth can differ because compiler frame sizes change under the same byte limit.

### Performance

Without PGO, against v0.27.0.

- `o.x` and `o.x(...)` of a string, a number or an array find a method on the prototype in the interpreter, without the generic property read: 0.996 of the time, and 0.975 on the MUI pages of tifa's test app, whose CSS-in-JS code reads string methods up to about 320000 times a render. Memory use does not change.
- `js_malloc` takes a block from the cache of freed small blocks itself, without the four calls it took before: 0.985 of the time, and 0.983 on the MUI pages of tifa's test apps. Memory does not change.

## [v0.27.0] - 2026-10-22

### Added

- `Intl.DateTimeFormat` accepts Temporal values in all four formatting methods, with type-specific defaults and calendar checks. `formatRange` and `formatRangeToParts` also accept Date and numeric values. Temporal date localization uses the same formats. All 18 Chromium calendars, including ISO, are supported through the shared go-intl data packs.
- `Intl.Collator`, `Intl.Segmenter`, `Intl.RelativeTimeFormat`, `Intl.ListFormat`, `Intl.DisplayNames`, `Intl.DurationFormat`, `Intl.Locale` and `Intl.supportedValuesOf`, in every locale Chromium has, compared with Chromium 156 on 14,269 expressions. `localeCompare` sorts by the locale's collation, and `toLocaleUpperCase` and `toLocaleLowerCase` follow Turkish, Azeri, Lithuanian and Greek rules. They share go-intl's data with the other constructors. Optional build tags `quickjs_intl_nonames` and `quickjs_intl_nocjdict` omit display names and the Chinese and Japanese word dictionary. A program using `intl` must carry the notices listed in `NOTICE.md`. Current size and memory measurements are in `docs/changes/time-and-intl.md`.
- `Args.WTF8`: read a string argument of a Builtin with its lone surrogates as the engine holds them, where `Args.String` gives U+FFFD for each.
- `VM.SetRegExpCache`, `DefaultRegExpCache` and `DefaultRegExpCacheBytes`: how many compiled regexps a VM keeps, and in how many bytes (256 in 1 MiB by default). The cache counts against the memory limit: give a VM that runs close to its limit a smaller cache, or none.
- `VM.OnClose`: run a function when the VM is closed, so a package that keeps something per VM in a table of its own can drop it then.
- Package `temporal`: `temporal.Register(vm)` adds `Temporal`, every type and method V8 ships, and `Date.prototype.toTemporalInstant`, computed in Go by go-intl with ICU's zones for tz 2026e built in, so the host's zoneinfo is never read. `Temporal.Now` uses the zone `intl.TimeZone` sets, and the clock `temporal.Clock` sets. The original built-in test262 run passed 9206 of 9210 cases. The current `intl402/Temporal` run passes 4038 of 4058. The original implementation took 13.4 ms on first use in a process and 1.1 ms in a later VM. Current size measurements are in the fork-change docs.

### Changed

- `performance.now()` of `StdAddHelpers` counts from the start of the process. Before, it counted from the boot of the system on Linux and macOS, and from 1970 on Windows and FreeBSD, where it moved when the system clock was set. On macOS it no longer counts the time the system sleeps. `Date.now()` and `new Date()` give the same values as before.
- All Intl constructors, including NumberFormat, DateTimeFormat and PluralRules, now use go-intl. Number and Date locale methods share the same data. Rounding increments and priorities work, and all date calendars are available. The separate `intl/cldr.bin` and its generator are removed. Against main `641e996`, the shop built with its application PGO profile is 0.11 MB smaller. Compiled plans and direct string output cut the measured warm number formatting time by 41% to 42% and date formatting time by 28% to 37% against main. The speedup leaves all recorded answers unchanged. See [measured costs](docs/changes/time-and-intl.md#focused-timing-and-memory-costs).
- Intl resolves named zones, offsets and daylight flags from the embedded tz 2026e pack shared with Temporal. It no longer reads host zoneinfo or needs a DST correction table. Winnipeg follows tz 2026e's permanent daylight time, newer than Chromium 156's data.
- The full Intl402 test262 run passes 6618 of 6764 cases, against 6276 on main `641e996`. All 24 newly failing assertions also fail in Chromium 156. Existing golden sets have no new unexplained differences.

### Removed

- `lib` no longer has the `std`, `os` and `Worker` modules of QuickJS's shell (`quickjs-libc.c`), which the Go API never registered: `js_init_module_std`, `js_init_module_os`, `js_module_check_attributes`, `js_std_set_worker_new_context_func`, `js_std_init_handlers`, `js_std_free_handlers`, `js_std_promise_rejection_tracker`, `js_std_await`, `js_std_loop`, `js_std_eval_binary` and `js_std_eval_binary_json_module` are gone. `StdAddHelpers`, `SetDefaultModuleLoader` and the bytecode of modules work as before. A program no longer links that code, nor the parts of libc only it called (`fork`, `execve`, `popen`, `setenv`, files, the terminal): one that only calls `Eval` is 354 KB smaller on linux/amd64, and the server of tifa's example shop 181 KB smaller, built as `tifa build` builds it (`-ldflags=-w` with the shop's PGO profile, linux/amd64).

### Fixed

- `Temporal.Duration.prototype.toLocaleString` formats through `Intl.DurationFormat`, including styles, unit options and fractional digits, instead of returning an ISO duration string.
- `Intl` no longer throws on a page that replaced `String.prototype.concat`, which QuickJS calls for a template literal: `Intl.NumberFormat`, `Intl.DateTimeFormat` and the `toLocale` methods threw there in v0.26.17.
- `Intl.PluralRules` selects the plural of a compact or scientific number as ICU does, and every `supportedLocalesOf` reads its `localeMatcher` option.
- A function in a list of locales is read as an object, as ECMA-402 has it, so `new Intl.NumberFormat([function () {}])` throws a RangeError, where v0.26.17 threw a TypeError.
- The first use of `Intl` keeps what a page did before it: its own `Date.prototype.toLocaleString` or `Number.prototype.toLocaleString` stays, so does an `Intl` it defined with `Object.defineProperty` or deleted, and a frozen `Date.prototype` or `globalThis` no longer makes that use throw a `TypeError`.
- `EvalBytecode` of a module whose top-level await is still pending when no job is left to run, such as `await new Promise(() => {})`, returns an error that says so instead of crashing the process. If what the module waits for settles later, from Go for one, a later `ExecutePendingJobs` runs the rest of it. It waits for the module as `ExecutePendingJobs` runs jobs: a job that throws no longer stops it, the module runs to its end and the call returns the job's error, or the module's own rejection. The eval timeout, an interrupt, out of memory or a panic in a Go function after a module's first `await` ends the call at once, where it was printed to stderr and the process then crashed.
- On Windows, `Date.now()` and `new Date()` no longer overflow in 2038.

### Performance

All without PGO, against v0.26.17.

- A VM keeps the bytecode of the regexps it compiled, so code that makes the same regexps on every render compiles them once: the journal post of tifa's example shop that highlights CSS renders in 0.893 of the time, the other pages in the same time. A VM of the shop holds 12 KiB more, and one that rendered the CSS post keeps about 714 KiB of regexps.
- `Date.now()`, `new Date()` and `performance.now()` read the time from Go instead of a system call of modernc.org/libc: 122 ns rather than 689, 261 rather than 870, and 107 rather than 686. A render of the shop does not change.
- The likely and unlikely tests of the engine branch on their condition, without making an integer of it first: 0.983 of the time, and 0.976 to 0.987 on the pages of tifa's bench app.
- On linux, the engine takes and frees its TLS frames without calling libc, except in the interpreter function: about 360000 fewer calls in a render of tifa's bench pages. On top of the change above, 0.998 of the time and 0.990 on the bench pages. Both together: 0.979 of the time of v0.26.17, and 0.967 on the bench pages.

## [v0.26.17] - 2026-10-21

### Added

- `NewBuiltin` and `VM.NewBuiltinFunction`: a Go function that javascript calls as a built-in, such as `Math.max`, with `Args` to read its arguments without conversions. A call that adds two numbers takes about 55 ns on an Apple M4, against about 470 ns through `RegisterHostFunc`. An `*Error` it returns is thrown as the error its `Name` says.
- `VM.NewArray`: an array of values made in Go, made as an array literal makes one, so no setter a page put on `Array.prototype` runs.
- `VM.SetStrip` with `StripSource` and `StripDebug`: compile functions without their source, so their `toString` shows `[native code]`, or without line and column numbers.
- `VM.RunGC`, `VM.MemorySize` and `VM.GCThreshold`: run a GC now, read the memory a VM takes as the memory limit counts it, and read the threshold of the next GC. A program can collect between two calls into javascript and set the threshold for the next one, as tifa does between two renders.

### Changed

- `Error()` of an `*Error` made in Go is `Name: Message`, as javascript's `Error.prototype.toString` writes it, where it was empty. So the `TypeError` a `RegisterHostFunc` function throws for such an error now has that message. An error taken from javascript keeps the engine's text.
- `Intl` formatters are shared by every VM of the process, up to 1024, and the oldest are dropped first. A VM that has used `Intl` holds about 15 KB less.
- `Intl`'s functions have the shape of built-ins: a `length` and `name` as ECMA-402 gives them, no `prototype`, they cannot be called with `new`, and `toString` shows `[native code]`.
- `Intl.NumberFormat.call(o)` and `Intl.DateTimeFormat.call(o)`, for an `o` that inherits from their prototype, keep the new formatter on `o` and return it, as V8 does.
- The files this fork created are under the MIT license (`LICENSE-MIT`), and its source files carry an SPDX line. 14 files in `lib/` that repeat parts of the engine say `MIT AND BSD-3-Clause`. Code from upstream stays under BSD-3-Clause and QuickJS's MIT license, and the locale data under the Unicode License V3. `NOTICE.md` lists which license covers which path, and which notices a program that ships a binary has to include.
- The README is a short landing page, in English and Chinese. What it held moved to `docs/`: the fork changes by area, testing, upgrading from upstream, performance, releasing, and where the code comes from, with what changes when switching from modernc.org/quickjs.

### Fixed

- A page that changes `Array.prototype`, `Object.prototype`, `JSON` or other built-ins after `Register` no longer changes what `Intl` does.
- `Intl` converts options, values and locale lists as ECMA-402 does: a Symbol or a BigInt where a string or a number is wanted throws a `TypeError`, `null` locales throw a `TypeError`, and options are read in the order ECMA-402 reads them.
- `Intl.NumberFormat` formats a wrapped BigInt, or an object whose `valueOf` gives a long numeric string, exactly, where it rounded it through a double.
- test262's intl402 tests pass 912 runs of 6764, against 770, none of those that passed before failing.

### Performance

All without PGO, against v0.26.16.

- `===`, `!==`, `==` and `!=` answer the usual pairs (`null`, `undefined`, booleans, objects, symbols, strings that are atoms) in the interpreter: 0.982 of the time.
- A string key that is already an atom is found in one step: 1.3% less time. Reading with such a key takes 13% less time in a micro benchmark, writing 30% less.
- A call of a javascript function takes one TLS frame instead of two on linux, and 34 more functions take a TLS frame less often: about a fifth fewer frames per render, and 1.4% less time.
- A regexp of alternatives no longer tries the alternatives that cannot match at a position: about 1% to 2.5% less time. A `replace` with react-spring's regexp of 148 color names takes 5.9 µs rather than 44.5.
- A new function gets its `length` and `name` without the shapes in between: 0.982 of the time.
- Bitwise operators on floats skip the slow path: the pages of tifa's MUI test app take 0.952 to 0.960 of the time. The shop runs them too rarely to gain.
- A `for in` loop skips prototypes with no enumerable property: up to 3% less time on pages that run many such loops.
- `JSON.stringify` and `JSON.parse` copy runs of plain characters at once: quoting a route's data takes 41% less time, though a render gains less than 1%.
- `Intl`: `new Intl.NumberFormat('en')` takes 2.7 µs rather than 16.5, a `DateTimeFormat` with a time zone 8.1 µs to make rather than 32.0 and 2.6 µs to format rather than 5.2, on an AMD EPYC 7B13 core.

## [v0.26.16] - 2026-10-20

### Added

- `VM.SetShapeCache` and `DefaultShapeCache`: how many shapes of plain objects a VM keeps once their objects are gone. A new VM keeps 2048, and 0 turns the cache off. `lib` has `JS_SetShapeCache`, off until called.

### Changed

- A VM of the shop holds 722 KiB more after a GC (8.03 MiB rather than 7.33), 647 KiB of it the kept shapes. Kept shapes count against the memory limit and are not let go when an allocation fails, so a script that runs close to its limit can run out of memory where it did not before: give such a VM a smaller shape cache, or 0.
- Objects that get keys no other object has, one at a time, take about 6% longer to build than with the shape cache off.

### Performance

- The TLS frame of 8 hot functions, among them property reads, `==` and turning a value into a string, is taken only around the statements that need it. Renders take 1.6% less time with PGO and 1.1% less without, and 2.6% less on darwin/arm64 (Apple M4, with PGO).
- The interpreter writes out its 46 calls of `set_value`: 1.4% less. With the frames above, 2.8% less than v0.26.15 with PGO and 2.9% less without.
- The interpreter counts its interrupt counter down in its own body instead of calling a function on every loop pass: 0.6% less with PGO.
- Each runtime keeps freed blocks of 16 to 512 bytes in a cache of its own, given back at each GC: 0.9% less with PGO.
- A VM keeps the shapes of plain objects, so the next object literal with the same keys makes no shape: object literals with the same keys take 30% less time to build, and a render 3.6% less without PGO. With the two changes above, 4.5% less than without the three. Two VMs on the two hyperthreads of one core render 2.9% more pages a second.

## [v0.26.15] - 2026-10-19

### Changed

- A panic in the engine itself, or a Go function that called `runtime.Goexit` (as `t.Fatal` does), leaves the VM good only for `Close`: every later call into javascript panics. A Go function given to javascript that panics is still recovered as before.
- Outside linux each VM keeps a block of up to 1 MiB of TLS stack until it is closed. On windows that block counts in full against the commit limit.
- A PGO profile collected with an earlier release matches far fewer call sites: collect it again.

### Performance

- Engine functions free their TLS frame before each return instead of in a `defer`, so Go inlines them: about 10% less time, with or without PGO.
- A regexp that starts with one character or one class of characters, such as `/["&<>]/`, starts where that character first occurs: about 3% less.
- `===` and the names of keys take a TLS frame only on their rare paths: 0.7% less with PGO, 2% without.
- Outside linux, VMs on several goroutines no longer wait on libc's allocator lock for their TLS stack: 10 VMs on an Apple M4 render 1.4 to 1.55 times as many pages a second.
- Together, renders take 0.859 of the time of v0.26.14 with a new PGO profile, and 0.843 without PGO.

## [v0.26.14] - 2026-10-19

### Fixed

- `NewVM` no longer leaks the memory it allocated when it cannot make a context.

### Changed

- `internal/fixlib` stops wherever an upstream change could make it fix the generated code wrong. Programs see no change.

## [v0.26.13] - 2026-10-18

### Performance

- The interpreter loop does less work with the stack: about 12% less time on linux/amd64 and 17% less on darwin/arm64 (Apple M4). Its Go stack frame went from 2608 to 896 bytes on amd64.
- Adding a new property to a plain object, as object literals, spread, class fields and `JSON.parse` do, skips the generic path: about 6% less.
- Together, renders take 0.823 of the time of v0.26.12.

## [v0.26.12] - 2026-10-18

### Changed

- On linux, a recursion through bound functions can hold up to about 1 KB more of TLS blocks per level, and on 32-bit targets a block can take up to 1 KB where it took 256 bytes. The worst case of a recursion to the default limit stays about 20.7 MB on linux/amd64.

### Performance

- A JavaScript call allocates no Go memory and no longer takes libc's allocator lock: about 11% less time on linux/amd64 and 14% less on darwin/arm64, and loading bytecode into a VM about 11% less on linux. Several VMs on several cores no longer hold each other back on that lock.

## [v0.26.11] - 2026-10-18

### Fixed

- `RegisterFunc` and `RegisterHostFunc` no longer abort the process when the global object already has the name. The function replaces a configurable property, and the call returns an error for a property that is not configurable, a global object that is not extensible, or a name with a NUL byte.

## [v0.26.10] - 2026-10-18

### Fixed

- `VM.Interrupt` is safe from any goroutine while the VM closes and after `Close`, where it does nothing.
- A Go string that is not valid UTF-8 becomes a JS string with U+FFFD for each byte that does not start a valid sequence, as Go decodes it. The engine used to drop the bytes after an invalid one, ASCII included.

## [v0.26.9] - 2026-10-18

### Fixed

- `intl` names daylight saving time as ICU does on every system. On Linux, Europe/Dublin and a few other zones swapped their summer and winter names.

## [v0.26.8] - 2026-10-18

### Fixed

- The module builds on linux and freebsd again, which v0.26.6 and v0.26.7 broke.
- A Go string made from a JS string is valid UTF-8: a lone surrogate, as left by cutting a string inside an emoji, becomes U+FFFD, as Node writes it.

## [v0.26.7] - 2026-10-17

Does not build on linux and freebsd. Use v0.26.8 or later.

### Added

- `*Error` has `Uncatchable`, `OutOfMemory` and `Interrupted`, which say whether the call ended with an exception no script could catch, and why. What a script throws is never uncatchable, whatever its name and message.

### Fixed

- The eval timeout runs on the monotonic clock, so setting the system clock no longer moves it, and a timeout too long for the clock never fires instead of firing at once.
- Taking an uncatchable error runs no page code, such as an `Error.prototype.toString` the page replaced.

## [v0.26.6] - 2026-10-17

Does not build on linux and freebsd. Use v0.26.8 or later.

### Changed

- An interrupt or the eval timeout can no longer be caught through a promise, an iterator or a Go function: the call returns it, as for a script without promises. `ExecutePendingJobs` stops at the job that failed with it and leaves the rest queued.
- Running out of memory is uncatchable too, and the call returns `*Error{Name: "InternalError", Message: "out of memory"}`. Page code can no longer fill memory and catch the failure.
- A panic in a Go function that javascript called no longer unwinds through the engine. The script ends as for an interrupt, and the Go code that called javascript panics again with the same value.
- Recursing without end throws `RangeError: Maximum call stack size exceeded`, which a script can catch. It used to kill the process with a Go stack overflow. A new runtime starts with a stack limit of 10000 slots.

### Fixed

- Compiling a script, a function body, `eval` code or a regexp with little memory left fails with the out of memory error, where the engine could crash, report a `SyntaxError` the script does not have, or return bytecode cut short.

## [v0.26.5] - 2026-10-17

### Fixed

- A string that cannot be converted to Go, for want of memory, comes back as the error instead of `""`, from `Eval`, `Call`, the JSON of an `Object`, `Value.MarshalJSON` and the arguments of Go functions.
- A BigInt whose `toString` throws reports that exception, and reading the name and message of a thrown value leaves no exception behind.

## [v0.26.4] - 2026-10-16

### Fixed

- `Intl.NumberFormat` reads fraction digits as ECMA-402 has since 2021: a currency with `maximumFractionDigits: 0`, or fraction digits next to significant digits, no longer throws a `RangeError`.

## [v0.26.3] - 2026-10-16

### Changed

- A script's local time is UTC in every VM, whatever zone the process has, without changing `TZ` for the rest of the process. `getTimezoneOffset()` is 0, the local getters and setters work in UTC, and `toString` prints `GMT+0000`. To show another zone, pass a `timeZone` to `Intl`.

## [v0.26.2] - 2026-10-16

### Added

- Package `intl` formats dates, numbers and plurals in every locale Chromium has, from CLDR 48.2, and adds `Intl.PluralRules`. Its output is compared with Chromium 156 on about 60,000 expressions. A program that uses it is 1.7 MB larger.

### Changed

- `intl` defaults to UTC instead of the host's zone, and installs itself the first time a script uses `Intl` or a `toLocale*` method, from bytecode compiled once per process. `Register` takes 0.09 ms and 9 KB of the VM's memory, against 0.85 ms and 46 KB before.

### Fixed

- A Go function that evaluates more javascript on its own VM no longer sets the eval timeout again or drops an `Interrupt` asked for meanwhile.
- An `intl` install cut short by running out of memory, a stack overflow or an interrupt is rolled back, and the next use installs again.

## [v0.26.1] - 2026-10-15

### Changed

- An error that something keeps holds the bytecode of the functions on its stack, and their realm, until its `stack` is read.

### Performance

- Making an error no longer formats its `stack`: the text is made the first time `stack` is read, with the same result. A server render of a 100-item React component with the development build of React took 11.2 ms instead of 42.1 ms on darwin/arm64.

## [v0.26.0] - 2026-10-14

The first release of the fork, from modernc.org/quickjs v0.25.0 and libquickjs v0.13.1.

### Changed

- **Breaking:** the module is `github.com/liustack/quickjs`, and the engine is its package `github.com/liustack/quickjs/lib`, so it no longer depends on `modernc.org/libquickjs`. It needs Go 1.27.

### Fixed

- A VM that runs out of memory inside `JSON.parse`, or anything else that converts a string with non-ASCII characters to UTF-8, no longer leaks that string.
- `VM.Call` and `VM.CallValue` report the exception thrown while evaluating the function expression instead of `cannot call a non-function`.

[Unreleased]: https://github.com/liustack/quickjs/compare/v0.30.4...main
[v0.30.4]: https://github.com/liustack/quickjs/compare/v0.30.3...v0.30.4
[v0.30.3]: https://github.com/liustack/quickjs/compare/v0.30.2...v0.30.3
[v0.30.2]: https://github.com/liustack/quickjs/compare/v0.30.1...v0.30.2
[v0.30.1]: https://github.com/liustack/quickjs/compare/v0.30.0...v0.30.1
[v0.30.0]: https://github.com/liustack/quickjs/compare/v0.29.11...v0.30.0
[v0.29.11]: https://github.com/liustack/slowjs/compare/v0.29.10...v0.29.11
[v0.29.10]: https://github.com/liustack/slowjs/compare/v0.29.9...v0.29.10
[v0.29.9]: https://github.com/liustack/slowjs/compare/v0.29.8...v0.29.9
[v0.29.8]: https://github.com/liustack/slowjs/compare/v0.29.7...v0.29.8
[v0.29.7]: https://github.com/liustack/slowjs/compare/v0.29.6...v0.29.7
[v0.29.6]: https://github.com/liustack/slowjs/compare/v0.29.5...v0.29.6
[v0.29.5]: https://github.com/liustack/slowjs/compare/v0.29.4...v0.29.5
[v0.29.4]: https://github.com/liustack/slowjs/compare/v0.29.3...v0.29.4
[v0.29.3]: https://github.com/liustack/slowjs/compare/v0.29.2...v0.29.3
[v0.29.2]: https://github.com/liustack/slowjs/compare/v0.29.1...v0.29.2
[v0.29.1]: https://github.com/liustack/slowjs/compare/v0.29.0...v0.29.1
[v0.29.0]: https://github.com/liustack/slowjs/compare/v0.28.6...v0.29.0
[v0.28.6]: https://github.com/liustack/slowjs/compare/v0.28.5...v0.28.6
[v0.28.5]: https://github.com/liustack/slowjs/compare/v0.28.4...v0.28.5
[v0.28.4]: https://github.com/liustack/slowjs/compare/v0.28.3...v0.28.4
[v0.28.3]: https://github.com/liustack/slowjs/compare/v0.28.2...v0.28.3
[v0.28.2]: https://github.com/liustack/slowjs/compare/v0.28.1...v0.28.2
[v0.28.1]: https://github.com/liustack/slowjs/compare/v0.28.0...v0.28.1
[v0.28.0]: https://github.com/liustack/slowjs/compare/v0.27.3...v0.28.0
[v0.27.3]: https://github.com/liustack/quickjs/compare/v0.27.2...v0.27.3
[v0.27.2]: https://github.com/liustack/quickjs/compare/v0.27.1...v0.27.2
[v0.27.1]: https://github.com/liustack/quickjs/compare/v0.27.0...v0.27.1
[v0.27.0]: https://github.com/liustack/quickjs/compare/v0.26.17...v0.27.0
[v0.26.17]: https://github.com/liustack/quickjs/compare/v0.26.16...v0.26.17
[v0.26.16]: https://github.com/liustack/quickjs/compare/v0.26.15...v0.26.16
[v0.26.15]: https://github.com/liustack/quickjs/compare/v0.26.14...v0.26.15
[v0.26.14]: https://github.com/liustack/quickjs/compare/v0.26.13...v0.26.14
[v0.26.13]: https://github.com/liustack/quickjs/compare/v0.26.12...v0.26.13
[v0.26.12]: https://github.com/liustack/quickjs/compare/v0.26.11...v0.26.12
[v0.26.11]: https://github.com/liustack/quickjs/compare/v0.26.10...v0.26.11
[v0.26.10]: https://github.com/liustack/quickjs/compare/v0.26.9...v0.26.10
[v0.26.9]: https://github.com/liustack/quickjs/compare/v0.26.8...v0.26.9
[v0.26.8]: https://github.com/liustack/quickjs/compare/v0.26.7...v0.26.8
[v0.26.7]: https://github.com/liustack/quickjs/compare/v0.26.6...v0.26.7
[v0.26.6]: https://github.com/liustack/quickjs/compare/v0.26.5...v0.26.6
[v0.26.5]: https://github.com/liustack/quickjs/compare/v0.26.4...v0.26.5
[v0.26.4]: https://github.com/liustack/quickjs/compare/v0.26.3...v0.26.4
[v0.26.3]: https://github.com/liustack/quickjs/compare/v0.26.2...v0.26.3
[v0.26.2]: https://github.com/liustack/quickjs/compare/v0.26.1...v0.26.2
[v0.26.1]: https://github.com/liustack/quickjs/compare/v0.26.0...v0.26.1
[v0.26.0]: https://github.com/liustack/quickjs/tree/v0.26.0
