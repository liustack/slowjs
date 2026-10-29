---
summary: 'What the engine leaves out because the Go API can never reach it: the std, os and Worker modules and the event loop of QuickJS shell, and why a program that uses this module is smaller and links no code that forks, execs or opens files'
read_when:
  - Looking for js_init_module_std, js_std_loop, js_os_exec and the like in the engine
  - Asking why a program that uses this module does not link fork, exec or popen
---

# Left out of the engine

## No shell library

`quickjs-libc.c` is the C file of QuickJS's shell, `qjs`: the `std`, `os` and `Worker` modules, the event loop and a few helpers. The C tree in `engine/` does not have it. The Go API registers no such module, so no script run through it could reach one, and the Go API gives what the shell gave a script in Go: `console`-like helpers are the program's to add as [Builtins](go-api.md#builtin-a-go-function-called-as-a-built-in), the module loader is the program's, and a module's top-level `await` is waited for in Go (see [go-api.md](go-api.md#evalbytecode-waits-for-a-modules-top-level-await-in-go)).

What the engine still needs from that file is small and is written in `engine/` or in the Go host: the printing of an error for `JS_PrintValue` (only with `CONFIG_PRINT_VALUE`, which the native test host sets), `import.meta` of a module (set by the Go host) and the loading of a file for the native test host (`engine/test/host.c`).

Why it is safe: the shell code was never called by the Go API. Module bytecode gives what it gave, and test262 gives the same 58 known errors.

A program that only evaluates javascript links nothing that calls `fork`, `execve`, `popen`, `pipe`, `kill`, `waitpid` or `tcsetattr`: the engine asks its host for the callbacks of `engine/host.h` and nothing else (`engine/test/check-symbols.sh` checks the list of symbols the engine may use), and the Go host links no libc.
