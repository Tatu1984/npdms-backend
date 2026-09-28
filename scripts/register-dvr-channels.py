#!/usr/bin/env python3
"""Register a DVR's channels as cameras and write the Edge Agent's config.

A DVR is one box with many channels, and each channel is a camera in the Phase
03 register with its own ingest key. Doing that by hand means repeating the
same form once per channel and copying a token that is shown exactly once; on
a sixteen-channel recorder that is an afternoon and a transcription error.

    DVR_PASSWORD=... NPDMS_TOKEN=... python3 scripts/register-dvr-channels.py \
        --api https://api-black-pi.vercel.app/api/v1 \
        --host 192.168.100.64 --brand dahua --channels 1-4 \
        --station-code BHW --username admin \
        --prefix GARIAHAT --out ~/edge-agent-config.yaml

Writes config.yaml for the Edge Agent (chmod 600 — it holds the ingest tokens)
and prints a table of what was registered. The DVR password is read from the
environment, never from the command line, so it does not appear in the process
list or the shell history.

Channel naming and placement are per site, so --name and --location may be
given once per channel in order; anything not named falls back to
"<prefix> channel N".
"""
import argparse, json, os, sys, urllib.request, urllib.error

# How each maker spells a channel in an RTSP path.
#   Dahua   (XVR/NVR):  /cam/realmonitor?channel=N&subtype=0   0 = main, 1 = sub
#   Hikvision:          /Streaming/Channels/N01                 N01 main, N02 sub
BRANDS = {
    "dahua":     lambda ch, sub: f"/cam/realmonitor?channel={ch}&subtype={1 if sub else 0}",
    "hikvision": lambda ch, sub: f"/Streaming/Channels/{ch}{'02' if sub else '01'}",
}


def call(api, method, path, token, body=None):
    req = urllib.request.Request(api + path, method=method)
    req.add_header("Authorization", "Bearer " + token)
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, data, timeout=60) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, {"message": raw.decode("utf-8", "replace")[:300]}


def parse_channels(spec):
    out = []
    for part in spec.split(","):
        part = part.strip()
        if "-" in part:
            a, b = part.split("-", 1)
            out.extend(range(int(a), int(b) + 1))
        elif part:
            out.append(int(part))
    return out


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--api", required=True, help="API base, ending /api/v1")
    p.add_argument("--host", required=True, help="the DVR's address on the site LAN")
    p.add_argument("--port", type=int, default=554)
    p.add_argument("--brand", choices=sorted(BRANDS), default="dahua")
    p.add_argument("--channels", required=True, help="e.g. 1-16 or 1,2,5")
    p.add_argument("--username", required=True, help="a read-only account on the DVR")
    p.add_argument("--station-code", required=True, help="station the cameras belong to")
    p.add_argument("--prefix", required=True, help="code prefix, e.g. GARIAHAT")
    p.add_argument("--owner-agency", default="KP", choices=["KP", "KMC", "TRAFFIC", "PRIVATE", "OTHER"])
    p.add_argument("--retention", default="STANDARD", choices=["SHORT", "STANDARD", "EXTENDED"])
    p.add_argument("--sub-stream", action="store_true",
                   help="publish the low-resolution stream. Cheaper, but plates need the main stream")
    p.add_argument("--name", action="append", default=[], help="per channel, in order")
    p.add_argument("--location", action="append", default=[], help="per channel, in order")
    p.add_argument("--latitude", type=float)
    p.add_argument("--longitude", type=float)
    p.add_argument("--out", required=True, help="where to write the Edge Agent config")
    p.add_argument("--dry-run", action="store_true")
    args = p.parse_args()

    api_token = os.environ.get("NPDMS_TOKEN")
    dvr_password = os.environ.get("DVR_PASSWORD")
    if not api_token:
        sys.exit("NPDMS_TOKEN is not set (an officer of SHO rank or above)")
    if not dvr_password and not args.dry_run:
        sys.exit("DVR_PASSWORD is not set")

    channels = parse_channels(args.channels)
    path_of = BRANDS[args.brand]
    registered, failed = [], []

    for i, ch in enumerate(channels):
        name = args.name[i] if i < len(args.name) else f"{args.prefix.title()} channel {ch}"
        location = args.location[i] if i < len(args.location) else name
        body = {
            "code": f"{args.prefix}-CH{ch:02d}",
            "name": name,
            "location": location,
            "ownerAgency": args.owner_agency,
            "streamType": "RTSP",
            "streamHost": args.host,
            "streamPort": args.port,
            "streamPath": path_of(ch, args.sub_stream),
            "retentionClass": args.retention,
            "credentialUsername": args.username,
            "credentialSecret": dvr_password,
            "enableStreaming": True,
        }
        if args.latitude is not None and args.longitude is not None:
            body["latitude"], body["longitude"] = args.latitude, args.longitude

        if args.dry_run:
            shown = dict(body, credentialSecret="***")
            print(f"would register {shown['code']}: {shown['streamPath']}")
            continue

        status, resp = call(args.api, "POST", "/video/cameras", api_token, body)
        if status not in (200, 201):
            failed.append((body["code"], status, resp.get("message", resp)))
            continue
        agent = resp.get("edgeAgent") or {}
        if not agent.get("ingestToken"):
            failed.append((body["code"], status, "registered but no Edge Agent settings returned"))
            continue
        registered.append({
            "code": body["code"], "name": name, "channel": ch,
            "cameraId": agent["cameraId"], "token": agent["ingestToken"],
            "ingestUrl": agent["ingestUrl"], "rtspPath": body["streamPath"],
        })

    if args.dry_run:
        return

    if registered:
        out = os.path.expanduser(args.out)
        base = registered[0]["ingestUrl"]
        lines = ["schemaVersion: 1", f"portalBaseUrl: {base}", "cameras:"]
        for r in registered:
            rtsp = f"rtsp://{args.username}:{dvr_password}@{args.host}:{args.port}{r['rtspPath']}"
            lines += [f"  # {r['code']} — {r['name']}",
                      f"  - cameraId: {r['cameraId']}",
                      f"    streamKey: {r['cameraId']}",
                      f"    token: {r['token']}",
                      f"    rtsp: {rtsp}",
                      f"    enabled: true"]
        fd = os.open(out, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w") as f:
            f.write("\n".join(lines) + "\n")
        print(f"\nEdge Agent config written to {out} (chmod 600 — it holds the ingest tokens")
        print("and the DVR password; it never belongs in a repository).\n")
        print(f"{'CODE':<16} {'CH':>3}  NAME")
        for r in registered:
            print(f"{r['code']:<16} {r['channel']:>3}  {r['name']}")
        print(f"\n{len(registered)} channel(s) registered and streaming enabled.")

    if failed:
        print(f"\n{len(failed)} channel(s) not registered:")
        for code, status, msg in failed:
            print(f"  {code}: {status} {msg}")
        sys.exit(1)


if __name__ == "__main__":
    main()
