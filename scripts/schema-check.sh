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

echo "schema constraints verified"
