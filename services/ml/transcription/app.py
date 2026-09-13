"""
NPDMS Voice Transcription Service
Multi-language speech-to-text with entity extraction and IPC section suggestions
"""

import os
import io
import json
import logging
import tempfile
from typing import Optional, List, Dict, Any
from dataclasses import dataclass
from datetime import datetime

from fastapi import FastAPI, File, UploadFile, HTTPException, Form
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import StreamingResponse
from pydantic import BaseModel
import uvicorn

# For speech recognition
import speech_recognition as sr
from pydub import AudioSegment

# For NLP and entity extraction
import spacy
from spacy.language import Language

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(
    title="NPDMS Voice Transcription Service",
    description="Multi-language speech-to-text with entity extraction",
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

# Supported languages
SUPPORTED_LANGUAGES = {
    "en": "English",
    "hi": "Hindi",
    "bn": "Bengali",
    "te": "Telugu",
    "mr": "Marathi",
    "ta": "Tamil",
    "gu": "Gujarati",
    "kn": "Kannada",
    "ml": "Malayalam",
    "pa": "Punjabi",
    "or": "Odia",
    "ur": "Urdu",
}

# IPC Section mappings for common crimes
IPC_SECTION_KEYWORDS = {
    "murder": ["302", "Murder - IPC Section 302"],
    "homicide": ["302", "Murder - IPC Section 302"],
    "kill": ["302", "Murder - IPC Section 302"],
    "death": ["302", "304", "Murder/Culpable Homicide - IPC Section 302/304"],
    "theft": ["379", "Theft - IPC Section 379"],
    "stolen": ["379", "Theft - IPC Section 379"],
    "robbery": ["392", "Robbery - IPC Section 392"],
    "dacoity": ["395", "Dacoity - IPC Section 395"],
    "assault": ["323", "324", "Assault - IPC Section 323/324"],
    "hurt": ["323", "324", "Voluntarily causing hurt - IPC Section 323/324"],
    "grievous": ["325", "326", "Grievous hurt - IPC Section 325/326"],
    "rape": ["376", "Rape - IPC Section 376"],
    "sexual": ["354", "376", "Sexual assault/Rape - IPC Section 354/376"],
    "molest": ["354", "Outraging modesty - IPC Section 354"],
    "kidnap": ["363", "364", "Kidnapping - IPC Section 363/364"],
    "abduction": ["362", "Abduction - IPC Section 362"],
    "fraud": ["420", "Cheating - IPC Section 420"],
    "cheat": ["420", "Cheating - IPC Section 420"],
    "forgery": ["465", "Forgery - IPC Section 465"],
    "extortion": ["384", "Extortion - IPC Section 384"],
    "blackmail": ["384", "Extortion - IPC Section 384"],
    "threat": ["503", "506", "Criminal intimidation - IPC Section 503/506"],
    "defamation": ["499", "500", "Defamation - IPC Section 499/500"],
    "trespass": ["441", "447", "Criminal trespass - IPC Section 441/447"],
    "burglary": ["457", "458", "House-breaking - IPC Section 457/458"],
    "arson": ["435", "436", "Mischief by fire - IPC Section 435/436"],
    "dowry": ["498A", "304B", "Dowry harassment/death - IPC Section 498A/304B"],
    "domestic": ["498A", "Cruelty - IPC Section 498A"],
    "accident": ["279", "304A", "Rash driving/Negligent death - IPC Section 279/304A"],
    "drunk driving": ["279", "185 MV Act", "Rash driving - IPC Section 279"],
    "cybercrime": ["IT Act 66", "Cyber crime - IT Act Section 66"],
    "hacking": ["IT Act 66", "Computer hacking - IT Act Section 66"],
    "online fraud": ["IT Act 66C", "66D", "Online fraud - IT Act Section 66C/66D"],
    "narcotics": ["NDPS Act", "Drug offenses - NDPS Act"],
    "drugs": ["NDPS Act", "Drug offenses - NDPS Act"],
}

# Entity types to extract
ENTITY_TYPES = [
    "PERSON",      # Names of people
    "GPE",         # Geopolitical entities (locations)
    "DATE",        # Dates
    "TIME",        # Times
    "MONEY",       # Money amounts
    "CARDINAL",    # Numbers
    "ORG",         # Organizations
]


class TranscriptionRequest(BaseModel):
    language: str = "en"
    extract_entities: bool = True
    suggest_ipc: bool = True


class TranscriptionResult(BaseModel):
    text: str
    language: str
    confidence: float
    duration_seconds: float
    entities: Optional[List[Dict[str, Any]]] = None
    ipc_suggestions: Optional[List[Dict[str, str]]] = None
    keywords: Optional[List[str]] = None


class EntityExtractionResult(BaseModel):
    entities: List[Dict[str, Any]]
    keywords: List[str]
    ipc_suggestions: List[Dict[str, str]]


# Load spaCy model (lazy loading)
_nlp_models: Dict[str, Language] = {}


def get_nlp_model(language: str = "en") -> Language:
    """Get or load spaCy model for the given language"""
    if language not in _nlp_models:
        try:
            if language == "en":
                _nlp_models[language] = spacy.load("en_core_web_sm")
            else:
                # For other languages, use multilingual model or English as fallback
                try:
                    _nlp_models[language] = spacy.load("xx_ent_wiki_sm")
                except:
                    _nlp_models[language] = spacy.load("en_core_web_sm")
        except Exception as e:
            logger.error(f"Failed to load spaCy model: {e}")
            # Create a blank model as fallback
            _nlp_models[language] = spacy.blank("en")
    return _nlp_models[language]


def extract_entities(text: str, language: str = "en") -> Dict[str, Any]:
    """Extract named entities from text"""
    nlp = get_nlp_model(language)
    doc = nlp(text)

    entities = []
    for ent in doc.ents:
        if ent.label_ in ENTITY_TYPES:
            entities.append({
                "text": ent.text,
                "label": ent.label_,
                "start": ent.start_char,
                "end": ent.end_char,
            })

    # Extract keywords (nouns and proper nouns)
    keywords = []
    for token in doc:
        if token.pos_ in ["NOUN", "PROPN"] and not token.is_stop and len(token.text) > 2:
            keywords.append(token.text.lower())

    keywords = list(set(keywords))[:20]  # Unique, max 20

    return {
        "entities": entities,
        "keywords": keywords,
    }


def suggest_ipc_sections(text: str) -> List[Dict[str, str]]:
    """Suggest IPC sections based on text content"""
    text_lower = text.lower()
    suggestions = []
    seen_sections = set()

    for keyword, section_info in IPC_SECTION_KEYWORDS.items():
        if keyword in text_lower:
            section = section_info[0]
            if section not in seen_sections:
                suggestions.append({
                    "section": section,
                    "description": section_info[1] if len(section_info) > 1 else section,
                    "matched_keyword": keyword,
                })
                seen_sections.add(section)

    return suggestions[:10]  # Max 10 suggestions


def transcribe_audio(audio_data: bytes, language: str = "en") -> Dict[str, Any]:
    """Transcribe audio data to text"""
    recognizer = sr.Recognizer()

    # Save audio to temp file and convert to WAV if needed
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as temp_file:
        temp_path = temp_file.name

        try:
            # Try to convert audio to WAV format
            audio = AudioSegment.from_file(io.BytesIO(audio_data))
            audio = audio.set_frame_rate(16000).set_channels(1)
            audio.export(temp_path, format="wav")
            duration = len(audio) / 1000.0  # Duration in seconds
        except Exception as e:
            logger.error(f"Audio conversion error: {e}")
            # Try direct WAV save
            with open(temp_path, "wb") as f:
                f.write(audio_data)
            duration = 0.0

    try:
        with sr.AudioFile(temp_path) as source:
            audio = recognizer.record(source)

        # Map language codes to Google Speech Recognition language codes
        lang_code = {
            "en": "en-IN",
            "hi": "hi-IN",
            "bn": "bn-IN",
            "te": "te-IN",
            "mr": "mr-IN",
            "ta": "ta-IN",
            "gu": "gu-IN",
            "kn": "kn-IN",
            "ml": "ml-IN",
            "pa": "pa-IN",
            "or": "or-IN",
            "ur": "ur-IN",
        }.get(language, "en-IN")

        # Use Google Speech Recognition
        text = recognizer.recognize_google(audio, language=lang_code)
        confidence = 0.85  # Estimated confidence

        return {
            "text": text,
            "confidence": confidence,
            "duration": duration,
        }
    except sr.UnknownValueError:
        return {
            "text": "",
            "confidence": 0.0,
            "duration": duration,
            "error": "Could not understand audio",
        }
    except sr.RequestError as e:
        raise HTTPException(
            status_code=503,
            detail=f"Speech recognition service unavailable: {e}"
        )
    finally:
        # Clean up temp file
        try:
            os.unlink(temp_path)
        except:
            pass


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {
        "status": "healthy",
        "service": "transcription",
        "timestamp": datetime.utcnow().isoformat(),
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


@app.post("/transcribe", response_model=TranscriptionResult)
async def transcribe(
    audio: UploadFile = File(...),
    language: str = Form(default="en"),
    extract_entities: bool = Form(default=True),
    suggest_ipc: bool = Form(default=True),
):
    """
    Transcribe audio file to text with optional entity extraction and IPC suggestions

    - **audio**: Audio file (WAV, MP3, WebM, OGG, etc.)
    - **language**: Language code (en, hi, bn, te, mr, ta, gu, kn, ml, pa, or, ur)
    - **extract_entities**: Extract named entities from transcription
    - **suggest_ipc**: Suggest relevant IPC sections
    """
    if language not in SUPPORTED_LANGUAGES:
        raise HTTPException(
            status_code=400,
            detail=f"Unsupported language: {language}. Supported: {list(SUPPORTED_LANGUAGES.keys())}"
        )

    # Read audio data
    audio_data = await audio.read()
    if len(audio_data) == 0:
        raise HTTPException(status_code=400, detail="Empty audio file")

    # Transcribe
    result = transcribe_audio(audio_data, language)

    if "error" in result:
        raise HTTPException(status_code=422, detail=result["error"])

    response = TranscriptionResult(
        text=result["text"],
        language=language,
        confidence=result["confidence"],
        duration_seconds=result["duration"],
    )

    # Extract entities if requested
    if extract_entities and result["text"]:
        entity_result = extract_entities(result["text"], language)
        response.entities = entity_result["entities"]
        response.keywords = entity_result["keywords"]

    # Suggest IPC sections if requested
    if suggest_ipc and result["text"]:
        response.ipc_suggestions = suggest_ipc_sections(result["text"])

    return response


@app.post("/extract-entities", response_model=EntityExtractionResult)
async def extract_entities_from_text(
    text: str = Form(...),
    language: str = Form(default="en"),
):
    """
    Extract named entities and suggest IPC sections from text
    """
    if not text.strip():
        raise HTTPException(status_code=400, detail="Empty text provided")

    entity_result = extract_entities(text, language)
    ipc_suggestions = suggest_ipc_sections(text)

    return EntityExtractionResult(
        entities=entity_result["entities"],
        keywords=entity_result["keywords"],
        ipc_suggestions=ipc_suggestions,
    )


@app.post("/suggest-ipc")
async def suggest_ipc(text: str = Form(...)):
    """
    Suggest IPC sections based on text content
    """
    if not text.strip():
        raise HTTPException(status_code=400, detail="Empty text provided")

    suggestions = suggest_ipc_sections(text)
    return {"suggestions": suggestions}


@app.post("/transcribe-stream")
async def transcribe_stream(
    audio: UploadFile = File(...),
    language: str = Form(default="en"),
):
    """
    Stream transcription results (for real-time transcription)
    Returns partial results as they become available
    """
    # For now, this is a simplified version
    # In production, this would use WebSocket for real-time streaming
    audio_data = await audio.read()
    result = transcribe_audio(audio_data, language)

    async def generate():
        yield json.dumps({
            "type": "partial",
            "text": result.get("text", "")[:len(result.get("text", ""))//2],
            "is_final": False,
        }) + "\n"

        yield json.dumps({
            "type": "final",
            "text": result.get("text", ""),
            "confidence": result.get("confidence", 0),
            "is_final": True,
        }) + "\n"

    return StreamingResponse(
        generate(),
        media_type="application/x-ndjson"
    )


if __name__ == "__main__":
    # Ensure spaCy model is downloaded
    try:
        spacy.load("en_core_web_sm")
    except OSError:
        logger.info("Downloading spaCy model...")
        import subprocess
        subprocess.run(["python", "-m", "spacy", "download", "en_core_web_sm"])

    uvicorn.run(
        "app:app",
        host="0.0.0.0",
        port=8005,
        reload=True,
        workers=1,
    )
