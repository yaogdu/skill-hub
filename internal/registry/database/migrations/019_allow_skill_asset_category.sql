-- Allow SHUB-native skill-directory assets to be represented as first-class assets.

ALTER TABLE assets DROP CONSTRAINT IF EXISTS check_asset_category_valid;

ALTER TABLE assets ADD CONSTRAINT check_asset_category_valid
    CHECK (category IN ('prompt', 'skill', 'agent', 'mcp'));
