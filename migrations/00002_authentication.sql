-- +goose Up
CREATE TABLE tenants (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT tenants_name_not_blank CHECK (btrim(name) <> '')
);

CREATE TABLE principals (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    external_id text NOT NULL,
    principal_type text NOT NULL,
    role text NOT NULL,
    permissions text[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    disabled_at timestamptz,
    CONSTRAINT principals_external_id_not_blank
        CHECK (btrim(external_id) <> ''),
    CONSTRAINT principals_type_valid
        CHECK (principal_type IN ('user', 'service')),
    CONSTRAINT principals_role_not_blank CHECK (btrim(role) <> ''),
    CONSTRAINT principals_tenant_external_id_unique
        UNIQUE (tenant_id, external_id)
);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    principal_id uuid NOT NULL REFERENCES principals (id) ON DELETE CASCADE,
    key_prefix text NOT NULL,
    key_hash bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    revoked_at timestamptz,
    last_used_at timestamptz,
    CONSTRAINT api_keys_prefix_not_blank CHECK (btrim(key_prefix) <> ''),
    CONSTRAINT api_keys_hash_sha256_length
        CHECK (octet_length(key_hash) = 32),
    CONSTRAINT api_keys_expiry_after_creation
        CHECK (expires_at IS NULL OR expires_at > created_at),
    CONSTRAINT api_keys_hash_unique UNIQUE (key_hash)
);

CREATE INDEX api_keys_principal_id_idx ON api_keys (principal_id);

-- +goose Down
DROP TABLE api_keys;
DROP TABLE principals;
DROP TABLE tenants;
