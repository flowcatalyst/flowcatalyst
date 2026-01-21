#!/bin/sh
#
# Entrypoint script for flowcatalyst-message-router
# Ensures virtual thread scheduler has at least 2 carrier threads
# even on single-core containers to prevent thread starvation.
#

# Get available CPUs (works in containers with cgroup limits)
CPUS=$(nproc 2>/dev/null || echo 1)

# Virtual thread parallelism: at least 2, otherwise match CPU count
if [ "$CPUS" -lt 2 ]; then
    VT_PARALLELISM=2
else
    VT_PARALLELISM=$CPUS
fi

echo "Starting with $CPUS CPU(s), virtual thread parallelism: $VT_PARALLELISM"

exec java \
    -XX:+UseZGC \
    -Djdk.virtualThreadScheduler.parallelism=$VT_PARALLELISM \
    -Djdk.virtualThreadScheduler.maxPoolSize=$((VT_PARALLELISM * 256)) \
    -Dquarkus.http.host=0.0.0.0 \
    -jar /deployments/quarkus-run.jar \
    "$@"
