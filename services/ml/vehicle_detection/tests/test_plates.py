import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import plates  # noqa: E402


class NormaliseTest(unittest.TestCase):
    def test_phase06_rule(self):
        # Same rule as repository.normaliseRegistration in the API.
        for text in ["WB-06-AX-3304", "wb 06 ax 3304", "WB06AX3304", " WB.06.AX.3304 "]:
            self.assertEqual(plates.normalise(text), "WB06AX3304")


class ValidateTest(unittest.TestCase):
    def check(self, text, normalised, fmt, corrections=0):
        v = plates.validate(text)
        self.assertEqual((v.normalised, v.format), (normalised, fmt), text)
        self.assertEqual(len(v.corrections), corrections, (text, v.corrections))
        return v

    def test_west_bengal_standard(self):
        v = self.check("WB 06 AX 3304", "WB06AX3304", "STANDARD")
        self.assertTrue(v.kolkata_area_series)
        self.assertEqual(v.display, "WB-06-AX-3304")
        self.check("WB-02-P-7302", "WB02P7302", "STANDARD")
        self.check("WB 19 6695", "WB196695", "STANDARD")
        self.assertFalse(plates.validate("WB 74 AB 1234").kolkata_area_series)

    def test_other_states_and_bh(self):
        self.check("DL 3C AB 1234", "DL3CAB1234", "STANDARD")
        self.check("22 BH 1234 AA", "22BH1234AA", "BH")

    def test_old_bengal_series(self):
        self.check("WBC 1844", "WBC1844", "OLD")
        self.check("WMA 12", "WMA12", "OLD")
        # Another state's old series is not guessed.
        self.check("MHA 1234", "MHA1234", "UNRECOGNISED")

    def test_ocr_confusions_corrected_by_position(self):
        v = self.check("WB O6 AX 33O4", "WB06AX3304", "STANDARD", corrections=2)
        self.assertIn("position 3: O→0", v.corrections)
        self.check("W8 06 AX 3304", "WB06AX3304", "STANDARD", corrections=1)

    def test_signs_are_not_plates(self):
        for text in ["TAXI", "MORS", "ANTRS", "CHTL", "UBER", "POLICE", "HOTEL SAAD", "12345"]:
            self.assertFalse(plates.validate(text).valid, text)

    def test_corrected_read_needs_full_number(self):
        # "AN1R5" would be Andaman with a one-digit number only after two corrections.
        self.assertFalse(plates.validate("ANTRS").valid)

    def test_ind_mark_dropped(self):
        v = plates.validate("INDWB06AX3304")
        self.assertEqual(v.normalised, "WB06AX3304")
        self.assertTrue(v.valid)

    def test_unrecognised_kept_not_invented(self):
        v = plates.validate("XX99")
        self.assertFalse(v.valid)
        self.assertEqual(v.normalised, "XX99")


if __name__ == "__main__":
    unittest.main()
