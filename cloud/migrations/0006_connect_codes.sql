CREATE TABLE share_connect_endpoints (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id INTEGER NOT NULL REFERENCES share_groups(id) ON DELETE CASCADE,
  connection_code TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK(status IN ('active','paused','deleted')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(owner_id)
);

CREATE INDEX idx_connect_endpoints_code ON share_connect_endpoints(connection_code,status);

CREATE TABLE share_connect_windows (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  endpoint_id INTEGER NOT NULL REFERENCES share_connect_endpoints(id) ON DELETE CASCADE,
  password_version INTEGER NOT NULL,
  password_salt TEXT NOT NULL,
  password_verifier TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('active','exhausted','expired','replaced','stopped')),
  max_claims INTEGER NOT NULL CHECK(max_claims BETWEEN 1 AND 20),
  claimed_count INTEGER NOT NULL DEFAULT 0 CHECK(claimed_count BETWEEN 0 AND max_claims),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(endpoint_id,password_version)
);

CREATE UNIQUE INDEX idx_connect_window_active ON share_connect_windows(endpoint_id) WHERE status='active';
CREATE INDEX idx_connect_windows_expiry ON share_connect_windows(status,expires_at);

CREATE TABLE share_connect_claims (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  window_id INTEGER NOT NULL REFERENCES share_connect_windows(id) ON DELETE CASCADE,
  recipient_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  recipient_grant_id INTEGER NOT NULL REFERENCES share_group_recipients(id) ON DELETE CASCADE,
  idempotency_key TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(window_id,recipient_user_id),
  UNIQUE(recipient_user_id,idempotency_key)
);

CREATE INDEX idx_connect_claims_recipient ON share_connect_claims(recipient_user_id,created_at DESC);

CREATE TRIGGER share_connect_claim_reserve BEFORE INSERT ON share_connect_claims
BEGIN
  SELECT RAISE(ABORT,'connect_window_unavailable') WHERE NOT EXISTS (
    SELECT 1 FROM share_connect_windows w
    JOIN share_connect_endpoints e ON e.id=w.endpoint_id
    JOIN share_groups g ON g.id=e.group_id
    WHERE w.id=NEW.window_id AND w.status='active' AND w.expires_at>NEW.created_at
      AND w.claimed_count<w.max_claims AND e.status='active' AND g.status='active'
  );
END;

CREATE TRIGGER share_connect_claim_count AFTER INSERT ON share_connect_claims
BEGIN
  UPDATE share_connect_windows
    SET claimed_count=claimed_count+1,
        status=CASE WHEN claimed_count+1>=max_claims THEN 'exhausted' ELSE status END,
        updated_at=NEW.created_at
    WHERE id=NEW.window_id;
END;

INSERT INTO platform_settings(key,value,updated_at) VALUES
  ('connect_codes_enabled','true',datetime('now')),
  ('enforce_client_version','false',datetime('now')),
  ('minimum_client_version','0.4.2',datetime('now')),
  ('latest_client_version','0.4.2',datetime('now')),
  ('client_release_url','https://github.com/oyg123less/sub2api-desktop/releases/latest',datetime('now'))
ON CONFLICT(key) DO NOTHING;
