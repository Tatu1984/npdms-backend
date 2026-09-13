"""
NPDMS OCR Service
Production-ready OCR with multi-language support, document classification, and entity extraction
"""

import os
import io
import json
import logging
import tempfile
from typing import Optional, List, Dict, Any
from dataclasses import dataclass
from datetime import datetime
from enum import Enum

from fastapi import FastAPI, File, UploadFile, HTTPException, Form, Query
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import uvicorn

# OCR libraries
import pytesseract
from PIL import Image
import cv2
import numpy as np

# PDF processing
import fitz  # PyMuPDF

# NLP for entity extraction
import spacy
import re

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(
    title="NPDMS OCR Service",
    description="Production-ready OCR with multi-language support and entity extraction",
    version="1.0.0"
)

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


class DocumentType(str, Enum):
    FIR = "FIR"
    CHARGE_SHEET = "CHARGE_SHEET"
    COURT_ORDER = "COURT_ORDER"
    WARRANT = "WARRANT"
    BAIL_APPLICATION = "BAIL_APPLICATION"
    EVIDENCE_REPORT = "EVIDENCE_REPORT"
    WITNESS_STATEMENT = "WITNESS_STATEMENT"
    ID_DOCUMENT = "ID_DOCUMENT"
    VEHICLE_DOCUMENT = "VEHICLE_DOCUMENT"
    FINANCIAL_DOCUMENT = "FINANCIAL_DOCUMENT"
    OTHER = "OTHER"


# Supported languages for OCR
SUPPORTED_LANGUAGES = {
    "eng": "English",
    "hin": "Hindi",
    "ben": "Bengali",
    "tel": "Telugu",
    "mar": "Marathi",
    "tam": "Tamil",
    "guj": "Gujarati",
    "kan": "Kannada",
    "mal": "Malayalam",
    "pan": "Punjabi",
    "ori": "Odia",
    "urd": "Urdu",
}

# Document type patterns for classification
DOCUMENT_PATTERNS = {
    DocumentType.FIR: [
        r"first\s+information\s+report",
        r"f\.?i\.?r\.?\s*no",
        r"police\s+station",
        r"ipc\s+section",
        r"प्रथम\s+सूचना\s+रिपोर्ट",
    ],
    DocumentType.CHARGE_SHEET: [
        r"charge\s*sheet",
        r"prosecution\s+report",
        r"accused",
        r"investigation\s+officer",
    ],
    DocumentType.COURT_ORDER: [
        r"court\s+order",
        r"in\s+the\s+court\s+of",
        r"magistrate",
        r"sessions\s+judge",
        r"judgment",
        r"verdict",
    ],
    DocumentType.WARRANT: [
        r"warrant",
        r"arrest\s+warrant",
        r"search\s+warrant",
        r"non[-\s]?bailable",
    ],
    DocumentType.BAIL_APPLICATION: [
        r"bail\s+application",
        r"anticipatory\s+bail",
        r"regular\s+bail",
        r"interim\s+bail",
    ],
    DocumentType.ID_DOCUMENT: [
        r"aadhaar",
        r"pan\s+card",
        r"voter\s+id",
        r"passport",
        r"driving\s+licence",
        r"आधार",
    ],
    DocumentType.VEHICLE_DOCUMENT: [
        r"registration\s+certificate",
        r"vehicle\s+number",
        r"chassis\s+number",
        r"engine\s+number",
        r"insurance\s+policy",
    ],
    DocumentType.FINANCIAL_DOCUMENT: [
        r"bank\s+statement",
        r"transaction",
        r"account\s+number",
        r"ifsc",
        r"cheque",
    ],
}

# Entity patterns for extraction
ENTITY_PATTERNS = {
    "fir_number": r"F\.?I\.?R\.?\s*(?:No\.?|Number)?\s*[:\-]?\s*(\d+[/\-]\d{4}|\d+)",
    "case_number": r"(?:Case|Crime)\s*(?:No\.?|Number)?\s*[:\-]?\s*(\d+[/\-]\d{4}|\d+)",
    "date": r"(\d{1,2}[/\-\.]\d{1,2}[/\-\.]\d{2,4})",
    "aadhaar": r"(\d{4}\s?\d{4}\s?\d{4})",
    "pan": r"([A-Z]{5}\d{4}[A-Z])",
    "phone": r"(?:\+91[\-\s]?)?([6-9]\d{9})",
    "vehicle_number": r"([A-Z]{2}[\-\s]?\d{1,2}[\-\s]?[A-Z]{1,3}[\-\s]?\d{1,4})",
    "ipc_section": r"(?:IPC|BNS)\s*(?:Section)?\s*(\d+[A-Z]?(?:/\d+[A-Z]?)*)",
    "pincode": r"(\d{6})",
    "email": r"([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})",
    "amount": r"(?:Rs\.?|INR|₹)\s*([\d,]+(?:\.\d{2})?)",
}


class OCRRequest(BaseModel):
    languages: List[str] = ["eng"]
    extract_entities: bool = True
    classify_document: bool = True
    enhance_image: bool = True


class ExtractedEntity(BaseModel):
    type: str
    value: str
    confidence: float
    position: Optional[Dict[str, int]] = None


class OCRResult(BaseModel):
    text: str
    confidence: float
    language: str
    document_type: Optional[str] = None
    document_confidence: Optional[float] = None
    entities: Optional[List[ExtractedEntity]] = None
    pages: Optional[int] = None
    processing_time_ms: int


# Load spaCy model
_nlp = None


def get_nlp():
    global _nlp
    if _nlp is None:
        try:
            _nlp = spacy.load("en_core_web_sm")
        except:
            _nlp = spacy.blank("en")
    return _nlp


def preprocess_image(image: np.ndarray, enhance: bool = True) -> np.ndarray:
    """Preprocess image for better OCR results"""
    if not enhance:
        return image

    # Convert to grayscale if needed
    if len(image.shape) == 3:
        gray = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)
    else:
        gray = image.copy()

    # Denoise
    denoised = cv2.fastNlMeansDenoising(gray, None, 10, 7, 21)

    # Adaptive thresholding
    binary = cv2.adaptiveThreshold(
        denoised, 255, cv2.ADAPTIVE_THRESH_GAUSSIAN_C, cv2.THRESH_BINARY, 11, 2
    )

    # Deskew
    coords = np.column_stack(np.where(binary > 0))
    if len(coords) > 0:
        angle = cv2.minAreaRect(coords)[-1]
        if angle < -45:
            angle = 90 + angle
        if abs(angle) > 0.5:
            (h, w) = binary.shape[:2]
            center = (w // 2, h // 2)
            M = cv2.getRotationMatrix2D(center, angle, 1.0)
            binary = cv2.warpAffine(
                binary, M, (w, h), flags=cv2.INTER_CUBIC, borderMode=cv2.BORDER_REPLICATE
            )

    return binary


def classify_document(text: str) -> tuple[DocumentType, float]:
    """Classify document type based on content patterns"""
    text_lower = text.lower()
    scores = {}

    for doc_type, patterns in DOCUMENT_PATTERNS.items():
        score = 0
        for pattern in patterns:
            matches = re.findall(pattern, text_lower, re.IGNORECASE)
            score += len(matches) * 2
        scores[doc_type] = score

    if max(scores.values()) == 0:
        return DocumentType.OTHER, 0.5

    best_type = max(scores, key=scores.get)
    confidence = min(scores[best_type] / 10, 1.0)
    return best_type, confidence


def extract_entities_from_text(text: str) -> List[ExtractedEntity]:
    """Extract entities from text using regex and NLP"""
    entities = []

    # Regex-based extraction
    for entity_type, pattern in ENTITY_PATTERNS.items():
        matches = re.finditer(pattern, text, re.IGNORECASE)
        for match in matches:
            entities.append(ExtractedEntity(
                type=entity_type,
                value=match.group(1) if match.groups() else match.group(0),
                confidence=0.9,
                position={"start": match.start(), "end": match.end()}
            ))

    # NLP-based extraction
    nlp = get_nlp()
    doc = nlp(text[:100000])  # Limit text length for NLP

    for ent in doc.ents:
        entities.append(ExtractedEntity(
            type=ent.label_,
            value=ent.text,
            confidence=0.8,
            position={"start": ent.start_char, "end": ent.end_char}
        ))

    # Deduplicate
    seen = set()
    unique_entities = []
    for e in entities:
        key = (e.type, e.value.lower().strip())
        if key not in seen:
            seen.add(key)
            unique_entities.append(e)

    return unique_entities


def ocr_image(
    image: np.ndarray,
    languages: List[str],
    enhance: bool = True
) -> tuple[str, float]:
    """Perform OCR on an image"""
    # Preprocess
    processed = preprocess_image(image, enhance)

    # Build language string
    lang_str = "+".join(languages)

    # Perform OCR with confidence
    try:
        data = pytesseract.image_to_data(
            processed,
            lang=lang_str,
            output_type=pytesseract.Output.DICT
        )

        # Calculate average confidence
        confidences = [int(c) for c in data['conf'] if int(c) > 0]
        avg_confidence = sum(confidences) / len(confidences) if confidences else 0

        # Get text
        text = pytesseract.image_to_string(processed, lang=lang_str)

        return text.strip(), avg_confidence / 100
    except Exception as e:
        logger.error(f"OCR error: {e}")
        # Fallback to basic OCR
        text = pytesseract.image_to_string(processed, lang=lang_str)
        return text.strip(), 0.5


def ocr_pdf(
    pdf_bytes: bytes,
    languages: List[str],
    enhance: bool = True
) -> tuple[str, float, int]:
    """Perform OCR on a PDF document"""
    doc = fitz.open(stream=pdf_bytes, filetype="pdf")
    all_text = []
    all_confidence = []

    for page_num in range(len(doc)):
        page = doc.load_page(page_num)

        # Try to extract text directly first
        text = page.get_text()

        if text.strip():
            all_text.append(text)
            all_confidence.append(0.95)  # Native text is high confidence
        else:
            # OCR the page image
            pix = page.get_pixmap(dpi=300)
            img = np.frombuffer(pix.samples, dtype=np.uint8).reshape(
                pix.height, pix.width, pix.n
            )

            if pix.n == 4:
                img = cv2.cvtColor(img, cv2.COLOR_RGBA2RGB)

            text, confidence = ocr_image(img, languages, enhance)
            all_text.append(text)
            all_confidence.append(confidence)

    doc.close()

    full_text = "\n\n--- Page Break ---\n\n".join(all_text)
    avg_confidence = sum(all_confidence) / len(all_confidence) if all_confidence else 0

    return full_text, avg_confidence, len(all_text)


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {
        "status": "healthy",
        "service": "ocr",
        "timestamp": datetime.utcnow().isoformat(),
        "tesseract_version": pytesseract.get_tesseract_version(),
        "supported_languages": len(SUPPORTED_LANGUAGES),
    }


@app.get("/languages")
async def get_languages():
    """Get list of supported languages"""
    return {
        "languages": [
            {"code": code, "name": name}
            for code, name in SUPPORTED_LANGUAGES.items()
        ]
    }


@app.get("/document-types")
async def get_document_types():
    """Get list of supported document types"""
    return {
        "types": [dt.value for dt in DocumentType]
    }


@app.post("/extract", response_model=OCRResult)
async def extract_text(
    file: UploadFile = File(...),
    languages: str = Form(default="eng"),
    extract_entities: bool = Form(default=True),
    classify_document: bool = Form(default=True),
    enhance_image: bool = Form(default=True),
):
    """
    Extract text from image or PDF with optional entity extraction and document classification

    - **file**: Image (PNG, JPG, TIFF) or PDF file
    - **languages**: Comma-separated language codes (eng, hin, etc.)
    - **extract_entities**: Extract named entities and patterns
    - **classify_document**: Classify document type
    - **enhance_image**: Apply image preprocessing
    """
    import time
    start_time = time.time()

    # Parse languages
    lang_list = [l.strip() for l in languages.split(",")]
    for lang in lang_list:
        if lang not in SUPPORTED_LANGUAGES:
            raise HTTPException(
                status_code=400,
                detail=f"Unsupported language: {lang}. Supported: {list(SUPPORTED_LANGUAGES.keys())}"
            )

    # Read file
    content = await file.read()
    if len(content) == 0:
        raise HTTPException(status_code=400, detail="Empty file")

    # Determine file type and process
    filename = file.filename.lower() if file.filename else ""

    try:
        if filename.endswith(".pdf") or file.content_type == "application/pdf":
            text, confidence, pages = ocr_pdf(content, lang_list, enhance_image)
        else:
            # Process as image
            nparr = np.frombuffer(content, np.uint8)
            image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)

            if image is None:
                raise HTTPException(status_code=400, detail="Invalid image file")

            text, confidence = ocr_image(image, lang_list, enhance_image)
            pages = 1

        # Build response
        result = OCRResult(
            text=text,
            confidence=confidence,
            language=lang_list[0],
            pages=pages,
            processing_time_ms=int((time.time() - start_time) * 1000),
        )

        # Classify document
        if classify_document and text:
            doc_type, doc_confidence = classify_document(text)
            result.document_type = doc_type.value
            result.document_confidence = doc_confidence

        # Extract entities
        if extract_entities and text:
            result.entities = extract_entities_from_text(text)

        return result

    except Exception as e:
        logger.error(f"Processing error: {e}")
        raise HTTPException(status_code=500, detail=f"Processing failed: {str(e)}")


@app.post("/extract-batch")
async def extract_batch(
    files: List[UploadFile] = File(...),
    languages: str = Form(default="eng"),
    extract_entities: bool = Form(default=True),
    classify_document: bool = Form(default=True),
):
    """
    Extract text from multiple files in batch
    """
    results = []

    for file in files:
        try:
            content = await file.read()
            lang_list = [l.strip() for l in languages.split(",")]

            filename = file.filename.lower() if file.filename else ""

            import time
            start_time = time.time()

            if filename.endswith(".pdf"):
                text, confidence, pages = ocr_pdf(content, lang_list, True)
            else:
                nparr = np.frombuffer(content, np.uint8)
                image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
                text, confidence = ocr_image(image, lang_list, True) if image is not None else ("", 0)
                pages = 1

            result = {
                "filename": file.filename,
                "text": text,
                "confidence": confidence,
                "pages": pages,
                "processing_time_ms": int((time.time() - start_time) * 1000),
                "success": True,
            }

            if classify_document and text:
                doc_type, doc_confidence = classify_document(text)
                result["document_type"] = doc_type.value
                result["document_confidence"] = doc_confidence

            if extract_entities and text:
                result["entities"] = [e.dict() for e in extract_entities_from_text(text)]

            results.append(result)

        except Exception as e:
            results.append({
                "filename": file.filename,
                "success": False,
                "error": str(e),
            })

    return {"results": results, "total": len(results)}


@app.post("/classify")
async def classify_only(
    file: UploadFile = File(...),
    languages: str = Form(default="eng"),
):
    """
    Classify document type without full text extraction
    """
    content = await file.read()
    lang_list = [l.strip() for l in languages.split(",")]

    filename = file.filename.lower() if file.filename else ""

    try:
        if filename.endswith(".pdf"):
            text, _, _ = ocr_pdf(content, lang_list, True)
        else:
            nparr = np.frombuffer(content, np.uint8)
            image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)
            text, _ = ocr_image(image, lang_list, True)

        doc_type, confidence = classify_document(text)

        return {
            "document_type": doc_type.value,
            "confidence": confidence,
            "filename": file.filename,
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/extract-entities")
async def extract_entities_only(
    text: str = Form(...),
):
    """
    Extract entities from provided text
    """
    if not text.strip():
        raise HTTPException(status_code=400, detail="Empty text")

    entities = extract_entities_from_text(text)
    return {"entities": [e.dict() for e in entities]}


@app.post("/enhance")
async def enhance_image_only(
    file: UploadFile = File(...),
):
    """
    Enhance image for better OCR (returns processed image)
    """
    content = await file.read()
    nparr = np.frombuffer(content, np.uint8)
    image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)

    if image is None:
        raise HTTPException(status_code=400, detail="Invalid image")

    enhanced = preprocess_image(image, True)

    # Encode as PNG
    _, buffer = cv2.imencode('.png', enhanced)

    from fastapi.responses import Response
    return Response(content=buffer.tobytes(), media_type="image/png")


if __name__ == "__main__":
    # Ensure spaCy model is available
    try:
        spacy.load("en_core_web_sm")
    except:
        logger.info("Downloading spaCy model...")
        import subprocess
        subprocess.run(["python", "-m", "spacy", "download", "en_core_web_sm"])

    uvicorn.run(
        "app:app",
        host="0.0.0.0",
        port=8004,
        reload=True,
        workers=1,
    )
