# Deployment Guide

This guide covers deployment options, configurations, and best practices for running FlowCatalyst in production.

## Deployment Options

### Option 1: Full-Stack (flowcatalyst-app)

All components in a single deployment:

```
┌─────────────────────────────────────┐
│         flowcatalyst-app            │
├─────────────────────────────────────┤
│  • Platform Services                │
│  • Message Router                   │
│  • Event Processor                  │
│  • Dispatch Scheduler               │
└─────────────────────────────────────┘
```

**Best for**: Development, small deployments, single-instance scenarios

```bash
java -jar flowcatalyst-app/build/quarkus-app/quarkus-run.jar
```

### Option 2: Microservices

Separate deployments for scaling:

```
┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│  Platform   │  │   Router    │  │   Event     │
│    API      │  │  (scaled)   │  │  Processor  │
└─────────────┘  └─────────────┘  └─────────────┘
```

**Best for**: Production, high-volume, independent scaling

### Option 3: Router-Only (flowcatalyst-router-app)

Lightweight stateless router:

```bash
java -jar flowcatalyst-router-app/build/quarkus-app/quarkus-run.jar
```

**Best for**: Edge deployments, dedicated routing infrastructure

## Docker Deployment

### Building Images

```bash
# Build with Jib
./gradlew :core:flowcatalyst-app:jibDockerBuild

# Or use quarkus container-image
./gradlew :core:flowcatalyst-app:build \
  -Dquarkus.container-image.build=true
```

### Docker Compose

```yaml
version: '3.8'
services:
  flowcatalyst:
    image: flowcatalyst/flowcatalyst-app:latest
    ports:
      - "8080:8080"
    environment:
      - QUARKUS_MONGODB_CONNECTION_STRING=mongodb://mongo:27017
      - QUARKUS_MONGODB_DATABASE=flowcatalyst
      - MESSAGE_ROUTER_QUEUE_TYPE=SQS
      - AWS_REGION=us-east-1
    depends_on:
      - mongo
      - redis

  mongo:
    image: mongo:7
    volumes:
      - mongo-data:/data/db

  redis:
    image: redis:7-alpine

volumes:
  mongo-data:
```

### Running

```bash
docker-compose up -d
```

## Kubernetes Deployment

### Deployment Manifest

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flowcatalyst
spec:
  replicas: 3
  selector:
    matchLabels:
      app: flowcatalyst
  template:
    metadata:
      labels:
        app: flowcatalyst
    spec:
      containers:
      - name: flowcatalyst
        image: flowcatalyst/flowcatalyst-app:latest
        ports:
        - containerPort: 8080
        env:
        - name: QUARKUS_MONGODB_CONNECTION_STRING
          valueFrom:
            secretKeyRef:
              name: flowcatalyst-secrets
              key: mongodb-uri
        - name: MESSAGE_ROUTER_QUEUE_TYPE
          value: "SQS"
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: flowcatalyst
spec:
  selector:
    app: flowcatalyst
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

### Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: flowcatalyst
spec:
  rules:
  - host: api.flowcatalyst.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: flowcatalyst
            port:
              number: 80
```

## Environment Configuration

### Required Variables

| Variable | Description |
|----------|-------------|
| `QUARKUS_MONGODB_CONNECTION_STRING` | MongoDB connection URI |
| `QUARKUS_MONGODB_DATABASE` | Database name |
| `MESSAGE_ROUTER_QUEUE_TYPE` | Queue backend (SQS/ACTIVEMQ/CHRONICLE) |

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MESSAGE_ROUTER_CONFIG_URL` | `http://localhost:8080/api/config` | Config endpoint |
| `FLOWCATALYST_AUTH_EXTERNAL_BASE_URL` | (none) | External URL for OAuth callbacks |
| `QUARKUS_HTTP_PORT` | `8080` | HTTP port |

### Queue-Specific

See [Queue Configuration Guide](../guides/queue-configuration.md) for details.

## Health Checks

### Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/health/live` | Liveness - app running |
| `/health/ready` | Readiness - ready for traffic |
| `/health/startup` | Startup - initialization complete |

### Kubernetes Configuration

```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 3
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
  timeoutSeconds: 5
  failureThreshold: 3
```

## Resource Requirements

### Minimum (Development)

- CPU: 500m
- Memory: 512Mi
- Disk: 1Gi (for logs)

### Recommended (Production)

- CPU: 2000m
- Memory: 2Gi
- Disk: 10Gi

### High Volume

- CPU: 4000m
- Memory: 4Gi
- Disk: 50Gi

## Security Considerations

### Network

1. **Use HTTPS** - Terminate TLS at load balancer or ingress
2. **Restrict access** - Use network policies
3. **Secure MongoDB** - Enable authentication, use TLS

### Secrets

1. **Use secret management** - AWS Secrets Manager, Vault, K8s secrets
2. **Rotate credentials** - Regular rotation schedule
3. **Audit access** - Log secret access

### Application

1. **Enable authentication** - Configure OIDC or internal auth
2. **Use service accounts** - For machine-to-machine
3. **Implement RBAC** - Fine-grained permissions

## See Also

- [Monitoring Guide](monitoring.md) - Observability setup
- [Scaling Guide](scaling.md) - Scaling strategies
- [Troubleshooting](troubleshooting.md) - Common issues
