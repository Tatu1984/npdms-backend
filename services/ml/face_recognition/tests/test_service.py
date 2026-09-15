"""Unit checks for the face recognition service.

Synthetic faces only. Point FR_TEST_FACES at a folder of synthetic face images
(for example SFHQ part 1, MIT licence; see MODELS.md). Tests that need faces
are skipped when it is not set. The images are never committed.

    FR_TEST_FACES=/path/to/synthetic python -m pytest tests -q
"""

from __future__ import annotations

import glob
import json
import os
import random
import sys
import tempfile

import cv2
import numpy as np
import pytest

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, ROOT)

from fr_service.engine import MODEL_VERSION, FaceEngine  # noqa: E402
from tools.evaluate import degrade  # noqa: E402

MODEL_DIR = os.environ.get("FR_MODEL_DIR", os.path.join(ROOT, "models"))
FACES = os.environ.get("FR_TEST_FACES")
THRESHOLD = 0.50  # the platform default; see MODELS.md for how it was chosen

needs_faces = pytest.mark.skipif(not FACES, reason="set FR_TEST_FACES to a folder of synthetic faces")


@pytest.fixture(scope="module")
def engine():
    return FaceEngine(MODEL_DIR)


@pytest.fixture(scope="module")
def enrolled(engine):
    files = sorted(glob.glob(os.path.join(FACES or "", "*.jpg")))[:60]
    out = []
    for f in files:
        img = cv2.imread(f)
        r = engine.enrol(img)
        if r["accepted"]:
            out.append((img, r["embedding"]))
    assert len(out) >= 30, "need at least 30 enrolable synthetic faces"
    return out


def test_model_hashes_verified(engine):
    info = engine.describe()
    assert info["modelVersion"] == MODEL_VERSION
    assert info["detector"]["licence"] == "MIT"
    assert info["recognizer"]["licence"] == "Apache-2.0"


def test_no_face_rejected(engine):
    noise = np.random.default_rng(1).integers(0, 255, (480, 640, 3), dtype=np.uint8)
    r = engine.enrol(noise)
    assert not r["accepted"] and r["reason"] == "NO_FACE"


@needs_faces
def test_same_identity_above_different_below(enrolled, engine):
    rng = random.Random(11)
    gallery = np.stack([e for _, e in enrolled])
    genuine_ok = impostor_bad = impostor_total = 0
    for i, (img, _) in enumerate(enrolled):
        probe = degrade(img, rng, 80)
        faces, _ = engine.faces_in_frame(probe)
        if not faces:  # a missed detection counts against the genuine rate
            continue
        sims = gallery @ faces[0]["embedding"]
        genuine_ok += int(sims[i] >= THRESHOLD)
        others = np.delete(sims, i)
        impostor_bad += int((others >= THRESHOLD).sum())
        impostor_total += others.size
    assert genuine_ok / len(enrolled) >= 0.95
    assert impostor_bad / impostor_total <= 0.005


@needs_faces
def test_blurred_photo_rejected(enrolled, engine):
    img = enrolled[0][0]
    blurred = cv2.GaussianBlur(img, (0, 0), 6)
    r = engine.enrol(blurred)
    assert not r["accepted"]
    assert r["reason"] in ("BLURRED", "NO_FACE", "LOW_CONFIDENCE")


@needs_faces
def test_small_face_rejected(enrolled, engine):
    img = enrolled[0][0]
    small = cv2.resize(img, (90, 90), interpolation=cv2.INTER_AREA)
    canvas = np.full((400, 400, 3), 60, np.uint8)
    canvas[150:240, 150:240] = small
    r = engine.enrol(canvas)
    assert not r["accepted"] and r["reason"] in ("FACE_TOO_SMALL", "NO_FACE")


@needs_faces
def test_two_faces_rejected(enrolled, engine):
    a, b = enrolled[0][0], enrolled[1][0]
    both = np.hstack([a, b])
    r = engine.enrol(both)
    assert not r["accepted"] and r["reason"] == "MULTIPLE_FACES"


@needs_faces
def test_match_endpoint_on_video(enrolled):
    from fastapi.testclient import TestClient
    from fr_service.app import app
    from tools import make_test_clip

    files = sorted(glob.glob(os.path.join(FACES, "*.jpg")))
    with tempfile.TemporaryDirectory() as tmp:
        clip = os.path.join(tmp, "clip.mp4")
        target, o1, o2 = enrolled[2][0], enrolled[3][0], enrolled[4][0]
        paths = []
        for n, im in (("t", target), ("a", o1), ("b", o2)):
            p = os.path.join(tmp, n + ".jpg")
            cv2.imwrite(p, im)
            paths.append(p)
        sys.argv = ["make_test_clip", "--face", paths[0], "--others", paths[1], paths[2], "--out", clip, "--seconds", "9"]
        make_test_clip.main()
        client = TestClient(app)
        emb = enrolled[2][1].astype("<f4").tobytes()
        with open(clip, "rb") as fh:
            resp = client.post("/v1/match", data={
                "gallery_ids": json.dumps(["enrolment-target"]), "gallery_groups": json.dumps(["report-1"]),
                "threshold": str(THRESHOLD), "model_version": MODEL_VERSION, "sample_fps": "1",
            }, files={"file": ("clip.mp4", fh, "video/mp4"), "gallery": ("g.bin", emb, "application/octet-stream")})
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["framesAnalysed"] >= 8
        assert body["candidates"], "target face not found in the clip"
        c = body["candidates"][0]
        assert c["galleryId"] == "enrolment-target" and c["similarity"] >= THRESHOLD
        # The target is on screen only in the middle third (3 s - 6 s).
        assert all(2500 <= x["frameOffsetMs"] <= 6500 for x in body["candidates"])
        assert c["frame"]["sha256"] and c["crop"]["jpeg"]
    assert files
