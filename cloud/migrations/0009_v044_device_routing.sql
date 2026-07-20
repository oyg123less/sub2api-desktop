ALTER TABLE share_connect_endpoints ADD COLUMN default_host_device_id INTEGER REFERENCES share_devices(id) ON DELETE SET NULL;
ALTER TABLE share_connect_endpoints ADD COLUMN route_policy TEXT NOT NULL DEFAULT 'legacy_owner' CHECK(route_policy IN ('legacy_owner','fixed_device','primary_backup'));

CREATE TABLE share_endpoint_devices (
  endpoint_id INTEGER NOT NULL REFERENCES share_connect_endpoints(id) ON DELETE CASCADE,
  device_id INTEGER NOT NULL REFERENCES share_devices(id) ON DELETE CASCADE,
  priority INTEGER NOT NULL DEFAULT 100,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(endpoint_id,device_id)
);

CREATE INDEX idx_share_endpoint_devices_route
  ON share_endpoint_devices(endpoint_id,enabled,priority,device_id);

CREATE TABLE share_group_account_hosts (
  group_account_id INTEGER NOT NULL REFERENCES share_group_accounts(id) ON DELETE CASCADE,
  device_id INTEGER NOT NULL REFERENCES share_devices(id) ON DELETE CASCADE,
  priority INTEGER NOT NULL DEFAULT 100,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(group_account_id,device_id)
);

CREATE INDEX idx_share_group_account_hosts_route
  ON share_group_account_hosts(group_account_id,enabled,priority,device_id);

UPDATE share_connect_endpoints
SET default_host_device_id=(
  SELECT d.id FROM share_devices d
  WHERE d.user_id=share_connect_endpoints.owner_id AND d.revoked=0
  ORDER BY d.is_primary DESC,d.updated_at DESC,d.id ASC LIMIT 1
)
WHERE default_host_device_id IS NULL;

INSERT OR IGNORE INTO share_endpoint_devices(endpoint_id,device_id,priority,enabled,created_at,updated_at)
SELECT e.id,e.default_host_device_id,0,1,datetime('now'),datetime('now')
FROM share_connect_endpoints e WHERE e.default_host_device_id IS NOT NULL;

INSERT OR IGNORE INTO share_group_account_hosts(group_account_id,device_id,priority,enabled,created_at,updated_at)
SELECT a.id,e.default_host_device_id,0,1,datetime('now'),datetime('now')
FROM share_group_accounts a
JOIN share_connect_endpoints e ON e.group_id=a.group_id
WHERE a.relay_mode='owner_device' AND e.default_host_device_id IS NOT NULL;

UPDATE schema_version SET version=9;
