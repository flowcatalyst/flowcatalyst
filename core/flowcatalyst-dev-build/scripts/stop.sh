#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

# Colors
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

echo -e "${YELLOW}Stopping FlowCatalyst Developer Build...${NC}"

# Stop Docker containers (unless using external MongoDB)
if [ "${EXTERNAL_MONGODB:-false}" = "true" ]; then
    echo -e "${BLUE}Using external MongoDB - no Docker containers to stop${NC}"
else
    if docker info > /dev/null 2>&1; then
        docker compose -f "${PROJECT_DIR}/docker-compose.yml" down
    else
        echo "Docker not running, skipping container cleanup"
    fi
fi

echo -e "${GREEN}Stopped.${NC}"
