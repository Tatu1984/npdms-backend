"""Face detection, alignment, embedding, quality scoring and matching.

Models (see MODELS.md for provenance, hashes and licences):
  * YuNet (OpenCV Zoo, face_detection_yunet_2023mar.onnx) - MIT
  * SFace (OpenCV Zoo, face_recognition_sface_2021dec.onnx) - Apache-2.0

Everything runs on the CPU through OpenCV's DNN module. Nothing is sent
anywhere; the service holds no data apart from the model files.
"""

from __future__ import annotations

import hashlib
import math
import os
import threading
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Sequence, Tuple

import cv2
import numpy as np

DETECTOR_FILE = "face_detection_yunet_2023mar.onnx"
RECOGNIZER_FILE = "face_recognition_sface_2021dec.onnx"

# Expected SHA-256 of the model files. The service refuses to start with a
# file that does not match, so a swapped or corrupted model cannot silently
# produce matches under the recorded version.
EXPECTED_SHA256 = {
    DETECTOR_FILE: "8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4",
    RECOGNIZER_FILE: "0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79",
}

DETECTOR_VERSION = "yunet-2023mar"
RECOGNIZER_VERSION = "sface-2021dec"
# The version recorded on every enrolment and candidate. Embeddings from
# different recogniser versions are not comparable, so the API only matches
# enrolments made with the same model_version.
MODEL_VERSION = f"{RECOGNIZER_VERSION}+{DETECTOR_VERSION}"
EMBEDDING_DIM = 128

# Detection runs on a copy scaled so the long side is at most this. YuNet is
# trained on faces of roughly 10-300 px, so a close-up enrolment photo is
# detected more reliably scaled down; CCTV frames are rarely above this.
DETECT_MAX_SIDE_ENROL = 640
DETECT_MAX_SIDE_FRAME = 1280


@dataclass
class QualityPolicy:
    """Enrolment photo acceptance rules. A rejected photo gets a reason."""

    min_detection_score: float = 0.80
    min_face_px: int = 80           # shorter side of the face box in the original photo
    min_sharpness: float = 40.0     # variance of the Laplacian on the aligned 112x112 crop
    max_yaw_ratio: float = 0.30     # nose offset from the eye midline / inter-ocular distance
    max_pitch_ratio: float = 0.55   # |eyes-to-nose vs nose-to-mouth| imbalance
    min_brightness: float = 45.0
    max_brightness: float = 215.0
    # A second face larger than this fraction of the main face makes the
    # photo ambiguous: whose face is being enrolled?
    second_face_area_ratio: float = 0.20


@dataclass
class FrameFaceRules:
    """Faces found in footage are matched only if they clear these; smaller or
    blurrier faces are counted but not compared, because SFace similarity on
    them is unreliable."""

    min_detection_score: float = 0.70
    min_face_px: int = 36
    min_sharpness: float = 8.0


@dataclass
class Face:
    box: Tuple[float, float, float, float]  # x, y, w, h in original image pixels
    landmarks: List[Tuple[float, float]]    # right eye, left eye, nose, right mouth, left mouth
    score: float
    row: np.ndarray                         # YuNet row in original coordinates (15 floats)

    @property
    def area(self) -> float:
        return self.box[2] * self.box[3]


@dataclass
class Quality:
    detection_score: float
    face_px: int
    sharpness: float
    brightness: float
    yaw_ratio: float
    pitch_ratio: float
    roll_deg: float
    score: float  # 0-1 composite, for display and ranking only
    issues: List[str] = field(default_factory=list)

    def as_dict(self) -> Dict:
        return {
            "detectionScore": round(self.detection_score, 4),
            "facePx": self.face_px,
            "sharpness": round(self.sharpness, 1),
            "brightness": round(self.brightness, 1),
            "yawRatio": round(self.yaw_ratio, 3),
            "pitchRatio": round(self.pitch_ratio, 3),
            "rollDeg": round(self.roll_deg, 1),
            "score": round(self.score, 3),
            "issues": list(self.issues),
        }


def sha256_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


class ModelError(RuntimeError):
    pass


class FaceEngine:
    """Thread-safe wrapper. OpenCV DNN nets are not safe to share between
    threads, so inference is serialised with a lock; run several worker
    processes for throughput."""

    def __init__(self, model_dir: str, verify_hashes: bool = True):
        self.model_dir = model_dir
        det_path = os.path.join(model_dir, DETECTOR_FILE)
        rec_path = os.path.join(model_dir, RECOGNIZER_FILE)
        self.hashes: Dict[str, str] = {}
        for name, path in ((DETECTOR_FILE, det_path), (RECOGNIZER_FILE, rec_path)):
            if not os.path.isfile(path):
                raise ModelError(f"model file missing: {path} (run download_models.sh)")
            digest = sha256_file(path)
            if verify_hashes and digest != EXPECTED_SHA256[name]:
                raise ModelError(f"model file {name} has SHA-256 {digest}, expected {EXPECTED_SHA256[name]}")
            self.hashes[name] = digest
        self._detector = cv2.FaceDetectorYN.create(det_path, "", (320, 320), 0.6, 0.3, 5000)
        self._recognizer = cv2.FaceRecognizerSF.create(rec_path, "")
        self._lock = threading.Lock()

    # ------------------------------------------------------------------ info
    def describe(self) -> Dict:
        return {
            "modelVersion": MODEL_VERSION,
            "embeddingDim": EMBEDDING_DIM,
            "opencv": cv2.__version__,
            "detector": {
                "name": "YuNet", "version": DETECTOR_VERSION, "file": DETECTOR_FILE,
                "sha256": self.hashes[DETECTOR_FILE], "licence": "MIT",
                "source": "https://github.com/opencv/opencv_zoo/tree/main/models/face_detection_yunet",
            },
            "recognizer": {
                "name": "SFace", "version": RECOGNIZER_VERSION, "file": RECOGNIZER_FILE,
                "sha256": self.hashes[RECOGNIZER_FILE], "licence": "Apache-2.0",
                "source": "https://github.com/opencv/opencv_zoo/tree/main/models/face_recognition_sface",
            },
        }

    # ------------------------------------------------------------- detection
    def detect(self, image: np.ndarray, max_side: int = DETECT_MAX_SIDE_FRAME, min_score: float = 0.6) -> List[Face]:
        h, w = image.shape[:2]
        scale = min(1.0, float(max_side) / max(h, w))
        work = image if scale == 1.0 else cv2.resize(image, (int(round(w * scale)), int(round(h * scale))), interpolation=cv2.INTER_AREA)
        with self._lock:
            self._detector.setInputSize((work.shape[1], work.shape[0]))
            self._detector.setScoreThreshold(min_score)
            _, rows = self._detector.detect(work)
        faces: List[Face] = []
        if rows is None:
            return faces
        for row in rows:
            orig = row.astype(np.float32).copy()
            orig[:14] = orig[:14] / scale
            x, y, bw, bh = (float(v) for v in orig[:4])
            # Clip the box to the image so crops and stored boxes are valid.
            x0, y0 = max(0.0, x), max(0.0, y)
            x1, y1 = min(float(w), x + bw), min(float(h), y + bh)
            if x1 - x0 < 2 or y1 - y0 < 2:
                continue
            lm = [(float(orig[4 + 2 * i]), float(orig[5 + 2 * i])) for i in range(5)]
            faces.append(Face(box=(x0, y0, x1 - x0, y1 - y0), landmarks=lm, score=float(orig[14]), row=orig))
        faces.sort(key=lambda f: f.area, reverse=True)
        return faces

    # ------------------------------------------------------ embedding/quality
    def align(self, image: np.ndarray, face: Face) -> np.ndarray:
        with self._lock:
            return self._recognizer.alignCrop(image, face.row)

    def embed_aligned(self, aligned: np.ndarray) -> np.ndarray:
        with self._lock:
            feat = self._recognizer.feature(aligned)
        vec = feat.reshape(-1).astype(np.float32)
        norm = float(np.linalg.norm(vec))
        if norm == 0:
            raise ValueError("zero embedding")
        return vec / norm

    @staticmethod
    def quality(face: Face, aligned: np.ndarray) -> Quality:
        gray = cv2.cvtColor(aligned, cv2.COLOR_BGR2GRAY)
        sharpness = float(cv2.Laplacian(gray, cv2.CV_64F).var())
        brightness = float(gray[20:92, 20:92].mean())
        (rex, rey), (lex, ley), (nx, ny), (rmx, rmy), (lmx, lmy) = face.landmarks
        eye_mid = ((rex + lex) / 2, (rey + ley) / 2)
        mouth_mid = ((rmx + lmx) / 2, (rmy + lmy) / 2)
        iod = math.hypot(lex - rex, ley - rey) or 1.0
        roll = math.degrees(math.atan2(ley - rey, lex - rex))
        # Yaw: project the nose onto the eye line; a frontal face has the nose
        # at the eye midpoint.
        ux, uy = (lex - rex) / iod, (ley - rey) / iod
        yaw_ratio = ((nx - eye_mid[0]) * ux + (ny - eye_mid[1]) * uy) / iod
        # Pitch: on a frontal face the nose sits roughly midway (slightly
        # lower) between the eye line and the mouth.
        vx, vy = -uy, ux
        d_eye_nose = (nx - eye_mid[0]) * vx + (ny - eye_mid[1]) * vy
        d_nose_mouth = (mouth_mid[0] - nx) * vx + (mouth_mid[1] - ny) * vy
        total = abs(d_eye_nose) + abs(d_nose_mouth) or 1.0
        pitch_ratio = (d_eye_nose - d_nose_mouth) / total  # ~0.2 frontal; +/-1 extreme
        face_px = int(min(face.box[2], face.box[3]))

        def clamp01(v: float) -> float:
            return max(0.0, min(1.0, v))

        score = (
            0.30 * clamp01((face_px - 24) / (112 - 24))
            + 0.25 * clamp01(sharpness / 150.0)
            + 0.20 * clamp01(1 - abs(yaw_ratio) / 0.5)
            + 0.10 * clamp01(1 - abs(pitch_ratio - 0.2) / 0.8)
            + 0.15 * clamp01(face.score)
        )
        return Quality(face.score, face_px, sharpness, brightness, float(yaw_ratio), float(pitch_ratio), float(roll), float(score))

    # --------------------------------------------------------------- enrol
    def enrol(self, image: np.ndarray, policy: Optional[QualityPolicy] = None) -> Dict:
        """Assess a photo for enrolment. Returns accepted/reason, and the
        embedding only when accepted."""
        policy = policy or QualityPolicy()
        faces = self.detect(image, max_side=DETECT_MAX_SIDE_ENROL, min_score=0.5)
        if not faces:
            return {"accepted": False, "reason": "NO_FACE", "message": "No face was found in the photo."}
        main = faces[0]
        if len(faces) > 1 and faces[1].score >= 0.7 and faces[1].area >= policy.second_face_area_ratio * main.area:
            return {"accepted": False, "reason": "MULTIPLE_FACES",
                    "message": f"{len(faces)} faces were found. Crop the photo so only the missing person's face is in it.",
                    "facesFound": len(faces)}
        aligned = self.align(image, main)
        q = self.quality(main, aligned)
        checks = [
            (q.detection_score < policy.min_detection_score, "LOW_CONFIDENCE",
             f"The face is not clearly a face to the detector (score {q.detection_score:.2f}, needs {policy.min_detection_score:.2f})."),
            (q.face_px < policy.min_face_px, "FACE_TOO_SMALL",
             f"The face is {q.face_px} px across; at least {policy.min_face_px} px is needed. Use a closer or higher-resolution photo."),
            (q.sharpness < policy.min_sharpness, "BLURRED",
             f"The face is blurred (sharpness {q.sharpness:.0f}, needs {policy.min_sharpness:.0f})."),
            (abs(q.yaw_ratio) > policy.max_yaw_ratio, "POSE_TURNED",
             "The face is turned too far to the side. Use a photo looking towards the camera."),
            (abs(q.pitch_ratio - 0.2) > policy.max_pitch_ratio, "POSE_TILTED",
             "The face is tilted too far up or down. Use a photo looking towards the camera."),
            (q.brightness < policy.min_brightness, "TOO_DARK", "The face is too dark to use."),
            (q.brightness > policy.max_brightness, "OVEREXPOSED", "The face is overexposed."),
        ]
        for failed, code, _ in checks:
            if failed:
                q.issues.append(code)
        base = {"quality": q.as_dict(), "face": face_dict(main), "modelVersion": MODEL_VERSION, "facesFound": len(faces)}
        for failed, code, message in checks:
            if failed:
                return {"accepted": False, "reason": code, "message": message, **base}
        emb = self.embed_aligned(aligned)
        return {"accepted": True, "embedding": emb, "alignedFace": aligned, **base}

    # --------------------------------------------------------------- frames
    def faces_in_frame(self, frame: np.ndarray, rules: Optional[FrameFaceRules] = None) -> Tuple[List[Dict], int]:
        """Detect and embed every usable face. Returns (usable faces, faces seen)."""
        rules = rules or FrameFaceRules()
        faces = self.detect(frame, max_side=DETECT_MAX_SIDE_FRAME, min_score=min(0.6, rules.min_detection_score))
        out = []
        for f in faces:
            if f.score < rules.min_detection_score or min(f.box[2], f.box[3]) < rules.min_face_px:
                continue
            aligned = self.align(frame, f)
            q = self.quality(f, aligned)
            if q.sharpness < rules.min_sharpness:
                continue
            out.append({"face": f, "quality": q, "embedding": self.embed_aligned(aligned)})
        return out, len(faces)


def face_dict(f: Face) -> Dict:
    x, y, w, h = f.box
    return {"box": {"x": round(x), "y": round(y), "w": round(w), "h": round(h)},
            "landmarks": [[round(a, 1), round(b, 1)] for a, b in f.landmarks],
            "detectionScore": round(f.score, 4)}


def cosine_to_similarity(cos: float) -> float:
    """SFace scores are cosine similarities in [-1, 1]; unrelated faces sit
    near 0. The platform reports similarity on 0-1 by clamping negatives to 0
    (no rescaling), so the number shown is the raw cosine for any value that
    can matter."""
    return float(max(0.0, min(1.0, cos)))


@dataclass
class GalleryEntry:
    id: str
    embedding: np.ndarray


def match(embedding: np.ndarray, gallery_ids: Sequence[str], gallery: np.ndarray, threshold: float) -> List[Tuple[str, float]]:
    """Brute-force cosine against L2-normalised gallery rows."""
    if gallery.size == 0:
        return []
    sims = gallery @ embedding
    idx = np.nonzero(sims >= threshold)[0]
    return sorted(((gallery_ids[i], cosine_to_similarity(float(sims[i]))) for i in idx), key=lambda t: -t[1])


def crop_with_margin(image: np.ndarray, box: Dict, margin: float = 0.35) -> np.ndarray:
    h, w = image.shape[:2]
    x, y, bw, bh = box["x"], box["y"], box["w"], box["h"]
    mx, my = bw * margin, bh * margin
    x0, y0 = int(max(0, x - mx)), int(max(0, y - my))
    x1, y1 = int(min(w, x + bw + mx)), int(min(h, y + bh + my))
    return image[y0:y1, x0:x1]
