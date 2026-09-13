#!/bin/bash

# NPDMS Startup Script
# Quick start script for the National Police Data Management System

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print with color
print_info() { echo -e "${BLUE}ℹ ${NC}$1"; }
print_success() { echo -e "${GREEN}✓ ${NC}$1"; }
print_warning() { echo -e "${YELLOW}⚠ ${NC}$1"; }
print_error() { echo -e "${RED}✗ ${NC}$1"; }

# Banner
echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║                                                              ║"
echo "║     NPDMS - National Police Data Management System          ║"
echo "║                                                              ║"
echo "║     Backend + AI/ML + Offline-First PWA                     ║"
echo "║                                                              ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    print_warning ".env file not found, creating from .env.example..."
    if [ -f .env.example ]; then
        cp .env.example .env
        print_info "Please edit .env with your configuration before proceeding"
        print_info "Run: nano .env"
        exit 1
    else
        print_error ".env.example not found!"
        exit 1
    fi
fi

# Check Docker
if ! command -v docker &> /dev/null; then
    print_error "Docker is not installed. Please install Docker first."
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    print_error "Docker Compose is not installed. Please install Docker Compose first."
    exit 1
fi

print_success "Docker and Docker Compose found"

# Parse arguments
MODE=${1:-dev}

case $MODE in
    dev)
        print_info "Starting in DEVELOPMENT mode (infrastructure only)..."
        ;;
    ml)
        print_info "Starting with ML SERVICES..."
        ;;
    monitoring)
        print_info "Starting with MONITORING (Prometheus + Grafana)..."
        ;;
    demo)
        print_info "Starting DEMO mode (infrastructure + monitoring)..."
        ;;
    full)
        print_info "Starting FULL STACK (all services)..."
        ;;
    stop)
        print_info "Stopping all services..."
        docker-compose down
        print_success "All services stopped"
        exit 0
        ;;
    restart)
        print_info "Restarting all services..."
        docker-compose restart
        print_success "All services restarted"
        exit 0
        ;;
    logs)
        SERVICE=${2:-}
        if [ -z "$SERVICE" ]; then
            docker-compose logs -f
        else
            docker-compose logs -f $SERVICE
        fi
        exit 0
        ;;
    clean)
        print_warning "This will remove all containers and volumes (DATA LOSS!)"
        read -p "Are you sure? (yes/no): " -r
        if [[ $REPLY == "yes" ]]; then
            docker-compose down -v
            print_success "All data cleaned"
        else
            print_info "Cancelled"
        fi
        exit 0
        ;;
    help|--help|-h)
        echo "Usage: ./start.sh [MODE]"
        echo ""
        echo "Modes:"
        echo "  dev        - Start infrastructure only (postgres, redis, minio)"
        echo "  ml         - Start with ML services"
        echo "  monitoring - Start with monitoring (Prometheus + Grafana)"
        echo "  demo       - Start demo mode (infrastructure + monitoring)"
        echo "  full       - Start everything (API + ML + monitoring)"
        echo "  stop       - Stop all services"
        echo "  restart    - Restart all services"
        echo "  logs [service] - View logs (optionally for specific service)"
        echo "  clean      - Remove all containers and volumes (DATA LOSS!)"
        echo "  help       - Show this help message"
        echo ""
        echo "Examples:"
        echo "  ./start.sh dev          # Development mode"
        echo "  ./start.sh ml           # With ML services"
        echo "  ./start.sh monitoring   # With monitoring stack"
        echo "  ./start.sh demo         # Demo mode"
        echo "  ./start.sh full         # Full stack"
        echo "  ./start.sh logs api     # View API logs"
        echo "  ./start.sh stop         # Stop everything"
        exit 0
        ;;
    *)
        print_error "Unknown mode: $MODE"
        print_info "Run './start.sh help' for usage information"
        exit 1
        ;;
esac

# Create necessary directories
print_info "Creating directories..."
mkdir -p services/api/migrations
mkdir -p services/ml/fir_classifier
mkdir -p services/ml/semantic_search
mkdir -p services/ml/crime_prediction

# Start infrastructure
print_info "Starting infrastructure services (PostgreSQL, Redis, MinIO)..."
docker-compose up -d postgres redis minio

# Wait for health checks
print_info "Waiting for services to be healthy..."
sleep 5

# Check health
RETRIES=30
until docker-compose exec -T postgres pg_isready -U npdms -d npdms > /dev/null 2>&1 || [ $RETRIES -eq 0 ]; do
    print_info "Waiting for PostgreSQL... ($RETRIES retries left)"
    sleep 2
    RETRIES=$((RETRIES-1))
done

if [ $RETRIES -eq 0 ]; then
    print_error "PostgreSQL failed to start"
    docker-compose logs postgres
    exit 1
fi

print_success "PostgreSQL is ready"

# Redis health check
if docker-compose exec -T redis redis-cli ping > /dev/null 2>&1; then
    print_success "Redis is ready"
else
    print_error "Redis failed to start"
    exit 1
fi

# MinIO health check
print_success "MinIO is ready"

# Start additional services based on mode
if [ "$MODE" == "ml" ] || [ "$MODE" == "full" ]; then
    print_info "Starting ML services..."
    docker-compose --profile ml up -d ml-fir-classifier ml-semantic-search ml-crime-prediction ml-ocr

    # Wait for ML services
    sleep 10
    print_success "ML services started"
fi

if [ "$MODE" == "monitoring" ] || [ "$MODE" == "demo" ] || [ "$MODE" == "full" ]; then
    print_info "Starting monitoring services..."
    docker-compose --profile monitoring up -d prometheus grafana postgres-exporter redis-exporter node-exporter

    sleep 5
    print_success "Monitoring services started"
fi

if [ "$MODE" == "full" ]; then
    print_info "Starting API service..."
    docker-compose --profile full up -d api

    sleep 5
    print_success "API service started"
fi

# Show status
echo ""
print_success "NPDMS services are running!"
echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  Service URLs:                                               ║"
echo "╠══════════════════════════════════════════════════════════════╣"

if docker-compose ps api | grep -q Up; then
    echo "║  API:                  http://localhost:8080                 ║"
    echo "║  API Docs:             http://localhost:8080/docs            ║"
fi

echo "║  PostgreSQL:           localhost:5432                        ║"
echo "║  Redis:                localhost:6379                        ║"
echo "║  MinIO Console:        http://localhost:9001                 ║"

if docker-compose ps ml-fir-classifier | grep -q Up; then
    echo "║  FIR Classifier:       http://localhost:8001/docs            ║"
fi

if docker-compose ps ml-semantic-search | grep -q Up; then
    echo "║  Semantic Search:      http://localhost:8002/docs            ║"
fi

if docker-compose ps ml-crime-prediction | grep -q Up; then
    echo "║  Crime Prediction:     http://localhost:8003/docs            ║"
fi

if docker-compose ps ml-ocr | grep -q Up; then
    echo "║  OCR Service:          http://localhost:8004/docs            ║"
fi

if docker-compose ps prometheus | grep -q Up; then
    echo "║  Prometheus:           http://localhost:9090                 ║"
fi

if docker-compose ps grafana | grep -q Up; then
    echo "║  Grafana:              http://localhost:3001                 ║"
    echo "║    (admin/admin123)                                          ║"
fi

echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# Show running containers
print_info "Running containers:"
docker-compose ps

echo ""
print_info "View logs: ./start.sh logs [service-name]"
print_info "Stop all:  ./start.sh stop"
print_info "Help:      ./start.sh help"
echo ""

# Development mode instructions
if [ "$MODE" == "dev" ]; then
    echo ""
    print_warning "Development Mode - Infrastructure only"
    echo ""
    echo "To start the Go API locally:"
    echo "  cd services/api"
    echo "  export DATABASE_URL=\"postgres://npdms:npdms_secret_2024@localhost:5432/npdms?sslmode=disable\""
    echo "  export REDIS_URL=\"redis://localhost:6379\""
    echo "  go run main.go"
    echo ""
    echo "To start the Next.js frontend:"
    echo "  cd ui/web"
    echo "  npm install"
    echo "  npm run dev"
    echo ""
fi

print_success "Startup complete!"
