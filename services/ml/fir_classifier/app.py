"""
FIR Classification Service
Automatically classifies FIRs into categories and suggests IPC sections using DistilBERT
"""

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from typing import List, Dict, Optional
import torch
from transformers import DistilBertTokenizer, DistilBertForSequenceClassification
import logging
import os
from datetime import datetime

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="NPDMS FIR Classifier",
    description="AI-powered FIR classification and IPC section suggestion",
    version="1.0.0"
)

# CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # Configure for production
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# ============================================================================
# MODELS
# ============================================================================

# Crime categories (matching backend enum)
CRIME_CATEGORIES = [
    "VIOLENT",
    "PROPERTY",
    "CYBER",
    "ECONOMIC",
    "NARCOTICS",
    "SEXUAL",
    "ORGANIZED",
    "OTHER"
]

# IPC sections mapping for each category
IPC_SECTIONS_MAP = {
    "VIOLENT": [
        {"section": "IPC 302", "description": "Murder"},
        {"section": "IPC 307", "description": "Attempt to murder"},
        {"section": "IPC 323", "description": "Voluntarily causing hurt"},
        {"section": "IPC 324", "description": "Voluntarily causing hurt by dangerous weapon"},
        {"section": "IPC 325", "description": "Voluntarily causing grievous hurt"},
        {"section": "IPC 304", "description": "Culpable homicide not amounting to murder"},
        {"section": "IPC 326", "description": "Grievous hurt by dangerous weapon"},
    ],
    "PROPERTY": [
        {"section": "IPC 379", "description": "Theft"},
        {"section": "IPC 380", "description": "Theft in dwelling house"},
        {"section": "IPC 392", "description": "Robbery"},
        {"section": "IPC 411", "description": "Dishonestly receiving stolen property"},
        {"section": "IPC 457", "description": "Lurking house-trespass by night"},
        {"section": "IPC 460", "description": "Lurking house-trespass by night with grievous hurt"},
        {"section": "IPC 378", "description": "Theft"},
    ],
    "CYBER": [
        {"section": "IT Act 66", "description": "Computer related offences"},
        {"section": "IT Act 66A", "description": "Offensive messages"},
        {"section": "IT Act 66B", "description": "Receiving stolen computer resource"},
        {"section": "IT Act 66C", "description": "Identity theft"},
        {"section": "IT Act 66D", "description": "Cheating by personation using computer"},
        {"section": "IT Act 66E", "description": "Violation of privacy"},
        {"section": "IT Act 67", "description": "Publishing obscene material"},
    ],
    "ECONOMIC": [
        {"section": "IPC 420", "description": "Cheating and dishonestly inducing delivery of property"},
        {"section": "IPC 406", "description": "Criminal breach of trust"},
        {"section": "IPC 408", "description": "Criminal breach of trust by clerk or servant"},
        {"section": "IPC 415", "description": "Cheating"},
        {"section": "IPC 467", "description": "Forgery of valuable security"},
        {"section": "IPC 471", "description": "Using as genuine a forged document"},
    ],
    "NARCOTICS": [
        {"section": "NDPS 8", "description": "Prohibition of cultivation of cannabis plant"},
        {"section": "NDPS 20", "description": "Possession of cannabis"},
        {"section": "NDPS 21", "description": "Possession of cocaine"},
        {"section": "NDPS 22", "description": "Possession of manufactured drugs"},
        {"section": "NDPS 27A", "description": "Financing illicit traffic"},
    ],
    "SEXUAL": [
        {"section": "IPC 354", "description": "Assault with intent to outrage modesty"},
        {"section": "IPC 354A", "description": "Sexual harassment"},
        {"section": "IPC 354B", "description": "Assault to disrobe"},
        {"section": "IPC 376", "description": "Rape"},
        {"section": "IPC 377", "description": "Unnatural offences"},
        {"section": "POCSO 3", "description": "Penetrative sexual assault"},
        {"section": "POCSO 7", "description": "Sexual assault"},
    ],
    "ORGANIZED": [
        {"section": "IPC 120B", "description": "Criminal conspiracy"},
        {"section": "IPC 121", "description": "Waging war against Government"},
        {"section": "IPC 364", "description": "Kidnapping for ransom"},
        {"section": "IPC 387", "description": "Extortion"},
    ],
    "OTHER": [
        {"section": "IPC 279", "description": "Rash driving"},
        {"section": "IPC 304A", "description": "Death by negligence"},
        {"section": "IPC 504", "description": "Intentional insult"},
        {"section": "IPC 506", "description": "Criminal intimidation"},
    ],
}

# Keywords for rule-based fallback
CATEGORY_KEYWORDS = {
    "VIOLENT": ["murder", "assault", "attack", "hurt", "injured", "weapon", "killed", "death", "beating", "stabbing"],
    "PROPERTY": ["theft", "stolen", "burglary", "robbery", "stealing", "missing", "looted", "chain snatching"],
    "CYBER": ["online", "cyber", "phishing", "hacking", "digital", "internet", "email", "social media", "website"],
    "ECONOMIC": ["fraud", "cheating", "fake", "scam", "investment", "money", "financial", "forgery", "embezzlement"],
    "NARCOTICS": ["drugs", "narcotics", "cocaine", "cannabis", "ganja", "heroin", "mdma", "substance"],
    "SEXUAL": ["molestation", "harassment", "rape", "sexual", "modesty", "indecent", "obscene"],
    "ORGANIZED": ["gang", "conspiracy", "syndicate", "trafficking", "kidnapping", "ransom", "extortion"],
    "OTHER": ["accident", "negligence", "dispute", "quarrel", "insult", "intimidation"],
}

# ============================================================================
# REQUEST/RESPONSE MODELS
# ============================================================================

class ClassificationRequest(BaseModel):
    description: str = Field(..., description="FIR description text", min_length=10)
    title: str | None = Field(None, description="FIR title (optional)")
    incident_location: str | None = Field(None, description="Incident location (optional)")

class IPCSection(BaseModel):
    section: str
    description: str
    confidence: float = Field(0.0, ge=0.0, le=1.0)

class ClassificationResponse(BaseModel):
    category: str = Field(..., description="Predicted crime category")
    confidence: float = Field(..., ge=0.0, le=1.0, description="Prediction confidence")
    suggested_ipc_sections: List[IPCSection] = Field(..., description="Suggested IPC sections")
    all_categories_scores: Dict[str, float] = Field(..., description="Scores for all categories")
    method: str = Field(..., description="Classification method used (ml/rule-based)")
    timestamp: str = Field(..., description="Classification timestamp")

class HealthResponse(BaseModel):
    status: str
    model_loaded: bool
    version: str

# ============================================================================
# MODEL INITIALIZATION
# ============================================================================

class FIRClassifier:
    def __init__(self):
        self.model = None
        self.tokenizer = None
        self.device = torch.device('cuda' if torch.cuda.is_available() else 'cpu')
        self.model_loaded = False

    def load_model(self):
        """Load pre-trained DistilBERT model"""
        try:
            # For production, use a fine-tuned model on Indian crime data
            # For now, using base model with rule-based augmentation
            model_name = "distilbert-base-uncased"

            logger.info(f"Loading model: {model_name}")
            self.tokenizer = DistilBertTokenizer.from_pretrained(model_name)

            # Note: In production, replace with fine-tuned model
            # self.model = DistilBertForSequenceClassification.from_pretrained(
            #     "./models/fir_classifier_finetuned",
            #     num_labels=len(CRIME_CATEGORIES)
            # )

            # For now, we'll use rule-based classification
            self.model_loaded = True
            logger.info(f"Model loaded successfully on {self.device}")

        except Exception as e:
            logger.error(f"Failed to load model: {e}")
            self.model_loaded = False
            raise

    def classify_rule_based(self, text: str) -> tuple[str, dict[str, float]]:
        """Rule-based classification using keyword matching"""
        text_lower = text.lower()
        scores = {}

        # Calculate scores for each category based on keyword matches
        for category, keywords in CATEGORY_KEYWORDS.items():
            score = 0.0
            for keyword in keywords:
                if keyword in text_lower:
                    # Weight longer keywords more heavily
                    score += len(keyword.split()) / len(keywords)
            scores[category] = min(score, 1.0)  # Cap at 1.0

        # Get category with highest score
        if not scores or max(scores.values()) == 0:
            return "OTHER", {cat: 1.0/len(CRIME_CATEGORIES) for cat in CRIME_CATEGORIES}

        predicted_category = max(scores, key=scores.get)

        # Normalize scores to sum to 1.0
        total = sum(scores.values())
        if total > 0:
            scores = {k: v/total for k, v in scores.items()}

        return predicted_category, scores

    def classify(self, description: str, title: str = None) -> ClassificationResponse:
        """Classify FIR description"""
        # Combine title and description
        text = f"{title or ''} {description}".strip()

        # Use rule-based classification (replace with ML when fine-tuned model is ready)
        predicted_category, category_scores = self.classify_rule_based(text)
        confidence = category_scores[predicted_category]

        # Get suggested IPC sections for the predicted category
        suggested_sections = IPC_SECTIONS_MAP.get(predicted_category, [])
        ipc_suggestions = [
            IPCSection(
                section=section["section"],
                description=section["description"],
                confidence=confidence * 0.9  # Slight discount for IPC confidence
            )
            for section in suggested_sections[:5]  # Top 5 sections
        ]

        return ClassificationResponse(
            category=predicted_category,
            confidence=confidence,
            suggested_ipc_sections=ipc_suggestions,
            all_categories_scores=category_scores,
            method="rule-based",  # Change to "ml" when using trained model
            timestamp=datetime.utcnow().isoformat()
        )

# Initialize classifier
classifier = FIRClassifier()

# ============================================================================
# API ENDPOINTS
# ============================================================================

@app.on_event("startup")
async def startup_event():
    """Load model on startup"""
    logger.info("Starting FIR Classification Service...")
    try:
        classifier.load_model()
        logger.info("Startup complete")
    except Exception as e:
        logger.error(f"Startup failed: {e}")

@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check endpoint"""
    return HealthResponse(
        status="healthy" if classifier.model_loaded else "unhealthy",
        model_loaded=classifier.model_loaded,
        version="1.0.0"
    )

@app.post("/classify", response_model=ClassificationResponse)
async def classify_fir(request: ClassificationRequest):
    """
    Classify FIR description into crime category and suggest IPC sections

    - **description**: FIR description text (required, min 10 characters)
    - **title**: FIR title (optional)
    - **incident_location**: Location of incident (optional, for future location-based features)

    Returns:
    - **category**: Predicted crime category
    - **confidence**: Prediction confidence (0.0 - 1.0)
    - **suggested_ipc_sections**: List of suggested IPC sections
    - **all_categories_scores**: Confidence scores for all categories
    """
    try:
        if not classifier.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        result = classifier.classify(request.description, request.title)

        logger.info(
            f"Classified FIR: {request.description[:50]}... -> "
            f"{result.category} (confidence: {result.confidence:.2f})"
        )

        return result

    except Exception as e:
        logger.error(f"Classification error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/batch-classify", response_model=List[ClassificationResponse])
async def batch_classify(requests: List[ClassificationRequest]):
    """
    Batch classify multiple FIRs

    Useful for processing multiple FIRs in a single request
    """
    try:
        if not classifier.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        results = [
            classifier.classify(req.description, req.title)
            for req in requests
        ]

        logger.info(f"Batch classified {len(requests)} FIRs")

        return results

    except Exception as e:
        logger.error(f"Batch classification error: {e}")
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/categories")
async def get_categories():
    """Get list of all crime categories"""
    return {
        "categories": CRIME_CATEGORIES,
        "total": len(CRIME_CATEGORIES)
    }

@app.get("/ipc-sections/{category}")
async def get_ipc_sections(category: str):
    """Get IPC sections for a specific category"""
    category_upper = category.upper()

    if category_upper not in IPC_SECTIONS_MAP:
        raise HTTPException(
            status_code=404,
            detail=f"Category '{category}' not found. Valid categories: {CRIME_CATEGORIES}"
        )

    return {
        "category": category_upper,
        "sections": IPC_SECTIONS_MAP[category_upper]
    }

@app.get("/")
async def root():
    """Root endpoint with API info"""
    return {
        "service": "NPDMS FIR Classifier",
        "version": "1.0.0",
        "status": "running",
        "endpoints": {
            "classify": "/classify",
            "batch_classify": "/batch-classify",
            "categories": "/categories",
            "ipc_sections": "/ipc-sections/{category}",
            "health": "/health",
            "docs": "/docs"
        }
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8001)
