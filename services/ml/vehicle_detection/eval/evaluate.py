"""Measure plate reading on synthetic West Bengal plates, and detection on a
manually reviewed set of photographs.

    python eval/evaluate.py plates SYNTH_DIR                 # plate crops (from synth_plates.py)
    python eval/evaluate.py scenes SYNTH_DIR PHOTO_DIR       # synthetic plates placed on real vehicles
    python eval/evaluate.py detections PHOTO_DIR REVIEW.json # precision against a manual review

REVIEW.json maps an image file to a verdict per detection index, as produced
by the pinned detector: {"real04.jpg": {"0": "ok", "1": "wrong_class", "2": "false"}, ...}
plus optional "missed": N for vehicles the reviewer saw that were not detected.
"""

from __future__ import annotations

import collections
import json
import random
import sys
import time
from pathlib import Path

import cv2
import numpy as np

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from pipeline import Analyser  # noqa: E402


def report(title: str, counts: dict, times: list[float]) -> None:
    print(f"\n{title}")
    for key in sorted(counts):
        ok, n = counts[key]
        print(f"  {key:<22} {ok:>4}/{n:<4} {100 * ok / max(n, 1):5.1f}%")
    if times:
        print(f"  median {np.median(times):.0f} ms, p90 {np.percentile(times, 90):.0f} ms per frame (CPU)")


def tally(counts, keys, ok):
    for k in keys:
        a, n = counts[k]
        counts[k] = (a + int(ok), n + 1)


def plates(analyser: Analyser, synth: Path) -> None:
    labels = json.loads((synth / "labels.json").read_text())
    counts, times, wrong = collections.defaultdict(lambda: (0, 0)), [], collections.Counter()
    for item in labels:
        img = cv2.imread(str(synth / "plates" / item["file"]))
        # A plate sits inside a vehicle crop, never edge to edge.
        h, w = img.shape[:2]
        canvas = cv2.copyMakeBorder(img, h, h, w // 3, w // 3, cv2.BORDER_CONSTANT, value=(70, 70, 70))
        t = time.perf_counter()
        read = analyser._read_plate(canvas)
        times.append((time.perf_counter() - t) * 1000)
        got = read["plate"]["normalised"] if read else None
        ok = got == item["plate"]
        if read and not ok:
            wrong["misread"] += 1
        elif not read:
            wrong["no plate returned"] += 1
        tally(counts, ["all", "condition:" + item["condition"], "style:" + item["style"]], ok)
    report("Plate crops — exact match of the whole registration", counts, times)
    print("  failures:", dict(wrong))


def scenes(analyser: Analyser, synth: Path, photos: Path) -> None:
    labels = json.loads((synth / "labels.json").read_text())
    rng = random.Random(11)
    hosts = []
    for f in sorted(photos.glob("*.jpg")):
        img = cv2.imread(str(f))
        for d in analyser.detector.detect(img):
            x1, y1, x2, y2 = d.box
            if d.vehicle_class in ("CAR", "BUS", "TRUCK") and x2 - x1 >= 220:
                hosts.append((f, d.box))
    counts, times = collections.defaultdict(lambda: (0, 0)), []
    for item in labels:
        f, (x1, y1, x2, y2) = rng.choice(hosts)
        img = cv2.imread(str(f))
        plate = cv2.imread(str(synth / "plates" / item["file"]))
        if item["condition"] == "small":
            continue  # already tiny; placing it again would double the downscale
        pw = int((x2 - x1) * rng.uniform(0.22, 0.32))
        ph = int(pw * plate.shape[0] / plate.shape[1])
        plate = cv2.resize(plate, (pw, ph), interpolation=cv2.INTER_AREA)
        px = int((x1 + x2) / 2 - pw / 2)
        py = int(y1 + (y2 - y1) * rng.uniform(0.62, 0.78))
        if py + ph >= img.shape[0] or px < 0 or px + pw >= img.shape[1]:
            continue
        img[py:py + ph, px:px + pw] = plate
        t = time.perf_counter()
        result = analyser.analyse(img)
        times.append((time.perf_counter() - t) * 1000)
        reads = [d["plateRead"]["plate"]["normalised"] for d in result["detections"] if d["plateRead"]]
        reads += [r["plate"]["normalised"] for r in result["unattachedPlateReads"]]
        ok = item["plate"] in reads
        tally(counts, ["all", "condition:" + item["condition"], "style:" + item["style"],
                       f"plate width {'<90px' if pw < 90 else '90-150px' if pw < 150 else '>=150px'}"], ok)
    report("Synthetic plates on real vehicles — registration found in the frame's reads", counts, times)


def detections(analyser: Analyser, photos: Path, review_file: Path) -> None:
    review = json.loads(review_file.read_text())
    counts = collections.Counter()
    per_class = collections.defaultdict(collections.Counter)
    times = []
    for name, verdicts in review.items():
        img = cv2.imread(str(photos / name))
        t = time.perf_counter()
        dets = analyser.detector.detect(img)
        times.append((time.perf_counter() - t) * 1000)
        for i, d in enumerate(dets):
            v = verdicts.get(str(i), "unreviewed")
            counts[v] += 1
            per_class[d.vehicle_class][v] += 1
        counts["missed"] += int(verdicts.get("missed", 0))
    ok, wrong, false = counts["ok"], counts["wrong_class"], counts["false"]
    print("\nDetection — manual review")
    print(f"  detections {ok + wrong + false} (unreviewed {counts['unreviewed']})")
    print(f"  precision, vehicle present: {100 * (ok + wrong) / max(ok + wrong + false, 1):.1f}%")
    print(f"  precision, vehicle and class right: {100 * ok / max(ok + wrong + false, 1):.1f}%")
    print(f"  recall against vehicles the reviewer counted: {100 * (ok + wrong) / max(ok + wrong + counts['missed'], 1):.1f}%")
    for cls, c in sorted(per_class.items()):
        print(f"  {cls:<11} ok {c['ok']}, wrong class {c['wrong_class']}, false {c['false']}")
    print(f"  median {np.median(times):.0f} ms per frame for detection alone (CPU)")


if __name__ == "__main__":
    mode = sys.argv[1]
    a = Analyser(threads=int(__import__("os").getenv("ANPR_THREADS", "2")))
    if mode == "plates":
        plates(a, Path(sys.argv[2]))
    elif mode == "scenes":
        scenes(a, Path(sys.argv[2]), Path(sys.argv[3]))
    elif mode == "detections":
        detections(a, Path(sys.argv[2]), Path(sys.argv[3]))
    else:
        raise SystemExit(__doc__)
