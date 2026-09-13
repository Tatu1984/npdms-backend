# PHASE 3 — PROJECT STRUCTURE
# National Police Department Management System (NPDMS)

## Document Control
- **Version**: 1.0
- **Classification**: RESTRICTED
- **Author**: Engineering Architecture Team
- **Last Updated**: 2026-01-04

---

## 1. Complete Directory Tree

```
/Users/sudipto/Desktop/projects/npdms/
├── README.md
├── LICENSE
├── .gitignore
├── .editorconfig
├── Makefile
├── docker-compose.yml
├── docker-compose.dev.yml
├── docker-compose.test.yml
│
├── docs/                                    # Documentation
│   ├── architecture/
│   │   ├── PHASE_0_SYSTEM_BLUEPRINT.md
│   │   ├── PHASE_1_UI_UX_DESIGN.md
│   │   ├── PHASE_2_DEVELOPMENT_PLAN.md
│   │   ├── PHASE_3_PROJECT_STRUCTURE.md
│   │   ├── PHASE_4_TECH_STACK.md
│   │   ├── PHASE_5_AI_IMPLEMENTATION.md
│   │   └── PHASE_6_DEVSECOPS.md
│   ├── api/
│   │   ├── openapi/
│   │   │   ├── fir-service.yaml
│   │   │   ├── case-service.yaml
│   │   │   ├── evidence-service.yaml
│   │   │   ├── personnel-service.yaml
│   │   │   └── sync-service.yaml
│   │   └── grpc/
│   │       └── protos/                      # gRPC proto definitions
│   ├── runbooks/
│   │   ├── deployment.md
│   │   ├── disaster-recovery.md
│   │   ├── incident-response.md
│   │   └── security-incident.md
│   └── training/
│       ├── constable-guide.md
│       ├── sho-guide.md
│       └── admin-guide.md
│
├── services/                                # Backend Microservices
│   │
│   ├── gateway/                             # API Gateway (Go)
│   │   ├── cmd/
│   │   │   └── gateway/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   │   └── config.go
│   │   │   ├── middleware/
│   │   │   │   ├── auth.go
│   │   │   │   ├── ratelimit.go
│   │   │   │   ├── audit.go
│   │   │   │   └── cors.go
│   │   │   ├── handlers/
│   │   │   │   └── proxy.go
│   │   │   └── routes/
│   │   │       └── routes.go
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── auth/                                # Authentication Service (Go)
│   │   ├── cmd/
│   │   │   └── auth/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   │   └── config.go
│   │   │   ├── domain/
│   │   │   │   ├── user.go
│   │   │   │   ├── role.go
│   │   │   │   ├── session.go
│   │   │   │   └── token.go
│   │   │   ├── repository/
│   │   │   │   ├── user_repo.go
│   │   │   │   ├── session_repo.go
│   │   │   │   └── postgres/
│   │   │   │       └── user_postgres.go
│   │   │   ├── service/
│   │   │   │   ├── auth_service.go
│   │   │   │   ├── token_service.go
│   │   │   │   └── biometric_service.go
│   │   │   ├── handlers/
│   │   │   │   ├── login.go
│   │   │   │   ├── logout.go
│   │   │   │   ├── refresh.go
│   │   │   │   └── verify.go
│   │   │   └── grpc/
│   │   │       └── auth_grpc.go
│   │   ├── migrations/
│   │   │   ├── 001_create_users.up.sql
│   │   │   ├── 001_create_users.down.sql
│   │   │   ├── 002_create_sessions.up.sql
│   │   │   └── 002_create_sessions.down.sql
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── fir/                                 # FIR Service (Go)
│   │   ├── cmd/
│   │   │   └── fir/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/
│   │   │   │   ├── fir.go
│   │   │   │   ├── complainant.go
│   │   │   │   ├── accused.go
│   │   │   │   ├── property.go
│   │   │   │   └── events.go
│   │   │   ├── repository/
│   │   │   │   ├── fir_repo.go
│   │   │   │   └── postgres/
│   │   │   │       └── fir_postgres.go
│   │   │   ├── service/
│   │   │   │   ├── fir_service.go
│   │   │   │   ├── search_service.go
│   │   │   │   └── validation.go
│   │   │   ├── handlers/
│   │   │   │   ├── create_fir.go
│   │   │   │   ├── update_fir.go
│   │   │   │   ├── get_fir.go
│   │   │   │   ├── list_firs.go
│   │   │   │   └── search_firs.go
│   │   │   └── grpc/
│   │   │       └── fir_grpc.go
│   │   ├── migrations/
│   │   │   ├── 001_create_firs.up.sql
│   │   │   └── 001_create_firs.down.sql
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── case/                                # Case Management Service (Go)
│   │   ├── cmd/
│   │   │   └── case/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/
│   │   │   │   ├── case.go
│   │   │   │   ├── diary.go
│   │   │   │   ├── person.go
│   │   │   │   └── timeline.go
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   └── handlers/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── evidence/                            # Evidence Service (Go)
│   │   ├── cmd/
│   │   │   └── evidence/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/
│   │   │   │   ├── evidence.go
│   │   │   │   ├── custody.go
│   │   │   │   └── forensic.go
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   │   ├── evidence_service.go
│   │   │   │   ├── custody_service.go
│   │   │   │   ├── storage_service.go
│   │   │   │   └── integrity_service.go
│   │   │   └── handlers/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── personnel/                           # Personnel Service (Go)
│   │   ├── cmd/
│   │   │   └── personnel/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/
│   │   │   │   ├── officer.go
│   │   │   │   ├── duty.go
│   │   │   │   ├── attendance.go
│   │   │   │   └── leave.go
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   └── handlers/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── armoury/                             # Armoury Service (Go)
│   │   ├── cmd/
│   │   │   └── armoury/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── domain/
│   │   │   │   ├── weapon.go
│   │   │   │   ├── ammunition.go
│   │   │   │   └── issuance.go
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   └── handlers/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── vehicle/                             # Vehicle Service (Go)
│   │   ├── cmd/
│   │   │   └── vehicle/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── domain/
│   │   │   │   ├── vehicle.go
│   │   │   │   ├── trip.go
│   │   │   │   └── maintenance.go
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   └── handlers/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── sync/                                # Sync Engine Service (Go)
│   │   ├── cmd/
│   │   │   └── sync/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── config/
│   │   │   ├── domain/
│   │   │   │   ├── event.go
│   │   │   │   ├── conflict.go
│   │   │   │   └── outbox.go
│   │   │   ├── engine/
│   │   │   │   ├── sync_engine.go
│   │   │   │   ├── conflict_resolver.go
│   │   │   │   ├── publisher.go
│   │   │   │   └── consumer.go
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   └── handlers/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── audit/                               # Audit Service (Go)
│   │   ├── cmd/
│   │   │   └── audit/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── domain/
│   │   │   │   ├── audit_log.go
│   │   │   │   └── compliance.go
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   └── handlers/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── alert/                               # Alert Service (Go)
│   │   ├── cmd/
│   │   │   └── alert/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── domain/
│   │   │   │   ├── alert.go
│   │   │   │   └── broadcast.go
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   └── handlers/
│   │   ├── migrations/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   ├── analytics/                           # Analytics Service (Go)
│   │   ├── cmd/
│   │   │   └── analytics/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── domain/
│   │   │   ├── aggregator/
│   │   │   ├── service/
│   │   │   └── handlers/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   │
│   └── crypto/                              # Cryptography Service (Rust)
│       ├── src/
│       │   ├── main.rs
│       │   ├── lib.rs
│       │   ├── encryption/
│       │   │   ├── mod.rs
│       │   │   ├── aes.rs
│       │   │   └── key_derivation.rs
│       │   ├── signing/
│       │   │   ├── mod.rs
│       │   │   ├── ed25519.rs
│       │   │   └── hsm.rs
│       │   ├── hashing/
│       │   │   ├── mod.rs
│       │   │   └── sha256.rs
│       │   └── api/
│       │       ├── mod.rs
│       │       └── grpc.rs
│       ├── Cargo.toml
│       ├── Cargo.lock
│       └── Dockerfile
│
├── ai/                                      # AI/ML Services
│   │
│   ├── ocr/                                 # OCR Service (Python)
│   │   ├── src/
│   │   │   ├── __init__.py
│   │   │   ├── main.py
│   │   │   ├── config.py
│   │   │   ├── models/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── trocr.py
│   │   │   │   └── language_models.py
│   │   │   ├── preprocessing/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── image.py
│   │   │   │   └── binarization.py
│   │   │   ├── postprocessing/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── correction.py
│   │   │   │   └── legal_dictionary.py
│   │   │   ├── api/
│   │   │   │   ├── __init__.py
│   │   │   │   └── endpoints.py
│   │   │   └── utils/
│   │   │       └── logging.py
│   │   ├── models/                          # Trained model files
│   │   │   └── .gitkeep
│   │   ├── tests/
│   │   │   ├── __init__.py
│   │   │   ├── test_ocr.py
│   │   │   └── fixtures/
│   │   ├── requirements.txt
│   │   ├── Dockerfile
│   │   └── pyproject.toml
│   │
│   ├── nlp/                                 # NLP Service (Python)
│   │   ├── src/
│   │   │   ├── __init__.py
│   │   │   ├── main.py
│   │   │   ├── config.py
│   │   │   ├── models/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── entity_extractor.py
│   │   │   │   ├── ipc_classifier.py
│   │   │   │   ├── similarity.py
│   │   │   │   └── translation.py
│   │   │   ├── processing/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── fir_parser.py
│   │   │   │   └── text_normalizer.py
│   │   │   ├── api/
│   │   │   │   ├── __init__.py
│   │   │   │   └── endpoints.py
│   │   │   └── utils/
│   │   ├── models/
│   │   │   └── .gitkeep
│   │   ├── data/
│   │   │   ├── ipc_sections.json
│   │   │   ├── legal_terms.json
│   │   │   └── stopwords/
│   │   ├── tests/
│   │   ├── requirements.txt
│   │   ├── Dockerfile
│   │   └── pyproject.toml
│   │
│   ├── vision/                              # Computer Vision Service (Python)
│   │   ├── src/
│   │   │   ├── __init__.py
│   │   │   ├── main.py
│   │   │   ├── models/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── object_detector.py
│   │   │   │   ├── weapon_classifier.py
│   │   │   │   └── face_detector.py
│   │   │   ├── video/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── key_frame.py
│   │   │   │   └── summarizer.py
│   │   │   └── api/
│   │   ├── models/
│   │   ├── tests/
│   │   ├── requirements.txt
│   │   ├── Dockerfile
│   │   └── pyproject.toml
│   │
│   ├── graph/                               # Graph Intelligence (Python)
│   │   ├── src/
│   │   │   ├── __init__.py
│   │   │   ├── main.py
│   │   │   ├── neo4j_client.py
│   │   │   ├── analysis/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── network.py
│   │   │   │   ├── community.py
│   │   │   │   └── centrality.py
│   │   │   └── api/
│   │   ├── tests/
│   │   ├── requirements.txt
│   │   ├── Dockerfile
│   │   └── pyproject.toml
│   │
│   ├── analytics/                           # Predictive Analytics (Python)
│   │   ├── src/
│   │   │   ├── __init__.py
│   │   │   ├── main.py
│   │   │   ├── models/
│   │   │   │   ├── __init__.py
│   │   │   │   ├── crime_predictor.py
│   │   │   │   ├── patrol_optimizer.py
│   │   │   │   └── trend_analyzer.py
│   │   │   └── api/
│   │   ├── tests/
│   │   ├── requirements.txt
│   │   ├── Dockerfile
│   │   └── pyproject.toml
│   │
│   ├── training/                            # Model Training Infrastructure
│   │   ├── pipelines/
│   │   │   ├── ocr_training.py
│   │   │   ├── nlp_training.py
│   │   │   └── vision_training.py
│   │   ├── configs/
│   │   │   ├── ocr_config.yaml
│   │   │   ├── nlp_config.yaml
│   │   │   └── vision_config.yaml
│   │   ├── scripts/
│   │   │   ├── prepare_data.py
│   │   │   ├── evaluate.py
│   │   │   └── export_onnx.py
│   │   ├── requirements.txt
│   │   └── Dockerfile
│   │
│   └── governance/                          # AI Governance
│       ├── src/
│       │   ├── __init__.py
│       │   ├── bias_detector.py
│       │   ├── model_registry.py
│       │   ├── explainability.py
│       │   └── audit.py
│       ├── tests/
│       ├── requirements.txt
│       └── Dockerfile
│
├── ui/                                      # Frontend Applications
│   │
│   ├── web/                                 # Main Web Application (Next.js)
│   │   ├── src/
│   │   │   ├── app/                         # Next.js App Router
│   │   │   │   ├── (auth)/
│   │   │   │   │   ├── login/
│   │   │   │   │   │   └── page.tsx
│   │   │   │   │   └── layout.tsx
│   │   │   │   ├── (dashboard)/
│   │   │   │   │   ├── layout.tsx
│   │   │   │   │   ├── page.tsx
│   │   │   │   │   ├── fir/
│   │   │   │   │   │   ├── page.tsx
│   │   │   │   │   │   ├── new/
│   │   │   │   │   │   │   └── page.tsx
│   │   │   │   │   │   └── [id]/
│   │   │   │   │   │       └── page.tsx
│   │   │   │   │   ├── cases/
│   │   │   │   │   │   ├── page.tsx
│   │   │   │   │   │   └── [id]/
│   │   │   │   │   │       └── page.tsx
│   │   │   │   │   ├── evidence/
│   │   │   │   │   ├── personnel/
│   │   │   │   │   ├── armoury/
│   │   │   │   │   ├── vehicles/
│   │   │   │   │   ├── intelligence/
│   │   │   │   │   ├── analytics/
│   │   │   │   │   └── settings/
│   │   │   │   ├── api/
│   │   │   │   │   └── [...path]/
│   │   │   │   │       └── route.ts
│   │   │   │   └── layout.tsx
│   │   │   │
│   │   │   ├── components/
│   │   │   │   ├── ui/
│   │   │   │   │   ├── Button.tsx
│   │   │   │   │   ├── Input.tsx
│   │   │   │   │   ├── Select.tsx
│   │   │   │   │   ├── Modal.tsx
│   │   │   │   │   ├── Table.tsx
│   │   │   │   │   ├── Card.tsx
│   │   │   │   │   ├── Badge.tsx
│   │   │   │   │   ├── Alert.tsx
│   │   │   │   │   ├── Toast.tsx
│   │   │   │   │   ├── Tabs.tsx
│   │   │   │   │   └── index.ts
│   │   │   │   ├── layout/
│   │   │   │   │   ├── Header.tsx
│   │   │   │   │   ├── Sidebar.tsx
│   │   │   │   │   ├── Footer.tsx
│   │   │   │   │   └── SyncStatusBar.tsx
│   │   │   │   ├── domain/
│   │   │   │   │   ├── fir/
│   │   │   │   │   │   ├── FIRForm.tsx
│   │   │   │   │   │   ├── FIRList.tsx
│   │   │   │   │   │   ├── FIRCard.tsx
│   │   │   │   │   │   ├── VoiceInput.tsx
│   │   │   │   │   │   └── HandwritingScanner.tsx
│   │   │   │   │   ├── cases/
│   │   │   │   │   ├── evidence/
│   │   │   │   │   ├── personnel/
│   │   │   │   │   ├── armoury/
│   │   │   │   │   ├── vehicles/
│   │   │   │   │   ├── intelligence/
│   │   │   │   │   └── command/
│   │   │   │   └── shared/
│   │   │   │       ├── BiometricAuth.tsx
│   │   │   │       ├── AuditTrail.tsx
│   │   │   │       ├── OfflineIndicator.tsx
│   │   │   │       ├── RoleGuard.tsx
│   │   │   │       └── ErrorBoundary.tsx
│   │   │   │
│   │   │   ├── hooks/
│   │   │   │   ├── useAuth.ts
│   │   │   │   ├── useOffline.ts
│   │   │   │   ├── useSync.ts
│   │   │   │   ├── useBiometric.ts
│   │   │   │   └── useRole.ts
│   │   │   │
│   │   │   ├── stores/
│   │   │   │   ├── authStore.ts
│   │   │   │   ├── syncStore.ts
│   │   │   │   ├── firStore.ts
│   │   │   │   ├── alertStore.ts
│   │   │   │   └── uiStore.ts
│   │   │   │
│   │   │   ├── lib/
│   │   │   │   ├── api/
│   │   │   │   │   ├── client.ts
│   │   │   │   │   ├── offline.ts
│   │   │   │   │   └── sync.ts
│   │   │   │   ├── db/
│   │   │   │   │   └── indexedDB.ts
│   │   │   │   ├── crypto/
│   │   │   │   │   └── encrypt.ts
│   │   │   │   └── utils/
│   │   │   │       ├── date.ts
│   │   │   │       ├── format.ts
│   │   │   │       └── validation.ts
│   │   │   │
│   │   │   ├── types/
│   │   │   │   ├── fir.ts
│   │   │   │   ├── case.ts
│   │   │   │   ├── evidence.ts
│   │   │   │   ├── personnel.ts
│   │   │   │   └── api.ts
│   │   │   │
│   │   │   └── styles/
│   │   │       ├── globals.css
│   │   │       └── themes/
│   │   │           ├── dark.css
│   │   │           └── high-contrast.css
│   │   │
│   │   ├── public/
│   │   │   ├── icons/
│   │   │   ├── images/
│   │   │   └── manifest.json
│   │   ├── tests/
│   │   │   ├── e2e/
│   │   │   └── unit/
│   │   ├── next.config.js
│   │   ├── tailwind.config.js
│   │   ├── tsconfig.json
│   │   ├── package.json
│   │   ├── package-lock.json
│   │   └── Dockerfile
│   │
│   ├── mobile/                              # Mobile PWA (Future)
│   │   └── .gitkeep
│   │
│   └── shared/                              # Shared UI Packages
│       ├── design-tokens/
│       │   ├── colors.ts
│       │   ├── spacing.ts
│       │   ├── typography.ts
│       │   └── index.ts
│       └── package.json
│
├── infra/                                   # Infrastructure as Code
│   │
│   ├── terraform/                           # Terraform Modules
│   │   ├── modules/
│   │   │   ├── kubernetes/
│   │   │   │   ├── main.tf
│   │   │   │   ├── variables.tf
│   │   │   │   └── outputs.tf
│   │   │   ├── database/
│   │   │   │   ├── main.tf
│   │   │   │   ├── variables.tf
│   │   │   │   └── outputs.tf
│   │   │   ├── networking/
│   │   │   ├── storage/
│   │   │   └── security/
│   │   ├── environments/
│   │   │   ├── dev/
│   │   │   │   ├── main.tf
│   │   │   │   ├── variables.tf
│   │   │   │   └── terraform.tfvars
│   │   │   ├── staging/
│   │   │   └── prod/
│   │   └── .terraform-version
│   │
│   ├── kubernetes/                          # Kubernetes Manifests
│   │   ├── base/
│   │   │   ├── namespace.yaml
│   │   │   ├── configmaps/
│   │   │   ├── secrets/
│   │   │   ├── services/
│   │   │   └── deployments/
│   │   ├── overlays/
│   │   │   ├── dev/
│   │   │   ├── staging/
│   │   │   └── prod/
│   │   └── kustomization.yaml
│   │
│   ├── helm/                                # Helm Charts
│   │   ├── npdms/
│   │   │   ├── Chart.yaml
│   │   │   ├── values.yaml
│   │   │   ├── values-dev.yaml
│   │   │   ├── values-staging.yaml
│   │   │   ├── values-prod.yaml
│   │   │   └── templates/
│   │   │       ├── deployment.yaml
│   │   │       ├── service.yaml
│   │   │       ├── ingress.yaml
│   │   │       ├── configmap.yaml
│   │   │       ├── secret.yaml
│   │   │       └── hpa.yaml
│   │   └── edge/
│   │       ├── Chart.yaml
│   │       ├── values.yaml
│   │       └── templates/
│   │
│   ├── ansible/                             # Ansible Playbooks
│   │   ├── inventory/
│   │   │   ├── dev/
│   │   │   ├── staging/
│   │   │   └── prod/
│   │   ├── playbooks/
│   │   │   ├── deploy.yaml
│   │   │   ├── rollback.yaml
│   │   │   ├── backup.yaml
│   │   │   └── security-hardening.yaml
│   │   ├── roles/
│   │   │   ├── common/
│   │   │   ├── docker/
│   │   │   ├── kubernetes/
│   │   │   └── monitoring/
│   │   └── ansible.cfg
│   │
│   └── docker/                              # Docker Configurations
│       ├── base-images/
│       │   ├── go-base/
│       │   │   └── Dockerfile
│       │   ├── python-base/
│       │   │   └── Dockerfile
│       │   └── node-base/
│       │       └── Dockerfile
│       └── registry/
│           └── config.yaml
│
├── edge/                                    # Edge Deployment Package
│   │
│   ├── station/                             # Police Station Edge Node
│   │   ├── compose/
│   │   │   └── docker-compose.yaml
│   │   ├── k3s/
│   │   │   └── manifests/
│   │   ├── scripts/
│   │   │   ├── install.sh
│   │   │   ├── upgrade.sh
│   │   │   ├── backup.sh
│   │   │   └── health-check.sh
│   │   └── config/
│   │       └── edge-config.yaml
│   │
│   ├── district/                            # District Edge Node
│   │   ├── compose/
│   │   ├── k3s/
│   │   ├── scripts/
│   │   └── config/
│   │
│   └── ai-models/                           # Edge AI Models (ONNX)
│       ├── ocr/
│       │   └── .gitkeep
│       ├── nlp/
│       │   └── .gitkeep
│       └── vision/
│           └── .gitkeep
│
├── scripts/                                 # Development Scripts
│   ├── setup/
│   │   ├── install-deps.sh
│   │   ├── setup-dev.sh
│   │   └── setup-test-data.sh
│   ├── build/
│   │   ├── build-all.sh
│   │   ├── build-services.sh
│   │   └── build-ui.sh
│   ├── deploy/
│   │   ├── deploy-dev.sh
│   │   ├── deploy-staging.sh
│   │   └── deploy-prod.sh
│   ├── db/
│   │   ├── migrate.sh
│   │   ├── seed.sh
│   │   └── backup.sh
│   └── test/
│       ├── run-unit-tests.sh
│       ├── run-integration-tests.sh
│       └── run-e2e-tests.sh
│
├── tests/                                   # Integration & E2E Tests
│   ├── integration/
│   │   ├── fir_flow_test.go
│   │   ├── sync_test.go
│   │   └── auth_test.go
│   ├── e2e/
│   │   ├── playwright.config.ts
│   │   ├── fir-creation.spec.ts
│   │   └── case-management.spec.ts
│   ├── load/
│   │   ├── k6/
│   │   │   ├── fir-load.js
│   │   │   └── search-load.js
│   │   └── locust/
│   │       └── locustfile.py
│   └── security/
│       ├── zap-config.yaml
│       └── nuclei-templates/
│
├── proto/                                   # Protocol Buffer Definitions
│   ├── npdms/
│   │   ├── v1/
│   │   │   ├── fir.proto
│   │   │   ├── case.proto
│   │   │   ├── evidence.proto
│   │   │   ├── personnel.proto
│   │   │   ├── sync.proto
│   │   │   ├── alert.proto
│   │   │   └── common.proto
│   │   └── buf.yaml
│   └── buf.gen.yaml
│
├── configs/                                 # Shared Configurations
│   ├── dev/
│   │   ├── services.yaml
│   │   └── database.yaml
│   ├── staging/
│   └── prod/
│
└── .github/                                 # GitHub Workflows
    ├── workflows/
    │   ├── ci.yaml
    │   ├── cd-dev.yaml
    │   ├── cd-staging.yaml
    │   ├── cd-prod.yaml
    │   ├── security-scan.yaml
    │   └── dependency-update.yaml
    ├── CODEOWNERS
    └── pull_request_template.md
```

---

## 2. Folder Purposes

### 2.1 Services (Backend)

| Folder | Purpose | Language |
|--------|---------|----------|
| `services/gateway` | API Gateway - routing, rate limiting, auth validation | Go |
| `services/auth` | Authentication, authorization, session management | Go |
| `services/fir` | FIR CRUD, search, validation | Go |
| `services/case` | Case management, diary, timeline | Go |
| `services/evidence` | Evidence registry, chain of custody | Go |
| `services/personnel` | Officer records, duty roster, attendance | Go |
| `services/armoury` | Weapon registry, ammunition, issuance | Go |
| `services/vehicle` | Fleet management, tracking, allocation | Go |
| `services/sync` | Sync engine, conflict resolution, event publishing | Go |
| `services/audit` | Audit logging, compliance reporting | Go |
| `services/alert` | Alert broadcasting, acknowledgment tracking | Go |
| `services/analytics` | Data aggregation, statistics computation | Go |
| `services/crypto` | Cryptographic operations, HSM integration | Rust |

### 2.2 AI Services

| Folder | Purpose | Language |
|--------|---------|----------|
| `ai/ocr` | Handwritten text extraction | Python |
| `ai/nlp` | Entity extraction, IPC suggestion, similarity | Python |
| `ai/vision` | Image/video analysis, object detection | Python |
| `ai/graph` | Criminal network analysis, community detection | Python |
| `ai/analytics` | Crime prediction, patrol optimization | Python |
| `ai/training` | Model training pipelines | Python |
| `ai/governance` | Bias detection, model registry, explainability | Python |

### 2.3 UI Applications

| Folder | Purpose | Technology |
|--------|---------|------------|
| `ui/web` | Main web application | Next.js 14, React, TypeScript |
| `ui/mobile` | Future mobile PWA | Reserved |
| `ui/shared` | Shared design tokens and utilities | TypeScript |

### 2.4 Infrastructure

| Folder | Purpose |
|--------|---------|
| `infra/terraform` | Cloud infrastructure provisioning |
| `infra/kubernetes` | K8s manifests with Kustomize |
| `infra/helm` | Helm charts for deployment |
| `infra/ansible` | Configuration management playbooks |
| `infra/docker` | Base images and registry config |

### 2.5 Edge Deployment

| Folder | Purpose |
|--------|---------|
| `edge/station` | Police station edge node package |
| `edge/district` | District HQ edge node package |
| `edge/ai-models` | ONNX models for edge inference |

---

## 3. Shell Commands to Initialize Project

```bash
#!/bin/bash
# File: scripts/setup/init-project.sh
# Run from: /Users/sudipto/Desktop/projects/npdms

set -e

PROJECT_ROOT="/Users/sudipto/Desktop/projects/npdms"

echo "=== NPDMS Project Initialization ==="
echo "Project Root: $PROJECT_ROOT"

# Create root files
cat > "$PROJECT_ROOT/README.md" << 'EOF'
# National Police Department Management System (NPDMS)

## Overview
NPDMS is a federated, edge-first police management system designed for nationwide deployment.

## Quick Start
```bash
make setup-dev
make start-dev
```

## Documentation
See `docs/` for full documentation.

## License
RESTRICTED - Government of India
EOF

cat > "$PROJECT_ROOT/.gitignore" << 'EOF'
# Dependencies
node_modules/
vendor/
__pycache__/
*.pyc
.venv/
venv/

# Build
dist/
build/
*.exe
*.dll
*.so
*.dylib
bin/

# IDE
.idea/
.vscode/
*.swp
*.swo

# Environment
.env
.env.local
.env.*.local
*.env

# Logs
*.log
logs/

# OS
.DS_Store
Thumbs.db

# Test
coverage/
.coverage
htmlcov/

# Terraform
.terraform/
*.tfstate
*.tfstate.*
.terraform.lock.hcl

# Models (large files)
*.onnx
*.pt
*.pth
*.h5

# Secrets
secrets/
*.pem
*.key
credentials.json
EOF

cat > "$PROJECT_ROOT/.editorconfig" << 'EOF'
root = true

[*]
indent_style = space
indent_size = 2
end_of_line = lf
charset = utf-8
trim_trailing_whitespace = true
insert_final_newline = true

[*.go]
indent_style = tab
indent_size = 4

[*.py]
indent_size = 4

[Makefile]
indent_style = tab
EOF

cat > "$PROJECT_ROOT/Makefile" << 'EOF'
.PHONY: help setup-dev start-dev stop-dev test build

help:
	@echo "NPDMS Development Commands"
	@echo "=========================="
	@echo "make setup-dev    - Setup development environment"
	@echo "make start-dev    - Start all services in development mode"
	@echo "make stop-dev     - Stop all development services"
	@echo "make test         - Run all tests"
	@echo "make build        - Build all services"
	@echo "make lint         - Run linters"

setup-dev:
	./scripts/setup/setup-dev.sh

start-dev:
	docker-compose -f docker-compose.dev.yml up -d

stop-dev:
	docker-compose -f docker-compose.dev.yml down

test:
	./scripts/test/run-unit-tests.sh

build:
	./scripts/build/build-all.sh

lint:
	@echo "Running linters..."
	cd services && golangci-lint run ./...
	cd ai && ruff check .
	cd ui/web && npm run lint
EOF

# Create directory structure
echo "Creating directory structure..."

# Documentation
mkdir -p "$PROJECT_ROOT/docs/architecture"
mkdir -p "$PROJECT_ROOT/docs/api/openapi"
mkdir -p "$PROJECT_ROOT/docs/api/grpc/protos"
mkdir -p "$PROJECT_ROOT/docs/runbooks"
mkdir -p "$PROJECT_ROOT/docs/training"

# Backend Services
for service in gateway auth fir case evidence personnel armoury vehicle sync audit alert analytics; do
    mkdir -p "$PROJECT_ROOT/services/$service/cmd/$service"
    mkdir -p "$PROJECT_ROOT/services/$service/internal/config"
    mkdir -p "$PROJECT_ROOT/services/$service/internal/domain"
    mkdir -p "$PROJECT_ROOT/services/$service/internal/repository/postgres"
    mkdir -p "$PROJECT_ROOT/services/$service/internal/service"
    mkdir -p "$PROJECT_ROOT/services/$service/internal/handlers"
    mkdir -p "$PROJECT_ROOT/services/$service/internal/grpc"
    mkdir -p "$PROJECT_ROOT/services/$service/migrations"
    touch "$PROJECT_ROOT/services/$service/Dockerfile"
    touch "$PROJECT_ROOT/services/$service/go.mod"
done

# Crypto service (Rust)
mkdir -p "$PROJECT_ROOT/services/crypto/src/encryption"
mkdir -p "$PROJECT_ROOT/services/crypto/src/signing"
mkdir -p "$PROJECT_ROOT/services/crypto/src/hashing"
mkdir -p "$PROJECT_ROOT/services/crypto/src/api"
touch "$PROJECT_ROOT/services/crypto/Cargo.toml"
touch "$PROJECT_ROOT/services/crypto/Dockerfile"

# AI Services
for ai_service in ocr nlp vision graph analytics; do
    mkdir -p "$PROJECT_ROOT/ai/$ai_service/src/models"
    mkdir -p "$PROJECT_ROOT/ai/$ai_service/src/api"
    mkdir -p "$PROJECT_ROOT/ai/$ai_service/models"
    mkdir -p "$PROJECT_ROOT/ai/$ai_service/tests"
    touch "$PROJECT_ROOT/ai/$ai_service/requirements.txt"
    touch "$PROJECT_ROOT/ai/$ai_service/Dockerfile"
    touch "$PROJECT_ROOT/ai/$ai_service/pyproject.toml"
done

# AI Training and Governance
mkdir -p "$PROJECT_ROOT/ai/training/pipelines"
mkdir -p "$PROJECT_ROOT/ai/training/configs"
mkdir -p "$PROJECT_ROOT/ai/training/scripts"
mkdir -p "$PROJECT_ROOT/ai/governance/src"
mkdir -p "$PROJECT_ROOT/ai/governance/tests"

# UI Web Application
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(auth)/login"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/fir/new"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/fir/[id]"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/cases/[id]"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/evidence"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/personnel"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/armoury"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/vehicles"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/intelligence"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/analytics"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/(dashboard)/settings"
mkdir -p "$PROJECT_ROOT/ui/web/src/app/api/[...path]"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/ui"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/layout"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/domain/fir"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/domain/cases"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/domain/evidence"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/domain/personnel"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/domain/armoury"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/domain/vehicles"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/domain/intelligence"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/domain/command"
mkdir -p "$PROJECT_ROOT/ui/web/src/components/shared"
mkdir -p "$PROJECT_ROOT/ui/web/src/hooks"
mkdir -p "$PROJECT_ROOT/ui/web/src/stores"
mkdir -p "$PROJECT_ROOT/ui/web/src/lib/api"
mkdir -p "$PROJECT_ROOT/ui/web/src/lib/db"
mkdir -p "$PROJECT_ROOT/ui/web/src/lib/crypto"
mkdir -p "$PROJECT_ROOT/ui/web/src/lib/utils"
mkdir -p "$PROJECT_ROOT/ui/web/src/types"
mkdir -p "$PROJECT_ROOT/ui/web/src/styles/themes"
mkdir -p "$PROJECT_ROOT/ui/web/public/icons"
mkdir -p "$PROJECT_ROOT/ui/web/public/images"
mkdir -p "$PROJECT_ROOT/ui/web/tests/e2e"
mkdir -p "$PROJECT_ROOT/ui/web/tests/unit"
touch "$PROJECT_ROOT/ui/web/package.json"
touch "$PROJECT_ROOT/ui/web/tsconfig.json"
touch "$PROJECT_ROOT/ui/web/next.config.js"
touch "$PROJECT_ROOT/ui/web/tailwind.config.js"
touch "$PROJECT_ROOT/ui/web/Dockerfile"

# UI Shared
mkdir -p "$PROJECT_ROOT/ui/shared/design-tokens"
mkdir -p "$PROJECT_ROOT/ui/mobile"

# Infrastructure
mkdir -p "$PROJECT_ROOT/infra/terraform/modules/kubernetes"
mkdir -p "$PROJECT_ROOT/infra/terraform/modules/database"
mkdir -p "$PROJECT_ROOT/infra/terraform/modules/networking"
mkdir -p "$PROJECT_ROOT/infra/terraform/modules/storage"
mkdir -p "$PROJECT_ROOT/infra/terraform/modules/security"
mkdir -p "$PROJECT_ROOT/infra/terraform/environments/dev"
mkdir -p "$PROJECT_ROOT/infra/terraform/environments/staging"
mkdir -p "$PROJECT_ROOT/infra/terraform/environments/prod"
mkdir -p "$PROJECT_ROOT/infra/kubernetes/base/configmaps"
mkdir -p "$PROJECT_ROOT/infra/kubernetes/base/secrets"
mkdir -p "$PROJECT_ROOT/infra/kubernetes/base/services"
mkdir -p "$PROJECT_ROOT/infra/kubernetes/base/deployments"
mkdir -p "$PROJECT_ROOT/infra/kubernetes/overlays/dev"
mkdir -p "$PROJECT_ROOT/infra/kubernetes/overlays/staging"
mkdir -p "$PROJECT_ROOT/infra/kubernetes/overlays/prod"
mkdir -p "$PROJECT_ROOT/infra/helm/npdms/templates"
mkdir -p "$PROJECT_ROOT/infra/helm/edge/templates"
mkdir -p "$PROJECT_ROOT/infra/ansible/inventory/dev"
mkdir -p "$PROJECT_ROOT/infra/ansible/inventory/staging"
mkdir -p "$PROJECT_ROOT/infra/ansible/inventory/prod"
mkdir -p "$PROJECT_ROOT/infra/ansible/playbooks"
mkdir -p "$PROJECT_ROOT/infra/ansible/roles/common"
mkdir -p "$PROJECT_ROOT/infra/ansible/roles/docker"
mkdir -p "$PROJECT_ROOT/infra/ansible/roles/kubernetes"
mkdir -p "$PROJECT_ROOT/infra/ansible/roles/monitoring"
mkdir -p "$PROJECT_ROOT/infra/docker/base-images/go-base"
mkdir -p "$PROJECT_ROOT/infra/docker/base-images/python-base"
mkdir -p "$PROJECT_ROOT/infra/docker/base-images/node-base"
mkdir -p "$PROJECT_ROOT/infra/docker/registry"

# Edge Deployment
mkdir -p "$PROJECT_ROOT/edge/station/compose"
mkdir -p "$PROJECT_ROOT/edge/station/k3s/manifests"
mkdir -p "$PROJECT_ROOT/edge/station/scripts"
mkdir -p "$PROJECT_ROOT/edge/station/config"
mkdir -p "$PROJECT_ROOT/edge/district/compose"
mkdir -p "$PROJECT_ROOT/edge/district/k3s/manifests"
mkdir -p "$PROJECT_ROOT/edge/district/scripts"
mkdir -p "$PROJECT_ROOT/edge/district/config"
mkdir -p "$PROJECT_ROOT/edge/ai-models/ocr"
mkdir -p "$PROJECT_ROOT/edge/ai-models/nlp"
mkdir -p "$PROJECT_ROOT/edge/ai-models/vision"

# Scripts
mkdir -p "$PROJECT_ROOT/scripts/setup"
mkdir -p "$PROJECT_ROOT/scripts/build"
mkdir -p "$PROJECT_ROOT/scripts/deploy"
mkdir -p "$PROJECT_ROOT/scripts/db"
mkdir -p "$PROJECT_ROOT/scripts/test"

# Tests
mkdir -p "$PROJECT_ROOT/tests/integration"
mkdir -p "$PROJECT_ROOT/tests/e2e"
mkdir -p "$PROJECT_ROOT/tests/load/k6"
mkdir -p "$PROJECT_ROOT/tests/load/locust"
mkdir -p "$PROJECT_ROOT/tests/security"

# Proto
mkdir -p "$PROJECT_ROOT/proto/npdms/v1"

# Configs
mkdir -p "$PROJECT_ROOT/configs/dev"
mkdir -p "$PROJECT_ROOT/configs/staging"
mkdir -p "$PROJECT_ROOT/configs/prod"

# GitHub
mkdir -p "$PROJECT_ROOT/.github/workflows"

# Create placeholder files
touch "$PROJECT_ROOT/ui/mobile/.gitkeep"
touch "$PROJECT_ROOT/edge/ai-models/ocr/.gitkeep"
touch "$PROJECT_ROOT/edge/ai-models/nlp/.gitkeep"
touch "$PROJECT_ROOT/edge/ai-models/vision/.gitkeep"

echo "Creating docker-compose files..."

cat > "$PROJECT_ROOT/docker-compose.dev.yml" << 'EOF'
version: '3.8'

services:
  # Databases
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: npdms
      POSTGRES_PASSWORD: dev_password
      POSTGRES_DB: npdms
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  # Message Broker
  redpanda:
    image: docker.redpanda.com/redpandadata/redpanda:v23.3.5
    command:
      - redpanda start
      - --smp 1
      - --memory 1G
      - --overprovisioned
      - --node-id 0
      - --kafka-addr PLAINTEXT://0.0.0.0:9092
      - --advertise-kafka-addr PLAINTEXT://redpanda:9092
    ports:
      - "9092:9092"

  # Search
  opensearch:
    image: opensearchproject/opensearch:2.11.0
    environment:
      - discovery.type=single-node
      - DISABLE_SECURITY_PLUGIN=true
    ports:
      - "9200:9200"
    volumes:
      - opensearch_data:/usr/share/opensearch/data

  # Graph Database
  neo4j:
    image: neo4j:5.15-community
    environment:
      NEO4J_AUTH: neo4j/dev_password
    ports:
      - "7474:7474"
      - "7687:7687"
    volumes:
      - neo4j_data:/data

  # Object Storage
  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: npdms
      MINIO_ROOT_PASSWORD: dev_password
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - minio_data:/data

volumes:
  postgres_data:
  opensearch_data:
  neo4j_data:
  minio_data:
EOF

echo "Creating script templates..."

cat > "$PROJECT_ROOT/scripts/setup/setup-dev.sh" << 'EOF'
#!/bin/bash
set -e

echo "Setting up NPDMS development environment..."

# Check dependencies
command -v docker >/dev/null 2>&1 || { echo "Docker required"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "Go required"; exit 1; }
command -v node >/dev/null 2>&1 || { echo "Node.js required"; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "Python 3 required"; exit 1; }

echo "Installing Go dependencies..."
cd services
for dir in */; do
    if [ -f "$dir/go.mod" ]; then
        echo "  Installing deps for $dir"
        cd "$dir" && go mod download && cd ..
    fi
done
cd ..

echo "Installing UI dependencies..."
cd ui/web
npm install
cd ../..

echo "Installing AI dependencies..."
cd ai
python3 -m venv .venv
source .venv/bin/activate
for dir in */; do
    if [ -f "$dir/requirements.txt" ]; then
        echo "  Installing deps for $dir"
        pip install -r "$dir/requirements.txt"
    fi
done
cd ..

echo "Starting infrastructure services..."
docker-compose -f docker-compose.dev.yml up -d

echo "Waiting for services to be ready..."
sleep 10

echo "Running database migrations..."
./scripts/db/migrate.sh dev

echo "Setup complete!"
EOF

chmod +x "$PROJECT_ROOT/scripts/setup/setup-dev.sh"

cat > "$PROJECT_ROOT/scripts/db/migrate.sh" << 'EOF'
#!/bin/bash
set -e

ENV=${1:-dev}
echo "Running migrations for environment: $ENV"

# Run migrations for each service
for service in auth fir case evidence personnel armoury vehicle sync audit alert; do
    if [ -d "services/$service/migrations" ]; then
        echo "Migrating $service..."
        migrate -path "services/$service/migrations" \
                -database "postgres://npdms:dev_password@localhost:5432/npdms?sslmode=disable" \
                up
    fi
done

echo "Migrations complete!"
EOF

chmod +x "$PROJECT_ROOT/scripts/db/migrate.sh"

echo "Creating GitHub workflows..."

cat > "$PROJECT_ROOT/.github/workflows/ci.yaml" << 'EOF'
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main, develop]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Lint Go
        uses: golangci/golangci-lint-action@v4
        with:
          working-directory: services

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Lint UI
        run: |
          cd ui/web
          npm ci
          npm run lint

  test-services:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: npdms
          POSTGRES_PASSWORD: test_password
          POSTGRES_DB: npdms_test
        ports:
          - 5432:5432
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Run tests
        run: |
          cd services
          go test -v -race -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v4

  test-ai:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.11'

      - name: Install dependencies
        run: |
          cd ai
          python -m pip install --upgrade pip
          pip install pytest pytest-cov
          for dir in */; do
            if [ -f "$dir/requirements.txt" ]; then
              pip install -r "$dir/requirements.txt"
            fi
          done

      - name: Run tests
        run: |
          cd ai
          pytest --cov=. --cov-report=xml

  test-ui:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install dependencies
        run: |
          cd ui/web
          npm ci

      - name: Run tests
        run: |
          cd ui/web
          npm test

  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run Trivy vulnerability scanner
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: 'fs'
          ignore-unfixed: true
          format: 'sarif'
          output: 'trivy-results.sarif'
          severity: 'CRITICAL,HIGH'

      - name: Upload Trivy scan results
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: 'trivy-results.sarif'

  build:
    needs: [lint, test-services, test-ai, test-ui]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build services
        run: |
          for service in gateway auth fir case evidence personnel armoury vehicle sync audit alert analytics; do
            echo "Building $service..."
            docker build -t npdms/$service:${{ github.sha }} services/$service
          done

      - name: Build AI services
        run: |
          for service in ocr nlp vision graph analytics; do
            echo "Building AI $service..."
            docker build -t npdms/ai-$service:${{ github.sha }} ai/$service
          done

      - name: Build UI
        run: |
          docker build -t npdms/web:${{ github.sha }} ui/web
EOF

echo "=== Project initialization complete ==="
echo ""
echo "Next steps:"
echo "1. cd $PROJECT_ROOT"
echo "2. ./scripts/setup/setup-dev.sh"
echo "3. make start-dev"
```

---

## 4. Run Initialization

Execute the following commands to initialize the project:

```bash
# Navigate to project root
cd /Users/sudipto/Desktop/projects/npdms

# Create the initialization script
mkdir -p scripts/setup

# Copy the init script content above to scripts/setup/init-project.sh
# Then run:
chmod +x scripts/setup/init-project.sh
./scripts/setup/init-project.sh
```

Or run individual commands:

```bash
# Create all directories (paste the mkdir commands from the script)
# Create all files (paste the cat commands from the script)
# Set permissions
find scripts -name "*.sh" -exec chmod +x {} \;
```
