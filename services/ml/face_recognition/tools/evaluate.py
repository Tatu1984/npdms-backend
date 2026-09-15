"""Measure similarity distributions on a folder of synthetic face images.

    python -m tools.evaluate --faces /path/to/synthetic/faces [--limit 200] [--json out.json]

Use only synthetic or clearly licensed images, never photos of real private
individuals. Each image is one identity. Because a synthetic set has one image
per identity, "same identity" probes are made by degrading the image the way
CCTV degrades a face (downscaling, blur, compression, noise, lighting, small
rotation and horizontal squeeze for pose). That is much easier than a real
same-person pair taken years apart in different light, so treat the genuine
scores here as an upper bound; see MODELS.md for what this does not measure.
"""

from __future__ import annotations

import argparse
import glob
import json
import os
import random
import sys
import time

import cv2
import numpy as np

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from fr_service.engine import FaceEngine, FrameFaceRules  # noqa: E402

MODEL_DIR = os.environ.get("FR_MODEL_DIR", os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "models"))


def degrade(img: np.ndarray, rng: random.Random, face_px: int) -> np.ndarray:
    """A CCTV-like view of the same synthetic face."""
    h, w = img.shape[:2]
    out = img.copy()
    # Pose: rotation up to 12 degrees and a horizontal squeeze (crude yaw).
    angle = rng.uniform(-12, 12)
    squeeze = rng.uniform(0.82, 1.0)
    m = cv2.getRotationMatrix2D((w / 2, h / 2), angle, 1.0)
    m[0, 0] *= squeeze
    m[0, 1] *= squeeze
    m[0, 2] += (1 - squeeze) * w / 2
    out = cv2.warpAffine(out, m, (w, h), borderMode=cv2.BORDER_REFLECT)
    # Lighting.
    alpha, beta = rng.uniform(0.6, 1.25), rng.uniform(-35, 25)
    out = cv2.convertScaleAbs(out, alpha=alpha, beta=beta)
    # Resolution: the face in the source images is ~55% of the width. Scale so
    # it is about face_px across, then back up (what a CCTV crop looks like).
    scale = face_px / (0.55 * w)
    small = cv2.resize(out, (max(8, int(w * scale)), max(8, int(h * scale))), interpolation=cv2.INTER_AREA)
    if rng.random() < 0.7:
        small = cv2.GaussianBlur(small, (3, 3), rng.uniform(0.3, 1.0))
    noise = np.random.default_rng(rng.randrange(1 << 30)).normal(0, rng.uniform(2, 7), small.shape)
    small = np.clip(small.astype(np.float32) + noise, 0, 255).astype(np.uint8)
    ok, buf = cv2.imencode(".jpg", small, [cv2.IMWRITE_JPEG_QUALITY, rng.randint(35, 70)])
    small = cv2.imdecode(buf, cv2.IMREAD_COLOR)
    # Place the small face on a larger dark frame, as in a wide CCTV shot.
    canvas = np.full((max(small.shape[0] * 3, 240), max(small.shape[1] * 3, 320), 3), 40, np.uint8)
    y0 = (canvas.shape[0] - small.shape[0]) // 2
    x0 = (canvas.shape[1] - small.shape[1]) // 2
    canvas[y0:y0 + small.shape[0], x0:x0 + small.shape[1]] = small
    return canvas


def summarise(values):
    a = np.asarray(values, dtype=np.float64)
    if a.size == 0:
        return {"n": 0}
    return {"n": int(a.size), "mean": round(float(a.mean()), 4), "std": round(float(a.std()), 4),
            "min": round(float(a.min()), 4), "p01": round(float(np.percentile(a, 1)), 4),
            "p05": round(float(np.percentile(a, 5)), 4), "p50": round(float(np.percentile(a, 50)), 4),
            "p95": round(float(np.percentile(a, 95)), 4), "p99": round(float(np.percentile(a, 99)), 4),
            "p999": round(float(np.percentile(a, 99.9)), 4), "max": round(float(a.max()), 4)}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--faces", required=True)
    ap.add_argument("--limit", type=int, default=300)
    ap.add_argument("--probes", type=int, default=3, help="degraded probes per identity per resolution")
    ap.add_argument("--resolutions", default="112,64,40", help="probe face sizes in px")
    ap.add_argument("--seed", type=int, default=7)
    ap.add_argument("--json")
    args = ap.parse_args()

    rng = random.Random(args.seed)
    eng = FaceEngine(MODEL_DIR)
    files = sorted(glob.glob(os.path.join(args.faces, "*.jpg")) + glob.glob(os.path.join(args.faces, "*.png")))[: args.limit]

    enrol_ids, enrol_embs, rejections = [], [], {}
    t0 = time.perf_counter()
    for f in files:
        r = eng.enrol(cv2.imread(f))
        if r["accepted"]:
            enrol_ids.append(f)
            enrol_embs.append(r["embedding"])
        else:
            rejections[r["reason"]] = rejections.get(r["reason"], 0) + 1
    enrol_ms = (time.perf_counter() - t0) * 1000 / max(1, len(files))
    gallery = np.stack(enrol_embs)

    report = {"images": len(files), "enrolled": len(enrol_ids), "enrolRejections": rejections,
              "enrolMsPerImage": round(enrol_ms, 1), "byResolution": {}}

    # Impostor scores between enrolment photos of different identities.
    sims = gallery @ gallery.T
    iu = np.triu_indices(len(enrol_ids), k=1)
    report["impostorEnrolVsEnrol"] = summarise(sims[iu])

    rules = FrameFaceRules()  # the production rules: faces they drop count as not detected
    for res in [int(x) for x in args.resolutions.split(",")]:
        genuine, impostor, not_detected, frame_ms = [], [], 0, []
        rank1_ok = 0
        probes = 0
        for i, f in enumerate(enrol_ids):
            img = cv2.imread(f)
            for _ in range(args.probes):
                probe = degrade(img, rng, res)
                t = time.perf_counter()
                faces, _ = eng.faces_in_frame(probe, rules)
                frame_ms.append((time.perf_counter() - t) * 1000)
                probes += 1
                if not faces:
                    not_detected += 1
                    continue
                emb = faces[0]["embedding"]
                s = gallery @ emb
                genuine.append(float(s[i]))
                impostor.extend(float(v) for j, v in enumerate(s) if j != i)
                rank1_ok += int(int(np.argmax(s)) == i)
        g, im = np.asarray(genuine), np.asarray(impostor)
        sweep = {}
        for th in (0.30, 0.363, 0.40, 0.45, 0.50, 0.55, 0.60):
            sweep[f"{th:.3f}"] = {
                "trueMatchRate": round(float((g >= th).mean()) if g.size else 0.0, 4),
                "falseMatchRate": round(float((im >= th).mean()) if im.size else 0.0, 6),
                "falseMatchesPerProbeAgainstGallery": round(float((im >= th).sum()) / max(1, len(genuine)), 4),
            }
        report["byResolution"][f"{res}px"] = {
            "probes": probes, "faceNotDetectedOrBelowRules": not_detected, "rank1Accuracy": round(rank1_ok / max(1, len(genuine)), 4),
            "genuine": summarise(genuine), "impostor": summarise(impostor), "thresholdSweep": sweep,
            "frameMsMedian": round(float(np.median(frame_ms)), 1),
        }

    print(json.dumps(report, indent=2))
    if args.json:
        with open(args.json, "w") as fh:
            json.dump(report, fh, indent=2)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
