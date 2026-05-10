# sharelink:ttwid Cookie + 内存缓存 — 设计文档

- 日期:2026-05-10
- 状态:已批准,待实现
- 作者:procmeans (with Claude)
- 关联:基础设计 `2026-05-10-douyin-direct-link-design.md`

## 1. 目标与范围

为现有 sharelink 服务增加两项防护,降低被抖音风控/限制的概率,并减少对上游的不必要请求:

1. **ttwid cookie**:服务启动时从 `https://www.douyin.com/` 拿到 `ttwid` cookie,后续所有对 douyin/iesdouyin 的请求带上;遇到 share 解析失败(`ErrNoPlayURL`/code 2002)时刷新一次并重试 share 步骤。
2. **内存 LRU+TTL 缓存**:按 `video_id` 缓存最终的 `play_url`,TTL 30 分钟,容量 1000 条,满时驱逐最久未用的。

**范围内**:
- 新增 `internal/cache/` 通用 string→string LRU+TTL 缓存包。
- 新增 `internal/douyin/ttwid.go` ttwid 管理器。
- 新增 `internal/douyin/service.go` 把 `Extract` 重构成 `Service.Extract` 方法,持有 ttwid + cache 依赖。
- 调整 `ResolveVideoID` / `fetchPlayURLFromBase` 签名,增加 `cookie string` 参数。
- 调整 `cmd/server/main.go` 装配 Service。

**范围外**:
- 持久化缓存(进程重启即丢失,可接受 — 个人/内网用)。
- 缓存预热 / 主动失效。
- ttwid 之外的其他 cookie(如 `msToken`、`s_v_web_id`)。
- 自动降速 / IP 风控的进一步规避(代理池等)。
- 任何配置文件或新的环境变量;TTL/容量/douyin 首页 URL 都是硬编码常量。

## 2. 模块结构

新增 / 修改:

```
internal/
├── cache/                    # 新增
│   ├── cache.go              # 通用 string→string LRU+TTL 缓存
│   ├── cache_test.go
│   └── doc.go
├── douyin/
│   ├── ttwid.go              # 新增:TTWidManager
│   ├── ttwid_test.go
│   ├── service.go            # 新增:Service struct(替代包级 Extract/ExtractWithConfig)
│   ├── service_test.go       # 替代当前的 extract_test.go
│   ├── extract.go            # 删除(被 service.go 替代)
│   ├── extract_test.go       # 删除
│   ├── resolve.go            # 改:增加 cookie string 参数
│   ├── resolve_test.go       # 改:测试调用传 ""
│   ├── share.go              # 改:增加 cookie string 参数
│   └── share_test.go         # 改:测试调用传 ""
└── ...

cmd/server/main.go            # 改:装配 cache + ttwid + Service
```

## 3. 接口

### 3.1 `internal/cache/cache.go`

```go
package cache

type Cache struct {
    // 内含 sync.Mutex + map[string]*entry + container/list 实现 LRU
}

func New(maxEntries int, ttl time.Duration) *Cache
func (c *Cache) Get(key string) (value string, ok bool)
func (c *Cache) Set(key, value string)
func (c *Cache) Len() int
```

**语义**:
- `Get`:命中且未过期返回 `(value, true)`;未命中或已过期返回 `("", false)`。过期项被惰性删除。
- `Set`:覆盖已有项的值并刷新到队头;新项入队头;超过 `maxEntries` 时驱逐队尾(最久未访问的)。
- 并发安全:所有操作在内部 mutex 内完成。
- 不暴露驱逐回调、不暴露 stats。YAGNI。

### 3.2 `internal/douyin/ttwid.go`

```go
package douyin

const ttwidSourceURL = "https://www.douyin.com/"

type TTWidManager struct {
    // 内含 sync.RWMutex + value string + client *http.Client
}

func NewTTWidManager(client *http.Client) *TTWidManager
func (m *TTWidManager) Refresh() error
func (m *TTWidManager) Cookie() string
```

**语义**:
- `Refresh`:GET `ttwidSourceURL`(带移动端 UA),扫响应的 `Set-Cookie` 头,找到名为 `ttwid` 的 cookie,把值存到内部状态。无 `ttwid` 返回 → 返回 `fmt.Errorf("%w: ttwid cookie not in Set-Cookie", ErrUpstream)`;HTTP 错误同样包 `ErrUpstream`。
- `Cookie`:在读锁下返回当前值。从未 Refresh 过或 Refresh 失败时返回 `""`。
- 调用方负责拼成完整的 cookie header:`"ttwid=" + m.Cookie()`(只在非空时设置)。

### 3.3 `internal/douyin/service.go`

```go
package douyin

type Service struct {
    ttwid *TTWidManager
    cache *cache.Cache
}

func NewService(ttwid *TTWidManager, c *cache.Cache) *Service
func (s *Service) Extract(input string) (Result, error)
func (s *Service) ExtractWithConfig(cfg Config) (Result, error)
```

`Result` 和 `Config` 类型保持当前定义(`Result{PlayURL, VideoID}`,`Config{ShortURL, ShareBaseURL}`)。

### 3.4 修改的现有签名

```go
// 旧
func ResolveVideoID(client *http.Client, shortURL string) (string, error)
// 新
func ResolveVideoID(client *http.Client, cookie, shortURL string) (string, error)

// 旧
func fetchPlayURLFromBase(client *http.Client, baseURL, videoID string) (string, error)
// 新
func fetchPlayURLFromBase(client *http.Client, cookie, baseURL, videoID string) (string, error)
```

`cookie` 为空字符串时不设置 `Cookie:` header(行为与今日完全一致)。非空时设置 `Cookie: ttwid=<value>`。

公开的 `FetchPlayURL` 移除(原本只是 `fetchPlayURLFromBase` 的便捷封装,新代码全走 Service)。

### 3.5 删除的符号

- 包级 `func Extract(input string)`
- 包级 `func ExtractWithConfig(cfg Config)`
- `func FetchPlayURL`(原便捷封装,无人调用)

## 4. 请求流

```
input
  │
  ▼
[1] ExtractShortURL(input) → short_url
  │
  ▼
[2] ResolveVideoID(client, ttwid.Cookie(), short_url) → video_id
  │
  ▼
[3] cache.Get(video_id)
  ├─ HIT  → return Result{PlayURL: cached, VideoID: video_id}, nil  ✓
  └─ MISS → 继续
        │
        ▼
[4] fetchPlayURLFromBase(client, ttwid.Cookie(), shareBase, video_id)
  ├─ ErrNoPlayURL (2002) → ttwid.Refresh()
  │                         ├─ Refresh 失败 → 返回原 ErrNoPlayURL
  │                         └─ Refresh 成功 → 再调一次 fetchPlayURLFromBase(新 cookie)
  │                                            ├─ 仍 ErrNoPlayURL → 返回错误
  │                                            └─ 成功 → 进入 [5]
  └─ 其他成功            → 进入 [5]
        │
        ▼
[5] play_url = StripWatermark(playWM)
[6] cache.Set(video_id, play_url)
[7] return Result{PlayURL: play_url, VideoID: video_id}, nil
```

**关键决策**:

1. **缓存键是 `video_id` 而不是输入文本**:同一视频有多种分享文本(不同时间复制、带不同水印),用 `video_id` 命中率高得多。
2. **缓存只存 `play_url`(strip 后的最终值),不存中间结果**:缓存命中直接返回,不再经 strip。
3. **重试只针对 `ErrNoPlayURL`,且只重试 share 步骤**:`ErrUpstream`(网络/HTTP 错误)和 `ErrInvalidRedirect` 不重试 — 风险/收益不对等。Resolve 已成功的 video_id 不会丢。
4. **重试只做 1 次**:再失败就报错,不无限循环。

## 5. 启动装配(`cmd/server/main.go`)

```go
func main() {
    slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

    ttwid := douyin.NewTTWidManager(httpclient.NewFollowing())
    if err := ttwid.Refresh(); err != nil {
        slog.Error("ttwid refresh failed at startup", "err", err)
        os.Exit(1)
    }
    slog.Info("ttwid acquired", "len", len(ttwid.Cookie()))

    c := cache.New(1000, 30*time.Minute)
    svc := douyin.NewService(ttwid, c)

    handler := httpapi.NewHandler(svc.Extract)
    // ... existing http.Server setup
}
```

**为什么阻塞启动而不是后台拉取**:douyin.com 启动时连不上是真信号 — 不是网络断了就是被 IP 风控了。让进程启动失败比静默运行无 cookie 的退化模式更好。

## 6. 日志

新增字段(在已有的 `parse` 日志行上扩展):

| 字段 | 含义 |
|------|------|
| `cache_hit` | bool — 是否命中缓存 |
| `ttwid_refreshed` | bool — 本次请求是否触发了 ttwid 刷新 |

启动时新增一行 `slog.Info("ttwid acquired", "len", N)`,把 cookie 值长度记下(不记原文)。

## 7. 测试

### 7.1 新增测试

| 模块 | 测试 |
|------|------|
| `cache.Cache` | TTL 过期(注入 clock 或用极短 TTL + sleep);LRU 驱逐(填满后再插入,验证最旧的不见了);并发读写(`go test -race`);Get 未命中;Set 覆盖已有 key 后该 key 升至最近使用 |
| `TTWidManager` | httptest mock 一次带 `Set-Cookie: ttwid=abc123` 的响应,验证 `Cookie()=="abc123"`;mock 无 ttwid 的响应,验证 `Refresh` 返回 `ErrUpstream`;mock 5xx 响应,验证返回 `ErrUpstream` |
| `Service.Extract` 端到端 | 缓存未命中 → 走完整流水线 + 写缓存;缓存命中 → 不打 share 服务器(用 httptest 计数);share 第一次返回空 _ROUTER_DATA → 触发 ttwid Refresh + 第二次返回正常数据 → 成功;share 两次都失败 → 返回 ErrNoPlayURL |

### 7.2 修改的现有测试

- `resolve_test.go` 4 个 subtest:全部传 `""` 作为新的 cookie 参数,断言不变。
- `share_test.go` 4 个 subtest:同上。
- 删除 `extract_test.go`(被 `service_test.go` 替代)。
- `httpapi/handler_test.go` 不变(注入 stub 的接口签名 `func(string) (Result, error)` 不变 — `svc.Extract` 是方法值,绑定到该签名)。

### 7.3 不写

- 真实 douyin.com 的集成测试(同基础 spec,会被风控、CI 不稳)。

## 8. 跨切关注

### 8.1 并发

- `cache.Cache` 用 `sync.Mutex` 保护 map+list;无 lock-free 优化(YAGNI)。
- `TTWidManager` 用 `sync.RWMutex`:`Cookie()` 走读锁(每个请求都调),`Refresh()` 走写锁。`Refresh()` 内部的 HTTP 请求不持锁(避免长时间阻塞读)— 而是先发请求拿到新 cookie,再持写锁更新。
- 多个并发请求同时触发 ttwid Refresh:可以容忍重复刷新(每次最多并发 ~10),不加 singleflight,简单优先。如实测刷新风暴成问题,后续再加。

### 8.2 资源

- 缓存内存上限:1000 条 × 平均 ~250 字节(URL 长度)≈ 256 KB,可忽略。
- 没有 goroutine 泄露:不启后台清理 goroutine,过期项靠 `Get` 时惰性删除。如果一个 key 长期不被访问会留在 map 里直到 LRU 驱逐 — 1000 条上限保证内存有界。

### 8.3 可观测性

- ttwid 永远只记长度不记内容(避免日志泄露)。
- 缓存命中率不主动暴露 metrics(YAGNI;`cache_hit` 字段在每行日志里,用日志聚合工具能算)。

## 9. 已知风险

1. **ttwid 也可能不够**:抖音可能要求 `msToken`、`s_v_web_id`、JS 签名等其他凭证。如果实测仍然 2002,需要加更多 cookie 或退化到方案 B(签名 API)。
2. **缓存过期导致返回失效 URL**:CDN URL 通常签名几小时有效,30min TTL 远低于此 — 风险低。如果用户反馈"播放失败",看日志 `cache_hit=true` 的请求是不是涉及到 30min 前的 video_id。
3. **进程重启清空缓存**:接受,无持久化。

## 10. 验收标准

- [ ] 服务启动时阻塞拉取 ttwid,失败则进程退出码 1。
- [ ] 同一链接连续两次请求:第二次 `cache_hit=true` 且服务端日志的 `duration_ms` 比第一次显著低(预期 <50ms,因为只走 [1][2][3])。
- [ ] 缓存命中时 share 服务器不被请求(端到端测试用 httptest 计数验证)。
- [ ] share 接口连续返回空数据时,ttwid 被刷新且只重试 1 次。
- [ ] `go test ./... -race` 全部通过。
- [ ] `go vet ./...` 无告警。
