# PHASE 4 — CORE TECH STACK
# National Police Department Management System (NPDMS)

## Document Control
- **Version**: 1.0
- **Classification**: RESTRICTED
- **Author**: Technology Architecture Team
- **Last Updated**: 2026-01-04

---

## 1. Technology Stack Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          NPDMS TECHNOLOGY STACK                              │
└─────────────────────────────────────────────────────────────────────────────┘

┌─── FRONTEND ─────────────────────────────────────────────────────────────────┐
│  Next.js 14 │ React 18 │ TypeScript │ TailwindCSS │ Zustand │ TanStack Query │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── API GATEWAY ──────────────────────────────────────────────────────────────┐
│                        Kong / Custom Go Gateway                               │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── BACKEND SERVICES ─────────────────────────────────────────────────────────┐
│  Go 1.22+ (Core Services)  │  Rust (Cryptography)  │  Python 3.11+ (AI/ML)   │
│  gRPC + Protobuf           │  REST/JSON (External) │  FastAPI (AI APIs)      │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── MESSAGE BROKER ───────────────────────────────────────────────────────────┐
│               Apache Kafka / Redpanda (Edge-compatible)                       │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── DATABASES ────────────────────────────────────────────────────────────────┐
│  PostgreSQL 16   │  ScyllaDB      │  Neo4j 5       │  OpenSearch 2.x         │
│  (Transactional) │  (Distributed) │  (Graph)       │  (Search & Analytics)   │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── CACHING ──────────────────────────────────────────────────────────────────┐
│                         Redis 7 (Cluster Mode)                                │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── OBJECT STORAGE ───────────────────────────────────────────────────────────┐
│                         MinIO (S3-Compatible)                                 │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── AI/ML INFRASTRUCTURE ─────────────────────────────────────────────────────┐
│  PyTorch 2.x │ ONNX Runtime │ OpenVINO │ NVIDIA Triton │ Hugging Face        │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── CONTAINER ORCHESTRATION ──────────────────────────────────────────────────┐
│  Kubernetes (Central/State) │ K3s (District/Station Edge)                    │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── SECURITY ─────────────────────────────────────────────────────────────────┐
│  HashiCorp Vault │ HSM (PKCS#11) │ WireGuard/ZeroTier │ OPA │ Falco          │
└──────────────────────────────────────────────────────────────────────────────┘

┌─── OBSERVABILITY ────────────────────────────────────────────────────────────┐
│  Prometheus │ Grafana │ Loki │ Tempo │ OpenTelemetry                          │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Backend Technologies

### 2.1 Go (Golang) — Core Services

**Version**: Go 1.22+

**Use For**:
- All core microservices (auth, FIR, case, evidence, personnel, etc.)
- API Gateway
- Sync Engine
- High-performance data processing

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **Performance** | Compiled language with near-C performance. Critical for high-throughput sync operations. |
| **Concurrency** | Native goroutines handle thousands of concurrent connections efficiently. Essential for handling multiple station syncs. |
| **Memory Safety** | Garbage collected with no manual memory management. Reduces security vulnerabilities. |
| **Static Typing** | Compile-time type checking catches errors early. Critical for government systems. |
| **Deployment** | Single static binary. No runtime dependencies. Simplified edge deployment. |
| **Ecosystem** | Mature ecosystem for microservices (gRPC, database drivers, observability). |
| **Government Precedent** | Used by Kubernetes, Docker, and government systems worldwide. |

**Key Libraries**:
```go
// Web Framework
github.com/gin-gonic/gin          // HTTP routing
github.com/grpc-ecosystem/grpc-gateway // REST-gRPC bridge

// Database
github.com/jackc/pgx/v5           // PostgreSQL driver
github.com/scylladb/gocqlx/v2     // ScyllaDB driver
github.com/neo4j/neo4j-go-driver  // Neo4j driver

// Messaging
github.com/twmb/franz-go          // Kafka client

// Security
github.com/golang-jwt/jwt/v5      // JWT handling
golang.org/x/crypto               // Cryptographic primitives

// Observability
go.opentelemetry.io/otel          // Tracing
github.com/prometheus/client_golang // Metrics
```

### 2.2 Rust — Cryptographic Services

**Version**: Rust 1.75+ (stable)

**Use For**:
- Cryptographic operations (encryption, signing, hashing)
- HSM integration
- Zero-copy data handling for evidence files

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **Memory Safety** | Zero-cost abstractions with compile-time memory safety. No buffer overflows, no use-after-free. |
| **Performance** | C-level performance for cryptographic operations. No garbage collection pauses. |
| **Security Focus** | Language designed with security as primary goal. Perfect for handling sensitive operations. |
| **HSM Integration** | Strong PKCS#11 bindings for hardware security modules. |
| **Audit Readiness** | Rust code is easier to audit for security vulnerabilities than C/C++. |

**Key Crates**:
```toml
# Cargo.toml
[dependencies]
# Cryptography
ring = "0.17"                      # Core crypto primitives
ed25519-dalek = "2.0"              # Ed25519 signatures
aes-gcm = "0.10"                   # AES-GCM encryption
sha2 = "0.10"                      # SHA-256 hashing
argon2 = "0.5"                     # Password hashing

# HSM
pkcs11 = "0.5"                     # PKCS#11 bindings
cryptoki = "0.6"                   # HSM abstraction

# API
tonic = "0.11"                     # gRPC
tokio = { version = "1", features = ["full"] }  # Async runtime

# Serialization
serde = { version = "1.0", features = ["derive"] }
prost = "0.12"                     # Protobuf
```

### 2.3 Python — AI/ML Services

**Version**: Python 3.11+

**Use For**:
- OCR service
- NLP service
- Computer vision
- Graph analytics
- Model training

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **AI Ecosystem** | Unmatched ML/AI library ecosystem (PyTorch, Transformers, SpaCy). |
| **Model Availability** | Direct access to Hugging Face models, pretrained weights. |
| **Rapid Prototyping** | Faster iteration on AI features compared to compiled languages. |
| **Research Transfer** | Easy to port academic research directly to production. |
| **Edge Inference** | ONNX Runtime and OpenVINO have excellent Python bindings. |

**Key Libraries**:
```txt
# requirements.txt

# ML Framework
torch==2.2.0
transformers==4.37.0
sentence-transformers==2.3.0

# NLP
spacy==3.7.0
indic-nlp-library==0.92

# Computer Vision
opencv-python==4.9.0
ultralytics==8.1.0

# Inference
onnxruntime==1.17.0
openvino==2024.0.0

# API
fastapi==0.109.0
uvicorn==0.27.0
pydantic==2.6.0

# Database
asyncpg==0.29.0
neo4j==5.17.0
```

---

## 3. Frontend Technologies

### 3.1 Next.js 14 + React 18

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **Server Components** | Reduced JavaScript bundle size. Faster initial load for edge deployments. |
| **App Router** | Modern routing with layouts, loading states, error boundaries. |
| **Static Generation** | Pre-render static pages for offline capability. |
| **API Routes** | Backend-for-frontend pattern for secure API proxying. |
| **TypeScript Native** | First-class TypeScript support for type safety. |
| **PWA Support** | Easy Progressive Web App configuration for offline-first. |

### 3.2 State Management

**TanStack Query (React Query)** — Server State:
```typescript
// Handles server state: caching, background refetch, stale-while-revalidate
const { data, isLoading, error } = useQuery({
  queryKey: ['fir', firId],
  queryFn: () => firService.getFIR(firId),
  staleTime: 5 * 60 * 1000, // 5 minutes
  cacheTime: 30 * 60 * 1000, // 30 minutes
});
```

**Zustand** — Client State:
```typescript
// Handles client state: auth, UI, offline queue
interface AuthState {
  user: User | null;
  token: string | null;
  login: (credentials: Credentials) => Promise<void>;
  logout: () => void;
}

const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      token: null,
      login: async (credentials) => { /* ... */ },
      logout: () => set({ user: null, token: null }),
    }),
    { name: 'auth-storage' }
  )
);
```

### 3.3 UI Framework

**TailwindCSS** — Utility-First CSS:
```typescript
// Consistent design system via configuration
// tailwind.config.js
module.exports = {
  theme: {
    extend: {
      colors: {
        'npdms-primary': '#1F6FEB',
        'npdms-success': '#238636',
        'npdms-error': '#DA3633',
        'npdms-warning': '#9E6A03',
      },
    },
  },
};
```

**Component Library**: Custom components built on Radix UI primitives for accessibility.

---

## 4. Database Technologies

### 4.1 PostgreSQL 16 — Primary Transactional Database

**Use For**:
- All transactional data (FIRs, cases, personnel, etc.)
- Complex queries with joins
- Strong consistency requirements

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **ACID Compliance** | Full transactional integrity. Critical for legal records. |
| **JSON Support** | JSONB for flexible schema evolution without migrations. |
| **Full-Text Search** | Built-in FTS for local search before OpenSearch sync. |
| **Replication** | Streaming replication for HA. Logical replication for sync. |
| **Extensions** | PostGIS (spatial), pg_crypto (encryption), pg_audit (auditing). |
| **Government Standard** | Widely adopted in government systems. Strong support ecosystem. |

**Configuration for Edge**:
```sql
-- Optimized for edge deployment (limited resources)
-- postgresql.conf

shared_buffers = 256MB           # 25% of available RAM on edge
effective_cache_size = 768MB     # 75% of available RAM
work_mem = 16MB
maintenance_work_mem = 64MB
wal_level = logical              # Required for sync
max_wal_senders = 3
wal_keep_size = 1GB
```

### 4.2 ScyllaDB — Distributed Database (State/Central)

**Use For**:
- High-throughput event storage
- Time-series data (audit logs, telemetry)
- Cross-region replication

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **Throughput** | 10x faster than Cassandra. Handles massive event streams. |
| **Low Latency** | P99 latencies under 10ms. Critical for real-time dashboards. |
| **Linear Scaling** | Add nodes without performance degradation. |
| **Multi-DC** | Native multi-datacenter replication for geo-distribution. |

### 4.3 Neo4j 5 — Graph Database

**Use For**:
- Criminal network analysis
- Relationship mapping (accused, witnesses, cases)
- Community detection
- Pattern matching

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **Native Graph** | Index-free adjacency. O(1) relationship traversal. |
| **Cypher Query** | Expressive query language for complex patterns. |
| **GDS Library** | Built-in graph algorithms (PageRank, Louvain, betweenness). |
| **Visualization** | Native visualization for investigation dashboards. |

**Example Query**:
```cypher
// Find all persons connected to a suspect within 3 hops
MATCH path = (suspect:Person {id: $suspectId})-[:ASSOCIATED_WITH*1..3]-(connected:Person)
WHERE connected.id <> suspect.id
RETURN connected, length(path) as distance
ORDER BY distance
```

### 4.4 OpenSearch 2.x — Search & Analytics

**Use For**:
- Full-text search across all records
- Log aggregation
- Real-time analytics dashboards

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **Apache License** | Fully open source. No vendor lock-in concerns. |
| **Scalability** | Distributed by design. Handles petabyte-scale. |
| **Query DSL** | Powerful query language for complex searches. |
| **Aggregations** | Real-time analytics without separate data warehouse. |

---

## 5. Messaging & Event Streaming

### 5.1 Apache Kafka / Redpanda

**Primary**: Redpanda for edge (lighter), Kafka for central (mature)

**Use For**:
- Event sourcing for all state changes
- Sync queue between tiers
- Audit event streaming
- Real-time alert propagation

**Justification**:

| Factor | Rationale |
|--------|-----------|
| **Durability** | Persistent log. Events survive restarts. |
| **Ordering** | Strict ordering within partitions. Essential for sync. |
| **Replay** | Can replay events for recovery or new consumers. |
| **Scalability** | Horizontal scaling to millions of events/second. |

**Redpanda Advantages for Edge**:
- Single binary deployment
- No JVM dependency
- Lower memory footprint (300MB vs 4GB+)
- Compatible with Kafka API

**Topic Design**:
```
# Sync topics (per station)
station.{station_id}.events.fir
station.{station_id}.events.case
station.{station_id}.events.evidence

# District aggregation
district.{district_id}.events.aggregated

# State topics
state.{state_id}.events.aggregated
state.{state_id}.alerts

# National topics
national.alerts
national.wanted
national.statistics
```

---

## 6. AI/ML Stack

### 6.1 Training Infrastructure (Central)

**PyTorch 2.x**:
- Primary framework for model development
- Native ONNX export for edge deployment
- Distributed training support

**NVIDIA Infrastructure**:
- GPU cluster with A100/H100 for training
- CUDA 12.x for GPU acceleration
- cuDNN for optimized deep learning

### 6.2 Inference Infrastructure

**Edge Inference (Station/District)**:

| Tool | Use Case | Rationale |
|------|----------|-----------|
| **ONNX Runtime** | Cross-platform inference | Runs on any hardware, optimized for CPU |
| **OpenVINO** | Intel CPU optimization | 3-5x speedup on Intel processors |

**Central/State Inference**:

| Tool | Use Case | Rationale |
|------|----------|-----------|
| **NVIDIA Triton** | Model serving | Production-grade, supports batching |
| **vLLM** | LLM inference | Efficient serving for large models |

**Model Format Pipeline**:
```
PyTorch (.pt) → ONNX (.onnx) → OpenVINO IR (.xml/.bin)
                     ↓
              TensorRT (.plan) [for NVIDIA GPUs]
```

### 6.3 Model Registry & Governance

**MLflow**:
- Model versioning
- Experiment tracking
- Model deployment coordination

**Custom Governance Layer**:
- Bias detection metrics
- Fairness constraints
- Explainability reports
- Audit trails for model decisions

---

## 7. Edge Computing Stack

### 7.1 Kubernetes Distribution

**Central/State**: Standard Kubernetes 1.29+
- Full feature set
- Horizontal pod autoscaling
- Advanced networking (Cilium)

**District/Station**: K3s
- Lightweight (< 100MB binary)
- Optimized for edge deployment
- ARM support for low-power devices
- Embedded etcd, SQLite options

### 7.2 Edge Hardware Requirements

**Police Station (Minimum)**:
```
CPU:    4 cores (Intel/AMD x86_64 or ARM64)
RAM:    16 GB
Storage: 500 GB SSD
Network: 1 Gbps Ethernet
UPS:    4-hour backup
```

**District HQ (Minimum)**:
```
CPU:    16 cores
RAM:    64 GB
Storage: 2 TB NVMe SSD + 10 TB HDD
Network: 10 Gbps Ethernet
GPU:    NVIDIA T4 (optional, for AI)
```

### 7.3 Edge Software Stack

```yaml
# Edge Node Components
core_services:
  - postgres:16-alpine          # Local database
  - redis:7-alpine              # Local cache
  - redpanda:latest             # Local message broker
  - minio:latest                # Evidence storage

application_services:
  - npdms/auth:edge             # Auth service (offline capable)
  - npdms/fir:edge              # FIR service
  - npdms/case:edge             # Case service
  - npdms/evidence:edge         # Evidence service
  - npdms/sync:edge             # Sync engine

ai_services:
  - npdms/ocr:edge              # OCR (ONNX inference)
  - npdms/nlp:edge              # NLP (quantized models)

monitoring:
  - prometheus:latest           # Metrics collection
  - grafana:latest              # Dashboards
  - node-exporter:latest        # System metrics
```

---

## 8. Networking & Security

### 8.1 Network Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         NETWORK TOPOLOGY                                     │
└─────────────────────────────────────────────────────────────────────────────┘

CENTRAL (Ministry)
    │
    │ [Dedicated Leased Line / MPLS]
    │ [Backup: Encrypted Internet VPN]
    │
    ├─────────────────────────────────────────────┐
    │                                             │
STATE HQ 1                                   STATE HQ N
    │                                             │
    │ [State WAN / BSNL/NIC Network]              │
    │                                             │
    ├───────────────┬───────────────┐             │
    │               │               │             │
DISTRICT 1    DISTRICT 2      DISTRICT N         ...
    │               │               │
    │ [District WAN / VPN]          │
    │                               │
    ├─────────┬─────────┐           │
    │         │         │           │
STATION 1  STATION 2  STATION N    ...
```

### 8.2 VPN / Mesh Networking

**WireGuard** (Primary):
- Modern, audited cryptography
- Low overhead (< 5% performance impact)
- Simple configuration

**ZeroTier** (Backup/Alternative):
- SDN-based mesh networking
- Works through NAT without port forwarding
- Centralized management

**Configuration Example**:
```ini
# WireGuard config for station
[Interface]
PrivateKey = <station_private_key>
Address = 10.100.{district}.{station}/24
DNS = 10.100.0.1

[Peer]
# District HQ
PublicKey = <district_public_key>
AllowedIPs = 10.100.{district}.0/24, 10.0.0.0/8
Endpoint = district-{id}.npdms.gov.in:51820
PersistentKeepalive = 25
```

### 8.3 Encryption Standards

| Data State | Algorithm | Key Size | Implementation |
|------------|-----------|----------|----------------|
| **At Rest** | AES-256-GCM | 256-bit | PostgreSQL TDE, MinIO encryption |
| **In Transit** | TLS 1.3 | P-384 ECDHE | All HTTP/gRPC connections |
| **Evidence Files** | AES-256-GCM | 256-bit per file | Custom envelope encryption |
| **Backups** | AES-256-GCM | 256-bit | Encrypted before transfer |
| **Key Storage** | HSM-backed | - | HashiCorp Vault + HSM |

### 8.4 Authentication & Authorization

**Authentication Stack**:
```
┌─────────────────────────────────────────────────────────────────────────────┐
│  User → Password + OTP + Biometric → Auth Service → JWT Token               │
│                                            │                                │
│                                            ├── Device Certificate Validation│
│                                            ├── IP/Location Validation       │
│                                            └── Time-based Access Rules      │
└─────────────────────────────────────────────────────────────────────────────┘
```

**JWT Token Structure**:
```json
{
  "sub": "officer-uuid",
  "iss": "npdms-auth",
  "iat": 1704360000,
  "exp": 1704388800,
  "role": "SI",
  "station_id": "station-uuid",
  "district_id": "district-uuid",
  "state_id": "state-uuid",
  "permissions": ["fir:create", "fir:read", "case:read"],
  "device_id": "device-uuid",
  "offline_valid_until": 1704532800
}
```

**Authorization (OPA - Open Policy Agent)**:
```rego
# policy/fir.rego
package npdms.fir

default allow = false

# Constables can create and read FIRs in their station
allow {
    input.action == "create"
    input.user.role == "CONSTABLE"
}

allow {
    input.action == "read"
    input.user.role == "CONSTABLE"
    input.resource.station_id == input.user.station_id
}

# SHO can read and update FIRs in their station
allow {
    input.action in ["read", "update"]
    input.user.role == "SHO"
    input.resource.station_id == input.user.station_id
}

# SP can read FIRs in their district
allow {
    input.action == "read"
    input.user.role == "SP"
    input.resource.district_id == input.user.district_id
}
```

---

## 9. Observability Stack

### 9.1 Metrics (Prometheus)

**Collection**:
- Node Exporter (system metrics)
- PostgreSQL Exporter (database)
- Custom /metrics endpoints (application)

**Key Metrics**:
```yaml
# Application metrics
npdms_fir_created_total
npdms_fir_creation_duration_seconds
npdms_sync_queue_depth
npdms_sync_latency_seconds
npdms_auth_attempts_total
npdms_auth_failures_total

# AI metrics
npdms_ocr_requests_total
npdms_ocr_confidence_score
npdms_nlp_suggestions_total
npdms_ai_latency_seconds
```

### 9.2 Logging (Loki)

**Log Format**:
```json
{
  "timestamp": "2026-01-04T14:30:00Z",
  "level": "INFO",
  "service": "fir-service",
  "trace_id": "abc123",
  "span_id": "def456",
  "user_id": "officer-uuid",
  "station_id": "station-uuid",
  "action": "FIR_CREATED",
  "fir_id": "fir-uuid",
  "message": "FIR created successfully"
}
```

**Log Retention**:
- Hot: 7 days (Loki)
- Warm: 90 days (S3/MinIO)
- Cold: 20 years (Archive)

### 9.3 Tracing (Tempo + OpenTelemetry)

**Trace Propagation**:
```go
// Automatic trace context propagation
func CreateFIR(ctx context.Context, req *CreateFIRRequest) (*FIR, error) {
    ctx, span := tracer.Start(ctx, "CreateFIR")
    defer span.End()

    span.SetAttributes(
        attribute.String("station_id", req.StationID),
        attribute.String("user_id", getCurrentUser(ctx).ID),
    )

    // Database operation (automatically traced)
    fir, err := repo.Create(ctx, req)
    if err != nil {
        span.RecordError(err)
        return nil, err
    }

    span.SetAttributes(attribute.String("fir_id", fir.ID))
    return fir, nil
}
```

### 9.4 Dashboards (Grafana)

**Pre-built Dashboards**:
1. **System Health**: CPU, memory, disk, network
2. **Service Health**: Request rates, latencies, errors
3. **Sync Status**: Queue depth, sync latency, conflicts
4. **AI Performance**: Inference latency, confidence scores
5. **Security**: Auth attempts, failures, anomalies
6. **Business Metrics**: FIRs/day, case clearance, response times

---

## 10. Technology Version Matrix

### 10.1 Production Versions

| Component | Version | EOL Date | Upgrade Path |
|-----------|---------|----------|--------------|
| Go | 1.22.x | Feb 2025 | Quarterly updates |
| Rust | 1.75.x | Stable | Every 6 weeks |
| Python | 3.11.x | Oct 2027 | Annual review |
| Node.js | 20 LTS | Apr 2026 | LTS track |
| PostgreSQL | 16.x | Nov 2028 | Major annually |
| Kubernetes | 1.29.x | Feb 2025 | Quarterly updates |
| Next.js | 14.x | - | Minor updates |
| React | 18.x | - | Following Next.js |

### 10.2 Dependency Update Policy

1. **Security patches**: Within 48 hours
2. **Bug fixes**: Within 2 weeks
3. **Minor versions**: Monthly review
4. **Major versions**: Quarterly evaluation, staged rollout

---

## Appendix A: Alternative Technologies Considered

| Category | Selected | Alternatives Considered | Reason for Selection |
|----------|----------|------------------------|---------------------|
| Backend | Go | Java, .NET, Node.js | Simplest deployment, best concurrency |
| Crypto | Rust | C, Go | Memory safety without GC pauses |
| AI | Python | Julia, Rust | Ecosystem maturity, model availability |
| Frontend | Next.js | Angular, Vue | React ecosystem, SSR support |
| SQL DB | PostgreSQL | MySQL, MariaDB | JSON support, extensions, replication |
| NoSQL | ScyllaDB | Cassandra, MongoDB | Performance, compatibility |
| Graph | Neo4j | TigerGraph, JanusGraph | Maturity, query language, GDS |
| Search | OpenSearch | Elasticsearch | License, community |
| Messaging | Redpanda/Kafka | RabbitMQ, NATS | Durability, ordering, replay |
| K8s | K3s (edge) | MicroK8s, K0s | Lightest footprint, full API |

---

## Appendix B: License Compliance

All selected technologies are either:
- Open Source (Apache 2.0, MIT, BSD, MPL 2.0)
- Government-approved commercial licenses

| Technology | License | Commercial Use | Government Compliant |
|------------|---------|----------------|---------------------|
| Go | BSD-3 | Yes | Yes |
| Rust | MIT/Apache 2.0 | Yes | Yes |
| Python | PSF | Yes | Yes |
| PostgreSQL | PostgreSQL License | Yes | Yes |
| Neo4j Community | GPL v3 | Yes (AGPL exception) | Yes |
| OpenSearch | Apache 2.0 | Yes | Yes |
| Kafka | Apache 2.0 | Yes | Yes |
| Kubernetes | Apache 2.0 | Yes | Yes |
