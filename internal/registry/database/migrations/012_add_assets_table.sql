-- =============================================================================
-- ASSETS TABLE
-- =============================================================================

CREATE TABLE assets (
    asset_id VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    category VARCHAR(50) NOT NULL,

    status VARCHAR(50) NOT NULL DEFAULT 'active',
    published_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    is_latest BOOLEAN NOT NULL DEFAULT true,

    value JSONB NOT NULL,

    CONSTRAINT assets_pkey PRIMARY KEY (asset_id, version)
);

CREATE INDEX idx_assets_id ON assets (asset_id);
CREATE INDEX idx_assets_id_version ON assets (asset_id, version);
CREATE INDEX idx_assets_latest ON assets (asset_id, is_latest) WHERE is_latest = true;
CREATE INDEX idx_assets_category ON assets (category);
CREATE INDEX idx_assets_status ON assets (status);
CREATE INDEX idx_assets_published_at ON assets (published_at DESC);
CREATE INDEX idx_assets_updated_at ON assets (updated_at DESC);
CREATE UNIQUE INDEX idx_unique_latest_per_asset ON assets (asset_id) WHERE is_latest = true;

ALTER TABLE assets ADD CONSTRAINT check_asset_status_valid
    CHECK (status IN ('active', 'deprecated', 'deleted'));

ALTER TABLE assets ADD CONSTRAINT check_asset_category_valid
    CHECK (category IN ('prompt', 'agent', 'mcp'));

ALTER TABLE assets ADD CONSTRAINT check_asset_version_not_empty
    CHECK (length(trim(version)) > 0);

CREATE OR REPLACE FUNCTION update_assets_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_assets_updated_at
    BEFORE UPDATE ON assets
    FOR EACH ROW
    EXECUTE FUNCTION update_assets_updated_at();

-- Backfill native assets from existing SHUB-compatible skills.
WITH shub_skills AS (
    SELECT
        COALESCE(
            NULLIF(value->'shub'->>'assetId', ''),
            NULLIF(value->'shub'->'manifest'->>'id', '')
        ) AS asset_id,
        version,
        COALESCE(
            NULLIF(value->'shub'->'manifest'->>'category', ''),
            NULLIF(value->'shub'->>'category', '')
        ) AS category,
        status,
        published_at,
        updated_at,
        is_latest,
        value
    FROM skills
    WHERE value ? 'shub'
      AND jsonb_typeof(value->'shub') = 'object'
      AND jsonb_typeof(value->'shub'->'manifest') = 'object'
)
INSERT INTO assets (asset_id, version, category, status, published_at, updated_at, is_latest, value)
SELECT
    asset_id,
    version,
    category,
    status,
    published_at,
    updated_at,
    is_latest,
    jsonb_strip_nulls(jsonb_build_object(
        'id', asset_id,
        'name', COALESCE(
            NULLIF(value->'shub'->'manifest'->>'name', ''),
            NULLIF(value->>'title', ''),
            NULLIF(value->>'name', '')
        ),
        'description', COALESCE(
            NULLIF(value->'shub'->'manifest'->>'description', ''),
            NULLIF(value->>'description', '')
        ),
        'version', version,
        'category', category,
        'allowedTools', COALESCE(value->'shub'->'manifest'->'allowedTools', '[]'::jsonb),
        'sourceSkill', COALESCE(
            value->'shub'->'manifest'->'sourceSkill',
            jsonb_build_object('path', 'SKILL.md', 'bodyFormat', 'markdown')
        ),
        'manifest', value->'shub'->'manifest',
        'source', value->'shub'->'source',
        'status', status
    ))
FROM shub_skills
WHERE asset_id IS NOT NULL
  AND asset_id <> ''
  AND category IN ('prompt', 'agent', 'mcp')
ON CONFLICT (asset_id, version) DO NOTHING;
