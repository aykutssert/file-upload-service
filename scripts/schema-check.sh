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
    status
)
SELECT
    tenant_id,
    id,
    'tenants/invalid-status/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    1048576,
    'unknown'
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
    checksum
)
SELECT
    tenant_id,
    id,
    'tenants/invalid-checksum/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    1048576,
    decode('cd', 'hex')
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
    expected_size
)
SELECT
    tenant_id,
    id,
    'tenants/invalid-size/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    -1
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
    expected_size
)
SELECT
    second_tenant.id,
    principal.id,
    'tenants/cross-tenant/objects/' || gen_random_uuid()::text,
    'document.pdf',
    'application/pdf',
    1
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
    now()
FROM principal;
ROLLBACK;
SQL
then
  echo "file in ready status without ready_at was accepted" >&2
  exit 1
fi

echo "schema constraints verified"
