---
summary: 'Go 的时钟、UTC 本地时间、基于共享的 go-intl 数据和 tz 2026e 的 Intl 格式化、与 Chromium 的兼容性，以及测得的代价'
read_when:
  - 想让 Date、Intl 和 Temporal 读一个自己控制的时钟，比如测试或服务端渲染里固定的时间
  - Date.now、new Date() 或 performance.now 很慢，或者给出的时间不是 Go 的 time.Now
  - 页面在服务器上显示的日期或时间和在浏览器里的不一样，或者时区名和 Chromium 不同
  - 想知道 Intl 在程序体积、VM 内存和时间上的代价
  - 改 intl/bridge.go、intl/registry.go、intl.js 或 stub.js，或者往桥上加一个构造函数
  - 页面改了某个内置对象，Intl 给出了另一个结果
  - 改基于 go-intl 的构造函数，或 intl/gointl.go 里的 chromiumData
  - 页面在第一次使用之前替换或删除了 Intl 或某个 toLocaleString 方法，Intl 又把它放了回去
---

# 时间和 Intl

[English](time-and-intl.md) · 简体中文

服务端渲染的页面必须在每台机器上输出相同的内容，并且和浏览器显示的一致。[Updating the intl locale data](../upgrading.zh-CN.md#更新-intl-语言数据) 说明了怎样换到新的 CLDR、tzdb 或 Chromium。

## 每个 VM 里本地时间都是 UTC

发布版本：v0.26.3。

脚本的本地时间在每个 VM 里都是 UTC，不管进程在哪个时区。引擎以前从 libc 的 `localtime` 取时区，它读 `TZ`。现在引擎相对 UTC 的偏移始终是 0，进程的时区不去动它。`intl` 包提供的 `Intl` 默认也是 UTC。这对脚本的影响：

- `getTimezoneOffset()` 是 0，`getHours`、`getDate`、`getDay` 和其他本地 getter 给出的值和对应的 `getUTC*` 相同。本地 setter（`setHours` 等）设置的是 UTC 字段。
- `new Date(2026, 0, 1)`、对不带偏移的日期时间（`'2026-01-01T13:05:00'`）做 `Date.parse`，以及其他本地形式，都把字段当作 UTC 来读。
- `toString`、`toDateString` 和 `toTimeString` 按 UTC 打印，带 `GMT+0000`。引擎自己的 `toLocaleString`、`toLocaleDateString` 和 `toLocaleTimeString` 也打印 UTC。`intl` 包替换了这三个，它的版本除非给了 `timeZone`，否则打印 UTC。

需要其他时区的脚本，在注册了 `intl` 包的情况下把时区名交给 `Intl`：`d.toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})` 或 `new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York', hour: 'numeric'}).formatToParts(d)`。没有 `intl`，引擎没有办法显示别的时区。

## 引擎读 Go 的时钟

发布版本：v0.27.0。

引擎里每个读取时间的函数，都问 Go 而不是 libc。

| 函数 | 谁在读 | 时钟 |
| --- | --- | --- |
| `date_now` | `Date.now()`、`new Date()`、`Date()` | `time.Now().UnixMilli()`：1970 年以来的整毫秒，向下取整，和以前一样 |
| `js_random_init` | 新 context 里 `Math.random` 的种子 | `time.Now().UnixMicro()`，为 0 时用 1，和以前一样 |
| `get_time_ns` | `StdAddHelpers` 的 `performance.now()`，现在已经没有了（[go-api.md](go-api.zh-CN.md#没有模块加载器shell-辅助函数和-setcanblock)） | Go 的单调时钟，从进程启动算起 |

`Atomics.wait` 仍然读 libc 的 `CLOCK_REALTIME`：它把截止时间交给 libc 的 `pthread_cond_timedwait`，后者按那个时钟计时，而且它不是热路径。eval 超时本来就跑在 Go 的单调时钟上（见 [uncatchable.md](uncatchable.zh-CN.md#eval-超时用单调时钟)），本地时间仍然是 UTC。

脚本能看到的变化：`performance.now()` 和 Node 一样，从进程启动算起。以前在 Linux 和 macOS 上，它从系统启动算起，在 Windows 上 quickjs-libc 为它读墙上时钟（`gettimeofday`），给出的是 1970 年以来的毫秒数，系统时钟被调整时它会跟着动。它的精度（以毫秒为单位的 double 里的纳秒）不变，速率也不变，macOS 除外：macOS 以前的 `CLOCK_MONOTONIC` 在系统睡眠时继续走，而 Go 的单调时钟（`mach_absolute_time`）不走，所以跨过一次睡眠的脚本看到流逝的时间更少。在 Windows 上，`Date.now()` 和 `new Date()` 不再在 2038 年溢出：那里 libc 的 `timeval` 只有 32 位秒。

在 AMD EPYC 7B13 的一个核上（linux/amd64，不开 PGO，`BenchmarkClock`，前后交替跑 6 轮，每项取 18 次运行的中位数）：`Date.now()` 用 122 ns，以前是 689，`new Date()` 用 261 ns，以前是 870，`performance.now()` 用 107 ns，以前是 686。tifa 示例商店一个页面的渲染读时钟的次数太少，得不到收益：6 轮中（第七轮与同一个核上的另一个任务重叠，所以不计），六个 P50 之和是原来的 0.994 到 1.009（中位数 1.000），在一轮的波动之内，HTML 相同。

## 每个 VM 一个墙上时钟

发布版本：v0.28.2。

`VM.SetClock(func() time.Time)` 设置一个 VM 的墙上时钟。`Date.now()`、`new Date()`、`Date()`、不带日期的 `Intl.DateTimeFormat`（`format()`、`formatToParts()`）和 `Temporal.Now` 都读它，`VM.Now()` 在 Go 里以完整的 `time.Time` 精度读它。`SetClock(nil)` 回到实时的 `time.Now`。Date 和 Intl 取整毫秒，向下取整，1970 年之前也一样。Temporal 保留纳秒。

```go
at := time.Date(2026, 10, 22, 12, 0, 0, 123456789, time.UTC)
vm.SetClock(func() time.Time { return at })
// Run the module or the render here, and drain its jobs.
vm.SetClock(nil)
```

- 在调用 JavaScript 的间隙设置它。绝不要在 VM 运行时、从 VM 内部的回调里、或从另一个 goroutine 里设置。要让模块的初始化也受控，就在执行模块字节码之前设置。
- 回调不能阻塞，因为eval 超时无法打断它。多个 VM 共用一个回调时，它必须能安全地被并发调用。它里面的 panic 遵循引擎里 panic 的规则：关闭那个 VM。
- 非 nil 的 `temporal.Clock` 仍然只覆盖 `Temporal.Now`，并且优先于 VM 时钟。没有它，或者 `Clock(nil)`，Temporal 每次调用都读 VM 时钟，之后再调用 `SetClock` 也一样。
- 保存下来的函数读的是调用时当前的时钟：模块加载时保存的 `Date.now`、`Temporal.Now` 的方法、`Intl` 的格式化器。Intl 在注册时取原生的 `Date.now`，所以页面替换 `Date.now` 不会改变 Intl 读到的。
- 它只是墙上时钟。eval 超时、`Atomics.wait` 和 `Math.random` 的种子保留它们各自的时钟。
- `Close` 和 `SetClock(nil)` 会释放回调，即使它引用着这个 VM。

运行时里只存一个标志，从不存 Go 指针，因为运行时内存在 Go 堆之外。有时钟的 VM，它的回调存在按运行时索引的 Go 注册表里，在被替换、重置或关闭之前一直保持存活。没有时钟时，调用像以前一样读 Go 的时钟：一次标志测试，不查表、不加锁、不分配。

代价：在 AMD EPYC 7B13 的一对核上（linux/amd64，Go 1.27.2，不开 PGO，没有自定义时钟），对比没有这项改动的同一份代码：3 个不同代码布局的构建，前后交替跑 6 轮，每一对都跑在闲置了 30 秒的一对线程上。`BenchmarkClock` 给出的 `Date.now()` 是之前 116.6 ns、之后 115.7 ns（配对中位数 0.991），`new Date()` 是 227.6 和 229.2 ns（0.986），`performance.now()` 是 99.6 和 100.3 ns（1.005），这一项没被这次改动碰过，所以它显示了该方法自己的波动。三项都保持 0 B/op 和 0 allocs/op。在 tifa 示例商店的六个页面上（一个预热过的 VM，每个页面 500 次渲染），六个 P50 之和在 18 对里是之前的 0.980 到 1.022，中位数 1.0035：没有哪项变化超出了布局的波动。

## 覆盖 Chromium 全部语言的 Intl

发布版本：v0.26.2。

第一版 locale 实现用 CLDR 48.2（cldr-json 48.2.3）来格式化 Chromium 有的每一种 locale，而上游只格式化 en-US。现在共享的 go-intl 数据包取代了那份数据。它覆盖 `Intl.DateTimeFormat`、`Intl.NumberFormat`、`Intl.PluralRules` 和 `toLocale*` 方法，它的输出与 Chromium 156 在约 60,000 个表达式上做了对比（`intl/golden_test.go`）。不带 locale 参数时，locale 是 en-US，不带 `timeZone` 选项时，时区是 UTC：`Intl` 和 `toLocale*` 方法从不读主机的设置，引擎的 `Date` 也不读（见 [每个 VM 里本地时间都是 UTC](#每个-vm-里本地时间都是-utc)）。`Register` 只在 VM 上放替身。第一次使用 `Intl` 或某个 `toLocale*` 方法时才安装其余部分，字节码每个进程编译一次。包文档列出了没有实现的东西，以及输出和 Chromium 不同的地方。

在那个版本，对比 en-US 版本，在 darwin/arm64 上测得：用了 `intl` 的程序大 1.7 MB（其中 1.3 MB 是 locale 数据）。`Register` 用 0.09 ms 和 VM 的 9 KB 内存，以前是 0.85 ms 和 46 KB。VM 里第一次使用另外要 0.15 ms 和 42 KB。一种语言第一次使用时解码进 Go 堆，每个进程一次，约 0.3 ms 和 100 到 180 KB。第一次使用任何语言时，还会解码约 1 MB 的共享数据。格式化一个日期，en-US 用 3.3 µs（以前 4.3 µs），zh-CN 用 3.7 µs，格式化一个数，en-US 用 2.4 µs（以前 2.4 µs），de 用 2.7 µs。此后的耗时见 [Intl 通过 Builtin 调用 Go](#intl-通过-builtin-调用-go)。

## 小数位数按 ECMA-402 自 2021 年起的读法处理

发布版本：v0.26.4。

`Intl.NumberFormat` 按 ECMA-402 自 2021 年版起的方式读取小数位数，和浏览器一样。默认有两位小数的货币，加上 `maximumFractionDigits: 0`，以前会抛出 `RangeError`，这是 ECMA-402 到 2020 年版为止的行为：现在最小值取它的默认值和给定最大值中较小的那个。小数位数只在用到时才读取，所以 `{maximumSignificantDigits: 3, maximumFractionDigits: 200}` 能格式化，以前它会抛出 `RangeError`。

测试：`intl/all_test.go`。

## 夏令时的名称和 ICU 一致

发布版本：v0.26.9。

这个版本修正了主机使用负夏令时（例如都柏林的冬季时段）时的时区名。它用一份 rearguard 的 DST 表来对齐 ICU。[共享 go-intl 的迁移](#所有-intl-格式化器共用-go-intl) 把这张表和读取主机时区的做法都去掉了。偏移和夏令时标志现在在每台机器上都来自同一个内嵌的 tz 2026e 数据包。

测试：`intl/zonepack_test.go` 和 Chromium 的时区语料。

## Intl 通过 Builtin 调用 Go

发布版本：v0.26.17。

`intl` 包通过 `Builtin` 调用 Go（见 [go-api.md](go-api.zh-CN.md#builtin作为内建函数调用的-go-函数)），进来的路上没有反射也没有 JSON，并且把它的格式化器放在整个进程共用的一个注册表里：在另一个 VM 里用同样的 locale 和选项、或者换一种写法做出的 `NumberFormat`，会复用已经做好的那个。注册表最多保存 1024 个格式化器，丢弃最旧的，格式化器被丢掉的对象会在下次调用时重新创建它。它的函数有内置函数的 shape（引擎记录一个对象有哪些键、按什么顺序的结构，这里是 `length`、`name`、没有 `prototype`、`toString` 显示 `[native code]`、访问器和描述符与 ECMA-402 一致），页面在 `Register` 之后改动 `Array.prototype`、`Object.prototype`、`JSON` 或其他内置对象，不会改变它的行为。

在 AMD EPYC 7B13 的一个核上：`new Intl.NumberFormat('en')` 用 2.7 µs（以前 16.5 µs），`format` 用 3.2 µs（5.6 µs），`formatToParts` 用 6.7 µs（12.9 µs），带时区的 `DateTimeFormat` 创建用 8.1 µs（32.0 µs），格式化用 2.6 µs（5.2 µs），`PluralRules.select` 用 0.9 µs（2.9 µs），带选项的 `toLocaleDateString` 用 9.5 µs（41.8 µs）。用过 `Intl` 的 VM 少占约 15 KB。test262 的 intl402 测试在 6764 个运行中通过 912 个（以前 770 个），以前通过的没有一个失败。

工作原理见 [architecture.md](../architecture.zh-CN.md#引擎旁边的包)，`intl/bridge.go` 顶部的注释说明了怎样添加一个构造函数。

测试：`intl/registry_test.go`、`intl/shape_test.go`、`intl/tamper_test.go`、`intl/convert_test.go`、`intl/legacy_test.go`。

## 基于 go-intl 的 Intl 构造函数

发布版本：v0.27.0。

`intl` 包加上了 `Intl.Collator`、`Intl.Segmenter`、`Intl.RelativeTimeFormat`、`Intl.ListFormat`、`Intl.DisplayNames`、`Intl.DurationFormat`、`Intl.Locale` 和 `Intl.supportedValuesOf`，它们建立在 go-intl 上（`internal/gointl`，见 [Updating go-intl](../upgrading.zh-CN.md#更新-go-intlinternalgointl)），和其他构造函数共用同一座桥和同一个注册表。`localeCompare` 按 locale 的排序规则排序，`toLocaleUpperCase` 和 `toLocaleLowerCase` 像 V8 一样遵循土耳其语、阿塞拜疆语、立陶宛语和希腊语的规则，而 QuickJS 按码点排序，各种语言的大小写映射也一样。`Intl.Locale` 在任何构造函数接收 locale 的地方都代表它的标签。其余的构造函数现在使用同样的数据包，见下文。

每个构造函数接受 Chromium 156 有的 locale，并提供 Chromium 为它们提供的数据：Chromium 的 ICU 数据里缺少某种语言的相对时间、列表或单位的地方（az、bs、kk、kok、pa、sd、uz 以及它们的一些文字和地区就是这样），输出是 root 的或英语的，和 Chromium 一样（`intl/gointl.go` 里的 `chromiumData`，由 `TestChromiumData` 推导出来）。输出与 Chromium 的在这些语言的 14,269 个表达式上做了对比（`intl/golden_gointl_test.go`，用 `record.cjs golden <set>` 记录）。现在有 13,671 个完全一致。另外 598 个不同，原因在包文档里列出，每个都对应 `intl/golden_test.go` 里 `goldenDifferences` 的一条规则：`Segmenter` 的 2,901 个里有 104 个（Chromium 自己的断词规则以及老挝语和高棉语的词典），`DisplayNames` 的 1,635 个里有 475 个（Chromium 的 ICU 数据名称更少，另有几个它自己的名称），`DurationFormat` 的 1,042 个里有 6 个（V8 在超过 2\*\*63 纳秒时按机器的规则溢出），`supportedValuesOf` 的 23 个里有 1 个（更多的货币），`Locale` 的 1,415 个里有 12 个（001 的五个时区、`baseName` 里的 `-u-va-posix`、未知地区）。`Collator`（1,372）、`RelativeTimeFormat`（1,247）、`ListFormat`（1,298）和大小写映射（3,336）完全一致。test262 对 ECMA-402 的理解与 V8 不同时，答案以 V8 为准：值为 `yes` 的 `-u-` 关键字、没有已知书写方向的文字的 `getTextInfo`、只检查第一个 locale 的 `toLocaleUpperCase`，以及两个 `DurationFormat` 的用例。这些函数有内置函数的 shape，并且和其他的一样忽略页面对内置对象的改动：`shape.json` 和 `tamper.json` 是在 Chromium 里记录的。在 Temporal 和其余格式化器迁到 go-intl 之前，test262 的 Intl402 测试在 6764 个运行中通过 2282 个。当前完整的对比见下文。

这第一批构造函数的代价，在 AMD EPYC 7B13 的一个核上，不开 PGO，对比之前的版本：

- 下面 Temporal 格式化改动之前的体积：tifa 的示例商店为 linux/amd64 构建，大 9.8 MB（37.6 MB，对比 27.9 MB），几乎全是 go-intl 的数据：`core.bin`（4.4 MB）、`names.bin`（1.9 MB）、`cjdict.bin`（2.0 MB）和 `tz.bin`（91 KB）。用 `-tags quickjs_intl_nonames,quickjs_intl_nocjdict` 构建，会去掉显示名称（这时 `Intl.DisplayNames` 对每个代码都返回它的回退值）以及中文和日文的词典（这时一段中文或日文就是一个词），大 5.8 MB。go-intl 的日期和时区名（2.8 MB）只有它的 `DateTimeFormat` 才读，在那个阶段没有编进二进制（见 [Updating go-intl](../upgrading.zh-CN.md#更新-go-intlinternalgointl)）：带上它们，商店大 12.6 MB。
- 内存和启动：`Register` 多用 0.01 ms 和 4 KB。VM 里第一次使用用 0.49 ms（以前 0.24 ms），用过 `Intl` 的 VM 比 `Register` 之后多占 151 KB（以前 77 KB）。它安装的 JavaScript 从 17 KB 的源码变成了 36 KB。进程里第一次排序用 4.7 ms，第一次分词用 2.7 ms，用来读取它们的数据。
- 调用：`new Intl.Collator('de')` 用 2.8 µs，`compare` 用 2.4 µs，`new Intl.Segmenter('en', {granularity: 'word'})` 用 4.0 µs，遍历 60 个字符的词用 78 µs（它的字素用 185 µs），`RelativeTimeFormat.format` 用 2.4 µs，三项的 `ListFormat.format` 用 6.5 µs，`DisplayNames.of` 用 1.6 µs，`DurationFormat.format` 用 11 到 16 µs，`new Intl.Locale` 用 19 µs，`maximize` 用 18 µs，`getWeekInfo` 用 105 µs，`supportedValuesOf('timeZone')` 用 61 µs，60 个字符的 `toLocaleUpperCase('tr')` 用 4.8 µs（不带 locale 时 1.4 µs）。
- 创建 `NumberFormat` 或 `PluralRules` 多用 0.1 µs，创建 `DateTimeFormat` 多用 0.3 µs。格式化用时不变。

现在导入 `intl` 的程序会链接 `internal/gointl` 和 `internal/gointl/temporal`，并且必须附带它们的声明（见 [NOTICE.md](../../NOTICE.md#what-a-program-that-ships-a-binary-must-include)）。

测试：`intl/golden_gointl_test.go`、`intl/chromiumroot_test.go`、`intl/registry_gointl_test.go`、`intl/values_test.go`、`intl/shape_test.go`、`intl/tamper_test.go`、`intl/link_test.go`、`intl/nonames_test.go`（带 `-tags quickjs_intl_nonames`）、`internal/gointl/qjs_readers_test.go`，以及检查 `Args.WTF8` 的 `builtin_test.go`。

## Intl 不动页面放在替身位置上的东西

发布版本：v0.27.0。

`Register` 在 VM 上放替身，对象有 `Intl`、`Date` 的三个 `toLocale*String` 方法、`Number` 和 `BigInt` 的 `toLocaleString`，第一次使用其中某一个时才安装真正的实现。这次安装以前会写每个替身的属性：页面自己的 `Date.prototype.toLocaleString`，或者它用 `Object.defineProperty` 定义的 `Intl`，会在第一次使用时丢掉，被它删掉的属性会回来，被冻结的 `Date.prototype` 或 `globalThis` 会让第一次使用抛出 `TypeError`。现在安装只在属性仍然是那个替身、而且可以被替换时才替换它：页面放在那里的、删掉的或冻结的东西保持原样，就像 V8 的内置对象那样，而留在原处的替身会转发给真正的实现。`Temporal` 以同样的方式安装它的替身（见 [temporal.md](temporal.zh-CN.md)）。

具名时区和别名现在通过内嵌的数据包解析，不再查看主机上的文件。

测试：`intl/all_test.go` 里的 `TestStandInsKeepPageCode`，`intl/zonepack_test.go`。

## Temporal 的 duration 使用 DurationFormat

发布版本：v0.27.0。

`Temporal.Duration.prototype.toLocaleString` 使用和 `Intl.DurationFormat` 相同的格式化器注册表和 Builtin。它按构造函数的顺序读一次 locale 和选项，并格式化私有的 duration 字段，不调用公开的 getter 或 `toString`。没有 `intl.Register` 它也能用。

测试：`temporal` 里的 `TestDurationLocaleString`、`TestChromium` 和 `TestTamper`。针对性的语料用 Chromium 156，通过 `node intl/testdata/chromium/record.cjs temporal duration-locale` 记录。

## DateTimeFormat 接受 Temporal 值

发布版本：v0.27.0。

`Intl.DateTimeFormat` 的 `format`、`formatToParts`、`formatRange` 和 `formatRangeToParts` 接受 `PlainDate`、`PlainTime`、`PlainDateTime`、`PlainYearMonth`、`PlainMonthDay` 和 `Instant`。`ZonedDateTime`，或者范围里混用不同的 Temporal 类型，会抛出 `TypeError`。每种类型有自己的默认字段。日历检查使用解析出的日历，ISO 的年月和月日的值在显式指定 ISO 日历时可用。Plain 类型保留它们的墙上时间字段，不受 `timeZone` 影响。处在 Temporal 边界的合法 plain 值，不会被裁剪到 Date 更小的范围。范围方法也接受旧式的 Date 和数字输入。和 Chromium 156 一样，反向的范围保持调用者给的顺序。

JavaScript 的垫片共享一个私有对象，它扎根在 VM 全局对象的一个私有字段里。它不暴露页面可见的属性。Temporal 的垫片提供读取它的私有槽的读取器，Intl 的垫片提供它的私有 locale 读取器。注册顺序、被改动的原型和 GC 都无法伪造或丢失一个值的品牌。格式化器的选项只读一次，并保留下来用于按类型解析和注册表淘汰。范围方法在检查类型之前先转换两个参数。DurationFormat 对真正的 Temporal duration 也使用槽读取器，所以被替换的 getter 不会被调用。

Date 和 Temporal 的值现在使用同一个 go-intl 日期和区间格式化器。`qjs_temporalformat.go` 像 Chromium 156 那样保留未改动的模式。[共享数据的迁移](#所有-intl-格式化器共用-go-intl) 记录了当前的数据体积和计时。猜测主机时区的数据包仍然不编进来，两个包的链接测试都检查这一点。

针对性的语料有 3,216 个日期表达式和 219 个 duration 表达式，从 Chromium 156.0.8078.4 记录而来。日期用例覆盖全部六种接受的类型、两个范围方法和它们的来源、日历不匹配、字段默认值、样式、小数秒、边界、转换、时区和被改动的实例属性。用 `node intl/testdata/chromium/record.cjs temporal datetime-format` 和 `temporal duration-locale` 来记录。构造函数的形状和原型篡改也用 `record.cjs shape` 和 `record.cjs tamper` 记录。

在 test262 提交 `2e0a5676` 上，提到 Temporal 的 Intl402 文件在 linux/amd64 上两种模式都跑了：4,222 个运行中通过 4,172 个，`641e996` 上是 4,008 个。新增通过 184 个，新增失败 20 个。Chromium 156 在同样的 20 个纪元和小时周期的断言上也失败。其余 50 个失败中，有 26 个在 Chromium 里也以同样的断言失败。在那个阶段，另外 24 个反映的是日历数据有限或日历枚举的问题。共享数据的迁移修好了这些用例。阴阳历的比较在 Chromium 里也失败，但日期不同，所以它仍留在日历这一组。引擎代码没有改动。引擎单独的 58 个错误的基线没有重跑。

测试：`temporal/formatting_test.go`、`temporal/chromium_test.go`、`intl/temporal_test.go`、`intl/shape_test.go`、`intl/tamper_test.go`、`internal/intlhook/state_test.go` 以及两个包的链接测试。go-intl 更新之后，对照未改动模式、ISO 模式和提供的偏移这几种行为，复查 `qjs_temporalformat.go` 适配器，并重新运行这些 Chromium 语料。

## 所有 Intl 格式化器共用 go-intl

发布版本：v0.27.0。

`NumberFormat`、针对 Date 值的 `DateTimeFormat` 和 `PluralRules` 现在使用 go-intl，Number 和 Date 的 locale 方法也一样。数字和复数的舍入使用完整的 `roundingIncrement` 和 `roundingPriority` 规则。日期格式化支持 Chromium 列出的全部 18 种日历。旧的 `intl/cldr.bin`（1,275,236 字节）、它的生成器、主机时区加载器和它的 DST 表都去掉了。

数字的桥在原始值转换之后，让 BigInt 的数字和数字字符串保持区分。超出 Number 范围的 BigInt 保持精确，而超出这个范围的数字字符串变成无穷大。`TestNumberFormatBigIntBeyondNumberRange` 通过公开的 VM API 覆盖两种符号、装箱的 BigInt、对象转换、字符串、parts 和 BigInt 的 locale 方法。

生成器按 Chromium 156 的 ICU 清单，从 go-intl 的模型里选出数字、日期和时区的资源。同一套数据包服务每个构造函数。它保留缺失的资源、父级选择、非拉丁的数字符号和 Chromium 的时区名。希伯来语的样式用例，在样式构造器因缺少 RBNF 数据而失败之后，采用 ICU 的默认模式。混合的日期和时间样式随后遵循 V8 的小时周期重新生成。未知的数据错误仍然会失败。

Intl 和 Temporal 共用内嵌的 ICU tz 2026e 数据包。伪造的 `ZONEINFO` 文件改变不了任何答案。Chromium 156 早于曼尼托巴的永久夏令时：在 `2026-11-15T12:00Z`，温尼伯在这里是 `7:00 AM Central Daylight Time`，在那边是 `6:00 AM Central Standard Time`。迁移语料保留了浏览器的记录，并对三个表达式要求精确得到更新的答案。

进程范围的格式化器注册表和已捕获的内置对象保持不变。日期的注册表键还包括它实际用的模式。V8 对不同的模式可能报告相同的公开选项，例如西班牙语里加了引号的 `de`，所以只靠公开选项无法确定是哪个格式化器。PluralRules 的键也保留解析后的选项，因为存在有效数字时，公开的 resolved options 会省略小数位数的限制，而两者都影响精度的选择。`TestPluralRulesRoundingRegistry` 通过公开的 JS API 检查两种构造顺序、跨 VM 和淘汰之后的情形。

现有的 74,868 个 golden 表达式全部通过精确对比，或者落在已有的、有文档记录的差异规则内。356 个答案相对前一次格式化提交发生了变化：349 个现在与 Chromium 完全一致。七个 DisplayNames 组合修好了货币字段，同时保留其他字段已知的差异。基础的日期、数字和复数集合完全一致。新的迁移语料有 939 个表达式，只有上面三个 tz 版本造成的精确差异。现有的 723 个 Temporal 语料表达式全部一致。

在 GCP linux/amd64 上，test262 `2e0a5676` 用同样的新建 VM 运行器，注册 Intl 和 Temporal，两种模式运行每个 Intl402 文件。对比 main `641e996`，通过数从 6764 个运行中的 6276 个升到 6618 个。新增通过 366 个，新增失败 24 个。Chromium 156 在这 24 个断言上也全部失败：伊斯兰历的回退、旧的 ACT 时区别名，以及 Temporal 的纪元和小时周期规则。其余 146 个失败包括 NumberFormat 的范围、运行器承载不了的第二个 realm 的测试，以及已有的 V8 差异。没有哪个新增失败的用例在 Chromium 里是通过的。引擎单独的 58 个错误的测试集没有重跑。

数字数据包记录了它的 ICU 资源已经解析好。go-intl 的原始源保留各自独立的数字和货币别名规则。`qjs-sync.sh test` 拿原始的完整数据对照上游语料检查，`test-chromium` 则拿发布的数据包对照 liustack/quickjs 的浏览器语料检查。

六个链接进来的数据包共 9,352,287 字节。猜测主机时区的数据包仍然不链接。在 GCP linux/amd64、Go 1.27.2 上，tifa 商店 `fb31f616` 用 `go tool tifa build` 加 `-ldflags=-w` 和同一份应用 PGO profile 构建，是 31,967,614 字节，main `641e996` 上是 32,078,049。小了 110,435 字节，约 0.34%。profile 的 SHA256 是 `9a3ca92cd8f8ab68477e5e5fe3e27fafee8719ac59223c6526a122d10d1de64a`。

### 针对性的计时和内存代价

对比 main `641e996`，在同一台 GCP AMD EPYC 7B13 上，不开 PGO，对比使用逻辑 CPU 1，它的同伴 5 空闲，nice 19，内存上限 4 GiB 且不用 swap。每个计时行有六个 main 和分支交替的样本，每个样本一秒。数值是中位数，括号里是最小值和最大值。

NumberFormat 使用 USD 货币和 `1234567.891`。DateTimeFormat 使用 medium 日期样式和 short 时间样式、UTC 和 `1755324309123`。`New` 测量共享注册表预热之后的 JavaScript 构造。`Format` 使用准备好的格式化器。`Resolve` 在不经过注册表的情况下构造 Go 格式化器，locale 数据已经加载。它不包括 JavaScript 的桥。三项测量都不包括解析测试源码。

| 操作 | Main，µs | 分支，µs | 分支 / main |
| --- | ---: | ---: | ---: |
| NumberFormat/en-US/New | 7.732 (7.693 to 7.756) | 7.915 (7.766 to 8.026) | 1.024 |
| NumberFormat/en-US/Format | 3.744 (3.728 to 3.763) | 2.176 (2.162 to 2.192) | 0.581 |
| NumberFormat/en-US/Resolve | 3.420 (3.392 to 3.459) | 9.959 (9.866 to 10.117) | 2.912 |
| DateTimeFormat/en-US/New | 8.169 (8.095 to 8.245) | 8.258 (8.233 to 8.397) | 1.011 |
| DateTimeFormat/en-US/Format | 3.061 (3.027 to 3.092) | 1.915 (1.911 to 1.938) | 0.626 |
| DateTimeFormat/en-US/Resolve | 10.531 (10.409 to 10.606) | 48.794 (48.166 to 49.137) | 4.633 |
| NumberFormat/zh-CN/New | 7.785 (7.756 to 7.827) | 7.814 (7.766 to 7.985) | 1.004 |
| NumberFormat/zh-CN/Format | 3.728 (3.702 to 3.785) | 2.189 (2.184 to 2.239) | 0.587 |
| NumberFormat/zh-CN/Resolve | 3.436 (3.399 to 3.535) | 9.799 (9.724 to 9.854) | 2.852 |
| DateTimeFormat/zh-CN/New | 8.207 (8.189 to 8.243) | 8.338 (8.243 to 8.461) | 1.016 |
| DateTimeFormat/zh-CN/Format | 2.791 (2.771 to 2.829) | 2.008 (1.998 to 2.020) | 0.719 |
| DateTimeFormat/zh-CN/Resolve | 10.275 (10.257 to 10.380) | 42.288 (41.645 to 43.010) | 4.115 |

数字格式化比 main 少用 41% 到 42% 的时间，日期格式化少用 28% 到 37%。NumberFormat 每次调用分配 600 字节、8 次分配，main 是 1888 字节、24 次。DateTimeFormat 英语分配 152 字节、7 次，中文分配 128 字节、7 次，main 是 1220 字节、25 次和 1144 字节、22 次。带缓存的 JavaScript 构造在这次运行里慢了 0.4% 到 2.4%。这些都是固定输入下的代价。[History](../history.zh-CN.md#intl-格式化) 描述了这次对比。

不带缓存的 Resolve，数字用 9.8 到 10.0 µs，日期用 42.3 到 48.8 µs。在编译并共享计划之前，大约分别是 25 到 26 µs 和 108 到 109 µs。locale 和选项的解析、货币间距和数字规则，每个新格式化器仍然要运行。日期构造还要选择一个模式并构建它的区间规则。这些依赖选项的步骤仍然在数据缓存之外。当完整的请求以前见过时，格式化器注册表会避开它们。

最终的 profile 仍显示数字构造里加载 Unicode 属性、日期构造里解码时区。这一遍共享了四个依赖 locale 的大型数据结构。属性和时区的加载器有各自独立的键和错误规则，仍在这些缓存之外。日期模式匹配约占不带缓存的日期构造 CPU 时间的 29%。这些是注册表未命中时的代价，不是每次 Format 调用的代价。

每个内存用例在五个新进程中运行，只注册 Intl。VM 内存是第一次使用并 `RunGC` 之后的 `VM.MemorySize`。Go 堆的增量是在 GC 之后，对比创建 VM 之前的状态测得的。它包括注册、每个进程加载一次的共享数据和第一个 VM 的 Go 状态。它不是 RSS，也不是要乘以 VM 数量的代价。

| 用例 | Main VM，KiB | 分支 VM，KiB | Main 第一次使用的 Go 堆增量合计，KiB | 分支第一次使用的 Go 堆增量合计，KiB |
| --- | ---: | ---: | ---: | ---: |
| DateTimeFormat/en-US | 358.586 (358.586 to 358.586) | 371.617 (371.617 to 371.617) | 1274.289 (1274.289 to 1274.602) | 786.328 (786.016 to 786.484) |
| DateTimeFormat/zh-CN | 358.586 (358.586 to 358.586) | 371.617 (371.617 to 371.617) | 1248.891 (1248.734 to 1249.203) | 790.445 (790.445 to 790.602) |
| NumberFormat/en-US | 358.586 (358.586 to 358.586) | 371.617 (371.617 to 371.617) | 1203.305 (1202.992 to 1203.461) | 678.273 (677.867 to 678.336) |
| NumberFormat/zh-CN | 358.586 (358.586 to 358.586) | 371.617 (371.617 to 371.617) | 1177.516 (1177.359 to 1177.984) | 678.898 (678.898 to 679.055) |

用过的 VM 多保留 13.031 KiB 引擎内存。进程里的第一次使用少保留约 458 到 525 KiB 的 Go 堆。数据预热之后，再多 32 个 VM，每个多保留的 Go 堆，main 上约 1.08 KiB，这里是 1.36 到 1.38 KiB。这个估计的完整样本范围见下表。

| 用例 | Main 预热后每个 VM 的 Go 堆，字节 | 分支预热后每个 VM 的 Go 堆，字节 |
| --- | ---: | ---: |
| DateTimeFormat/en-US | 1116.000 (1116.000 to 1116.000) | 1412.000 (1392.000 to 1412.000) |
| DateTimeFormat/zh-CN | 1116.000 (1116.000 to 1116.000) | 1407.000 (1397.000 to 1422.000) |
| NumberFormat/en-US | 1109.000 (1109.000 to 1109.000) | 1395.000 (1375.000 to 1405.000) |
| NumberFormat/zh-CN | 1109.000 (1109.000 to 1109.000) | 1405.000 (1390.000 to 1410.000) |

测试：`TestGoIntlMigration`、`TestEmbeddedZonePack`、每个 Intl golden 集合、Temporal 格式化语料、构造函数的形状、篡改、注册表的共享和淘汰，以及链接测试。`qjs-sync.sh pack -check` 检查可复现性，数据包测试检查共享池的引用。go-intl 或 Chromium 更新之后，重新记录 ICU 清单，重建数据包，并复查每个有变化的表达式。见 [upgrading.md](../upgrading.zh-CN.md#更新-intl-语言数据)。

## 编译好的 Intl 格式化计划

发布版本：v0.27.0。

字符串格式化和 parts 格式化从同样的舍入和日历结果写出。它跳过只为给部分分配类型而做的 parts 分类、Unicode 裁剪和合并，也避免在 Go 的桥上复制 parts。数字的前后缀和数字表在构造时编译好。分组一次预留好它的 parts，只有符号取决于舍入后的零时，才为选择符号再舍入一次值。日期格式化直接写出编译好的字段。它把墙上时间的日历运算，和用于天文历法及时区名的时刻分开，所以不需要为每个值创建一个固定时区的对象。`TestIslamicCalendarInstant` 对两种天文伊斯兰历，在正偏移和负偏移的时区里，在月份边界上，对字符串、parts 和范围，按固定的答案检查。

`CacheSource` 为一个不可变的源共享已解码的数字数据、日期数据、日历和日期模式生成器。它的四个缓存各最多保存 64 个条目。条目是只读的，淘汰不会使格式化器失效，不同的源不能共享条目。现有的 1024 个条目的格式化器注册表不变。格式化时不对这些数据缓存加锁。货币元数据的查找直接找到所需的那一行。

对英语和中文的 Format 调用做 profile，发现 parts 分类和桥上的复制，约占 NumberFormat 所分配字节的 56%。日期的分段、parts 和桥上的复制，约占 DateTimeFormat 的 86%。全部 74,868 个 golden 答案和 5,097 个迁移与 Temporal 的答案保持逐字节不变，包括已有的、有文档记录的 Chromium 差异。同样的十个商店页面的 HTML 也保持不变，只对路由器的时间戳做了规范化。

最终的 profile 把数字剩下的分配挪到了取决于值的分组和组装上。日期分配的是它的输出、空格替换和数字字段。这些共享的舍入和字段例程仍然同时服务字符串和 parts。[页面测量](../history.zh-CN.md#intl-格式化) 并不能证明整页变快了。

测试：`TestFormatStringMatchesParts`、`TestQJSCachedSource`、`TestQJSDataCacheBound`、现有的 Chromium 语料、并发注册表测试、篡改和链接测试。`BenchmarkFormat` 用上面的输入覆盖构造、格式化和不带缓存的 resolve。上游的改动保留在 `qjs.patch` 里。更新 go-intl 之后，从那个补丁重建它，检查源的隔离和并发使用，对比每个记录下来的答案，并重复针对性的测量。

## Intl.Segmenter 边读边找分段

发布版本：Unreleased。

Segments 对象的 `containing` 和迭代器现在在 javascript 读的时候才找文本的分段，一次一批，原来第一次调用就在 Go 里找出整段文本的每个分段。在 8 MiB 的文本上，`containing(0)` 让 go-intl 列出全部 8,388,608 个字素：Go 里分配 1,260 MiB，不算在 VM 的内存上限里，还要 0.8 到 1.5 秒，期间不检查中断标志。现在只要 24 MiB（UTF-16 的文本，以及 Go 读进来的那份拷贝），在 GCP 上 17 到 36 ms。这些内存不算在 VM 的内存上限里，VM 留着的每个迭代器带着的文本（见下文）也不算，而且前面很远处的一个断点（比如一长段要用词典切分的文本之后的断点）是在不检查 VM 的情况下找出来的。

第一批是 64 个断点，之后每批和前面所有批加起来一样长，最多 65,536 个，所以读完所有分段只要很少几次调用。每批的活每 256 个断点检查一次 VM（见 [uncatchable.md](uncatchable.zh-CN.md#intl-在长列表的循环里读中断标志)）。go-intl 的 `breaker.go` 通过 `qjs.patch` 改成一次找一个边界，`qjs_segments.go` 把它们作为 `Segmenter.Iter` 提供出来。

每一批从上一批停下的地方接着找，用的是同一个断点迭代器。go-intl 找到的断点取决于它的迭代器之前看到过什么（词典切分的那些片段，它找到的断词引擎），所以只有一个迭代器从头找到尾，结果才和整段走一遍相同。因此 VM 保留最近读过的 4 个 Segments 对象的迭代器，每个带着它的 UTF-16 文本，直到 VM 关闭。迭代器被丢掉的 Segments 对象会从文本开头重新建一个，并走过它已经有的断点：出来的断点一样，只是多花些时间。每个分段都和原来一样，不管是通过迭代器，还是在任何位置调用 `containing`。

读完 1 MiB 混合文本的每个词要 1.38 秒，和原来一样，Go 里分配 35 MiB，原来是 88 MiB。把一段短文本的每个词读 10,000 遍要 315 ms，原来是 311 ms（在 GCP 的 0 和 4 号核上各跑一次）。

测试：`intl/segmenter_test.go` 里的 `TestSegmentsInBatches` 在 10 万个码元、几种文字、带孤立代理项的文本上，对六种 locale 和粒度，把迭代器、在新的 Segments 对象上和在同一个对象上调用 `containing`、以及轮流读比 VM 保留的迭代器更多的 Segments 对象，都和 go-intl 整段走一遍的结果比较。`TestSegmentsMemory` 检查 8 MiB 文本的头几个分段在 Go 里用不到 64 MiB。

## Temporal 格式的缓存只保留短请求

发布版本：Unreleased。

`toLocaleString` 和 `DateTimeFormat.prototype.format` 为 Temporal 值造的格式由进程里所有 VM 共用，它们会把页面要求的东西不管多长都留下来，VM 关闭之后也还在。`toLocaleString` 的缓存最多存 512 个格式，但每个都带着页面给的原样请求，失败的也一样：一个 1 MiB 长、根本不是历法的历法选项，会和它的错误一起留下。格式化器的注册表最多留 1,024 个格式化器，每个都带着它的解析后选项，而 Temporal 日期格式的解析后选项里装着整个请求。在 GCP 上，做过这类调用后关闭的 40 个 VM，失败的历法留下了 81 MiB，用 10,000 个 locale 的列表对 Temporal 值调用 `DateTimeFormat.format` 留下了 11 MiB。

现在 `toLocaleString` 的缓存只在请求不超过 512 字节（和注册表一样）并且解析成功时才保留格式：错误每次调用都重新造，因为它可能带着页面传进来的东西。所以这个缓存里的请求最多占 256 KiB。Temporal 的日期格式不再保留请求本身，改为保留它的 SHA-256（十六进制 64 字节），这仍然能区分那些只有请求才能区分开的格式。不超过 512 字节的请求（页面造的就是这种）照旧缓存，所以常见调用花的时间不变。关闭的 40 个 VM 现在留下的不到 1 MiB。

测试：`temporal/limits_test.go` 里的 `TestFormatCachesAfterClose`（1 MiB 的失败历法，以及对 10,000 个 locale 的列表调用 `DateTimeFormat.format` 和 `toLocaleString`，每种都在调用后关闭的 40 个 VM 里做）。

## 数字选项先检查范围再向下取整

发布版本：Unreleased。

`fractionalSecondDigits`、`minimumIntegerDigits`、`maximumFractionDigits`、`roundingIncrement` 这类数字选项，现在像 ECMA-402 的 DefaultNumberOption 那样，先检查是否在范围内再向下取整，所以刚超出范围上端的值是 RangeError，和 Chromium 一样。原来 `fractionalSecondDigits: 3.9` 和 `minimumIntegerDigits: 21.9` 先被向下取整成 3 和 21，然后被接受。范围内的值照旧向下取整：`fractionalSecondDigits: 1.5` 是 1。

测试：`intl/options_test.go` 里的 `TestNumberOptionRange`（DateTimeFormat、NumberFormat 和 PluralRules 的 20 个选项，取在范围两端，答案来自 Node 24）。

## 长的 locale 标签按长度的线性时间解析

Release: Unreleased.

Intl 的构造函数、`supportedLocalesOf` 和 `toLocaleString` 现在用和标签长度成正比的时间解析一个长的 locale 标签。ECMA-402 的 BestAvailableLocale 每次从标签末尾去掉一个子标签，直到某个服务支持剩下的部分，而 go-intl 对去掉后的每一段都查一次，每次都把它复制进查找用的键里，所用时间随标签长度的平方增长。合法的标签可以很长，因为私用部分可以带任意多个子标签：`new Intl.NumberFormat("en-x-" + "abcdefgh-".repeat(116508) + "x")` 是一个 1 MiB 的标签，在 M 系列 Mac 上要 6.8 秒，4 MiB 的要 58 秒，都在 Go 里，而且不检查中断标志。

现在 go-intl 的 `negotiate.go` 通过 `qjs.patch` 记下每个服务列表里最长标签的长度，比它长的部分不再查找，因为没有服务支持这么长的标签。去掉子标签的方式和原来一样，所以每个答案都不变。上面两个标签现在分别要 33 和 55 ms，也就是把它们变成规范形式的时间。页面传的那种标签比每个上限都短，查找方式和原来一样。

测试：`intl/limits_test.go` 里的 `TestLongTagResolves`（在一个带 64Ki 个子标签的私用标签上测构造函数和 `supportedLocalesOf`，原来每个都要一秒以上）。
