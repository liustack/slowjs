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
    API, `Object.getOwnPropertyDescriptor(s)` and `JS_GetOwnProperty`,
    redefining a frozen `stack` with a value (the engine compares the old
    and the new value), and the checks a proxy makes after a `get` or `set`
    trap on a frozen `stack`. What only looks at keys or attributes leaves
    it pending: `Object.keys`, `Object.freeze`, `Object.seal`,
    `Object.isFrozen`, `Object.isSealed`, `propertyIsEnumerable`,
    `__lookupGetter__`, `__lookupSetter__`, and the proxy `has`,
    `deleteProperty` and `ownKeys` checks. `stack` is not enumerable, so
    spreading, `Object.assign` and `Object.entries` skip it.
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
    an out of memory error and the stack stays pending, so a later read can
    give the text. Reading the property descriptor formats the stack the
    same way and so can throw too. Printing an error (`console.log`) never
    throws and never leaves a pending exception: out of memory there prints
    the error without its stack, and a later read can still give it.
  - If the text cannot be formatted when a stack is built at once (only a
    function name that needs more memory to convert to UTF-8 can cause this),
    `stack` becomes `null`, in Go and in the C patch alike. The upstream
    engine instead keeps a half-formatted stack with the name shown as
    `<anonymous>` and leaves an exception pending.

  Tests: `errorstack_test.go`, `errorstack_unix_test.go`,
  `lib/backtrace_test.go`, `lib/backtrace_round2_test.go`,
  `lib/backtrace_round3_test.go`, `internal/fixlib/main_test.go`.

The engine lives in the same module so that a fix to it ships together with
the Go API, without waiting for an upstream release of either module.

```sh
go get github.com/liustack/quickjs
```

The license files of both projects are kept: [LICENSE](LICENSE) and
[LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) at the root, and
[lib/LICENSE](lib/LICENSE), [lib/LICENSE-LIBQUICKJS](lib/LICENSE-LIBQUICKJS)
and [lib/LICENSE_TEST262](lib/LICENSE_TEST262) for the engine. Maintenance
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
package wraps. Package [quickjs/intl] adds `Intl.DateTimeFormat` and
`Intl.NumberFormat`, en-US over Go's time zone database, for a VM that opts in
by calling its `Register` — once per VM, including one created to replace
another, because `Intl` lives on that VM's global object.

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
  two lines added to `XJS_ToCStringLen2` and the calls into
  `lib/backtrace.go` (lazy error stacks) conflict. Take the upstream version
  of every generated file, then put the changes back:

  ```sh
  git checkout --theirs -- 'lib/ccgo_*.go' 'lib/internal/run-test262/ccgo_*.go'
  go run ./internal/fixlib
  ```

  `fixlib` also rewrites `modernc.org/libquickjs` imports in hand-written
  files under `lib/`. `go run ./internal/fixlib -check` lists files that still
  need changes. If `fixlib` reports that `XJS_ToCStringLen2` changed shape,
  read the new upstream code: QuickJS may have fixed the leak itself. If it
  reports that `_build_backtrace`, `_get_prop_string`,
  `JS_GetPropertyInternal`, `JS_GetOwnPropertyInternal`, the interpreter's
  `get_field`, `js_print_error`, `js_print_object` or `JS_ComputeMemoryUsage`
  changed shape, compare the new upstream `build_backtrace` with
  `lib/backtrace.go`, which formats the same text from the recorded frames
  and must change with it.
  `fixlib` also keeps a list (`slotSites`) of every engine function that
  reaches a property value, with how often it does. A pending `stack` must
  be formatted before any of them hands the value on. If a release adds such
  a function or changes how often one reaches a value, `fixlib` stops and
  names it: check whether it can see a pending `stack`, wire it to
  `lib/backtrace.go` if so, and update the list. It does the same for any
  new property lookup in the interpreter loop (`JS_CallInternal`).
- `lib/internal/patch/quickjs.diff`: holds the same changes in C, so that a
  regeneration keeps them. Keep the `JS_ToCStringLen2` hunk unless upstream
  QuickJS has fixed the bug, and the lazy stack hunks (`JSBacktrace`, the
  built-in `JS_CLASS_BACKTRACE` class, `js_backtrace_build` and the
  `js_backtrace_*` calls in `JS_GetPropertyInternal`,
  `JS_GetOwnPropertyInternal`, `JS_DefineProperty`, the interpreter's
  `GET_FIELD_INLINE`, `get_prop_string`, `js_print_error`,
  `js_print_object`, `JS_ComputeMemoryUsage`, and the callers that only
  need the attributes of a property (`js_get_own_property_flags`)). After a regeneration that includes them,
  `lib/backtrace.go` and its wiring in `fixlib` are no longer needed.
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
leftover is the Symbol key `"modernc.org/quickjs/intl"` in `intl/intl.go`,
which is a runtime marker, not an import path.

### Checks

```sh
go test -short ./...                                     # all packages
go test -vet=off -run 'TestToCString' ./lib              # the leak reproduction
go test -run 'TestErrorStack' .                          # stack text and memory of lazy error stacks
go test -vet=off -run 'TestBacktrace' ./lib              # shapes, retries and writes of pending stacks
go run ./internal/fixlib -check                          # fork changes present in lib/
go test ./internal/fixlib                                # fixlib notices new reads of property values
for t in linux/amd64 linux/arm64 darwin/arm64 windows/amd64; do
	GOOS=${t%/*} GOARCH=${t#*/} go build ./...
done
```

`lib/internal/run-test262` is a module of its own, so its 69 MiB of
transpiled test runner stay out of the module zip. `Test262` in `lib/` builds
it with `go build -C`. The runner runs the engine in `lib/` (see `fixlib`),
so it checks the changes made there.
