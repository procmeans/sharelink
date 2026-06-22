# sharelink

Extract direct video URLs from share text or links.

- **Douyin (抖音)** — watermark-free `.mp4` direct link from share text or `v.douyin.com` short links.
- **YouTube** — from `watch?v=`, `youtu.be/`, and `shorts/` links. By default returns a single **muxed** stream (audio+video in one directly-playable URL, ~360p). Pass `--hq` or `--mp4` for higher quality, which YouTube only ships as **separated** streams (a video-only URL + an audio-only URL you mux yourself).

The right provider is chosen automatically from the link in your input.

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

YouTube — default is a single muxed stream (one playable URL, ~360p, no muxing needed):

```bash
sharelink parse "https://www.youtube.com/watch?v=ElLTyRLLRpg"
# https://rr3---...googlevideo.com/videoplayback?...   (audio+video, ready to play)
```

Higher quality is delivered as separated streams (video URL on line 1, audio URL on line 2):

```bash
sharelink parse --hq  "https://youtu.be/ElLTyRLLRpg"   # best, any codec (VP9/AV1, up to 4K)
sharelink parse --mp4 "https://youtu.be/ElLTyRLLRpg"   # best H.264/AAC (up to 1080p, plays everywhere)
# https://...videoplayback?...   (video-only)
# https://...videoplayback?...   (audio-only)
# mux with: ffmpeg -i video -i audio -c copy out.mp4
```

Or pipe from clipboard:

```bash
pbpaste | sharelink parse
```

JSON output:

```bash
sharelink parse --json "https://v.douyin.com/iABCxxx/"
# {"source":"douyin","play_url":"https://...","video_id":"7234567890123456789"}

sharelink parse --json "https://www.youtube.com/watch?v=ElLTyRLLRpg"
# {"source":"youtube","play_url":"https://...","video_id":"ElLTyRLLRpg","title":"..."}   (default: muxed)

sharelink parse --hq --json "https://www.youtube.com/watch?v=ElLTyRLLRpg"
# {"source":"youtube","video_url":"https://...","audio_url":"https://...","video_id":"ElLTyRLLRpg","title":"..."}
```

`--with-ttwid` (Douyin only) fetches a cookie first (~+500ms) if you hit `2002`-style errors:

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

The body may contain a Douyin or YouTube link; the provider is chosen automatically.

Success response (Douyin):

```json
{ "code": 0, "source": "douyin", "play_url": "https://..." }
```

Success response (YouTube — separated streams):

```json
{ "code": 0, "source": "youtube", "video_url": "https://...", "audio_url": "https://...", "title": "..." }
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
| 1002 | 422  | no supported video link in input (no `v.douyin.com` / YouTube URL) |
| 1003 | 422  | redirect target had no video id (Douyin) |
| 1004 | 405  | method not allowed |
| 2001 | 502  | upstream request failed |
| 2002 | 502  | no extractable stream (Douyin RENDER_DATA missing / YouTube no stream) |

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
