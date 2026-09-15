"""Pull still frames from a CCTV RTSP stream and submit them to the NPDMS API
for face matching against open missing-person reports.

This is for when camera access exists. It needs network access to the camera
(or the video management system's restream) and a platform account of ASI rank
or above created for the control room integration.

    NPDMS_API_URL=http://edge:8080/api/v1 \
    NPDMS_USERNAME=cr.integration NPDMS_PASSWORD=... \
    python -m tools.rtsp_snapshots \
        --camera-id 7f1c...                       # the camera's id in the Phase 03 camera register
        --rtsp rtsp://user:pass@10.0.0.5:554/stream1 \
        --every 5 \
        --purpose "Matching open missing-person reports under order KP/FR/2026/01"

What it does and does not do:
  * Sends one JPEG every --every seconds to POST /face-recognition/camera-snapshots.
    The API refuses unless face recognition is switched on under an active
    authorisation, and records every submission with its purpose.
  * Keeps nothing locally. Frames with no candidate are not stored by the API
    either (only their SHA-256 is recorded).
  * Does not decide anything: every hit is a PENDING candidate for an officer.
  * Reconnects with backoff when the stream drops. Stream credentials stay on
    the machine running this; they are never sent to the API.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timezone

import cv2


class Api:
    def __init__(self, base: str, username: str, password: str, token: str = ""):
        self.base = base.rstrip("/")
        self.username, self.password = username, password
        self.token = token

    def login(self) -> None:
        body = json.dumps({"username": self.username, "password": self.password}).encode()
        req = urllib.request.Request(self.base + "/auth/login", data=body, headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(req, timeout=20) as resp:
            self.token = json.load(resp)["accessToken"]

    def submit(self, camera_id: str, captured_at: datetime, jpeg: bytes, purpose: str) -> dict:
        boundary = uuid.uuid4().hex
        parts = []
        for name, value in (("cameraId", camera_id), ("capturedAt", captured_at.isoformat()), ("purpose", purpose)):
            parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode())
        parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="snapshot.jpg"\r\n'
                     f"Content-Type: image/jpeg\r\n\r\n".encode() + jpeg + b"\r\n")
        parts.append(f"--{boundary}--\r\n".encode())
        data = b"".join(parts)
        for attempt in (1, 2):
            if not self.token:
                self.login()
            req = urllib.request.Request(self.base + "/face-recognition/camera-snapshots", data=data, headers={
                "Content-Type": f"multipart/form-data; boundary={boundary}", "Authorization": f"Bearer {self.token}"})
            try:
                with urllib.request.urlopen(req, timeout=120) as resp:
                    return json.load(resp)
            except urllib.error.HTTPError as exc:
                if exc.code == 401 and attempt == 1 and self.username:
                    self.token = ""
                    continue
                raise
        return {}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--camera-id", required=True)
    ap.add_argument("--rtsp", required=True)
    ap.add_argument("--every", type=float, default=5.0, help="seconds between submitted frames (minimum 1)")
    ap.add_argument("--purpose", required=True)
    ap.add_argument("--max-frames", type=int, default=0, help="stop after this many submissions (0 = run until stopped)")
    args = ap.parse_args()
    if args.every < 1:
        ap.error("--every must be at least 1 second")
    if len(args.purpose.strip()) < 10:
        ap.error("--purpose must say why, in at least 10 characters")

    api = Api(os.environ.get("NPDMS_API_URL", "http://localhost:8080/api/v1"),
              os.environ.get("NPDMS_USERNAME", ""), os.environ.get("NPDMS_PASSWORD", ""), os.environ.get("NPDMS_API_TOKEN", ""))
    os.environ.setdefault("OPENCV_FFMPEG_CAPTURE_OPTIONS", "rtsp_transport;tcp")

    sent = 0
    backoff = 2.0
    while True:
        cap = cv2.VideoCapture(args.rtsp, cv2.CAP_FFMPEG)
        if not cap.isOpened():
            print(f"stream not reachable; retrying in {backoff:.0f}s", file=sys.stderr)
            time.sleep(backoff)
            backoff = min(backoff * 2, 120)
            continue
        backoff = 2.0
        last = 0.0
        while True:
            ok = cap.grab()  # keep draining so the frame we send is current
            if not ok:
                print("stream dropped; reconnecting", file=sys.stderr)
                break
            now = time.monotonic()
            if now - last < args.every:
                continue
            ok, frame = cap.retrieve()
            if not ok:
                continue
            last = now
            ok, buf = cv2.imencode(".jpg", frame, [cv2.IMWRITE_JPEG_QUALITY, 92])
            if not ok:
                continue
            try:
                result = api.submit(args.camera_id, datetime.now(timezone.utc), buf.tobytes(), args.purpose)
                print(json.dumps({"at": datetime.now(timezone.utc).isoformat(),
                                  "candidates": result.get("candidatesCreated"), "faces": result.get("facesSeen")}))
            except urllib.error.HTTPError as exc:
                detail = exc.read().decode(errors="replace")[:300]
                print(f"API refused the snapshot ({exc.code}): {detail}", file=sys.stderr)
                if exc.code in (403, 409, 423):  # switched off, not authorised: stop rather than hammer the API
                    return 2
            except urllib.error.URLError as exc:
                print(f"API unreachable: {exc.reason}", file=sys.stderr)
            sent += 1
            if args.max_frames and sent >= args.max_frames:
                cap.release()
                return 0
        cap.release()


if __name__ == "__main__":
    raise SystemExit(main())
