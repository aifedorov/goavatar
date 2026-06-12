# GoAvatar

Avatar service written in Go. It stores metadata in PostgreSQL, original images
and thumbnails in MinIO, and sends thumbnail processing jobs through RabbitMQ.

## Requirements

- Go 1.26
- Docker
- Docker Compose
- Kubernetes cluster for K8s deploy, for example Rancher Desktop
- `kubectl`
- `helm`
- `oapi-codegen` for OpenAPI model generation

## Run Locally With Docker Compose

Start application, dependencies, and observability stack:

```bash
docker-compose up --build
```

Application endpoints:

- API: `http://localhost:8080`
- MinIO console: `http://localhost:9001`
- RabbitMQ management UI: `http://localhost:15672`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000`
- Jaeger: `http://localhost:16686`

Default local credentials:

- MinIO: `minioadmin` / `minioadmin`
- RabbitMQ: `rabbitmq` / `rabbitmq`
- Grafana: `admin` / `admin`

Check service health:

```bash
curl http://localhost:8080/health
```

Upload avatar:

```bash
curl -X POST http://localhost:8080/api/v1/avatars \
  -H "X-User-ID: user-1" \
  -F "file=@scripts/image.png"
```

Stop local stack:

```bash
docker-compose down
```

Remove local volumes too:

```bash
docker-compose down -v
```

## Run Locally Without Docker Compose App Containers

Start dependencies:

```bash
docker-compose up -d postgres migrate minio minio-setup rabbitmq
```

Export local configuration:

```bash
export DATABASE_URI="postgres://goavatar:password@localhost:5432/goavatar?sslmode=disable"
export S3_ENDPOINT="localhost:9000"
export S3_BUCKET="avatars"
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"
export S3_USE_SSL="false"
export RABBITMQ_URL="amqp://rabbitmq:rabbitmq@localhost:5672/"
export HTTP_ADDRESS=":8080"
```

Run server:

```bash
make server
```

Run worker in another terminal:

```bash
make worker
```

## OpenAPI

OpenAPI contract lives in:

```text
api/openapi.yaml
```

Generate Go models from OpenAPI:

```bash
make openapi
```

Run all code generation:

```bash
make generate
```

Generated file:

```text
internal/api/openapi.gen.go
```

## Architecture

```mermaid
flowchart LR
  classDef entry fill:#e8f2ff,stroke:#2563eb,stroke-width:1px,color:#111827
  classDef app fill:#ecfdf5,stroke:#059669,stroke-width:1px,color:#111827
  classDef data fill:#fff7ed,stroke:#ea580c,stroke-width:1px,color:#111827
  classDef config fill:#f5f3ff,stroke:#7c3aed,stroke-width:1px,color:#111827
  classDef monitor fill:#fef2f2,stroke:#dc2626,stroke-width:1px,color:#111827

  subgraph Traffic
    user["Client"]
    ingress["Ingress"]
    service["Service: goavatar-server"]
  end

  subgraph Application
    server["Deployment: server pods"]
    worker["Deployment: worker pod"]
    hpa["HPA: CPU 70%, memory 80%"]
  end

  subgraph Data
    postgres["StatefulSet: PostgreSQL"]
    postgresPvc["PVC: postgres-data"]
    minio["StatefulSet: MinIO"]
    minioPvc["PVC: minio-data"]
    rabbitmq["StatefulSet: RabbitMQ"]
    rabbitmqPvc["PVC: rabbitmq-data"]
  end

  subgraph HelmHooks
    migrationJob["Job: DB migrations"]
    bucketJob["Job: MinIO bucket"]
  end

  subgraph Configuration
    config["ConfigMap: app config"]
    secrets["Secret: credentials"]
  end

  subgraph Monitoring
    serviceMonitor["ServiceMonitor: /metrics"]
    prometheus["Prometheus: Operator stack"]
    grafana["Grafana: dashboards"]
  end

  user --> ingress --> service --> server
  hpa --> server

  server --> postgres
  server --> minio
  server --> rabbitmq
  rabbitmq --> worker
  worker --> postgres
  worker --> minio

  postgres --> postgresPvc
  minio --> minioPvc
  rabbitmq --> rabbitmqPvc

  migrationJob --> postgres
  bucketJob --> minio

  config --> server
  config --> worker
  secrets --> server
  secrets --> worker

  serviceMonitor --> service
  prometheus --> serviceMonitor
  grafana --> prometheus

  class user,ingress,service entry
  class server,worker,hpa app
  class postgres,postgresPvc,minio,minioPvc,rabbitmq,rabbitmqPvc data
  class config,secrets config
  class serviceMonitor,prometheus,grafana monitor
```

Request flow:

1. Client sends HTTP request to `Ingress`.
2. `Ingress` routes traffic to `goavatar-server` `Service`.
3. `Service` load-balances traffic to server pods.
4. Server stores avatar metadata in PostgreSQL.
5. Server stores original image in MinIO.
6. Server publishes processing event to RabbitMQ.
7. Worker consumes event, creates thumbnails, and stores them in MinIO.
8. Prometheus discovers `ServiceMonitor` and scrapes `/metrics`.
9. Grafana reads metrics from Prometheus.

## Deploy To Kubernetes With Helm

Build local image first. Kubernetes deploys images; it does not build them.

For Rancher Desktop with Docker-compatible image store:

```bash
docker build -t goavatar:latest .
```

Install or upgrade Helm release:

```bash
helm upgrade --install goavatar deploy/helm/goavatar \
  -n goavatar \
  --create-namespace \
  -f deploy/helm/goavatar/values-local.yaml
```

If namespace already exists and is not owned by Helm:

```bash
helm upgrade --install goavatar deploy/helm/goavatar \
  -n goavatar \
  -f deploy/helm/goavatar/values-local.yaml \
  --set namespace.create=false
```

Check workloads:

```bash
kubectl get pods -n goavatar
kubectl get jobs -n goavatar
kubectl get hpa -n goavatar
```

Follow pod status:

```bash
kubectl get pods -n goavatar -w
```

Port-forward API:

```bash
kubectl port-forward -n goavatar svc/goavatar-server 8080:80
```

Check deployed service:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/metrics
```

Upload avatar to K8s deployment:

```bash
curl -X POST http://localhost:8080/api/v1/avatars \
  -H "X-User-ID: user-1" \
  -F "file=@scripts/image.png"
```

List user avatars:

```bash
curl http://localhost:8080/api/v1/users/user-1/avatars
```

Uninstall release:

```bash
helm uninstall goavatar -n goavatar
```

PVCs may remain after uninstall to preserve database, MinIO, and RabbitMQ data.
Remove them only when data loss is acceptable:

```bash
kubectl delete pvc -n goavatar --all
```

## Monitoring In Kubernetes

Helm chart creates `ServiceMonitor` when `serviceMonitor.enabled=true`.
Prometheus Operator discovers it and scrapes `/metrics` from `goavatar-server`.

Install kube-prometheus-stack first if cluster does not have Prometheus Operator:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  -n monitoring \
  --create-namespace
```

## Quality Checks

Run tests:

```bash
make test
```

Run formatter, linter, and tests:

```bash
make all
```
