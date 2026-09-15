# NPDMS vehicle detection and number-plate reading, for the edge server.
# CPU only. Models are fetched and digest-checked at build time, so the
# running container needs no internet access and sends nothing off premises.
#
#   docker build -t npdms-vehicle-detection services/ml/vehicle_detection
#   docker run -p 8010:8010 npdms-vehicle-detection
#
# On an air-gapped build host, build with --build-arg FETCH_MODELS=0 and mount
# a verified models directory: -v /srv/anpr-models:/app/models:ro. The service
# checks every file's SHA-256 at start and refuses to analyse if one differs.
FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 ANPR_MODEL_DIR=/app/models ANPR_THREADS=2
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY *.py ./
ARG FETCH_MODELS=1
RUN mkdir -p models && if [ "$FETCH_MODELS" = "1" ]; then python fetch_models.py; fi

RUN useradd --system --uid 10010 anpr && chown -R anpr /app
USER anpr

EXPOSE 8010
HEALTHCHECK --interval=30s --timeout=5s CMD python -c "import urllib.request,json,sys; sys.exit(0 if json.load(urllib.request.urlopen('http://127.0.0.1:8010/health'))['status']=='ok' else 1)"
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8010", "--workers", "1"]
