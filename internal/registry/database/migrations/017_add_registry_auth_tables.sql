CREATE TABLE IF NOT EXISTS registry_users (
    id TEXT PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT check_registry_user_role_valid CHECK (role IN ('admin', 'user'))
);

CREATE TABLE IF NOT EXISTS registry_api_keys (
    id TEXT PRIMARY KEY DEFAULT uuid_generate_v4()::text,
    user_id TEXT NOT NULL REFERENCES registry_users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_registry_api_keys_user_id ON registry_api_keys (user_id);
CREATE INDEX IF NOT EXISTS idx_registry_api_keys_active ON registry_api_keys (user_id, revoked_at);

CREATE TABLE IF NOT EXISTS registry_resource_owners (
    resource_type TEXT NOT NULL,
    resource_name TEXT NOT NULL,
    owner_user_id TEXT NOT NULL REFERENCES registry_users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT registry_resource_owners_pkey PRIMARY KEY (resource_type, resource_name)
);

CREATE INDEX IF NOT EXISTS idx_registry_resource_owners_owner_user_id ON registry_resource_owners (owner_user_id);
