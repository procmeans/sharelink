# sharelink

Extract watermark-free direct video URLs from Douyin (抖音) share text or short links.

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

Error response:

```json
{ "code": 2001, "message": "douyin: upstream request failed: ..." }
```

#### Error codes

| code | HTTP | meaning |
|------|------|---------|
| 0    | 200  | success |
| 1001 | 400  | bad request body |
| 1002 | 422  | no v.douyin.com short URL in input |
| 1003 | 422  | redirect target had no video id |
| 1004 | 405  | method not allowed |
| 2001 | 502  | upstream request failed |
| 2002 | 502  | RENDER_DATA missing or play URL empty |

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

## Build and test

```bash
go test ./... -race
go vet ./...
go build -o sharelink ./cmd/sharelink
```

All tests are offline; no real Douyin requests.

## Release (maintainer)

A push of a `v*.*.*` tag triggers `.github/workflows/release.yml`, which runs goreleaser to build cross-platform binaries and publishes a brew formula to `procmeans/homebrew-tap`. See `docs/superpowers/specs/2026-05-19-cli-and-brew-design.md` §6.3 for the one-time setup (creating the tap repo and `TAP_GITHUB_TOKEN` secret).

```bash
git tag v0.1.0
git push origin v0.1.0
```
