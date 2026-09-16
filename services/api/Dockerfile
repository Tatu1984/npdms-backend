# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application.
# TARGETARCH is supplied automatically by BuildKit and names the architecture
# the image is actually being built for. Hardcoding amd64 here produced an
# image tagged arm64 that carried an x86-64 binary, which then only ran where
# emulation happened to be available. The default keeps the old behaviour for
# the legacy builder, which does not populate the argument.
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o /app/api \
    .

# Final stage
FROM alpine:3.19

# Install runtime dependencies.
#
# poppler-utils gives pdftotext (a PDF's own text layer) and pdftoppm (turning
# a scanned page into an image). tesseract-ocr reads those images.
RUN apk add --no-cache ca-certificates tzdata poppler-utils tesseract-ocr curl

# The language models for the languages of India.
#
# Alpine packages only some of them — Marathi, Gujarati, Punjabi, Odia, Assamese
# and Urdu are not in its repository — so they are fetched from the tessdata
# project itself, which is where Alpine gets them too. These are the "best"
# (LSTM) models rather than the "fast" ones: on a Bengali page the best model
# read 38% of words correctly against the fast model's 9%, which is the
# difference between a scanned circular being findable and not. Apache-2.0,
# from https://github.com/tesseract-ocr/tessdata_best.
#
# About 150 MB, and no GPU is needed to use them.
# Pinned so an image built next year reads pages the same way as this one.
# tessdata_best at e12c65a (9 March 2024); osd comes from the tessdata
# repository, which carries the script-detection model.
ARG TESSDATA_BEST_COMMIT=e12c65a915945e4c28e237a9b52bc4a8f39a0cec
ARG TESSDATA_COMMIT=ced78752cc61322fb554c280d13360b35b8684e4
RUN set -eux; \
    mkdir -p /usr/share/tessdata; \
    for lang in ben hin mar tam tel kan mal guj pan ori asm urd san nep eng; do \
        curl -fsSL -o "/usr/share/tessdata/$lang.traineddata" \
            "https://raw.githubusercontent.com/tesseract-ocr/tessdata_best/${TESSDATA_BEST_COMMIT}/$lang.traineddata"; \
    done; \
    curl -fsSL -o /usr/share/tessdata/osd.traineddata \
        "https://raw.githubusercontent.com/tesseract-ocr/tessdata/${TESSDATA_COMMIT}/osd.traineddata"; \
    apk del curl
ENV TESSDATA_PREFIX=/usr/share/tessdata

# Create non-root user
RUN addgroup -g 1001 -S npdms && \
    adduser -u 1001 -S npdms -G npdms

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/api .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Set ownership
RUN chown -R npdms:npdms /app

# Switch to non-root user
USER npdms

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./api"]
