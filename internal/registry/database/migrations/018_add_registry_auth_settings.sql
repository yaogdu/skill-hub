CREATE TABLE IF NOT EXISTS registry_auth_settings (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    api_key_validation_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO registry_auth_settings (singleton, api_key_validation_enabled)
VALUES (TRUE, TRUE)
ON CONFLICT (singleton) DO NOTHING;
