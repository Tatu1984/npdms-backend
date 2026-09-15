"""Render synthetic West Bengal number plates for verification.

Nothing here is a real registration: numbers are drawn at random from the
formats in plates.py. Used to measure OCR accuracy, never shipped as data.

    python eval/synth_plates.py OUT_DIR [--count 200] [--seed 7]

Writes OUT_DIR/plates/*.png (plate crops, with degradations) and
OUT_DIR/labels.json ([{file, plate, style, condition}]).
"""

from __future__ import annotations

import argparse
import json
import os
import random
from pathlib import Path

import cv2
import numpy as np
from PIL import Image, ImageDraw, ImageFont

FONT_CANDIDATES = [
    "/System/Library/Fonts/Supplemental/DIN Condensed Bold.ttf",
    "/System/Library/Fonts/Supplemental/Arial Narrow Bold.ttf",
    "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
    "/System/Library/Fonts/Supplemental/Impact.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSansCondensed-Bold.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
]
LETTERS = "ABCDEFGHJKLMNPRSTUVWXYZ"  # I and O are not issued in series letters


def fonts() -> list[str]:
    found = [f for f in FONT_CANDIDATES if os.path.exists(f)]
    if not found:
        raise SystemExit("no bold TrueType font found; install DejaVu fonts")
    return found


def random_plate(rng: random.Random) -> tuple[str, list[str]]:
    """Returns the normalised number and its display groups."""
    kind = rng.choices(["STANDARD", "BH", "OLD"], weights=[85, 8, 7])[0]
    if kind == "BH":
        g = [f"{rng.randint(21, 26)}", "BH", f"{rng.randint(0, 9999):04d}", "".join(rng.choices(LETTERS, k=rng.randint(1, 2)))]
    elif kind == "OLD":
        g = ["W" + rng.choice("BM") + rng.choice("ABCDEFGH"), f"{rng.randint(1, 9999)}"]
    else:
        g = ["WB", f"{rng.randint(1, 26):02d}", "".join(rng.choices(LETTERS, k=rng.choice([1, 2, 2, 2]))), f"{rng.randint(1, 9999):04d}"]
    return "".join(g), g


def render(groups: list[str], style: str, font_path: str, rng: random.Random) -> np.ndarray:
    two_line = style == "two_line"
    bg = (255, 214, 0) if style == "commercial" else (245, 245, 240)
    fg = (15, 15, 15)
    if two_line:
        w, h = 400, 240
    else:
        w, h = 1040, 220
    img = Image.new("RGB", (w, h), bg)
    d = ImageDraw.Draw(img)
    d.rectangle([4, 4, w - 5, h - 5], outline=fg, width=6)
    left = 20
    if style in ("hsrp", "commercial", "two_line") and rng.random() < 0.8:
        # IND strip: a blue band with the letters IND, as on high-security plates.
        strip_w = 60 if not two_line else 46
        d.rectangle([10, 10, 10 + strip_w, h - 11], fill=(20, 60, 160))
        small = ImageFont.truetype(font_path, 34 if not two_line else 26)
        d.text((10 + strip_w / 2, h - 50), "IND", font=small, fill=(255, 255, 255), anchor="mm")
        left = 20 + strip_w
    if two_line:
        split = 2 if len(groups) != 2 else 1
        lines = [" ".join(groups[:split]), " ".join(groups[split:])]
        font = ImageFont.truetype(font_path, 100)
        for i, text in enumerate(lines):
            d.text(((left + w) / 2, 70 + i * 105), text, font=font, fill=fg, anchor="mm")
    else:
        text = (" " if rng.random() < 0.7 else "-").join(groups) if style != "old" else " ".join(groups)
        size = 170
        font = ImageFont.truetype(font_path, size)
        while d.textlength(text, font=font) > (w - left - 30) and size > 60:
            size -= 6
            font = ImageFont.truetype(font_path, size)
        d.text(((left + w) / 2, h / 2 + 4), text, font=font, fill=fg, anchor="mm")
    return cv2.cvtColor(np.array(img), cv2.COLOR_RGB2BGR)


def degrade(img: np.ndarray, condition: str, rng: random.Random) -> np.ndarray:
    h, w = img.shape[:2]
    out = img.copy()
    if condition in ("angled", "night", "rain", "blur", "small"):
        # Mild perspective for everything but the clean case.
        k = 0.18 if condition == "angled" else 0.05
        dx, dy = w * k * rng.uniform(0.5, 1), h * k * rng.uniform(0.3, 1)
        src = np.float32([[0, 0], [w, 0], [w, h], [0, h]])
        dst = np.float32([[rng.uniform(0, dx), rng.uniform(0, dy)], [w - rng.uniform(0, dx), rng.uniform(0, dy)],
                          [w - rng.uniform(0, dx), h - rng.uniform(0, dy)], [rng.uniform(0, dx), h - rng.uniform(0, dy)]])
        out = cv2.warpPerspective(out, cv2.getPerspectiveTransform(src, dst), (w, h), borderValue=(90, 90, 90))
    if condition == "blur":
        n = rng.choice([9, 13, 17])
        kernel = np.zeros((n, n), np.float32)
        kernel[n // 2, :] = 1.0 / n
        out = cv2.filter2D(out, -1, kernel)
    if condition == "night":
        out = (out.astype(np.float32) * rng.uniform(0.18, 0.3)).clip(0, 255)
        out = (out + np.random.default_rng(rng.randint(0, 1 << 30)).normal(0, 9, out.shape)).clip(0, 255).astype(np.uint8)
    if condition == "rain":
        streaks = np.zeros_like(out)
        r2 = np.random.default_rng(rng.randint(0, 1 << 30))
        for _ in range(160):
            x, y = int(r2.integers(0, w)), int(r2.integers(0, h))
            cv2.line(streaks, (x, y), (x + 6, y + 30), (200, 200, 200), 2)
        out = cv2.addWeighted(out, 0.8, cv2.GaussianBlur(streaks, (5, 5), 0), 0.5, 0)
        out = cv2.GaussianBlur(out, (5, 5), 0)
    if condition == "small":
        scale = rng.uniform(0.09, 0.13)
        small = cv2.resize(out, (max(8, int(w * scale)), max(8, int(h * scale))), interpolation=cv2.INTER_AREA)
        out = small
    q = rng.randint(35, 80)
    ok, buf = cv2.imencode(".jpg", out, [cv2.IMWRITE_JPEG_QUALITY, q])
    return cv2.imdecode(buf, cv2.IMREAD_COLOR)


CONDITIONS = ["clean", "angled", "night", "rain", "blur", "small"]
STYLES = ["hsrp", "hsrp", "commercial", "two_line", "old"]


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("out")
    ap.add_argument("--count", type=int, default=180)
    ap.add_argument("--seed", type=int, default=7)
    args = ap.parse_args()
    rng = random.Random(args.seed)
    out = Path(args.out)
    (out / "plates").mkdir(parents=True, exist_ok=True)
    font_list = fonts()
    labels = []
    for i in range(args.count):
        plate, groups = random_plate(rng)
        style = rng.choice(STYLES)
        condition = CONDITIONS[i % len(CONDITIONS)]
        img = degrade(render(groups, style, rng.choice(font_list), rng), condition, rng)
        name = f"plate_{i:04d}.png"
        cv2.imwrite(str(out / "plates" / name), img)
        labels.append({"file": name, "plate": plate, "style": style, "condition": condition})
    (out / "labels.json").write_text(json.dumps(labels, indent=1))
    print(f"wrote {len(labels)} plates to {out}")


if __name__ == "__main__":
    main()
