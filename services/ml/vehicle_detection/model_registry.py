"""The model files this service runs, where they come from, and their licences.

Every file is pinned by SHA-256. The service refuses to start a model whose
file is missing or does not match — there is no fallback and no mock output.
See MODELS.md for the licence review behind each entry.
"""

from __future__ import annotations

import hashlib
import os
from dataclasses import dataclass
from pathlib import Path

MODEL_DIR = Path(os.getenv("ANPR_MODEL_DIR", Path(__file__).parent / "models"))

RAPIDOCR_WHEEL = (
    "https://files.pythonhosted.org/packages/ba/12/1e5497183bdbe782dbb91bad1d0d2297dba4d2831b2652657f7517bfc6df/"
    "rapidocr_onnxruntime-1.4.4-py3-none-any.whl"
)
RAPIDOCR_WHEEL_SHA256 = "971d7d5f223a7a808662229df1ef69893809d8457d834e6373d3854bc1782cbf"


@dataclass(frozen=True)
class ModelFile:
    key: str
    filename: str
    sha256: str
    version: str
    licence: str
    source: str
    url: str
    # When set, the file is a member of the archive at url (verified by the
    # archive's own digest first).
    archive_member: str | None = None
    archive_sha256: str | None = None

    @property
    def path(self) -> Path:
        return MODEL_DIR / self.filename

    @property
    def label(self) -> str:
        return f"{self.version} (sha256 {self.sha256[:12]})"


VEHICLE_DETECTOR = ModelFile(
    key="vehicle_detector",
    filename="yolox_s.onnx",
    sha256="c5c2d13e59ae883e6af3b45daea64af4833a4951c92d116ec270d9ddbe998063",
    version="YOLOX-s COCO, Megvii release 0.1.1rc0",
    licence="Apache-2.0",
    source="https://github.com/Megvii-BaseDetection/YOLOX",
    url="https://github.com/Megvii-BaseDetection/YOLOX/releases/download/0.1.1rc0/yolox_s.onnx",
)

TEXT_DETECTOR = ModelFile(
    key="plate_text_detector",
    filename="ch_PP-OCRv4_det_infer.onnx",
    sha256="d2a7720d45a54257208b1e13e36a8479894cb74155a5efe29462512d42f49da9",
    version="PaddleOCR PP-OCRv4 mobile text detection (ONNX from RapidOCR 1.4.4)",
    licence="Apache-2.0",
    source="https://github.com/PaddlePaddle/PaddleOCR ; ONNX conversion https://github.com/RapidAI/RapidOCR",
    url=RAPIDOCR_WHEEL,
    archive_member="rapidocr_onnxruntime/models/ch_PP-OCRv4_det_infer.onnx",
    archive_sha256=RAPIDOCR_WHEEL_SHA256,
)

TEXT_RECOGNISER = ModelFile(
    key="plate_text_recogniser",
    filename="ch_PP-OCRv4_rec_infer.onnx",
    sha256="48fc40f24f6d2a207a2b1091d3437eb3cc3eb6b676dc3ef9c37384005483683b",
    version="PaddleOCR PP-OCRv4 mobile text recognition (ONNX from RapidOCR 1.4.4)",
    licence="Apache-2.0",
    source="https://github.com/PaddlePaddle/PaddleOCR ; ONNX conversion https://github.com/RapidAI/RapidOCR",
    url=RAPIDOCR_WHEEL,
    archive_member="rapidocr_onnxruntime/models/ch_PP-OCRv4_rec_infer.onnx",
    archive_sha256=RAPIDOCR_WHEEL_SHA256,
)

ALL_MODELS = [VEHICLE_DETECTOR, TEXT_DETECTOR, TEXT_RECOGNISER]


def sha256_of(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def verify(model: ModelFile) -> str | None:
    """Returns a problem description, or None when the file is present and matches."""
    if not model.path.exists():
        return f"{model.filename} is not installed in {MODEL_DIR}; run fetch_models.py"
    digest = sha256_of(model.path)
    if digest != model.sha256:
        return f"{model.filename} has sha256 {digest[:12]}…, expected {model.sha256[:12]}…"
    return None
