UPDATE platform_settings SET value='0.4.3',updated_at=datetime('now')
WHERE key IN ('minimum_client_version','latest_client_version');
