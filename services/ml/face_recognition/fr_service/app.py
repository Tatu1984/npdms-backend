"""NPDMS face recognition service (FastAPI).

Stateless: it holds the model files and nothing else. The NPDMS API owns
enrolments, candidates, authorisation and audit, and sends this service the
media and the enrolled embeddings to compare against for each request. No
media or embeddings are written to disk except a temporary file while a video
is decoded, which is deleted before the response is sent.

This service is only reachable from the API (bind it to the private network;
set FR_SERVICE_TOKEN on both sides). It never calls out to the internet.
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import time
from typing import Dict, List, Optional

import cv2
import numpy as np
from fastapi import Depends, FastAPI, File, Form, Header, HTTPException, UploadFile
from fastapi.responses import JSONResponse

from .engine import (
    EMBEDDING_DIM, MODEL_VERSION, FaceEngine, FrameFaceRules, ModelError, QualityPolicy,
    crop_with_margin, face_dict, match,
)
from .media import MediaError, VideoFile, decode_image, is_video

MODEL_DIR = os.environ.get("FR_MODEL_DIR", os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "models"))
MAX_UPLOAD_MB = int(os.environ.get("FR_MAX_UPLOAD_MB", "512"))
MAX_FRAMES = int(os.environ.get("FR_MAX_FRAMES", "7200"))
MAX_CANDIDATES = int(os.environ.get("FR_MAX_CANDIDATES", "100"))
SERVICE_TOKEN = os.environ.get("FR_SERVICE_TOKEN", "")

app = FastAPI(title="NPDMS face recognition", version="1.0.0", docs_url=None, redoc_url=None)

_engine: Optional[FaceEngine] = None
_engine_error: Optional[str] = None


def engine() -> FaceEngine:
    global _engine, _engine_error
    if _engine is None and _engine_error is None:
        try:
            _engine = FaceEngine(MODEL_DIR)
        except ModelError as exc:  # reported by /health and every call
            _engine_error = str(exc)
    if _engine is None:
        raise HTTPException(status_code=503, detail=f"face recognition models are not available: {_engine_error}")
    return _engine


def require_token(authorization: Optional[str] = Header(default=None)) -> None:
    if not SERVICE_TOKEN:
        return
    expected = f"Bearer {SERVICE_TOKEN}"
    if not authorization or not hmac.compare_digest(authorization, expected):
        raise HTTPException(status_code=401, detail="invalid service token")


async def read_upload(file: UploadFile) -> bytes:
    data = await file.read(MAX_UPLOAD_MB * 1024 * 1024 + 1)
    if len(data) > MAX_UPLOAD_MB * 1024 * 1024:
        raise HTTPException(status_code=413, detail=f"file is larger than {MAX_UPLOAD_MB} MB")
    if not data:
        raise HTTPException(status_code=400, detail="empty file")
    return data


def jpeg_b64(image: np.ndarray, quality: int = 92) -> Dict[str, str]:
    ok, buf = cv2.imencode(".jpg", image, [cv2.IMWRITE_JPEG_QUALITY, quality])
    if not ok:
        raise HTTPException(status_code=500, detail="could not encode image")
    raw = buf.tobytes()
    return {"jpeg": base64.b64encode(raw).decode("ascii"), "sha256": hashlib.sha256(raw).hexdigest(),
            "width": int(image.shape[1]), "height": int(image.shape[0])}


@app.get("/health")
def health() -> JSONResponse:
    try:
        info = engine().describe()
        return JSONResponse({"status": "ok", **info})
    except HTTPException as exc:
        return JSONResponse({"status": "unavailable", "error": exc.detail}, status_code=503)


@app.post("/v1/enrol", dependencies=[Depends(require_token)])
async def enrol(file: UploadFile = File(...)) -> Dict:
    eng = engine()
    data = await read_upload(file)
    try:
        image = decode_image(data)
    except MediaError as exc:
        raise HTTPException(status_code=422, detail=str(exc))
    started = time.perf_counter()
    result = eng.enrol(image, QualityPolicy())
    out = {k: v for k, v in result.items() if k not in ("embedding", "alignedFace")}
    out["photoSha256"] = hashlib.sha256(data).hexdigest()
    out["imageSize"] = {"width": int(image.shape[1]), "height": int(image.shape[0])}
    if result["accepted"]:
        emb: np.ndarray = result["embedding"]
        out["embedding"] = [round(float(v), 7) for v in emb]
        out["alignedFace"] = jpeg_b64(result["alignedFace"])
    out["durationMs"] = int((time.perf_counter() - started) * 1000)
    return out


def parse_gallery(gallery_ids: str, gallery_groups: Optional[str], gallery: bytes):
    try:
        ids = json.loads(gallery_ids)
        groups = json.loads(gallery_groups) if gallery_groups else ids
    except ValueError:
        raise HTTPException(status_code=400, detail="gallery_ids and gallery_groups must be JSON arrays of strings")
    if not isinstance(ids, list) or not isinstance(groups, list) or len(groups) != len(ids):
        raise HTTPException(status_code=400, detail="gallery_ids and gallery_groups must be arrays of the same length")
    if len(gallery) != len(ids) * EMBEDDING_DIM * 4:
        raise HTTPException(status_code=400, detail=f"gallery must hold {len(ids)} x {EMBEDDING_DIM} little-endian float32 values")
    matrix = np.frombuffer(gallery, dtype="<f4").reshape(len(ids), EMBEDDING_DIM).astype(np.float32)
    norms = np.linalg.norm(matrix, axis=1, keepdims=True)
    norms[norms == 0] = 1
    return [str(i) for i in ids], [str(g) for g in groups], matrix / norms


@app.post("/v1/match", dependencies=[Depends(require_token)])
async def match_media(
    file: UploadFile = File(...),
    gallery_ids: str = Form(...),
    gallery: UploadFile = File(...),
    threshold: float = Form(...),
    gallery_groups: Optional[str] = Form(default=None),
    model_version: str = Form(...),
    sample_fps: float = Form(default=1.0),
    merge_window_s: float = Form(default=5.0),
) -> Dict:
    """Compare every usable face in a still or in sampled video frames against
    the gallery. Hits for the same gallery group (a missing-person report)
    within merge_window_s of each other are merged, keeping the best one."""
    eng = engine()
    if model_version != MODEL_VERSION:
        raise HTTPException(status_code=409, detail=f"gallery was enrolled with {model_version}; this service runs {MODEL_VERSION}. Re-enrol first.")
    if not (0.0 < threshold <= 1.0):
        raise HTTPException(status_code=400, detail="threshold must be in (0, 1]")
    if not (0.1 <= sample_fps <= 5.0):
        raise HTTPException(status_code=400, detail="sample_fps must be between 0.1 and 5")
    ids, groups, matrix = parse_gallery(gallery_ids, gallery_groups, await gallery.read())
    data = await read_upload(file)
    started = time.perf_counter()
    rules = FrameFaceRules()

    frames_analysed = faces_seen = faces_compared = 0
    # group -> list of kept hits (each carries its frame bytes until the end)
    kept: Dict[str, List[Dict]] = {}
    video_info = None
    kind = "video" if is_video(file.filename or "", file.content_type) else "image"

    def consider(frame: np.ndarray, frame_index: int, offset_ms: Optional[int]) -> None:
        nonlocal faces_seen, faces_compared
        usable, seen = eng.faces_in_frame(frame, rules)
        faces_seen += seen
        faces_compared += len(usable)
        encoded_frame = None
        for face in usable:
            best: Dict[str, Dict] = {}
            for gid, sim in match(face["embedding"], ids, matrix, threshold):
                group = groups[ids.index(gid)]
                if group not in best or sim > best[group]["similarity"]:
                    best[group] = {"galleryId": gid, "similarity": sim}
            for group, hit in best.items():
                fd = face_dict(face["face"])
                entry = {
                    "galleryId": hit["galleryId"], "group": group, "similarity": round(hit["similarity"], 4),
                    "frameIndex": frame_index, "frameOffsetMs": offset_ms, "box": fd["box"],
                    "detectionScore": fd["detectionScore"], "quality": face["quality"].as_dict(),
                }
                hits = kept.setdefault(group, [])
                prev = hits[-1] if hits else None
                same_window = (
                    prev is not None and offset_ms is not None and prev["frameOffsetMs"] is not None
                    and offset_ms - prev["frameOffsetMs"] <= merge_window_s * 1000
                )
                if same_window and prev is not None and entry["similarity"] <= prev["similarity"]:
                    continue
                if encoded_frame is None:
                    encoded_frame = frame
                entry["_frame"] = frame
                if same_window:
                    # Keep the window anchored on the first hit so a person
                    # standing in view for minutes still yields one candidate
                    # per window, not one per frame.
                    entry["frameWindowStartMs"] = prev.get("frameWindowStartMs", prev["frameOffsetMs"])
                    hits[-1] = entry
                else:
                    entry["frameWindowStartMs"] = offset_ms
                    hits.append(entry)

    try:
        if kind == "image":
            try:
                image = decode_image(data)
            except MediaError as exc:
                raise HTTPException(status_code=422, detail=str(exc))
            frames_analysed = 1
            consider(image, 0, None)
        else:
            suffix = os.path.splitext(file.filename or "")[1] or ".mp4"
            try:
                video = VideoFile(data, suffix=suffix)
            except MediaError as exc:
                raise HTTPException(status_code=422, detail=str(exc))
            try:
                video_info = video.info
                for frame_index, offset_ms, frame in video.frames(sample_fps, MAX_FRAMES):
                    frames_analysed += 1
                    consider(frame, frame_index, offset_ms)
            finally:
                video.close()
            if frames_analysed == 0:
                raise HTTPException(status_code=422, detail="no frames could be read from the footage")
    finally:
        del data

    candidates = [h for hits in kept.values() for h in hits]
    candidates.sort(key=lambda h: -h["similarity"])
    truncated = len(candidates) > MAX_CANDIDATES
    candidates = candidates[:MAX_CANDIDATES]
    out_candidates = []
    for c in candidates:
        frame = c.pop("_frame")
        c["frame"] = jpeg_b64(frame)
        c["crop"] = jpeg_b64(crop_with_margin(frame, c["box"]))
        out_candidates.append(c)
    out_candidates.sort(key=lambda h: (h["frameOffsetMs"] or 0, -h["similarity"]))

    return {
        "modelVersion": MODEL_VERSION,
        "threshold": threshold,
        "kind": kind,
        "sampleFps": sample_fps if kind == "video" else None,
        "framesAnalysed": frames_analysed,
        "facesSeen": faces_seen,
        "facesCompared": faces_compared,
        "gallerySize": len(ids),
        "truncated": truncated,
        "video": None if video_info is None else {
            "fps": round(video_info.fps, 3), "durationS": round(video_info.duration_s, 2),
            "width": video_info.width, "height": video_info.height,
        },
        "durationMs": int((time.perf_counter() - started) * 1000),
        "candidates": out_candidates,
    }
