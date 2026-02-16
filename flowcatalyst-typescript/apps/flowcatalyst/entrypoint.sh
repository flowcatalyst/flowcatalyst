#!/bin/sh
set -e

# Construct DATABASE_URL from separate components.
# ECS injects DB_PASSWORD from Secrets Manager as a plain env var at runtime,
# so we assemble the full postgres:// connection string here.
if [ -z "$DATABASE_URL" ] && [ -n "$DB_HOST" ]; then
  export DATABASE_URL="postgres://${DB_USER:-inhance_admin}:${DB_PASSWORD}@${DB_HOST}/${DB_NAME:-flowcatalyst}?sslmode=require&ssl=true&sslrootcert=/certs/global-bundle.pem"
fi

exec node dist/index.js "$@"
