-- +goose Up
CREATE TABLE multipart_uploads (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    owner_principal_id uuid NOT NULL,
    s3_upload_id text NOT NULL,
    object_key text NOT NULL,
    original_name text NOT NULL,
    content_type text NOT NULL,
    expected_size bigint NOT NULL,
    part_size bigint NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    idempotency_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    aborted_at timestamptz,
    CONSTRAINT multipart_uploads_s3_upload_id_not_blank
        CHECK (btrim(s3_upload_id) <> ''),
    CONSTRAINT multipart_uploads_object_key_not_blank
        CHECK (btrim(object_key) <> ''),
    CONSTRAINT multipart_uploads_object_key_unique UNIQUE (object_key),
    CONSTRAINT multipart_uploads_original_name_not_blank
        CHECK (btrim(original_name) <> ''),
    CONSTRAINT multipart_uploads_content_type_not_blank
        CHECK (btrim(content_type) <> ''),
    CONSTRAINT multipart_uploads_expected_size_non_negative
        CHECK (expected_size >= 0),
    CONSTRAINT multipart_uploads_part_size_minimum
        CHECK (part_size >= 5242880),
    CONSTRAINT multipart_uploads_owner_in_tenant
        FOREIGN KEY (tenant_id, owner_principal_id)
        REFERENCES principals (tenant_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT multipart_uploads_idempotency_key_unique
        UNIQUE (tenant_id, owner_principal_id, idempotency_key),
    CONSTRAINT multipart_uploads_status_valid
        CHECK (status IN ('pending', 'completed', 'aborted')),
    CONSTRAINT multipart_uploads_updated_at_after_created
        CHECK (updated_at >= created_at),
    CONSTRAINT multipart_uploads_status_timestamps_consistent CHECK (
        CASE status
            WHEN 'pending' THEN
                completed_at IS NULL AND aborted_at IS NULL
            WHEN 'completed' THEN
                completed_at IS NOT NULL AND aborted_at IS NULL
            WHEN 'aborted' THEN
                aborted_at IS NOT NULL AND completed_at IS NULL
            ELSE false
        END
    )
);

CREATE INDEX multipart_uploads_tenant_created_at_idx
    ON multipart_uploads (tenant_id, created_at, id);
CREATE INDEX multipart_uploads_tenant_owner_created_at_idx
    ON multipart_uploads (tenant_id, owner_principal_id, created_at, id);
CREATE INDEX multipart_uploads_tenant_status_created_at_idx
    ON multipart_uploads (tenant_id, status, created_at, id);

CREATE TABLE multipart_parts (
    multipart_upload_id uuid NOT NULL
        REFERENCES multipart_uploads (id) ON DELETE RESTRICT,
    part_number int NOT NULL,
    etag text NOT NULL,
    size bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (multipart_upload_id, part_number),
    CONSTRAINT multipart_parts_part_number_valid
        CHECK (part_number >= 1 AND part_number <= 10000),
    CONSTRAINT multipart_parts_etag_not_blank
        CHECK (btrim(etag) <> ''),
    CONSTRAINT multipart_parts_size_positive
        CHECK (size > 0)
);

-- +goose Down
DROP TABLE multipart_parts;
DROP TABLE multipart_uploads;
