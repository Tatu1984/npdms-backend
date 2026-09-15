"""NPDMS vehicle detection and number-plate reading service.

Stateless: an uploaded still or clip is analysed in memory (a clip passes
through a temporary file that is deleted before the response is sent) and
nothing is kept. The API stores what it needs.

If a model file is missing or does not match its pinned digest, the analysis
endpoints answer 503 and /health says why. There is no mock or fallback
output.

    ANPR_MODEL_DIR=./models uvicorn app:app --host 0.0.0.0 --port 8010
"""

from __future__ import annotations

import base64
import logging
import os
import tempfile
import threading
import time

import cv2
import numpy as np
from fastapi import FastAPI, File, Form, HTTPException, UploadFile
from fastapi.concurrency import run_in_threadpool

import detector as detector_module
from model_registry import ALL_MODELS, verify
from pipeline import PIPELINE_VERSION, Analyser, detector_version, plate_reader_version

log = logging.getLogger("vehicle_detection")

MAX_IMAGE_BYTES = int(os.getenv("ANPR_MAX_IMAGE_BYTES", 20 << 20))
MAX_VIDEO_BYTES = int(os.getenv("ANPR_MAX_VIDEO_BYTES", 512 << 20))
MAX_VIDEO_FRAMES = int(os.getenv("ANPR_MAX_VIDEO_FRAMES", 60))
THREADS = int(os.getenv("ANPR_THREADS", 2))

app = FastAPI(title="NPDMS vehicle detection", version=PIPELINE_VERSION)

_problems = {m.key: verify(m) for m in ALL_MODELS}
_analyser: Analyser | None = None
_load_error: str | None = None
if not any(_problems.values()):
    try:
        _analyser = Analyser(threads=THREADS)
    except Exception as exc:  # a corrupt model is reported, not hidden
        _load_error = f"models failed to load: {exc}"
        log.exception("model load failed")
# One analysis at a time keeps memory bounded on a small edge server.
_lock = threading.Lock()


def _require_analyser() -> Analyser:
    if _analyser is None:
        reason = _load_error or "; ".join(p for p in _problems.values() if p)
        raise HTTPException(status_code=503, detail=f"vehicle detection models are not available: {reason}")
    return _analyser


def _versions() -> dict:
    return {"pipeline": PIPELINE_VERSION, "detector": detector_version(), "plateReader": plate_reader_version()}


@app.get("/health")
def health():
    ready = _analyser is not None
    return {
        "status": "ok" if ready else "unavailable",
        "service": "vehicle_detection",
        "versions": _versions(),
        "models": [
            {"key": m.key, "file": m.filename, "version": m.version, "sha256": m.sha256, "licence": m.licence,
             "source": m.source, "ready": ready and not _problems[m.key], "problem": _problems[m.key] or _load_error}
            for m in ALL_MODELS
        ],
        "vehicleClasses": sorted(set(detector_module.VEHICLE_CLASSES.values())),
        "unsupportedVehicleClasses": detector_module.UNSUPPORTED_CLASSES,
        "colour": "not estimated: no colour model has been validated on Kolkata footage",
        "plateFormats": ["STANDARD", "BH", "OLD"],
        "limits": {"maxImageBytes": MAX_IMAGE_BYTES, "maxVideoBytes": MAX_VIDEO_BYTES, "maxVideoFrames": MAX_VIDEO_FRAMES},
    }


async def _read_limited(file: UploadFile, limit: int) -> bytes:
    data = await file.read(limit + 1)
    if len(data) > limit:
        raise HTTPException(status_code=413, detail=f"file is larger than {limit >> 20} MB")
    if not data:
        raise HTTPException(status_code=400, detail="the uploaded file is empty")
    return data


@app.post("/v1/analyse/image")
async def analyse_image(file: UploadFile = File(...)):
    analyser = _require_analyser()
    data = await _read_limited(file, MAX_IMAGE_BYTES)
    img = cv2.imdecode(np.frombuffer(data, np.uint8), cv2.IMREAD_COLOR)
    if img is None:
        raise HTTPException(status_code=400, detail="the file is not an image this service can decode (JPEG, PNG, BMP, WebP)")

    def run():
        with _lock:
            return analyser.analyse(img)

    result = await run_in_threadpool(run)
    return {"kind": "IMAGE", "versions": _versions(), "sampledFrames": 1,
            "processingMs": result["timingMs"]["total"],
            "frames": [{"index": 0, "offsetSeconds": 0, **result}]}


@app.post("/v1/analyse/video")
async def analyse_video(file: UploadFile = File(...), sample_seconds: float = Form(1.0), max_frames: int = Form(30)):
    analyser = _require_analyser()
    if not (0.2 <= sample_seconds <= 60):
        raise HTTPException(status_code=400, detail="sample_seconds must be between 0.2 and 60")
    max_frames = max(1, min(max_frames, MAX_VIDEO_FRAMES))
    data = await _read_limited(file, MAX_VIDEO_BYTES)

    def run():
        suffix = os.path.splitext(file.filename or "")[1][:8] or ".mp4"
        with tempfile.NamedTemporaryFile(suffix=suffix) as tmp:
            tmp.write(data)
            tmp.flush()
            cap = cv2.VideoCapture(tmp.name)
            if not cap.isOpened():
                raise HTTPException(status_code=400, detail="the file is not a video this service can decode")
            fps = cap.get(cv2.CAP_PROP_FPS) or 0.0
            count = cap.get(cv2.CAP_PROP_FRAME_COUNT) or 0.0
            if fps <= 0:
                cap.release()
                raise HTTPException(status_code=400, detail="the video does not state its frame rate, so frame times cannot be given")
            frames, sampled, started = [], 0, time.perf_counter()
            step = max(1, int(round(fps * sample_seconds)))
            index = 0
            while sampled < max_frames:
                cap.set(cv2.CAP_PROP_POS_FRAMES, index)
                ok, img = cap.read()
                if not ok:
                    break
                sampled += 1
                with _lock:
                    result = analyser.analyse(img)
                if result["detections"] or result["unattachedPlateReads"]:
                    ok, jpg = cv2.imencode(".jpg", img, [cv2.IMWRITE_JPEG_QUALITY, 90])
                    frames.append({"index": index, "offsetSeconds": round(index / fps, 3), **result,
                                   "jpegBase64": base64.b64encode(jpg.tobytes()).decode()})
                index += step
            cap.release()
        return {
            "kind": "VIDEO", "versions": _versions(), "fps": round(fps, 3),
            "durationSeconds": round(count / fps, 3) if count else None,
            "sampleSeconds": sample_seconds, "sampledFrames": sampled,
            "truncated": sampled >= max_frames and (count == 0 or index < count),
            "processingMs": round((time.perf_counter() - started) * 1000, 1),
            "frames": frames,
        }

    return await run_in_threadpool(run)
