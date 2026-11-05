---
summary: '提交规则：提交信息的格式，type 列表，原子提交，对 C 引擎及其生成文件的改动，分支和合并提交'
read_when:
  - 准备提交
  - 写提交信息或合并信息
  - 拆分一组改动
  - 把对 engine/ 的改动和 internal/cengine 的生成文件一起提交
---

# 提交规则

[English](commit.md) · 简体中文

## 格式

```
<type>[scope]: <imperative summary>

<optional body: why the change was made, and any trade-offs that are not obvious>
```

- 摘要最多 72 个字符，末尾不加句号。
- type：`feat` `fix` `docs` `style` `refactor` `perf` `test` `build` `ci` `chore` `revert`。
- scope 是受影响的包或领域，比如 `engine`、`gen`、`intl`、`vm`、`compare`、`readme`。
- 破坏性变更在 type 后面加 `!`，或者在正文末尾写 `BREAKING CHANGE:`。
- 提交信息里不用分号，也不用破折号作标点。
- 不加签名或共同作者行。

历史里的例子：

```
fix(engine): make javascript's local time UTC whatever zone the process has
perf(engine): count the interrupt counter down inside the interpreter
feat(gen): refuse a call the stack check does not cover
build!: rename the module to github.com/liustack/quickjs
```

## 原子提交

- 一个提交只做一件事，能单独验证，也能单独回退。
- 行为变更和它的测试放在同一个提交里。
- 纯重构、格式化和改名这类机械性的改动单独提交，不和行为变更混在一起。
- 对 `engine/` 的 C、`internal/goasm/gen` 下的生成器，或者 `scripts/gen-engine.sh` 或 `engine/test/flags.sh` 的参数的改动，要和 `make gen` 与 `make gen ARCH=arm64` 由它生成的 `internal/cengine` 的文件放在同一个提交里，这样每个提交上 `make check-gen` 都能通过。绝不手工编辑生成的文件。
- 不要提交构建或测试的输出：`*.test`、`mem.out`、test262 的归档，目录 `engine/test/out` 和 `.build`，以及 `go.work`。它们在 `.gitignore` 里。
- 不要用"WIP"或"fix stuff"这样没说明改了什么的摘要。

## 分支和合并

- 不要直接在 `main` 上开发。先运行 `git switch -c <branch>`。
- 分支用合并提交（`git merge --no-ff`）进入 `main`，信息是 `merge: <分支做了什么>`，比如 `merge: count interrupts down inline, cache small blocks and keep shapes`。这样 `git log --first-parent main` 读起来就是改动的列表，版本就打在这些合并提交上（见 [releasing.md](releasing.zh-CN.md)）。
- 提交之前，运行 [testing.md](testing.md#before-committing) 里的检查。
