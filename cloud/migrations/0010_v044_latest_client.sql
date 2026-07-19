UPDATE platform_settings
SET value='0.4.4',updated_at=datetime('now')
WHERE key='latest_client_version';

-- Keep minimum_client_version at 0.4.3 until the v0.4.4 installer is
-- published. Raise the minimum in a separate post-release operation so the
-- Worker deployment cannot lock users out before an upgrade is available.
