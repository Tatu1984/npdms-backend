-- Remove forensic request numbers. The counter seeds are left alone: rolling
-- them back would let a register reissue a number already quoted to a court.

DROP INDEX IF EXISTS idx_forensics_request_number;
ALTER TABLE forensics DROP COLUMN IF EXISTS request_number;
