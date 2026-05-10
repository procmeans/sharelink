# sharelink

A small Go HTTP service that takes a Douyin (抖音) share text or short link and returns the watermark-free mp4 CDN URL.

## Run

```bash
go build -o server ./cmd/server
PORT=8080 ./server
```

## API

### `POST /api/parse`

Request:

```json
{ "url": "5.99 fyu:/ 复制打开抖音... https://v.douyin.com/iABCxyz/ ..." }
```

Success response:

```json
{ "code": 0, "play_url": "https://v3-web.douyinvod.com/.../play/...mp4" }
```

Error response:

```json
{ "code": 2001, "message": "douyin: upstream request failed: ..." }
```

### Error codes

| code | HTTP | meaning |
|------|------|---------|
| 0    | 200  | success |
| 1001 | 400  | bad request body |
| 1002 | 422  | no v.douyin.com short URL in input |
| 1003 | 422  | redirect target had no video id |
| 1004 | 405  | method not allowed |
| 2001 | 502  | upstream request failed |
| 2002 | 502  | RENDER_DATA missing or play URL empty |

### `GET /healthz`

Returns `200 ok`. For health checks.

## Test

```bash
go test ./...
```

All tests are offline; no real Douyin requests.

## Operational notes

- On startup the server fetches a `ttwid` cookie from `https://www.douyin.com/`. Failure is fatal — the process exits with status 1.
- Successful parses are cached in-process by `video_id` for 30 minutes (LRU, max 1000 entries). Cache survives only the lifetime of the process.
- If the share-page parser returns "no play URL" the server refreshes ttwid and retries the share fetch exactly once before giving up.
- Each request log includes `cache_hit` and `ttwid_refreshed` fields. A wave of `ttwid_refreshed=true` followed by `code:2002` is the signal that ttwid alone is no longer sufficient.
