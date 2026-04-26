-- Backfill native assets from existing legacy MCP servers.
INSERT INTO assets (asset_id, version, category, status, published_at, updated_at, is_latest, value)
SELECT
    s.server_name AS asset_id,
    s.version,
    'mcp' AS category,
    s.status,
    s.published_at,
    s.updated_at,
    s.is_latest,
    jsonb_strip_nulls(jsonb_build_object(
        'id', s.server_name,
        'name', COALESCE(NULLIF(s.value->>'title', ''), s.server_name),
        'description', COALESCE(NULLIF(s.value->>'description', ''), s.server_name),
        'version', s.version,
        'category', 'mcp',
        'sourceSkill', jsonb_build_object(
            'path', 'SKILL.md',
            'body', COALESCE(
                NULLIF(convert_from(sr.content, 'UTF8'), ''),
                CONCAT_WS(E'\n\n',
                    '# ' || COALESCE(NULLIF(s.value->>'title', ''), s.server_name),
                    NULLIF(s.value->>'description', '')
                )
            ),
            'bodyFormat', 'markdown'
        ),
        'manifest', jsonb_strip_nulls(jsonb_build_object(
            'schemaVersion', 'shub.asset/v1alpha1',
            'id', s.server_name,
            'category', 'mcp',
            'name', COALESCE(NULLIF(s.value->>'title', ''), s.server_name),
            'description', COALESCE(NULLIF(s.value->>'description', ''), s.server_name),
            'version', s.version,
            'sourceSkill', jsonb_build_object(
                'path', 'SKILL.md',
                'body', COALESCE(
                    NULLIF(convert_from(sr.content, 'UTF8'), ''),
                    CONCAT_WS(E'\n\n',
                        '# ' || COALESCE(NULLIF(s.value->>'title', ''), s.server_name),
                        NULLIF(s.value->>'description', '')
                    )
                ),
                'bodyFormat', 'markdown'
            ),
            'entry', jsonb_build_object(
                'kind', 'mcp-config',
                'path', COALESCE(
                    NULLIF(s.value->'remotes'->0->>'url', ''),
                    NULLIF(s.value->'packages'->0->>'identifier', ''),
                    NULLIF(s.value->'repository'->>'url', ''),
                    'server.json'
                )
            ),
            'runtime', jsonb_build_object(
                'type', COALESCE(
                    NULLIF(s.value->'packages'->0->>'runtimeHint', ''),
                    NULLIF(LOWER(s.value->'packages'->0->>'registryType'), ''),
                    CASE WHEN NULLIF(s.value->'remotes'->0->>'url', '') IS NOT NULL THEN 'remote' END,
                    'mcp'
                )
            ),
            'metadata', jsonb_build_object(
                'legacyServer', jsonb_strip_nulls(jsonb_build_object(
                    'server', s.value
                ))
            )
        )),
        'source', jsonb_strip_nulls(jsonb_build_object(
            'repositoryUrl', NULLIF(s.value->'repository'->>'url', ''),
            'packageType', NULLIF(s.value->'packages'->0->>'registryType', ''),
            'packageRef', NULLIF(s.value->'packages'->0->>'identifier', '')
        )),
        'status', s.status
    ))
FROM servers s
LEFT JOIN server_readmes sr
    ON sr.server_name = s.server_name
   AND sr.version = s.version
ON CONFLICT (asset_id, version) DO NOTHING;
