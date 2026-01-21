#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
ROOT_DIR="$(dirname "$(dirname "$PROJECT_DIR")")"
DATA_DIR="${PROJECT_DIR}/data"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}"
echo "  _____ _                ____      _        _           _   "
echo " |  ___| | _____      __/ ___|__ _| |_ __ _| |_   _ ___| |_ "
echo " | |_  | |/ _ \ \ /\ / / |   / _\` | __/ _\` | | | | / __| __|"
echo " |  _| | | (_) \ V  V /| |__| (_| | || (_| | | |_| \__ \ |_ "
echo " |_|   |_|\___/ \_/\_/  \____\__,_|\__\__,_|_|\__, |___/\__|"
echo "                                              |___/         "
echo -e "${NC}"
echo "Developer Build - All Services in One"
echo "======================================"

# Load environment if exists
if [ -f "${PROJECT_DIR}/.env" ]; then
    echo -e "${BLUE}Loading environment from .env${NC}"
    set -a
    source "${PROJECT_DIR}/.env"
    set +a
fi

# Create data directory for SQLite
mkdir -p "$DATA_DIR"

# Check if using external MongoDB
if [ "${EXTERNAL_MONGODB:-false}" = "true" ]; then
    echo -e "\n${BLUE}Using external MongoDB: ${MONGODB_URI:-not set}${NC}"
    echo -e "${YELLOW}Note: External MongoDB must be a replica set for change streams${NC}"
else
    # Check if Docker is running
    if ! docker info > /dev/null 2>&1; then
        echo -e "${YELLOW}Warning: Docker is not running. MongoDB will not be started.${NC}"
        echo "Options:"
        echo "  1. Start Docker and re-run this script"
        echo "  2. Set EXTERNAL_MONGODB=true and MONGODB_URI to use your own MongoDB"
        exit 1
    else
        # Start MongoDB
        echo -e "\n${YELLOW}Starting MongoDB via Docker...${NC}"
        docker compose -f "${PROJECT_DIR}/docker-compose.yml" up -d mongo

        # Wait for MongoDB to be ready
        echo "Waiting for MongoDB replica set to initialize..."
        for i in {1..30}; do
            if docker exec fc-dev-mongo mongosh --quiet --eval "rs.status().ok" 2>/dev/null | grep -q "1"; then
                echo -e "${GREEN}MongoDB is ready!${NC}"
                break
            fi
            sleep 1
            echo -n "."
        done
        echo ""
    fi
fi

# Check if native executable exists
NATIVE_EXE="${PROJECT_DIR}/build/flowcatalyst-dev-build-1.0.0-SNAPSHOT-runner"
if [ -f "$NATIVE_EXE" ]; then
    echo -e "\n${YELLOW}Starting FlowCatalyst (native executable)...${NC}"
    cd "$PROJECT_DIR"
    exec "$NATIVE_EXE"
else
    echo -e "\n${YELLOW}Starting FlowCatalyst (JVM mode via Gradle)...${NC}"
    echo -e "${BLUE}Tip: Build native with:${NC}"
    echo "  ./gradlew :core:flowcatalyst-dev-build:build -Dquarkus.native.enabled=true"
    echo ""
    cd "$ROOT_DIR"
    exec ./gradlew :core:flowcatalyst-dev-build:quarkusDev
fi
