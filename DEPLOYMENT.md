# NPDMS Deployment Guide

Complete deployment guide for the National Police Data Management System with backend, PWA, and AI/ML capabilities.

## Table of Contents

1. [System Requirements](#system-requirements)
2. [Quick Start](#quick-start)
3. [Development Setup](#development-setup)
4. [Production Deployment](#production-deployment)
5. [ML Services](#ml-services)
6. [Troubleshooting](#troubleshooting)

---

## System Requirements

### Hardware (Production - MDC)
- **CPU**: 16+ cores (8 API, 4 DB, 4 ML)
- **RAM**: 64GB (16 API, 24 PostgreSQL, 16 ML, 8 Redis)
- **Storage**: 2TB SSD
  - 500GB for database
  - 1TB for MinIO (evidence files)
  - 500GB for ML models and cache
- **Network**: 1 Gbps internal, stable internet for initial setup

### Software
- **Docker**: 24.0+
- **Docker Compose**: 2.20+
- **Go**: 1.22+ (for local development)
- **Node.js**: 20+ (for frontend development)
- **PostgreSQL**: 16 (via Docker)
- **Python**: 3.10+ (for ML services)

---

## Quick Start

### 1. Clone and Setup

```bash
# Clone repository
git clone <repository-url> /opt/npdms
cd /opt/npdms

# Copy environment file
cp .env.example .env

# Edit environment variables
nano .env
```

### 2. Configure Environment

Edit `.env` with production values:

```env
# Database
DATABASE_URL=postgres://npdms:CHANGE_THIS_PASSWORD@postgres:5432/npdms?sslmode=disable
POSTGRES_PASSWORD=CHANGE_THIS_PASSWORD

# Redis
REDIS_URL=redis://redis:6379

# MinIO
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=npdms_admin
MINIO_SECRET_KEY=CHANGE_THIS_MINIO_SECRET

# JWT
JWT_SECRET=CHANGE_THIS_JWT_SECRET_MIN_32_CHARS

# ML Services
ML_FIR_CLASSIFIER_URL=http://ml-fir-classifier:8001
ML_SEMANTIC_SEARCH_URL=http://ml-semantic-search:8002
ML_CRIME_PREDICTION_URL=http://ml-crime-prediction:8003

# Environment
ENV=production
PORT=8080
```

### 3. Start Services

```bash
# Start infrastructure only (fastest)
docker-compose up -d postgres redis minio

# Wait for services to be healthy (30 seconds)
docker-compose ps

# Start with ML services
docker-compose --profile ml up -d

# Start everything (full stack)
docker-compose --profile full up -d
```

### 4. Initialize Database

```bash
# Run migrations
docker-compose exec api /app/migrate -path /app/migrations -database "${DATABASE_URL}" up

# Or manually
cd services/api
go run cmd/migrate/main.go up
```

### 5. Access Services

- **Frontend**: http://localhost:3000
- **API**: http://localhost:8080
- **API Docs**: http://localhost:8080/swagger
- **MinIO Console**: http://localhost:9001
- **ML - FIR Classifier**: http://localhost:8001/docs
- **ML - Semantic Search**: http://localhost:8002/docs
- **ML - Crime Prediction**: http://localhost:8003/docs

---

## Development Setup

### Backend (Go API)

```bash
cd services/api

# Install dependencies
go mod download

# Run database
docker-compose up -d postgres redis minio

# Set environment
export DATABASE_URL="postgres://npdms:npdms_secret_2024@localhost:5432/npdms?sslmode=disable"
export REDIS_URL="redis://localhost:6379"
export JWT_SECRET="dev_jwt_secret_key"

# Run migrations
make migrate-up

# Run API
go run main.go
```

### Frontend (Next.js PWA)

```bash
cd ui/web

# Install dependencies
npm install

# Set environment
echo "NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1" > .env.local

# Run development server
npm run dev

# Build for production
npm run build
npm start
```

### ML Services

```bash
cd services/ml

# FIR Classifier
cd fir_classifier
pip install -r requirements.txt
python app.py

# Semantic Search
cd semantic_search
pip install -r requirements.txt
python app.py

# Crime Prediction
cd crime_prediction
pip install -r requirements.txt
python app.py
```

---

## Production Deployment

### 1. Pre-Deployment Checklist

- [ ] Change all default passwords in `.env`
- [ ] Generate strong JWT secret (min 32 characters)
- [ ] Configure SSL/TLS certificates
- [ ] Set up firewall rules
- [ ] Configure backup strategy
- [ ] Set up monitoring (Prometheus + Grafana)
- [ ] Configure log aggregation (Loki)

### 2. Build Production Images

```bash
# Build API
cd services/api
docker build -t npdms-api:latest .

# Build ML services
cd ../ml
docker build --target fir_classifier -t npdms-ml-fir-classifier:latest .
docker build --target semantic_search -t npdms-ml-semantic-search:latest .
docker build --target crime_prediction -t npdms-ml-crime-prediction:latest .

# Build frontend
cd ../../ui/web
docker build -t npdms-web:latest .
```

### 3. Deploy Stack

```bash
# Start infrastructure
docker-compose up -d postgres redis minio

# Wait for health checks
sleep 30

# Start API
docker-compose --profile full up -d api

# Start ML services
docker-compose --profile ml up -d

# Start frontend
docker-compose up -d web
```

### 4. Verify Deployment

```bash
# Check all services
docker-compose ps

# Check logs
docker-compose logs -f

# Health checks
curl http://localhost:8080/health
curl http://localhost:8001/health
curl http://localhost:8002/health
curl http://localhost:8003/health

# Test API
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "admin123"}'
```

---

## ML Services

### FIR Classifier

**Purpose**: Automatically categorize FIRs and suggest IPC sections

**Endpoints**:
- `POST /classify` - Classify single FIR
- `POST /batch-classify` - Batch classification
- `GET /categories` - List all categories
- `GET /ipc-sections/{category}` - Get IPC sections

**Usage**:
```bash
curl -X POST http://localhost:8001/classify \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Laptop and mobile phone stolen from parked vehicle",
    "title": "Theft from vehicle"
  }'
```

**Response**:
```json
{
  "category": "PROPERTY",
  "confidence": 0.89,
  "suggested_ipc_sections": [
    {"section": "IPC 379", "description": "Theft", "confidence": 0.85},
    {"section": "IPC 411", "description": "Dishonestly receiving stolen property", "confidence": 0.75}
  ],
  "method": "rule-based"
}
```

### Semantic Search

**Purpose**: Find similar FIRs using AI-powered semantic search

**Endpoints**:
- `POST /search` - Search similar FIRs
- `POST /index/add` - Add FIR to index
- `POST /index/batch-add` - Bulk indexing
- `GET /index/stats` - Index statistics

**Usage**:
```bash
# Search
curl -X POST http://localhost:8002/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "chain snatching by motorcycle riders",
    "top_k": 5,
    "threshold": 0.5
  }'

# Add to index (happens automatically via Go API)
curl -X POST http://localhost:8002/index/add \
  -H "Content-Type: application/json" \
  -d '{
    "fir_id": "123e4567-e89b-12d3-a456-426614174000",
    "fir_number": "KOR/2024/00123",
    "title": "Chain snatching",
    "description": "Gold chain snatched by two men on motorcycle",
    "category": "PROPERTY",
    "status": "UNDER_INVESTIGATION",
    "registered_at": "2024-01-15T10:30:00Z"
  }'
```

### Crime Prediction

**Purpose**: Predict crime patterns and identify hotspots

**Endpoints**:
- `POST /train` - Train model with historical data
- `POST /predict` - Get predictions
- `POST /hotspots` - Identify hotspots
- `GET /stats` - Crime statistics

**Usage**:
```bash
# Get predictions
curl http://localhost:8003/predict?forecast_days=7

# Get hotspots
curl http://localhost:8003/hotspots?hours=24&top_k=10
```

---

## Backup & Restore

### Database Backup

```bash
# Daily backup (add to cron: 0 2 * * *)
docker exec npdms-postgres pg_dump -U npdms npdms | \
  gzip > /backup/npdms_$(date +%Y%m%d).sql.gz

# Restore
gunzip < /backup/npdms_20240115.sql.gz | \
  docker exec -i npdms-postgres psql -U npdms -d npdms
```

### MinIO Backup

```bash
# Sync to external storage (cron: 0 3 * * *)
docker exec npdms-minio mc mirror local/npdms /backup/minio/

# Restore
docker exec npdms-minio mc mirror /backup/minio/ local/npdms
```

### Full System Backup

```bash
#!/bin/bash
# backup.sh - Full system backup

BACKUP_DIR="/backup/npdms_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$BACKUP_DIR"

# Database
docker exec npdms-postgres pg_dump -U npdms npdms | \
  gzip > "$BACKUP_DIR/database.sql.gz"

# MinIO (evidence files)
docker exec npdms-minio mc mirror local/npdms "$BACKUP_DIR/minio/"

# ML models and indexes
docker cp npdms-ml-semantic-search:/data "$BACKUP_DIR/ml_data/"

# Configuration
cp .env "$BACKUP_DIR/"
cp docker-compose.yml "$BACKUP_DIR/"

# Create archive
tar -czf "$BACKUP_DIR.tar.gz" "$BACKUP_DIR"
rm -rf "$BACKUP_DIR"

echo "Backup complete: $BACKUP_DIR.tar.gz"
```

---

## Monitoring

### Health Checks

```bash
# All services health
curl http://localhost:8080/health      # API
curl http://localhost:8001/health      # FIR Classifier
curl http://localhost:8002/health      # Semantic Search
curl http://localhost:8003/health      # Crime Prediction

# Database connection
docker-compose exec postgres pg_isready -U npdms

# Redis connection
docker-compose exec redis redis-cli ping
```

### Logs

```bash
# View all logs
docker-compose logs -f

# Specific service
docker-compose logs -f api
docker-compose logs -f ml-fir-classifier

# Last 100 lines
docker-compose logs --tail=100 api
```

### Performance Monitoring

```bash
# System resources
docker stats

# Database stats
docker-compose exec postgres psql -U npdms -d npdms -c "
  SELECT schemaname, tablename, n_live_tup
  FROM pg_stat_user_tables
  ORDER BY n_live_tup DESC;"

# Cache stats
docker-compose exec redis redis-cli INFO stats
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check logs
docker-compose logs <service-name>

# Check if port is in use
lsof -i :8080  # API port
lsof -i :5432  # PostgreSQL port

# Restart service
docker-compose restart <service-name>

# Rebuild service
docker-compose up -d --build <service-name>
```

### Database Connection Issues

```bash
# Check PostgreSQL is running
docker-compose ps postgres

# Test connection
docker-compose exec postgres psql -U npdms -d npdms -c "SELECT 1;"

# Reset database (CAUTION: Data loss!)
docker-compose down -v
docker-compose up -d postgres
# Run migrations again
```

### ML Service Errors

```bash
# Check ML service logs
docker-compose logs ml-fir-classifier

# Common issues:
# 1. Out of memory - increase Docker memory limit
# 2. Model download failed - check internet connection
# 3. Port conflict - change port in docker-compose.yml

# Rebuild ML services
docker-compose down ml-fir-classifier ml-semantic-search ml-crime-prediction
docker-compose up -d --build ml-fir-classifier ml-semantic-search ml-crime-prediction
```

### PWA Not Working

```bash
# Check service worker registration
# Open browser console and check for errors

# Clear service worker
# In browser DevTools > Application > Service Workers > Unregister

# Clear cache
# In browser DevTools > Application > Cache Storage > Delete

# Rebuild frontend
cd ui/web
npm run build
npm start
```

### Sync Queue Issues

```bash
# Check queue stats via API
curl http://localhost:8080/api/v1/sync/stats \
  -H "Authorization: Bearer <token>"

# Clear failed queue items (in browser console)
import { queueManager } from '@/lib/sync/queue-manager';
await queueManager.clearFailed();

# Retry failed items
await queueManager.retryFailed();
```

---

## Security Best Practices

1. **Change Default Passwords**: Update all passwords in `.env`
2. **Enable HTTPS**: Use reverse proxy (nginx/Caddy) with SSL
3. **Firewall**: Only expose necessary ports
4. **JWT Rotation**: Rotate JWT secret periodically
5. **Database**: Use strong password, enable SSL
6. **Backups**: Encrypt backup files
7. **Audit Logs**: Monitor and review regularly
8. **Updates**: Keep Docker images and dependencies updated

---

## Support

For issues and questions:
- GitHub Issues: <repository-url>/issues
- Documentation: See `/docs` folder
- API Documentation: http://localhost:8080/docs

---

**Last Updated**: 2026-01-05
