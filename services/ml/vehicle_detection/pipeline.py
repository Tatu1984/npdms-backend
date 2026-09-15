"""From one frame to vehicle detections and plate reads."""

from __future__ import annotations

import time
from dataclasses import dataclass

import cv2
import numpy as np

import plates
from detector import VehicleBox, VehicleDetector
from model_registry import TEXT_DETECTOR, TEXT_RECOGNISER, VEHICLE_DETECTOR
from ocr import PlateTextEngine, TextRegion

PIPELINE_VERSION = "npdms-anpr 0.1.0"
PLATE_RULES_VERSION = "IN plate rules 1 (STANDARD, BH, OLD)"

# A vehicle narrower than this in the frame cannot carry a legible plate.
MIN_VEHICLE_WIDTH_FOR_PLATE = 60
# Plate reading is attempted for at most this many vehicles per frame, largest first.
MAX_VEHICLES_READ = 15
# The shortest registration accepted from OCR (WB 02 1234 is 8; WBC 12 is 5).
MIN_PLATE_LENGTH = 6
# A read whose weakest character is below this is still returned, but flagged.
LOW_CHARACTER_CONFIDENCE = 0.6


def detector_version() -> str:
    return VEHICLE_DETECTOR.label


def plate_reader_version() -> str:
    return (f"PP-OCRv4 det {TEXT_DETECTOR.sha256[:12]} + rec {TEXT_RECOGNISER.sha256[:12]}; "
            f"{PLATE_RULES_VERSION}; {PIPELINE_VERSION}")


def _strip_ind(text: str, confs: list[float]) -> tuple[str, list[float]]:
    # The "IND" mark on high-security plates is often read with the number.
    for _ in range(2):
        if text.startswith("IND") and len(text) > 6:
            text, confs = text[3:], confs[3:]
        if text.endswith("IND") and len(text) > 6:
            text, confs = text[:-3], confs[:-3]
    return text, confs


@dataclass
class _Candidate:
    quads: list[np.ndarray]
    two_line: bool


def _candidates(regions: list[TextRegion]) -> list[_Candidate]:
    cands = [_Candidate([r.quad], False) for r in regions]
    for a in regions:
        ax1, ay1, ax2, ay2 = a.bbox
        for b in regions:
            if a is b:
                continue
            bx1, by1, bx2, by2 = b.bbox
            ah, bh = ay2 - ay1, by2 - by1
            overlap = min(ax2, bx2) - max(ax1, bx1)
            # b sits directly under a, similar height, largely overlapping horizontally.
            if (by1 > ay1 and 0 <= by1 - ay2 + 0.35 * ah < 0.9 * ah
                    and overlap > 0.4 * min(ax2 - ax1, bx2 - bx1) and 0.5 < bh / max(ah, 1) < 2):
                cands.append(_Candidate([a.quad, b.quad], True))
    return cands


class Analyser:
    def __init__(self, threads: int = 2):
        self.detector = VehicleDetector(threads=threads)
        self.text = PlateTextEngine(threads=threads)

    def _read_plate(self, crop: np.ndarray) -> dict | None:
        h, w = crop.shape[:2]
        regions = [r for r in self.text.detect(crop, max_side=640 if max(h, w) > 320 else 480) if r.score >= 0.5]
        # Plates are wider than tall and not the size of the whole crop.
        regions = [r for r in regions if (r.bbox[2] - r.bbox[0]) >= 0.8 * (r.bbox[3] - r.bbox[1])][:30]
        if not regions:
            return None
        cands = _candidates(regions)
        crops, owners = [], []
        for ci, c in enumerate(cands):
            for q in c.quads:
                crops.append(self.text.crop(crop, q))
                owners.append(ci)
        reads = self.text.read(crops)
        joined: dict[int, tuple[str, list[float]]] = {}
        for ci, r in zip(owners, reads):
            t, cf = joined.get(ci, ("", []))
            rt, rc = _strip_ind(r.text, r.char_confidences)
            joined[ci] = (t + rt, cf + rc)
        best = None
        for ci, (raw, confs) in joined.items():
            if len(raw) < MIN_PLATE_LENGTH:
                continue
            v = plates.validate(raw)
            if not v.valid or len(v.normalised) != len(confs):
                continue
            mean = float(np.mean(confs))
            if best is None or mean > best[0]:
                best = (mean, ci, raw, confs, v)
        if best is None:
            return None
        mean, ci, raw, confs, v = best
        pts = np.concatenate(cands[ci].quads)
        chars = []
        corrected_positions = {int(n.split(":")[0].split()[1]) - 1 for n in v.corrections if n.startswith("position")}
        for i, ch in enumerate(v.normalised):
            chars.append({"char": ch, "confidence": round(confs[i], 4), "corrected": i in corrected_positions,
                          "raw": raw[i] if i < len(raw) else ""})
        return {
            "rawText": raw,
            "plate": v.as_dict(),
            "confidence": round(mean, 4),
            "minCharConfidence": round(float(min(confs)), 4),
            "lowConfidence": bool(min(confs) < LOW_CHARACTER_CONFIDENCE),
            "characters": chars,
            "twoLine": cands[ci].two_line,
            "box": [float(pts[:, 0].min()), float(pts[:, 1].min()), float(pts[:, 0].max()), float(pts[:, 1].max())],
        }

    def analyse(self, bgr: np.ndarray) -> dict:
        started = time.perf_counter()
        h, w = bgr.shape[:2]
        vehicles: list[VehicleBox] = self.detector.detect(bgr)
        t_detect = time.perf_counter() - started

        order = sorted(range(len(vehicles)), key=lambda i: -(vehicles[i].box[2] - vehicles[i].box[0]) * (vehicles[i].box[3] - vehicles[i].box[1]))
        attempt = set(i for i in order if vehicles[i].box[2] - vehicles[i].box[0] >= MIN_VEHICLE_WIDTH_FOR_PLATE)
        attempt = set([i for i in order if i in attempt][:MAX_VEHICLES_READ])

        detections, seen_plates = [], set()
        for i, v in enumerate(vehicles):
            x1, y1, x2, y2 = v.box
            det = {"vehicleClass": v.vehicle_class, "confidence": v.confidence, "box": [x1, y1, x2, y2],
                   "plateReadAttempted": i in attempt, "plateRead": None}
            if i in attempt:
                # A small margin keeps plates at the edge of the box.
                mx, my = int(0.04 * (x2 - x1)), int(0.04 * (y2 - y1))
                cx1, cy1, cx2, cy2 = max(0, x1 - mx), max(0, y1 - my), min(w, x2 + mx), min(h, y2 + my)
                crop = bgr[cy1:cy2, cx1:cx2]
                if min(crop.shape[:2]) >= 16:
                    read = self._read_plate(crop)
                    if read and read["plate"]["normalised"] not in seen_plates:
                        b = read["box"]
                        read["box"] = [int(b[0] + cx1), int(b[1] + cy1), int(b[2] + cx1), int(b[3] + cy1)]
                        seen_plates.add(read["plate"]["normalised"])
                        det["plateRead"] = read
            detections.append(det)

        # A close-up of a plate, or a vehicle the detector missed: read the frame itself.
        unattached = []
        if not any(d["plateRead"] for d in detections):
            read = self._read_plate(bgr)
            if read and read["plate"]["normalised"] not in seen_plates:
                read["box"] = [int(x) for x in read["box"]]
                unattached.append(read)

        return {
            "width": w,
            "height": h,
            "detections": detections,
            "unattachedPlateReads": unattached,
            "timingMs": {"detect": round(t_detect * 1000, 1), "total": round((time.perf_counter() - started) * 1000, 1)},
        }
