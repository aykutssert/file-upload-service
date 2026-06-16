# File Upload Service Roadmap

This roadmap builds a production-oriented file upload and processing platform
entirely in a local environment. Each version must produce a working system,
measured behavior, failure evidence, and explicit operational limits.

The project demonstrates backend engineering for products such as document
management systems, marketplaces, healthcare archives, media platforms, and
cloud drives. It does not attempt to reproduce a cloud provider's global
durability or scale.

## Core Architecture

```text
Client
  -> Upload API
  -> PostgreSQL
  -> SeaweedFS S3-compatible object storage
  -> NATS JetStream
  -> Processing workers
  -> Download API or Nginx cache
```

Primary technology choices:

- Go
- Chi on the Go standard library HTTP stack
- PostgreSQL with pgx and sqlc
- SeaweedFS as the S3-compatible object store
- NATS JetStream for durable asynchronous work
- ClamAV, libvips, FFmpeg, and Tesseract workers
- Prometheus, Grafana, OpenTelemetry, and Jaeger
- Docker Compose
- k6
- Go test and Testcontainers

Verified foundation versions on June 15, 2026:

| Component | Version |
| --- | --- |
| Go | 1.26.4 |
| PostgreSQL | 18.4 |
| SeaweedFS | 4.33 |
| NATS Server | 2.14.2 |
| Chi | 5.3.0 |
| pgx | 5.10.0 |
| sqlc | 1.31.1 |
| Goose | 3.27.1 |

Runtime images will use explicit version tags. Go library dependencies will be
added to `go.mod` only when the version that needs them begins.

Redis is not part of the initial architecture. It will be added only if a
measured requirement for distributed caching or locking appears.

## Service Boundary

The file service is a domain-neutral storage and processing platform. It owns:

- File identity
- Tenant and owner identity
- Generic metadata
- Upload and processing state
- Object-storage location
- Security, retention, and deletion state
- Download authorization

Product services own business relationships such as profile photos,
marketplace listing images, message attachments, medical records, folders, or
albums. They store the file ID alongside their own domain records.

The core API may list files by tenant, owner, status, or creation time and may
resolve multiple file IDs in one request. It must not contain product-specific
queries or business rules.

## Authentication And Authorization

The local portfolio system authenticates clients directly in the File Upload
API. A separate API gateway is intentionally out of scope.

Each API key resolves to a principal containing:

- `tenant_id`
- `subject_id`
- Principal type: user or service account
- Role
- Explicit permissions
- Revocation state

Security rules:

- API keys are stored only as cryptographic hashes.
- A raw key is shown only once when it is created.
- `tenant_id`, owner identity, role, and permissions are never trusted from the
  request body.
- Every file query and mutation is constrained by the authenticated tenant.
- Normal users manage only permitted files.
- Tenant administrators may manage files within their own tenant.
- Revoked keys are rejected immediately.
- Authentication, authorization decisions, and administrative key operations
  produce audit events.

The demo environment seeds at least two tenants, multiple users, and one
service account. Tests must prove that no credential can read, list, modify, or
delete another tenant's files.

The authentication component remains replaceable. A production deployment may
use OIDC/JWT, an existing API gateway, mTLS, or OAuth client credentials
without changing file ownership and authorization rules.

## Engineering Rules

- File bytes must not pass through the API when direct object-store upload is
  possible.
- Every externally retried command must be idempotent.
- Database and object-storage operations must define compensation behavior.
- Queue delivery is at least once, so every worker must tolerate duplicates.
- A file must not become downloadable before all required security checks pass.
- Large-file operations must use bounded memory.
- Every version closes with tests, review, failure checks, and documentation.
- Benchmarks and failure evidence must be stored as structured results.

## v0.1 - Service Foundations

Build the API and local infrastructure without accepting file content.

Scope:

- Go module and package boundaries
- Configuration through environment variables
- Structured JSON logging
- Liveness and readiness endpoints
- Graceful startup and shutdown
- PostgreSQL migrations
- SeaweedFS and NATS connectivity checks
- API key authentication middleware foundation
- Docker Compose development environment
- Unit and integration test foundations
- Continuous integration

Initial services:

- `upload-api`
- PostgreSQL
- SeaweedFS
- NATS JetStream

Completion criteria:

- The complete stack starts with one command.
- Liveness reports process state.
- Readiness reflects PostgreSQL, SeaweedFS, and NATS availability.
- Migrations run deterministically.
- Shutdown drains active HTTP requests.
- Formatting, linting, tests, race checks, and image builds pass.

Status: complete. The Go API, PostgreSQL, SeaweedFS, and NATS JetStream start
with one command under Docker Compose. Configuration, JSON logging, health
checks, graceful shutdown, migrations, API key middleware contracts, unit and
race tests, container builds, schema constraints, and clean-environment CI
validation are implemented. Evidence and operational limits are documented in
`REPORT.md`.

## v0.2 - Direct Upload And Download

Implement the secure single-part upload lifecycle.

Scope:

- Create an upload session
- Generate short-lived presigned upload URLs
- Upload directly from the client to SeaweedFS
- Confirm upload completion
- Store file metadata in PostgreSQL
- Generate short-lived presigned download URLs
- File status state machine
- Content-length and content-type constraints
- Ownership and authorization boundaries
- Principal permissions for create, read, list, delete, and administration
- API key creation, hashing, lookup, and revocation
- Idempotency keys for upload creation and completion
- Paginated file listing by tenant, owner, status, and creation time
- Batch metadata lookup by file ID

Initial file states:

```text
pending -> uploaded -> processing -> ready
                         |             |
                         v             v
                      rejected       deleted
```

Completion criteria:

- File bytes travel directly between the client and SeaweedFS.
- Retrying upload creation returns the same logical operation.
- A download URL is unavailable until the file reaches `ready`.
- Expired or modified presigned requests are rejected.
- Metadata cannot reference a missing or unconfirmed object without detection.
- Listing and batch lookup never cross tenant boundaries.
- Cross-tenant reads, listings, mutations, and downloads are rejected.
- Revoked and unknown API keys are rejected.
- Pagination uses stable cursors rather than offset-based page traversal.
- Integration tests exercise the real PostgreSQL and SeaweedFS containers.

Status: complete. The full direct upload and download lifecycle is implemented and
verified against real PostgreSQL and SeaweedFS containers. API key creation and
revocation, file listing with stable cursors, batch metadata lookup, and
cross-tenant isolation are in place. The complete endpoint transitions files
directly to `ready` in v0.2; the intermediate `uploaded` and `processing` states
are exercised by the async worker pipeline beginning in v0.4.

## v0.3 - Multipart Upload And Resume

Support large files and interrupted connections.

Scope:

- Initiate multipart uploads
- Select and validate part size
- Generate presigned URLs per part
- Persist upload ID and completed-part metadata
- List uploaded parts
- Resume from the last confirmed part
- Complete multipart upload
- Abort abandoned multipart uploads
- Concurrent part uploads with bounded client concurrency
- Upload checksum verification

Completion criteria:

- A multi-gigabyte test file uploads without API memory growth proportional to
  file size.
- An interrupted upload resumes without retransmitting completed parts.
- Duplicate part completion calls are safe.
- Invalid or missing parts cannot finalize an upload.
- Abandoned multipart state is discoverable and abortable.
- Structured benchmarks report throughput, part latency, and API memory use.

## v0.3.1 - Demo Client

Build a browser interface that makes upload-system behavior visible without
turning the project into a frontend product.

Technology:

- React
- TypeScript
- Vite

Scope:

- File selection and drag-and-drop
- Single-part and multipart upload
- Upload progress, throughput, and estimated remaining time
- Pause, resume, and cancel controls
- Bounded concurrent part uploads
- File metadata and processing status
- Download and delete actions
- Visible retry and failure states
- Thumbnail and processing-result display when available

The browser uploads file bytes directly to object storage through presigned
URLs. It must not proxy file content through the frontend server or Upload API.

Completion criteria:

- A user can start, pause, resume, cancel, and complete a multipart upload.
- Refreshing the page can recover an active upload from persisted server state.
- Progress reflects confirmed uploaded parts rather than only bytes read by the
  browser.
- API, storage, and processing failures produce actionable UI states.
- The UI remains a thin system demonstration and contains no business-specific
  product features.

## v0.3.2 - Remote Network Validation

Use the remaining RunPod credit for a short, controlled validation over a real
internet connection after multipart upload and resume are complete.

Budget:

- Maximum spend: USD 6
- Compute: low-cost CPU Pod, no GPU
- Runtime: only long enough to collect repeatable evidence

Deployment:

- Run SeaweedFS on the remote Pod.
- Keep the Upload API, PostgreSQL, NATS, and demo client on the local Mac.
- Expose only the required storage endpoint with temporary credentials.
- Delete the Pod immediately after the measurements are complete.

Scenarios:

- Upload a large multipart file to remote object storage.
- Interrupt the connection and resume from confirmed parts.
- Compare local-storage and remote-storage throughput.
- Measure part latency, total duration, retries, and failed requests.
- Introduce temporary network interruption or storage unavailability.

Completion criteria:

- Resume does not retransmit already confirmed parts.
- API memory remains bounded while storage is remote.
- The benchmark records latency and throughput differences between local and
  remote storage.
- Temporary storage failure produces a controlled retry or actionable error.
- Credentials, public endpoints, and Pod identifiers are not committed.
- The report states that this is a limited internet-path experiment, not proof
  of cloud durability, regional failover, or production SLA.

## v0.4 - Durable Processing Pipeline

Move expensive file analysis into asynchronous workers.

Scope:

- Publish processing jobs after confirmed upload
- Transactional outbox for database-to-queue consistency
- Durable NATS JetStream consumers
- Explicit acknowledgement
- Retry with exponential backoff
- Dead-letter handling
- Idempotent worker execution
- Processing status and attempt history
- Initial workers:
  - File validation
  - ClamAV virus scanning
  - Image thumbnail generation

Security rule:

A newly uploaded object remains private and quarantined until required
validation and virus scanning succeed.

Completion criteria:

- A committed upload cannot lose its processing job if NATS is unavailable.
- Restarting a worker does not lose acknowledged or unacknowledged work.
- Delivering the same message more than once does not duplicate side effects.
- Malware test fixtures are rejected and cannot receive download URLs.
- Failed jobs reach a visible terminal state after bounded retries.
- Dead-letter jobs can be inspected and replayed deliberately.

## v0.4.1 - Pending Session Cleanup

Remove stale multipart sessions and their orphaned S3 objects.

Background: multipart sessions that are initiated but never completed leave two
types of garbage — a row in PostgreSQL and an in-progress multipart upload in
SeaweedFS. SeaweedFS does not automatically abort incomplete multipart uploads,
so without cleanup, storage fills with orphaned parts indefinitely.

Scope:

- Background worker that scans for multipart sessions in `pending` status older
  than a configurable threshold (default 24 hours)
- Call `AbortMultipartUpload` against SeaweedFS for each stale session to release
  object-store parts
- Transition the session row to `abandoned` status after successful abort
- Handle abort failures gracefully: retry with exponential backoff, record a
  `cleanup_attempted_at` timestamp on the row
- Detect orphaned S3 multipart uploads that have no corresponding session row
- Configurable thresholds: session age limit, cleanup interval, per-run batch size
- Metrics: sessions abandoned, S3 parts freed, cleanup failures

State transition:

```text
pending (age > threshold) -> abandoned
```

Completion criteria:

- A session older than the threshold is cleaned up in the next cleanup run.
- Aborting an already-aborted S3 upload does not produce an error.
- A partially failed run leaves already-cleaned sessions in `abandoned`.
- Storage usage drops measurably after a cleanup run on seeded stale data.
- Cleanup never touches sessions in `completed` or `aborted` status.

## v0.5 - Rich Media Processing

Add pluggable processing for common product workloads.

Scope:

- OCR with Tesseract
- Image metadata and thumbnails with libvips
- Video metadata and preview generation with FFmpeg
- Derived-object naming and ownership
- Processor capability routing by media type
- Per-processor timeouts and resource limits
- Partial processing results
- Processor version tracking

Completion criteria:

- Unsupported media is rejected before expensive processing.
- Worker CPU, memory, execution time, and output size are bounded.
- Reprocessing with a new processor version does not overwrite unrelated
  outputs.
- Derived files retain a traceable relationship to the source object.
- A failed optional processor does not corrupt successful required results.

## v0.6 - Consistency, Deduplication, And Recovery

Harden cross-system behavior between PostgreSQL, SeaweedFS, and NATS.

Scope:

- Content-addressed checksums
- Tenant-aware deduplication policy
- Reference counting or explicit ownership records
- Orphan object detection
- Missing object detection
- Reconciliation jobs
- Compensating actions for partial failures
- Optimistic concurrency control
- Administrative repair commands

Required failure scenarios:

- Object upload succeeds but completion request never arrives.
- Object exists but the metadata transaction fails.
- Metadata commits but event publication is delayed.
- Worker succeeds but acknowledgement fails.
- The same completion request arrives concurrently.
- An object is deleted outside the application.

Completion criteria:

- Every required failure scenario has a deterministic recovery path.
- Reconciliation can identify and report metadata/object divergence.
- Deduplication never exposes one tenant's file to another tenant.
- Concurrent completion cannot publish duplicate logical workflows.
- Repair commands are auditable and safe to retry.

## v0.7 - Retention And Deletion

Implement the complete file lifecycle.

Scope:

- Soft deletion
- Configurable retention policies
- Legal-hold representation
- Delayed physical deletion
- Cleanup of expired upload sessions
- Abortion of incomplete multipart uploads
- Removal of orphaned derived objects
- Deletion audit trail
- Storage usage accounting

Completion criteria:

- Deleted files become inaccessible immediately.
- Physical objects remain until the configured retention boundary.
- Legal hold prevents physical deletion.
- Cleanup workers are idempotent and safe under concurrent execution.
- No referenced object is removed by orphan cleanup.
- Retention tests use controllable time rather than real waiting.

## v0.8 - Observability And Operations

Make system state and bottlenecks visible.

Scope:

- Prometheus metrics
- Grafana dashboards
- OpenTelemetry traces
- Jaeger trace inspection
- Correlation IDs across API, outbox, queue, and workers
- Queue depth and oldest-message age
- Upload and processing latency histograms
- Error classification
- Structured audit events
- Operational runbooks

Completion criteria:

- One upload can be followed from session creation to final processing through
  logs and traces.
- Dashboards expose API latency, upload completion rate, queue pressure,
  worker attempts, dead letters, processing duration, and storage divergence.
- Metrics avoid unbounded labels such as raw user IDs or file names.
- Alerts are defined for stalled processing, growing dead letters, dependency
  failures, and reconciliation drift.

## v0.9 - Load, Backpressure, And Failure Testing

Measure behavior under realistic local pressure.

Workloads:

- Many small files
- Concurrent medium files
- Large multipart files
- Slow and interrupted clients
- Repeated duplicate files
- Expensive OCR and video processing
- Download traffic through Nginx

Failure tests:

- PostgreSQL restart
- SeaweedFS outage and recovery
- NATS outage and recovery
- Worker crash during processing
- API termination during completion
- Queue saturation
- Disk pressure
- Slow object storage
- Poison message

Measure:

- Upload and download throughput
- API p50, p95, and p99 latency
- Multipart part latency
- Queue wait time
- End-to-end processing time
- Worker throughput and retry rate
- Error and rejection rate
- API and worker memory
- Storage and metadata divergence

Completion criteria:

- The system applies explicit backpressure instead of failing unpredictably.
- API memory remains bounded during large uploads.
- Recovery behavior is documented for every injected failure.
- Capacity limits and the first bottleneck are identified with measurements.
- Benchmark and failure results are committed as structured evidence.

## v1.0 - Portfolio Release

Package the project as a defensible backend engineering case study.

Deliverables:

- Architecture diagram
- API contract
- Data model and state machine
- Local deployment guide
- Security model
- Consistency and recovery design
- Benchmark report
- Failure-injection report
- Operational runbook
- Cloud migration design
- Explicit local-environment limitations
- CV bullets and interview discussion topics

Completion criteria:

- Every major design decision is supported by code, tests, measurements, or a
  documented tradeoff.
- A reviewer can start the complete system locally and reproduce the main
  success and failure scenarios.
- The documentation clearly separates demonstrated behavior from cloud-scale
  claims.

## Local Evidence Boundaries

This project can demonstrate:

- Multipart upload and resume behavior
- Direct presigned upload and download
- Bounded-memory large-file handling
- Idempotency and duplicate delivery safety
- Queue retry, backoff, dead-letter, and worker recovery
- Quarantine and malware rejection
- Metadata and object-store reconciliation
- Retention and orphan cleanup
- Backpressure, metrics, traces, and failure recovery

This project cannot demonstrate:

- AWS S3 or Azure Blob regional durability guarantees
- Global CDN edge latency
- Multi-region replication and failover
- Real mobile and public internet behavior
- Production cloud IAM and KMS integration
- Petabyte-scale storage operation
- Managed-service SLAs
- Real cloud egress and storage cost

These limitations are not missing implementation tasks. The portfolio goal is
to prove correct system behavior locally and explain how cloud deployment
would change the architecture.

## Cloud Migration Reference

The local architecture maps to managed infrastructure as follows:

| Local component | AWS example | Azure example |
| --- | --- | --- |
| SeaweedFS | S3 | Blob Storage |
| PostgreSQL | RDS for PostgreSQL | Azure Database for PostgreSQL |
| NATS JetStream | MSK, SQS, or self-managed NATS | Service Bus or self-managed NATS |
| Nginx cache | CloudFront | Azure Front Door or CDN |
| Docker Compose services | ECS or EKS | Container Apps or AKS |
| Local secrets | Secrets Manager and KMS | Key Vault |
| Prometheus and Grafana | Managed Prometheus and Grafana | Azure Monitor and Managed Grafana |

The exact cloud mapping must follow measured workload requirements rather than
being treated as a one-to-one replacement checklist.
