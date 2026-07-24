UPDATE platform_settings
SET value='0.4.5',updated_at=datetime('now')
WHERE key='latest_client_version';

-- Keep minimum_client_version unchanged until the v0.4.5 installer is
-- published and verified. Raising the minimum is a separate release action.
