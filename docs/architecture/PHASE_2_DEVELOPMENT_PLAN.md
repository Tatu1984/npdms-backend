# PHASE 2 — PHASED DEVELOPMENT PLAN
# National Police Department Management System (NPDMS)

## Document Control
- **Version**: 1.0
- **Classification**: RESTRICTED
- **Author**: Program Management Team
- **Last Updated**: 2026-01-04

---

## Development Phase Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    NPDMS DEVELOPMENT ROADMAP                                │
└─────────────────────────────────────────────────────────────────────────────┘

PHASE A: CORE BACKBONE
├── Identity & Access Management
├── Core Data Models
├── Edge Sync Engine
└── Base UI Framework

         │
         ▼

PHASE B: POLICE STATION OPERATIONS
├── FIR Management
├── Case Management
├── Evidence Management
├── Personnel Management
├── Armoury & Vehicles
└── Station Dashboard

         │
         ▼

PHASE C: INTELLIGENCE & AI
├── OCR & Document Processing
├── NLP for FIR/Reports
├── Computer Vision (Evidence)
├── Crime Analytics
├── Graph Intelligence
└── Predictive Models

         │
         ▼

PHASE D: INTER-DISTRICT & STATE FEDERATION
├── District Aggregation
├── State Command Center
├── Cross-Jurisdiction Cases
├── Intelligence Fusion
└── Resource Optimization

         │
         ▼

PHASE E: NATIONAL COMMAND
├── Central Dashboard
├── National Alert System
├── Inter-State Coordination
├── National Analytics
└── Policy Engine
```

---

## PHASE A: CORE BACKBONE

### A.1 Objectives

Build the foundational infrastructure that all other phases depend on:
- Federated identity system that works offline
- Core database schemas and sync mechanism
- Base UI components and layout
- Security primitives (encryption, audit logging)

### A.2 Deliverables

| ID | Deliverable | Description | Priority |
|----|-------------|-------------|----------|
| A.1 | Identity & Access Management | Federated auth with offline capability | CRITICAL |
| A.2 | Core Database Schema | PostgreSQL schemas for all entities | CRITICAL |
| A.3 | Edge Sync Engine | Outbox pattern, conflict resolution | CRITICAL |
| A.4 | Base UI Framework | Next.js app with component library | CRITICAL |
| A.5 | Audit Logging System | Append-only, tamper-evident logs | CRITICAL |
| A.6 | Encryption Layer | At-rest and in-transit encryption | CRITICAL |
| A.7 | Local Storage (IndexedDB) | Offline data persistence | HIGH |
| A.8 | Message Broker Setup | Kafka/Redpanda for event streaming | HIGH |
| A.9 | Development Environment | Docker Compose, local K8s | HIGH |
| A.10 | CI/CD Pipeline | Basic build, test, deploy | HIGH |

### A.3 Technical Specifications

#### A.3.1 Identity & Access Management

```
COMPONENTS:
├── Auth Service (Go)
│   ├── JWT token issuance/validation
│   ├── Offline token verification (cached public keys)
│   ├── Role/permission resolution
│   └── Session management
│
├── Biometric Service (Go + Native)
│   ├── Fingerprint capture/match
│   ├── Local template storage
│   └── Fallback to PIN
│
└── Directory Service (PostgreSQL)
    ├── Officer records
    ├── Role assignments
    ├── Jurisdiction mappings
    └── Device registrations

AUTHENTICATION FLOW:
1. User enters credentials (username/password)
2. Device certificate validated
3. Biometric verification (fingerprint)
4. JWT issued with:
   - User ID, Role, Jurisdiction
   - Device ID
   - Token expiry (8 hours)
   - Offline validity period (72 hours)
5. Token stored locally (encrypted)
6. Public key cached for offline validation
```

#### A.3.2 Core Database Schema

```sql
-- Core Entity: Officer
CREATE TABLE officers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    badge_number VARCHAR(20) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    rank VARCHAR(50) NOT NULL,
    station_id UUID REFERENCES stations(id),
    district_id UUID REFERENCES districts(id),
    state_id UUID REFERENCES states(id),
    date_of_joining DATE NOT NULL,
    status VARCHAR(20) DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    version INTEGER DEFAULT 1,
    sync_status VARCHAR(20) DEFAULT 'SYNCED'
);

-- Core Entity: Station
CREATE TABLE stations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(20) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    district_id UUID REFERENCES districts(id),
    address TEXT,
    latitude DECIMAL(10, 8),
    longitude DECIMAL(11, 8),
    contact_numbers TEXT[],
    status VARCHAR(20) DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Sync Outbox Table
CREATE TABLE sync_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(50) NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id UUID NOT NULL,
    payload JSONB NOT NULL,
    priority VARCHAR(10) DEFAULT 'NORMAL',
    destination_tier VARCHAR(20) NOT NULL,
    status VARCHAR(20) DEFAULT 'PENDING',
    retry_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    sent_at TIMESTAMPTZ,
    error_message TEXT
);

-- Audit Log Table (Append-Only)
CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    user_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL,
    entity_type VARCHAR(50) NOT NULL,
    entity_id UUID,
    old_value JSONB,
    new_value JSONB,
    ip_address INET,
    device_id VARCHAR(100),
    session_id UUID
);

-- Create audit trigger
CREATE OR REPLACE FUNCTION audit_trigger_func()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO audit_log (user_id, action, entity_type, entity_id, old_value, new_value)
    VALUES (
        current_setting('app.current_user_id')::UUID,
        TG_OP,
        TG_TABLE_NAME,
        COALESCE(NEW.id, OLD.id),
        CASE WHEN TG_OP = 'DELETE' THEN row_to_json(OLD) ELSE NULL END,
        CASE WHEN TG_OP != 'DELETE' THEN row_to_json(NEW) ELSE NULL END
    );
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
```

#### A.3.3 Edge Sync Engine

```go
// sync/engine.go
type SyncEngine struct {
    db           *sql.DB
    broker       MessageBroker
    conflictRes  ConflictResolver
    retryPolicy  RetryPolicy
}

type SyncItem struct {
    ID            uuid.UUID
    EventType     string
    EntityType    string
    EntityID      uuid.UUID
    Payload       []byte
    Priority      Priority
    Destination   Tier
    Status        SyncStatus
    RetryCount    int
    CreatedAt     time.Time
}

// Sync priorities
const (
    PriorityFlash  Priority = "FLASH"   // Immediate
    PriorityUrgent Priority = "URGENT"  // < 5 min
    PriorityNormal Priority = "NORMAL"  // < 15 min
    PriorityBulk   Priority = "BULK"    // < 1 hour
)

// Main sync loop
func (e *SyncEngine) Run(ctx context.Context) error {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := e.processOutbox(ctx); err != nil {
                log.Error("sync error", "error", err)
            }
        }
    }
}

func (e *SyncEngine) processOutbox(ctx context.Context) error {
    items, err := e.fetchPendingItems(ctx, 100)
    if err != nil {
        return err
    }

    for _, item := range items {
        if err := e.syncItem(ctx, item); err != nil {
            e.handleSyncError(ctx, item, err)
        } else {
            e.markSynced(ctx, item.ID)
        }
    }
    return nil
}

// Conflict resolution
func (e *SyncEngine) resolveConflict(local, remote Entity) (Entity, error) {
    // 1. Check legal precedence (court-submitted wins)
    if remote.IsCourtSubmitted() {
        return remote, nil
    }

    // 2. Check rank precedence
    if remote.ModifierRank > local.ModifierRank {
        return remote, nil
    }

    // 3. Temporal precedence
    if remote.UpdatedAt.After(local.UpdatedAt) {
        return remote, nil
    }

    // 4. Attempt field-level merge
    merged, conflicts := mergeFields(local, remote)
    if len(conflicts) == 0 {
        return merged, nil
    }

    // 5. Flag for manual resolution
    return local, ErrManualResolutionRequired
}
```

### A.4 Dependencies

| Dependency | Type | Risk if Delayed |
|------------|------|-----------------|
| None (Foundation phase) | - | - |

### A.5 Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Sync engine complexity underestimated | Medium | High | Start with simple conflict resolution, iterate |
| Offline-first adds development overhead | High | Medium | Use proven patterns (Outbox, CRDT principles) |
| Security review delays | Medium | High | Engage security team early, parallel review |
| Database schema changes during phase | Medium | Medium | Schema versioning, migrations from day 1 |

### A.6 Validation & Acceptance Criteria

| Criterion | Test Method | Pass Condition |
|-----------|-------------|----------------|
| Offline login works | Manual test | Login succeeds with cached credentials up to 72 hours |
| Sync queue persists | Kill process, restart | Queue items recovered, sync resumes |
| Conflict detection | Create conflicting edits | Conflict flagged, both versions preserved |
| Audit log complete | Review logs | All CRUD operations logged with user context |
| Encryption verified | Security scan | All data at rest encrypted, TLS 1.3 in transit |
| UI framework loads | Browser test | Dashboard renders in < 2 seconds |

---

## PHASE B: POLICE STATION OPERATIONS

### B.1 Objectives

Deliver complete police station functionality:
- Full FIR lifecycle management
- Case investigation workflow
- Evidence chain of custody
- Personnel and duty management
- Armoury and vehicle tracking

### B.2 Deliverables

| ID | Deliverable | Description | Priority |
|----|-------------|-------------|----------|
| B.1 | FIR Registration | Multi-modal input (type, voice, scan) | CRITICAL |
| B.2 | FIR Search & List | Full-text search, filters | CRITICAL |
| B.3 | Case Diary | Daily entries, attachments | CRITICAL |
| B.4 | Case Timeline | Chronological case view | HIGH |
| B.5 | Evidence Registry | Register, track, chain of custody | CRITICAL |
| B.6 | Evidence Upload | Photos, videos, documents | HIGH |
| B.7 | Personnel Management | Roster, attendance, leave | HIGH |
| B.8 | Duty Assignment | Shift planning, beat allocation | HIGH |
| B.9 | Armoury Management | Weapon registry, issuance | HIGH |
| B.10 | Vehicle Management | Fleet tracking, allocation | HIGH |
| B.11 | Station Dashboard | SHO command view | HIGH |
| B.12 | Constable Dashboard | Personal task view | HIGH |
| B.13 | Reports & Exports | Daily/monthly station reports | MEDIUM |
| B.14 | Citizen Interface | Complaint status (redacted) | MEDIUM |

### B.3 Technical Specifications

#### B.3.1 FIR Data Model

```sql
CREATE TABLE firs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fir_number VARCHAR(50) UNIQUE NOT NULL,
    station_id UUID REFERENCES stations(id) NOT NULL,
    district_id UUID REFERENCES districts(id) NOT NULL,

    -- Complainant
    complainant_name VARCHAR(200) NOT NULL,
    complainant_father_name VARCHAR(200),
    complainant_address TEXT NOT NULL,
    complainant_phone VARCHAR(20),
    complainant_id_type VARCHAR(50),
    complainant_id_number VARCHAR(50),
    complainant_age INTEGER,
    complainant_gender VARCHAR(20),

    -- Incident
    incident_date DATE NOT NULL,
    incident_time TIME,
    incident_time_approximate BOOLEAN DEFAULT FALSE,
    incident_location TEXT NOT NULL,
    incident_location_lat DECIMAL(10, 8),
    incident_location_lng DECIMAL(11, 8),
    incident_beat VARCHAR(100),

    -- Offence
    offence_category VARCHAR(100) NOT NULL,
    offence_type VARCHAR(100) NOT NULL,
    ipc_sections TEXT[] NOT NULL,
    special_acts TEXT[],

    -- Description
    incident_description TEXT NOT NULL,

    -- Status
    status VARCHAR(50) DEFAULT 'REGISTERED',
    priority VARCHAR(20) DEFAULT 'NORMAL',

    -- Accused (if known)
    accused_known BOOLEAN DEFAULT FALSE,

    -- Property (summarized)
    total_property_value DECIMAL(15, 2) DEFAULT 0,

    -- Recording officer
    recorded_by UUID REFERENCES officers(id) NOT NULL,
    recorded_at TIMESTAMPTZ DEFAULT NOW(),

    -- Investigation
    investigating_officer UUID REFERENCES officers(id),
    investigation_started_at TIMESTAMPTZ,

    -- Court
    chargesheet_filed BOOLEAN DEFAULT FALSE,
    chargesheet_date DATE,
    court_name VARCHAR(200),

    -- Closure
    closed_at TIMESTAMPTZ,
    closure_type VARCHAR(50),
    closure_remarks TEXT,

    -- Sync
    version INTEGER DEFAULT 1,
    sync_status VARCHAR(20) DEFAULT 'PENDING',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- FIR Accused
CREATE TABLE fir_accused (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fir_id UUID REFERENCES firs(id) ON DELETE CASCADE,
    name VARCHAR(200),
    alias VARCHAR(200),
    description TEXT,
    address TEXT,
    identification_marks TEXT,
    photo_url TEXT,
    status VARCHAR(50) DEFAULT 'WANTED',
    arrested_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- FIR Property
CREATE TABLE fir_property (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fir_id UUID REFERENCES firs(id) ON DELETE CASCADE,
    item_name VARCHAR(200) NOT NULL,
    description TEXT,
    estimated_value DECIMAL(15, 2),
    recovered BOOLEAN DEFAULT FALSE,
    recovered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Case Diary
CREATE TABLE case_diary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fir_id UUID REFERENCES firs(id) ON DELETE CASCADE,
    entry_date DATE NOT NULL,
    entry_time TIME DEFAULT CURRENT_TIME,
    officer_id UUID REFERENCES officers(id) NOT NULL,
    content TEXT NOT NULL,
    next_action TEXT,
    attachments TEXT[],
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Evidence
CREATE TABLE evidence (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_number VARCHAR(50) UNIQUE NOT NULL,
    fir_id UUID REFERENCES firs(id),
    case_id UUID,

    -- Classification
    evidence_type VARCHAR(50) NOT NULL, -- PHYSICAL, DIGITAL
    category VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,

    -- Collection
    collected_by UUID REFERENCES officers(id) NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL,
    collection_location TEXT,
    collection_lat DECIMAL(10, 8),
    collection_lng DECIMAL(11, 8),

    -- Storage
    storage_location VARCHAR(200),
    storage_locker VARCHAR(50),
    seal_number VARCHAR(50),

    -- Digital evidence
    file_path TEXT,
    file_hash VARCHAR(64), -- SHA-256
    file_size BIGINT,
    mime_type VARCHAR(100),

    -- Status
    status VARCHAR(50) DEFAULT 'COLLECTED',
    current_custodian UUID REFERENCES officers(id),

    -- Forensic
    forensic_request_id UUID,
    forensic_status VARCHAR(50),
    forensic_result TEXT,

    -- Integrity
    integrity_verified BOOLEAN DEFAULT TRUE,
    last_integrity_check TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Evidence Chain of Custody
CREATE TABLE evidence_custody_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_id UUID REFERENCES evidence(id) NOT NULL,
    action VARCHAR(50) NOT NULL, -- COLLECTED, SEALED, TRANSFERRED, OPENED, etc.
    from_custodian UUID REFERENCES officers(id),
    to_custodian UUID REFERENCES officers(id),
    location VARCHAR(200),
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    witness_id UUID REFERENCES officers(id),
    biometric_verified BOOLEAN DEFAULT FALSE,
    remarks TEXT,
    photo_proof TEXT
);
```

#### B.3.2 FIR Creation API

```go
// api/fir/create.go
type CreateFIRRequest struct {
    // Complainant
    ComplainantName       string `json:"complainant_name" validate:"required"`
    ComplainantFatherName string `json:"complainant_father_name"`
    ComplainantAddress    string `json:"complainant_address" validate:"required"`
    ComplainantPhone      string `json:"complainant_phone"`
    ComplainantIDType     string `json:"complainant_id_type"`
    ComplainantIDNumber   string `json:"complainant_id_number"`
    ComplainantAge        int    `json:"complainant_age"`
    ComplainantGender     string `json:"complainant_gender"`

    // Incident
    IncidentDate            string   `json:"incident_date" validate:"required"`
    IncidentTime            string   `json:"incident_time"`
    IncidentTimeApproximate bool     `json:"incident_time_approximate"`
    IncidentLocation        string   `json:"incident_location" validate:"required"`
    IncidentBeat            string   `json:"incident_beat"`

    // Offence
    OffenceCategory string   `json:"offence_category" validate:"required"`
    OffenceType     string   `json:"offence_type" validate:"required"`
    IPCSections     []string `json:"ipc_sections" validate:"required,min=1"`
    SpecialActs     []string `json:"special_acts"`

    // Description
    IncidentDescription string `json:"incident_description" validate:"required,min=100"`

    // Property
    PropertyItems []PropertyItem `json:"property_items"`

    // Accused
    AccusedKnown bool           `json:"accused_known"`
    Accused      []AccusedInput `json:"accused"`
}

func (h *FIRHandler) CreateFIR(c *gin.Context) {
    var req CreateFIRRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Get current user from context
    user := c.MustGet("user").(*auth.User)

    // Generate FIR number
    firNumber := h.generateFIRNumber(user.StationID)

    // Create FIR in transaction
    tx, err := h.db.BeginTx(c, nil)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to start transaction"})
        return
    }
    defer tx.Rollback()

    fir := &models.FIR{
        FIRNumber:       firNumber,
        StationID:       user.StationID,
        DistrictID:      user.DistrictID,
        RecordedBy:      user.ID,
        RecordedAt:      time.Now(),
        Status:          "REGISTERED",
        SyncStatus:      "PENDING",
        // ... map all fields
    }

    if err := h.repo.Create(tx, fir); err != nil {
        c.JSON(500, gin.H{"error": "Failed to create FIR"})
        return
    }

    // Create property items
    for _, item := range req.PropertyItems {
        property := &models.FIRProperty{
            FIRID:          fir.ID,
            ItemName:       item.Name,
            Description:    item.Description,
            EstimatedValue: item.Value,
        }
        if err := h.propertyRepo.Create(tx, property); err != nil {
            c.JSON(500, gin.H{"error": "Failed to create property record"})
            return
        }
    }

    // Create accused records
    if req.AccusedKnown {
        for _, acc := range req.Accused {
            accused := &models.FIRAccused{
                FIRID:       fir.ID,
                Name:        acc.Name,
                Description: acc.Description,
                Status:      "WANTED",
            }
            if err := h.accusedRepo.Create(tx, accused); err != nil {
                c.JSON(500, gin.H{"error": "Failed to create accused record"})
                return
            }
        }
    }

    // Queue for sync
    syncItem := &sync.Item{
        EventType:   "FIR_CREATED",
        EntityType:  "FIR",
        EntityID:    fir.ID,
        Payload:     fir,
        Priority:    sync.PriorityNormal,
        Destination: sync.TierDistrict,
    }
    if err := h.syncEngine.Queue(tx, syncItem); err != nil {
        c.JSON(500, gin.H{"error": "Failed to queue sync"})
        return
    }

    // Create audit log
    h.audit.Log(c, "FIR_CREATED", "FIR", fir.ID, nil, fir)

    if err := tx.Commit(); err != nil {
        c.JSON(500, gin.H{"error": "Failed to commit transaction"})
        return
    }

    c.JSON(201, gin.H{
        "id":         fir.ID,
        "fir_number": fir.FIRNumber,
        "status":     fir.Status,
    })
}
```

### B.4 Dependencies

| Dependency | From Phase | Required For |
|------------|------------|--------------|
| Identity & Access Management | A | All operations require auth |
| Core Database Schema | A | All data storage |
| Edge Sync Engine | A | Data sync to district |
| Base UI Framework | A | All user interfaces |
| Audit Logging | A | All operations |

### B.5 Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| FIR workflow complexity | Medium | Medium | Engage police SMEs early |
| Evidence handling requirements | Medium | High | Legal team review of chain of custody |
| Offline evidence upload (large files) | High | Medium | Chunked upload, resume capability |
| Voice/scan input quality issues | Medium | Medium | Quality indicators, manual fallback |

### B.6 Validation & Acceptance Criteria

| Criterion | Test Method | Pass Condition |
|-----------|-------------|----------------|
| FIR created offline | Disconnect, create FIR | FIR saved locally, syncs when online |
| FIR search < 1 second | Performance test | 95th percentile < 1s for 10k records |
| Evidence chain complete | Create evidence, transfer | All custody changes logged |
| Biometric on weapon issue | Manual test | Cannot issue without fingerprint |
| SHO sees all station data | Role test | Full visibility, constable sees own only |

---

## PHASE C: INTELLIGENCE & AI

### C.1 Objectives

Deploy AI capabilities that assist officers:
- Document digitization (handwritten registers)
- Automated FIR structuring from unstructured input
- Evidence analysis (image/video)
- Crime pattern detection
- Criminal network analysis

### C.2 Deliverables

| ID | Deliverable | Description | Priority |
|----|-------------|-------------|----------|
| C.1 | OCR Service | Handwritten text extraction | CRITICAL |
| C.2 | FIR NLP Service | Entity extraction, structuring | CRITICAL |
| C.3 | IPC Section Suggester | Suggest relevant sections | HIGH |
| C.4 | Similar Case Detector | Find related cases | HIGH |
| C.5 | Evidence Image Analysis | Object/weapon detection | HIGH |
| C.6 | Video Summarization | Key frame extraction from CCTV | MEDIUM |
| C.7 | Crime Heatmap | Temporal-spatial visualization | HIGH |
| C.8 | Trend Analysis | Crime trend detection | HIGH |
| C.9 | Criminal Network Graph | Relationship visualization | HIGH |
| C.10 | Patrol Optimization | Route suggestions | MEDIUM |
| C.11 | AI Governance Dashboard | Model monitoring, bias tracking | HIGH |
| C.12 | Human Review Interface | Accept/reject AI suggestions | CRITICAL |

### C.3 Technical Specifications

#### C.3.1 AI Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         AI DEPLOYMENT MODEL                                  │
└─────────────────────────────────────────────────────────────────────────────┘

EDGE (Station/District) - INFERENCE ONLY
├── OCR Model (ONNX)
│   └── Handwritten text extraction
│   └── CPU inference via OpenVINO
│   └── Latency: < 3 seconds per page
│
├── FIR NLP Model (ONNX)
│   └── Entity extraction
│   └── IPC section suggestion
│   └── Quantized for edge
│
└── Basic Image Classifier (ONNX)
    └── Evidence categorization
    └── Weapon detection

STATE - INFERENCE + BATCH PROCESSING
├── Full NLP Pipeline
│   └── Cross-case similarity
│   └── Trend detection
│
├── Video Analysis
│   └── Key frame extraction
│   └── Face detection (for matching)
│
└── Graph Analytics
    └── Criminal network analysis
    └── Neo4j + GDS

CENTRAL - TRAINING + HEAVY INFERENCE
├── Model Training Infrastructure
│   └── PyTorch + distributed training
│   └── GPU cluster (NVIDIA A100)
│
├── Model Registry
│   └── Version control
│   └── A/B testing
│   └── Bias monitoring
│
└── National Analytics
    └── Cross-state patterns
    └── Predictive models
```

#### C.3.2 OCR Service

```python
# ai/ocr/service.py
import torch
from transformers import TrOCRProcessor, VisionEncoderDecoderModel
from PIL import Image
import numpy as np

class OCRService:
    def __init__(self, model_path: str, device: str = "cpu"):
        self.processor = TrOCRProcessor.from_pretrained(model_path)
        self.model = VisionEncoderDecoderModel.from_pretrained(model_path)
        self.model.to(device)
        self.model.eval()
        self.device = device

        # Language-specific models
        self.lang_models = {
            "hi": self._load_hindi_model(),
            "ta": self._load_tamil_model(),
            "te": self._load_telugu_model(),
            # ... other Indian languages
        }

    def extract_text(
        self,
        image: Image.Image,
        language: str = "en"
    ) -> OCRResult:
        """
        Extract text from handwritten image.

        Args:
            image: PIL Image of handwritten document
            language: ISO language code

        Returns:
            OCRResult with text, confidence, and bounding boxes
        """
        # Preprocess
        pixel_values = self.processor(
            image,
            return_tensors="pt"
        ).pixel_values.to(self.device)

        # Select model based on language
        model = self.lang_models.get(language, self.model)

        # Inference
        with torch.no_grad():
            generated_ids = model.generate(
                pixel_values,
                max_length=512,
                num_beams=4,
                return_dict_in_generate=True,
                output_scores=True
            )

        # Decode
        text = self.processor.batch_decode(
            generated_ids.sequences,
            skip_special_tokens=True
        )[0]

        # Calculate confidence
        confidence = self._calculate_confidence(generated_ids.scores)

        # Post-process with legal dictionary
        corrected_text = self._apply_legal_corrections(text, language)

        return OCRResult(
            text=corrected_text,
            original_text=text,
            confidence=confidence,
            language=language,
            corrections_applied=text != corrected_text
        )

    def _apply_legal_corrections(self, text: str, language: str) -> str:
        """Apply corrections using police/legal vocabulary."""
        corrections = self._load_legal_dictionary(language)

        for wrong, correct in corrections.items():
            text = text.replace(wrong, correct)

        # IPC section format correction
        text = re.sub(
            r'section\s*(\d+)',
            r'Section \1',
            text,
            flags=re.IGNORECASE
        )

        return text

    def _calculate_confidence(self, scores) -> float:
        """Calculate overall confidence score."""
        if not scores:
            return 0.0

        probs = [torch.softmax(s, dim=-1).max().item() for s in scores]
        return sum(probs) / len(probs)


# API endpoint
@router.post("/ocr/extract")
async def extract_text(
    file: UploadFile,
    language: str = "en",
    current_user: User = Depends(get_current_user)
):
    # Validate file
    if file.content_type not in ["image/jpeg", "image/png", "image/tiff"]:
        raise HTTPException(400, "Invalid file type")

    # Read image
    contents = await file.read()
    image = Image.open(io.BytesIO(contents))

    # Extract text
    result = ocr_service.extract_text(image, language)

    # Log for audit
    audit_log.info(
        "OCR_EXTRACTION",
        user_id=current_user.id,
        file_name=file.filename,
        confidence=result.confidence
    )

    # Flag low confidence for human review
    requires_review = result.confidence < 0.85

    return {
        "text": result.text,
        "confidence": result.confidence,
        "requires_review": requires_review,
        "language": result.language,
        "disclaimer": "AI-extracted text. Human verification required."
    }
```

#### C.3.3 FIR NLP Service

```python
# ai/nlp/fir_service.py
from transformers import AutoTokenizer, AutoModelForTokenClassification
import spacy
from typing import List, Dict

class FIRNLPService:
    def __init__(self, model_path: str):
        self.tokenizer = AutoTokenizer.from_pretrained(model_path)
        self.model = AutoModelForTokenClassification.from_pretrained(model_path)
        self.nlp = spacy.load("en_core_web_lg")

        # Load IPC section classifier
        self.ipc_classifier = self._load_ipc_classifier()

    def structure_fir(self, raw_text: str) -> FIRStructuredOutput:
        """
        Extract structured information from raw FIR text.

        Returns structured data with confidence scores.
        """
        # Named Entity Recognition
        entities = self._extract_entities(raw_text)

        # IPC Section suggestion
        suggested_sections = self._suggest_ipc_sections(raw_text)

        # Location extraction and geocoding
        locations = self._extract_locations(raw_text)

        # Date/time extraction
        temporal = self._extract_temporal(raw_text)

        return FIRStructuredOutput(
            entities=entities,
            suggested_ipc_sections=suggested_sections,
            locations=locations,
            temporal_info=temporal,
            confidence_overall=self._calculate_overall_confidence(
                entities, suggested_sections, locations, temporal
            ),
            requires_human_review=True  # Always require review
        )

    def _extract_entities(self, text: str) -> List[Entity]:
        """Extract named entities with custom police domain."""
        doc = self.nlp(text)

        entities = []
        for ent in doc.ents:
            entity = Entity(
                text=ent.text,
                label=ent.label_,
                start=ent.start_char,
                end=ent.end_char,
                confidence=0.0  # SpaCy doesn't provide this
            )
            entities.append(entity)

        # Custom extraction for police-specific entities
        # Vehicle numbers
        vehicles = re.findall(
            r'[A-Z]{2}[-\s]?\d{1,2}[-\s]?[A-Z]{1,2}[-\s]?\d{4}',
            text
        )
        for v in vehicles:
            entities.append(Entity(
                text=v,
                label="VEHICLE",
                confidence=0.95
            ))

        # Phone numbers
        phones = re.findall(r'\b\d{10}\b', text)
        for p in phones:
            entities.append(Entity(
                text=p,
                label="PHONE",
                confidence=0.95
            ))

        # Aadhaar numbers (masked for display)
        aadhaar = re.findall(r'\b\d{4}[-\s]?\d{4}[-\s]?\d{4}\b', text)
        for a in aadhaar:
            entities.append(Entity(
                text=f"XXXX-XXXX-{a[-4:]}",
                label="AADHAAR",
                confidence=0.90,
                sensitive=True
            ))

        return entities

    def _suggest_ipc_sections(self, text: str) -> List[IPCSuggestion]:
        """Suggest applicable IPC sections based on FIR content."""
        # Encode text
        inputs = self.tokenizer(
            text,
            return_tensors="pt",
            max_length=512,
            truncation=True
        )

        # Get predictions
        with torch.no_grad():
            outputs = self.ipc_classifier(**inputs)
            probs = torch.sigmoid(outputs.logits)[0]

        # Get top suggestions
        suggestions = []
        for idx, prob in enumerate(probs):
            if prob > 0.3:  # Threshold
                section = self.ipc_sections[idx]
                suggestions.append(IPCSuggestion(
                    section=section.code,
                    description=section.description,
                    confidence=prob.item(),
                    explanation=self._generate_explanation(text, section)
                ))

        # Sort by confidence
        suggestions.sort(key=lambda x: x.confidence, reverse=True)

        return suggestions[:10]  # Top 10 suggestions

    def _generate_explanation(self, text: str, section: IPCSection) -> str:
        """Generate human-readable explanation for suggestion."""
        # Find keywords that triggered this suggestion
        keywords = section.keywords
        found = [k for k in keywords if k.lower() in text.lower()]

        if found:
            return f"Suggested because text contains: {', '.join(found[:3])}"
        return f"Based on overall context matching {section.description}"


# Similar case detection
class SimilarCaseDetector:
    def __init__(self, embedding_model: str, vector_db: VectorDB):
        self.embedder = SentenceTransformer(embedding_model)
        self.vector_db = vector_db

    def find_similar(
        self,
        fir_text: str,
        jurisdiction: str,
        top_k: int = 5
    ) -> List[SimilarCase]:
        """Find similar cases within jurisdiction."""
        # Generate embedding
        embedding = self.embedder.encode(fir_text)

        # Search in vector database
        results = self.vector_db.search(
            embedding,
            filter={"jurisdiction": jurisdiction},
            top_k=top_k
        )

        similar = []
        for result in results:
            similar.append(SimilarCase(
                fir_number=result.metadata["fir_number"],
                similarity_score=result.score,
                summary=result.metadata["summary"],
                ipc_sections=result.metadata["ipc_sections"],
                status=result.metadata["status"]
            ))

        return similar
```

#### C.3.4 Criminal Network Graph

```python
# ai/graph/network_analysis.py
from neo4j import GraphDatabase
import networkx as nx

class CriminalNetworkAnalyzer:
    def __init__(self, neo4j_uri: str, neo4j_auth: tuple):
        self.driver = GraphDatabase.driver(neo4j_uri, auth=neo4j_auth)

    def build_network(self, case_ids: List[str]) -> NetworkGraph:
        """Build criminal network from related cases."""
        with self.driver.session() as session:
            # Query relationships
            result = session.run("""
                MATCH (c:Case)-[:INVOLVES]->(p:Person)
                WHERE c.id IN $case_ids
                WITH p
                MATCH (p)-[r:ASSOCIATED_WITH]-(other:Person)
                RETURN p, r, other,
                       [(p)-[:ACCUSED_IN]->(case) | case] as cases
            """, case_ids=case_ids)

            # Build graph
            G = nx.Graph()
            for record in result:
                person = record["p"]
                other = record["other"]
                rel = record["r"]

                G.add_node(person["id"], **dict(person))
                G.add_node(other["id"], **dict(other))
                G.add_edge(
                    person["id"],
                    other["id"],
                    weight=rel["strength"],
                    type=rel["type"]
                )

            return NetworkGraph(
                nodes=list(G.nodes(data=True)),
                edges=list(G.edges(data=True)),
                stats=self._calculate_stats(G)
            )

    def identify_key_figures(
        self,
        jurisdiction: str
    ) -> List[KeyFigure]:
        """Identify key figures in criminal networks."""
        with self.driver.session() as session:
            result = session.run("""
                MATCH (p:Person)-[:ACCUSED_IN]->(c:Case)
                WHERE c.jurisdiction = $jurisdiction
                WITH p, count(c) as case_count
                WHERE case_count > 2
                MATCH (p)-[r:ASSOCIATED_WITH]-(other:Person)
                WITH p, case_count, count(r) as connections
                RETURN p.id as id,
                       p.name as name,
                       case_count,
                       connections,
                       case_count * connections as influence_score
                ORDER BY influence_score DESC
                LIMIT 20
            """, jurisdiction=jurisdiction)

            return [
                KeyFigure(
                    id=r["id"],
                    name=r["name"],
                    case_count=r["case_count"],
                    connections=r["connections"],
                    influence_score=r["influence_score"]
                )
                for r in result
            ]

    def detect_gang_clusters(self) -> List[GangCluster]:
        """Detect gang clusters using community detection."""
        with self.driver.session() as session:
            # Run Louvain community detection
            session.run("""
                CALL gds.graph.project(
                    'criminal_network',
                    'Person',
                    'ASSOCIATED_WITH'
                )
            """)

            result = session.run("""
                CALL gds.louvain.stream('criminal_network')
                YIELD nodeId, communityId
                WITH gds.util.asNode(nodeId) as person, communityId
                RETURN communityId,
                       collect(person.name) as members,
                       count(*) as size
                ORDER BY size DESC
            """)

            clusters = []
            for r in result:
                if r["size"] >= 3:  # Minimum 3 members
                    clusters.append(GangCluster(
                        id=r["communityId"],
                        members=r["members"],
                        size=r["size"]
                    ))

            return clusters
```

### C.4 Dependencies

| Dependency | From Phase | Required For |
|------------|------------|--------------|
| FIR Data Model | B | NLP training data |
| Evidence Storage | B | Image/video analysis |
| Case Data | B | Similar case detection |
| Sync Engine | A | Model updates distribution |

### C.5 Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Model bias in crime prediction | High | Critical | Mandatory bias audits, geographic fairness checks |
| AI over-reliance by officers | Medium | High | Clear disclaimers, human-in-loop enforced |
| Edge device compute limits | Medium | Medium | Model quantization, batching |
| Training data quality | High | High | Data validation pipeline, expert review |
| Adversarial attacks on AI | Medium | High | Input validation, anomaly detection |

### C.6 Validation & Acceptance Criteria

| Criterion | Test Method | Pass Condition |
|-----------|-------------|----------------|
| OCR accuracy (Hindi) | Test set evaluation | > 85% character accuracy |
| IPC suggestion relevance | Expert review | > 70% suggestions relevant |
| Similar case detection | Precision/recall test | Precision > 80%, Recall > 60% |
| AI disclaimer shown | UI test | Disclaimer visible on all AI outputs |
| Human override works | Manual test | All AI suggestions can be rejected |
| Bias audit passed | Fairness metrics | No geographic/demographic bias > 10% |

---

## PHASE D: INTER-DISTRICT & STATE FEDERATION

### D.1 Objectives

Enable district and state-level operations:
- Aggregate station data at district level
- State-level command and control
- Cross-jurisdiction case management
- Intelligence fusion across districts

### D.2 Deliverables

| ID | Deliverable | Description | Priority |
|----|-------------|-------------|----------|
| D.1 | District Aggregation Service | Collect from all stations | CRITICAL |
| D.2 | District Dashboard | SP/DCP command view | CRITICAL |
| D.3 | Cross-Station Case Linking | Link related cases | HIGH |
| D.4 | District Analytics | Crime patterns, performance | HIGH |
| D.5 | State Sync Hub | Aggregate from districts | CRITICAL |
| D.6 | State Dashboard | DGP command view | CRITICAL |
| D.7 | Inter-District Coordination | Case transfers, requests | HIGH |
| D.8 | State Intelligence Fusion | Cross-district patterns | HIGH |
| D.9 | Resource Optimization | Patrol, personnel allocation | MEDIUM |
| D.10 | State Alert System | State-wide broadcasts | HIGH |
| D.11 | Performance Monitoring | Station/district rankings | MEDIUM |
| D.12 | Court Integration Gateway | State-level court system | MEDIUM |

### D.3 Technical Specifications

#### D.3.1 District Aggregation Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      DISTRICT AGGREGATION SERVICE                            │
└─────────────────────────────────────────────────────────────────────────────┘

Station 1 ──┐
Station 2 ──┼──► Redpanda ──► Aggregation ──► District ──► ScyllaDB
Station 3 ──┤    (Events)     Service         PostgreSQL   (Distributed)
   ...     ──┘

AGGREGATION RULES:
─────────────────
1. FIR Events: Store full record, index in OpenSearch
2. Evidence Events: Store metadata only, files in MinIO
3. Personnel Events: Store full record
4. Alerts: Broadcast immediately
5. Statistics: Aggregate hourly

DATA RETENTION:
──────────────
• Hot data (< 30 days): PostgreSQL
• Warm data (30-365 days): ScyllaDB
• Cold data (> 1 year): Object storage (MinIO)
• Audit logs: 20 years (append-only, compressed)
```

#### D.3.2 State Sync Hub

```go
// state/sync_hub.go
type StateSyncHub struct {
    districts    map[string]*DistrictConnection
    kafka        *kafka.Client
    stateDB      *sql.DB
    aggregator   *Aggregator
}

type DistrictConnection struct {
    ID           string
    Status       ConnectionStatus
    LastSync     time.Time
    PendingItems int
    Connection   *grpc.ClientConn
}

func (h *StateSyncHub) Run(ctx context.Context) error {
    // Subscribe to all district topics
    for districtID := range h.districts {
        topic := fmt.Sprintf("district.%s.events", districtID)
        go h.consumeDistrict(ctx, districtID, topic)
    }

    // Aggregation loop
    ticker := time.NewTicker(1 * time.Minute)
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            h.aggregateStatistics()
        }
    }
}

func (h *StateSyncHub) consumeDistrict(
    ctx context.Context,
    districtID string,
    topic string,
) {
    consumer := h.kafka.NewConsumer(topic)

    for {
        msg, err := consumer.ReadMessage(ctx)
        if err != nil {
            log.Error("kafka read error", "district", districtID, "error", err)
            continue
        }

        event := &DistrictEvent{}
        if err := proto.Unmarshal(msg.Value, event); err != nil {
            log.Error("unmarshal error", "error", err)
            continue
        }

        // Process based on event type
        switch event.Type {
        case "FIR_CREATED", "FIR_UPDATED":
            h.handleFIREvent(event)
        case "CASE_LINKED":
            h.handleCaseLinkEvent(event)
        case "ALERT":
            h.handleAlert(event)
        case "STATISTICS":
            h.handleStatistics(event)
        }

        // Update district status
        h.districts[districtID].LastSync = time.Now()
    }
}

func (h *StateSyncHub) broadcastStateAlert(alert *Alert) error {
    // Sign the alert
    signature, err := h.signAlert(alert)
    if err != nil {
        return err
    }
    alert.Signature = signature

    // Broadcast to all districts
    for districtID, conn := range h.districts {
        go func(id string, c *DistrictConnection) {
            client := pb.NewAlertServiceClient(c.Connection)
            _, err := client.BroadcastAlert(context.Background(), alert)
            if err != nil {
                log.Error("alert broadcast failed", "district", id, "error", err)
            }
        }(districtID, conn)
    }

    return nil
}
```

### D.4 Dependencies

| Dependency | From Phase | Required For |
|------------|------------|--------------|
| Station Operations | B | Data to aggregate |
| Sync Engine | A | Inter-tier communication |
| AI Services | C | Intelligence analysis |

### D.5 Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| State network bandwidth | Medium | High | Delta sync, compression |
| Data consistency across tiers | Medium | High | Eventual consistency model, clear SLAs |
| Jurisdiction conflicts | Medium | Medium | Clear ownership rules |
| Performance at scale | Medium | High | Load testing, horizontal scaling |

### D.6 Validation & Acceptance Criteria

| Criterion | Test Method | Pass Condition |
|-----------|-------------|----------------|
| All stations visible at district | Integration test | 100% station data visible |
| State sees all districts | Integration test | All district dashboards aggregated |
| Cross-district case link works | End-to-end test | Cases linkable, history preserved |
| Alert propagates in < 5 min | Timing test | URGENT alerts reach all stations |
| District offline resilience | Disconnect test | District operates, resumes sync |

---

## PHASE E: NATIONAL COMMAND

### E.1 Objectives

Complete national integration:
- Central command dashboard
- National alert broadcast system
- Inter-state coordination
- National analytics and reporting
- Policy distribution engine

### E.2 Deliverables

| ID | Deliverable | Description | Priority |
|----|-------------|-------------|----------|
| E.1 | Central Dashboard | Home Ministry command view | CRITICAL |
| E.2 | National Alert System | Nationwide broadcasts | CRITICAL |
| E.3 | Inter-State Coordination | Cross-state case management | HIGH |
| E.4 | National Crime Map | Visualization | HIGH |
| E.5 | National Analytics | Cross-state trends | HIGH |
| E.6 | Wanted List Management | National wanted database | HIGH |
| E.7 | Policy Distribution | National directives | HIGH |
| E.8 | Compliance Dashboard | Audit and oversight | HIGH |
| E.9 | Inter-Agency Gateway | CBI, NIA, NCB integration | MEDIUM |
| E.10 | Disaster Recovery | National DR orchestration | CRITICAL |

### E.3 Technical Specifications

#### E.3.1 National Alert System

```go
// central/alert_system.go
type NationalAlertSystem struct {
    states      map[string]*StateConnection
    kafka       *kafka.Client
    alertDB     *sql.DB
    signer      crypto.Signer
    validator   *AlertValidator
}

type NationalAlert struct {
    ID           string
    Type         AlertType     // FLASH, URGENT, NOTICE
    Scope        AlertScope    // NATIONAL, REGIONAL, STATE_SPECIFIC
    Title        string
    Description  string
    ActionItems  []string
    TargetStates []string      // Empty = all states
    IssuedBy     string        // Authority ID
    IssuedAt     time.Time
    ExpiresAt    time.Time
    Signature    []byte
    Attachments  []Attachment
}

const (
    AlertTypeFlash   AlertType = "FLASH"   // Immediate action required
    AlertTypeUrgent  AlertType = "URGENT"  // < 4 hours response
    AlertTypeNotice  AlertType = "NOTICE"  // Informational
)

func (s *NationalAlertSystem) BroadcastAlert(
    ctx context.Context,
    alert *NationalAlert,
) (*BroadcastResult, error) {
    // Validate alert
    if err := s.validator.Validate(alert); err != nil {
        return nil, fmt.Errorf("invalid alert: %w", err)
    }

    // Sign with HSM
    signature, err := s.signer.Sign(alert.Hash())
    if err != nil {
        return nil, fmt.Errorf("signing failed: %w", err)
    }
    alert.Signature = signature

    // Store in database
    if err := s.alertDB.Create(alert); err != nil {
        return nil, fmt.Errorf("storage failed: %w", err)
    }

    // Determine target states
    targetStates := alert.TargetStates
    if len(targetStates) == 0 {
        targetStates = s.getAllStateIDs()
    }

    // Broadcast to states
    results := make(chan StateAckResult, len(targetStates))
    for _, stateID := range targetStates {
        go func(id string) {
            result := s.sendToState(ctx, id, alert)
            results <- result
        }(stateID)
    }

    // Collect acknowledgments
    acks := make([]StateAckResult, 0, len(targetStates))
    timeout := time.After(30 * time.Second)

    for i := 0; i < len(targetStates); i++ {
        select {
        case result := <-results:
            acks = append(acks, result)
        case <-timeout:
            break
        }
    }

    // Calculate result
    acknowledged := 0
    for _, ack := range acks {
        if ack.Acknowledged {
            acknowledged++
        }
    }

    return &BroadcastResult{
        AlertID:         alert.ID,
        TotalStates:     len(targetStates),
        Acknowledged:    acknowledged,
        Pending:         len(targetStates) - acknowledged,
        Acknowledgments: acks,
    }, nil
}

func (s *NationalAlertSystem) sendToState(
    ctx context.Context,
    stateID string,
    alert *NationalAlert,
) StateAckResult {
    conn, ok := s.states[stateID]
    if !ok {
        return StateAckResult{
            StateID: stateID,
            Error:   "state not connected",
        }
    }

    client := pb.NewAlertServiceClient(conn.Connection)

    ack, err := client.ReceiveNationalAlert(ctx, alert.ToProto())
    if err != nil {
        return StateAckResult{
            StateID: stateID,
            Error:   err.Error(),
        }
    }

    return StateAckResult{
        StateID:      stateID,
        Acknowledged: true,
        AckTime:      time.Now(),
        AckBy:        ack.AcknowledgedBy,
    }
}
```

#### E.3.2 National Wanted List

```go
// central/wanted_list.go
type NationalWantedList struct {
    db           *sql.DB
    searchEngine *opensearch.Client
    cache        *redis.Client
    syncPub      *kafka.Producer
}

type WantedPerson struct {
    ID                string
    Name              string
    Aliases           []string
    DOB               time.Time
    Gender            string
    Nationality       string
    Photo             string
    IdentificationMarks string

    // Crime details
    CrimeType         string
    IPCSections       []string
    OriginatingState  string
    OriginatingFIR    string

    // Status
    Priority          WantedPriority // A, B, C
    Status            WantedStatus   // WANTED, DETAINED, ARRESTED, ABSCONDED
    Reward            decimal.Decimal

    // Dates
    WantedSince       time.Time
    LastSeenDate      time.Time
    LastSeenLocation  string

    // Timestamps
    CreatedAt         time.Time
    UpdatedAt         time.Time
}

func (w *NationalWantedList) Add(
    ctx context.Context,
    person *WantedPerson,
) error {
    // Validate
    if err := person.Validate(); err != nil {
        return err
    }

    // Store in database
    if err := w.db.Create(person); err != nil {
        return err
    }

    // Index for search
    if err := w.searchEngine.Index("wanted", person); err != nil {
        log.Error("search index failed", "error", err)
        // Non-fatal, continue
    }

    // Publish for sync to all states
    event := &WantedListEvent{
        Type:   "WANTED_ADDED",
        Person: person,
    }
    if err := w.syncPub.Publish("national.wanted", event); err != nil {
        return fmt.Errorf("sync publish failed: %w", err)
    }

    // Invalidate cache
    w.cache.Del(ctx, "wanted:list:*")

    return nil
}

func (w *NationalWantedList) Search(
    ctx context.Context,
    query SearchQuery,
) ([]WantedPerson, error) {
    // Build search
    searchBody := map[string]interface{}{
        "query": map[string]interface{}{
            "bool": map[string]interface{}{
                "should": []map[string]interface{}{
                    {"match": map[string]interface{}{"name": query.Term}},
                    {"match": map[string]interface{}{"aliases": query.Term}},
                    {"match": map[string]interface{}{
                        "identification_marks": query.Term,
                    }},
                },
                "filter": buildFilters(query),
            },
        },
    }

    result, err := w.searchEngine.Search(
        ctx,
        "wanted",
        searchBody,
    )
    if err != nil {
        return nil, err
    }

    return parseWantedResults(result), nil
}
```

### E.4 Dependencies

| Dependency | From Phase | Required For |
|------------|------------|--------------|
| State Federation | D | Data to aggregate nationally |
| State Sync Hubs | D | Communication channel |
| AI Services | C | National analytics |

### E.5 Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| National network reliability | Medium | Critical | Multiple redundant links, satellite backup |
| Sovereign data concerns | High | High | Data localization, clear policies |
| Central point of failure | Medium | Critical | Active-active across data centers |
| Political/jurisdictional issues | Medium | High | Clear authority matrix, legal framework |

### E.6 Validation & Acceptance Criteria

| Criterion | Test Method | Pass Condition |
|-----------|-------------|----------------|
| All states visible | Integration test | 36 states/UTs connected |
| National alert < 10 min | Timing test | Alert reaches all states |
| Wanted list sync | Sync test | Updates reach stations within 1 hour |
| Central failure recovery | DR test | Failover < 15 minutes |
| Inter-state case search | Search test | Cross-state results in < 5 seconds |

---

## Consolidated Risk Matrix

| Phase | Risk | Probability | Impact | Overall |
|-------|------|-------------|--------|---------|
| A | Sync complexity | Medium | High | HIGH |
| A | Security review delays | Medium | High | HIGH |
| B | FIR workflow complexity | Medium | Medium | MEDIUM |
| B | Evidence legal requirements | Medium | High | HIGH |
| C | AI bias | High | Critical | CRITICAL |
| C | AI over-reliance | Medium | High | HIGH |
| D | Scale performance | Medium | High | HIGH |
| D | Data consistency | Medium | High | HIGH |
| E | Central failure | Medium | Critical | CRITICAL |
| E | Jurisdictional issues | Medium | High | HIGH |

---

## Phase Execution Checklist

### Pre-Phase Checklist (All Phases)
- [ ] Previous phase validation complete
- [ ] Team assignments confirmed
- [ ] Infrastructure provisioned
- [ ] Security review scheduled
- [ ] Test environment ready
- [ ] Documentation updated

### Phase Completion Checklist (All Phases)
- [ ] All deliverables completed
- [ ] All tests passing
- [ ] Security audit passed
- [ ] Performance benchmarks met
- [ ] Documentation complete
- [ ] Training materials ready
- [ ] Rollback plan tested
- [ ] Stakeholder sign-off obtained
