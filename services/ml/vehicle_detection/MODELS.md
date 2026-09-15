# Vehicle detection and ANPR — models, licences, measured accuracy

The service runs three model files, all on CPU through ONNX Runtime. Each is
pinned by SHA-256 in `model_registry.py`; `fetch_models.py` downloads and
verifies them, and the service refuses to analyse if a file is missing or
differs. There is no mock or fallback output.

| Role | Model | Licence | Source | SHA-256 |
|---|---|---|---|---|
| Vehicle detection | YOLOX-s (COCO), Megvii release 0.1.1rc0, `yolox_s.onnx` | Apache-2.0 (repository licence, checked via the GitHub API) | github.com/Megvii-BaseDetection/YOLOX | `c5c2d13e…998063` |
| Plate text localisation | PaddleOCR PP-OCRv4 mobile detection, ONNX conversion shipped in `rapidocr_onnxruntime` 1.4.4 | Apache-2.0 (PaddleOCR and RapidOCR; PyPI metadata Apache-2.0) | github.com/PaddlePaddle/PaddleOCR, github.com/RapidAI/RapidOCR | `d2a7720d…f49da9` |
| Plate reading | PaddleOCR PP-OCRv4 mobile recognition, same wheel | Apache-2.0 | as above | `48fc40f2…83683b` |

Runtime libraries: ONNX Runtime (MIT), OpenCV headless (Apache-2.0), NumPy
(BSD-3-Clause), FastAPI (MIT), Uvicorn (BSD-3-Clause), python-multipart
(Apache-2.0). The wheel the PP-OCR files come from is itself pinned
(`971d7d5f…782cbf`); only the two model files are extracted from it.

Training data: COCO (for YOLOX) and PaddleOCR's text datasets contain images
under mixed licences. The weights are distributed under the licences above; a
legal review should confirm that this is acceptable for police use, as for any
pretrained model.

## Considered and not used

- **Ultralytics YOLOv5/v8/v11** (the prototype in `video_analysis`): AGPL-3.0,
  or a commercial licence. Not used.
- **open-image-models YOLOv9 plate detector** (published as MIT): the released
  `.pt` checkpoints pickle module paths `models.yolo` and
  `models.common.RepNCSPELAN4`, i.e. they were trained with
  WongKinYiu/yolov9, which is GPL-3.0. Weight provenance is unclear, so it is
  not used. Plates are instead localised as text with PP-OCRv4 detection and
  accepted only if the read matches an Indian registration format.
- **fast-plate-ocr** global models (MIT): per-character confidence out of the
  box, but no India region in training; measured 62% exact-match on the
  synthetic WB set against 94% for PP-OCRv4 recognition. Not used.
- **RF-DETR nano** (Apache-2.0): more accurate than YOLOX-s on COCO but a
  120 MB ONNX file; YOLOX-s fits a small edge server better. Worth revisiting
  on GPU hardware.

## What it detects and reads

- Vehicle classes: CAR, MOTORCYCLE, BUS, TRUCK, BICYCLE (the COCO classes).
- **No class** for auto-rickshaw, e-rickshaw, cycle-rickshaw, taxi or light
  commercial vehicle. Autos are usually reported as TRUCK or CAR, sometimes
  missed; a tram is reported as BUS. `/health` states this.
- **Colour is not estimated**: no colour model has been validated on Kolkata
  footage.
- Plates: STANDARD (`WB 06 AX 3304`, `WB 19 6695`, `DL 3C AB 1234`), BH
  (`22 BH 1234 AA`), OLD Bengal three-letter series (`WBC 1844`). Two-line
  plates are read by pairing stacked text regions. Normalisation is the Phase 06
  rule (upper case, letters and digits). OCR confusions (0/O, 1/I, 8/B…) are
  corrected only where the format fixes the character type, at most one per four
  characters, and a corrected read needs a full four-digit number; every
  correction is reported with the raw text. Other states' old series and
  temporary, diplomatic and army plates are not recognised.
- Each read carries a confidence per character (CTC probability, restricted to
  A–Z/0–9), the mean, the weakest character, and the model versions.

## Measured accuracy (15 September 2026, Apple M-series CPU, 2 threads)

Reproduce with `eval/synth_plates.py` and `eval/evaluate.py`.

**Plate reading on synthetic WB plates** (240 plates, seed 2026, held out from
the 180-plate set used while tuning): 234/240 = **97.5%** exact whole-plate
match. By condition: clean 97.5%, angled (±18% perspective) 100%, simulated
night 100%, rain streaks 97.5%, motion blur 97.5%, small (plate ~20 px high)
92.5%. By style: HSRP 99%, yellow commercial 100%, old painted 100%, two-line
87.5%. Median 25 ms per plate crop. Synthetic plates are cleaner than real ones;
treat these as an upper bound.

**Synthetic plates placed on vehicles in real Kolkata photographs** (198
scenes, full pipeline): **83.8%** of plates found and read exactly. Plate
width ≥150 px 96%, 90–150 px 88%, <90 px 70%. Simulated night 69%, angled 80%.
Median 377 ms per 1280 px frame including detection of every vehicle.

**Real plates in licensed photographs** (14 Wikimedia Commons photos, see
`eval/photo_attribution.json`): of 7 legible plates, 5 read correctly
(`WB32N4999`, `WB06H2055`, `WB02P7302`, `WBC1844`, `WB02AA7264`), 1 not read
(a jeep's `WB 12A 7051`), 1 bus plate could not be verified at photo
resolution. No false plate was reported across the 14 photos after the format
rules were tightened (shop signs such as TAXI and UBER were being accepted
before).

**Vehicle detection**, manual review of 8 photographs (`eval/detection_review.json`,
91 detections): precision (a vehicle is there) **97.8%**; vehicle and class
right **91.2%**; recall against vehicles the reviewer counted **78.1%**.
Errors: auto-rickshaws labelled TRUCK/CAR, a bus labelled TRUCK, one billboard
as TRUCK, one duplicate box. Median 46 ms per frame for detection alone.

## Limits to state to users

- Night, rain and glare: synthetic results overstate real performance; no real
  night footage was available with a usable licence.
- Small or distant plates (under ~90 px wide in the frame) are read about 70%
  of the time; junction cameras need to be placed and zoomed for plates.
- Motion blur beyond a short horizontal smear and strongly angled plates
  (>20°) degrade quickly.
- Autos, e-rickshaws and cycle-rickshaws — a large share of Kolkata traffic —
  have no class of their own.
- CPU speed: about 0.3–0.5 s per 1280 px frame with several vehicles, so one
  CPU core handles roughly two frames per second. Footage is sampled (default
  one frame per second).
- Exact matching only: a read with one wrong character does not match the
  watchlist. Fuzzy matching would raise more hits for operators to dismiss and
  is not enabled.
