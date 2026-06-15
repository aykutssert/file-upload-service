-- +goose Up
ALTER TABLE principals
    ADD CONSTRAINT principals_tenant_id_id_unique UNIQUE (tenant_id, id);

CREATE TABLE files (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    owner_principal_id uuid NOT NULL,
    object_key text NOT NULL,
    original_name text NOT NULL,
    content_type text NOT NULL,
    expected_size bigint NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    checksum bytea,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    uploaded_at timestamptz,
    ready_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT files_object_key_not_blank CHECK (btrim(object_key) <> ''),
    CONSTRAINT files_object_key_unique UNIQUE (object_key),
    CONSTRAINT files_original_name_not_blank
        CHECK (btrim(original_name) <> ''),
    CONSTRAINT files_content_type_not_blank
        CHECK (btrim(content_type) <> ''),
    CONSTRAINT files_expected_size_non_negative CHECK (expected_size >= 0),
    CONSTRAINT files_owner_in_tenant
        FOREIGN KEY (tenant_id, owner_principal_id)
        REFERENCES principals (tenant_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT files_checksum_sha256_length
        CHECK (checksum IS NULL OR octet_length(checksum) = 32),
    CONSTRAINT files_status_valid CHECK (
        status IN (
            'pending', 'uploaded', 'processing', 'ready', 'rejected', 'deleted'
        )
    ),
    CONSTRAINT files_updated_at_after_created
        CHECK (updated_at >= created_at),
    CONSTRAINT files_uploaded_at_after_created
        CHECK (uploaded_at IS NULL OR uploaded_at >= created_at),
    CONSTRAINT files_ready_at_after_uploaded
        CHECK (
            ready_at IS NULL
            OR (uploaded_at IS NOT NULL AND ready_at >= uploaded_at)
        ),
    CONSTRAINT files_deleted_at_after_upload
        CHECK (
            deleted_at IS NULL
            OR (
                uploaded_at IS NOT NULL
                AND deleted_at >= COALESCE(ready_at, uploaded_at)
            )
        ),
    CONSTRAINT files_status_timestamps_consistent CHECK (
        CASE status
            WHEN 'pending' THEN
                uploaded_at IS NULL AND ready_at IS NULL
                AND deleted_at IS NULL
            WHEN 'uploaded' THEN
                uploaded_at IS NOT NULL AND ready_at IS NULL
                AND deleted_at IS NULL
            WHEN 'processing' THEN
                uploaded_at IS NOT NULL AND ready_at IS NULL
                AND deleted_at IS NULL
            WHEN 'rejected' THEN
                uploaded_at IS NOT NULL AND ready_at IS NULL
                AND deleted_at IS NULL
            WHEN 'ready' THEN
                uploaded_at IS NOT NULL AND ready_at IS NOT NULL
                AND deleted_at IS NULL
            WHEN 'deleted' THEN
                uploaded_at IS NOT NULL AND deleted_at IS NOT NULL
            ELSE false
        END
    )
);

CREATE INDEX files_tenant_created_at_idx ON files (tenant_id, created_at, id);
CREATE INDEX files_tenant_owner_created_at_idx
    ON files (tenant_id, owner_principal_id, created_at, id);
CREATE INDEX files_tenant_status_created_at_idx
    ON files (tenant_id, status, created_at, id);

-- +goose Down
DROP TABLE files;
ALTER TABLE principals
    DROP CONSTRAINT IF EXISTS principals_tenant_id_id_unique;
