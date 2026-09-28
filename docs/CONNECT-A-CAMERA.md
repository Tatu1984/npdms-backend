# Connect a camera — live CCTV through the Edge Agent

The same design as the Live Feed Portal and KMCP:

```
CCTV camera ──RTSP──▶ Edge Agent ──HTTPS PUT (outbound only)──▶ API /api/edge/ingest/<Camera ID>/<file> ──▶ Cloudflare R2
  camera LAN          site PC / mini-PC                                   │
                                     officer ──purpose-logged session──▶ GET the same path ──▶ browser (hls.js)
```

## 1. In NPDMS (SHO or above)

1. **Video Intelligence → Register camera**, keep **Enable live streaming through the Edge Agent** ticked — or, for a camera already registered, open it and choose **Enable live streaming**.
2. The screen shows, **once**:
   - **Ingest URL** — the API origin, e.g. `https://npdms-api.example.in`
   - **Token** — `ing_…`. Only its SHA-256 is stored; it cannot be shown again.
   - **Camera ID** — the camera's unguessable ingest key.
   - (Publish URL — what the agent builds from the two above: `<Ingest URL>/api/edge/ingest/<Camera ID>/index.m3u8`.)
3. Lost the token? **Rotate token** issues a new one and revokes the old immediately. **Turn off live streaming** revokes it, ends viewing sessions and deletes the stored feed.

## 1b. A DVR, rather than one camera

A recorder is one box with many channels, and each channel is a camera in the
register with its own ingest key. Registering sixteen of those by hand means
repeating the same form sixteen times and copying a token that is shown once,
so there is a script:

```
DVR_PASSWORD=... NPDMS_TOKEN=<an SHO's token> \
python3 scripts/register-dvr-channels.py \
    --api https://npdms-api.example.in/api/v1 \
    --host 192.168.100.64 --brand dahua --channels 1-16 \
    --username npdms-ro --station-code BHW --prefix GARIAHAT \
    --latitude 22.5186 --longitude 88.3665 \
    --out ~/edge-agent-config.yaml
```

It registers each channel, turns streaming on, and writes the agent's
`config.yaml` with the tokens already in it (mode 600 — that file holds the
ingest tokens and the DVR password, and belongs in no repository). `--dry-run`
shows what it would register without writing anything. Give `--name` and
`--location` once per channel, in order, for the ones you can name; the rest
fall back to "<prefix> channel N".

How each maker spells a channel:

| | Main stream | Sub stream |
|---|---|---|
| Dahua (XVR/NVR, e.g. DH-XVR4816) | `/cam/realmonitor?channel=N&subtype=0` | `subtype=1` |
| Hikvision | `/Streaming/Channels/N01` | `/Streaming/Channels/N02` |

Three things about a recorder that decide whether this works:

- **Set the channels to H.264, not H.265.** The agent copies an H.264 stream
  straight through; H.265 has to be transcoded for browsers, which is the
  largest CPU cost in the whole chain and the thing that limits how many
  channels one site machine can carry.
- **Plates need the main stream.** `--sub-stream` is cheaper and fine for
  watching, but a plate wants roughly 150px of width, which a sub stream
  rarely gives. Face matching is happier on the sub stream.
- **A recorder allows only so many simultaneous RTSP pulls.** The agent takes
  one per channel and the portal fans out from stored video, so watchers cost
  the DVR nothing — but anything else pulling from it (a spare analytics
  process, someone's phone app) competes for the same slots. Give the platform
  its own read-only account so you can see what it is using.

## 2. On the site machine — the Edge Agent

Download: `Smart-Parking/edge-agent`, branch **`live-feed-only`** (the multi-camera agent with Portal Connection; `main` still carries the older single-camera agent). Build with `make build` (headless worker), `make macos` / `make windows` (desktop app). ffmpeg and ffprobe must be installed.

- **Portal Connection → Ingest URL**: paste the Ingest URL.
- **Add camera**: Camera ID = the Camera ID; Stream key = the same Camera ID; Ingest token = the Token; RTSP = the camera's local address (`rtsp://user:pass@192.168.1.100:554/…`). Leave the Portal Connection fallback token empty — each camera has its own.
- Start. Within a few seconds the camera reads **Live** in NPDMS.

Headless `config.yaml` equivalent:

```yaml
schemaVersion: 1
portalBaseUrl: https://npdms-api.example.in
cameras:
  - cameraId: <Camera ID>
    streamKey: <Camera ID>
    token: <Token>
    rtsp: rtsp://user:pass@192.168.1.100:554/Streaming/Channels/101
    enabled: true
```

**Network:** outbound HTTPS to the API only. No inbound port, public IP, port forwarding, VPN, UDP or TURN. Works behind NAT/CGNAT. Camera passwords never leave the site.

## 3. What officers see

| Status | Meaning |
|---|---|
| Live | Segments are arriving |
| Waiting for Edge Agent | The agent wrote a playlist, no video yet |
| Stream stopped | The playlist ended, or was not rewritten for 30 s (`LIVE_STALE_SECONDS`) — the agent runs ffmpeg with `omit_endlist`, so a dead agent never says it ended |
| Offline | Nothing stored, or storage unreachable |

**Privacy (Phase 03 rules apply to live viewing).** Nothing plays until the officer states a purpose (≥ 10 characters). That opens a live viewing session over the chosen cameras, recorded **once** in the append-only video purpose log (`VIEW_LIVE`, with the cameras) and the audit trail — not once per segment. Watching needs **ASI** or above; a camera flagged for masking needs **SHO** or above, because masking is not applied to live video. A session expires 15 min after playback stops (`LIVE_SESSION_IDLE_SECONDS`) and ends after 8 h at most (`LIVE_SESSION_MAX_SECONDS`). DSPs read the purpose log.

## 4. Cloudflare R2 and Vercel

One-time R2 setup: Cloudflare → R2 → Create bucket (e.g. `npdms-live`) → Manage R2 API Tokens → **Object Read & Write** for that bucket → note Account ID, Access Key ID and Secret. No public bucket is needed: playback streams through the API so it stays purpose-gated. Add a lifecycle rule deleting `hls/` objects older than 1 day to collect anything a purge missed.

Vercel (API project) environment variables:

| Variable | Value |
|---|---|
| `MEDIA_BACKEND` | `r2` |
| `R2_ACCOUNT_ID` | Cloudflare account id |
| `R2_ACCESS_KEY_ID` | R2 token access key |
| `R2_SECRET_ACCESS_KEY` | R2 token secret |
| `R2_BUCKET` | e.g. `npdms-live` |
| `R2_PREFIX` | optional, default `hls` |
| `PUBLIC_API_URL` | the API origin shown as the Ingest URL, e.g. `https://npdms-api.vercel.app` |
| `R2_TIMEOUT_MS` | optional, default `4000` (uploads get twice that) |

Also accepted: `S3_ENDPOINT`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_BUCKET`, `S3_REGION` (KMCP names), `R2_PUBLIC_BASE`/`HLS_PUBLIC_BASE` (read through a CDN, still gated), `MEDIA_BACKEND=minio` with the `MINIO_*` settings, `MEDIA_BACKEND=fs` + `MEDIA_FS_DIR` for a disk-backed edge server.

Refusals, by design, each with a message naming what to set: `fs` on Vercel; `database` for live video (explicit, or inherited from `STORAGE_BACKEND=database`) — a camera writes ~30 000 objects a day. Evidence keeps its own `STORAGE_BACKEND`. Without a usable store the API still runs: ingest answers 503 with the reason and every camera reads Offline.

**Limits.** A Vercel function accepts request bodies up to about 4.5 MB; a 2-second segment from a typical 1080p H.264 camera (2–4 Mbit/s) is 0.5–1 MB. The API's own ingest cap is 20 MB (`INGEST_MAX_BYTES`). Ingest and playback are exempt from the global rate limiter (a camera makes ~60 requests a minute; each viewer tile ~1 a second plus a CORS preflight).
