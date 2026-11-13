# engine/pgo

[English](README.md) · 简体中文

`make gen` 用性能画像（profile）编译引擎所需的东西：画像本身，训练它的程序，以及训练用的页面。画像对生成的代码做了什么，带来多少收益，在哪里止步，见 [docs/changes/c-engine.zh-CN.md](../../docs/changes/c-engine.zh-CN.md#用性能画像生成引擎)。

## 这里有什么

| 路径 | 内容 |
| --- | --- |
| `train.sh` | 训练画像。按生成器编译引擎的方式构建带计数器的引擎，渲染页面，跑微基准，写出 `profile.txt` |
| `profile.txt` | 画像，用 `llvm-profdata` 的文本格式。amd64 和 arm64 共用一份：函数的哈希只取决于 C 代码。同样的训练跑两次，文件里有几个计数不同，所以文件入库，`make gen` 从不训练 |
| `micro.txt` | 训练时 `compare/testdata` 的每个微基准跑多少次（它脚本里的 `$N`），让页面从不走的路径也有计数 |
| `pages/` | 训练渲染的页面（见下） |

## 重新训练

在装有 LLVM 22.1.8 的 Linux 或 macOS 上（`LLVM_BIN` 是它的 bin 目录，默认 `/usr/lib/llvm-22/bin`）：

```sh
LLVM_BIN=/opt/homebrew/opt/llvm@22/bin engine/pgo/train.sh   # 约 30 秒
make gen && make gen ARCH=arm64
git add engine/pgo/profile.txt internal/cengine
```

`LLVM_BIN` 里的 clang 不是 22.1.8 时 `train.sh` 会停下。它在 `.build/pgo-train` 里工作（`TRAIN_OUT` 可改）。它不需要仓库之外的任何东西。画像和生成的文件放进同一个提交，`make check-gen` 要求如此。什么时候该重新训练，见 [docs/releasing.zh-CN.md](../../docs/releasing.zh-CN.md#性能画像)。

## 页面

训练渲染 [tifa](https://github.com/liustack/tifa) 两个应用的真实页面，跑在原生测试宿主上（`engine/test/render.c`，`tifa=1`：它像 tifa 一样定义 tifa 给每个 VM 的四个函数）。每个页面列表渲染两遍，第二遍开始时把计数器清零，所以画像里只有热渲染：没有加载 bundle，也没有第一次渲染。

| 路径 | 内容 | 大小 |
| --- | --- | --- |
| `pages/shop/shop.js` | tifa 的 `examples/shop` 的服务端 bundle，构建成 ES 模块，再由 `compare/engines/cmd/modulescript` 转成脚本。React、tifa 的运行时和应用用到的包 | 1.6 MB |
| `pages/shop/in/*.json` | shop 的 10 个页面的输入：工作台 3 个（`wb-studio`、`wb-order-studio`、`wb-stats-studio`），en-US 的商店 7 个（首页、商品列表、商品、日志、文章、购物车、结算） | 0.18 MB |
| `pages/mui/mui.js` | tifa 的 `test/apps/mui` 的服务端 bundle，做法相同。React、MUI 和 emotion | 0.76 MB |
| `pages/mui/in/home.json` | 该应用首页的输入 | 7 KB |
| `pages/fakeintl.js` | 原生宿主加载 bundle 需要的一个小 `Intl`，因为 C 引擎没有它（它来自 Go 包 `intl` 和 `temporal`）。它用 `toFixed` 格式化数字，日期写成 ISO 日期，复数只分 one 和 other，所以这几处的 HTML 和 tifa 的真实页面不同，Intl 内部的工作也不在画像里 |
| `pages/THIRD_PARTY_NOTICES.md` | 两个 bundle 里的包，带版本、许可证和许可证文本 |

输入是 tifa 交给 `startRender` 的那段 JSON 文本：`buildId`、`language`、`alternates`、`url`、`data`、`preload`、`session` 等，loader 的文本放进结果的 `json` 字段，没有 `nonce`。

### 重新做出它们

bundle 和输入来自 tifa 的提交 `07b6ce0f7e052558d47b8ff649623bebd047ae04`，包取自每个应用的 `pnpm-lock.yaml`，用 Go 1.27.2、Node 24.13.0 和 pnpm 10.33.0。用锁文件里的包构建，两个 bundle 得到的字节和这里的文件相同。

```sh
# 1. 取这个提交的干净 tifa，装好包
git clone https://github.com/liustack/tifa && cd tifa && git checkout 07b6ce0f7e052558d47b8ff649623bebd047ae04
(cd examples/shop && pnpm install --frozen-lockfile)
(cd test/apps/mui && pnpm install --frozen-lockfile)

# 2. bundle：<app>/.tifa/dist/ssr.js，再转成脚本形式
(cd examples/shop && go tool tifa build)
(cd test/apps/mui && go tool tifa build)
(cd /path/to/quickjs/compare/engines &&
  go run ./cmd/modulescript /path/to/tifa/examples/shop/.tifa/dist/ssr.js /path/to/quickjs/engine/pgo/pages/shop/shop.js &&
  go run ./cmd/modulescript /path/to/tifa/test/apps/mui/.tifa/dist/ssr.js /path/to/quickjs/engine/pgo/pages/mui/mui.js)

# 3. 声明文件，它读构建出的 source map
(cd /path/to/quickjs/compare/engines && go run ./cmd/notices /path/to/tifa ../../engine/pgo/pages)
```

`shop.js` 的 SHA-256 是 `0d2f9542f12c68959572a2dc3e8a0775a01244cfcc0f59778413e6911d3ea6a3`，`mui.js` 的是 `3c8d8c50d448f80b67e92f89228043c5b8cf7cae7f0268254c593e56f5dbcd1c`。bundle 和 `pages/` 里的不一致时，声明文件工具会停下。

记录输入时，把下面的文件作为 `zz_dump.go` 放进 tifa 的 `internal/ssr/`，并在 `internal/ssr/pool.go` 的 `renderInVM` 里，紧跟在 `encode input` 的错误检查之后加一行 `dumpInput(input, in.Texts)`：

```go
package ssr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// dumpInput writes the input of a render, with each text put into its result as `json`
// and without the nonce, so that a host that passes only the input to startRender can render it.
func dumpInput(input []byte, texts []string) {
	dir := os.Getenv("TIFA_DUMP_INPUT")
	if dir == "" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		panic(err)
	}
	delete(m, "nonce")
	data, _ := m["data"].(map[string]any)
	for _, v := range data {
		r := v.(map[string]any)
		if t, ok := r["text"]; ok {
			r["json"] = texts[int(t.(float64))]
			delete(r, "text")
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	name := strings.NewReplacer("/", "_", "?", "@", "&", "+", "=", "-").Replace(m["url"].(string))
	if err := os.WriteFile(filepath.Join(dir, name+".json"), out, 0o644); err != nil {
		panic(err)
	}
}
```

重新构建每个应用，用 `--ssr-memo-size 0` 启动（让每个请求都渲染），shop 还要设 `SHOP_OPEN=1`（不登录也能进工作台），把 `TIFA_DUMP_INPUT` 设成一个空文件夹，不带 cookie 把每个页面请求一次。shop 的页面是 `/projects/studio`、`/projects/studio/orders/SH-1042`、`/projects/studio/stats` 和 `/store`、`/store/products`、`/store/products/pour-over-kettle`、`/store/journal`、`/store/journal/stock-sync-api`、`/store/cart`、`/store/checkout`。MUI 的首页是 `/`。文件夹里每个文件以 URL 命名，改成 `pages/<app>/in/` 里的名字。
