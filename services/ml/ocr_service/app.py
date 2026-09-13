"""
OCR Service for NPDMS
Extract text from evidence photos and documents
Uses EasyOCR for multi-language support
"""

from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import easyocr
import numpy as np
from PIL import Image
import io
from typing import List, Dict, Optional
import logging

# Setup logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(
    title="NPDMS OCR Service",
    description="Extract text from evidence photos and documents",
    version="1.0.0"
)

# CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Global OCR reader (lazy loaded)
reader: Optional[easyocr.Reader] = None

def get_reader():
    """Lazy load OCR reader"""
    global reader
    if reader is None:
        logger.info("Initializing EasyOCR reader (English + Hindi)...")
        # Support both English and Hindi (Devanagari)
        reader = easyocr.Reader(['en', 'hi'], gpu=False)
        logger.info("OCR reader initialized successfully")
    return reader

# ============================================================================
# MODELS
# ============================================================================

class OCRResult(BaseModel):
    """OCR extraction result"""
    text: str
    confidence: float
    bbox: List[List[int]]  # Bounding box coordinates [[x1,y1], [x2,y2], [x3,y3], [x4,y4]]

class OCRResponse(BaseModel):
    """OCR API response"""
    success: bool
    full_text: str
    results: List[OCRResult]
    total_words: int
    average_confidence: float
    languages_detected: List[str]

class HealthResponse(BaseModel):
    """Health check response"""
    status: str
    service: str
    model_loaded: bool

# ============================================================================
# ENDPOINTS
# ============================================================================

@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check endpoint"""
    return HealthResponse(
        status="healthy",
        service="ocr",
        model_loaded=reader is not None
    )

@app.post("/ocr", response_model=OCRResponse)
async def extract_text(
    file: UploadFile = File(...),
    detail: int = 1  # 0: paragraph, 1: word-level
):
    """
    Extract text from image

    Args:
        file: Image file (JPG, PNG, etc.)
        detail: 0 for paragraph-level, 1 for word-level extraction

    Returns:
        OCRResponse with extracted text and bounding boxes
    """
    try:
        # Validate file type
        if not file.content_type.startswith('image/'):
            raise HTTPException(status_code=400, detail="File must be an image")

        # Read image
        contents = await file.read()
        image = Image.open(io.BytesIO(contents))

        # Convert to numpy array
        img_array = np.array(image)

        # Perform OCR
        logger.info(f"Processing image: {file.filename}")
        ocr_reader = get_reader()
        results = ocr_reader.readtext(img_array, detail=detail)

        # Parse results
        ocr_results = []
        full_text_parts = []
        total_confidence = 0.0

        for bbox, text, confidence in results:
            # Convert bbox to integer coordinates
            bbox_int = [[int(x), int(y)] for x, y in bbox]

            ocr_results.append(OCRResult(
                text=text,
                confidence=confidence,
                bbox=bbox_int
            ))

            full_text_parts.append(text)
            total_confidence += confidence

        # Calculate average confidence
        avg_confidence = total_confidence / len(results) if results else 0.0

        # Combine full text
        full_text = ' '.join(full_text_parts)

        # Detect languages (simple heuristic)
        languages = ['en']
        # Check if text contains Devanagari characters
        if any('\u0900' <= char <= '\u097F' for char in full_text):
            languages.append('hi')

        logger.info(f"OCR complete: {len(results)} text regions found")

        return OCRResponse(
            success=True,
            full_text=full_text,
            results=ocr_results,
            total_words=len(results),
            average_confidence=avg_confidence,
            languages_detected=languages
        )

    except Exception as e:
        logger.error(f"OCR error: {str(e)}")
        raise HTTPException(status_code=500, detail=f"OCR processing failed: {str(e)}")

@app.post("/ocr/batch", response_model=List[OCRResponse])
async def extract_text_batch(
    files: List[UploadFile] = File(...),
    detail: int = 1
):
    """
    Extract text from multiple images

    Args:
        files: List of image files
        detail: 0 for paragraph-level, 1 for word-level extraction

    Returns:
        List of OCRResponse for each image
    """
    results = []

    for file in files:
        try:
            result = await extract_text(file, detail)
            results.append(result)
        except Exception as e:
            logger.error(f"Failed to process {file.filename}: {e}")
            # Return error result for this file
            results.append(OCRResponse(
                success=False,
                full_text="",
                results=[],
                total_words=0,
                average_confidence=0.0,
                languages_detected=[]
            ))

    return results

@app.get("/")
async def root():
    """Root endpoint"""
    return {
        "service": "NPDMS OCR Service",
        "version": "1.0.0",
        "endpoints": {
            "/health": "Health check",
            "/ocr": "Extract text from single image",
            "/ocr/batch": "Extract text from multiple images",
            "/docs": "API documentation"
        }
    }

# ============================================================================
# STARTUP
# ============================================================================

@app.on_event("startup")
async def startup_event():
    """Initialize service on startup"""
    logger.info("Starting OCR service...")
    logger.info("OCR reader will be loaded on first request (lazy loading)")
    logger.info("OCR service ready!")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8004)
