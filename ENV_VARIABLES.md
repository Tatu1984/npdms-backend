# NPDMS - Environment Variables

## Complete Environment Variables List

### 1. Frontend (Next.js) - Vercel Environment Variables

```bash
# ========================================
# CORE APPLICATION
# ========================================
NEXT_PUBLIC_APP_URL=https://npdms.infinititechpartners.com
NEXT_PUBLIC_APP_NAME=NPDMS
NEXT_PUBLIC_APP_VERSION=1.0.0

# ========================================
# API CONFIGURATION
# ========================================
NEXT_PUBLIC_API_URL=https://api.npdms.infinititechpartners.com/api/v1
NEXT_PUBLIC_USE_REAL_API=true

# ========================================
# AUTHENTICATION - NextAuth.js
# ========================================
NEXTAUTH_URL=https://npdms.infinititechpartners.com
NEXTAUTH_SECRET=<generate-with-openssl-rand-base64-32>

# ========================================
# MICROSOFT ENTRA ID (Azure AD) - Optional
# ========================================
AZURE_AD_CLIENT_ID=<your-azure-ad-client-id>
AZURE_AD_CLIENT_SECRET=<your-azure-ad-client-secret>
AZURE_AD_TENANT_ID=<your-azure-ad-tenant-id>

# ========================================
# DATABASE - NeonDB PostgreSQL
# ========================================
DATABASE_URL=postgresql://<DB_USER>:<DB_PASSWORD>@<DB_HOST>/neondb?sslmode=require&channel_binding=require
POSTGRES_PRISMA_URL=postgresql://<DB_USER>:<DB_PASSWORD>@<DB_HOST>/neondb?sslmode=require&channel_binding=require&pgbouncer=true
POSTGRES_URL_NON_POOLING=postgresql://<DB_USER>:<DB_PASSWORD>@<DB_HOST_DIRECT>/neondb?sslmode=require

# ========================================
# AI MODEL SERVICES (on premises only)
# ========================================
# Models run on the platform's own hardware. No case data, audio, footage,
# statement or personal detail goes to an outside AI provider, so there is no
# API key for one here and there is not meant to be.
#
# The AI gateway reads one address per registered model, and the model registry
# records which variable holds it (ai_model_configs.endpoint_env). Adding a
# model is therefore a registry entry and a variable, not a code change:
#
#   <NAME>_URL                   the service address
#   <NAME>_URL_TOKEN             bearer token, if the service expects one
#   <NAME>_URL_TIMEOUT_SECONDS   request timeout in seconds, default 30
#
# Where a model's variable is unset, that model is not connected on this
# deployment and every screen says so. The staging API on Vercel runs no model
# services at all.
#
# The two model services that exist today:
FR_SERVICE_URL=                  # face recognition, missing persons only
FR_SERVICE_TOKEN=
ML_VEHICLE_DETECTION_URL=        # vehicle detection and ANPR

# ========================================
# ANALYTICS & MONITORING (Future)
# ========================================
SENTRY_DSN=<your-sentry-dsn>
NEXT_PUBLIC_POSTHOG_KEY=<your-posthog-key>
NEXT_PUBLIC_POSTHOG_HOST=https://app.posthog.com
```

### 2. Backend (Go API) - Docker/Cloud Run Environment Variables

```bash
# ========================================
# SERVER
# ========================================
PORT=8080
ENV=production

# ========================================
# DATABASE - PostgreSQL
# ========================================
DATABASE_URL=postgresql://<DB_USER>:<DB_PASSWORD>@<DB_HOST>/neondb?sslmode=require

# ========================================
# REDIS - For sessions & rate limiting
# ========================================
REDIS_URL=redis://<user>:<password>@<redis-host>:6379

# ========================================
# OBJECT STORAGE - MinIO/S3
# ========================================
MINIO_ENDPOINT=s3.amazonaws.com
MINIO_ACCESS_KEY=<your-aws-access-key>
MINIO_SECRET_KEY=<your-aws-secret-key>
MINIO_BUCKET=npdms-evidence
MINIO_USE_SSL=true

# ========================================
# JWT AUTHENTICATION
# ========================================
JWT_SECRET=<generate-strong-secret-min-32-chars>

# ========================================
# CORS
# ========================================
ALLOWED_ORIGINS=https://npdms.infinititechpartners.com,https://www.npdms.infinititechpartners.com
```

### 3. Redis Cloud (Recommended: Upstash or Redis Cloud)

```bash
# Upstash Redis (Serverless - Recommended for Vercel)
REDIS_URL=rediss://default:<password>@<region>.upstash.io:6379

# OR Redis Cloud
REDIS_URL=redis://default:<password>@<host>:<port>
```

### 4. Object Storage Options

```bash
# Option A: AWS S3
AWS_ACCESS_KEY_ID=<your-access-key>
AWS_SECRET_ACCESS_KEY=<your-secret-key>
AWS_REGION=ap-south-1
S3_BUCKET=npdms-evidence

# Option B: Cloudflare R2 (Cheaper)
R2_ACCESS_KEY_ID=<your-r2-access-key>
R2_SECRET_ACCESS_KEY=<your-r2-secret-key>
R2_BUCKET=npdms-evidence
R2_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com

# Option C: Vercel Blob (Easiest for Vercel)
BLOB_READ_WRITE_TOKEN=<your-vercel-blob-token>
```

---

## Service Recommendations

| Service | Provider | Cost Estimate |
|---------|----------|---------------|
| Database | NeonDB (already setup) | Free tier / $19/mo |
| Redis | Upstash | Free tier / $10/mo |
| Object Storage | Cloudflare R2 | $0.015/GB/mo |
| API Hosting | Railway / Render / Fly.io | $5-20/mo |
| Frontend | Vercel | Free tier / $20/mo |
| Auth | Microsoft Entra ID | Free (included in M365) |
| AI | On-premises model services on the edge server | Hardware, not per use |
| Monitoring | Sentry | Free tier |

---

## Quick Setup Commands

```bash
# Generate NEXTAUTH_SECRET
openssl rand -base64 32

# Generate JWT_SECRET
openssl rand -hex 32

# Test database connection
psql "postgresql://<DB_USER>:<DB_PASSWORD>@<DB_HOST>/neondb?sslmode=require"
```
