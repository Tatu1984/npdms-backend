# Multi-stage Dockerfile for NPDMS ML Services
# Supports: fir_classifier, semantic_search, crime_prediction, ocr_service

FROM python:3.10-slim as base

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    && rm -rf /var/lib/apt/lists/*

# ============================================================================
# FIR Classifier Image
# ============================================================================
FROM base as fir_classifier

COPY fir_classifier/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY fir_classifier/app.py .

# Download model at build time (optional, can be done at runtime)
RUN python -c "from transformers import DistilBertTokenizer; DistilBertTokenizer.from_pretrained('distilbert-base-uncased')"

EXPOSE 8001

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8001"]

# ============================================================================
# Semantic Search Image
# ============================================================================
FROM base as semantic_search

COPY semantic_search/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY semantic_search/app.py .

# Download sentence transformer model at build time
RUN python -c "from sentence_transformers import SentenceTransformer; SentenceTransformer('sentence-transformers/all-MiniLM-L6-v2')"

# Create data directory for FAISS index
RUN mkdir -p /data

VOLUME ["/data"]

EXPOSE 8002

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8002"]

# ============================================================================
# Crime Prediction Image
# ============================================================================
FROM base as crime_prediction

COPY crime_prediction/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY crime_prediction/app.py .

EXPOSE 8003

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8003"]

# ============================================================================
# OCR Service Image
# ============================================================================
FROM base as ocr_service

# Install additional system dependencies for OCR
RUN apt-get update && apt-get install -y \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender-dev \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

COPY ocr_service/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY ocr_service/app.py .

# Download OCR models at build time (English + Hindi)
RUN python -c "import easyocr; easyocr.Reader(['en', 'hi'], gpu=False, download_enabled=True)"

EXPOSE 8004

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8004"]
