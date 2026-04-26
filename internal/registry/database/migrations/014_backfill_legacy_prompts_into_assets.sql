-- Backfill native assets from existing legacy prompts.
INSERT INTO assets (asset_id, version, category, status, published_at, updated_at, is_latest, value)
SELECT
    prompt_name AS asset_id,
    version,
    'prompt' AS category,
    status,
    published_at,
    updated_at,
    is_latest,
    jsonb_strip_nulls(jsonb_build_object(
        'id', prompt_name,
        'name', prompt_name,
        'description', COALESCE(NULLIF(value->>'description', ''), prompt_name),
        'version', version,
        'category', 'prompt',
        'sourceSkill', jsonb_build_object(
            'path', 'SKILL.md',
            'body', COALESCE(NULLIF(value->>'content', ''), ''),
            'bodyFormat', 'markdown'
        ),
        'manifest', jsonb_strip_nulls(jsonb_build_object(
            'schemaVersion', 'shub.asset/v1alpha1',
            'id', prompt_name,
            'category', 'prompt',
            'name', prompt_name,
            'description', COALESCE(NULLIF(value->>'description', ''), prompt_name),
            'version', version,
            'sourceSkill', jsonb_build_object(
                'path', 'SKILL.md',
                'body', COALESCE(NULLIF(value->>'content', ''), ''),
                'bodyFormat', 'markdown'
            ),
            'entry', jsonb_build_object(
                'kind', 'skill-body',
                'path', 'SKILL.md'
            ),
            'runtime', jsonb_build_object(
                'type', 'none'
            )
        )),
        'status', status
    ))
FROM prompts
ON CONFLICT (asset_id, version) DO NOTHING;
