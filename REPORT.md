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

---

# v0.3 — Multipart Upload and Resume

## Overview

v0.3 adds server-side multipart upload sessions backed by S3 multipart upload
API. Large files are split into parts by the client; each part is uploaded
directly to SeaweedFS via a short-lived presigned PUT URL. The API orchestrates
session state and part metadata but never transfers file bytes.

## Endpoints

| Method | Path | Description |
| --- | --- | --- |
| POST | /v1/multipart-sessions | Create session; returns session ID |
| GET | /v1/multipart-sessions/{id}/parts/{n}?size=N | Presign part N upload URL |
| POST | /v1/multipart-sessions/{id}/parts/{n} | Confirm part N (ETag + size) |
| GET | /v1/multipart-sessions/{id}/parts | List confirmed parts |
| POST | /v1/multipart-sessions/{id}/complete | Finalize; creates file record |
| DELETE | /v1/multipart-sessions/{id} | Abort and clean up |

All six require `file:create` permission. Completed sessions appear in the
standard `GET /v1/files` listing.

## Schema

`multipart_uploads` stores session state. `multipart_parts` stores confirmed
part ETags. Constraints enforce:

- `part_size >= 5242880` (S3 minimum 5 MiB per part)
- `part_number` between 1 and 10000
- `status IN ('pending','completed','aborted')`
- Check constraint: `completed_at NOT NULL` when `status='completed'`, and
  `aborted_at NOT NULL` when `status='aborted'`
- Composite FK `(tenant_id, owner_principal_id)` references `principals`
- Unique `(tenant_id, owner_principal_id, idempotency_key)` prevents duplicate
  sessions

On complete, a record is inserted into `files` with `status='ready'` using the
multipart session ID as the idempotency key. This makes completed multipart
files visible through the standard file list and download endpoints.

## Session State Machine

```
pending ──complete──> completed
pending ──abort────> aborted
```

`FindMultipartSession` filters out `aborted` sessions. Both `complete` and
`abort` are naturally idempotent: repeated calls return the current state
without side effects.

## Part Lifecycle

1. Client calls `GET /v1/multipart-sessions/{id}/parts/{n}?size=N` to receive
   a presigned PUT URL.
2. Client uploads the part bytes directly to SeaweedFS and receives an ETag in
   the response.
3. Client calls `POST /v1/multipart-sessions/{id}/parts/{n}` with the ETag and
   size to confirm the part.
4. `AddPart` uses `ON CONFLICT (multipart_upload_id, part_number) DO UPDATE`
   to mirror S3 semantics: re-uploading a part replaces the previous ETag.

## Memory Benchmark

File bytes never flow through the API. Allocations per request are constant
with respect to file size.

```
BenchmarkCreateMultipartSession_1MB-10     259490    4429 ns/op    8109 B/op    38 allocs/op
BenchmarkCreateMultipartSession_10GB-10    263342    4459 ns/op    8125 B/op    38 allocs/op
BenchmarkConfirmPart-10                    438008    2693 ns/op    7383 B/op    36 allocs/op
```

The 16-byte difference between the 1 MB and 10 GB session benchmarks is the
JSON integer representation of `expected_size`. Allocation count is identical.
A session for a 10 GB file uses the same handler memory as a session for a
1 MB file.

Benchmarks run on Apple M4, Go 1.26, `go test -bench -benchmem -count=3`.

## Known Limitations

- **Orphan S3 uploads on idempotent create**: when a `POST /v1/multipart-sessions`
  is retried with the same idempotency key, `CreateMultipartUpload` is called
  before the DB conflict is detected. The new S3 multipart upload is abandoned.
  A cleanup sweep is planned for v0.6.
- **Non-atomic S3 + DB complete**: if the process crashes between
  `CompleteMultipartUpload` (S3) and the DB update, the next retry detects
  `ErrMultipartUploadNotFound` and skips the S3 call, but a duplicate INSERT
  into `files` will fail. Recovery is planned for v0.6.
- Part size minimum is enforced in the DB schema (5 MiB). The last part may be
  smaller; clients provide the actual size via the `?size=` query parameter.
- Resume across session restarts is supported: `GET /v1/multipart-sessions/{id}/parts`
  returns all confirmed parts so the client can skip already-uploaded parts.

## Verified Behavior

- Session create returns 201 on first call, 200 with `reused:true` on retry.
- Cross-tenant session access is rejected at every endpoint.
- Part confirm with wrong session owner returns 404.
- Re-confirming a part with a new ETag updates the stored ETag (S3 semantics).
- Complete with zero confirmed parts returns 409.
- Complete on an already-completed session returns 200 with the existing file ID.
- Abort on a missing or already-aborted session returns 204.
- Abort on a completed session returns 409.
- `gofmt`, `go vet`, `go test ./...`, `go test -race ./...` pass.
- PostgreSQL integration tests: full session lifecycle, abort lifecycle,
  cross-tenant isolation, part deduplication.
