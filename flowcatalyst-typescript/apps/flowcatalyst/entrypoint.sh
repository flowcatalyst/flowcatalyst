#!/bin/sh
set -e

# Construct DATABASE_URL from separate components.
# ECS injects DB_PASSWORD from Secrets Manager as a plain env var at runtime,
# so we assemble the full postgres:// connection string here.
# The password is URL-encoded because RDS-generated passwords contain
# special characters (?#[]@: etc.) that break URL parsing.
if [ -z "$DATABASE_URL" ] && [ -n "$DB_HOST" ]; then
  ENCODED_PASSWORD=$(node -e "process.stdout.write(encodeURIComponent(process.env.DB_PASSWORD ?? ''))")
  export DATABASE_URL="postgres://${DB_USER:-inhance_admin}:${ENCODED_PASSWORD}@${DB_HOST}/${DB_NAME:-flowcatalyst}?ssl=true"
fi

# Trust the RDS CA bundle for SSL connections (postgres.js uses Node's TLS, not libpq)
if [ -f /certs/global-bundle.pem ]; then
  export NODE_EXTRA_CA_CERTS=/certs/global-bundle.pem
fi

# If the first arg is an executable (e.g. "node", "sh"), run it directly
# instead of routing through the CLI. This allows ECS command overrides
# like ["node", "-e", "..."] to work.
case "$1" in
  node|sh|bash) exec "$@" ;;
  *) exec node dist/index.js "$@" ;;
esac
