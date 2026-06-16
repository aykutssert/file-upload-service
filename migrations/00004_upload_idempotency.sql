-- +goose Up
ALTER TABLE files
    ADD COLUMN idempotency_key text NOT NULL,
    ADD COLUMN create_request_hash bytea NOT NULL,
    ADD CONSTRAINT files_idempotency_key_not_blank
        CHECK (btrim(idempotency_key) <> ''),
    ADD CONSTRAINT files_create_request_hash_sha256_length
        CHECK (octet_length(create_request_hash) = 32),
    ADD CONSTRAINT files_tenant_owner_idempotency_unique
        UNIQUE (tenant_id, owner_principal_id, idempotency_key);

-- +goose Down
ALTER TABLE files
    DROP CONSTRAINT files_tenant_owner_idempotency_unique,
    DROP CONSTRAINT files_create_request_hash_sha256_length,
    DROP CONSTRAINT files_idempotency_key_not_blank,
    DROP COLUMN create_request_hash,
    DROP COLUMN idempotency_key;
