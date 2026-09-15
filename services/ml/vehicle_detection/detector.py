"""Vehicle detection with YOLOX-s (Apache-2.0) on ONNX Runtime, CPU.

YOLOX is trained on COCO, so it knows five road-vehicle classes: bicycle, car,
motorcycle, bus and truck. It has no class for auto-rickshaws, e-rickshaws,
cycle-rickshaws, taxis or light commercial vehicles; an auto-rickshaw is
usually reported as a car or motorcycle, sometimes not at all. The service says
so in /health rather than guessing a class the model does not have.
"""

from __future__ import annotations

from dataclasses import dataclass

import cv2
import numpy as np
import onnxruntime as ort

from model_registry import VEHICLE_DETECTOR

# COCO index -> platform vehicle class.
VEHICLE_CLASSES = {1: "BICYCLE", 2: "CAR", 3: "MOTORCYCLE", 5: "BUS", 7: "TRUCK"}
UNSUPPORTED_CLASSES = ["AUTO_RICKSHAW", "E_RICKSHAW", "CYCLE_RICKSHAW", "TAXI", "LCV"]

INPUT_SIZE = 640
STRIDES = (8, 16, 32)


@dataclass
class VehicleBox:
    vehicle_class: str
    confidence: float
    box: tuple[int, int, int, int]  # x1, y1, x2, y2 in source pixels


def _nms(boxes: np.ndarray, scores: np.ndarray, iou: float) -> list[int]:
    order = scores.argsort()[::-1]
    keep = []
    areas = (boxes[:, 2] - boxes[:, 0]) * (boxes[:, 3] - boxes[:, 1])
    while order.size:
        i = order[0]
        keep.append(int(i))
        xx1 = np.maximum(boxes[i, 0], boxes[order[1:], 0])
        yy1 = np.maximum(boxes[i, 1], boxes[order[1:], 1])
        xx2 = np.minimum(boxes[i, 2], boxes[order[1:], 2])
        yy2 = np.minimum(boxes[i, 3], boxes[order[1:], 3])
        inter = np.clip(xx2 - xx1, 0, None) * np.clip(yy2 - yy1, 0, None)
        ovr = inter / (areas[i] + areas[order[1:]] - inter + 1e-9)
        order = order[1:][ovr <= iou]
    return keep


class VehicleDetector:
    def __init__(self, threads: int = 2, score_threshold: float = 0.35):
        opts = ort.SessionOptions()
        opts.intra_op_num_threads = threads
        self.session = ort.InferenceSession(str(VEHICLE_DETECTOR.path), opts, providers=["CPUExecutionProvider"])
        self.input_name = self.session.get_inputs()[0].name
        self.score_threshold = score_threshold
        grids, strides = [], []
        for s in STRIDES:
            n = INPUT_SIZE // s
            xv, yv = np.meshgrid(np.arange(n), np.arange(n))
            grids.append(np.stack((xv, yv), 2).reshape(-1, 2))
            strides.append(np.full((n * n, 1), s))
        self.grids = np.concatenate(grids).astype(np.float32)
        self.strides = np.concatenate(strides).astype(np.float32)

    def detect(self, bgr: np.ndarray) -> list[VehicleBox]:
        h, w = bgr.shape[:2]
        r = min(INPUT_SIZE / h, INPUT_SIZE / w)
        resized = cv2.resize(bgr, (int(w * r), int(h * r)), interpolation=cv2.INTER_LINEAR)
        canvas = np.full((INPUT_SIZE, INPUT_SIZE, 3), 114, dtype=np.uint8)
        canvas[: resized.shape[0], : resized.shape[1]] = resized
        blob = canvas.transpose(2, 0, 1)[None].astype(np.float32)
        out = self.session.run(None, {self.input_name: blob})[0][0]

        xy = (out[:, :2] + self.grids) * self.strides
        wh = np.exp(out[:, 2:4]) * self.strides
        obj = out[:, 4:5]
        cls = out[:, 5:]
        ids = np.array(list(VEHICLE_CLASSES))
        vehicle_scores = obj * cls[:, ids]
        best = vehicle_scores.argmax(1)
        score = vehicle_scores[np.arange(len(best)), best]
        mask = score >= self.score_threshold
        if not mask.any():
            return []
        xy, wh, score, best = xy[mask], wh[mask], score[mask], best[mask]
        boxes = np.concatenate([xy - wh / 2, xy + wh / 2], 1) / r
        boxes[:, [0, 2]] = boxes[:, [0, 2]].clip(0, w)
        boxes[:, [1, 3]] = boxes[:, [1, 3]].clip(0, h)

        keep = []
        for c in np.unique(best):
            idx = np.where(best == c)[0]
            keep += [int(idx[k]) for k in _nms(boxes[idx], score[idx], 0.45)]
        keep = np.array(keep)
        # One object reported as both car and truck keeps its stronger class.
        final = [int(keep[k]) for k in _nms(boxes[keep], score[keep], 0.75)]
        result = []
        for i in sorted(final, key=lambda i: -score[i]):
            x1, y1, x2, y2 = boxes[i]
            if x2 - x1 < 8 or y2 - y1 < 8:
                continue
            result.append(VehicleBox(
                vehicle_class=VEHICLE_CLASSES[int(ids[best[i]])],
                confidence=round(float(score[i]), 4),
                box=(int(round(x1)), int(round(y1)), int(round(x2)), int(round(y2))),
            ))
        return result
