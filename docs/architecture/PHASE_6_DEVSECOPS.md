# PHASE 6 — DEVSECOPS
# National Police Department Management System (NPDMS)

## Document Control
- **Version**: 1.0
- **Classification**: RESTRICTED
- **Author**: Security Architecture Team
- **Last Updated**: 2026-01-04

---

## Security Philosophy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    NPDMS SECURITY PRINCIPLES                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. ASSUME BREACH: Design as if attackers are already inside               │
│  2. DEFENSE IN DEPTH: Multiple layers, no single point of failure          │
│  3. LEAST PRIVILEGE: Minimal access, maximum accountability                 │
│  4. ZERO TRUST: Never trust, always verify, even internal traffic          │
│  5. AUDIT EVERYTHING: Complete, immutable, legally defensible logs         │
│  6. INSIDER THREAT: Assume nation-state adversaries with inside access     │
│  7. 20-YEAR HORIZON: System must survive decades of evolving threats       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 1. CI/CD Pipeline

### 1.1 Pipeline Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        CI/CD PIPELINE FLOW                                   │
└─────────────────────────────────────────────────────────────────────────────┘

DEVELOPER WORKSTATION
        │
        │ [1] Commit + Push
        │     • Pre-commit hooks run locally
        │     • Secrets scan (gitleaks)
        │     • Code formatting check
        │
        ▼
┌───────────────────────────────────────┐
│     SOURCE CONTROL (GitLab/GitHub)    │
│                                       │
│  • Signed commits required            │
│  • Branch protection enabled          │
│  • Code owners enforcement            │
└───────────────────────────────────────┘
        │
        │ [2] Pipeline Triggered
        │
        ▼
┌───────────────────────────────────────┐
│            BUILD STAGE                │
│                                       │
│  • Compile code                       │
│  • Run unit tests                     │
│  • Generate SBOM                      │
│  • Build container images             │
└───────────────────────────────────────┘
        │
        │ [3] Security Scan Stage
        │
        ▼
┌───────────────────────────────────────┐
│          SECURITY SCANS               │
│                                       │
│  • SAST (Semgrep, CodeQL)             │
│  • SCA (Dependency scan)              │
│  • Container scan (Trivy)             │
│  • Secrets scan (truffleHog)          │
│  • License compliance                 │
└───────────────────────────────────────┘
        │
        │ [4] Quality Gate
        │     FAIL if:
        │     • Critical/High vulnerabilities
        │     • Secret detected
        │     • Test coverage < 80%
        │     • License violation
        │
        ▼
┌───────────────────────────────────────┐
│         INTEGRATION TESTS             │
│                                       │
│  • API contract tests                 │
│  • Integration tests                  │
│  • Performance benchmarks             │
└───────────────────────────────────────┘
        │
        │ [5] Artifact Publishing
        │
        ▼
┌───────────────────────────────────────┐
│        ARTIFACT REGISTRY              │
│                                       │
│  • Signed container images            │
│  • Signed helm charts                 │
│  • Attestation attached               │
└───────────────────────────────────────┘
        │
        │ [6] GitOps Sync
        │
        ▼
┌───────────────────────────────────────┐
│         ARGOCD (GitOps)               │
│                                       │
│  • Detects manifest changes           │
│  • Syncs to target environment        │
│  • Rollback on failure                │
└───────────────────────────────────────┘
        │
        │ [7] Deployment
        │
        ├──────────────┬──────────────┐
        ▼              ▼              ▼
   DEV CLUSTER   STAGING CLUSTER  PROD CLUSTER
```

### 1.2 GitHub Actions CI Pipeline

```yaml
# .github/workflows/ci.yaml
name: CI Pipeline

on:
  push:
    branches: [main, develop, 'release/*']
  pull_request:
    branches: [main, develop]

env:
  GO_VERSION: '1.22'
  NODE_VERSION: '20'
  PYTHON_VERSION: '3.11'

jobs:
  # Pre-flight security checks
  security-preflight:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Detect secrets
        uses: trufflesecurity/trufflehog@main
        with:
          path: ./
          base: ${{ github.event.repository.default_branch }}
          head: HEAD

      - name: Check commit signatures
        run: |
          git log --format='%H %G?' origin/main..HEAD | while read hash status; do
            if [ "$status" != "G" ]; then
              echo "Unsigned commit: $hash"
              exit 1
            fi
          done

  # Build and test Go services
  build-go:
    needs: security-preflight
    runs-on: ubuntu-latest
    strategy:
      matrix:
        service: [gateway, auth, fir, case, evidence, personnel, armoury, vehicle, sync, audit, alert, analytics]
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Build
        run: |
          cd services/${{ matrix.service }}
          go build -o bin/${{ matrix.service }} ./cmd/${{ matrix.service }}

      - name: Test with coverage
        run: |
          cd services/${{ matrix.service }}
          go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: services/${{ matrix.service }}/coverage.out
          flags: ${{ matrix.service }}

  # Build and test Rust crypto service
  build-rust:
    needs: security-preflight
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Rust
        uses: dtolnay/rust-toolchain@stable

      - name: Build
        run: |
          cd services/crypto
          cargo build --release

      - name: Test
        run: |
          cd services/crypto
          cargo test

      - name: Security audit
        run: |
          cargo install cargo-audit
          cd services/crypto
          cargo audit

  # Build and test Python AI services
  build-python:
    needs: security-preflight
    runs-on: ubuntu-latest
    strategy:
      matrix:
        service: [ocr, nlp, vision, graph, analytics]
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: ${{ env.PYTHON_VERSION }}

      - name: Install dependencies
        run: |
          cd ai/${{ matrix.service }}
          pip install -r requirements.txt
          pip install pytest pytest-cov ruff mypy

      - name: Lint
        run: |
          cd ai/${{ matrix.service }}
          ruff check .
          mypy src/

      - name: Test
        run: |
          cd ai/${{ matrix.service }}
          pytest --cov=src --cov-report=xml tests/

  # Build UI
  build-ui:
    needs: security-preflight
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Node
        uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
          cache: 'npm'
          cache-dependency-path: ui/web/package-lock.json

      - name: Install dependencies
        run: |
          cd ui/web
          npm ci

      - name: Lint
        run: |
          cd ui/web
          npm run lint

      - name: Type check
        run: |
          cd ui/web
          npm run type-check

      - name: Test
        run: |
          cd ui/web
          npm test -- --coverage

      - name: Build
        run: |
          cd ui/web
          npm run build

  # Security scans
  security-scan:
    needs: [build-go, build-rust, build-python, build-ui]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      # SAST
      - name: Run Semgrep
        uses: returntocorp/semgrep-action@v1
        with:
          config: >-
            p/security-audit
            p/secrets
            p/owasp-top-ten

      # Dependency scan
      - name: Run Trivy (filesystem)
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: 'fs'
          severity: 'CRITICAL,HIGH'
          exit-code: '1'

      # SBOM generation
      - name: Generate SBOM
        uses: anchore/sbom-action@v0
        with:
          path: .
          output-file: sbom.spdx.json

      - name: Upload SBOM
        uses: actions/upload-artifact@v4
        with:
          name: sbom
          path: sbom.spdx.json

  # Build and push container images
  build-images:
    needs: security-scan
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    strategy:
      matrix:
        service: [gateway, auth, fir, case, evidence, personnel, armoury, vehicle, sync, audit, alert, analytics]
    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to registry
        uses: docker/login-action@v3
        with:
          registry: ${{ secrets.REGISTRY_URL }}
          username: ${{ secrets.REGISTRY_USER }}
          password: ${{ secrets.REGISTRY_PASSWORD }}

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: services/${{ matrix.service }}
          push: true
          tags: |
            ${{ secrets.REGISTRY_URL }}/npdms/${{ matrix.service }}:${{ github.sha }}
            ${{ secrets.REGISTRY_URL }}/npdms/${{ matrix.service }}:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max

      # Sign image
      - name: Sign image
        run: |
          cosign sign --key env://COSIGN_PRIVATE_KEY \
            ${{ secrets.REGISTRY_URL }}/npdms/${{ matrix.service }}:${{ github.sha }}
        env:
          COSIGN_PRIVATE_KEY: ${{ secrets.COSIGN_PRIVATE_KEY }}

      # Scan image
      - name: Scan image
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: ${{ secrets.REGISTRY_URL }}/npdms/${{ matrix.service }}:${{ github.sha }}
          severity: 'CRITICAL,HIGH'
          exit-code: '1'
```

### 1.3 Quality Gates

```yaml
# Quality gate thresholds
quality_gates:
  code_coverage:
    minimum: 80%
    critical_paths: 90%

  security:
    critical_vulnerabilities: 0
    high_vulnerabilities: 0
    medium_vulnerabilities: 10  # Max allowed
    secrets_detected: 0

  performance:
    api_latency_p99: 500ms
    memory_increase: 10%  # Max increase from baseline

  dependencies:
    outdated_critical: 0
    license_violations: 0
```

---

## 2. GitOps Workflow

### 2.1 Repository Structure

```
npdms-gitops/
├── apps/                          # Application manifests
│   ├── base/
│   │   ├── gateway/
│   │   │   ├── deployment.yaml
│   │   │   ├── service.yaml
│   │   │   ├── configmap.yaml
│   │   │   └── kustomization.yaml
│   │   ├── auth/
│   │   ├── fir/
│   │   └── ...
│   │
│   ├── overlays/
│   │   ├── dev/
│   │   │   ├── kustomization.yaml
│   │   │   └── patches/
│   │   ├── staging/
│   │   └── prod/
│   │       ├── kustomization.yaml
│   │       ├── patches/
│   │       │   ├── replicas.yaml
│   │       │   └── resources.yaml
│   │       └── secrets/          # Encrypted with SOPS
│   │           └── secrets.enc.yaml
│   │
│   └── argocd/
│       ├── project.yaml
│       └── applications/
│           ├── dev.yaml
│           ├── staging.yaml
│           └── prod.yaml
│
├── infrastructure/                # Infrastructure components
│   ├── base/
│   │   ├── namespace.yaml
│   │   ├── network-policies/
│   │   ├── rbac/
│   │   └── monitoring/
│   │
│   └── overlays/
│       ├── dev/
│       ├── staging/
│       └── prod/
│
└── clusters/                      # Cluster-specific configs
    ├── central/
    │   └── kustomization.yaml
    ├── state-karnataka/
    ├── state-maharashtra/
    └── ...
```

### 2.2 ArgoCD Configuration

```yaml
# argocd/applications/prod.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: npdms-prod
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: npdms
  source:
    repoURL: https://git.npdms.gov.in/npdms/npdms-gitops.git
    targetRevision: main
    path: apps/overlays/prod
  destination:
    server: https://kubernetes.default.svc
    namespace: npdms-prod
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
      allowEmpty: false
    syncOptions:
      - CreateNamespace=true
      - PrunePropagationPolicy=foreground
      - PruneLast=true
    retry:
      limit: 5
      backoff:
        duration: 5s
        factor: 2
        maxDuration: 3m
  ignoreDifferences:
    - group: apps
      kind: Deployment
      jsonPointers:
        - /spec/replicas  # Ignore HPA-managed replicas

---
# Sync waves for ordered deployment
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: npdms-infrastructure
  annotations:
    argocd.argoproj.io/sync-wave: "-1"  # Deploy first
spec:
  # ...

---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: npdms-databases
  annotations:
    argocd.argoproj.io/sync-wave: "0"  # After infrastructure
spec:
  # ...

---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: npdms-services
  annotations:
    argocd.argoproj.io/sync-wave: "1"  # After databases
spec:
  # ...
```

### 2.3 Promotion Workflow

```yaml
# Promotion from staging to prod requires:
promotion:
  requirements:
    - staging_tests_passed: true
    - security_scan_passed: true
    - performance_benchmark_passed: true
    - manual_approval:
        approvers:
          - security-team
          - operations-team
        minimum_approvals: 2

  process:
    1. Create PR from staging to prod branch
    2. Automated tests run against staging
    3. Security team reviews changes
    4. Operations team reviews changes
    5. Both teams approve PR
    6. PR merged
    7. ArgoCD detects change
    8. Canary deployment begins
    9. Progressive rollout (10% -> 50% -> 100%)
    10. Automated rollback if metrics degrade
```

---

## 3. Secrets Management

### 3.1 HashiCorp Vault Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    SECRETS MANAGEMENT ARCHITECTURE                           │
└─────────────────────────────────────────────────────────────────────────────┘

┌───────────────────────────────────────────────────────────────────────────┐
│                         HASHICORP VAULT CLUSTER                            │
│                                                                            │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐        │
│  │   Vault Node 1   │  │   Vault Node 2   │  │   Vault Node 3   │        │
│  │   (Active)       │  │   (Standby)      │  │   (Standby)      │        │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘        │
│              │                   │                   │                    │
│              └───────────────────┴───────────────────┘                    │
│                                  │                                        │
│                          ┌───────┴────────┐                               │
│                          │   HSM Backend  │  ← Auto-unseal keys          │
│                          │   (PKCS#11)    │                               │
│                          └────────────────┘                               │
│                                                                            │
│  Secret Engines:                                                          │
│  ├── kv-v2/npdms/         (Key-Value secrets)                            │
│  ├── database/            (Dynamic database credentials)                  │
│  ├── pki/npdms/           (Certificate authority)                        │
│  └── transit/             (Encryption as a service)                       │
│                                                                            │
│  Auth Methods:                                                            │
│  ├── kubernetes/          (Pod authentication)                            │
│  ├── approle/             (Application authentication)                    │
│  └── ldap/                (Human authentication)                          │
└───────────────────────────────────────────────────────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    │                             │
                    ▼                             ▼
         ┌──────────────────┐          ┌──────────────────┐
         │  Central Cluster │          │   Edge Nodes     │
         │  (Full access)   │          │  (Limited cache) │
         └──────────────────┘          └──────────────────┘
```

### 3.2 Vault Policies

```hcl
# vault/policies/fir-service.hcl
path "kv-v2/data/npdms/fir/*" {
  capabilities = ["read"]
}

path "database/creds/fir-service" {
  capabilities = ["read"]
}

path "pki/npdms/issue/fir-service" {
  capabilities = ["create", "update"]
}

path "transit/encrypt/fir-encryption-key" {
  capabilities = ["update"]
}

path "transit/decrypt/fir-encryption-key" {
  capabilities = ["update"]
}

# Deny access to other services' secrets
path "kv-v2/data/npdms/auth/*" {
  capabilities = ["deny"]
}
```

### 3.3 Kubernetes Integration

```yaml
# Vault Agent Injector configuration
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fir-service
spec:
  template:
    metadata:
      annotations:
        vault.hashicorp.com/agent-inject: "true"
        vault.hashicorp.com/role: "fir-service"
        vault.hashicorp.com/agent-inject-secret-config: "kv-v2/data/npdms/fir/config"
        vault.hashicorp.com/agent-inject-template-config: |
          {{- with secret "kv-v2/data/npdms/fir/config" -}}
          DATABASE_URL={{ .Data.data.database_url }}
          JWT_SECRET={{ .Data.data.jwt_secret }}
          {{- end -}}
    spec:
      serviceAccountName: fir-service
      containers:
        - name: fir-service
          image: npdms/fir:latest
          envFrom:
            - secretRef:
                name: fir-service-secrets  # Injected by Vault
```

### 3.4 Secret Rotation

```yaml
# Automated secret rotation policy
rotation:
  database_credentials:
    method: dynamic  # Vault generates on-demand
    ttl: 1h
    max_ttl: 24h

  api_keys:
    method: scheduled
    frequency: 30d
    notification: 7d_before

  certificates:
    method: auto_renewal
    renew_before: 30d
    validity: 365d

  encryption_keys:
    method: manual  # Requires approval
    rotation_ceremony: required
    key_shares: 5
    threshold: 3
```

---

## 4. Zero Trust Enforcement

### 4.1 Zero Trust Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        ZERO TRUST ARCHITECTURE                               │
└─────────────────────────────────────────────────────────────────────────────┘

                         ┌─────────────────────────┐
                         │    Policy Engine (OPA)   │
                         │                         │
                         │  • Identity verification│
                         │  • Context evaluation   │
                         │  • Access decision      │
                         └───────────┬─────────────┘
                                     │
    ┌────────────────────────────────┼────────────────────────────────┐
    │                                │                                │
    ▼                                ▼                                ▼
┌─────────┐                    ┌─────────┐                      ┌─────────┐
│ User    │ ──► Identity ──►  │ Service │ ──► Identity ──►    │ Service │
│         │     Verification  │   A     │     Verification     │   B     │
└─────────┘                    └─────────┘                      └─────────┘

EVERY REQUEST MUST:
─────────────────
1. Authenticate (prove identity)
2. Present valid context (device, location, time)
3. Request specific resource
4. Be evaluated against policy
5. Be logged for audit
```

### 4.2 Service Mesh (Istio) Configuration

```yaml
# Strict mTLS for all services
apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: default
  namespace: npdms-prod
spec:
  mtls:
    mode: STRICT

---
# Authorization policy for FIR service
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: fir-service-policy
  namespace: npdms-prod
spec:
  selector:
    matchLabels:
      app: fir-service
  action: ALLOW
  rules:
    # Allow gateway to call FIR service
    - from:
        - source:
            principals: ["cluster.local/ns/npdms-prod/sa/gateway"]
      to:
        - operation:
            methods: ["GET", "POST", "PUT"]
            paths: ["/api/v1/fir/*"]
    # Allow sync service
    - from:
        - source:
            principals: ["cluster.local/ns/npdms-prod/sa/sync-service"]
      to:
        - operation:
            methods: ["GET"]
            paths: ["/internal/sync/*"]
    # Deny all other traffic (implicit)

---
# Network policy - defense in depth
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: fir-service-network-policy
  namespace: npdms-prod
spec:
  podSelector:
    matchLabels:
      app: fir-service
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: gateway
        - podSelector:
            matchLabels:
              app: sync-service
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - podSelector:
            matchLabels:
              app: postgres
      ports:
        - protocol: TCP
          port: 5432
    - to:
        - podSelector:
            matchLabels:
              app: redis
      ports:
        - protocol: TCP
          port: 6379
```

### 4.3 OPA Policies

```rego
# policies/api_access.rego
package npdms.api

import future.keywords.if
import future.keywords.in

default allow = false

# Allow if all checks pass
allow if {
    valid_token
    valid_role
    valid_jurisdiction
    valid_device
    valid_time_window
}

# Token validation
valid_token if {
    token := input.token
    io.jwt.verify_es256(token, data.jwks)
    claims := io.jwt.decode(token)[1]
    time.now_ns() < claims.exp * 1e9
}

# Role-based access
valid_role if {
    claims := io.jwt.decode(input.token)[1]
    required_roles := data.api_permissions[input.path][input.method]
    claims.role in required_roles
}

# Jurisdiction check
valid_jurisdiction if {
    claims := io.jwt.decode(input.token)[1]
    resource_jurisdiction := input.resource.jurisdiction

    # User's jurisdiction must contain resource jurisdiction
    jurisdiction_contains(claims.jurisdiction, resource_jurisdiction)
}

# Device binding
valid_device if {
    claims := io.jwt.decode(input.token)[1]
    input.device_id == claims.device_id
    device_registered(claims.device_id, claims.user_id)
}

# Time-based access control
valid_time_window if {
    claims := io.jwt.decode(input.token)[1]
    current_hour := time.clock(time.now_ns())[0]

    # Check if current hour is within allowed window
    claims.role == "CONSTABLE"
    current_hour >= 6
    current_hour <= 22
}

valid_time_window if {
    claims := io.jwt.decode(input.token)[1]
    # Supervisors have 24/7 access
    claims.role in ["SHO", "SP", "DGP"]
}
```

---

## 5. Logging & Audit

### 5.1 Logging Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        LOGGING ARCHITECTURE                                  │
└─────────────────────────────────────────────────────────────────────────────┘

APPLICATIONS
     │
     │ Structured JSON logs
     │ OpenTelemetry SDK
     │
     ▼
┌─────────────────────────────────────┐
│      FLUENTBIT (DaemonSet)          │
│                                     │
│  • Log collection                   │
│  • PII masking                      │
│  • Enrichment (pod labels, etc.)    │
│  • Buffer and retry                 │
└─────────────────────────────────────┘
     │
     ├──────────────────┬──────────────────┐
     │                  │                  │
     ▼                  ▼                  ▼
┌──────────┐      ┌──────────┐      ┌──────────┐
│   LOKI   │      │ OPENSEARCH│      │  S3/MINIO │
│ (Recent) │      │ (Indexed)│      │ (Archive) │
│          │      │          │      │           │
│ 30 days  │      │ 1 year   │      │ 20 years  │
└──────────┘      └──────────┘      └──────────┘
     │                  │                  │
     └──────────────────┴──────────────────┘
                       │
                       ▼
              ┌──────────────┐
              │   GRAFANA    │
              │  Dashboards  │
              │   & Alerts   │
              └──────────────┘
```

### 5.2 Audit Log Schema

```json
{
  "$schema": "https://npdms.gov.in/schemas/audit-log-v1.json",
  "type": "object",
  "required": ["timestamp", "event_id", "action", "actor", "outcome"],
  "properties": {
    "timestamp": {
      "type": "string",
      "format": "date-time",
      "description": "ISO 8601 timestamp with timezone"
    },
    "event_id": {
      "type": "string",
      "format": "uuid",
      "description": "Unique event identifier"
    },
    "action": {
      "type": "string",
      "enum": ["CREATE", "READ", "UPDATE", "DELETE", "LOGIN", "LOGOUT", "EXPORT", "PRINT"],
      "description": "Action performed"
    },
    "actor": {
      "type": "object",
      "properties": {
        "user_id": {"type": "string"},
        "badge_number": {"type": "string"},
        "role": {"type": "string"},
        "station_id": {"type": "string"},
        "device_id": {"type": "string"},
        "ip_address": {"type": "string"},
        "session_id": {"type": "string"}
      }
    },
    "resource": {
      "type": "object",
      "properties": {
        "type": {"type": "string"},
        "id": {"type": "string"},
        "attributes": {"type": "object"}
      }
    },
    "outcome": {
      "type": "string",
      "enum": ["SUCCESS", "FAILURE", "DENIED"]
    },
    "reason": {
      "type": "string",
      "description": "Reason for failure or denial"
    },
    "context": {
      "type": "object",
      "properties": {
        "trace_id": {"type": "string"},
        "span_id": {"type": "string"},
        "request_id": {"type": "string"}
      }
    }
  }
}
```

### 5.3 Audit Log Immutability

```yaml
# Audit log storage configuration
audit_storage:
  primary:
    type: postgresql
    table: audit_log
    features:
      - append_only: true  # No UPDATE/DELETE allowed
      - trigger_protected: true  # Triggers prevent modification
      - row_level_security: true

  secondary:
    type: blockchain_anchor  # Periodic hash anchoring
    frequency: hourly
    chain: private_permissioned

  archive:
    type: s3
    bucket: npdms-audit-archive
    encryption: AES-256-GCM
    retention: 20_years
    worm: true  # Write Once Read Many

# PostgreSQL append-only enforcement
sql: |
  -- Prevent any modifications to audit_log
  CREATE OR REPLACE FUNCTION prevent_audit_modification()
  RETURNS TRIGGER AS $$
  BEGIN
    RAISE EXCEPTION 'Audit log modification not allowed';
  END;
  $$ LANGUAGE plpgsql;

  CREATE TRIGGER audit_log_immutable
  BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION prevent_audit_modification();

  -- Revoke dangerous permissions
  REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM PUBLIC;
  REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM app_user;
```

---

## 6. Insider Threat Controls

### 6.1 Threat Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     INSIDER THREAT CATEGORIES                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. MALICIOUS ADMINISTRATOR                                                 │
│     • Has system access                                                     │
│     • May collude with external actors                                      │
│     • Mitigation: Multi-party controls, audit logging                       │
│                                                                             │
│  2. COMPROMISED CREDENTIALS                                                 │
│     • Legitimate credentials stolen                                         │
│     • May be nation-state level                                             │
│     • Mitigation: MFA, anomaly detection, short-lived tokens               │
│                                                                             │
│  3. DISGRUNTLED EMPLOYEE                                                    │
│     • Data exfiltration                                                     │
│     • Sabotage                                                              │
│     • Mitigation: Least privilege, DLP, termination procedures             │
│                                                                             │
│  4. SOCIAL ENGINEERING VICTIM                                               │
│     • Unintentional disclosure                                              │
│     • Phishing                                                              │
│     • Mitigation: Training, technical controls                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Technical Controls

```yaml
# Insider threat controls
controls:
  # Privileged access management
  privileged_access:
    - just_in_time_access: true  # No standing privileges
    - approval_required: true
    - time_limited: 4_hours
    - session_recording: true
    - break_glass_procedure: documented

  # Data loss prevention
  dlp:
    - bulk_export_alert: 100_records
    - bulk_export_block: 1000_records
    - sensitive_field_masking: true
    - screenshot_detection: true
    - usb_disabled: true

  # Anomaly detection
  anomaly_detection:
    - unusual_access_times: alert
    - unusual_access_volume: alert
    - unusual_access_patterns: alert
    - geographic_anomalies: block_and_alert
    - impossible_travel: block_and_alert

  # Separation of duties
  separation:
    - no_single_admin: true
    - four_eyes_principle: critical_operations
    - key_ceremony: encryption_keys

  # Monitoring
  monitoring:
    - admin_actions: all_logged
    - database_queries: logged_and_analyzed
    - network_traffic: monitored
    - file_access: logged
```

### 6.3 Behavioral Analytics

```python
class InsiderThreatDetector:
    def __init__(self):
        self.baseline = UserBaselineModel()
        self.alert_service = AlertService()

    def analyze_user_activity(self, user_id: str, activity: Activity) -> ThreatScore:
        """Analyze user activity for insider threat indicators."""

        # Get user baseline
        baseline = self.baseline.get(user_id)

        risk_factors = []

        # Check access time
        if not baseline.is_normal_time(activity.timestamp):
            risk_factors.append(RiskFactor(
                type="UNUSUAL_TIME",
                severity=0.3,
                description=f"Access at {activity.timestamp}, outside normal hours"
            ))

        # Check access volume
        recent_volume = self.get_access_volume(user_id, hours=24)
        if recent_volume > baseline.normal_volume * 3:
            risk_factors.append(RiskFactor(
                type="HIGH_VOLUME",
                severity=0.5,
                description=f"3x normal access volume in last 24h"
            ))

        # Check data sensitivity
        if activity.involves_sensitive_data:
            if not baseline.normally_accesses_sensitive:
                risk_factors.append(RiskFactor(
                    type="UNUSUAL_SENSITIVITY",
                    severity=0.4,
                    description="Accessing sensitive data not in normal pattern"
                ))

        # Check geographic anomaly
        if self.is_impossible_travel(user_id, activity.location):
            risk_factors.append(RiskFactor(
                type="IMPOSSIBLE_TRAVEL",
                severity=0.9,
                description="Login from impossible geographic location"
            ))

        # Calculate overall score
        score = ThreatScore(
            user_id=user_id,
            score=min(1.0, sum(f.severity for f in risk_factors)),
            factors=risk_factors
        )

        # Alert if threshold exceeded
        if score.score > 0.7:
            self.alert_service.send_alert(
                type="HIGH_INSIDER_THREAT_SCORE",
                user_id=user_id,
                score=score
            )

        return score
```

---

## 7. Disaster Recovery

### 7.1 DR Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     DISASTER RECOVERY ARCHITECTURE                           │
└─────────────────────────────────────────────────────────────────────────────┘

                    PRIMARY SITE                    DR SITE
                    (Delhi)                         (Hyderabad)
                        │                               │
┌───────────────────────┼───────────────────────────────┼──────────────────────┐
│                       │                               │                      │
│  ┌─────────────────┐  │  Synchronous Replication     │  ┌─────────────────┐ │
│  │  PostgreSQL     │──┼──────────────────────────────┼─▶│  PostgreSQL     │ │
│  │  (Primary)      │  │  < 10ms latency              │  │  (Standby)      │ │
│  └─────────────────┘  │                               │  └─────────────────┘ │
│                       │                               │                      │
│  ┌─────────────────┐  │  Async Replication           │  ┌─────────────────┐ │
│  │  ScyllaDB       │──┼──────────────────────────────┼─▶│  ScyllaDB       │ │
│  │  (Multi-DC)     │  │  < 1 min lag                 │  │  (Multi-DC)     │ │
│  └─────────────────┘  │                               │  └─────────────────┘ │
│                       │                               │                      │
│  ┌─────────────────┐  │  Cross-Region Replication    │  ┌─────────────────┐ │
│  │  MinIO          │──┼──────────────────────────────┼─▶│  MinIO          │ │
│  │  (Evidence)     │  │  < 15 min lag                │  │  (Evidence)     │ │
│  └─────────────────┘  │                               │  └─────────────────┘ │
│                       │                               │                      │
│  ┌─────────────────┐  │                               │  ┌─────────────────┐ │
│  │  Kubernetes     │  │                               │  │  Kubernetes     │ │
│  │  (Active)       │  │                               │  │  (Standby)      │ │
│  └─────────────────┘  │                               │  └─────────────────┘ │
│                       │                               │                      │
└───────────────────────┼───────────────────────────────┼──────────────────────┘
                        │                               │
                        │      DNS / Global Load        │
                        │         Balancer              │
                        └───────────────────────────────┘
```

### 7.2 RTO/RPO Targets

| Tier | RPO | RTO | Strategy |
|------|-----|-----|----------|
| **Central** | 0 (zero data loss) | 15 minutes | Synchronous replication, hot standby |
| **State** | 5 minutes | 30 minutes | Async replication, warm standby |
| **District** | 15 minutes | 1 hour | Async replication, cold standby |
| **Station** | 4 hours | 4 hours | Local backup + cloud sync |

### 7.3 Backup Strategy

```yaml
# Backup configuration
backups:
  postgresql:
    full_backup:
      frequency: daily
      time: "02:00 UTC"
      retention: 30_days

    incremental_backup:
      frequency: hourly
      retention: 7_days

    wal_archiving:
      enabled: true
      destination: s3://npdms-backups/wal/

  evidence_storage:
    backup_type: cross_region_replication
    replication_lag: 15_minutes
    versioning: enabled

  configuration:
    backup_type: gitops  # Stored in Git
    encryption: sops

  encryption_keys:
    backup_type: hsm_backup
    frequency: monthly
    ceremony_required: true
    key_shares: 5
    threshold: 3
```

### 7.4 Failover Procedure

```yaml
# Automated failover procedure
failover:
  trigger_conditions:
    - primary_site_unreachable: 5_minutes
    - database_replication_lag: 30_minutes
    - critical_service_failure: 3_services

  automatic_failover:
    enabled: true
    confirmation_required: false  # For RTO < 15 min
    notification: immediate

  procedure:
    1. Detect primary failure
    2. Verify DR site health
    3. Promote PostgreSQL standby to primary
    4. Update DNS records (TTL: 60s)
    5. Scale up DR Kubernetes workloads
    6. Verify service health
    7. Notify operations team
    8. Begin incident review

  rollback:
    manual_only: true
    data_reconciliation: required
    approval: operations_lead
```

### 7.5 DR Testing

```yaml
# DR testing schedule
dr_testing:
  tabletop_exercise:
    frequency: monthly
    participants:
      - operations_team
      - development_team
      - security_team
    scenarios:
      - site_failure
      - ransomware
      - insider_attack

  partial_failover:
    frequency: quarterly
    scope: non_production
    duration: 4_hours

  full_failover:
    frequency: annually
    scope: production
    duration: 24_hours
    advance_notice: 2_weeks
    rollback_tested: true
```

---

## 8. Compliance & Audit Readiness

### 8.1 Compliance Framework

```yaml
compliance:
  frameworks:
    - ISO_27001:2022
    - ISO_27701:2019  # Privacy
    - NIST_SP_800-53
    - IT_Act_2000
    - Evidence_Act

  continuous_compliance:
    tool: OpenSCAP
    frequency: daily
    reporting: automated

  audit_readiness:
    evidence_collection: automated
    retention: 7_years
    format: machine_readable
```

### 8.2 Security Controls Matrix

| Control | Implementation | Verification |
|---------|---------------|--------------|
| Access Control | RBAC + ABAC via OPA | Daily policy audit |
| Encryption | TLS 1.3 + AES-256-GCM | Certificate monitoring |
| Audit Logging | Immutable logs, 20-year retention | Log integrity checks |
| Vulnerability Mgmt | Daily scans, 48h patch SLA | Scan reports |
| Incident Response | Documented procedures | Quarterly drills |
| Business Continuity | DR site, tested annually | DR test reports |
| Data Protection | Encryption, masking, DLP | Data flow audits |

---

## Appendix: Security Checklist

### Pre-Deployment Checklist

- [ ] All containers signed with cosign
- [ ] SBOM generated and stored
- [ ] Vulnerability scan passed (0 critical/high)
- [ ] Secrets scan passed (0 secrets in code)
- [ ] SAST scan passed
- [ ] Dependency audit passed
- [ ] License compliance verified
- [ ] Network policies applied
- [ ] mTLS enabled
- [ ] Vault integration tested
- [ ] Audit logging verified
- [ ] Backup verified
- [ ] DR runbook updated
- [ ] Security team sign-off obtained
