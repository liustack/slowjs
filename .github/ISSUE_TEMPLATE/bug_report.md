---
name: Bug report
about: A script gave a wrong answer, crashed, ran slow, or a limit did not hold
title: ''
labels: bug
assignees: ''
---

A security problem (memory outside the VM, a crash of the Go process, a limit a script got past) goes in a [private report](https://github.com/liustack/quickjs/security/advisories/new), not here.

## What happened

A clear description of the problem.

## The smallest code that shows it

The Go code and the JavaScript, cut down as far as it still shows the problem.

```go
vm, _ := quickjs.NewVM()
defer vm.Close()
v, err := vm.Eval(`...`, quickjs.EvalGlobal)
```

## The full output

Paste the complete output or error, not a paraphrase. For a crash, the whole Go stack trace.

```
...
```

## Environment

- liustack/quickjs version (`go list -m github.com/liustack/quickjs`):
- `go version`:
- OS and CPU (`go env GOOS GOARCH`, and the CPU model):
- Other packages registered on the VM (`intl`, `temporal`, `fmt`):

## Expected

What you expected instead. If Node or C QuickJS gives a different answer for the same script, say which and what it gives.

---

Not sure it is a bug, or just want to talk it through? Find me on X: [@liustack](https://x.com/liustack).
