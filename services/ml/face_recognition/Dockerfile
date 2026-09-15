# NPDMS face recognition service, for the edge server (CPU).
#   ./download_models.sh            # once, on a machine with internet access
#   docker build -t npdms-face-recognition .
#   docker run --rm -p 127.0.0.1:18203:8000 -e FR_SERVICE_TOKEN=... npdms-face-recognition
# Keep the port on the private network: only the NPDMS API should reach it.
FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 FR_MODEL_DIR=/app/models OMP_NUM_THREADS=2
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY fr_service ./fr_service
# Model files are copied in, not downloaded at build or run time; the service
# verifies their SHA-256 on start and refuses to run with a mismatch.
COPY models/*.onnx ./models/

RUN useradd --system --uid 10001 fr && chown -R fr /app
USER fr
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s CMD python -c "import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:8000/health').status==200 else 1)"
# One worker per ~2 cores; each worker loads its own copy of the models (~45 MB).
CMD ["uvicorn", "fr_service.app:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "2"]
