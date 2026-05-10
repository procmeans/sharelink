# 抖音去水印直链解析服务 — 设计文档

- 日期:2026-05-10
- 状态:已批准,待实现
- 作者:procmeans (with Claude)

## 1. 目标与范围

构建一个 HTTP 服务,接收抖音(Douyin)分享文本或短链接,返回该作品去水印后的 mp4 直链(指向抖音 CDN 的原始地址)。

**范围内**:
- 单视频作品的去水印 URL 提取。
- 输入支持:APP 复制的完整分享文本(含 emoji、话题、链接)、纯 `https://v.douyin.com/xxx` 短链。
- HTTP API 服务形态,Go 语言实现,标准库为主。
- 个人 / 内网使用,无认证、无限流。

**范围外(不做)**:
- 图文作品、音频提取、用户主页作品列表。
- 视频文件的服务端代理下载(只返回 URL,不返回字节流)。
- 多清晰度选择、互动数据、作者信息等元数据。
- 认证、API Key、IP 限流、缓存。

## 2. 技术选型

| 维度 | 选择 | 理由 |
|------|------|------|
| 语言 | Go | 用户指定 |
| 框架 | 标准库 `net/http` | 服务体量小,标准库够用,避免引入框架带来的版本/学习成本 |
| 解析方式 | `iesdouyin.com/share/video/{id}/` 页面 + RENDER_DATA 正则 + JSON 解析 | 不需要 X-Bogus/a_bogus 签名,实现简单,社区开源工具普遍采用 |
| 依赖 | 仅标准库(`net/http`、`encoding/json`、`regexp`、`net/url`、`log/slog`) | 控制依赖面,降低长期维护成本 |

**已评估并否决的方案**:
- **Web `aweme/detail/` API + X-Bogus 签名**:返回数据更完整,但签名算法频繁变化,需要内嵌 JS 引擎执行抖音混淆代码,维护成本高。
- **第三方 SaaS 解析服务(如 tikhub)**:依赖外部、可能收费,与"自己写工具"的目的相悖。

## 3. 架构与模块

```
sharelink/
├── cmd/server/main.go        # 入口:绑定端口、注册路由、启动 server
├── internal/
│   ├── httpapi/              # HTTP 层
│   │   └── handler.go        # 路由 + handler + 请求/响应模型 + 错误映射
│   ├── douyin/               # 业务核心(纯逻辑,不依赖 HTTP server)
│   │   ├── extract.go        # 对外:Extract(input string) (string, error)
│   │   ├── shorturl.go       # 从分享文本里提取 v.douyin.com 短链
│   │   ├── resolve.go        # 跟随 302,从落地 URL 提取 video_id
│   │   ├── share.go          # 请求 share 页 + 解析 RENDER_DATA
│   │   ├── nowm.go           # playwm → play 替换
│   │   └── errors.go         # sentinel errors
│   └── httpclient/           # 共享 HTTP client(超时、UA 配置)
│       └── client.go
├── docs/superpowers/specs/   # 本文档所在
└── go.mod
```

**职责边界**:
- `douyin` 包是纯业务逻辑,只依赖 `httpclient`,不知道 HTTP server 的存在 — 方便单元测试,也方便以后改成 CLI 复用。
- `httpapi` 只做参数校验、调用 `douyin.Extract`、组装 JSON 响应、把业务错误映射到 HTTP 状态码 / 业务 code。**不写任何抖音相关逻辑**。
- `httpclient` 单独成包,因为 `resolve` 阶段需要"禁止自动跟随 302",而 `share` 阶段需要正常跟随 — 两个 client 配置不同,集中管理避免散落。

## 4. 解析流程

`douyin.Extract(input string) (playURL string, err error)` 的内部步骤:

```
input (string)
  │
  ▼
[1] shorturl.Extract(input)
  │   正则:https?://v\.douyin\.com/[A-Za-z0-9]+
  │   抓不到 → ErrNoShortURL
  ▼
short_url (https://v.douyin.com/xxx)
  │
  ▼
[2] resolve.FollowRedirect(short_url)
  │   GET 短链,client.CheckRedirect 配置为返回 http.ErrUseLastResponse(禁止自动跟随)
  │   读取响应 Location header(应为 301/302)
  │   形如:https://www.iesdouyin.com/share/video/{video_id}/?...
  │   或:  https://www.douyin.com/video/{video_id}
  │   正则:/(?:share/video|video)/(\d+)/? 提取 video_id
  │   提不到 → ErrInvalidRedirect
  │   网络/状态码异常 → ErrUpstream
  ▼
video_id (e.g. 7234567890123456789)
  │
  ▼
[3] share.FetchDetail(video_id)
  │   GET https://www.iesdouyin.com/share/video/{video_id}/
  │     Header: User-Agent = 移动端 iPhone Safari UA
  │     (允许跟随 302)
  │   正则:<script id="RENDER_DATA"[^>]*>([^<]+)</script> 抓内容
  │   url.QueryUnescape 解码内层 URL-encoded JSON
  │   json.Unmarshal 到最小化 struct(只挑需要的字段路径)
  │   取 aweme_detail.video.play_addr.url_list[0]
  │   字段缺失/为空 → ErrNoPlayURL
  │   网络/状态码异常 → ErrUpstream
  ▼
play_url_with_wm (URL 中包含 "playwm")
  │
  ▼
[4] nowm.Strip(play_url_with_wm)
  │   strings.Replace(url, "playwm", "play", 1)
  ▼
play_url (无水印 mp4 直链) → 返回
```

**关键决策**:

1. **第 [3] 步只反序列化最小 struct**,而不是把整个 RENDER_DATA 全解。RENDER_DATA 字段巨多还经常变,只挑 `video.play_addr.url_list` 这一条路径,稳定性高得多。
2. **不缓存**。瓶颈是抖音侧速率,不是 CPU;加缓存收益小且让 cookie/UA 调试更难。如果实际有重复请求需求再加。
3. **不重试**。HTTP 失败、JSON 字段缺失都直接返回错误。重试可能让抖音风控加码,反而更糟。

## 5. HTTP 接口

### 5.1 `POST /api/parse`

**请求**:
```http
POST /api/parse
Content-Type: application/json

{
  "url": "5.99 fyu:/ 复制打开抖音 https://v.douyin.com/iABCxxx/ ..."
}
```

**成功响应 (200)**:
```json
{
  "code": 0,
  "play_url": "https://v3-web.douyinvod.com/.../play/...mp4"
}
```

**失败响应 (4xx / 5xx)**:
```json
{
  "code": 1002,
  "message": "no douyin short url found in input"
}
```

### 5.2 为什么 POST 而不是 GET

输入是用户从 APP 复制的分享文本,常含 emoji、空格、`#话题`,放 query string 容易被 URL 编码搞乱;放 JSON body 干净可靠。

### 5.3 错误码

| code | HTTP | 触发场景 |
|------|------|----------|
| 0    | 200  | 成功 |
| 1001 | 400  | 请求体 JSON 解析失败 / `url` 字段缺失 |
| 1002 | 422  | 文本中找不到 `v.douyin.com` 短链 |
| 1003 | 422  | 短链 302 后拿不到合法的 `video_id` |
| 2001 | 502  | 上游(抖音)请求失败、超时、状态码非 2xx/3xx |
| 2002 | 502  | RENDER_DATA 缺失 / 解析失败 / `play_addr` 为空 |

`douyin` 包返回 sentinel error(`ErrNoShortURL`、`ErrInvalidRedirect`、`ErrUpstream`、`ErrNoPlayURL`),`httpapi` 用 `errors.Is` 映射到上述 code。业务包不依赖 HTTP 概念。

### 5.4 健康检查

`GET /healthz` 返回 `200 ok`。无业务逻辑,仅用于容器/反向代理探活。

## 6. 跨切关注

### 6.1 超时

- HTTP client(对抖音请求):8s
- HTTP server read/write timeout:15s
- 避免被抖音慢响应吊死整个进程。

### 6.2 日志

使用标准库 `log/slog`,JSON 输出。每个请求记一行:

```
time | input_truncated(100字符) | video_id | result(ok/error_code) | duration_ms
```

便于事后排查解析失效(尤其当 share 页 HTML 结构变更导致系统性失败时)。

### 6.3 配置

- 监听端口:环境变量 `PORT`,默认 `8080`。
- 不引入配置文件,保持简单。

### 6.4 User-Agent

固定一个移动端 iPhone Safari UA。`iesdouyin.com` 对 PC UA 返回内容不同,移动端 UA 更稳定。UA 字符串集中在 `httpclient` 包。

## 7. 测试策略

| 模块 | 测试方式 |
|------|----------|
| `shorturl` | Table-driven 单元测试(纯字符串处理) |
| `nowm` | Table-driven 单元测试 |
| `share` | 把真实 share 页 HTML 落到 `testdata/sample_share.html`,测试读文件解析,**不发真实请求** |
| `resolve` | `httptest.NewServer` mock 302 响应 |
| `Extract` 端到端 | `httptest.NewServer` mock 抖音的 302 和 share 页,验证整条流水线 |
| `httpapi` handler | 用 `httptest.NewRecorder` 测试请求/响应 + 错误码映射 |

**不写**对真实抖音的集成测试 — 会被风控、CI 不稳定。提供一个 `cmd/probe` 小工具,手动跑真实链接做冒烟验证。

## 8. 已知风险

1. **share 页 HTML 结构变更**:`<script id="RENDER_DATA">` 的位置或字段路径若调整,需要更新 `share.go`。日志中的系统性 `2002` 错误是首要信号。
   - **2026-05-10 实测**:share 页结构已变更,RENDER_DATA 不再存在;已切换到解析 `window._ROUTER_DATA` 内联 JSON,路径 `loaderData["video_(id)/page"].videoInfoRes.item_list[0].video.play_addr.url_list[0]`。`playwm` 子串仍在,去水印逻辑不变。
2. **抖音对 IP 风控**:个人/内网使用预计影响小,但若部署到公网且被高频调用,可能被封。此时需要退化到方案 B(签名 API)或加代理池 — **超出当前 spec 范围**。
3. **RENDER_DATA 偶尔需要 cookie**:目前不带 cookie 也能拿到,如果失效可考虑请求一次首页拿 `ttwid` 等基础 cookie。

## 9. 验收标准

- [ ] `POST /api/parse` 接收任意一条真实分享文本,返回的 `play_url` 用 `curl` 或浏览器能直接下载到无水印 mp4。
- [ ] 错误输入(空字符串、非抖音链接、无效 video_id)返回对应错误码,不 panic、不 5xx 兜底。
- [ ] `go test ./...` 全部通过,且不依赖外网。
- [ ] `go vet ./...` 无告警。
