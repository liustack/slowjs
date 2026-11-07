# 参与 liustack/quickjs

[English](CONTRIBUTING.md) · 简体中文

先说规矩：**liustack/quickjs 不接受 pull request。** 引擎、生成器和 Go API 的每一行都由一个维护者审读、负责到底，正是这样收紧的流程让它靠得住。

真正有用的参与方式有两种：

- **[提一个 issue](https://github.com/liustack/quickjs/issues/new/choose)。** 脚本算错了，渲染慢，限制没拦住，报错看不懂，文档写得不清楚，都可以提。每个 issue 都会读，它决定下一步做什么。模板里写了要附上哪些信息。中文英文都行。
- **Fork 一份。** liustack/quickjs 自己写的文件用 MIT 许可，来自其他项目的代码保留各自的许可（见 [NOTICE.md](NOTICE.md)）。你的那份完全归你，想怎么改、怎么发布都行。

安全问题请私下报告，不要开公开 issue，见 [SECURITY.zh-CN.md](SECURITY.zh-CN.md)。

下面的内容写给在 fork 上开发的人。

## 范围

liustack/quickjs 是给 Go 程序用的 JavaScript 引擎，为 [tifa](https://github.com/liustack/tifa) 而建、为它维护。tifa 在 Go 进程里把 React 页面渲染成 HTML。它会一直保持：

- 纯 Go，用 `CGO_ENABLED=0` 构建，使用这个模块的人不需要 C 编译器，也不需要动态库
- 是引擎，不是 Node：没有 `require`、`fs`、`process`，也没有自己的事件循环。脚本需要什么，由宿主通过 Go API 提供
- 是固定提交的 QuickJS，加上 [docs/changes](docs/changes/README.zh-CN.md) 里逐条写明的改动。上游 QuickJS 的提交一次合入一个（见 [docs/upgrading.zh-CN.md](docs/upgrading.zh-CN.md)）
- 在每台机器上输出都一样，为速度做的改动不改变引擎给出的结果

## 准备

```sh
go build ./...
go vet ./...
scripts/docs-list.sh --check
go test -count=1 -p 1 .   # 大约一分钟
```

需要 Go 1.26.6 或更高版本。从 `engine/` 的 C 代码重新生成引擎（`make gen`）需要 LLVM 22.1.8，`make check-full` 会跑全部检查，包括 test262 和 sanitizer。详见 [docs/testing.zh-CN.md](docs/testing.zh-CN.md)。

## 测试

- 新行为先写一个会失败的测试，修 bug 要带上能复现这个 bug 的测试。
- 改了 `engine/` 的 C 代码，要在同一个提交里带上重新生成的 `internal/cengine` 文件。两者不一致时 `make check-gen` 会失败。
- test262 保持 33 个已知错误（`engine/test/test262_errors.txt`）。

## 提交

- 格式是 `type(scope): 祈使句摘要`，不超过 72 个字符，末尾不加句号。
- 一个提交只做一件事，每个提交之后都能构建、测试都能通过。

完整规则见 [docs/commit.zh-CN.md](docs/commit.zh-CN.md)。代码的全貌见 [docs/architecture.zh-CN.md](docs/architecture.zh-CN.md)，在代码上干活的规则见 [AGENTS.md](AGENTS.md)。
