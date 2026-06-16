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

---

# v0.2 Direct Upload And Download Report

## Outcome

v0.2 implements the complete secure single-part upload and download lifecycle
with tenant isolation, API key management, paginated file listing, and batch
metadata lookup. File bytes travel directly between the client and SeaweedFS
without passing through the API.

## Upload Lifecycle

```text
POST /v1/upload-sessions
  -> INSERT files (status = pending)
  -> PresignPutObject -> presigned PUT URL returned to client

Client PUT -> SeaweedFS (directly, not via API)

POST /v1/files/{id}/complete
  -> HeadObject (verify size and content-type match)
  -> UPDATE files SET status = ready, uploaded_at = now(), ready_at = now()

GET /v1/files/{id}/download
  -> PresignGetObject -> presigned GET URL returned to client
```

In v0.2 the complete endpoint transitions files directly from `pending` to
`ready`. The intermediate `uploaded` and `processing` states are reserved for
the async worker pipeline in v0.4, when the complete endpoint will only advance
to `uploaded` and workers will carry the file through `processing` to `ready`.

## File State Machine

```text
pending -> ready
             |
             v
           deleted
```

The database enforces state consistency via a CHECK constraint. `ready_at` is
required when status is `ready`. `deleted_at` is required when status is
`deleted`. Timestamp ordering (`uploaded_at <= ready_at`) is also enforced.

## Database Schema

Migration `00003_files.sql` creates the `files` table with:

- Composite FK `(tenant_id, owner_principal_id) REFERENCES principals(tenant_id, id)`
  ensuring owners cannot be transplanted across tenants.
- `expected_size >= 0` constraint.
- `checksum` nullable with `octet_length(checksum) = 32` when present.
- CASE-based state consistency constraint.
- Three composite indexes supporting tenant listing, owner filtering, and
  status filtering.

Migration `00004_upload_idempotency.sql` adds `idempotency_key` and
`create_request_hash` columns with a `UNIQUE (tenant_id, owner_principal_id,
idempotency_key)` constraint.

## API Key Management

`KeyCreator` hashes a randomly generated raw key with SHA-256 and stores only
the hash. The raw key is returned once at creation time. `KeyRevoker` sets
`revoked_at` via a join that enforces the caller's `tenant_id`, preventing
cross-tenant revocation. Revoked keys are rejected by the PostgreSQL resolver
immediately.

## Idempotency

Upload creation accepts an `Idempotency-Key` header. The same key with identical
request body returns the existing session. A key reused with a different request
body returns HTTP 409. The request hash is a SHA-256 of `original_name`,
`content_type`, and `expected_size`.

The complete endpoint is naturally idempotent: if the file is already `ready`,
it returns HTTP 200 with the current state. This handles the case where the
client completed successfully but did not receive the response.

## Listing And Batch Lookup

`GET /v1/files` lists files for the authenticated tenant with optional filters
(`owner_id`, `status`) and cursor-based pagination. Cursors are base64-encoded
JSON containing `created_at` and `id`, enabling stable descending order without
offset drift.

`POST /v1/files/batch` resolves up to 100 file IDs in one request, returning
only files belonging to the authenticated tenant. Unknown or cross-tenant IDs
produce an empty result rather than an error.

## Permissions

| Permission | Endpoints |
| --- | --- |
| `file:create` | POST /v1/upload-sessions, POST /v1/files/{id}/complete |
| `file:read` | GET /v1/files, POST /v1/files/batch, GET /v1/files/{id}/download |
| `file:delete` | DELETE /v1/files/{id} |

Key management endpoints (`POST /v1/keys`, `DELETE /v1/keys/{id}`) require only
a valid authenticated principal with no specific permission.

## Verified Behavior

- Upload creation with the same idempotency key returns the same session.
- Upload creation reuse with a different request hash returns HTTP 409.
- HeadObject mismatch (wrong size or content-type) blocks completion.
- Completing an already-ready file returns HTTP 200 (idempotent).
- Download URL is only issued for `ready` files.
- Delete is only accepted for `ready` files.
- A deleted file is invisible to `FindUpload` and returns HTTP 404.
- Double-delete returns HTTP 409 (state conflict).
- Batch lookup returns empty for unknown or cross-tenant IDs.
- Cross-tenant file access, listing, mutation, and download are all rejected.
- Revoked API keys are rejected immediately.
- Cross-tenant key revocation is rejected.

Integration tests exercise real PostgreSQL and SeaweedFS containers. All
database tests wrap operations in a transaction that is rolled back after each
test to leave the schema clean.

## Verification

- `gofmt`, `go vet`, `go test ./...`, `go test -race ./...` pass.
- `make schema-check` verifies all DB constraints.
- PostgreSQL integration tests: resolver, key creator, key revoker, upload
  repository (create, MarkReady, complete, list, delete, batch).
- SeaweedFS integration test: presign PUT → HeadObject → presign GET round-trip.
- Unit tests cover all handler paths, input validation, cursor encoding, and
  request hash behavior.

## Limits

- SeaweedFS runs as a single node; object loss is undetected until HeadObject.
- There is no checksum verification of uploaded bytes in v0.2.
- The `uploaded` and `processing` states exist in the schema but are not
  traversed by the API; async workers are added in v0.4.
- NATS is present and healthy but not used in v0.2.
- The platform proves local behavior, not cloud durability or availability.
