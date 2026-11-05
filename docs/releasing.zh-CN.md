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

然后用合并提交把它合并进 `main`，信息是 `merge: <分支做了什么>`（见 [commit.md](commit.zh-CN.md#分支和合并)）。

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

次版本发布时，重新运行和其他纯 Go JavaScript 引擎的对比（[benchmarks.md](benchmarks.md)）。补丁版本跳过它，除非所有者要求：引擎每周都在变，每个补丁都出一张新表，要花机器好几个小时，说明的东西却很少。它在 `compare/engines` 里，是一个独立的模块，在一台 Linux 机器上运行，为它留出一个 CPU 和它的超线程兄弟（下面的 CPU 1 和 CPU 5）：

1. 让对比指向这个版本，如果其他引擎的最新版本应该参加这次运行，就更新它们：

   ```sh
   cd compare/engines
   go get <module>@latest && go mod tidy      # optional, for each other engine
   ```

   `compare/engines` 通过对仓库根目录的 `replace` 找到这个模块，所以这次运行测的是它所在的检出：先检出那个标签。

2. 运行每个工作负载，五轮，以及带参考行的 tifa 页面：

   ```sh
   run="<date>-<version>-<host>"
   go run ./cmd/compare -cpu 1 -sibling 5 -name "$run"
   tifa/polyfill/build.sh && native/build.sh "$HOME/jsnative"
   page_dir=$(mktemp -d "/tmp/$run-pages.XXXXXX")
   tifa/run.sh "$page_dir" bin "$HOME/jsnative"    # TIFA_ONLY=a,b runs only some engines
   go run ./cmd/behind "results/$run.json"
   ```

   带 `-cpu 1 -sibling 5` 时，驱动程序把计时的进程钉在 CPU 1 上，GOMAXPROCS=1。`tifa/run.sh` 把功能检查（正常、桩和代理）和计时的进程都钉在 CPU 1 上，GOMAXPROCS=1，TZ=UTC。两个阶段用同样限定 4 GiB 的入口，因 CPU 干扰最多允许三次尝试。二进制用 `-pgo=off` 构建。在共用的机器上，用 `-add` 把运行切成几段，并把 test262 放到另一个 CPU 上以低优先级运行，如 [benchmarks.md](benchmarks.md#rerun) 所示。当某个引擎在 shop 页面上失败并关闭了它的运行时，把其他页面放在单独的一次运行里测，同一节也这样说。

3. 在任何补测运行或单个页面的探测之后，用一条命令写出并提交两个页面文件：

   ```sh
   go run ./cmd/pages "$page_dir" --output "results/$run-pages" &&
     git add "results/$run-pages.md" "results/$run-pages.json" &&
     git commit -m "docs(bench): record $run pages" -m "Archive page times and normalized HTML digests so equality can be checked from the saved results."
   ```

   JSON 保存原始的进程结果、补测运行、探测、尝试次数和每个页面的 HTML 比较。也提交 `results/<run>.json` 和 `results/<run>.md`，更新 [benchmarks.md](benchmarks.md) 的表格和引擎远远落后之处的列表，并把这次运行加进它的 History。
4. 如果方法变了（一个工作负载、一个引擎、一个钉住的输入、一个设置），在 `compare/engines/CHANGELOG.md` 里加一条。

### 升级 tifa

liustack/quickjs 是为 tifa 而存在的，所以每个版本接下来都要进 tifa：

1. 在 tifa 里运行 `go get github.com/liustack/quickjs@<version>` 和 `go mod tidy`。
2. 把每个改动合并之前测得的数字，放进 tifa 的 docs/performance.md 和它的更新记录。不要把整个版本再测一遍：每个改动都已经对着 main 测过了。只有在所有者要求，或者这个版本的两个改动可能互相影响时，才在 tifa 的页面上测（[Measuring in tifa](performance.zh-CN.md#在-tifa-里测量)）。
3. 运行 tifa 的检查。SSR 浸泡测试（`TIFA_SSR_SOAK=1`）也覆盖内存耗尽和 VM 复用。
