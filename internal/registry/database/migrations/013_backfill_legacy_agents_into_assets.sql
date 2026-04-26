-- Backfill native assets from existing legacy agents.
INSERT INTO assets (asset_id, version, category, status, published_at, updated_at, is_latest, value)
SELECT
    agent_name AS asset_id,
    version,
    'agent' AS category,
    status,
    published_at,
    updated_at,
    is_latest,
    jsonb_strip_nulls(jsonb_build_object(
        'id', agent_name,
        'name', COALESCE(NULLIF(value->>'name', ''), agent_name),
        'description', COALESCE(NULLIF(value->>'description', ''), agent_name),
        'version', version,
        'category', 'agent',
        'sourceSkill', jsonb_build_object(
            'path', 'SKILL.md',
            'body', CONCAT_WS(E'\n\n',
                '# ' || COALESCE(NULLIF(value->>'title', ''), NULLIF(value->>'name', ''), agent_name),
                NULLIF(value->>'description', '')
            ),
            'bodyFormat', 'markdown'
        ),
        'manifest', jsonb_strip_nulls(jsonb_build_object(
            'schemaVersion', 'shub.asset/v1alpha1',
            'id', agent_name,
            'category', 'agent',
            'name', COALESCE(NULLIF(value->>'name', ''), agent_name),
            'description', COALESCE(NULLIF(value->>'description', ''), agent_name),
            'version', version,
            'sourceSkill', jsonb_build_object(
                'path', 'SKILL.md',
                'body', CONCAT_WS(E'\n\n',
                    '# ' || COALESCE(NULLIF(value->>'title', ''), NULLIF(value->>'name', ''), agent_name),
                    NULLIF(value->>'description', '')
                ),
                'bodyFormat', 'markdown'
            ),
            'entry', jsonb_strip_nulls(jsonb_build_object(
                'kind', CASE
                    WHEN NULLIF(value->>'image', '') IS NOT NULL THEN 'image'
                    WHEN NULLIF(value->'packages'->0->>'identifier', '') IS NOT NULL THEN CASE
                        WHEN LOWER(COALESCE(NULLIF(value->'packages'->0->>'registryType', ''), '')) IN ('docker', 'oci') THEN 'image'
                        WHEN NULLIF(value->'packages'->0->>'registryType', '') IS NOT NULL THEN LOWER(value->'packages'->0->>'registryType')
                        ELSE 'package'
                    END
                    WHEN NULLIF(value->'remotes'->0->>'url', '') IS NOT NULL THEN 'remote'
                    WHEN NULLIF(value->'repository'->>'url', '') IS NOT NULL THEN 'repository'
                    ELSE 'agent-manifest'
                END,
                'path', COALESCE(
                    NULLIF(value->>'image', ''),
                    NULLIF(value->'packages'->0->>'identifier', ''),
                    NULLIF(value->'remotes'->0->>'url', ''),
                    NULLIF(value->'repository'->>'url', ''),
                    'agent.json'
                )
            )),
            'runtime', jsonb_strip_nulls(jsonb_build_object(
                'type', CASE
                    WHEN NULLIF(value->>'language', '') IS NOT NULL THEN LOWER(value->>'language')
                    WHEN NULLIF(value->>'image', '') IS NOT NULL THEN 'container'
                    WHEN NULLIF(value->'packages'->0->>'registryType', '') IS NOT NULL THEN LOWER(value->'packages'->0->>'registryType')
                    WHEN NULLIF(value->>'framework', '') IS NOT NULL THEN LOWER(value->>'framework')
                    ELSE 'agent'
                END
            )),
            'metadata', jsonb_build_object(
                'legacyAgent', jsonb_strip_nulls(jsonb_build_object(
                    'agent', value,
                    'mcpServerRefs', COALESCE(mcp_server_refs, '[]'::jsonb),
                    'skillRefs', COALESCE(skill_refs, '[]'::jsonb),
                    'promptRefs', COALESCE(prompt_refs, '[]'::jsonb)
                ))
            )
        )),
        'source', jsonb_strip_nulls(jsonb_build_object(
            'repositoryUrl', NULLIF(value->'repository'->>'url', ''),
            'packageType', CASE
                WHEN NULLIF(value->>'image', '') IS NOT NULL THEN 'docker'
                ELSE NULLIF(value->'packages'->0->>'registryType', '')
            END,
            'packageRef', COALESCE(
                NULLIF(value->'packages'->0->>'identifier', ''),
                NULLIF(value->>'image', '')
            )
        )),
        'status', status
    ))
FROM agents
ON CONFLICT (asset_id, version) DO NOTHING;
