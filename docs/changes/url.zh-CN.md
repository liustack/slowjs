---
summary: '包 url，给 VM 用的 URL 和 URLSearchParams，解析在 Go 里做：覆盖什么，一个 URL 怎么存，字符串的长度、内存上限、eval 超时和 Interrupt 怎么守住，解析器怎么从 nlnwa/whatwg-url 拿来并用 web-platform-tests 测，脚本能看到的和 core-js 以及浏览器有什么不同，以及它的代价和收益'
read_when:
  - 改包 url、url.js、bridge.go 或 internal/whatwgurl
  - 页面在服务端得到的 URL、search 或 host 和浏览器里的不一样
  - 问 URL 占多少 VM 内存、程序体积和时间，或者它在一次渲染里省多少
  - 把 URL 解析器或 IDNA 的表升到新的标准或新的 Unicode 版本
  - 加一个接受脚本字符串的 Go 函数，看它怎么守住限额
---

# URL 和 URLSearchParams

[English](url.md) · 简体中文

QuickJS 既没有 `URL` 也没有 `URLSearchParams`。包 `url` 给调用了 `url.Register(vm)` 的 VM 加上它们，做法和 `intl.Register` 加上 `Intl` 一样。Go 负责解析，什么也不留：一个 URL 就活在 VM 里。

## 解析在 Go 里的 URL 和 URLSearchParams

版本：v0.30.3。

一个 bundle 如果用 `new URL(path, base)` 解析链接，不然就得带一个 polyfill，core-js 的在这个引擎上每个 URL 要 42 到 87 µs。tifa 商店页渲染一次，TanStack Router 要调它 100 到 170 次，每次还读 protocol、username、password、origin、pathname、search 和 hash。core-js 会先找一个够好的原生 `URL`，找到了就什么也不装，所以宿主在 bundle 运行之前注册这个包，bundle 一行不用改。

脚本拿到的是标准的全部，不只是这几个页面调的那些：带 base 和不带 base 的构造函数，`URL.canParse` 和 `URL.parse`，每个 getter 和 setter，两个方向都联动的 `searchParams`，`toString` 和 `toJSON`，以及 `URLSearchParams` 的三种构造形式（字符串、成对的序列、记录），`append`，带可选值的 `delete` 和 `has`，`get`、`getAll`、`set`、`sort`、`forEach`、`keys`、`values`、`entries`、迭代器、`toString` 和 `size`。这些类的形状就是 WebIDL 给的（`TestShape` 里写着每个属性是什么）。

### 一个 URL 怎么存

- `new URL(input, base)` 调一个 Builtin。它在 Go 里解析，返回一个数组：11 个字符串，href、protocol、username、password、host、hostname、port、pathname、search、hash 和 origin，再加一个数，它的四个位说的是 getter 看不出来的东西：URL 有没有主机（主机可以是空的），路径是不是 opaque，有没有查询，有没有片段（查询和片段都可以是空的）。`url.js` 把这个数组放在一个私有字段里，每个 getter 只是读它。Go 什么也不留，所以一个 URL 是 VM 的内存，受内存上限管，由垃圾回收释放。不需要 Go 的 finalizer，也不需要注册表。
- setter 把部分名、新值和这个 URL 的记录交给 Go：getter 给出的 protocol、username、password、hostname、port、pathname、search 和 hash，以及那四个位。Go 不解析任何东西，直接用它们造出记录（`whatwgurl.FromParts`），按标准的 setter 去改（带 state override 的基本 URL 解析器），返回一个新数组。`FromParts` 只接受 URL 的各部分：它原样还得回来，href 是 ASCII，再解析得到同一个记录（下面那两种记录除外），加起来不超过 65,536 个单位。别的部分、和各部分对不上的位、不是 setter 的部分编号，都是 TypeError，URL 保持原样（`TestFromPartsRefuses`、`FuzzFromParts`、`TestForgedRecord`）。
- 一个 URL 就是它的记录，而不是它的 href，和标准一样。setter 能造出解析器从 href 造不出来的记录：`http://localhost/` 设了 `protocol = "file"` 之后，是一个主机为 `localhost` 的 file URL，而解析器会把这个主机变成空主机，`http://h/C|/x` 设了之后，第一段是盘符 `C|`，而解析器会把它写成 `C:`。按 href 存的 URL 到下一个 setter 就会丢掉它们。`FuzzParse` 检查解析器造出的东西序列化再解析得到它自己，`FuzzSetters` 检查 setter 留下的东西能从它的各部分再造出来。
- 脚本碰不到记录，也碰不到 Builtin，所以没法把自己造的各部分交给 Go。数组放在 `URL` 类的私有字段 `#r` 里，只有这个类的方法读它：子类、`Proxy`、换了接收者的 `Reflect`、从 `URL.prototype` 造出的对象都没有这个字段，通过它们去读是 TypeError，和任何私有字段一样。Go 造数组时把元素定义成自有属性（`VM.NewArray` 和数组字面量一样定义它们），所以 `Array.prototype` 和 `Object.prototype` 上的 getter 和 setter 都看不到它，`url.js` 也从不把它交给脚本。五个 Builtin 是 `url.js` 这个函数的参数，不是任何东西的属性。`toString` 会改 URL 的值，在 setter 读记录之前就跑完了，所以 setter 改的是转换之后的 URL。一个 VM 是一个 realm，Go API 拒绝别的 VM 的值。`TestStateIsUnreachable` 从全局对象，从一个 URL、它的列表和一个迭代器出发，走遍脚本能到达的每个对象，既找不到记录，也找不到 Builtin。然后它在 `Array.prototype` 和 `Object.prototype` 的下标属性、`Symbol.iterator` 和 `String.prototype` 的方法上设陷阱，把两个类的每个成员都跑一遍，没有一个陷阱被调用。
- 一个 `URLSearchParams` 就是它的列表，一个名和值依次排开的扁平数组，放在私有字段里，还有它所属的 URL（当它来自 `url.searchParams` 时）。两者按标准保持同步：列表一变，就通过同一个 Builtin 设这个 URL 的 `search`，`search` 或 `href` 一变，列表就重新生成。`url.js` 切开查询串，保存、排序和序列化列表。Go 一次只解码或编码一个名或值，做 `application/x-www-form-urlencoded` 的解析和序列化。
- 对 URL 的列表的修改先在一份副本上做，URL 接受了副本的序列化之后，列表才换成这份副本，所以 URL 拒绝的修改让两者都保持原样。

### 用 Register 装上

`Register(vm, opts ...Option) error` 的形状和 `intl.Register`、`temporal.Register` 一样：必须在页面运行之前调，同一个 VM 上调第二次是错误，也不需要释放什么，`Close` 都会做。它现在没有选项。和 `intl.Register` 的区别：它一次装好 `URL` 和 `URLSearchParams`，而 `intl` 先放占位，第一次用时才装其余的。一个注册了 `url` 却从不造 URL 的 VM 多占 37 KiB（新 VM 的 `MemoryUsedSize` 从 75 KiB 到 113 KiB），多花 243 µs（见代价和收益）。tifa 商店页每次渲染都造 URL，用占位只是把代价挪个地方。

脚本每个进程只编译一次，不带源码和调试信息（所以 `toString` 显示 `[native code]`），每个 VM 都用 `EvalBytecodeValue` 加载同一份字节码，和 `intl` 一样。它不用 `SharedBytecode`：去掉了源码的脚本没有源码和行号表可以共享。任意多个 goroutine 上的 VM 共用这些字节，VM 加载时把它们复制进自己的内存。一个加载 bundle 共享字节码的 VM 池照常工作：注册是每个 VM 一次，不碰 bundle。

### 页面改了内建对象

`url.js` 在 `Register` 运行它时，也就是任何页面代码之前，把 `Reflect.apply`、`Object.defineProperty`、`String.prototype.toWellFormed` 和其他要用的内建对象取下来，除此之外什么也不用：对象和数组字面量，给要往里追加的数组用没有原型的数组，用下标的循环，不用模板字面量（QuickJS 把它变成对 `String.prototype.concat` 的调用），自己的数组上不用展开、解构和 `for...of`。URL 和列表在私有字段里，页面读不到、伪造不了、也换不掉。`TestTamper` 在页面改过下面这些东西的 VM 上跑一组调用：`Array.prototype` 的方法和下标 setter，`Object.prototype` 的访问器，字符串方法，`Object` 和 `Reflect` 的函数，`Function.prototype.call`、`apply` 和 `bind`，`JSON`，`String`、`Object`、`Array` 和 `Symbol` 这些全局对象，以及 `URL` 全局对象本身。每个 VM 上这组调用的答案都一样，页面设的陷阱一个也没被调到。页面确实能看到的，浏览器里也看得到：`URLSearchParams` 的序列形式会读它拿到的对象的 `Symbol.iterator`。

### 限额

解析的工作在 Go 里，引擎的轮询够不到那里，内存上限也不算 Go 分配的内存。这个包守住限额的办法，是让每次 Go 的调用都很短，并且在开始之前就知道它要多少内存，而不是在 Go 的循环里做检查。

- **一个字符串最多 65,536 个 UTF-16 单位，进来和出去都是。** Builtin 收到的每个字符串都经过同一个函数，`url/door.go` 里的 `admit`。它读引擎为这个字符串存着的长度（`Args.StringLen`，rope 也就是引擎分成几段存的字符串，它也不用拼起来就能读到长度），有字符串长过 `whatwgurl.MaxUnits` 就拒绝这次调用，检查一次剩余空间，然后才读这些字符串，读成 `whatwgurl.Input`，别的地方造不出这种值。解析器不造 href 会超过这个长度的 URL，表单序列化也不造会超过的编码，所以这个包返回的每个字符串都能再交回给它：脚本手里的 URL 总能被修改、读成列表、设成它自己。
- **超过之后脚本看到什么。** 超过 64 KiB 的 `data:` URL，比如一张小图片，在这里不是 URL。浏览器接受更长的：Chromium 到 2 MiB，Firefox 到 1 MB，`data:` URL 到 32 MB。

  | 调用 | 超过限额时 |
  | --- | --- |
  | `new URL(input, base)` | TypeError：输入或 base 按原样长过 65,536 个单位（解析器会去掉的空格、制表符和换行也算），或者 href 会长过这个数。路径里 7,281 个 CJK 字符就太长：百分号编码之后每个是 9 个单位 |
  | `URL.parse`、`URL.canParse` | `null`、`false` |
  | `href` 以外的 setter | URL 保持原样 |
  | `href` 的 setter | TypeError |
  | 特殊协议的非 ASCII 主机 | `http`、`https`、`ws`、`wss`、`ftp` 和 `file` 的主机要过 IDNA，百分号解码之后、IDNA 映射之后或规范化之后长过 1,024 个单位，就不是主机：1,024 个软连字符（映射会把它们删掉），或 1,024 个会变成 IPv4 地址的全角数字，都太长，257 个 U+3300（映射成四个片假名）也太长：映射之后超过 1,024 个单位的域名被拒绝。ASCII 的主机，和别的协议的主机（不过 IDNA，只做百分号编码），除了 URL 本身的限额没有别的限额：`URL.canParse("x://" + "é".repeat(1025))` 是 true，换成 `http://` 是 false（`TestDomainLimit`） |
  | 查询串里的一个名或值 | 长过 65,536 个单位并且含有 `+` 或 `%` 时，`URLSearchParams` 的构造函数抛 RangeError（不含它们的不用解码，没有限额），编码之后会更长时，`toString` 抛 RangeError |
  | URL 的列表上的 `append`、`delete`、`set` 和 `sort` | href 会长过限额时抛 RangeError，列表和 URL 都不变 |

- **时间。** 字符串这么短，一次 Go 的调用几毫秒就做完。它的工作量和输入成正比，只有主机的 IDNA 例外，而 1,024 个单位的限额把它也限住了。最慢的调用是限额长度的一条 CJK 字符路径：Apple M4 上 2.0 ms，一台 x86-64 服务器上 5.6 ms，都在引擎两次检查停止标志之间的目标 12.5 ms 以内，也就是杀死宽限 50 ms 的四分之一。限额长度的主机，IDNA 在 M4 上最多 0.24 ms。每次调用在开始时看一次停止标志。引擎为下一次轮询计的是它运行的字节码，不是 Go 函数用的时间，所以在一个调用几毫秒的循环里，两次轮询之间会有 9 到 75 次调用。有了这一眼，eval 超时和 `Interrupt` 在下一次调用时，或者在两次调用之间 javascript 的下一次轮询时结束脚本，看哪个先到。
- **n log n 的 Punycode。** golang.org/x/net 里 RFC 3492 的 Punycode 编码器，每个不是 ASCII 的码点都把整个标签过一遍，所以码点各不相同的标签，代价是长度的平方：1,024 个单位时每个单位 4 µs，8,192 个时 16 到 29 µs。这里的编码器（`internal/whatwgurl/idna/encode.go`）输出相同，用 Fenwick 树计数，时间是 n log n：每个单位 48 到 60 ns。
- **列表上的循环在 javascript 里。** Go 从来见不到列表，也见不到整个查询串。`url.js` 用它事先拿好的 `indexOf` 和 `slice`，在 `&` 和 `=` 处切开查询串，只有含 `+` 或 `%` 的名或值才调 Go，列表的排序、遍历和序列化都由它自己做。这些循环和任何脚本一样，受引擎的轮询和内存上限管。
- **内存。** 在读字符串之前，一次调用用 `vmhook.Room` 检查这些空间：它接受的每个单位 160 字节，它可能解析的每个主机 128 KiB（带 base 的解析算两个，`host` 和 `hostname` 的 setter 算两个，别的 setter 算一个，因为 `FromParts` 会再解析一次记录里的 hostname，别的调用不算），再加 16 KiB，装不下就以内存不足错误结束（`vmhook.OutOfMemory`）。主机单独算空间，是因为 IDNA 按单位算代价最高，而一个短字符串能造出一个长域名：U+FDFA 映射成 18 个单位，是表里映射得最长的码点（`idna.Widest` 找出它），所以 57 个就造出一个 1,026 个单位的域名。映射一超过 1,024 个单位，域名就被拒绝，所以后面的工作不管字符串是什么，最多只对 1,024 个单位做。测试里 Go 为一个主机分配得最多的是 81 KB，一个 Punycode 标签，解码出来每个字符两个单位。空间是给调用进行中 Go 同时占着的内存的。测试量的是 Go 在调用里一共分配了多少，比这个多，所以这个检查偏向安全的一边。一个进程第一次做规范化要用的表（190 KB）在 `Register` 时就建好，不放到第一次需要它们的调用里，那次调用没有为它们检查过空间。
- **一个记录的各部分加起来最多 65,536 个单位。** setter 接受新值和记录的八个部分，每个最多 65,536 个单位。一个 URL 的各部分加起来比它的 href 短，所以不超过限额，但它们来自 javascript，门在读它们之前检查这一点：各部分加起来更长的调用是 TypeError（`TestDoorSumsTheParts`）。空间按全部九个字符串检查。
- **调用结束后什么也不留。** Go 为一次解析造出的东西，Builtin 返回时就是垃圾。包里没有哪个变量留着脚本给的东西，或者会变的东西（`TestNoPackageState` 读源码，遇到不是 Builtin、不是编译好的脚本、不是测试钩子的变量就失败）。这个包早先的版本为了省掉页面里一百次对 `http://localhost` 的解析，把解析过的 base 放在缓存里，现在不留了：它们是 Go 的内存，不算进任何 VM，VM 关了还留着。
- **包 quickjs 的缓冲。** `Args.String` 把长过 64 个单位的字符串的 UTF-8 写进一块从 `sync.Pool` 里取的缓冲，用完放回去留给下一个字符串，所以 Go 读过的字符串的字节可能留在里面。读 n 个单位时，取到的缓冲装得下 3n 字节就把它原样放回，否则放回一块正好 3n 字节的（根包里的 `TestScratchPutBack`），所以经过门（n 最多 65,536）的话，这个包的调用造出的缓冲不超过 192 KiB。池里更大的缓冲是进程别处读更长的字符串造的，而且都不超过 2^18+1 个单位每个 3 字节，因为更长的字符串分段读。长度被拒绝的调用什么也不读。池留几块缓冲、留多久由 Go 决定：回收时或任何时候都可能丢掉其中任何一块。它们是进程的内存：不算进任何 VM 的上限，VM 关了也不走。

限额的测试：

- `TestDoorIsTheOnlyWayIn` 读源码：`url` 里 `door.go` 以外的文件读了参数、给 javascript 造了字符串或者造了 `Input`，有文件引入 `internal/whatwgurl/idna`，或者 `internal/whatwgurl` 导出了 `NewInput` 以外接受字符串的函数，它就失败。`TestDoorCheckCatches` 检查它在一个新文件里能找出每一种。
- `TestDoor` 给每个入口一个 65,537 个单位的字符串、一个 16 倍限额的字符串和一个 2^30 − 1 个单位的 rope（引擎最长的字符串）：每次调用的结果和上表一样，在 Go 里分配不到 1 KiB，用时不到 200 µs，每种长度都一样，rope 也没有被拼起来。在限额上，每个都能造出来。`TestStringLen` 拿门读到的长度和 javascript 看到的长度比。`TestDoorNineLikeOne` 检查九个字符串的调用在门里分配的不比一个字符串的多：都是零次（`Args.StringLen` 不分配）。
- `TestRoomCoversGo` 量 Go 在每次 Builtin 调用里分配多少，31 个入口，44 种限额长度的输入形状，有调用分配的超过它检查过的空间就失败。最多用到 43%（一个 CJK 字符的 userinfo）。其中八种是主机：用 `idna.Widest` 在表里找出的码点，映射得最长的那个和分解得最长的那个（U+FDFA 到 18，U+01D6 到 3，`TestWidest` 检查这两个），放在域名按原样和映射之后的限额上，还有解码出来每个字符两个单位的 Punycode 标签。`FuzzRoom` 对 fuzzer 造出的文本做同样的事，一个单元重复 fuzzer 选的次数，种子里有这些码点。`TestFirstNormalizationCounted` 在一个新进程里跑，检查第一个需要 NFC 的主机不超出它的空间。
- `TestTimeout`、`TestInterrupt` 和 `TestOutOfMemory` 跑 14 个永不结束的脚本，每个都卡在 URL 或列表的工作里：限额长度的解析、setter 和 IDNA，一个 16 MiB 的查询串一遍又一遍地切，一个 262,144 对的列表反复排序、遍历和序列化，URL 的列表反复修改。每个都在超时之后 50 ms 内以中断结束，或者在只剩 8 MiB 空间时以内存不足错误结束，之后 VM 照常可用。`TestStopAtNextCall` 在每个脚本的第五次 Go 调用里要求停止，检查之后没有调用再开始。`TestNoRoomNoRead` 检查空间不够的调用在 Go 读它的字符串之前就结束。
- `TestAtTheLimit` 和 `TestOwnOutputGoesBack` 在限额上和超过一个单位时检查上表，并检查限额长度的 URL 能被修改、设成它自己、读成列表。`internal/whatwgurl` 里，`TestNewInput`、`TestSpacesCount`、`TestNothingLonger`、`TestDomainLimit` 和 `TestFormEncodeLimit` 对解析器做同样的检查（`TestDomainLimit` 也检查映射之后在限额上的域名），`TestWorstAtLimit` 给 36 种限额长度的形状，以及表里增长最多的码点组成的主机，各计时一次解析，和 12.5 ms 比（乘上机器当时比平常慢多少）。
- `TestEncodeRandom` 在 200,000 个随机标签上（最长 1,024 个码点）拿 Punycode 编码器和 RFC 3492 的比，`TestEncodeWPT` 在 web-platform-tests 的 IDNA 文件里的每个标签上，以及其中每个 Punycode 标签解码出来的东西上比。`TestEncodeLinear` 检查 1,024 个不同码点的标签用时最多是 256 个的 10 倍（RFC 的编码器是 16 倍）。

### 解析器

`internal/whatwgurl` 是 2026 年 URL 标准的解析器：带 state override 的基本 URL 解析器、主机解析器（IPv4、IPv6、opaque 主机）、百分号编码集、序列化器、API 的 setter 和 origin。[SOURCES.md](../../internal/whatwgurl/SOURCES.md) 写着什么来自哪里。

- 它起初是 github.com/nlnwa/whatwg-url v0.6.2 的 `url/`（Apache-2.0，2025-03）。那个库在它自己 2023 年的 WPT 数据上全过，在今天的数据上，`urltestdata.json` 904 个用例错了 15 个，setter 的 278 个错了 11 个，`toascii.json` 的 87 个错了 7 个（配 golang.org/x/net v0.62）。这些来自 2024 到 2025 年标准的三处改动，现在都有了：opaque path 末尾的空格，后面跟着 `?` 或 `#` 时写成 `%20`，`^` 在 path 的百分号编码集里，域名是 ASCII 而 domain to ASCII 不严格时不过 IDNA，所以 punycode 无效的 `xn--` 标签照写保留。
- 然后它是重写的，不是打补丁。那个库把输入读成 rune 列表，要看后面是什么时就复制整个输入，一个字符一个字符地往 userinfo 字符串上加（所以长密码的代价是长度的平方），带着选项、编码覆盖和验证错误列表，也没有停止检查。这里的状态机就在原处读 UTF-8，用字节偏移当指针。对着 `http://localhost` 和一个商店路径，它用 0.55 µs 和 14 次分配，那个库是 1.9 µs 和 44 次。
- IDNA 的代码是 golang.org/x/net v0.62.0 的 `idna/`，只留一个配置（CheckBidi 和 CheckJoiners 开，不做 transitional 处理，用 UTS #46 第 35 版的算法），每个 Go 版本都用 Unicode 17 的表（x/net 在 Go 1.27 之前用 Unicode 15），Bidi 的表和规则来自 golang.org/x/text v0.44.0，NFC 来自 `internal/gointl`。不需要新的模块。punycode 解码器保留 x/net 的限制，一个标签最多 1,024 个字符。

数据是 web-platform-tests 在 2026-11-12 的提交 `8af128e` 里的 URL 数据（`internal/whatwgurl/testdata`）。`TestWPTParse`、`TestWPTSetters`、`TestWPTToASCII` 和 `TestWPTPercentEncoding` 在解析器上跑它，每个用例都比 href、各个部分和 origin。除了 `IdnaTestV2.json` 里的 7 个用例，其余全过，这 7 个列在 `testdata/known-failures.txt`：这个文件来自比 Unicode 17 更新的 `IdnaTestV2.txt`，里面 U+3E8AC 和 U+3F8CD 是合法的，而这里的表把它们当作不允许。一个列在里面却通过的用例会让测试失败，所以这个列表只会变短。

`url/` 在 VM 里跑同样的数据（`TestWPT`）：web-platform-tests 的 `url/` 里 23 个文件，`url-constructor`、`url-origin`、`url-setters`、`url-setters-stripping`、`url-statics-canparse`、`url-statics-parse`、`url-tojson`、`url-searchparams`、`urlencoded-parser`、11 个 `urlsearchparams-*`、`historical`、`IdnaTestV2` 和 `IdnaTestV2-removed`，原样放在 testharness.js 的几个函数上跑（`testdata/harness.js`）。跑了 4,852 个测试。81 个列在 `url/testdata/known-failures.txt` 里，每个都有原因：72 个调用 `Request`、`Response`、`FormData` 或 `DOMException`，它们属于宿主，不属于引擎，2 个要 `structuredClone`，7 个是上面的 IDNA 用例。core-js 的检查原样运行 core-js 3.50.0 的 `internals/url-constructor-detection.js`，以及 `web.url.can-parse` 对 `URL.canParse` 的测试（`TestCoreJSDetection`）：两者都说原生的够好。

### 脚本能看到的和 core-js 不同的地方

- `Object.getOwnPropertyNames(url)` 是空的。tifa 的 core-js 把状态存在一个属性里（`'\0tifa-core-js-property-state'`），脚本看得到，而浏览器里没有。
- 主机按 UTS #46 处理，有映射、CheckBidi 和 CheckJoiners，core-js 只有 punycode：全角字母、`xn--` 标签和 IPv4 主机的边角情况，结果和浏览器一样。tifa 页面传的相对路径是 ASCII 的，输出不变。
- 出错都是 `TypeError`，和浏览器一样，只是措辞不同。
- 长过 65,536 个 UTF-16 单位的输入、base 或 href 不是 URL，特殊协议的非 ASCII 主机按原样或 IDNA 映射之后长过 1,024 个单位就不是主机，查询串里这么长、含有 `+` 或 `%` 的名或值是 RangeError，编码之后会这么长的也是，punycode 标签超过 1,024 个字符也被拒绝（x/net 的限制）：标准都没有这些。见限额。
- 没有 `URL.createObjectURL` 和 `URL.revokeObjectURL`：VM 没有 Blob。core-js 有会抛错的桩。
- core-js 装上自己的 `URLSearchParams` 时会包住 `fetch` 和 `Request`，给这种类型的 body 加上 content type。用原生的就不包了，whatwg-fetch 自己测 `URLSearchParams` 来做这件事。
- `structuredClone` 只拿它和 `URL.prototype` 比，所以不受影响。

### 和 Node 的不同，以及体积

解析器还和 Node 24（ada 3.3.0，ICU 的 Unicode 16）比过：对着 8 个 base 的 60,000 个随机输入。VM 里的 URL 也和它比过：60,000 个随机 URL 各用一个 setter 改，60,000 个各用两个，60,000 个各用三个，值有随机的，也有专门走到 setter 边角的（file、localhost、盘符、空主机、默认端口）。Node 在这几处和标准不同，这个包按标准来（Chromium 也一样，它检查 Bidi 规则）：

- Node 不用 CheckBidi：`new URL("http://example.com\u0627")`（一个标签里拉丁字母加一个阿拉伯字母）在 Node 里是个 URL，在这里和浏览器里是失败。
- Node 24 对 ASCII 的、带 punycode 无效的 `xn--` 标签的域名（`http://xn--a`）会失败。2025 年的标准照写保留它，这里也是。
- Node 对着有 opaque path 的 base 解析带 `#` 的输入会成功（`new URL("0x#@", "data:text/plain,x")` 是 `data:text/0x#@`）。标准是失败，这里也是：对这种 base，只有以 `#` 开头的输入才能解析。
- 把没有主机的 URL（`x:/`）的 `host` 或 `hostname` 设成空值，或以 `/` 开头的值，Node 里 URL 保持原样。标准给它一个空主机（`x:///`），和 `view-source+http://example.net/foo` 一样，web-platform-tests 测的就是这个。
- Node 把 URL 存成 href 并重新解析（ada 的 `url_aggregator`），所以解析器从 href 造不出来的记录到后面的 setter 就丢了：`http://localhost/` 或 `http://h/C|/x` 设了 `protocol = "file"` 之后，Node 在下一个 setter 把主机变成空的，或把盘符变成 `C:`，也不让把协议改回去。标准和这个包两者都保留，Chromium 也保留，只是它在任何路径里都把 | 写成 `%7C`。
- 在没有主机的 URL 上把 `pathname` 设成以 `//` 开头的值，Node 会丢掉查询和片段。标准保留它们。
- URL 之后有了主机时，Node 还留着以 `//` 开头的路径前面的 `/.`（`x://C/.//`）。标准只在没有主机时才写它（`x://C//`）。

去掉撞上前三条的输入，两者在 60,000 次解析上完全一致。用一个、两个和三个 setter 改过的 URL 里，分别有 100、243 和 341 个不同，每个都属于后四种之一：没有主机的 URL 得到空主机（438 个），Node 重新解析 href（184 个），查询或片段被丢掉（60 个），以及 `/.`（2 个）。

包 `url` 为了 NFC 链接了 `internal/gointl`，连带 go-intl 内嵌的数据。一个链接了 `url` 而没有 `intl` 的程序会大 8.1 MB（darwin/arm64 上，带一个 VM 和一次 `Eval`，从 8.1 MB 变成 16.2 MB）。已经链接了 `intl` 的程序本来就有这些，不会再多。

### 代价和收益

除非另外说明，都是在 Apple M4 上，`GOMAXPROCS=1`。

**在 Go 里。** `BenchmarkShopURL`（`internal/whatwgurl`）：对着已经解析好的 `http://localhost` 解析一个商店路径，再把它连同各部分序列化出来，用 0.55 µs 和 14 次分配。起点的那个库在同一台机器上是 1.9 µs 和 44 次分配（选解析器时量的）。

**在 VM 里。** `BenchmarkShop`（`url`）：`new URL(path, "http://localhost")`，写一次 `pathname`，再读 protocol、username、password、origin、pathname、search 和 hash，用 4.67 µs，Go 里 49 次分配、1,816 字节。检查交给 setter 的记录是不是 URL 的记录，要把它的 href 再解析一遍，占其中 0.66 µs、12 次分配和 432 字节：没有这个检查是 4.01 µs。core-js 的 polyfill 做同样的事要 42 到 87 µs。

**Register 给一个 VM 加多少。** `BenchmarkRegister`（`url`），3 次各 2 s：只 `NewVM` 是 81 µs，`NewVM` 加 `Register` 是 324 µs，所以字节码编译好之后 `Register` 用 243 µs（进程里第一次调用要编译它）。新 VM 的 `MemoryUsage`：`MemoryUsedSize` 没有它是 77,128 字节，有它是 115,465（多 37.4 KiB），`MallocSize` 是 202,872 和 318,072（多 112.5 KiB），引擎自己数的对象从 164 个变成 258 个，shape 从 116 个变成 154 个。一个链接了 `url` 而没有 `intl` 的程序大 8.1 MB（见上）。

**在 tifa 里。** 一份 tifa 的临时副本，用 `replace` 指到这份工作树，在 bundle 运行之前注册这个包（`internal/ssr.Load`）。商店和 MUI 示例的 23 个页面，有它和没有它的热渲染，6 轮，两者交替（每轮把顺序转一下），每个分组和每个版本一个进程，`GOMAXPROCS=1`，机器的通道都占着，但旁边还有别的工作（负载 4 到 7）。时间是每个页面 6 轮渲染的中位数，再对各页取平均，单位是每次渲染的 ms：

| 页面 | 每次渲染的 `new URL` 次数（平均，范围） | 没有 | 有 | 变化 |
| --- | --- | --- | --- | --- |
| 商店，14 个页面（7 个页面两种语言） | 107（69 到 169） | 19.88 | 13.30 | -33.1% |
| 工作台，6 个页面 | 25（17 到 40） | 8.99 | 7.73 | -14.0% |
| MUI，3 个页面 | 2 | 12.45 | 12.43 | -0.2%，在噪声之内没有变化 |

每个 URL 省下的时间，商店页面是 61 µs（107 个 URL 省 6.5 ms），工作台页面是 50 µs，也就是 polyfill 的代价减去剩下的 5 µs。商店里最好的一页，zh-CN 的 journal，时间降到 43%（7.75 ms 到 3.30 ms），zh-CN 的商品列表有 169 个 URL，从 36.09 ms 降到 23.78 ms。23 个页面的 HTML 在有这个包和没有这个包时逐字节相同，每一轮都是（每个进程第一次渲染的 SHA-256）。次数是一次热渲染里 `new URL` 的调用次数，是在单独的一次运行里，用全局上的 `Proxy` 数出来的。

处理器执行完的指令数说的是同一件事（macOS 的 `/usr/bin/time -l` 会数它）。每个分组和每个版本，一个进程在 5 次热渲染之后把每个页面渲染 5 次，另一个只渲染 1 次：两者之差是每个页面 4 次渲染。三次这样的运行取中位数，按每次渲染的指令数：

| 页面 | 没有 | 有 | 变化 |
| --- | --- | --- | --- |
| 商店，14 个页面 | 272.8 M | 174.5 M | -36.0% |
| 工作台，6 个页面 | 112.5 M | 95.7 M | -15.0% |
| MUI，3 个页面 | 147.3 M | 146.4 M | -0.6% |

这些运行里，每个页面在每个进程里也只有一个 SHA-256，有这个包和没有这个包都一样。

### 测试

`go test ./url ./internal/whatwgurl/...`，也带 `-race`，也用 `GOTOOLCHAIN=go1.26.6`。`internal/whatwgurl` 里有：WPT 数据，上面那些限额的测试，`TestPartsKeepTheRecord`、`TestFromPartsTakes` 和 `TestFromPartsRefuses`（解析器从 href 造不出来的那两种记录能从各部分再造出来，各种 URL 的各部分都被接受，46 种不是 URL 的各部分都以错误拒绝：不是序列化器写法的协议、端口、主机、用户、路径、查询或片段，和各部分对不上的位，加起来超过限额的各部分），`TestPunycodeLabelChecks`，`TestParallel`（16 个 goroutine，主机要过 NFC）和四个 fuzz 目标，`idna` 里有编码器的三个测试和 `TestWidest`。`url` 里有：`TestWPT`、`TestCoreJSDetection`、`TestCoreJSKeepsNative`、`TestBattery` 和 `TestTamper`、`TestShape`、`TestParallelVMs`、`TestConversion...`、`TestSequenceConvertsFirst`、`TestManyURLsCountedInVM`、`TestSettersKeepTheRecord`（改过协议之后，setter 保留主机 `localhost` 和盘符 `C|`），`TestForgedRecord`（29 种伪造的 `set` 参数，每种都是 TypeError，URL 保持原样），`TestStateIsUnreachable`（见一个 URL 怎么存）和上面那些限额的测试。根包里有 `TestScratchPutBack`。

fuzz 目标用 WPT 数据当种子。`internal/whatwgurl/fuzz_test.go` 里：`FuzzParse`（不超过 65,536 个单位的输入不会 panic，造出来的东西是 ASCII，不超过 65,536 长，再解析得到同一个记录），`FuzzSetters`（每个 setter 留下的记录能从它的各部分再造出来，它的 href 是个 URL，因为太长而拒绝的，URL 保持原样），`FuzzFromParts`（把一个 URL 的某个部分换掉、把位翻转：`FromParts` 从不 panic，它接受的能原样还回来，也过得了 `FuzzSetters` 的检查）和 `FuzzLimit`（一个 65,536 个单位、由前缀、重复的单元和后缀拼成的输入，解析它，或者在一个长 URL 上用它调 setter，用时不超过 12.5 ms 乘上代码当时慢了多少倍）。`url/room_test.go` 里：`FuzzRoom`（一次调用里 Go 分配的不超过它检查过的空间）。跑一个用 `go test -run '^$' -fuzz '^FuzzParse$' -fuzztime 3m ./internal/whatwgurl`。让测试失败的输入放进 `testdata/fuzz/<目标>/`，之后 `go test` 把它当测试跑，和修复一起提交。
