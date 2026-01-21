# Message Router Authentication

The Message Router supports optional authentication for standalone deployments. Authentication can be enabled in opt-in mode with support for both BasicAuth and OIDC.

## Features

- **Opt-in Authentication**: Authentication is disabled by default. Enable it only when needed.
- **BasicAuth Support**: Simple username/password authentication without requiring a database.
- **OIDC Support**: Integrates with OpenID Connect providers (Keycloak, Auth0, Okta, etc.).
- **Protected Endpoints**: Automatically secures sensitive monitoring and configuration endpoints.
- **Health Checks Always Open**: K8s probes (`/health/*`) are never protected.

## Configuration

### Enable/Disable Authentication

Set in `application.properties`:

```properties
# Enable authentication
authentication.enabled=true

# Set authentication mode: NONE, BASIC, or OIDC
authentication.mode=BASIC
```

Or via environment variables:

```bash
export AUTHENTICATION_ENABLED=true
export AUTHENTICATION_MODE=BASIC
```

## BasicAuth Configuration

BasicAuth allows a single username/password pair for all users. Ideal for internal tools and small deployments.

### Setup

1. Set authentication mode to BASIC:

```properties
authentication.enabled=true
authentication.mode=BASIC
authentication.basic-username=${AUTH_BASIC_USERNAME:}
authentication.basic-password=${AUTH_BASIC_PASSWORD:}
```

2. Set credentials via environment variables (recommended for deployments):

```bash
export AUTHENTICATION_ENABLED=true
export AUTHENTICATION_MODE=BASIC
export AUTH_BASIC_USERNAME=admin
export AUTH_BASIC_PASSWORD=secure-password-here
```

### Usage

**Dashboard Access (Auto-Redirect):**
- Navigate to `/dashboard.html` (always accessible)
- If authentication is disabled: Dashboard loads immediately
- If authentication is enabled: Login modal appears automatically
- Enter your credentials and click "Login"
- Credentials are stored in browser localStorage (base64 encoded)
- Click "Logout" to clear stored credentials and return to login

**API Access:**
- Include Authorization header in requests:

```bash
curl -H "Authorization: Basic $(echo -n 'admin:secure-password-here' | base64)" \
  http://localhost:8080/monitoring/health

# Example response:
# {"status": "HEALTHY", ...}
```

**Dashboard Login:**
```javascript
// JavaScript example
const username = "admin";
const password = "secure-password-here";
const encoded = btoa(`${username}:${password}`);

fetch('/monitoring/health', {
  headers: { 'Authorization': `Basic ${encoded}` }
})
.then(r => r.json())
.then(data => console.log(data));
```

## OIDC Configuration

OIDC integrates with external identity providers like Keycloak, Auth0, Okta, etc.

### Setup

1. Set authentication mode to OIDC:

```properties
authentication.enabled=true
authentication.mode=OIDC
quarkus.oidc.enabled=true
quarkus.oidc.auth-server-url=${OIDC_AUTH_SERVER_URL:}
quarkus.oidc.client-id=${OIDC_CLIENT_ID:}
quarkus.oidc.credentials.secret=${OIDC_CLIENT_SECRET:}
quarkus.oidc.application-type=web-app
quarkus.oidc.token.principal-claim=preferred_username
```

2. Set credentials via environment variables:

```bash
export AUTHENTICATION_ENABLED=true
export AUTHENTICATION_MODE=OIDC
export OIDC_AUTH_SERVER_URL=https://keycloak.example.com/realms/flowcatalyst
export OIDC_CLIENT_ID=message-router
export OIDC_CLIENT_SECRET=your-client-secret
```

### Usage

**Dashboard Access:**
- Navigate to `/dashboard.html`
- Browser redirects to OIDC provider login page
- After authentication, redirected back to dashboard
- Token is automatically managed by Quarkus OIDC extension

**API Access:**
- Obtain token from OIDC provider
- Include Authorization header with Bearer token:

```bash
# Get token from OIDC provider (example with Keycloak)
export TOKEN=$(curl -s -X POST \
  http://keycloak.example.com/realms/flowcatalyst/protocol/openid-connect/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=message-router" \
  -d "client_secret=your-client-secret" \
  -d "grant_type=client_credentials" \
  | jq -r '.access_token')

# Use token in API request
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/monitoring/health
```

## Protected Endpoints

The following endpoints require authentication when enabled:

- `/monitoring/*` - All monitoring and metrics endpoints
- `/api/seed/*` - Message seeding endpoints
- `/api/config` - Configuration endpoint

### Always Open Endpoints

These endpoints are never protected:

- `/dashboard.html` - Dashboard UI (auto-redirects to login modal if auth required)
- `/health` - Kubernetes liveness probe
- `/health/live` - Kubernetes liveness probe
- `/health/ready` - Kubernetes readiness probe
- `/health/startup` - Kubernetes startup probe
- `/q/health*` - Quarkus health endpoints

## Implementation Details

### Authentication Filter

The `AuthenticationFilter` class (`tech.flowcatalyst.messagerouter.security.AuthenticationFilter`) intercepts all requests and:

1. Checks if authentication is enabled
2. Skips health check endpoints
3. Validates BasicAuth credentials or OIDC tokens
4. Returns 401 Unauthorized if credentials are invalid

### BasicAuth Provider

The `BasicAuthIdentityProvider` class validates credentials against configured username/password. It:

1. Decodes Base64 authorization headers
2. Compares provided credentials with configured values
3. Creates security identity with "authenticated" role

### Configuration Classes

- `AuthenticationConfig` - Reads and provides authentication settings
- `Protected` - Annotation marking endpoints that require authentication
- `BasicAuthRequest` - Carries username/password for authentication

## Security Considerations

### BasicAuth

- **HTTPS Only**: Always use HTTPS in production (BasicAuth credentials are Base64 encoded, not encrypted)
- **Environment Variables**: Set credentials via environment variables, never hardcode in properties files
- **Password Rotation**: Update credentials by restarting with new environment variables
- **No Database**: Suitable for single user/internal tools only

### OIDC

- **Standard Protocol**: Follows OAuth 2.0 and OpenID Connect standards
- **Token Validation**: Tokens are validated against OIDC provider's public keys
- **Encryption**: All OIDC communication uses HTTPS (enforced)
- **Multi-User**: Supports multiple users and roles

## Docker/Kubernetes Deployment

### BasicAuth in Docker

```dockerfile
FROM quarkus/java21-jvm
COPY build/quarkus-app /app
WORKDIR /app

ENTRYPOINT ["java", "-jar", "quarkus-run.jar"]
```

```bash
docker run -e AUTHENTICATION_ENABLED=true \
  -e AUTHENTICATION_MODE=BASIC \
  -e AUTH_BASIC_USERNAME=admin \
  -e AUTH_BASIC_PASSWORD=secret \
  -p 8080:8080 \
  message-router:latest
```

### Kubernetes with BasicAuth

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: message-router-auth
type: Opaque
stringData:
  username: admin
  password: supersecret123

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: message-router
spec:
  template:
    spec:
      containers:
      - name: message-router
        env:
        - name: AUTHENTICATION_ENABLED
          value: "true"
        - name: AUTHENTICATION_MODE
          value: "BASIC"
        - name: AUTH_BASIC_USERNAME
          valueFrom:
            secretKeyRef:
              name: message-router-auth
              key: username
        - name: AUTH_BASIC_PASSWORD
          valueFrom:
            secretKeyRef:
              name: message-router-auth
              key: password
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
```

### Kubernetes with OIDC

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: message-router-oidc
type: Opaque
stringData:
  auth-server-url: https://keycloak.example.com/realms/flowcatalyst
  client-id: message-router
  client-secret: supersecret456

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: message-router
spec:
  template:
    spec:
      containers:
      - name: message-router
        env:
        - name: AUTHENTICATION_ENABLED
          value: "true"
        - name: AUTHENTICATION_MODE
          value: "OIDC"
        - name: OIDC_AUTH_SERVER_URL
          valueFrom:
            secretKeyRef:
              name: message-router-oidc
              key: auth-server-url
        - name: OIDC_CLIENT_ID
          valueFrom:
            secretKeyRef:
              name: message-router-oidc
              key: client-id
        - name: OIDC_CLIENT_SECRET
          valueFrom:
            secretKeyRef:
              name: message-router-oidc
              key: client-secret
```

## Troubleshooting

### "401 Unauthorized" Error

- Verify `AUTHENTICATION_ENABLED=true` and `AUTHENTICATION_MODE=BASIC` (or `OIDC`)
- For BasicAuth: Check `AUTH_BASIC_USERNAME` and `AUTH_BASIC_PASSWORD` are set
- For OIDC: Verify OIDC provider is reachable and token is valid
- Check application logs for authentication errors

### Dashboard Login Modal Not Appearing

- Authentication may not be enabled. Check application properties:
  ```bash
  curl http://localhost:8080/monitoring/health
  ```
  If this returns 200 without auth, authentication is disabled.

### "Invalid credentials" Error

- For BasicAuth: Verify username and password are correct
- Check environment variables are properly set:
  ```bash
  echo $AUTH_BASIC_USERNAME
  echo $AUTH_BASIC_PASSWORD
  ```

### OIDC Token Errors

- Verify OIDC server is reachable
- Check client-id and client-secret match OIDC provider configuration
- Verify redirect URIs are configured in OIDC provider

## Disabling Authentication

To disable authentication, set:

```properties
authentication.enabled=false
```

All endpoints become open to public access. This is the default configuration.

## API Documentation

Protected endpoints require authentication headers. Use the OpenAPI docs at `/q/openapi.json` for full endpoint documentation.
