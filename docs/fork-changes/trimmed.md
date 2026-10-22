---
summary: 'What the fork leaves out of the engine because the Go API can never reach it: the std, os and Worker modules of quickjs-libc.c, which functions stay and why, what a program saves, and what to do when fixlib stops on quickjsLibcSites'
read_when:
  - Changing internal/fixlib/quickjslibc.go or quickjsLibcRoots
  - fixlib stops on quickjsLibcSites, or says a function of quickjs-libc.c is used outside what stays
  - Calling a function of quickjs-libc.c from the Go API or a test, or looking for js_init_module_std, js_std_loop and the like in lib
  - Asking why a program that uses this module is smaller than with modernc.org/quickjs, or why it does not link fork, exec or popen
---

# Left out of the engine

## quickjs-libc.c keeps only what the Go API calls

Release: v0.27.0.

`quickjs-libc.c` is the C file of QuickJS's shell, `qjs`: the `std`, `os` and `Worker` modules, the event loop and a few helpers. Only `js_init_module_std` and `js_init_module_os` register the modules, and the Go API calls neither, so no script run through it can reach them. They were in every program all the same: ccgo writes the tables of the modules as package variables that hold the addresses of their functions, and the Go linker keeps whatever a package variable refers to, and with those functions the parts of libc they call, such as `fork`, `execve`, `popen`, `setenv`, the file functions and the terminal ones.

`fixlib` now drops from each generated file the functions and variables of `quickjs-libc.c` that six functions do not reach, with the `init` functions that fill the dropped variables (`fixQuickJSLibc` in `internal/fixlib/quickjslibc.go`). The six are `quickjsLibcRoots`:

| Function | Who calls it |
| --- | --- |
| `js_std_add_helpers` | `VM.StdAddHelpers` (`console.log`, `print`, `performance.now`, `__loadScript`) |
| `js_module_set_import_meta` | `VM.EvalBytecode` of a module |
| `js_module_loader` | `VM.SetDefaultModuleLoader` |
| `js_load_file`, `js_module_test_json`, `js_std_dump_error` | the test262 runner, through `lib` |

They reach 10 more, which stay too: `js_loadScript`, `js_module_loader_so`, `json_module_init`, `create_json_module`, `js_print_value_write`, `get_time_ns`, `js_os_now`, `js_print`, `js_console_log` and `js_std_dump_error1`. `VM.EvalBytecode` waits for a module's top-level await in Go (see [go-api.md](go-api.md#evalbytecode-waits-for-a-modules-top-level-await-in-go)), so `js_std_await` goes too. 149 functions and variables go (134 on windows, which has no `exec`, `pipe`, `kill` and the like), among them `js_init_module_std`, `js_init_module_os`, every `js_std_file_*`, `js_os_*` but `js_os_now`, and `js_worker_*`, `js_std_await`, `js_std_loop`, `js_std_init_handlers` and `js_std_eval_binary`. `lib` so no longer exports 11 functions: `js_init_module_std`, `js_init_module_os`, `js_module_check_attributes`, `js_std_set_worker_new_context_func`, `js_std_init_handlers`, `js_std_free_handlers`, `js_std_promise_rejection_tracker`, `js_std_await`, `js_std_loop`, `js_std_eval_binary` and `js_std_eval_binary_json_module`. The constants and types of the file stay, as they cost a program nothing.

ccgo writes the functions of each C file in the order of the file, and the C files one after the other, `quickjs-libc.c` last. So the part of a generated file between `Xrqsort`, the last function of `cutils.c`, and `__ccgo_fp`, which ccgo writes after the last file, is the whole of `quickjs-libc.c`. Its function names are the C file's at QuickJS `3d5e064`, apart from the `init` functions ccgo adds, and the three windows-only ones (`realpath`, `setenv`, `unsetenv`) that only `ccgo_windows.go` has.

Why it is safe: the dropped code was never called. A program that called these functions of `lib` itself, to register `std` or `os` or run the event loop, no longer builds. The Go API's helpers, module loader and module bytecode give what they gave, and the test262 runner gives the same 58 known errors.

A program that only evaluates javascript (`testdata/evalonly`) is 370 KB smaller on darwin/arm64 and 354 KB smaller on linux/amd64, both built on a Mac (from 7,331,282 to 6,961,074 bytes and from 8,288,179 to 7,934,135). The server of tifa's example shop on linux/amd64 is 181 KB smaller built as `tifa build` builds it, with `-ldflags=-w` and the shop's PGO profile (from 22,629,467 to 22,448,606 bytes), and 185 KB smaller with `-ldflags=-w` and `-pgo=off` (from 22,449,785 to 22,264,828). No dropped function was called during a render, so render times are not affected and were not measured.

Generated Go only, no C hunk: the C files keep `quickjs-libc.c`, and `fixlib` drops it again after every regeneration.

Tests: `quickjslibc_test.go` (`StdAddHelpers`, `SetDefaultModuleLoader` with a script and a JSON module, the bytecode of a module with a top-level `await` and `import.meta`, and a program that only calls `Eval` linking none of the dropped functions, among them `js_os_exec` and `js_std_popen`, which call `fork`, `execve` and `popen`, nor libc's `pclose`, `setenv`, `unsetenv`, `waitpid`, `pipe`, `kill`, `tcsetattr`, `symlink` and `tmpfile`. The compiler inlines `fork`, `execve` and `popen` into their callers, so `go tool nm` cannot show them gone, only their callers), `internal/fixlib/quickjslibc_test.go` (what stays on every target, the files of libquickjs v0.13.1 trimmed to the committed text when the module cache has them, the changes that stop `fixlib`, and the three calls of the test262 runner into `lib`) and [test262](../testing.md#test262).

### When upstream changes it

- **The functions and variables of quickjs-libc.c changed.** Upstream added, removed or renamed one. `fixlib` prints the fingerprint of the names of the part as upstream writes them, which `quickjsLibcSites` (`internal/fixlib/quickjslibcsites.go`) holds for each file. Run `go run ./internal/fixlib -qjslibc` on the files as taken from upstream: it prints the map and, for each file, what stays and what goes. Read the new functions. If the Go API or the test262 runner calls one, add it to `quickjsLibcRoots`. Then put the new map in.
- **What stays of quickjs-libc.c changed.** A function that stays, or one it reaches, has new text, or a function that stays calls one it did not call before. Read the change: what stays must still run without `js_std_init_handlers`, whose state (`JS_GetRuntimeOpaque`) the Go API never sets. Then run `-qjslibc` as above.
- **A name used outside what stays.** Code outside `quickjs-libc.c`, or kept code `fixlib` does not know, uses a function that goes: add it to `quickjsLibcRoots` if a caller needs it.
- **No function `Xrqsort`, or `__ccgo_fp` before it.** ccgo wrote the files in another order, or `cutils.c` has another last function: find where `quickjs-libc.c` starts and ends, and update `quickjsLibcAfter` or `quickjsLibcEnd`.
- **An init function fills something that is not a variable of quickjs-libc.c.** ccgo started filling a variable of another C file, or a function, from the part of `quickjs-libc.c`. `fixlib` reaches an `init` function only through the variable it fills, so it would drop it while the variable stays: find where ccgo now writes it, and have `quickjsLibcDecls` keep it.
- **A function of quickjsLibcRoots is gone.** Upstream renamed or removed it: change the caller in the Go API or the runner, then the list.
