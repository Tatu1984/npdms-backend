"""Build a synthetic test clip for local verification.

    python -m tools.make_test_clip --face synthetic_a.jpg --others synthetic_b.jpg synthetic_c.jpg --out clip.mp4

Only for synthetic faces (see MODELS.md). The clip is a dark "street" frame
with faces pasted at CCTV-like sizes: the target face appears for a few
seconds in the middle, other synthetic identities before and after. Never
commit or seed the output.
"""

from __future__ import annotations

import argparse
import random

import cv2
import numpy as np


def paste(canvas: np.ndarray, face: np.ndarray, x: int, y: int, size: int) -> None:
    small = cv2.resize(face, (size, size), interpolation=cv2.INTER_AREA)
    h, w = canvas.shape[:2]
    x = max(0, min(w - size, x))
    y = max(0, min(h - size, y))
    canvas[y:y + size, x:x + size] = small


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--face", required=True, help="synthetic face that should be found")
    ap.add_argument("--others", nargs="*", default=[], help="other synthetic faces (should not match)")
    ap.add_argument("--out", required=True)
    ap.add_argument("--seconds", type=int, default=12)
    ap.add_argument("--fps", type=int, default=15)
    ap.add_argument("--size", type=int, default=150, help="pasted image size in px (the face is ~55%% of it)")
    args = ap.parse_args()

    rng = random.Random(3)
    target = cv2.imread(args.face)
    others = [cv2.imread(p) for p in args.others]
    w, h = 1280, 720
    writer = cv2.VideoWriter(args.out, cv2.VideoWriter_fourcc(*"mp4v"), args.fps, (w, h))
    total = args.seconds * args.fps
    for i in range(total):
        t = i / args.fps
        frame = np.full((h, w, 3), 55, np.uint8)
        cv2.rectangle(frame, (0, 480), (w, h), (70, 70, 75), -1)
        cv2.putText(frame, f"SYNTHETIC TEST CLIP  t={t:05.2f}s", (20, 40), cv2.FONT_HERSHEY_SIMPLEX, 1.0, (200, 200, 200), 2)
        third = args.seconds / 3
        if others and t < third:
            paste(frame, others[0], 200 + int(t * 40), 250, args.size)
        if third <= t < 2 * third:
            paste(frame, target, 500 + int((t - third) * 30), 240, args.size)
        if len(others) > 1 and t >= 2 * third:
            paste(frame, others[1], 800 - int((t - 2 * third) * 40), 260, args.size)
        noise = np.random.default_rng(i).normal(0, 4, frame.shape)
        frame = np.clip(frame.astype(np.float32) + noise, 0, 255).astype(np.uint8)
        writer.write(frame)
    writer.release()
    print(args.out)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
