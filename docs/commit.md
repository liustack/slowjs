---
summary: 'Commit rules: commit message format, the type list, atomic commits, changes to the generated Go, branches and merge commits'
read_when:
  - Getting ready to commit
  - Writing a commit message or a merge message
  - Splitting up a set of changes
  - Committing a change to internal/fixlib together with the lib/ files it rewrites
---

# Commit rules

## Format

```
<type>[scope]: <imperative summary>

<optional body: why the change was made, and any trade-offs that are not obvious>
```

- The summary is at most 72 characters, with no period at the end.
- type: `feat` `fix` `docs` `style` `refactor` `perf` `test` `build` `ci` `chore` `revert`.
- scope is the affected package or area, such as `lib`, `fixlib`, `intl`, `vm`, `patch`, `readme`.
- For a breaking change, add `!` after the type, or write `BREAKING CHANGE:` at the end of the body.
- Do not use semicolons in commit messages, and do not use dashes as punctuation.
- No signature or co-author lines.

Examples from the history:

```
fix(lib): make javascript's local time UTC whatever zone the process has
perf(fixlib): count the interrupt counter down inside the interpreter
feat(fixlib): pin JS_GetOwnPropertyInternal, which gets the descriptor
build!: rename the module to github.com/liustack/quickjs
```

## Atomic commits

- One commit does one thing, and can be verified and reverted on its own.
- A behavior change and its tests go in the same commit.
- Mechanical changes such as pure refactors, formatting and renames are committed separately, not mixed with behavior changes.
- A change to `internal/fixlib` goes in the same commit as the `lib/ccgo_*.go` and `lib/internal/run-test262/ccgo_*.go` files it rewrites, so that `go run ./internal/fixlib -check` passes at every commit. Never edit the generated files by hand.
- A change to the engine that has a C form also updates `lib/internal/patch/quickjs.diff` (see [upgrading.md](upgrading.md#the-c-patch)).
- Do not commit build or test output: `*.test`, `mem.out`, the test262 and QuickJS archives and the `test262/` folder `lib/` unpacks, `go.work`. They are in `.gitignore`.
- Do not use summaries like "WIP" or "fix stuff" that do not say what changed.

## Branches and merges

- Do not develop directly on `main`. Run `git switch -c <branch>` first.
- `master` and the other upstream branches mirror gitlab.com/cznic/quickjs and get no commits of their own.
- A branch goes into `main` with a merge commit (`git merge --no-ff`), whose message is `merge: <what the branch does>`, such as `merge: count interrupts down inline, cache small blocks and keep shapes`. `git log --first-parent main` then reads as the list of changes, and releases are tagged on these merge commits (see [releasing.md](releasing.md)).
- Before committing, run the checks in [testing.md](testing.md#before-committing).
