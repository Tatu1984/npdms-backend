# PHASE 0 — SYSTEM BLUEPRINT & ARCHITECTURE
# National Police Department Management System (NPDMS)

## Document Control
- **Version**: 1.0
- **Classification**: RESTRICTED
- **Author**: Systems Architecture Team
- **Last Updated**: 2026-01-04

---

## 1. High-Level System Architecture

### 1.1 Tier Model

```
TIER 0: CENTRAL HOME MINISTRY
    │
    ├── National Crime Database (Aggregated)
    ├── Cross-State Intelligence Correlation
    ├── National Alert System
    ├── Policy Enforcement Engine
    ├── Central AI Training Cluster
    ├── Audit & Compliance Hub
    └── Disaster Recovery Orchestrator
    │
    ▼
TIER 1: STATE POLICE HQ (×28 States + 8 UTs)
    │
    ├── State Crime Database
    ├── State Intelligence Unit
    ├── AI Inference Cluster
    ├── State Sync Engine
    └── Personnel Master
    │
    ▼
TIER 2: DISTRICT HQ (×750+ Districts)
    │
    ├── District Crime Database
    ├── Edge AI Inference
    ├── Sync Engine
    └── Evidence Repository
    │
    ▼
TIER 3: POLICE STATION (×17,000+ Stations)
    │
    ├── Local PostgreSQL
    ├── Local OpenSearch
    ├── Edge AI Runtime (ONNX)
    ├── Sync Agent
    └── Evidence Vault
```

### 1.2 Architecture Diagram (ASCII)

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                           CENTRAL HOME MINISTRY (TIER 0)                                 │
│  ┌────────────────────────────────────────────────────────────────────────────────────┐ │
│  │  • National Crime Database (Aggregated)                                             │ │
│  │  • Cross-State Intelligence Correlation                                             │ │
│  │  • National Alert System                                                            │ │
│  │  • Policy Enforcement Engine                                                        │ │
│  │  • Central AI Training Cluster                                                      │ │
│  │  • Audit & Compliance Hub                                                           │ │
│  │  • Disaster Recovery Orchestrator                                                   │ │
│  └────────────────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────────────────┘
                                           │
           ┌───────────────────────────────┼───────────────────────────────┐
           │                               │                               │
           ▼                               ▼                               ▼
┌─────────────────────┐     ┌─────────────────────┐     ┌─────────────────────┐
│   STATE HQ (TIER 1) │     │   STATE HQ (TIER 1) │     │   STATE HQ (TIER 1) │
│   ─────────────────  │     │   ─────────────────  │     │   ─────────────────  │
│ • State Crime DB    │     │ • State Crime DB    │     │ • State Crime DB    │
│ • State Intel Unit  │     │ • State Intel Unit  │     │ • State Intel Unit  │
│ • AI Inference Clstr│     │ • AI Inference Clstr│     │ • AI Inference Clstr│
│ • State Sync Engine │     │ • State Sync Engine │     │ • State Sync Engine │
│ • Personnel Master  │     │ • Personnel Master  │     │ • Personnel Master  │
└─────────────────────┘     └─────────────────────┘     └─────────────────────┘
           │                               │                               │
     ┌─────┴─────┐                   ┌─────┴─────┐                   ┌─────┴─────┐
     │           │                   │           │                   │           │
     ▼           ▼                   ▼           ▼                   ▼           ▼
┌─────────┐ ┌─────────┐       ┌─────────┐ ┌─────────┐       ┌─────────┐ ┌─────────┐
│DISTRICT │ │DISTRICT │       │DISTRICT │ │DISTRICT │       │DISTRICT │ │DISTRICT │
│HQ(TIER2)│ │HQ(TIER2)│       │HQ(TIER2)│ │HQ(TIER2)│       │HQ(TIER2)│ │HQ(TIER2)│
│─────────│ │─────────│       │─────────│ │─────────│       │─────────│ │─────────│
│•Dist DB │ │•Dist DB │       │•Dist DB │ │•Dist DB │       │•Dist DB │ │•Dist DB │
│•Edge AI │ │•Edge AI │       │•Edge AI │ │•Edge AI │       │•Edge AI │ │•Edge AI │
│•Sync Eng│ │•Sync Eng│       │•Sync Eng│ │•Sync Eng│       │•Sync Eng│ │•Sync Eng│
└─────────┘ └─────────┘       └─────────┘ └─────────┘       └─────────┘ └─────────┘
           │                               │                               │
     ┌─────┴─────┐                   ┌─────┴─────┐                   ┌─────┴─────┐
     ▼           ▼                   ▼           ▼                   ▼           ▼
┌─────────┐ ┌─────────┐       ┌─────────┐ ┌─────────┐       ┌─────────┐ ┌─────────┐
│  PS     │ │  PS     │       │  PS     │ │  PS     │       │  PS     │ │  PS     │
│(TIER 3) │ │(TIER 3) │       │(TIER 3) │ │(TIER 3) │       │(TIER 3) │ │(TIER 3) │
│─────────│ │─────────│       │─────────│ │─────────│       │─────────│ │─────────│
│•Local DB│ │•Local DB│       │•Local DB│ │•Local DB│       │•Local DB│ │•Local DB│
│•Edge AI │ │•Edge AI │       │•Edge AI │ │•Edge AI │       │•Edge AI │ │•Edge AI │
│•Sync Agt│ │•Sync Agt│       │•Sync Agt│ │•Sync Agt│       │•Sync Agt│ │•Sync Agt│
└─────────┘ └─────────┘       └─────────┘ └─────────┘       └─────────┘ └─────────┘

PS = Police Station
```

---

## 2. Data Flow Diagrams

### 2.1 Operational Data Flow (Bottom-Up Sync)

```
POLICE STATION (Creates Data)
     │
     │ [1] FIR Created Locally
     │     - Stored in local PostgreSQL
     │     - Indexed in local OpenSearch
     │     - Queued for sync (Kafka/Outbox)
     │
     ▼
┌─────────────────────────────────────┐
│  SYNC QUEUE (LOCAL)                 │
│  • Event: FIR_CREATED               │
│  • Payload: Encrypted FIR blob      │
│  • Metadata: Station ID, Timestamp  │
│  • Priority: NORMAL/URGENT          │
│  • Retry count: 0                   │
└─────────────────────────────────────┘
     │
     │ [2] Network Available?
     │     YES → Immediate sync
     │     NO  → Queue persists locally
     │
     ▼
DISTRICT HQ (Aggregates)
     │
     │ [3] Receives from all stations
     │     - Validates integrity (hash check)
     │     - Deduplicates
     │     - Applies district-level policies
     │     - Enriches with district context
     │
     ▼
┌─────────────────────────────────────┐
│  DISTRICT AGGREGATED VIEW           │
│  • All FIRs from N stations         │
│  • Cross-station case linking       │
│  • District crime analytics         │
│  • Resource allocation view         │
└─────────────────────────────────────┘
     │
     │ [4] Selective upward sync
     │     - Only metadata + aggregates to State
     │     - Full records only on-demand
     │     - Classified data stays local
     │
     ▼
STATE HQ (State-wide Intelligence)
     │
     │ [5] Aggregates from all districts
     │     - State-level crime patterns
     │     - Inter-district correlations
     │     - AI model refinement data
     │
     ▼
┌─────────────────────────────────────┐
│  STATE INTELLIGENCE LAYER           │
│  • State crime database             │
│  • Cross-district gang tracking     │
│  • Resource deployment optimization │
└─────────────────────────────────────┘
     │
     │ [6] National-level sync
     │     - Aggregated statistics only
     │     - Inter-state alerts
     │     - National wanted lists
     │
     ▼
CENTRAL HOME MINISTRY (National View)
```

### 2.2 Command Flow (Top-Down)

```
CENTRAL HOME MINISTRY
     │
     │ [1] Issues Directive
     │     Type: ALERT / POLICY / LOOKOUT / OPERATION
     │     Scope: National / Regional / State-specific
     │     Priority: ROUTINE / URGENT / FLASH
     │
     ▼
┌─────────────────────────────────────┐
│  DIRECTIVE BROADCAST                │
│  • Digitally signed by authority    │
│  • Includes expiry timestamp        │
│  • Acknowledgment required          │
│  • Audit trail mandatory            │
└─────────────────────────────────────┘
     │
     │ [2] Parallel broadcast to all State HQs
     │
     ├────────────────┬────────────────┐
     ▼                ▼                ▼
STATE HQ 1       STATE HQ 2       STATE HQ N
     │                │                │
     │ [3] State validates & acknowledges
     │     - Checks digital signature
     │     - Logs receipt
     │     - Forwards to districts
     │
     ▼
DISTRICT HQ (receives, forwards)
     │
     │ [4] District-level acknowledgment
     │     - Cascades to all stations
     │
     ▼
POLICE STATION (receives, executes)
     │
     │ [5] Final acknowledgment + status
     │     - Updates flow back up
     │
     ▼
EXECUTION STATUS AGGREGATED UPWARD
```

### 2.3 Evidence Chain of Custody Flow

```
CRIME SCENE
     │
     │ [1] Evidence Collected
     │     - Physical: Tagged + photographed
     │     - Digital: Hash computed immediately
     │
     ▼
┌─────────────────────────────────────┐
│  EVIDENCE REGISTRATION (Edge)       │
│  • Unique evidence ID (UUID v7)     │
│  • SHA-256 hash of digital evidence │
│  • Collecting officer biometric     │
│  • GPS coordinates + timestamp      │
│  • Initial classification           │
└─────────────────────────────────────┘
     │
     │ [2] Local Storage (Immutable)
     │     - Write-once storage
     │     - Encrypted at rest (AES-256-GCM)
     │     - Replicated to district within 4 hours
     │
     ▼
DISTRICT EVIDENCE VAULT
     │
     │ [3] Forensic Lab Routing
     │     - Request queued
     │     - Physical evidence tracked
     │     - Digital copy sent for analysis
     │
     ▼
FORENSIC ANALYSIS
     │
     │ [4] Results Attached
     │     - Forensic report linked
     │     - Chain updated (append-only)
     │     - Original never modified
     │
     ▼
COURT SUBMISSION
     │
     │ [5] Legal Export
     │     - Complete audit trail included
     │     - All custody transfers logged
     │     - Integrity verifiable by court
```

---

## 3. Trust Boundaries

### 3.1 Trust Zone Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ TRUST ZONE 0: CENTRAL (HIGHEST CLASSIFICATION)                              │
│ ═══════════════════════════════════════════════                             │
│ • National intelligence databases                                           │
│ • Inter-state criminal networks                                             │
│ • National security alerts                                                  │
│ • AI model training infrastructure                                          │
│                                                                             │
│ Access: Ministry officials, National security personnel                     │
│ Auth: Multi-factor + Hardware token + Biometric                             │
│ Audit: Real-time SOC monitoring                                             │
│ Network: Air-gapped segment with controlled gateways                        │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                            [Gateway: mTLS + ABAC]
                                    │
┌─────────────────────────────────────────────────────────────────────────────┐
│ TRUST ZONE 1: STATE (STATE-LEVEL CLASSIFICATION)                            │
│ ═══════════════════════════════════════════════                             │
│ • State intelligence databases                                              │
│ • State-wide personnel records                                              │
│ • Cross-district case correlation                                           │
│ • State AI inference cluster                                                │
│                                                                             │
│ Access: State DGP office, State intelligence                                │
│ Auth: Multi-factor + Certificate + Biometric                                │
│ Audit: State SOC + Central visibility                                       │
│ Network: State WAN with encryption                                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                          [Gateway: mTLS + RBAC]
                                    │
┌─────────────────────────────────────────────────────────────────────────────┐
│ TRUST ZONE 2: DISTRICT (OPERATIONAL CLASSIFICATION)                         │
│ ═══════════════════════════════════════════════════                         │
│ • District crime database                                                   │
│ • District personnel                                                        │
│ • Evidence storage                                                          │
│ • Edge AI inference                                                         │
│                                                                             │
│ Access: District SP/DCP office, Investigation officers                      │
│ Auth: Password + OTP + Device certificate                                   │
│ Audit: District + State visibility                                          │
│ Network: District LAN + VPN to State                                        │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                          [Gateway: TLS + RBAC]
                                    │
┌─────────────────────────────────────────────────────────────────────────────┐
│ TRUST ZONE 3: STATION (OPERATIONAL / RESTRICTED)                            │
│ ══════════════════════════════════════════════════                          │
│ • Local FIR database                                                        │
│ • Daily logs                                                                │
│ • Attendance records                                                        │
│ • Local evidence cache                                                      │
│                                                                             │
│ Access: Station staff (role-based)                                          │
│ Auth: Password + Biometric + Device binding                                 │
│ Audit: Local + District visibility                                          │
│ Network: Station LAN (potentially air-gapped)                               │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                          [Strict Interface]
                                    │
┌─────────────────────────────────────────────────────────────────────────────┐
│ TRUST ZONE 4: CITIZEN (UNTRUSTED / PUBLIC)                                  │
│ ══════════════════════════════════════════                                  │
│ • Complaint submission                                                      │
│ • Status tracking (redacted)                                                │
│ • Public information                                                        │
│                                                                             │
│ Access: General public                                                      │
│ Auth: OTP-based / DigiLocker                                                │
│ Audit: All interactions logged                                              │
│ Network: Public internet (rate-limited, WAF-protected)                      │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Cross-Boundary Rules

| Rule | Description |
|------|-------------|
| Data flows DOWN freely | With classification markings preserved |
| Data flows UP with controls | Only after declassification/aggregation |
| No upward classification breach | No zone can access data above its classification |
| All cross-zone encryption | All communication encrypted + authenticated |
| Immutable audit logging | All cross-zone access logged immutably |

---

## 4. Component Responsibilities

### 4.1 Edge Node (Police Station) Components

| Component | Technology | Responsibility |
|-----------|------------|---------------|
| Local Database | PostgreSQL 16 | Primary transactional data (FIRs, cases, personnel) |
| Search Engine | OpenSearch | Full-text search on local records |
| Sync Engine | Go + gRPC | Manages outbound sync queue, handles reconnection |
| Edge AI Runtime | ONNX + OpenVINO | Inference for OCR, NLP |
| Auth Service | Go + SQLite | Local authentication with offline capability |
| Audit Logger | Go + append-only file | Append-only local audit trail |
| Evidence Vault | MinIO + EncFS | Encrypted evidence storage |

### 4.2 District HQ Components

| Component | Technology | Responsibility |
|-----------|------------|---------------|
| District Database | PostgreSQL 16 Cluster | Aggregated data from all stations |
| Distributed Store | ScyllaDB | High-throughput writes from stations |
| AI Cluster | NVIDIA Triton | Powerful inference, batch processing |
| Message Broker | Redpanda | Event streaming from/to stations |
| Sync Orchestrator | Go | Manages sync with all stations + State |
| Analytics Engine | Apache Spark | District-level crime analytics |
| Evidence Repository | MinIO Cluster | District-level evidence aggregation |

### 4.3 State HQ Components

| Component | Technology | Responsibility |
|-----------|------------|---------------|
| State Database | PostgreSQL 16 HA | State-wide authoritative data |
| Graph Database | Neo4j Enterprise | Criminal network graph analysis |
| AI Training | PyTorch + NVIDIA | Model fine-tuning on state data |
| Intelligence Fusion | Custom Go services | Cross-district correlation |
| State Message Hub | Kafka | Manages all district connections |
| Compliance Engine | OPA (Open Policy Agent) | State-level policy enforcement |

### 4.4 Central Ministry Components

| Component | Technology | Responsibility |
|-----------|------------|---------------|
| National Warehouse | PostgreSQL + TimescaleDB | Aggregated national statistics |
| AI Training Cluster | PyTorch + Kubernetes | National model training |
| Alert System | Custom Go + Kafka | Broadcast infrastructure |
| Policy Engine | OPA + Custom | National policy distribution |
| Audit Central | Elasticsearch + Kibana | National audit aggregation |
| DR Orchestrator | Terraform + Ansible | Disaster recovery coordination |

---

## 5. Failure Scenarios & Fallback Behavior

### 5.1 Network Failure Matrix

| Failure Scenario | Impact | Fallback Behavior | Recovery |
|-----------------|--------|-------------------|----------|
| Station ↔ District link down | Station isolated | Full local operation continues. Sync queue grows. All operations logged locally. | Auto-sync when connection restored. Conflict resolution via vector clocks. |
| District ↔ State link down | District isolated | District operates with local + station data. Inter-district queries fail gracefully. | Bulk sync on reconnection. State receives aggregated updates. |
| State ↔ Central link down | State isolated | State operates independently. National alerts cached locally. | Reconnection triggers national sync. |
| Complete station power failure | Station down | Other stations unaffected. District marks station as offline. | Cold boot recovery from local disk. |
| District data center failure | District down | Stations continue locally. State routes around failed district. | Failover to DR site. Stations reconnect automatically. |
| AI inference service down | AI unavailable | Manual fallback for all AI-assisted features. System remains fully functional. | AI service restart. No data loss. |

### 5.2 Conflict Resolution Strategy

```
SCENARIO: Same FIR edited at Station and District during network partition

DETECTION:
  • Each record has vector clock [station_version, district_version, ...]
  • On sync, vector clocks compared
  • Divergent clocks = conflict detected

RESOLUTION RULES (in order of precedence):
  1. LEGAL PRECEDENCE: Court-submitted version is canonical
  2. RANK PRECEDENCE: Higher authority's edit wins (configurable)
  3. TEMPORAL PRECEDENCE: Later timestamp wins
  4. MERGE: Non-conflicting fields merged automatically
  5. MANUAL: Conflicting changes flagged for human resolution

CONFLICT RECORD:
  • Both versions preserved
  • Resolution audit trail created
  • Resolving officer recorded
  • Resolution timestamp logged
```

### 5.3 Degraded Mode Operations

```
MODE: FULLY OFFLINE (Station isolated for extended period)
─────────────────────────────────────────────────────────

AVAILABLE OPERATIONS:
  ✓ FIR creation and management
  ✓ Case diary updates
  ✓ Evidence registration
  ✓ Personnel attendance
  ✓ Duty roster management
  ✓ Local search (all local records)
  ✓ OCR (edge AI)
  ✓ FIR structuring (edge AI)

UNAVAILABLE OPERATIONS:
  ✗ Cross-station case search
  ✗ National wanted list lookup (real-time)
  ✗ Inter-state case linking
  ✗ Real-time alerts from higher HQ
  ✗ Predictive analytics (needs aggregated data)

MITIGATIONS:
  • Last-known wanted list cached locally (TTL: 24 hours)
  • Critical alerts cached with TTL
  • Degraded mode banner shown in UI
  • Sync pending indicator visible
  • All actions queued for eventual sync
```

---

## 6. Synchronization Model

### 6.1 Federated Sync Architecture

```
                    ┌─────────────────────────────────────┐
                    │        CENTRAL SYNC HUB             │
                    │  • Global event ordering            │
                    │  • Cross-state routing              │
                    │  • Conflict arbitration             │
                    └─────────────────────────────────────┘
                                     │
                    ┌────────────────┼────────────────┐
                    ▼                ▼                ▼
           ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
           │ STATE SYNC   │  │ STATE SYNC   │  │ STATE SYNC   │
           │ HUB (KAFKA)  │  │ HUB (KAFKA)  │  │ HUB (KAFKA)  │
           │              │  │              │  │              │
           │ Topics:      │  │ Topics:      │  │ Topics:      │
           │ • fir.events │  │ • fir.events │  │ • fir.events │
           │ • case.events│  │ • case.events│  │ • case.events│
           │ • alert.cmd  │  │ • alert.cmd  │  │ • alert.cmd  │
           └──────────────┘  └──────────────┘  └──────────────┘
                    │                │                │
           ┌────────┴────────┐      ...             ...
           ▼                 ▼
    ┌─────────────┐   ┌─────────────┐
    │ DISTRICT    │   │ DISTRICT    │
    │ SYNC NODE   │   │ SYNC NODE   │
    │             │   │             │
    │ • Local     │   │ • Local     │
    │   Redpanda  │   │   Redpanda  │
    │ • Outbox    │   │ • Outbox    │
    │   pattern   │   │   pattern   │
    └─────────────┘   └─────────────┘
           │                 │
    ┌──────┴──────┐   ┌──────┴──────┐
    ▼             ▼   ▼             ▼
┌────────┐  ┌────────┐ ┌────────┐  ┌────────┐
│STATION │  │STATION │ │STATION │  │STATION │
│ SYNC   │  │ SYNC   │ │ SYNC   │  │ SYNC   │
│ AGENT  │  │ AGENT  │ │ AGENT  │  │ AGENT  │
└────────┘  └────────┘ └────────┘  └────────┘
```

### 6.2 Outbox Pattern Implementation

```
1. APPLICATION WRITES (Transactional):
   BEGIN TRANSACTION;
     INSERT INTO fir (...) VALUES (...);
     INSERT INTO sync_outbox (
       event_type,
       payload,
       created_at,
       priority,
       destination_tier
     ) VALUES ('FIR_CREATED', {...}, NOW(), 'NORMAL', 'DISTRICT');
   COMMIT;

2. SYNC AGENT POLLS OUTBOX:
   SELECT * FROM sync_outbox
   WHERE status = 'PENDING'
   ORDER BY priority DESC, created_at ASC
   LIMIT 100;

3. SYNC AGENT PUBLISHES:
   • Batch publish to district message broker
   • On success: UPDATE sync_outbox SET status = 'SENT', sent_at = NOW()
   • On failure: Exponential backoff retry (max 5 attempts)

4. DISTRICT RECEIVES & PROCESSES:
   • Validates message integrity (HMAC)
   • Deduplicates by event_id
   • Stores in district database
   • Acknowledges receipt
   • Publishes to state (if applicable)

5. STATE RECEIVES & AGGREGATES:
   • Aggregates from all districts
   • Publishes national-relevant events to central
```

### 6.3 Sync Priorities

| Priority | Max Delay | Use Case |
|----------|-----------|----------|
| FLASH | Immediate | Arrests, emergencies, officer down |
| URGENT | 5 minutes | Serious crimes, wanted matches |
| NORMAL | 15 minutes | Standard FIRs, case updates |
| BULK | 1 hour | Historical data, reports, analytics |

### 6.4 Data Classification & Sync Rules

| Data Type | Default Sync Level | Higher Sync Conditions |
|-----------|-------------------|------------------------|
| FIR Metadata | District | Always synced |
| FIR Full Record | Station + District | On-demand from State/Central |
| Evidence Metadata | District | Always synced |
| Evidence Files | Station + District | Never auto-sync to State |
| Personnel Records | State | Central only on explicit request |
| Intelligence Reports | State | Central only for inter-state relevance |
| Audit Logs | District | Central on compliance request |
| National Alerts | Central → All | Broadcast always |
| Wanted Lists | Central → All | Broadcast with local cache |

---

## 7. Security Architecture Summary

### 7.1 Zero Trust Principles Applied

1. **Never Trust, Always Verify**: Every request authenticated regardless of source
2. **Least Privilege**: Minimal access granted, role-based
3. **Assume Breach**: All segments isolated, lateral movement prevented
4. **Explicit Verification**: Context-aware access decisions (device, location, time)
5. **End-to-End Encryption**: All data encrypted in transit and at rest

### 7.2 Authentication Stack

| Tier | Primary Auth | Secondary Auth | Device Binding |
|------|-------------|----------------|----------------|
| Central | PIV/CAC Card | Biometric | HSM-backed certificates |
| State | Certificate | OTP + Biometric | Device certificate |
| District | Password | OTP + Biometric | Device certificate |
| Station | Password | Biometric | Device MAC binding |
| Citizen | OTP | DigiLocker | None |

### 7.3 Encryption Standards

| Data State | Algorithm | Key Management |
|------------|-----------|----------------|
| At Rest | AES-256-GCM | HashiCorp Vault |
| In Transit | TLS 1.3 | PKI with HSM |
| Database | TDE (Transparent Data Encryption) | Vault + HSM |
| Evidence | AES-256-GCM + integrity hash | Per-evidence keys |
| Backups | AES-256-GCM | Offline master keys |

---

## 8. Scalability Projections

### 8.1 Expected Scale

| Metric | Initial | Year 5 | Year 20 |
|--------|---------|--------|---------|
| Police Stations | 17,000 | 18,500 | 22,000 |
| Daily FIRs | 50,000 | 75,000 | 150,000 |
| Active Cases | 2M | 3.5M | 8M |
| Evidence Items | 10M | 50M | 500M |
| Personnel Records | 2.5M | 3M | 4M |
| Daily Queries | 5M | 15M | 50M |

### 8.2 Storage Projections

| Data Category | Year 1 | Year 5 | Year 20 |
|---------------|--------|--------|---------|
| Structured Data | 5 TB | 25 TB | 200 TB |
| Evidence (Digital) | 50 TB | 500 TB | 10 PB |
| Audit Logs | 10 TB | 100 TB | 2 PB |
| AI Models | 500 GB | 2 TB | 20 TB |

---

## Appendix A: Glossary

| Term | Definition |
|------|------------|
| Edge Node | Police station or district unit operating autonomously |
| Sync Engine | Component managing data synchronization between tiers |
| Vector Clock | Logical clock for conflict detection in distributed systems |
| Outbox Pattern | Transactional pattern ensuring reliable event publishing |
| ABAC | Attribute-Based Access Control |
| mTLS | Mutual TLS (both parties authenticate) |
| HSM | Hardware Security Module |
| OPA | Open Policy Agent |

---

## Appendix B: Referenced Standards

- ISO 27001:2022 - Information Security Management
- ISO 27701:2019 - Privacy Information Management
- NIST SP 800-53 - Security and Privacy Controls
- NIST SP 800-207 - Zero Trust Architecture
- Indian IT Act 2000 & Rules
- Evidence Act - Digital Evidence Handling
- CrPC - Procedural Requirements
