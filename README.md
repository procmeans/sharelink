# sharelink

Extract watermark-free direct video URLs from Douyin (抖音) and TikTok share text or links.

## Install

```bash
brew tap procmeans/tap
brew install sharelink
```

Or build from source:

```bash
go install github.com/procmeans/sharelink/cmd/sharelink@latest
```

## CLI

### Parse a share link

```bash
sharelink parse "5.99 fyu:/ 复制打开抖音 https://v.douyin.com/iABCxxx/ ..."
# https://aweme.snssdk.com/aweme/v1/play/?video_id=...
```

Or pipe from clipboard:

```bash
pbpaste | sharelink parse
```

JSON output:

```bash
sharelink parse --json "https://v.douyin.com/iABCxxx/"
# {"play_url":"https://...","video_id":"7234567890123456789"}
```

TikTok video links (`tiktok.com/@user/video/<id>`) and share short links (`vm.tiktok.com`, `vt.tiktok.com`, `tiktok.com/t/...`) work the same way:

```bash
sharelink parse -o video.mp4 "https://www.tiktok.com/@user/video/7560340173955878155"
```

TikTok's CDN returns 403 unless the request carries the `tt_chain_token` cookie issued with the page plus a `Referer: https://www.tiktok.com/` header, so the bare TikTok URL will **not** open in a browser. Use `-o <file>` to download it directly, or `--json`, which adds a `headers` object to send yourself:

```bash
sharelink parse --json "https://www.tiktok.com/@user/video/7560340173955878155"
# {"play_url":"https://v16-webapp-prime...","video_id":"7560...","headers":{"Cookie":"tt_chain_token=...","Referer":"https://www.tiktok.com/"}}
```

The TikTok URL and token expire after about 48 hours.

If you hit `2002`-style errors, try `--with-ttwid` to fetch a cookie first (~+500ms):

```bash
sharelink parse --with-ttwid "https://v.douyin.com/iABCxxx/"
```

Exit codes: `0` success, `1` parse/upstream error, `2` usage error.

### Run as HTTP service

```bash
sharelink serve              # listens on $PORT or :8080
sharelink serve --port 9090
```

#### `POST /api/parse`

```json
{ "url": "5.99 fyu:/ ... https://v.douyin.com/iABCxyz/ ..." }
```

Success response:

```json
{ "code": 0, "play_url": "https://..." }
```

For TikTok links the response also includes `"headers": { "Cookie": "...", "Referer": "..." }`, which must be sent when fetching `play_url` (see above).

Error response:

```json
{ "code": 2001, "message": "douyin: upstream request failed: ..." }
```

#### Error codes

| code | HTTP | meaning |
|------|------|---------|
| 0    | 200  | success |
| 1001 | 400  | bad request body |
| 1002 | 422  | no Douyin short URL or TikTok link in input |
| 1003 | 422  | redirect target had no video id |
| 1004 | 405  | method not allowed |
| 2001 | 502  | upstream request failed (TikTok: also missing `tt_chain_token`) |
| 2002 | 502  | page data missing or play URL empty (TikTok: also private/deleted video) |

#### `GET /healthz`

Returns `200 ok`. For health checks.

### Version

```bash
sharelink version
# sharelink v0.1.0 (commit abc1234, built 2026-05-19T...)
```

## Operational notes (serve mode)

- On startup the server fetches a `ttwid` cookie from `https://www.douyin.com/`. Failure is fatal.
- Successful parses are cached in-process by `video_id` for 30 minutes (LRU, max 1000 entries).
- If the share-page parser returns "no play URL" the server refreshes ttwid and retries once.
- Each request log includes `cache_hit` and `ttwid_refreshed` fields.
- TikTok requests are not cached and do not use ttwid; each one fetches the video page (~400 KB).

## Build and test

```bash
go test ./... -race
go vet ./...
go build -o sharelink ./cmd/sharelink
```

All tests are offline; no real Douyin or TikTok requests.

## Release (maintainer)

A push of a `v*.*.*` tag triggers `.github/workflows/release.yml`, which runs goreleaser to build cross-platform binaries and publishes a brew formula to `procmeans/homebrew-tap`. See `docs/superpowers/specs/2026-05-19-cli-and-brew-design.md` §6.3 for the one-time setup (creating the tap repo and `TAP_GITHUB_TOKEN` secret).

```bash
git tag v0.1.0
git push origin v0.1.0
```
