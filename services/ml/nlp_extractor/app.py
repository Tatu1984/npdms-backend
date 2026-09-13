"""
NLP Entity Extraction Service for NPDMS
Extract structured information from unstructured FIR narratives
Uses spaCy for NER and regex patterns for Indian-specific entities
"""

from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from typing import List, Dict, Optional, Any
import re
import logging
from datetime import datetime

# Setup logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(
    title="NPDMS NLP Entity Extractor",
    description="Extract structured information from FIR narratives",
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

# ============================================================================
# INDIAN-SPECIFIC PATTERNS
# ============================================================================

# Phone numbers (Indian format)
PHONE_PATTERN = re.compile(r'\b(?:\+91[-\s]?)?[6-9]\d{9}\b')

# Aadhaar number (12 digits, often spaced)
AADHAAR_PATTERN = re.compile(r'\b\d{4}[\s-]?\d{4}[\s-]?\d{4}\b')

# PAN number
PAN_PATTERN = re.compile(r'\b[A-Z]{5}\d{4}[A-Z]\b')

# Vehicle registration (Indian format)
VEHICLE_PATTERN = re.compile(
    r'\b[A-Z]{2}[-\s]?\d{1,2}[-\s]?[A-Z]{1,3}[-\s]?\d{4}\b',
    re.IGNORECASE
)

# Indian currency amounts
CURRENCY_PATTERN = re.compile(
    r'(?:Rs\.?|INR|₹)\s*(\d{1,3}(?:,\d{2,3})*(?:\.\d{2})?|\d+(?:\.\d{2})?)\s*(?:lakh|lac|crore)?',
    re.IGNORECASE
)

# Date patterns (various Indian formats)
DATE_PATTERNS = [
    re.compile(r'\b(\d{1,2})[/-](\d{1,2})[/-](\d{2,4})\b'),  # dd/mm/yyyy or dd-mm-yyyy
    re.compile(r'\b(\d{1,2})\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+(\d{4})\b', re.IGNORECASE),
    re.compile(r'\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+(\d{1,2}),?\s+(\d{4})\b', re.IGNORECASE),
]

# Time patterns
TIME_PATTERN = re.compile(r'\b(\d{1,2})[:\.](\d{2})\s*(AM|PM|am|pm|hrs|hours)?\b')

# FIR number pattern
FIR_PATTERN = re.compile(r'\bFIR[/\s-]?(?:No\.?)?[/\s-]?(\d{4}[/\s-]?\d+)\b', re.IGNORECASE)

# Police station pattern
PS_PATTERN = re.compile(r'\b(?:P\.?S\.?|Police\s+Station)[:\s]+([A-Za-z\s]+?)(?:,|\.|$)', re.IGNORECASE)

# Indian name titles
NAME_TITLES = ['Mr.', 'Mrs.', 'Ms.', 'Shri', 'Smt.', 'Kumari', 'Dr.', 'Sri', 'Sh.']

# Common Indian locations keywords
LOCATION_KEYWORDS = [
    'street', 'road', 'nagar', 'colony', 'layout', 'extension', 'ext',
    'cross', 'main', 'sector', 'block', 'phase', 'stage',
    'village', 'taluk', 'district', 'mandal', 'near', 'opposite',
    'beside', 'behind', 'front', 'lane', 'galli', 'mohalla', 'area'
]

# Relationship keywords
RELATIONSHIP_KEYWORDS = {
    'complainant': ['complainant', 'informant', 'victim', 'aggrieved', 'deponent'],
    'accused': ['accused', 'suspect', 'perpetrator', 'offender', 'criminal'],
    'witness': ['witness', 'eyewitness', 'saw', 'observed'],
    'family': ['father', 'mother', 'son', 'daughter', 'husband', 'wife', 'brother', 'sister', 'uncle', 'aunt']
}

# Property/item keywords
PROPERTY_KEYWORDS = [
    'mobile', 'phone', 'laptop', 'computer', 'gold', 'jewellery', 'jewelry',
    'chain', 'ring', 'necklace', 'bangle', 'cash', 'money', 'wallet', 'purse',
    'bag', 'vehicle', 'bike', 'motorcycle', 'scooter', 'car', 'auto',
    'watch', 'camera', 'tv', 'television', 'electronics'
]

# ============================================================================
# MODELS
# ============================================================================

class EntityExtraction(BaseModel):
    """Single extracted entity"""
    type: str
    value: str
    confidence: float = 1.0
    start: int = 0
    end: int = 0
    context: str = ""

class PersonEntity(BaseModel):
    """Person entity with details"""
    name: str
    role: str = "unknown"  # complainant, accused, witness, etc.
    age: Optional[str] = None
    gender: Optional[str] = None
    phone: Optional[str] = None
    address: Optional[str] = None
    id_type: Optional[str] = None
    id_number: Optional[str] = None
    relation: Optional[str] = None

class LocationEntity(BaseModel):
    """Location entity"""
    full_address: str
    area: Optional[str] = None
    landmark: Optional[str] = None
    city: Optional[str] = None
    district: Optional[str] = None
    state: Optional[str] = None
    pincode: Optional[str] = None

class IncidentEntity(BaseModel):
    """Incident details"""
    date: Optional[str] = None
    time: Optional[str] = None
    time_approximate: bool = False
    location: Optional[LocationEntity] = None

class PropertyEntity(BaseModel):
    """Property/item involved"""
    item: str
    description: Optional[str] = None
    quantity: Optional[int] = None
    value: Optional[str] = None

class ExtractedFIRData(BaseModel):
    """Complete extracted FIR data"""
    complainant: Optional[PersonEntity] = None
    accused: List[PersonEntity] = []
    witnesses: List[PersonEntity] = []
    incident: Optional[IncidentEntity] = None
    property_items: List[PropertyEntity] = []
    vehicles: List[str] = []
    phone_numbers: List[str] = []
    currency_amounts: List[str] = []
    aadhaar_numbers: List[str] = []
    pan_numbers: List[str] = []
    raw_entities: List[EntityExtraction] = []
    summary: str = ""

class ExtractionRequest(BaseModel):
    """Request model for extraction"""
    text: str = Field(..., min_length=10, description="FIR narrative text")
    extract_summary: bool = Field(True, description="Generate a summary")

class ExtractionResponse(BaseModel):
    """Response model for extraction"""
    success: bool
    data: ExtractedFIRData
    processing_time_ms: float
    word_count: int
    timestamp: str

class HealthResponse(BaseModel):
    status: str
    service: str
    version: str

# ============================================================================
# EXTRACTION FUNCTIONS
# ============================================================================

def extract_phone_numbers(text: str) -> List[EntityExtraction]:
    """Extract Indian phone numbers"""
    entities = []
    for match in PHONE_PATTERN.finditer(text):
        phone = match.group().replace('-', '').replace(' ', '')
        if phone.startswith('+91'):
            phone = phone[3:]
        entities.append(EntityExtraction(
            type="PHONE",
            value=phone,
            start=match.start(),
            end=match.end(),
            context=text[max(0, match.start()-20):min(len(text), match.end()+20)]
        ))
    return entities

def extract_aadhaar_numbers(text: str) -> List[EntityExtraction]:
    """Extract Aadhaar numbers"""
    entities = []
    for match in AADHAAR_PATTERN.finditer(text):
        aadhaar = match.group().replace('-', '').replace(' ', '')
        entities.append(EntityExtraction(
            type="AADHAAR",
            value=aadhaar,
            start=match.start(),
            end=match.end()
        ))
    return entities

def extract_pan_numbers(text: str) -> List[EntityExtraction]:
    """Extract PAN numbers"""
    entities = []
    for match in PAN_PATTERN.finditer(text):
        entities.append(EntityExtraction(
            type="PAN",
            value=match.group().upper(),
            start=match.start(),
            end=match.end()
        ))
    return entities

def extract_vehicle_numbers(text: str) -> List[EntityExtraction]:
    """Extract Indian vehicle registration numbers"""
    entities = []
    for match in VEHICLE_PATTERN.finditer(text):
        vehicle = match.group().upper().replace(' ', '-')
        entities.append(EntityExtraction(
            type="VEHICLE",
            value=vehicle,
            start=match.start(),
            end=match.end()
        ))
    return entities

def extract_currency_amounts(text: str) -> List[EntityExtraction]:
    """Extract currency amounts"""
    entities = []
    for match in CURRENCY_PATTERN.finditer(text):
        entities.append(EntityExtraction(
            type="CURRENCY",
            value=match.group(),
            start=match.start(),
            end=match.end()
        ))
    return entities

def extract_dates(text: str) -> List[EntityExtraction]:
    """Extract dates from text"""
    entities = []
    for pattern in DATE_PATTERNS:
        for match in pattern.finditer(text):
            entities.append(EntityExtraction(
                type="DATE",
                value=match.group(),
                start=match.start(),
                end=match.end()
            ))
    return entities

def extract_times(text: str) -> List[EntityExtraction]:
    """Extract times from text"""
    entities = []
    for match in TIME_PATTERN.finditer(text):
        entities.append(EntityExtraction(
            type="TIME",
            value=match.group(),
            start=match.start(),
            end=match.end()
        ))
    return entities

def extract_names_heuristic(text: str) -> List[EntityExtraction]:
    """Extract names using heuristics (title + capitalized words)"""
    entities = []

    # Pattern 1: Title followed by name
    for title in NAME_TITLES:
        pattern = re.compile(rf'\b{re.escape(title)}\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+)*)')
        for match in pattern.finditer(text):
            entities.append(EntityExtraction(
                type="PERSON",
                value=f"{title} {match.group(1)}",
                start=match.start(),
                end=match.end()
            ))

    # Pattern 2: "name:", "named", "son/daughter of" patterns
    name_patterns = [
        re.compile(r'(?:named?|name is)\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+)*)', re.IGNORECASE),
        re.compile(r'(?:son|daughter|wife|husband)\s+of\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+)*)', re.IGNORECASE),
        re.compile(r'(?:s/o|d/o|w/o|h/o)\s+([A-Z][a-z]+(?:\s+[A-Z][a-z]+)*)', re.IGNORECASE),
    ]

    for pattern in name_patterns:
        for match in pattern.finditer(text):
            entities.append(EntityExtraction(
                type="PERSON",
                value=match.group(1),
                start=match.start(),
                end=match.end()
            ))

    return entities

def extract_property_items(text: str) -> List[PropertyEntity]:
    """Extract property items mentioned"""
    items = []
    text_lower = text.lower()

    for keyword in PROPERTY_KEYWORDS:
        if keyword in text_lower:
            # Try to find quantity and value near the keyword
            pattern = re.compile(
                rf'(\d+\s+)?{keyword}s?\b[^.]*?(?:worth|valued?\s+at|costing?)?\s*(Rs\.?\s*\d+[,\d]*)?',
                re.IGNORECASE
            )
            for match in pattern.finditer(text):
                items.append(PropertyEntity(
                    item=keyword.capitalize(),
                    description=match.group().strip(),
                    quantity=int(match.group(1)) if match.group(1) else 1,
                    value=match.group(2) if match.group(2) else None
                ))

    return items

def determine_role(context: str) -> str:
    """Determine the role of a person based on surrounding context"""
    context_lower = context.lower()

    for role, keywords in RELATIONSHIP_KEYWORDS.items():
        for keyword in keywords:
            if keyword in context_lower:
                return role

    return "unknown"

def generate_summary(text: str, entities: ExtractedFIRData) -> str:
    """Generate a brief summary of the FIR"""
    parts = []

    if entities.complainant:
        parts.append(f"Complainant: {entities.complainant.name}")

    if entities.incident and entities.incident.date:
        parts.append(f"Date: {entities.incident.date}")

    if entities.incident and entities.incident.location:
        parts.append(f"Location: {entities.incident.location.full_address}")

    if entities.accused:
        accused_names = [a.name for a in entities.accused]
        parts.append(f"Accused: {', '.join(accused_names)}")

    if entities.property_items:
        items = [p.item for p in entities.property_items]
        parts.append(f"Property involved: {', '.join(items)}")

    if entities.currency_amounts:
        parts.append(f"Amount: {', '.join(entities.currency_amounts)}")

    return " | ".join(parts) if parts else "Unable to extract key details"

# ============================================================================
# MAIN EXTRACTION FUNCTION
# ============================================================================

def extract_fir_entities(text: str, extract_summary: bool = True) -> ExtractedFIRData:
    """Main function to extract all entities from FIR text"""

    all_entities = []

    # Extract all entity types
    phones = extract_phone_numbers(text)
    all_entities.extend(phones)

    aadhaar = extract_aadhaar_numbers(text)
    all_entities.extend(aadhaar)

    pan = extract_pan_numbers(text)
    all_entities.extend(pan)

    vehicles = extract_vehicle_numbers(text)
    all_entities.extend(vehicles)

    currency = extract_currency_amounts(text)
    all_entities.extend(currency)

    dates = extract_dates(text)
    all_entities.extend(dates)

    times = extract_times(text)
    all_entities.extend(times)

    persons = extract_names_heuristic(text)
    all_entities.extend(persons)

    property_items = extract_property_items(text)

    # Build structured data
    data = ExtractedFIRData(
        phone_numbers=[e.value for e in phones],
        aadhaar_numbers=[e.value for e in aadhaar],
        pan_numbers=[e.value for e in pan],
        vehicles=[e.value for e in vehicles],
        currency_amounts=[e.value for e in currency],
        property_items=property_items,
        raw_entities=all_entities
    )

    # Try to identify complainant and accused from person entities
    for person in persons:
        role = determine_role(person.context or text[max(0, person.start-50):min(len(text), person.end+50)])
        person_entity = PersonEntity(name=person.value, role=role)

        if role == "complainant" and not data.complainant:
            data.complainant = person_entity
        elif role == "accused":
            data.accused.append(person_entity)
        elif role == "witness":
            data.witnesses.append(person_entity)

    # Set up incident data
    if dates or times:
        location = None
        data.incident = IncidentEntity(
            date=dates[0].value if dates else None,
            time=times[0].value if times else None,
            location=location
        )

    # Generate summary
    if extract_summary:
        data.summary = generate_summary(text, data)

    return data

# ============================================================================
# ENDPOINTS
# ============================================================================

@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Health check endpoint"""
    return HealthResponse(
        status="healthy",
        service="nlp-extractor",
        version="1.0.0"
    )

@app.post("/extract", response_model=ExtractionResponse)
async def extract_entities(request: ExtractionRequest):
    """
    Extract structured information from FIR narrative text

    This endpoint analyzes the given text and extracts:
    - Phone numbers (Indian format)
    - Aadhaar numbers
    - PAN numbers
    - Vehicle registration numbers
    - Currency amounts
    - Dates and times
    - Person names with roles (complainant, accused, witness)
    - Property items

    Args:
        text: FIR narrative text (min 10 characters)
        extract_summary: Whether to generate a summary

    Returns:
        Structured FIR data with all extracted entities
    """
    start_time = datetime.now()

    try:
        data = extract_fir_entities(request.text, request.extract_summary)

        processing_time = (datetime.now() - start_time).total_seconds() * 1000

        logger.info(f"Extracted {len(data.raw_entities)} entities from text ({len(request.text.split())} words)")

        return ExtractionResponse(
            success=True,
            data=data,
            processing_time_ms=processing_time,
            word_count=len(request.text.split()),
            timestamp=datetime.utcnow().isoformat()
        )

    except Exception as e:
        logger.error(f"Extraction error: {e}")
        raise HTTPException(status_code=500, detail=f"Extraction failed: {str(e)}")

@app.get("/")
async def root():
    """Root endpoint"""
    return {
        "service": "NPDMS NLP Entity Extractor",
        "version": "1.0.0",
        "endpoints": {
            "/health": "Health check",
            "/extract": "Extract entities from FIR text",
            "/docs": "API documentation"
        },
        "supported_entities": [
            "PHONE (Indian mobile numbers)",
            "AADHAAR (12-digit Aadhaar numbers)",
            "PAN (PAN card numbers)",
            "VEHICLE (Indian vehicle registration)",
            "CURRENCY (Indian Rupee amounts)",
            "DATE (Various date formats)",
            "TIME (Time expressions)",
            "PERSON (Names with roles)",
            "PROPERTY (Items involved in crime)"
        ]
    }

# ============================================================================
# STARTUP
# ============================================================================

@app.on_event("startup")
async def startup_event():
    """Initialize service on startup"""
    logger.info("Starting NLP Entity Extractor service...")
    logger.info("Using regex-based extraction (spaCy integration available for production)")
    logger.info("NLP Entity Extractor ready!")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8005)
