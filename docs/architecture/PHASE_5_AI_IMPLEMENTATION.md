# PHASE 5 — AI IMPLEMENTATION
# National Police Department Management System (NPDMS)

## Document Control
- **Version**: 1.0
- **Classification**: RESTRICTED
- **Author**: AI Architecture Team
- **Last Updated**: 2026-01-04

---

## CRITICAL PRINCIPLE: AI IS ASSISTIVE ONLY

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     AI GOVERNANCE MANDATE                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. AI NEVER MAKES DECISIONS — only provides recommendations                │
│  2. ALL AI outputs require HUMAN VERIFICATION before action                 │
│  3. AI suggestions MUST be explainable and auditable                        │
│  4. Officers can ALWAYS override or reject AI recommendations               │
│  5. No enforcement action shall be based solely on AI output                │
│  6. Bias monitoring is MANDATORY for all deployed models                    │
│  7. All AI interactions are logged for legal defensibility                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 1. Handwritten OCR

### 1.1 Capability Overview

**Purpose**: Extract text from handwritten FIR registers, station diaries, and historical documents.

**Languages Supported**:
- Hindi (Devanagari script)
- English
- Tamil
- Telugu
- Kannada
- Malayalam
- Marathi
- Bengali
- Gujarati
- Punjabi (Gurmukhi)
- Odia
- Urdu (Perso-Arabic)

### 1.2 Technical Specification

| Attribute | Specification |
|-----------|---------------|
| **Input Data** | Scanned images (JPEG, PNG, TIFF), PDF documents |
| **Input Resolution** | Minimum 150 DPI, recommended 300 DPI |
| **Model Class** | CNN + Transformer hybrid (TrOCR architecture) |
| **Inference Location** | Edge (Station/District) |
| **Model Size** | ~200MB per language (quantized INT8) |
| **Latency Target** | < 3 seconds per page on CPU |
| **Accuracy Target** | > 85% character accuracy for clean documents |

### 1.3 Model Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      HANDWRITTEN OCR PIPELINE                                │
└─────────────────────────────────────────────────────────────────────────────┘

INPUT IMAGE
     │
     ▼
┌─────────────────────────────────────┐
│ 1. PREPROCESSING                    │
│    • Deskew (correct rotation)      │
│    • Denoise (bilateral filter)     │
│    • Binarization (adaptive)        │
│    • Line segmentation              │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ 2. LANGUAGE DETECTION               │
│    • Script classifier (CNN)        │
│    • Route to language model        │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ 3. OCR MODEL (per language)         │
│    • Vision Encoder (DeiT)          │
│    • Text Decoder (GPT-2 variant)   │
│    • Beam search decoding           │
└─────────────────────────────────────┘
     │
     ▼
┌─────────────────────────────────────┐
│ 4. POST-PROCESSING                  │
│    • Legal dictionary correction    │
│    • IPC section formatting         │
│    • Name/date normalization        │
└─────────────────────────────────────┘
     │
     ▼
OUTPUT: Structured text + confidence scores
```

### 1.4 Human Validation Points

| Checkpoint | Trigger Condition | Action Required |
|------------|-------------------|-----------------|
| Low Confidence | Overall confidence < 70% | Full manual review required |
| Partial Confidence | 70% ≤ confidence < 85% | Highlight uncertain sections |
| Legal Terms | Any IPC section detected | Officer must verify section numbers |
| Names/Dates | Extracted names or dates | Officer confirms entities |
| Special Characters | Symbols or numbers detected | Manual verification |

### 1.5 Error Handling

```python
class OCRResult:
    text: str
    confidence: float
    language: str
    segments: List[TextSegment]
    errors: List[OCRError]
    requires_review: bool
    review_reasons: List[str]

class OCRError:
    type: str  # LOW_QUALITY, UNRECOGNIZED_SCRIPT, PROCESSING_FAILED
    message: str
    segment_index: Optional[int]
    fallback_action: str

# Error handling flow
def process_document(image: Image) -> OCRResult:
    try:
        preprocessed = preprocess(image)
        if preprocessed.quality_score < 0.3:
            return OCRResult(
                text="",
                confidence=0.0,
                requires_review=True,
                errors=[OCRError(
                    type="LOW_QUALITY",
                    message="Image quality too low for reliable extraction",
                    fallback_action="REQUEST_RESCAN"
                )]
            )

        result = ocr_model.predict(preprocessed)

        # Always flag for review - AI never final
        result.requires_review = True
        result.review_reasons.append("AI extraction requires human verification")

        return result

    except Exception as e:
        log_error(e)
        return OCRResult(
            text="",
            confidence=0.0,
            requires_review=True,
            errors=[OCRError(
                type="PROCESSING_FAILED",
                message=str(e),
                fallback_action="MANUAL_ENTRY"
            )]
        )
```

### 1.6 Bias Mitigation

| Bias Type | Detection Method | Mitigation |
|-----------|-----------------|------------|
| Script Bias | Per-script accuracy metrics | Balanced training data across scripts |
| Regional Dialect | Accuracy by state/region | Include regional vocabulary in lexicons |
| Document Age | Accuracy by document era | Train on historical samples |
| Writing Style | Accuracy by style (cursive/print) | Multi-style training data |

### 1.7 Audit & Explainability

```json
{
  "request_id": "ocr-20260104-143000-abc123",
  "timestamp": "2026-01-04T14:30:00Z",
  "user_id": "officer-uuid",
  "station_id": "station-uuid",
  "input": {
    "file_hash": "sha256:a1b2c3...",
    "file_size": 2456789,
    "detected_dpi": 300
  },
  "processing": {
    "language_detected": "hi",
    "language_confidence": 0.95,
    "preprocessing_applied": ["deskew", "denoise", "binarize"],
    "model_version": "trocr-hindi-v2.1"
  },
  "output": {
    "text_length": 1234,
    "overall_confidence": 0.82,
    "low_confidence_segments": [12, 45, 78],
    "entities_extracted": {
      "ipc_sections": ["379", "380"],
      "names": ["राजेश कुमार"],
      "dates": ["04-01-2026"]
    }
  },
  "human_review": {
    "required": true,
    "reasons": ["confidence_below_threshold", "ipc_sections_detected"],
    "reviewed_by": null,
    "reviewed_at": null,
    "corrections_made": null
  }
}
```

---

## 2. FIR Natural Language Processing

### 2.1 Capability Overview

**Purpose**:
- Extract structured entities from FIR narratives
- Suggest applicable IPC sections
- Detect similar cases
- Translate between Indian languages

### 2.2 Technical Specification

| Attribute | Specification |
|-----------|---------------|
| **Input Data** | Free-form FIR text (typed, transcribed, or OCR output) |
| **Model Class** | Transformer-based NER + Multi-label classifier |
| **Inference Location** | Edge (basic), State (advanced) |
| **Model Size** | 500MB (quantized) |
| **Latency Target** | < 2 seconds for entity extraction |

### 2.3 Entity Extraction

**Entities Extracted**:

| Entity Type | Examples | Use Case |
|-------------|----------|----------|
| PERSON | "Rajesh Kumar", "श्री मोहन लाल" | Complainant, accused, witness identification |
| LOCATION | "4th Block Koramangala", "MG Road" | Incident mapping, jurisdiction |
| DATE | "04 January 2026", "कल रात" | Timeline construction |
| TIME | "approximately 2:30 PM", "रात 10 बजे" | Timeline construction |
| VEHICLE | "KA-01-AB-1234", "white Maruti Swift" | Vehicle tracking |
| PHONE | "9876543210" | Contact tracing |
| AADHAAR | "XXXX-XXXX-1234" (masked) | Identity verification |
| IPC_SECTION | "Section 379", "धारा 302" | Legal classification |
| AMOUNT | "Rs. 50,000", "₹1,50,000" | Property valuation |
| WEAPON | "knife", "pistol", "iron rod" | Evidence categorization |
| ORGANIZATION | "State Bank of India", "XYZ Company" | Institutional involvement |

### 2.4 IPC Section Suggestion

```python
class IPCSuggestionService:
    def __init__(self):
        self.classifier = load_model("ipc_classifier_v3.onnx")
        self.ipc_database = load_ipc_database()
        self.threshold = 0.3  # Minimum confidence to suggest

    def suggest_sections(self, fir_text: str) -> List[IPCSuggestion]:
        """
        Suggest applicable IPC sections based on FIR content.

        Returns suggestions with confidence scores and explanations.
        """
        # Encode text
        embeddings = self.encode(fir_text)

        # Multi-label classification
        predictions = self.classifier.predict(embeddings)

        suggestions = []
        for idx, confidence in enumerate(predictions):
            if confidence >= self.threshold:
                section = self.ipc_database[idx]
                suggestions.append(IPCSuggestion(
                    section_number=section.number,
                    section_title=section.title,
                    confidence=confidence,
                    explanation=self._generate_explanation(fir_text, section),
                    keywords_matched=self._find_keywords(fir_text, section),
                    is_cognizable=section.is_cognizable,
                    is_bailable=section.is_bailable,
                    max_punishment=section.max_punishment
                ))

        # Sort by confidence
        suggestions.sort(key=lambda x: x.confidence, reverse=True)

        # Always add disclaimer
        for s in suggestions:
            s.disclaimer = "AI suggestion. Legal verification by officer mandatory."

        return suggestions[:10]  # Top 10 suggestions

    def _generate_explanation(self, text: str, section: IPCSection) -> str:
        """Generate human-readable explanation for suggestion."""
        matched_keywords = self._find_keywords(text, section)
        if matched_keywords:
            return f"Suggested because the FIR mentions: {', '.join(matched_keywords[:3])}"

        return f"Contextual match with '{section.title}' based on incident description"
```

### 2.5 Similar Case Detection

```python
class SimilarCaseDetector:
    def __init__(self, vector_db: VectorDB, embedding_model: str):
        self.vector_db = vector_db
        self.embedder = SentenceTransformer(embedding_model)

    def find_similar(
        self,
        fir_text: str,
        jurisdiction: Jurisdiction,
        filters: Optional[SearchFilters] = None
    ) -> List[SimilarCase]:
        """
        Find similar cases within jurisdiction.

        Similarity based on:
        - Modus operandi
        - Crime type
        - Location proximity
        - Temporal patterns
        """
        # Generate embedding
        embedding = self.embedder.encode(fir_text)

        # Build filter query
        filter_query = {
            "jurisdiction": jurisdiction.to_dict(),
            "status": {"$ne": "CLOSED_FALSE_REPORT"}  # Exclude false reports
        }

        if filters:
            if filters.date_range:
                filter_query["incident_date"] = {
                    "$gte": filters.date_range.start,
                    "$lte": filters.date_range.end
                }
            if filters.crime_types:
                filter_query["crime_type"] = {"$in": filters.crime_types}

        # Search vector database
        results = self.vector_db.search(
            vector=embedding,
            filter=filter_query,
            top_k=10,
            min_score=0.6  # Minimum similarity threshold
        )

        similar_cases = []
        for result in results:
            similar_cases.append(SimilarCase(
                fir_number=result.metadata["fir_number"],
                station=result.metadata["station"],
                similarity_score=result.score,
                summary=result.metadata["summary"],
                ipc_sections=result.metadata["ipc_sections"],
                status=result.metadata["status"],
                modus_operandi=result.metadata.get("modus_operandi"),
                common_elements=self._find_common_elements(fir_text, result),
                disclaimer="AI-detected similarity. Investigate manually to confirm connection."
            ))

        return similar_cases
```

### 2.6 Human Validation Points

| Feature | Validation Required | Reason |
|---------|---------------------|--------|
| Entity Extraction | Officer review | Ensure accuracy of names, dates, locations |
| IPC Suggestions | Legal officer sign-off | Legal implications of section selection |
| Similar Cases | Investigation officer review | Prevent false correlations |
| Translations | Bilingual officer verification | Ensure legal accuracy |

### 2.7 Bias Mitigation

| Bias Type | Risk | Mitigation |
|-----------|------|------------|
| Geographic | Over-suggestion based on crime-prone areas | Blind location data during classification |
| Demographic | Name-based profiling | Remove demographic identifiers before classification |
| Historical | Bias from historical case outcomes | Regular retraining with balanced datasets |
| Language | Accuracy variance across languages | Per-language accuracy monitoring, balanced training |

### 2.8 Audit Trail

```json
{
  "request_id": "nlp-20260104-143500-def456",
  "timestamp": "2026-01-04T14:35:00Z",
  "user_id": "officer-uuid",
  "operation": "FIR_STRUCTURING",
  "input": {
    "text_hash": "sha256:d4e5f6...",
    "text_length": 2500,
    "language": "hi"
  },
  "entities_extracted": {
    "persons": [
      {"text": "राजेश कुमार", "role": "COMPLAINANT", "confidence": 0.92}
    ],
    "locations": [
      {"text": "कोरमंगला 4th ब्लॉक", "confidence": 0.88}
    ],
    "ipc_sections_suggested": [
      {"section": "379", "confidence": 0.85, "accepted": null},
      {"section": "380", "confidence": 0.62, "accepted": null}
    ]
  },
  "similar_cases": [
    {"fir_number": "KOR/2025/02345", "similarity": 0.78}
  ],
  "model_versions": {
    "ner": "indic-ner-v2.3",
    "ipc_classifier": "ipc-multi-v3.1",
    "embedder": "legal-bert-v1.2"
  },
  "human_review": {
    "required": true,
    "reviewed_by": null,
    "entities_corrected": null,
    "sections_accepted": null,
    "sections_rejected": null
  }
}
```

---

## 3. Evidence Computer Vision

### 3.1 Capability Overview

**Purpose**:
- Classify evidence images
- Detect weapons and objects
- Extract key frames from CCTV footage
- Generate video summaries

### 3.2 Technical Specification

| Attribute | Specification |
|-----------|---------------|
| **Input Data** | Images (JPEG, PNG), Videos (MP4, AVI) |
| **Model Class** | YOLO v8 (detection), CNN (classification) |
| **Inference Location** | District (GPU), Edge (CPU for basic) |
| **Model Size** | 50MB (YOLOv8n), 200MB (YOLOv8m) |
| **Latency Target** | < 500ms per image, real-time video (30fps on GPU) |

### 3.3 Detection Categories

| Category | Objects Detected | Use Case |
|----------|-----------------|----------|
| **Weapons** | Knives, firearms, rods, blunt objects | Violent crime evidence |
| **Vehicles** | Cars, bikes, trucks (with plates) | Vehicle-related crimes |
| **Documents** | ID cards, papers, money | Fraud, identity crimes |
| **Electronics** | Phones, laptops, storage devices | Cyber crime evidence |
| **Contraband** | Drug paraphernalia, counterfeit items | Narcotics, economic crimes |
| **Persons** | Face detection, body posture | Suspect identification |

### 3.4 Implementation

```python
class EvidenceVisionService:
    def __init__(self):
        self.detector = YOLO("yolov8m-evidence.onnx")
        self.classifier = load_model("evidence_classifier.onnx")

    def analyze_image(self, image: Image) -> EvidenceAnalysis:
        """
        Analyze evidence image for objects and classification.
        """
        # Object detection
        detections = self.detector.predict(image)

        objects = []
        for det in detections:
            objects.append(DetectedObject(
                class_name=det.class_name,
                confidence=det.confidence,
                bounding_box=det.bbox,
                is_weapon=det.class_name in WEAPON_CLASSES,
                requires_attention=det.class_name in HIGH_PRIORITY_CLASSES
            ))

        # Overall classification
        classification = self.classifier.predict(image)

        return EvidenceAnalysis(
            objects=objects,
            primary_category=classification.top_category,
            category_confidence=classification.confidence,
            contains_weapon=any(o.is_weapon for o in objects),
            contains_person=any(o.class_name == "person" for o in objects),
            forensic_suggestions=self._suggest_forensics(objects),
            disclaimer="AI analysis. All detections require human verification."
        )

    def analyze_video(
        self,
        video_path: str,
        extract_keyframes: bool = True
    ) -> VideoAnalysis:
        """
        Analyze video evidence, extract key frames.
        """
        cap = cv2.VideoCapture(video_path)
        fps = cap.get(cv2.CAP_PROP_FPS)
        total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))

        keyframes = []
        all_detections = []

        # Process every Nth frame for efficiency
        sample_rate = max(1, int(fps / 2))  # 2 samples per second

        frame_idx = 0
        while cap.isOpened():
            ret, frame = cap.read()
            if not ret:
                break

            if frame_idx % sample_rate == 0:
                detections = self.detector.predict(frame)

                # Keyframe detection based on significant events
                if self._is_keyframe(detections, all_detections):
                    keyframes.append(KeyFrame(
                        frame_number=frame_idx,
                        timestamp=frame_idx / fps,
                        image=frame,
                        detections=detections,
                        reason=self._keyframe_reason(detections)
                    ))

                all_detections.extend(detections)

            frame_idx += 1

        cap.release()

        return VideoAnalysis(
            duration=total_frames / fps,
            keyframes=keyframes,
            summary=self._generate_summary(keyframes),
            unique_persons=self._count_unique_persons(all_detections),
            vehicles_detected=self._extract_vehicles(all_detections),
            disclaimer="AI-extracted frames. Manual review of full footage required for court."
        )

    def _is_keyframe(self, current: List, history: List) -> bool:
        """Determine if frame represents a significant event."""
        # New person appeared
        # Weapon detected
        # Significant motion
        # Scene change
        pass
```

### 3.5 Human Validation Points

| Detection | Validation Required | Action |
|-----------|---------------------|--------|
| Weapon Detection | Mandatory | Cannot proceed without officer confirmation |
| Face Detection | Mandatory | Privacy implications, must verify identity match |
| Vehicle Plate | Recommended | Verify OCR accuracy |
| Key Frame Selection | Recommended | Ensure important moments captured |

### 3.6 Error Handling

```python
class VisionError:
    UNPROCESSABLE_IMAGE = "Image cannot be processed (corrupted/unsupported format)"
    LOW_QUALITY = "Image quality too low for reliable analysis"
    VIDEO_CORRUPT = "Video file is corrupted or incomplete"
    MODEL_FAILURE = "AI model failed to process"
    TIMEOUT = "Processing exceeded time limit"

def safe_analyze(image: Image) -> EvidenceAnalysis:
    try:
        # Validate input
        if not is_valid_image(image):
            return EvidenceAnalysis(
                error=VisionError.UNPROCESSABLE_IMAGE,
                fallback="Manual classification required"
            )

        # Check quality
        quality_score = assess_quality(image)
        if quality_score < 0.3:
            return EvidenceAnalysis(
                error=VisionError.LOW_QUALITY,
                quality_score=quality_score,
                fallback="Request better quality image"
            )

        # Process with timeout
        result = timeout(30)(vision_service.analyze_image)(image)
        return result

    except TimeoutError:
        return EvidenceAnalysis(
            error=VisionError.TIMEOUT,
            fallback="Process in batch queue"
        )
    except Exception as e:
        log_error(e)
        return EvidenceAnalysis(
            error=VisionError.MODEL_FAILURE,
            fallback="Manual analysis required"
        )
```

### 3.7 Bias Mitigation

| Bias | Risk | Mitigation |
|------|------|------------|
| Racial bias in face detection | Lower accuracy for certain ethnicities | Test across Indian demographics, retrain if disparities |
| Weapon misclassification | Everyday objects classified as weapons | High threshold (0.9) for weapon alerts, mandatory review |
| Vehicle type bias | Better detection for common vehicles | Include diverse vehicle types in training |

---

## 4. Crime Prediction & Analytics

### 4.1 Capability Overview

**Purpose**:
- Identify crime hotspots
- Predict high-risk times/locations
- Optimize patrol routes
- Detect emerging patterns

### 4.2 Technical Specification

| Attribute | Specification |
|-----------|---------------|
| **Input Data** | Historical FIRs, time, location, weather, events |
| **Model Class** | Gradient Boosting + LSTM (temporal) |
| **Inference Location** | State/Central only |
| **Update Frequency** | Daily model refresh |
| **Prediction Horizon** | 7-day forecast |

### 4.3 CRITICAL CONSTRAINTS

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                   CRIME PREDICTION GUARDRAILS                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ❌ PROHIBITED USES:                                                        │
│     • Individual-level predictions ("person X will commit crime")           │
│     • Profiling based on demographics, religion, caste                      │
│     • Automated arrests or detentions                                       │
│     • Denying services based on predicted risk                              │
│                                                                             │
│  ✓ PERMITTED USES:                                                          │
│     • Area-level resource allocation ("Beat A needs more patrols")          │
│     • Temporal pattern detection ("thefts increase at night")               │
│     • Infrastructure planning ("need more streetlights here")               │
│     • Informing (not replacing) human decision-making                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.4 Prediction Model

```python
class CrimePredictionService:
    def __init__(self):
        self.model = load_model("crime_forecast_v2.onnx")
        self.feature_extractor = CrimeFeatureExtractor()

    def predict_hotspots(
        self,
        jurisdiction: Jurisdiction,
        prediction_date: date,
        crime_types: Optional[List[str]] = None
    ) -> HotspotPrediction:
        """
        Predict crime hotspots for a given date.

        NOTE: Predictions are for RESOURCE ALLOCATION only.
        Never for profiling individuals.
        """
        # Extract features
        features = self.feature_extractor.extract(
            jurisdiction=jurisdiction,
            target_date=prediction_date,
            historical_days=365,
            include_events=True,
            include_weather=True
        )

        # Grid-based prediction (500m x 500m cells)
        grid = jurisdiction.to_grid(cell_size=500)

        predictions = []
        for cell in grid:
            cell_features = features.for_cell(cell)
            risk_score = self.model.predict(cell_features)

            if risk_score > 0.5:  # Only report elevated risk
                predictions.append(HotspotCell(
                    cell_id=cell.id,
                    center_lat=cell.center_lat,
                    center_lng=cell.center_lng,
                    risk_score=risk_score,
                    risk_level=self._risk_level(risk_score),
                    contributing_factors=self._explain(cell_features, risk_score),
                    recommended_action=self._suggest_action(risk_score)
                ))

        return HotspotPrediction(
            jurisdiction=jurisdiction,
            prediction_date=prediction_date,
            hotspots=predictions,
            model_version="crime_forecast_v2",
            confidence_interval=0.95,
            disclaimer="Predictions are probabilistic estimates for resource planning. "
                       "Not for individual profiling or automated enforcement."
        )

    def _explain(self, features: Features, risk_score: float) -> List[str]:
        """Generate human-readable explanation for prediction."""
        explanations = []

        if features.historical_crime_rate > 0.8:
            explanations.append("Historically high crime area")
        if features.is_weekend:
            explanations.append("Weekend (historically higher incidents)")
        if features.nearby_event:
            explanations.append(f"Nearby event: {features.event_name}")
        if features.poor_lighting:
            explanations.append("Low street lighting coverage")

        return explanations
```

### 4.5 Patrol Optimization

```python
class PatrolOptimizer:
    def __init__(self, hotspot_service: CrimePredictionService):
        self.hotspot_service = hotspot_service

    def optimize_routes(
        self,
        jurisdiction: Jurisdiction,
        available_patrols: int,
        shift_duration: timedelta
    ) -> PatrolPlan:
        """
        Optimize patrol routes based on predicted hotspots.

        Returns suggested routes, NOT mandatory assignments.
        """
        # Get hotspots
        hotspots = self.hotspot_service.predict_hotspots(
            jurisdiction=jurisdiction,
            prediction_date=date.today()
        )

        # Solve vehicle routing problem
        routes = self._solve_vrp(
            hotspots=hotspots.hotspots,
            num_vehicles=available_patrols,
            time_limit=shift_duration
        )

        return PatrolPlan(
            routes=routes,
            coverage_score=self._calculate_coverage(routes, hotspots),
            estimated_response_time_improvement="15-20%",
            disclaimer="AI-suggested routes. SHO has final authority on deployment."
        )
```

### 4.6 Bias Mitigation (Critical)

| Bias Type | Detection Method | Mitigation |
|-----------|-----------------|------------|
| **Feedback Loop** | Compare prediction vs actual across areas | Regularly audit for self-reinforcing patterns |
| **Historical Bias** | Analyze prediction disparity by neighborhood SES | Weight features to reduce historical policing bias |
| **Temporal Bias** | Check if predictions track enforcement, not crime | Use victimization surveys to calibrate |
| **Geographic Bias** | Prediction accuracy across urban/rural | Separate models or balanced training |

### 4.7 Mandatory Audits

```yaml
# Quarterly bias audit requirements
bias_audit:
  frequency: quarterly
  metrics:
    - prediction_accuracy_by_district
    - false_positive_rate_by_area_type
    - resource_allocation_disparity
    - correlation_with_demographics  # Should be near zero

  thresholds:
    max_accuracy_disparity: 0.1  # 10% max difference between areas
    max_demographic_correlation: 0.2

  actions_on_failure:
    - Suspend predictions for affected areas
    - Trigger model retraining
    - Document and report to oversight committee
```

---

## 5. Graph Intelligence

### 5.1 Capability Overview

**Purpose**:
- Map criminal networks
- Identify key figures
- Detect communities/gangs
- Trace financial connections

### 5.2 Technical Specification

| Attribute | Specification |
|-----------|---------------|
| **Input Data** | Case records, accused relationships, financial data |
| **Database** | Neo4j 5 with Graph Data Science library |
| **Inference Location** | State/Central |
| **Update Frequency** | Real-time relationship updates |

### 5.3 Graph Schema

```cypher
// Node types
(:Person {
  id: UUID,
  name: String,
  aliases: [String],
  status: String,  // WANTED, ARRESTED, CONVICTED, ACQUITTED
  risk_score: Float
})

(:Case {
  id: UUID,
  fir_number: String,
  crime_type: String,
  status: String,
  date: Date
})

(:Location {
  id: UUID,
  name: String,
  type: String,  // RESIDENCE, WORKPLACE, INCIDENT
  coordinates: Point
})

(:Organization {
  id: UUID,
  name: String,
  type: String  // GANG, COMPANY, INSTITUTION
})

// Relationship types
(:Person)-[:ACCUSED_IN {role: String}]->(:Case)
(:Person)-[:ASSOCIATED_WITH {
  type: String,  // FAMILY, CRIMINAL, FINANCIAL
  strength: Float,
  since: Date
}]->(:Person)
(:Person)-[:FREQUENTS]->(:Location)
(:Person)-[:MEMBER_OF]->(:Organization)
(:Case)-[:OCCURRED_AT]->(:Location)
```

### 5.4 Analysis Algorithms

```python
class GraphIntelligenceService:
    def __init__(self, neo4j_driver):
        self.driver = neo4j_driver

    def identify_key_figures(
        self,
        jurisdiction: str,
        min_connections: int = 3
    ) -> List[KeyFigure]:
        """
        Identify key figures in criminal networks using centrality.
        """
        query = """
        CALL gds.graph.project(
            'criminal_network',
            'Person',
            'ASSOCIATED_WITH',
            {relationshipProperties: 'strength'}
        )
        YIELD graphName;

        CALL gds.pageRank.stream('criminal_network', {
            maxIterations: 50,
            dampingFactor: 0.85
        })
        YIELD nodeId, score
        WITH gds.util.asNode(nodeId) AS person, score
        WHERE score > 0.1
        MATCH (person)-[:ACCUSED_IN]->(c:Case)
        WHERE c.jurisdiction = $jurisdiction
        RETURN person.id AS id,
               person.name AS name,
               score AS influence_score,
               count(c) AS case_count
        ORDER BY score DESC
        LIMIT 20
        """

        with self.driver.session() as session:
            results = session.run(query, jurisdiction=jurisdiction)
            return [KeyFigure(**r) for r in results]

    def detect_gang_clusters(self) -> List[GangCluster]:
        """
        Detect criminal communities using Louvain algorithm.
        """
        query = """
        CALL gds.louvain.stream('criminal_network')
        YIELD nodeId, communityId
        WITH gds.util.asNode(nodeId) AS person, communityId
        WITH communityId, collect(person) AS members
        WHERE size(members) >= 3
        RETURN communityId AS cluster_id,
               [m IN members | m.name] AS member_names,
               size(members) AS size
        ORDER BY size DESC
        """

        with self.driver.session() as session:
            results = session.run(query)
            clusters = []
            for r in results:
                clusters.append(GangCluster(
                    id=r["cluster_id"],
                    members=r["member_names"],
                    size=r["size"],
                    disclaimer="AI-detected cluster. Manual investigation required."
                ))
            return clusters

    def trace_financial_network(
        self,
        person_id: str,
        max_hops: int = 4
    ) -> FinancialNetwork:
        """
        Trace financial connections from a person.
        """
        query = """
        MATCH path = (start:Person {id: $person_id})
              -[:ASSOCIATED_WITH*1..$max_hops {type: 'FINANCIAL'}]-
              (connected:Person)
        RETURN path
        """

        with self.driver.session() as session:
            results = session.run(query, person_id=person_id, max_hops=max_hops)
            return self._build_network(results)
```

### 5.5 Human Validation Points

| Analysis | Validation | Reason |
|----------|------------|--------|
| Key Figure Identification | SP-level review | Prevent targeting based solely on network position |
| Gang Detection | Intelligence officer review | Verify actual criminal association |
| Financial Tracing | Legal review before action | Financial privacy protections |

### 5.6 Audit & Explainability

```json
{
  "analysis_id": "graph-20260104-150000-ghi789",
  "timestamp": "2026-01-04T15:00:00Z",
  "analyst_id": "officer-uuid",
  "analysis_type": "KEY_FIGURE_IDENTIFICATION",
  "parameters": {
    "jurisdiction": "district-uuid",
    "algorithm": "PageRank",
    "min_connections": 3
  },
  "results": {
    "key_figures_identified": 15,
    "top_score": 0.85,
    "average_score": 0.45
  },
  "algorithm_explanation": "PageRank identifies influential nodes by analyzing "
                           "the structure of incoming connections. A high score "
                           "indicates the person is connected to many other "
                           "connected individuals.",
  "limitations": [
    "Based on recorded associations only",
    "Does not prove criminal activity",
    "Network position != criminal culpability"
  ],
  "human_review": {
    "required": true,
    "reviewed_by": null,
    "action_taken": null
  }
}
```

---

## 6. AI Governance Framework

### 6.1 Model Registry

```yaml
# Model registry entry
model:
  id: "ocr-hindi-v2.1"
  name: "Hindi Handwritten OCR"
  version: "2.1.0"
  type: "PRODUCTION"

  training:
    date: "2025-12-15"
    dataset: "hindi-handwritten-v3"
    dataset_size: 500000
    training_duration: "72 hours"
    infrastructure: "8x A100 GPU"

  performance:
    test_accuracy: 0.87
    test_dataset: "hindi-handwritten-test-v3"
    latency_p50: "1.2s"
    latency_p99: "2.8s"

  bias_metrics:
    regional_disparity: 0.05
    script_style_disparity: 0.08
    document_age_disparity: 0.12

  deployment:
    edge_compatible: true
    quantized: true
    model_size: "198MB"
    runtime: "ONNX"

  governance:
    owner: "AI Team"
    approver: "Chief Data Officer"
    last_audit: "2026-01-01"
    next_audit: "2026-04-01"
    bias_audit_passed: true
```

### 6.2 Explainability Requirements

Every AI output MUST include:

1. **Model identification**: Which model produced the output
2. **Confidence score**: How certain the model is
3. **Contributing factors**: What inputs influenced the output
4. **Limitations**: Known weaknesses of the model
5. **Disclaimer**: Standard advisory-only disclaimer

### 6.3 Human-in-the-Loop Enforcement

```python
class AIOutputValidator:
    """Enforces human review before AI outputs are used."""

    def wrap_output(self, ai_output: AIOutput, config: OutputConfig) -> ValidatedOutput:
        """Wrap AI output with validation requirements."""

        # Determine review requirements
        if config.requires_mandatory_review:
            review_status = ReviewStatus.PENDING
            can_proceed = False
        elif ai_output.confidence < config.auto_accept_threshold:
            review_status = ReviewStatus.RECOMMENDED
            can_proceed = True  # Can proceed but flagged
        else:
            review_status = ReviewStatus.OPTIONAL
            can_proceed = True

        return ValidatedOutput(
            ai_output=ai_output,
            review_status=review_status,
            can_proceed_without_review=can_proceed,
            reviewer=None,
            reviewed_at=None,
            review_outcome=None,
            audit_id=generate_audit_id()
        )

    def complete_review(
        self,
        output_id: str,
        reviewer: Officer,
        outcome: ReviewOutcome,
        corrections: Optional[Dict] = None
    ) -> ValidatedOutput:
        """Record human review of AI output."""

        output = self.get_output(output_id)
        output.reviewer = reviewer
        output.reviewed_at = datetime.now()
        output.review_outcome = outcome
        output.corrections = corrections

        # Log to audit trail
        self.audit_log.log(
            action="AI_OUTPUT_REVIEWED",
            output_id=output_id,
            reviewer=reviewer.id,
            outcome=outcome.value,
            corrections=corrections
        )

        return output
```

### 6.4 Bias Monitoring Dashboard

```yaml
# Metrics tracked continuously
bias_metrics:
  ocr:
    - accuracy_by_language
    - accuracy_by_script_style
    - accuracy_by_document_age
    - regional_accuracy_disparity

  nlp:
    - entity_extraction_by_language
    - ipc_suggestion_accuracy_by_crime_type
    - false_positive_rate_by_category

  vision:
    - detection_accuracy_by_object_type
    - false_positive_rate_for_weapons
    - performance_by_lighting_condition

  prediction:
    - prediction_accuracy_by_district
    - false_alarm_rate_by_area_type
    - correlation_with_demographics  # Should be ~0

alerts:
  - metric: "regional_accuracy_disparity"
    threshold: 0.1
    action: "notify_ai_team"

  - metric: "correlation_with_demographics"
    threshold: 0.2
    action: "suspend_model_and_investigate"
```

---

## 7. AI Deployment Architecture

### 7.1 Edge Deployment

```yaml
# Station edge AI stack
edge_ai:
  models:
    - name: ocr-hindi
      path: /models/ocr/hindi.onnx
      runtime: onnxruntime
      quantization: int8
      max_memory: 512MB

    - name: nlp-entity
      path: /models/nlp/entity.onnx
      runtime: onnxruntime
      quantization: int8
      max_memory: 256MB

  inference_server:
    type: custom-go-server
    port: 8080
    max_concurrent: 4
    timeout: 30s

  fallback:
    on_model_failure: manual_input
    on_low_confidence: flag_for_review
    on_timeout: queue_for_batch
```

### 7.2 Central Training Infrastructure

```yaml
# Central AI training cluster
training_cluster:
  hardware:
    gpu_nodes: 8
    gpu_type: "NVIDIA A100 80GB"
    cpu_nodes: 16
    storage: "100TB NVMe"

  software:
    framework: pytorch
    distributed: "DeepSpeed ZeRO-3"
    experiment_tracking: mlflow
    model_registry: mlflow

  security:
    network: air-gapped
    access: biometric + approval workflow
    data: encrypted at rest
    audit: full session recording

  compliance:
    data_retention: as_per_policy
    pii_handling: anonymization_required
    model_approval: cdo_signoff_required
```

---

## Appendix: AI Model Inventory

| Model | Purpose | Input | Output | Location | Size |
|-------|---------|-------|--------|----------|------|
| ocr-hindi-v2.1 | Hindi OCR | Image | Text + confidence | Edge | 198MB |
| ocr-english-v2.0 | English OCR | Image | Text + confidence | Edge | 180MB |
| nlp-entity-v3.1 | Entity extraction | Text | Entities + spans | Edge | 250MB |
| nlp-ipc-v3.0 | IPC classification | Text | Sections + scores | Edge | 180MB |
| nlp-similarity-v2.0 | Case similarity | Text | Embedding | State | 400MB |
| vision-evidence-v2.0 | Evidence detection | Image | Objects + boxes | District | 150MB |
| vision-weapon-v1.5 | Weapon detection | Image | Detections | District | 50MB |
| graph-community-v1.0 | Gang detection | Graph | Communities | State | N/A |
| predict-hotspot-v2.0 | Crime prediction | Features | Risk scores | State | 100MB |
