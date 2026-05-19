# sharelink CLI + Homebrew 分发 — 设计文档

- 日期:2026-05-19
- 状态:已批准,待实现
- 作者:procmeans (with Claude)
- 关联:`2026-05-10-douyin-direct-link-design.md`、`2026-05-10-ttwid-and-cache-design.md`

## 1. 目标与范围

把 sharelink 从纯 HTTP 服务扩展为同时提供命令行工具,并通过 Homebrew tap 分发,让用户能 `brew install sharelink` 后直接 `sharelink parse <url>` 拿到去水印 mp4 直链。

**范围内**:
- 单二进制 + 子命令架构(`sharelink parse` / `sharelink serve` / `sharelink version`)。
- CLI 同时支持 argv 和 stdin 输入,默认输出纯 URL,`--json` 输出 JSON。
- CLI 默认不拉 ttwid,`--with-ttwid` 触发拉取。
- 用 stdlib `flag` 包做子命令分发,**不引入 cobra 等外部依赖**。
- 让 `internal/douyin.Service` 容忍 `nil` TTWidManager。
- goreleaser 配置 + GitHub Actions release workflow + 自动推 formula 到 tap repo。
- 新建 `procmeans/homebrew-tap` repo,formula 由 goreleaser 自动维护。
- 根目录加 LICENSE(MIT,作者 procmeans,年份 2026)。

**范围外**:
- shell completion(bash/zsh/fish)。
- 配置文件支持(`--config foo.toml`)。
- nightly / snapshot 发布通道,只在 git tag 推送时发版。
- Linux 包管理器(deb/rpm/apt/aur),只做 brew + raw binaries。
- Windows 二进制(可后续轻松加,但当前用不到)。
- 自动 changelog 生成之外的发版自动化。

## 2. 模块结构

```
sharelink/
├── cmd/sharelink/                # 新增,取代 cmd/server/
│   ├── main.go                   # 子命令 router
│   ├── parse.go                  # parse 子命令
│   ├── serve.go                  # serve 子命令(由原 cmd/server/main.go 改造)
│   ├── version.go                # version 子命令 + 版本变量
│   └── run.go                    # run(args, stdin, stdout, stderr) int — 测试入口
├── cmd/server/                   # 删除(整个目录)
├── internal/
│   └── douyin/service.go         # 改:容忍 nil ttwid
├── .goreleaser.yaml              # 新增
├── .github/workflows/release.yml # 新增
├── LICENSE                       # 新增(MIT)
└── README.md                     # 改:CLI 用法 + brew 安装
```

## 3. CLI 接口

### 3.1 顶层

```
$ sharelink
sharelink: extract watermark-free Douyin video URLs

Usage:
  sharelink <command> [flags]

Commands:
  parse     Extract a direct link from share text or short URL
  serve     Run as HTTP API service
  version   Print version information

Run 'sharelink <command> -h' for command-specific flags.
```
无子命令或未知子命令 → 输出 usage 到 stderr,exit code 2。

### 3.2 `sharelink parse`

```
Usage:
  sharelink parse [flags] [input]

Flags:
  --json           Output JSON {play_url, video_id} instead of plain URL
  --with-ttwid     Fetch ttwid cookie before parsing (~+500ms, may improve success rate)
  -h, --help

Input:
  If [input] is given on argv, use it directly.
  Otherwise read from stdin until EOF.

Exit codes:
  0  success
  1  parse/upstream error (writes error message to stderr)
  2  usage error (unknown flag, missing input from both argv and stdin, etc.)
```

**默认输出(stdout)**:
```
https://aweme.snssdk.com/aweme/v1/play/?video_id=...&ratio=720p&line=0
```
(一行,结尾 `\n`,无任何额外内容。)

**`--json` 输出**:
```json
{"play_url":"https://...","video_id":"7234567890123456789"}
```
(一行,结尾 `\n`。注意 `cache_hit` 和 `ttwid_refreshed` 不出现在 CLI JSON 输出里 — 那俩字段只对 server 的日志有意义。)

**错误输出(stderr)**:
```
sharelink: <error message>
```
exit 非零。stdout 不写任何东西。

### 3.3 `sharelink serve`

```
Usage:
  sharelink serve [flags]

Flags:
  --port <n>       Listen port (default: $PORT or 8080)
  -h, --help

Behavior:
  Identical to the current cmd/server binary:
  - Blocks at startup to fetch ttwid; failure exits 1.
  - Cache: LRU 1000 entries × 30 minute TTL.
  - HTTP endpoints: POST /api/parse, GET /healthz.
  - JSON slog output.
```

环境变量 `PORT` 仍然支持。`--port` 优先级高于 `PORT`。

### 3.4 `sharelink version`

```
Usage:
  sharelink version

Output:
  sharelink <version> (commit <commit>, built <date>)
```

`version`/`commit`/`date` 通过 goreleaser 在编译时 `-X main.xxx` 注入;本地 `go build` 时分别是 `dev`/`none`/`unknown`。

## 4. 实现细节

### 4.1 `run()` — 可测试入口

`cmd/sharelink/run.go`:
```go
package main

import (
    "context"
    "io"
)

// run is the testable entry point. main() is just `os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))`.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
    // args[0] is program name (e.g., "sharelink"); args[1] is subcommand.
    if len(args) < 2 {
        printUsage(stderr)
        return 2
    }
    switch args[1] {
    case "parse":
        return runParse(args[2:], stdin, stdout, stderr, realExtractorFactory)
    case "serve":
        return runServe(args[2:], stderr)
    case "version":
        return runVersion(stdout)
    case "-h", "--help", "help":
        printUsage(stdout)
        return 0
    default:
        fmt.Fprintf(stderr, "sharelink: unknown command %q\n", args[1])
        printUsage(stderr)
        return 2
    }
}
```

`main()`:
```go
func main() {
    os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
```

### 4.2 `runParse`

`runParse` takes an `extractorFactory` so tests can inject a stub instead of constructing a real `Service` that calls real Douyin.

```go
// extractorFactory returns an extractor function given the --with-ttwid flag.
// Real factory builds a Service; tests inject a stub.
type extractorFactory func(withTTWid bool) (func(input string) (douyin.Result, error), error)

func runParse(args []string, stdin io.Reader, stdout, stderr io.Writer, mkExtractor extractorFactory) int {
    fs := flag.NewFlagSet("parse", flag.ContinueOnError)
    fs.SetOutput(stderr)
    jsonOut := fs.Bool("json", false, "output JSON instead of plain URL")
    withTTWid := fs.Bool("with-ttwid", false, "fetch ttwid cookie before parsing")
    if err := fs.Parse(args); err != nil {
        return 2
    }

    input, err := readInput(fs.Args(), stdin)
    if err != nil {
        fmt.Fprintln(stderr, "sharelink:", err)
        return 2
    }

    extract, err := mkExtractor(*withTTWid)
    if err != nil {
        fmt.Fprintln(stderr, "sharelink:", err)
        return 1
    }

    res, err := extract(input)
    if err != nil {
        fmt.Fprintln(stderr, "sharelink:", err)
        return 1
    }

    if *jsonOut {
        return writeJSON(stdout, res)
    }
    fmt.Fprintln(stdout, res.PlayURL)
    return 0
}

// realExtractorFactory constructs a Service with a no-op cache and optional
// ttwid manager, then returns the bound Extract method.
func realExtractorFactory(withTTWid bool) (func(string) (douyin.Result, error), error) {
    c := cache.New(0, 0)
    var ttwid *douyin.TTWidManager
    if withTTWid {
        ttwid = douyin.NewTTWidManager(httpclient.NewNoRedirect())
        if err := ttwid.Refresh(); err != nil {
            return nil, fmt.Errorf("ttwid refresh: %w", err)
        }
    }
    return douyin.NewService(ttwid, c).Extract, nil
}

// readInput returns input from extraArgs[0] if any, else reads all of stdin.
func readInput(extraArgs []string, stdin io.Reader) (string, error) {
    if len(extraArgs) > 0 {
        return strings.TrimSpace(extraArgs[0]), nil
    }
    b, err := io.ReadAll(stdin)
    if err != nil {
        return "", err
    }
    s := strings.TrimSpace(string(b))
    if s == "" {
        return "", errors.New("no input on argv or stdin")
    }
    return s, nil
}
```

### 4.3 `runServe`

是把当前 `cmd/server/main.go` 的 `main()` 函数主体搬过来,改成接受 `args []string` 参数,加上 `--port` 解析:

```go
func runServe(args []string, stderr io.Writer) int {
    fs := flag.NewFlagSet("serve", flag.ContinueOnError)
    fs.SetOutput(stderr)
    portFlag := fs.String("port", "", "listen port (defaults to $PORT or 8080)")
    if err := fs.Parse(args); err != nil {
        return 2
    }

    slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

    ttwid := douyin.NewTTWidManager(httpclient.NewNoRedirect())
    if err := ttwid.Refresh(); err != nil {
        slog.Error("ttwid refresh failed at startup", "err", err.Error())
        return 1
    }
    slog.Info("ttwid acquired", "len", len(ttwid.Cookie()))

    c := cache.New(1000, 30*time.Minute)
    svc := douyin.NewService(ttwid, c)

    port := *portFlag
    if port == "" {
        port = os.Getenv("PORT")
    }
    if port == "" {
        port = "8080"
    }

    srv := &http.Server{
        Addr:         ":" + port,
        Handler:      httpapi.NewHandler(svc.Extract),
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
    }

    slog.Info("server starting", "addr", srv.Addr)
    err := srv.ListenAndServe()
    if err != nil && !errors.Is(err, http.ErrServerClosed) {
        slog.Error("server failed", "err", err)
        return 1
    }
    return 0
}
```

### 4.4 `runVersion`

```go
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

func runVersion(stdout io.Writer) int {
    fmt.Fprintf(stdout, "sharelink %s (commit %s, built %s)\n", version, commit, date)
    return 0
}
```

### 4.5 Service 容忍 nil ttwid

在 `internal/douyin/service.go` 加两个小辅助方法:
```go
func (s *Service) cookie() string {
    if s.ttwid == nil {
        return ""
    }
    return s.ttwid.Cookie()
}

func (s *Service) refresh() error {
    if s.ttwid == nil {
        return errors.New("ttwid manager not configured")
    }
    return s.ttwid.Refresh()
}
```

然后 `ExtractWithConfig` 把直接调用 `s.ttwid.Cookie()` 改成 `s.cookie()`,`s.ttwid.Refresh()` 改成 `s.refresh()`。retry 那块如果 `s.refresh()` 出错就走原来的 "refresh 失败 → 返回原 err" 路径,不需要其他改动。

## 5. 测试

### 5.1 新增

| 模块 | 测试 |
|------|------|
| `cmd/sharelink/parse` | argv 输入正常解析(用 httptest 模拟 douyin);stdin 输入正常解析;argv 和 stdin 都为空 → exit 2;无效输入(无短链)→ exit 1 + stderr 写错误;`--json` 输出 JSON 含 play_url 和 video_id;默认输出纯 URL 一行 |
| `cmd/sharelink/run` | 无子命令 → exit 2;未知子命令 → exit 2;`version` 子命令打印注入的版本字符串(测试时 `version = "test-version"` 通过 `init()` 覆盖,验证输出含 "test-version") |
| `cmd/sharelink/serve` | flag 解析 `--port 0` 不应该让进程跑起来(测试只验证 flag 解析不 panic;端到端 server 测试已经在 httpapi 包覆盖) |
| `internal/douyin/Service` | nil ttwid:`ExtractWithConfig` 走通整条流水线(用现有 fakeDouyin);nil ttwid 遇 `ErrNoPlayURL` 不重试,直接返回原 err |

CLI 测试用 `httptest.NewServer` 模拟 douyin 短链 302 + share 页(复用现有 `fakeDouyin` 的思路,但因为 CLI 在外层包不能直接复用,需要在测试里自己实现一个小 fake 或者把 `fakeDouyin` 工厂从测试包移到一个 `internal/douyintest` 包)。

**决策**:**不**移 `fakeDouyin` 到独立包 — 在 `cmd/sharelink/parse_test.go` 里重新写一个 mini fake,接受 `--shareBaseURL` 这类内部 flag 不优雅。改用更简单的方案:加一个 `cmd/sharelink/extract_test.go` 测试通过 `httptest` 替换 `cfg.ShareBaseURL` 和 `cfg.ShortURL`,直接调 `svc.ExtractWithConfig` 而不是 `svc.Extract`。Parse 子命令本身只测 flag 解析和输出格式,服务端逻辑信任 Service 已经测过。

具体测试形态:
```go
// parse_test.go
func TestRunParse_DefaultOutput(t *testing.T) {
    // 不调真实 douyin。用一个 stub extract func 注入。
    ...
}
```

这个注入点已经在 §4.2 描述为 `extractorFactory` 参数。`run()` 在生产路径调 `runParse(..., realExtractorFactory)`;测试传入返回 stub extract 函数的 factory。

### 5.2 修改

- `internal/douyin/service_test.go`:加 2 个新测试覆盖 nil ttwid 路径。现有测试不变。
- `README.md`:加 CLI 安装/用法说明;原有 server 章节改为"或 `sharelink serve`"。

### 5.3 删除

- `cmd/server/`:整个目录(含 `main.go`、`doc.go` 若仍存在)。

## 6. 分发(goreleaser + GitHub Actions)

### 6.1 `.goreleaser.yaml`

放在 repo 根:

```yaml
version: 2
project_name: sharelink

builds:
  - id: sharelink
    main: ./cmd/sharelink
    binary: sharelink
    env:
      - CGO_ENABLED=0
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
      - -X main.date={{.Date}}

archives:
  - id: sharelink
    format: tar.gz
    name_template: '{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}'
    files:
      - LICENSE
      - README.md

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
      - '^chore:'

brews:
  - name: sharelink
    repository:
      owner: procmeans
      name: homebrew-tap
      token: '{{ .Env.TAP_GITHUB_TOKEN }}'
    directory: Formula
    homepage: https://github.com/procmeans/sharelink
    description: Douyin watermark-free direct link extractor
    license: MIT
    test: |
      system "#{bin}/sharelink", "version"
    install: |
      bin.install "sharelink"
```

### 6.2 `.github/workflows/release.yml`

```yaml
name: release

on:
  push:
    tags: ['v*.*.*']

permissions:
  contents: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAP_GITHUB_TOKEN: ${{ secrets.TAP_GITHUB_TOKEN }}
```

### 6.3 必须的人工准备步骤(spec 之外,但要文档化)

按顺序:
1. 在 GitHub 上创建 `procmeans/sharelink`(public)。
2. 在 GitHub 上创建 `procmeans/homebrew-tap`(public,空仓库,加 README 即可)。
3. 生成一个 Personal Access Token,scope 至少包含 `repo`(可以只针对 `procmeans/homebrew-tap` 一个仓库的 fine-grained PAT,只给 "Contents: read and write" 权限)。
4. 在 `procmeans/sharelink` 的 Settings → Secrets and variables → Actions 里加一个 secret 名为 `TAP_GITHUB_TOKEN`,值是第 3 步生成的 token。
5. `git remote add origin git@github.com:procmeans/sharelink.git && git push -u origin main`
6. 首次发版:`git tag v0.1.0 && git push origin v0.1.0`,workflow 自动触发。

这些步骤在 README 里有一节"维护者发版指南",一次性操作。

### 6.4 用户安装

```bash
brew tap procmeans/tap
brew install sharelink
sharelink parse "5.99 fyu:/ ... https://v.douyin.com/xxx/ ..."
```

## 7. LICENSE

根目录加 `LICENSE`,标准 MIT 模板,版权行:
```
Copyright (c) 2026 procmeans <procmeans@proton.me>
```

## 8. 跨切关注

### 8.1 向后兼容

- `cmd/server` 删除是破坏性变更,但唯一调用方是 README 的 `go build -o server ./cmd/server` 命令 — 用户没有生产部署。README 更新即可。
- HTTP 接口、Service / TTWidManager / Cache 的公开签名都不变。
- `sharelink serve` 在 `PORT` 环境变量、`/api/parse` 接口、`/healthz` 接口、JSON 日志格式上和现有 `./server` 完全一致。

### 8.2 二进制大小

加了 CLI 子命令分发但没引入新依赖,二进制大小预期变化 < 200KB。

### 8.3 已知风险

- goreleaser v2 schema 偶尔会出现 breaking change;通过在 workflow 里 pin 到 `~> v2` 而非 `latest` 来减少风险。
- 首次发版前用户必须手动完成 §6.3 的准备步骤;workflow 提示不友好(就是 "TAP_GITHUB_TOKEN: unbound variable")。README 维护者章节里写清。

## 9. 验收标准

- [ ] `go build -o sharelink ./cmd/sharelink` 产出单二进制。
- [ ] `./sharelink`(无参数)打印 usage 到 stderr,exit code 2。
- [ ] `./sharelink parse "https://v.douyin.com/R2mGwrMjlLw/"` 在网络可达时输出一行 mp4 URL,exit 0。
- [ ] `echo "...share text..." | ./sharelink parse` 等效。
- [ ] `./sharelink parse --json "..."` 输出形如 `{"play_url":"...","video_id":"..."}` 的一行 JSON。
- [ ] `./sharelink parse --with-ttwid "..."` 额外耗时约 500ms 但流程通畅。
- [ ] `./sharelink serve --port 18080` 与原 `PORT=18080 ./server` 行为一致(/healthz 返回 ok,/api/parse 正常工作)。
- [ ] `./sharelink version` 输出包含 `dev` 字样(本地构建)或 goreleaser 注入的版本(发版后)。
- [ ] `go test ./... -race -count=1` 全绿。
- [ ] `go vet ./...` 无告警。
- [ ] `goreleaser check` 确认 `.goreleaser.yaml` 合法(可在 CI 之外本地跑)。
- [ ] (人工)按 §6.3 准备好 GitHub repos + token + push tag 后,workflow 跑通,`brew tap procmeans/tap && brew install sharelink && sharelink version` 可在干净的 macOS 上验证。
