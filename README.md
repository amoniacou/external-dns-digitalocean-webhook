# External DNS DigitalOcean Webhook

A webhook provider for [ExternalDNS](https://github.com/kubernetes-sigs/external-dns) that manages DNS records in DigitalOcean.

## Features

- Full DigitalOcean DNS API support
- Automatic retry with exponential backoff for rate limits (429) and server errors (5xx)
- Configurable retry parameters
- Graceful error handling with SoftError support
- Runs as a sidecar container alongside ExternalDNS

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DO_TOKEN` | Yes | - | DigitalOcean API token |
| `DO_DOMAIN_FILTER` | No | - | Comma-separated list of domains to manage |
| `DO_DRY_RUN` | No | `false` | Enable dry-run mode |
| `DO_API_PAGE_SIZE` | No | `200` | API pagination size |
| `DO_HTTP_RETRY_MAX` | No | `3` | Maximum HTTP retries |
| `DO_HTTP_RETRY_WAIT_MIN` | No | `1s` | Minimum wait between retries |
| `DO_HTTP_RETRY_WAIT_MAX` | No | `30s` | Maximum wait between retries |

### Command Line Flags

```
--log-level      Log level (debug, info, warn, error) [default: info]
--log-format     Log format (text, json) [default: text]
--host           Webhook server host [default: 0.0.0.0]
--port           Webhook server port [default: 8888]
--dry-run        Enable dry-run mode
--retry-max      Maximum HTTP retries [default: 3]
--retry-wait-max Maximum wait between retries [default: 30s]
```

## Deployment

### Using Helm

You can deploy ExternalDNS with this webhook using the [official ExternalDNS Helm chart](https://kubernetes-sigs.github.io/external-dns/latest/charts/external-dns/).

Create a `values.yaml` file:

```yaml
provider:
  name: webhook
  webhook:
    image:
      repository: ghcr.io/amoniacou/external-dns-digitalocean-webhook
      tag: latest
    env:
      - name: DO_TOKEN
        valueFrom:
          secretKeyRef:
            name: digitalocean-credentials
            key: token
      - name: DO_DOMAIN_FILTER
        value: "example.com"
      - name: DO_HTTP_RETRY_MAX
        value: "5"
    args:
      - --port=8080
      - --log-level=info
    securityContext:
      runAsUser: 65532
      runAsGroup: 65532
      runAsNonRoot: true
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
    livenessProbe:
      httpGet:
        path: /healthz
        port: http-webhook
      initialDelaySeconds: 10
      periodSeconds: 10
    readinessProbe:
      httpGet:
        path: /healthz
        port: http-webhook
      initialDelaySeconds: 5
      periodSeconds: 5
```

> **Note:** The `--port=8080` flag is required because the ExternalDNS Helm chart expects the webhook to listen on port 8080 (`http-webhook`), while the binary defaults to 8888.

Install the chart:

```bash
helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/
helm upgrade --install external-dns external-dns/external-dns -f values.yaml
```

### Kubernetes Deployment (Sidecar) - Manual

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
spec:
  template:
    spec:
      containers:
        # ExternalDNS container
        - name: external-dns
          image: registry.k8s.io/external-dns/external-dns:v0.20.0
          args:
            - --source=ingress
            - --source=crd
            - --provider=webhook
            - --webhook-provider-url=http://localhost:8888
            - --policy=sync
            - --registry=txt
            - --txt-owner-id=my-cluster
            - --interval=1m

        # DigitalOcean Webhook sidecar
        - name: digitalocean-webhook
          image: ghcr.io/amoniacou/external-dns-digitalocean-webhook:latest
          securityContext:
            runAsUser: 65532
            runAsGroup: 65532
            runAsNonRoot: true
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
          args:
            - --log-level=info
            - --retry-max=5
            - --retry-wait-max=60s
          env:
            - name: DO_TOKEN
              valueFrom:
                secretKeyRef:
                  name: digitalocean-credentials
                  key: token
            - name: DO_DOMAIN_FILTER
              value: "example.com,example.org"
          ports:
            - containerPort: 8888
              name: http
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 5
```

### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: digitalocean-credentials
type: Opaque
stringData:
  token: "your-digitalocean-api-token"
```

## Building

### Prerequisites

- Go 1.25+
- Make
- [GoReleaser](https://goreleaser.com/) (required for building Docker images)

### Commands

```bash
# Run tests
make test

# Build binary locally (outputs to bin/webhook)
make build

# Build Docker image (uses GoReleaser to prepare artifacts)
make docker-build

# Run locally
DO_TOKEN=your-token make run
```

## Metrics

The webhook exposes Prometheus metrics at `http://localhost:8888/metrics` by default. These metrics help track interactions with the DigitalOcean API and monitor rate limits.

| Metric | Description | Labels |
|--------|-------------|--------|
| `digitalocean_api_requests_total` | Total number of requests to DigitalOcean API | `action` (HTTP method + path) |
| `digitalocean_api_errors_total` | Total number of failed requests (4xx/5xx) | `action` |
| `digitalocean_api_rate_limits_total` | Total number of rate limit hits (HTTP 429) | `action` |

## Why Webhook Instead of In-Tree Provider?

1. **Independent release cycle** - No waiting for upstream ExternalDNS releases
2. **Custom features** - Rate limiting, retry logic, enhanced error handling
3. **Better control** - Configure retry parameters for your specific needs
4. **Upstream policy** - ExternalDNS prefers webhook providers for new/updated providers

## Rate Limiting

This webhook uses the built-in retry mechanism from the [godo](https://github.com/digitalocean/godo) library:

- Automatically retries on HTTP 429 (rate limit) and 5xx errors
- Exponential backoff between retries
- Configurable max retries and wait times

If all retries are exhausted, the error is returned as a `SoftError`, allowing ExternalDNS to retry on the next reconciliation cycle.

## Credits

This project is based on the original in-tree DigitalOcean provider code from [ExternalDNS](https://github.com/kubernetes-sigs/external-dns). We have adapted it to run as a standalone webhook provider with enhanced features and independent lifecycle.

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
