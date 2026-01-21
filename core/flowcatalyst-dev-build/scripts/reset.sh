#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DATA_DIR="${PROJECT_DIR}/data"

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

# Load environment if exists
if [ -f "${PROJECT_DIR}/.env" ]; then
    set -a
    source "${PROJECT_DIR}/.env"
    set +a
fi

echo -e "${RED}FlowCatalyst Developer Build - Reset${NC}"
echo "======================================"
echo ""

if [ "${EXTERNAL_MONGODB:-false}" = "true" ]; then
    echo -e "${YELLOW}WARNING: This will delete SQLite queue databases.${NC}"
    echo -e "${BLUE}Note: Using external MongoDB - you must reset that separately.${NC}"
else
    echo -e "${YELLOW}WARNING: This will delete ALL data including:${NC}"
    echo "  - MongoDB data (all collections)"
    echo "  - SQLite queue databases"
fi

echo ""
read -p "Are you sure you want to continue? (y/N) " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo ""

    # Stop Docker containers (unless using external MongoDB)
    if [ "${EXTERNAL_MONGODB:-false}" = "true" ]; then
        echo -e "${BLUE}Using external MongoDB - skipping Docker cleanup${NC}"
    else
        echo -e "${YELLOW}Stopping containers and removing volumes...${NC}"
        if docker info > /dev/null 2>&1; then
            docker compose -f "${PROJECT_DIR}/docker-compose.yml" --profile tools down -v
        else
            echo "Docker not running, skipping container cleanup"
        fi
    fi

    # Remove SQLite databases
    echo -e "${YELLOW}Removing SQLite databases...${NC}"
    rm -f "$DATA_DIR"/*.db 2>/dev/null || true

    echo ""
    echo -e "${GREEN}Reset complete.${NC}"
    echo "Run ./scripts/start.sh to start fresh."
else
    echo -e "${YELLOW}Cancelled.${NC}"
fi
