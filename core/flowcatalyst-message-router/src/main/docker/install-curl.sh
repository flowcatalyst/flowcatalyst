#!/bin/sh
# Install curl if not already present
if ! command -v curl &> /dev/null; then
    apk add --no-cache curl
fi
