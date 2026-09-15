"""Indian registration-number normalisation and validation.

The normalised form is the Phase 06 rule used by the API
(repository.normaliseRegistration): upper case, letters and digits only, so
"WB-06-AX-3304", "wb 06 ax 3304" and "WB06AX3304" are the same vehicle.

On top of that rule this module recognises the formats that appear on West
Bengal roads and says which one a read matches:

  STANDARD  SS DD L{0,3} N{1,4}   WB 06 AX 3304, WB 02 A 12, DL 3C AB 1234
  BH        YY BH NNNN L{1,2}     22 BH 1234 AA   (Bharat series, 2021 onward)
  OLD       WLL N{1,4}            WMA 1234, WBC 1844 (pre-1989 Bengal three-letter series)

A read that matches none of them is kept as UNRECOGNISED. It is never
silently corrected into a valid-looking number.

OCR confuses a few glyph pairs (0/O, 1/I, 8/B, 5/S, 2/Z, 6/G). A correction is
applied only where the format fixes whether a position is a letter or a digit,
and only if the corrected string then matches that format. Every correction is
reported alongside the raw text. Because a correction can turn a shop sign
into something plate-shaped, a corrected read must also be plate-length: a
four-digit number for STANDARD and OLD, and no more than one correction for
every four characters.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

# State and union-territory codes issued under the Motor Vehicles Act.
STATE_CODES = {
    "AN", "AP", "AR", "AS", "BR", "CG", "CH", "DD", "DL", "DN", "GA", "GJ", "HP", "HR", "JH", "JK",
    "KA", "KL", "LA", "LD", "MH", "ML", "MN", "MP", "MZ", "NL", "OD", "OR", "PB", "PY", "RJ", "SK",
    "TN", "TR", "TS", "TG", "UK", "UA", "UP", "WB",
}

# Kolkata Police's jurisdiction sees mostly the Kolkata-area RTO series.
WB_KOLKATA_AREA_RTO = {f"{n:02d}" for n in range(1, 27)}

TO_LETTER = {"0": "O", "1": "I", "2": "Z", "4": "A", "5": "S", "6": "G", "8": "B"}
TO_DIGIT = {"O": "0", "D": "0", "Q": "0", "U": "0", "I": "1", "L": "1", "T": "1", "Z": "2",
            "A": "4", "S": "5", "G": "6", "B": "8"}

STANDARD_RE = re.compile(r"^([A-Z]{2})(\d{2}|\d[A-Z])([A-Z]{0,3})(\d{1,4})$")
BH_RE = re.compile(r"^(\d{2})BH(\d{4})([A-Z]{1,2})$")
OLD_RE = re.compile(r"^([A-Z]{3})(\d{1,4})$")


def normalise(text: str) -> str:
    """Phase 06 rule: upper case, A–Z and 0–9 only."""
    return "".join(ch for ch in (text or "").upper() if ("A" <= ch <= "Z") or ("0" <= ch <= "9"))


@dataclass
class PlateValidation:
    normalised: str
    format: str  # STANDARD, BH, OLD, UNRECOGNISED
    valid: bool
    state_code: str | None = None
    rto_code: str | None = None
    kolkata_area_series: bool = False
    corrections: list[str] = field(default_factory=list)
    display: str = ""

    def as_dict(self) -> dict:
        return {
            "normalised": self.normalised,
            "display": self.display or self.normalised,
            "format": self.format,
            "valid": self.valid,
            "stateCode": self.state_code,
            "rtoCode": self.rto_code,
            "kolkataAreaSeries": self.kolkata_area_series,
            "corrections": self.corrections,
        }


def _match(s: str, corrected: bool = False) -> PlateValidation | None:
    m = STANDARD_RE.match(s)
    if m and m.group(1) in STATE_CODES:
        state, rto, series, number = m.groups()
        # A zero RTO code is not issued; a corrected read must carry a full number.
        if rto != "00" and not (corrected and len(number) < 4):
            parts = [state, rto] + ([series] if series else []) + [number]
            return PlateValidation(
                normalised=s, format="STANDARD", valid=True, state_code=state, rto_code=rto,
                kolkata_area_series=(state == "WB" and rto in WB_KOLKATA_AREA_RTO),
                display="-".join(parts),
            )
    m = BH_RE.match(s)
    if m:
        year, number, series = m.groups()
        return PlateValidation(normalised=s, format="BH", valid=True, display=f"{year}-BH-{number}-{series}")
    m = OLD_RE.match(s)
    # Only the Bengal old series (W..) is recognised; other states' old plates
    # are kept as UNRECOGNISED rather than guessed.
    if m and m.group(1)[0] == "W" and not (corrected and len(m.group(2)) < 4):
        return PlateValidation(normalised=s, format="OLD", valid=True, state_code="WB",
                               display=f"{m.group(1)}-{m.group(2)}")
    return None


# Letter/digit shape of each format, used to correct OCR confusions position by
# position. "L" letter, "D" digit, "?" either.
def _shapes(n: int) -> list[str]:
    shapes = []
    for series in range(0, 4):
        digits = n - 4 - series
        if 1 <= digits <= 4:
            shapes.append("LLDD" + "L" * series + "D" * digits)
            shapes.append("LLDL" + "L" * series + "D" * digits)  # DL 3C AB 1234
    if n in (9, 10):
        shapes.append("DDLLDDDD" + "L" * (n - 8))  # BH: YY BH NNNN L{1,2}
    if 4 <= n <= 7:
        shapes.append("LLL" + "D" * (n - 3))
    return shapes


def _coerce(s: str, shape: str) -> tuple[str, list[str]] | None:
    out, notes = [], []
    for i, (ch, kind) in enumerate(zip(s, shape)):
        if kind == "L" and ch.isdigit():
            if ch not in TO_LETTER:
                return None
            out.append(TO_LETTER[ch])
            notes.append(f"position {i + 1}: {ch}→{TO_LETTER[ch]}")
        elif kind == "D" and ch.isalpha():
            if ch not in TO_DIGIT:
                return None
            out.append(TO_DIGIT[ch])
            notes.append(f"position {i + 1}: {ch}→{TO_DIGIT[ch]}")
        else:
            out.append(ch)
    return "".join(out), notes


def validate(text: str) -> PlateValidation:
    s = normalise(text)
    # High-security plates carry "IND" beside the number; OCR often reads it in.
    candidates = [s]
    if s.startswith("IND") and len(s) > 7:
        candidates.append(s[3:])
    for cand in candidates:
        direct = _match(cand)
        if direct:
            if cand != s:
                direct.corrections.append("dropped the IND mark read with the number")
            return direct
    for cand in candidates:
        best = None
        for shape in _shapes(len(cand)):
            coerced = _coerce(cand, shape)
            if not coerced:
                continue
            fixed, notes = coerced
            if not notes or len(notes) > max(1, len(cand) // 4):
                continue
            v = _match(fixed, corrected=True)
            if v and (best is None or len(notes) < len(best.corrections)):
                v.corrections = (["dropped the IND mark read with the number"] if cand != s else []) + notes
                best = v
        if best:
            return best
    return PlateValidation(normalised=s, format="UNRECOGNISED", valid=False)
