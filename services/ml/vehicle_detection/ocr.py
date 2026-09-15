"""Plate text localisation and reading with PaddleOCR PP-OCRv4 (Apache-2.0).

There is no dedicated number-plate detector here. The permissively licensed
plate detectors we reviewed were trained with GPL-3.0 code (see MODELS.md), so
plates are localised as text: PP-OCRv4 text detection finds text regions inside
each vehicle box, each region (or a pair of stacked regions, for two-line
plates) is read by PP-OCRv4 recognition, and only a read that matches an Indian
registration format is kept as a plate. Shop signs, "TAXI" and route boards do
not match a format and are dropped.

Recognition is restricted to A–Z and 0–9 at decode time, and every character
carries its own probability from the CTC output.
"""

from __future__ import annotations

from dataclasses import dataclass

import cv2
import numpy as np
import onnxruntime as ort

from model_registry import TEXT_DETECTOR, TEXT_RECOGNISER


@dataclass
class TextRegion:
    quad: np.ndarray  # 4x2 float32, clockwise from top-left
    score: float

    @property
    def bbox(self) -> tuple[float, float, float, float]:
        return (float(self.quad[:, 0].min()), float(self.quad[:, 1].min()),
                float(self.quad[:, 0].max()), float(self.quad[:, 1].max()))


@dataclass
class TextRead:
    text: str
    char_confidences: list[float]


def _session(path, threads):
    opts = ort.SessionOptions()
    opts.intra_op_num_threads = threads
    return ort.InferenceSession(str(path), opts, providers=["CPUExecutionProvider"])


def _order_quad(pts: np.ndarray) -> np.ndarray:
    xs = pts[np.argsort(pts[:, 0])]
    left, right = xs[:2], xs[2:]
    tl, bl = left[np.argsort(left[:, 1])]
    tr, br = right[np.argsort(right[:, 1])]
    return np.array([tl, tr, br, bl], dtype=np.float32)


class PlateTextEngine:
    def __init__(self, threads: int = 2):
        self.det = _session(TEXT_DETECTOR.path, threads)
        self.rec = _session(TEXT_RECOGNISER.path, threads)
        chars = self.rec.get_modelmeta().custom_metadata_map["character"].splitlines()
        # CTC classes: 0 is blank, then the model's dictionary, then a space.
        self.classes = ["<blank>"] + chars + [" "]
        allowed = [i for i, c in enumerate(self.classes) if len(c) == 1 and c.isascii() and c.isalnum()]
        self.allowed = np.array([0] + allowed)
        self.allowed_chars = ["" ] + [self.classes[i].upper() for i in allowed]

    # ------------------------------------------------------------ detection --

    def detect(self, bgr: np.ndarray, max_side: int = 640, box_thresh: float = 0.5) -> list[TextRegion]:
        h, w = bgr.shape[:2]
        scale = max_side / max(h, w)
        nh, nw = max(32, int(round(h * scale / 32)) * 32), max(32, int(round(w * scale / 32)) * 32)
        img = cv2.resize(bgr, (nw, nh)).astype(np.float32)
        img = (img / 255.0 - 0.5) / 0.5
        pred = self.det.run(None, {"x": img.transpose(2, 0, 1)[None]})[0][0, 0]
        bitmap = cv2.dilate((pred > 0.3).astype(np.uint8), np.ones((2, 2), np.uint8))
        contours, _ = cv2.findContours(bitmap, cv2.RETR_LIST, cv2.CHAIN_APPROX_SIMPLE)
        regions = []
        sx, sy = w / nw, h / nh
        for contour in contours[:200]:
            (cx, cy), (rw, rh), angle = cv2.minAreaRect(contour)
            if min(rw, rh) < 3:
                continue
            x, y, bw, bh = cv2.boundingRect(contour)
            mask = np.zeros((bh, bw), np.uint8)
            cv2.fillPoly(mask, [contour - [x, y]], 1)
            score = float(cv2.mean(pred[y:y + bh, x:x + bw], mask)[0])
            if score < box_thresh:
                continue
            # Unclip: DB shrinks text regions during training, so grow them back.
            area, perimeter = rw * rh, 2 * (rw + rh)
            d = area * 1.6 / max(perimeter, 1e-6)
            box = cv2.boxPoints(((cx, cy), (rw + 2 * d, rh + 2 * d), angle))
            if min(rw + 2 * d, rh + 2 * d) < 5:
                continue
            box[:, 0] = (box[:, 0] * sx).clip(0, w - 1)
            box[:, 1] = (box[:, 1] * sy).clip(0, h - 1)
            regions.append(TextRegion(quad=_order_quad(box), score=score))
        return regions

    # ---------------------------------------------------------- recognition --

    @staticmethod
    def crop(bgr: np.ndarray, quad: np.ndarray, pad_x: float = 0.03, pad_y: float = 0.12) -> np.ndarray:
        tl, tr, br, bl = quad
        width = int(max(np.linalg.norm(tr - tl), np.linalg.norm(br - bl)))
        height = int(max(np.linalg.norm(bl - tl), np.linalg.norm(br - tr)))
        width, height = max(width, 4), max(height, 4)
        px, py = width * pad_x, height * pad_y
        dst = np.array([[px, py], [px + width, py], [px + width, py + height], [px, py + height]], np.float32)
        m = cv2.getPerspectiveTransform(quad.astype(np.float32), dst)
        return cv2.warpPerspective(bgr, m, (int(width + 2 * px), int(height + 2 * py)),
                                   borderMode=cv2.BORDER_REPLICATE)

    def read(self, crops: list[np.ndarray]) -> list[TextRead]:
        if not crops:
            return []
        prepared = []
        max_ratio = max(320 / 48, max(c.shape[1] / max(c.shape[0], 1) for c in crops))
        width = min(int(48 * max_ratio), 1280)
        for c in crops:
            h, w = c.shape[:2]
            rw = min(width, int(np.ceil(48 * w / max(h, 1))))
            img = cv2.resize(c, (max(rw, 8), 48)).astype(np.float32).transpose(2, 0, 1) / 255.0
            img = (img - 0.5) / 0.5
            padded = np.zeros((3, 48, width), np.float32)
            padded[:, :, : img.shape[2]] = img
            prepared.append(padded)
        probs = self.rec.run(None, {"x": np.stack(prepared)})[0]
        out = []
        for p in probs:
            p = p[:, self.allowed]  # keep blank and A–Z/0–9 only
            idx = p.argmax(1)
            text, confs, prev = [], [], 0
            for t, k in enumerate(idx):
                if k != 0 and k != prev:
                    text.append(self.allowed_chars[k])
                    confs.append(float(p[t, k]))
                elif k != 0 and k == prev:
                    confs[-1] = max(confs[-1], float(p[t, k]))
                prev = k
            out.append(TextRead("".join(text), confs))
        return out
