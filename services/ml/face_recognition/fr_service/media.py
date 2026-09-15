"""Decoding stills and sampling frames from video footage."""

from __future__ import annotations

import os
import tempfile
from dataclasses import dataclass
from typing import Iterator, Optional, Tuple

import cv2
import numpy as np

VIDEO_EXTENSIONS = {".mp4", ".mov", ".avi", ".mkv", ".m4v", ".webm", ".ts", ".3gp", ".mpg", ".mpeg"}


class MediaError(ValueError):
    pass


def decode_image(data: bytes) -> np.ndarray:
    arr = np.frombuffer(data, dtype=np.uint8)
    img = cv2.imdecode(arr, cv2.IMREAD_COLOR)
    if img is None:
        raise MediaError("the file is not an image this service can read (JPEG, PNG, BMP, WebP or TIFF)")
    return img


def is_video(filename: str, content_type: Optional[str]) -> bool:
    if content_type and content_type.lower().startswith("video/"):
        return True
    return os.path.splitext(filename or "")[1].lower() in VIDEO_EXTENSIONS


@dataclass
class VideoInfo:
    fps: float
    frame_count: int
    duration_s: float
    width: int
    height: int


class VideoFile:
    """Writes the upload to a temporary file (OpenCV needs a path) and samples
    frames at a fixed rate by timestamp, so variable frame rates are handled."""

    def __init__(self, data: bytes, suffix: str = ".mp4"):
        fd, self.path = tempfile.mkstemp(suffix=suffix or ".mp4")
        with os.fdopen(fd, "wb") as fh:
            fh.write(data)
        self.cap = cv2.VideoCapture(self.path)
        if not self.cap.isOpened():
            self.close()
            raise MediaError("the footage could not be decoded; convert it to H.264 MP4 and try again")
        fps = float(self.cap.get(cv2.CAP_PROP_FPS) or 0.0)
        count = int(self.cap.get(cv2.CAP_PROP_FRAME_COUNT) or 0)
        self.info = VideoInfo(
            fps=fps, frame_count=count, duration_s=(count / fps) if fps > 0 else 0.0,
            width=int(self.cap.get(cv2.CAP_PROP_FRAME_WIDTH) or 0), height=int(self.cap.get(cv2.CAP_PROP_FRAME_HEIGHT) or 0),
        )

    def frames(self, sample_fps: float, max_frames: int) -> Iterator[Tuple[int, int, np.ndarray]]:
        """Yields (frame_index, offset_ms, frame) at about sample_fps."""
        interval_ms = 1000.0 / sample_fps
        next_ms = 0.0
        index = -1
        emitted = 0
        native_fps = self.info.fps if self.info.fps > 0 else 25.0
        while emitted < max_frames:
            ok = self.cap.grab()
            if not ok:
                break
            index += 1
            pos = self.cap.get(cv2.CAP_PROP_POS_MSEC)
            offset_ms = pos if pos and pos > 0 else index * 1000.0 / native_fps
            if offset_ms + 1e-6 < next_ms:
                continue
            ok, frame = self.cap.retrieve()
            if not ok or frame is None:
                continue
            emitted += 1
            next_ms = offset_ms + interval_ms
            yield index, int(round(offset_ms)), frame

    def close(self) -> None:
        try:
            if getattr(self, "cap", None) is not None:
                self.cap.release()
        finally:
            try:
                os.unlink(self.path)
            except OSError:
                pass
