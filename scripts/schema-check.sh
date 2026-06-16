#!/bin/sh
set -eu

psql() {
  docker compose exec -T postgres psql \
    -U "${POSTGRES_USER:-file_upload}" \
    -d "${POSTGRES_DB:-file_upload}" \
    -v ON_ERROR_STOP=1 \
    "$@"
}

psql <<'SQL'
BEGIN;

WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Schema Test Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role,
        permissions
    )
    SELECT
        id,
        'schema-test-user',
        'user',
        'member',
        ARRAY['file:create', 'file:read']
    FROM tenant
    RETURNING id
)
INSERT INTO api_keys (principal_id, key_prefix, key_hash)
SELECT id, 'fus_schema', decode(repeat('ab', 32), 'hex')
FROM principal;

ROLLBACK;
SQL

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Invalid Hash Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'invalid-hash-user', 'user', 'member'
    FROM tenant
    RETURNING id
)
INSERT INTO api_keys (principal_id, key_prefix, key_hash)
SELECT id, 'fus_invalid', decode('ab', 'hex')
FROM principal;
ROLLBACK;
SQL
then
  echo "invalid API key hash was accepted" >&2
  exit 1
fi

psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Files Schema Test Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'files-schema-test-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO files (
    tenant_id,
    owner_principal_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    status,
    checksum,
    idempotency_key,
    create_request_hash,
    uploaded_at,
    ready_at
)
SELECT
    tenant_id,
    id,
    'tenants/files-schema-test/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    0,
    'ready',
    decode(repeat('cd', 32), 'hex'),
    'files-schema-ready',
    decode(repeat('ef', 32), 'hex'),
    now(),
    now()
FROM principal;
ROLLBACK;
SQL

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Invalid File Status Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'invalid-status-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO files (
    tenant_id,
    owner_principal_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    status,
    idempotency_key,
    create_request_hash
)
SELECT
    tenant_id,
    id,
    'tenants/invalid-status/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    1048576,
    'unknown',
    'invalid-status',
    decode(repeat('ef', 32), 'hex')
FROM principal;
ROLLBACK;
SQL
then
  echo "invalid file status was accepted" >&2
  exit 1
fi

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Invalid File Checksum Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'invalid-checksum-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO files (
    tenant_id,
    owner_principal_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    checksum,
    idempotency_key,
    create_request_hash
)
SELECT
    tenant_id,
    id,
    'tenants/invalid-checksum/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    1048576,
    decode('cd', 'hex'),
    'invalid-checksum',
    decode(repeat('ef', 32), 'hex')
FROM principal;
ROLLBACK;
SQL
then
  echo "invalid file checksum length was accepted" >&2
  exit 1
fi

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Invalid File Size Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'invalid-size-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO files (
    tenant_id,
    owner_principal_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    idempotency_key,
    create_request_hash
)
SELECT
    tenant_id,
    id,
    'tenants/invalid-size/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    -1,
    'invalid-size',
    decode(repeat('ef', 32), 'hex')
FROM principal;
ROLLBACK;
SQL
then
  echo "negative file expected_size was accepted" >&2
  exit 1
fi

if psql <<'SQL'
BEGIN;
WITH first_tenant AS (
    INSERT INTO tenants (name)
    VALUES ('File Owner Tenant A')
    RETURNING id
),
second_tenant AS (
    INSERT INTO tenants (name)
    VALUES ('File Owner Tenant B')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'cross-tenant-owner', 'user', 'member'
    FROM first_tenant
    RETURNING id
)
INSERT INTO files (
    tenant_id,
    owner_principal_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    idempotency_key,
    create_request_hash
)
SELECT
    second_tenant.id,
    principal.id,
    'tenants/cross-tenant/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    1,
    'cross-tenant-owner',
    decode(repeat('ef', 32), 'hex')
FROM second_tenant, principal;
ROLLBACK;
SQL
then
  echo "cross-tenant file owner was accepted" >&2
  exit 1
fi

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Invalid File State Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'invalid-state-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO files (
    tenant_id,
    owner_principal_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    status,
    idempotency_key,
    create_request_hash,
    uploaded_at
)
SELECT
    tenant_id,
    id,
    'tenants/invalid-state/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    1048576,
    'ready',
    'invalid-state',
    decode(repeat('ef', 32), 'hex'),
    now()
FROM principal;
ROLLBACK;
SQL
then
  echo "file in ready status without ready_at was accepted" >&2
  exit 1
fi

psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Multipart Schema Test Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'multipart-schema-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO multipart_uploads (
    tenant_id,
    owner_principal_id,
    s3_upload_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    part_size,
    idempotency_key
)
SELECT
    tenant_id,
    id,
    'mpu-schema-test-id',
    'tenants/multipart-schema-test/objects/' || gen_random_uuid()::text,
    'video.mp4',
    'video/mp4',
    104857600,
    10485760,
    'multipart-schema-test'
FROM principal;
ROLLBACK;
SQL

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Invalid Multipart Status Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'invalid-mpu-status-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO multipart_uploads (
    tenant_id,
    owner_principal_id,
    s3_upload_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    part_size,
    status,
    idempotency_key
)
SELECT
    tenant_id,
    id,
    'mpu-invalid-status',
    'tenants/invalid-mpu-status/objects/' || gen_random_uuid()::text,
    'video.mp4',
    'video/mp4',
    104857600,
    10485760,
    'unknown',
    'invalid-mpu-status'
FROM principal;
ROLLBACK;
SQL
then
  echo "invalid multipart_uploads status was accepted" >&2
  exit 1
fi

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Small Part Size Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'small-part-size-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO multipart_uploads (
    tenant_id,
    owner_principal_id,
    s3_upload_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    part_size,
    idempotency_key
)
SELECT
    tenant_id,
    id,
    'mpu-small-part',
    'tenants/small-part/objects/' || gen_random_uuid()::text,
    'video.mp4',
    'video/mp4',
    10485760,
    1048576,
    'small-part-size'
FROM principal;
ROLLBACK;
SQL
then
  echo "multipart part_size below 5 MiB was accepted" >&2
  exit 1
fi

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Multipart Cross Tenant A')
    RETURNING id
),
other_tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Multipart Cross Tenant B')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'multipart-cross-tenant-owner', 'user', 'member'
    FROM tenant
    RETURNING id
)
INSERT INTO multipart_uploads (
    tenant_id,
    owner_principal_id,
    s3_upload_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    part_size,
    idempotency_key
)
SELECT
    other_tenant.id,
    principal.id,
    'mpu-cross-tenant',
    'tenants/cross-tenant-mpu/objects/' || gen_random_uuid()::text,
    'video.mp4',
    'video/mp4',
    10485760,
    5242880,
    'cross-tenant-mpu'
FROM other_tenant, principal;
ROLLBACK;
SQL
then
  echo "cross-tenant multipart_uploads owner was accepted" >&2
  exit 1
fi

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Multipart Completed Without Timestamp Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'mpu-completed-no-ts-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
)
INSERT INTO multipart_uploads (
    tenant_id,
    owner_principal_id,
    s3_upload_id,
    object_key,
    original_name,
    content_type,
    expected_size,
    part_size,
    status,
    idempotency_key
)
SELECT
    tenant_id,
    id,
    'mpu-completed-no-ts',
    'tenants/mpu-completed-no-ts/objects/' || gen_random_uuid()::text,
    'video.mp4',
    'video/mp4',
    10485760,
    5242880,
    'completed',
    'mpu-completed-no-ts'
FROM principal;
ROLLBACK;
SQL
then
  echo "multipart_uploads completed without completed_at was accepted" >&2
  exit 1
fi

psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Multipart Parts Test Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'multipart-parts-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
),
mpu AS (
    INSERT INTO multipart_uploads (
        tenant_id,
        owner_principal_id,
        s3_upload_id,
        object_key,
        original_name,
        content_type,
        expected_size,
        part_size,
        idempotency_key
    )
    SELECT
        tenant_id,
        id,
        'mpu-parts-test-id',
        'tenants/mpu-parts-test/objects/' || gen_random_uuid()::text,
        'video.mp4',
        'video/mp4',
        20971520,
        10485760,
        'multipart-parts-test'
    FROM principal
    RETURNING id
)
INSERT INTO multipart_parts (multipart_upload_id, part_number, etag, size)
SELECT id, 1, '"abc123"', 10485760 FROM mpu;
ROLLBACK;
SQL

if psql <<'SQL'
BEGIN;
WITH tenant AS (
    INSERT INTO tenants (name)
    VALUES ('Invalid Part Number Tenant')
    RETURNING id
),
principal AS (
    INSERT INTO principals (
        tenant_id,
        external_id,
        principal_type,
        role
    )
    SELECT id, 'invalid-part-num-user', 'user', 'member'
    FROM tenant
    RETURNING id, tenant_id
),
mpu AS (
    INSERT INTO multipart_uploads (
        tenant_id,
        owner_principal_id,
        s3_upload_id,
        object_key,
        original_name,
        content_type,
        expected_size,
        part_size,
        idempotency_key
    )
    SELECT
        tenant_id,
        id,
        'mpu-invalid-part-num',
        'tenants/invalid-part-num/objects/' || gen_random_uuid()::text,
        'video.mp4',
        'video/mp4',
        10485760,
        5242880,
        'invalid-part-num'
    FROM principal
    RETURNING id
)
INSERT INTO multipart_parts (multipart_upload_id, part_number, etag, size)
SELECT id, 0, '"abc123"', 10485760 FROM mpu;
ROLLBACK;
SQL
then
  echo "multipart part_number 0 was accepted" >&2
  exit 1
fi

echo "schema constraints verified"
