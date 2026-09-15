# Face recognition models

The service uses two models from the OpenCV Zoo, run on the CPU through OpenCV's DNN module (`opencv-python-headless` 4.10.0.84). No cloud API is called and nothing is downloaded at run time. The model files are not committed; `download_models.sh` fetches them from a pinned OpenCV Zoo commit and checks their SHA-256. The service refuses to start if a file does not match.

| Role | Model | File | SHA-256 | Licence |
|---|---|---|---|---|
| Face detection and 5 landmarks | YuNet (2023mar) | `face_detection_yunet_2023mar.onnx` (232,589 bytes) | `8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4` | MIT (Shiqi Yu). `models/face_detection_yunet/LICENSE` in OpenCV Zoo |
| Face embedding, 128-d | SFace (2021dec, MobileFaceNet backbone) | `face_recognition_sface_2021dec.onnx` (38,696,353 bytes) | `0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79` | Apache-2.0. `models/face_recognition_sface/LICENSE` in OpenCV Zoo |

- Source: OpenCV Zoo, commit `47534e27c9851bb1128ccc0102f1145e27f23f98`. The licence files were read at that commit on 2026-09-15.
- Version recorded on every enrolment and candidate: `sface-2021dec+yunet-2023mar`. Embeddings from a different recogniser are not comparable. The API matches only enrolments made with the version the service reports, and flags older enrolments for re-enrolment.
- Not used: InsightFace / buffalo (non-commercial weights), dlib's `face_recognition` package, and any AGPL detector.

## Licence caveat to put to legal review

The weights are Apache-2.0. SFace's upstream repository (`zhongyy/SFace`) and its paper describe training on CASIA-WebFace, VGGFace2 and MS-Celeb-1M. The OpenCV Zoo does not say which of these the published ONNX file was trained on. Those datasets were collected from the web, and MS-Celeb-1M was withdrawn by its publisher. A permissive licence on the weights does not settle questions about the training data. Kolkata Police's counsel should consider this, as for any face model trained on public web images, before live use.

## Pipeline

1. **Detect.** YuNet runs on a copy scaled so the long side is at most 640 px for enrolment photos and 1280 px for footage frames. Box and landmarks are mapped back to the original image.
2. **Align.** `FaceRecognizerSF.alignCrop` does a five-point similarity warp to 112×112.
3. **Embed.** SFace produces 128 floats, which are L2-normalised.
4. **Assess quality.** The service measures detection score, face size (shorter side of the box), sharpness (variance of the Laplacian on the aligned crop), brightness, yaw (nose offset from the eye midline over the inter-ocular distance), pitch (eye–nose against nose–mouth distance) and roll.
   - An enrolment photo is rejected with a reason for any of: no face; more than one sizeable face; detection score below 0.80; face under 80 px; sharpness under 40; yaw ratio over 0.30; tilted; too dark; overexposed.
   - A face in footage is compared only if its detection score is at least 0.70, it is at least 36 px, and its sharpness is at least 8. Smaller or blurrier faces are counted but not compared.
5. **Match.** Brute-force cosine similarity against the gallery the API sends: enrolled faces of open reports only, demo and real kept apart. Similarity is reported on 0–1 as the raw cosine with negatives clamped to 0. For each report, hits within 5 seconds of each other are merged and the best one is kept. The service returns the frame and a face crop as JPEG with their SHA-256.
6. **Video.** OpenCV decodes the footage, with FFmpeg bundled in the wheel. Frames are sampled by timestamp at 1 frame per second by default (configurable from 0.2 to 2).

The service is stateless. The temporary video file is deleted before the response is sent.

## Measured on synthetic faces

- **Set.** 531 images from SFHQ part 1 ("Synthetic Faces High Quality", MIT licence, David Beniaguev), taken from a 512 px mirror of one validation shard. Part 1's inspiration images are artworks, 3D-rendered humans and avatars, not photographs of real people. The images were used locally for measurement only and are not committed or seeded.
- **Same-identity pairs.** Each synthetic identity has one image, so a "same identity" probe is the image degraded the way CCTV degrades a face:
  - rotation up to ±12° and a horizontal squeeze (a crude stand-in for yaw);
  - brightness and contrast change;
  - downscaling to a face of 112, 64, 48 or 36 px;
  - blur, sensor noise and JPEG quality 35–70;
  - placement on a wide dark frame.
- **Different-identity pairs.** Every probe against every other enrolled identity.
- **Reproduce.** `python -m tools.evaluate --faces <dir> --limit 531 --probes 3 --resolutions 112,64,48,36`.

Enrolment accepted 499 of 531 photos. 31 were rejected as turned away and 1 as too dark. Enrolment takes 11 ms per photo on an Apple M4 CPU.

Different identities, enrolment photo against enrolment photo (124,251 pairs):

| Mean | p95 | p99 | p99.9 | Max |
|---|---|---|---|---|
| 0.146 | 0.334 | 0.420 | 0.522 | 0.844 |

Probes, 3 per identity at each face size (1,497 per size). Rates are over faces that were detected and passed the frame rules. "False per probe" is the number of wrong candidates each probe raises against the 499-face gallery.

| Face size | Not compared | Genuine mean / p1 / p5 | Impostor p99 / p99.9 / max | At 0.45: TMR / FMR / false per probe | At 0.50: TMR / FMR / false per probe | At 0.55: TMR / FMR / false per probe |
|---|---|---|---|---|---|---|
| 112 px | 0 | 0.858 / 0.662 / 0.755 | 0.393 / 0.491 / 0.831 | 1.000 / 0.27% / 1.34 | 0.999 / 0.082% / 0.41 | 0.997 / 0.027% / 0.13 |
| 64 px | 2 | 0.789 / 0.582 / 0.669 | 0.377 / 0.471 / 0.767 | 0.997 / 0.17% / 0.86 | 0.996 / 0.049% / 0.24 | 0.993 / 0.017% / 0.09 |
| 48 px | 24 | 0.719 / 0.468 / 0.560 | 0.359 / 0.451 / 0.791 | 0.991 / 0.10% / 0.51 | 0.980 / 0.030% / 0.15 | 0.959 / 0.012% / 0.06 |
| 36 px | 1,325 | 0.644 / 0.413 / 0.476 | 0.343 / 0.442 / 0.708 | 0.971 / 0.09% / 0.43 | 0.924 / 0.019% / 0.09 | 0.831 / 0.004% / 0.02 |

**Chosen default threshold: 0.50.** OpenCV's published SFace cosine threshold of 0.363 raises 3 to 9 wrong candidates per face against a gallery of 500 on this set, which would bury reviewers. At 0.50, a face of 48 px or larger is found about 98% of the time. Wrong candidates run at 0.15–0.4 per face per 500 enrolled photos, and they grow in proportion to the gallery. The threshold is configurable from 0.30 to 0.95 in Settings. The value used is stored and shown on every candidate.

Frame processing takes about 10–16 ms for a 320–720 px frame with one face on an Apple M4, single-threaded through a lock. See the plan for server sizing.

## What this does not measure

- **Real same-person variation.** Different days, clothes, hairstyles and expressions, and the age gap between a family photo taken years ago and today. Degraded copies of one image are far easier than this, so the genuine scores above are an upper bound. Expect lower true-match rates in real use, especially for children, whose faces change fastest.
- **Real CCTV.** Top-down angles, motion blur, rolling shutter, compression artefacts, night-time IR (greyscale) footage, backlight, masks, helmets, dupattas and caps. None of these are in the synthetic set.
- **Demographics.** No measurement was made across skin tones, ages or sexes. The synthetic set is not representative of Kolkata's population. Error rates may differ between groups, and this must be measured on authorised, representative data before live use.
- **Tail behaviour of the synthetic set.** Some synthetic identities are near-duplicates (maximum impostor score 0.84), which inflates the impostor tail. Real populations include look-alikes and relatives too.
- **Speed at scale.** Measured on a laptop CPU with one worker.

Before enabling for real reports, measure on labelled, authorised Kolkata footage as the platform's ground rules require.
