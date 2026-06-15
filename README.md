# File Upload Service

A production-oriented large-file upload and processing platform built as a
versioned backend engineering project.

The current v0.1 foundation runs locally with:

- Go 1.26 and Chi
- PostgreSQL 18
- SeaweedFS S3-compatible object storage
- NATS JetStream
- Docker Compose

## Start

Requirements:

- Go 1.26 or newer
- Docker Desktop with Compose

Start the complete platform and apply database migrations:

```bash
make services-up
```

Verify the API:

```bash
curl http://127.0.0.1:8080/health/live
curl http://127.0.0.1:8080/health/ready
```

Expected responses:

```json
{"status":"ok"}
```

```json
{"status":"ready"}
```

Stop the platform:

```bash
make db-down
```

The named Docker volumes remain available for the next start.

## Quality Checks

```bash
make check
make schema-check
docker compose config --quiet
docker build --tag file-upload-service:test .
```

`make check` runs formatting verification, `go vet`, tests, and the race
detector.

## Health Model

- Liveness reports whether the API process is serving HTTP.
- Readiness checks PostgreSQL, SeaweedFS, and NATS.
- A dependency outage keeps liveness healthy but changes readiness to HTTP 503.

## Authentication Foundation

The API key middleware contract is implemented but not attached to public
health endpoints. API keys resolve to a tenant-scoped user or service-account
principal with explicit permissions.

The schema stores only a 32-byte SHA-256 API key hash. Raw keys are never
persisted. The PostgreSQL resolver and protected file endpoints begin in v0.2.

## Project Status

v0.1 Service Foundations is complete. See [REPORT.md](REPORT.md) for verified
behavior and [roadmap.md](roadmap.md) for later versions.
