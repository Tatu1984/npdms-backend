#!/bin/bash

# NPDMS Demo Data Seed Script Wrapper

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║                                                              ║"
echo "║     NPDMS Demo Data Seed Script                             ║"
echo "║                                                              ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# Check if PostgreSQL is running
if ! docker-compose ps postgres | grep -q Up; then
    echo "✗ PostgreSQL is not running. Please start it first:"
    echo "  ./start.sh dev"
    exit 1
fi

echo "✓ PostgreSQL is running"
echo ""

# Check if Python 3 is installed
if ! command -v python3 &> /dev/null; then
    echo "✗ Python 3 is not installed"
    exit 1
fi

echo "✓ Python 3 found"
echo ""

# Install dependencies if needed
if ! python3 -c "import psycopg2" 2>/dev/null; then
    echo "Installing Python dependencies..."
    pip3 install -r "$SCRIPT_DIR/requirements.txt"
    echo ""
fi

echo "✓ Dependencies installed"
echo ""

# Run seed script
echo "Running seed script..."
echo ""

export DATABASE_URL="${DATABASE_URL:-postgres://npdms:npdms_secret_2024@localhost:5432/npdms?sslmode=disable}"

python3 "$SCRIPT_DIR/seed-demo-data.py"

echo ""
echo "✓ Demo data seeded successfully!"
