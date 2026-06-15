# v0.1 Service Foundations Report

## Outcome

v0.1 establishes the local runtime and engineering controls required by later
file upload versions. It intentionally does not accept file content.

## Runtime

| Component | Version | Purpose |
| --- | --- | --- |
| Go | 1.26.4 | API runtime |
| Chi | 5.3.0 | HTTP routing |
| PostgreSQL | 18.4 | Metadata and authentication state |
| SeaweedFS | 4.33 | S3-compatible object storage |
| NATS Server | 2.14.2 | Durable JetStream messaging |

The complete platform starts with:

```bash
make services-up
```

This command builds the API image, starts all four services, waits for their
health checks, and applies pending Goose migrations.

## API Foundation

- Environment-based configuration
- Structured JSON logs
- Separate liveness and readiness endpoints
- Graceful SIGINT and SIGTERM shutdown through `http.Server.Shutdown`
- Read-header timeout
- Chi router
- pgx connection pool

The API image uses a multi-stage build, runs as a non-root user, and is
approximately 9.2 MB on Apple Silicon.

## Dependency Readiness

Readiness checks:

- PostgreSQL through pgx `Ping`
- SeaweedFS through `/cluster/status`
- NATS through `/healthz`

Live failure tests stopped each dependency independently. In every case:

- `GET /health/live` remained HTTP 200.
- `GET /health/ready` changed to HTTP 503.
- Readiness returned to HTTP 200 after dependency recovery.

## Storage And Messaging

SeaweedFS runs in single-node `weed mini` mode with persistent storage:

- S3 endpoint: port 8333
- Master health endpoint: port 9333
- Initial bucket: `file-upload`
- Unauthenticated S3 access returns HTTP 403

NATS runs with JetStream enabled and persistent storage:

- Client port: 4222
- Monitoring port: 8222
- `/jsz` confirmed JetStream availability

## Database And Authentication Foundation

Goose migrations create:

- `tenants`
- `principals`
- `api_keys`

The model supports tenant-scoped users and service accounts, explicit
permissions, disablement, key expiry, key revocation, and last-use tracking.

API key constraints require:

- Non-empty key prefix
- Unique 32-byte SHA-256 hash
- Expiry later than creation

Schema checks verify that valid records are accepted and invalid hash lengths
are rejected. Re-running migrations produces no changes.

The Go authentication foundation includes:

- Bearer API key parsing
- Replaceable resolver interface
- Tenant and subject principal context
- User and service-account types
- Permission checks
- HTTP 401 for missing or invalid keys
- HTTP 503 for authentication dependency failure

The database-backed resolver is deferred to v0.2.

## Verification

Completed checks:

- `gofmt`
- `go vet`
- Unit tests
- Race detector
- Compose configuration validation
- API image build
- Non-root container verification
- Container restart recovery
- Migration idempotency
- Schema constraints
- Live health and dependency failure tests

GitHub Actions repeats code quality, race, image build, full Compose startup,
migration, schema, and health checks in a clean Linux environment.

## Limits

- The local stack runs on one physical Mac.
- SeaweedFS and NATS each run as one node.
- Authentication middleware is not connected to a database resolver yet.
- No file upload or download endpoint exists in v0.1.
- The platform proves local behavior, not cloud durability or availability.
