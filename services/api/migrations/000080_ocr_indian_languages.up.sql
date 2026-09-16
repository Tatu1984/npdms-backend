-- OCR for scanned documents, in the languages of India.
--
-- A scanned circular has no text layer, so until now it was findable only by
-- its title and reference. OCR reads it well enough to find, which is a
-- different claim from reading it well enough to quote: on clean rendered text
-- Tesseract returns 72-100% of characters depending on the script, and whole
-- words come back exactly only about half the time in Bengali, Tamil, Kannada
-- and Odia. The trigram index on text_content already handles that — searching
-- the mangled Bengali for মোবাইল, বাজার or গড়িয়াহাট still finds the page.
--
-- So the text is stored as a search aid, always labelled as read by machine and
-- unchecked, and never presented as the document's content.

ALTER TABLE knowledge_documents
    -- Which engine read it, and at what version, so a page read by a worse
    -- model can be found and read again later.
    ADD COLUMN IF NOT EXISTS ocr_engine      VARCHAR(60),
    -- The languages the engine was told to expect, e.g. {ben,eng}.
    ADD COLUMN IF NOT EXISTS ocr_languages   TEXT[],
    -- The script the page detected as, from Tesseract's own orientation and
    -- script detection: Bengali, Devanagari, Tamil, Latin …
    ADD COLUMN IF NOT EXISTS ocr_script      VARCHAR(40),
    -- Mean per-word confidence the engine reported, 0-1. Shown with the text.
    ADD COLUMN IF NOT EXISTS ocr_confidence  DOUBLE PRECISION
        CHECK (ocr_confidence IS NULL OR (ocr_confidence >= 0 AND ocr_confidence <= 1)),
    ADD COLUMN IF NOT EXISTS ocr_pages       INTEGER,
    ADD COLUMN IF NOT EXISTS ocr_read_at     TIMESTAMPTZ;

-- 'OCR' joins the extraction statuses: text that a machine read off an image,
-- as distinct from TEXT_LAYER (text the document already carried).
DO $$
DECLARE
    v_constraint TEXT;
BEGIN
    SELECT conname INTO v_constraint
      FROM pg_constraint
     WHERE conrelid = 'knowledge_documents'::regclass
       AND contype = 'c'
       AND pg_get_constraintdef(oid) LIKE '%extraction_status%';

    IF v_constraint IS NOT NULL THEN
        EXECUTE format('ALTER TABLE knowledge_documents DROP CONSTRAINT %I', v_constraint);
    END IF;

    ALTER TABLE knowledge_documents
        ADD CONSTRAINT knowledge_documents_extraction_status_check
        CHECK (extraction_status IN ('TEXT_LAYER', 'PLAIN_TEXT', 'OCR', 'NO_TEXT_LAYER',
                                     'OCR_UNAVAILABLE', 'UNSUPPORTED', 'FAILED'));
END $$;

COMMENT ON COLUMN knowledge_documents.ocr_confidence IS
    'Mean per-word confidence the OCR engine reported. It is the engine''s own opinion of its reading, not a measure of whether the reading is right.';
COMMENT ON COLUMN knowledge_documents.text_content IS
    'Searchable text. Where extraction_status is OCR this was read off an image by machine and has not been checked by anyone: use it to find the document, not to quote it.';
