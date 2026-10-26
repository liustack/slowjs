// Command jsrun runs scripts on the Go engine as engine/test/jsrun.c runs
// them on the C tree, and prints what they give in the same form. Its output
// is what engine/test/compare.sh compares the C tree with when COMPARE_GO=1
// asks for it. The repository holds files for the C tree to be compared with
// when the Go engine is gone.
//
//	go run ./internal/jsrun [-result] [-call] [-regexp-cache=N,BYTES] FILE...
//
// Each file runs as a global script in a VM of its own, then the jobs it
// queued run. jsrun prints "== " and the name of the file, then what the
// script gave: the string of its completion value, or of what that value
// returns when it is a function (called after the jobs), or the exception.
// A file whose name ends in .mjs runs as a module instead, and gives
// globalThis.result. -result gives globalThis.result for a script too, and
// -call takes the file as the body of a function that the host calls (see
// engine/test/jsrun.c). The scripts find the Builtins GetAsyncContext and
// SetAsyncContext as the globals getAsyncContext and setAsyncContext, and
// gc, youngGC, memorySize and setGCThreshold, which run VM.RunGC and
// VM.RunYoungGC, give VM.MemorySize and set VM.SetGCThreshold (to its largest
// value when the argument is negative).
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/liustack/slowjs"
)

// The options of the command line, for every file.
var (
	optResult bool
	optCall   bool
	// regexpCache is the setting of -regexp-cache: the entries and the
	// bytes, or nil for the default of the VM.
	regexpCache []int
)

// formatSrc is what both runners call with the completion value. A lone
// surrogate is printed as \u{...}: Go would turn it into U+FFFD.
const formatSrc = `(v) => {
  const s = typeof v === 'function' ? String(v()) : String(v)
  if (s.isWellFormed()) return s
  let r = ''
  for (const c of s) {
    const u = c.charCodeAt(0)
    r += c.length === 1 && u >= 0xd800 && u <= 0xdfff ? '\\u{' + u.toString(16) + '}' : c
  }
  return r
}`

func main() {
	args := os.Args[1:]
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && len(args[0]) > 1 {
		switch arg := args[0]; {
		case arg == "-result":
			optResult = true
		case arg == "-call":
			optCall = true
		case strings.HasPrefix(arg, "-regexp-cache="):
			n, bytes, ok := strings.Cut(strings.TrimPrefix(arg, "-regexp-cache="), ",")
			a, err1 := strconv.Atoi(n)
			b, err2 := strconv.Atoi(bytes)
			if !ok || err1 != nil || err2 != nil {
				fmt.Fprintf(os.Stderr, "jsrun: %s: want N,BYTES\n", arg)
				os.Exit(2)
			}
			regexpCache = []int{a, b}
		default:
			fmt.Fprintf(os.Stderr, "jsrun: unknown option %s\n", arg)
			os.Exit(2)
		}
		args = args[1:]
	}
	for _, path := range args {
		fmt.Printf("== %s\n", filepath.Base(path))
		out, err := run(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "jsrun:", err)
			os.Exit(1)
		}
		fmt.Print(out)
	}
}

func run(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	vm, err := slowjs.NewVM()
	if err != nil {
		return "", err
	}
	defer vm.Close()
	if err := defineGlobals(vm); err != nil {
		return "", err
	}
	if regexpCache != nil {
		if err := vm.SetRegExpCache(regexpCache[0], regexpCache[1]); err != nil {
			return "", err
		}
	}
	module := strings.HasSuffix(path, ".mjs")
	var v slowjs.Value
	switch {
	case module:
		// The file is the module NAME, read from its folder, and a module of
		// <eval> imports it (see engine/test/jsrun.c).
		dir, name := filepath.Dir(path), filepath.Base(path)
		vm.SetModuleLoader(func(_ *slowjs.VM, n string) (string, error) {
			b, err := os.ReadFile(filepath.Join(dir, n))
			return string(b), err
		}, nil)
		v, err = vm.EvalValue(fmt.Sprintf("import %q;", name), slowjs.EvalModule)
	case optCall:
		// The body of a function that the host calls.
		if _, err = vm.Eval("function run() {\n"+string(src)+"\n}", slowjs.EvalGlobal); err == nil {
			v, err = vm.CallValue("run")
		}
	default:
		v, err = vm.EvalValue(string(src), slowjs.EvalGlobal)
	}
	if err != nil {
		return fmt.Sprintf("<exception=%s>\n", err), nil
	}
	defer func() { v.Free() }() // v is replaced below when it is globalThis.result
	out := ""
	if _, err := vm.ExecutePendingJobs(); err != nil {
		out = fmt.Sprintf("<job exception=%s>\n", err)
	}
	if module || optResult || optCall {
		v.Free()
		if v, err = vm.EvalValue("globalThis.result", slowjs.EvalGlobal); err != nil {
			return out + fmt.Sprintf("<exception=%s>\n", err), nil
		}
	}
	format, err := vm.EvalValue(formatSrc, slowjs.EvalGlobal)
	if err != nil {
		return out + fmt.Sprintf("<format=%s>\n", err), nil
	}
	defer format.Free()
	s, err := format.Call(slowjs.UndefinedValue, v)
	if err != nil {
		return out + fmt.Sprintf("<exception=%s>\n", err), nil
	}
	str, ok := s.(string)
	if !ok {
		return "", fmt.Errorf("%s: the format gave %T", path, s)
	}
	return out + str + "\n", nil
}

// defineGlobals gives the global object of vm what engine/test/jsrun.c
// gives it.
func defineGlobals(vm *slowjs.VM) error {
	for name, f := range map[string]any{
		"gc":         vm.RunGC,
		"youngGC":    vm.RunYoungGC,
		"memorySize": func() float64 { return float64(vm.MemorySize()) },
		"setGCThreshold": func(arg any) {
			// A number from javascript is an int or a float64.
			n, _ := arg.(float64)
			if i, ok := arg.(int); ok {
				n = float64(i)
			}
			if n < 0 || n >= math.MaxUint64 {
				vm.SetGCThreshold(^uintptr(0))
			} else {
				vm.SetGCThreshold(uintptr(n))
			}
		},
	} {
		if err := vm.RegisterFunc(name, f, false); err != nil {
			return err
		}
	}
	define, err := vm.EvalValue(`(get, set) => {
  globalThis.getAsyncContext = get
  globalThis.setAsyncContext = set
}`, slowjs.EvalGlobal)
	if err != nil {
		return err
	}
	defer define.Free()
	get, err := vm.NewBuiltinFunction(slowjs.GetAsyncContext)
	if err != nil {
		return err
	}
	defer get.Free()
	set, err := vm.NewBuiltinFunction(slowjs.SetAsyncContext)
	if err != nil {
		return err
	}
	defer set.Free()
	r, err := define.CallValue(slowjs.UndefinedValue, get, set)
	if err != nil {
		return err
	}
	r.Free()
	return nil
}
