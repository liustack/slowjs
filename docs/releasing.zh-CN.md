---
summary: '怎么发布一个版本：只发补丁版本，在 main 的合并提交上打轻量标签，打标签和推送前先问，发布前要更新什么，以及发布后重新运行和其他 Go 引擎的对比，并把 tifa 升级到新版本'
read_when:
  - 一个分支已合并进 main，准备发布
  - 选下一个版本的版本号
  - 要创建或推送标签
  - 版本发出去了，tifa 应该用上它
  - 重新运行和其他 Go 版 JavaScript 引擎的对比
---

# 发布

[English](releasing.md) · 简体中文

## 版本

- 发布的是补丁版本：v0.30.0、v0.30.1，依此类推。一个修复或一次提速，哪怕很大，也是下一个补丁版本。删除或改变公开 API 的版本是下一个次版本，v0.27.0 就是这样，它删除了模块加载器。别的情况都不要升到新的次版本：那由仓库的所有者决定。
- `github.com/liustack/quickjs` 没有 GitHub releases。一个版本就是一个标签，用户用 `go get github.com/liustack/quickjs@<version>` 获取。
- 从 v0.28.0 到 v0.29.11 的版本在 `github.com/liustack/slowjs`，其余都在 `github.com/liustack/quickjs`。Go 模块代理会继续在那里提供旧版本，v0.29.11 的 `go.mod` 把该模块标为已弃用，并写出了新路径。改名的经过见 [upstream.md](upstream.zh-CN.md#从-githubcomliustackslowjs-换过来)。

## 发布前

在分支上，合并之前：

1. [testing.md](testing.zh-CN.md#提交前) 里的检查通过，分支涉及的部分的检查也通过（重点检查、test262、`make build_all_targets`）。`make check-full` 在装了 LLVM 22.1.8 的机器上通过：提交进仓库的引擎就是生成器生成的，根包、竞态检测器、sanitizer 和 test262 都通过。`scripts/go-versions.sh` 通过：从 `go.mod` 里最低的 Go 起，用模块允许构建的每个 Go 补丁版本运行引擎的检查（Go 测试，在装了 LLVM 的机器上还有生成器的测试和链接后检查，见 [testing.md](testing.zh-CN.md#ci)）。
2. 每个改动都写在 [changes](changes/README.zh-CN.md) 下对应领域的文档里，并在索引里有一行，为速度做的改动还写在 [performance.md](performance.zh-CN.md) 里。
3. [CHANGELOG.md](../CHANGELOG.zh-CN.md) 和 [CHANGELOG.zh-CN.md](../CHANGELOG.zh-CN.md) 写明这个分支对用户改变了什么。分支的最后一个提交把它放到新版本号和日期之下（`docs(changelog): release <version>`），这样你要打标签的合并提交里就带着它。
4. 在 [changes](changes/README.zh-CN.md) 里，这个版本的改动仍写着"Unreleased"。在索引的表格里和每个领域文档的 `Release:` 行里，把它换成新版本号（`grep -rn Unreleased docs/changes`，"Adding a change"下面的那句话不用管），和更新记录放在同一个最后的提交里。没有别的东西会做这件事，打了标签之后仍写着"Unreleased"的一行，从那时起就是错的。
5. 性能画像在要紧的地方没有过期：见下面的[性能画像](#性能画像)。

然后用合并提交把它合并进 `main`，信息是 `merge: <分支做了什么>`（见 [commit.md](commit.zh-CN.md#分支和合并)）。

## 性能画像

引擎用一份页面渲染的性能画像（profile）生成（`engine/pgo/profile.txt`，见 [engine/pgo/README.zh-CN.md](../engine/pgo/README.zh-CN.md)）。训练之后 C 代码改过的函数没有可用的画像，编译时就像没有画像一样。引擎的其余部分仍保有收益。所以旧画像只会丢掉一部分收益，其他都不会坏。

`make check-full` 会打印这份清单：生成器以 `warning: profile:`、`profile:` 和 `profile ` 开头的行，每个架构各一份，在 `gen` 步骤的输出里。`scripts/check-gen.sh` 单独运行也打印同样的行。发布前读一遍：

- 清单里有 `JS_RegvmRun` 就重新训练。它是寄存器解释器，没有它的画像只剩 60% 到 80% 的收益（在 tifa 的页面上测得）。
- 生成器说超过 30 个函数的 C 改过，就重新训练。画像已经旧了。
- 清单上只有几个别的函数，不用处理。想把它们的收益拿回来时再训练。

重新训练，在装有 LLVM 22.1.8 的机器上：

```sh
engine/pgo/train.sh
make gen && make gen ARCH=arm64
git add engine/pgo/profile.txt internal/cengine
make check-gen
git commit -m "build(pgo): train the profile again"
```

`train.sh` 检查 `engine/pgo/loops.js` 不过时会停下：循环跑到的、页面留成冷的路径不够了，或者循环的某一部分不再走到它的函数。怎么办见 [engine/pgo/README.zh-CN.md](../engine/pgo/README.zh-CN.md#循环)。不要把 `compare/testdata` 的微基准放进循环。

新画像和它改动的文件放进同一个提交。然后再跑一次 `make check-gen`：不应再有 `warning: profile:` 开头的行（说 `likely()` 的那行 `profile:` 是提示，不是警告）。取入上游 QuickJS 的提交之后（[upgrading.md](upgrading.zh-CN.md)），以及换了 LLVM 版本之后（画像格式不同），也要重新训练。

## 打标签

创建标签之前要问所有者，推送之前再问一次。推送出去的标签立刻就是公开的，Go 模块代理会永久保留它：收不回来，只能被下一个版本取代。

1. 在 `main` 的合并提交上打一个轻量标签，和到目前为止的每个版本一样：

   ```sh
   git log --first-parent --oneline -3 main    # the merge commit to tag
   git tag <version> <merge commit>
   git cat-file -t <version>                    # commit, not tag
   ```

2. 推送 `main` 和标签：

   ```sh
   git push origin main
   git push origin <version>
   ```

3. 检查代理上有了它：`GOPROXY=https://proxy.golang.org go list -m github.com/liustack/quickjs@<version>`。

## 发布后

### 重新和其他引擎对比

次版本发布时，重新运行和其他 Go 的 JavaScript 引擎以及原生引擎的对比（[benchmarks.md](benchmarks.zh-CN.md)）。补丁版本跳过它，除非所有者要求：引擎每周都在变，每个补丁都出一张新表，要花机器好几个小时，说明的东西却很少。一次运行是两张表：tifa 页面的渲染时间和 V8 套件，只在解释器之间比（带 JIT 的引擎关掉 JIT 来跑，或者不比）。它在 `compare/engines` 里，是一个独立的 Go 模块，在没有别的任务的四核 Linux 机器上运行。命令见 [compare/engines/README.zh-CN.md](../compare/engines/README.zh-CN.md)。

1. 让对比指向这个版本，如果其他引擎的最新版本该进这次运行，就更新它们：

   ```sh
   cd compare/engines
   go get <module>@latest && go mod tidy      # 可选，对每个其他引擎
   ```

   `compare/engines` 通过一条指向仓库根的 `replace` 引用本模块，所以运行测的是它所在的检出：先检出这个标签。

2. 构建各引擎和原生参照（`native/build.sh`），按 README 所说运行：tifa 页面每次调用一个引擎，V8 套件五轮。每次计时的调用独占整台机器，钉在一个核上。所有的计时都在同一次会话里重新跑。对比从不在其他引擎上运行 test262。机器是共用的话，用 `-add` 把运行切成几段。
3. 用 `go run ./cmd/pages "$page_dir" --output "results/$run-pages"` 写页面文件，和这次运行的 V8 文件一起提交。同一个提交里删掉上一次运行的结果文件：`results/` 只留最近一次。更新 [benchmarks.md](benchmarks.zh-CN.md) 的表格。
4. 如果方法变了（负载、引擎、固定的输入、某项设置），在 `compare/engines/CHANGELOG.md` 里加一条。

### 升级 tifa

liustack/quickjs 是为 tifa 而存在的，所以每个版本接下来都要进 tifa：

1. 在 tifa 里运行 `go get github.com/liustack/quickjs@<version>` 和 `go mod tidy`。
2. 把每个改动合并之前测得的数字，放进 tifa 的 docs/performance.md 和它的更新记录。不要把整个版本再测一遍：每个改动都已经对着 main 测过了。只有在所有者要求，或者这个版本的两个改动可能互相影响时，才在 tifa 的页面上测（[Measuring in tifa](performance.zh-CN.md#在-tifa-里测量)）。
3. 运行 tifa 的检查。SSR 浸泡测试（`TIFA_SSR_SOAK=1`）也覆盖内存耗尽和 VM 复用。
