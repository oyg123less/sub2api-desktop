CREATE TABLE share_grants (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_uid TEXT NOT NULL,
  token_cipher TEXT NOT NULL,
  share_code TEXT NOT NULL UNIQUE,
  guest_key_hash TEXT NOT NULL UNIQUE,
  quota_requests INTEGER NOT NULL DEFAULT 0,
  used_requests INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT,
  revoked INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX idx_share_grants_owner ON share_grants(owner_id, created_at DESC, id DESC);
CREATE INDEX idx_share_grants_guest_key ON share_grants(guest_key_hash);
CREATE INDEX idx_share_grants_account ON share_grants(owner_id, account_uid, revoked);

CREATE TABLE share_usage_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  grant_id INTEGER NOT NULL REFERENCES share_grants(id) ON DELETE CASCADE,
  ts TEXT NOT NULL,
  model TEXT,
  status INTEGER NOT NULL,
  latency_ms INTEGER NOT NULL
);

CREATE INDEX idx_share_usage_grant ON share_usage_log(grant_id, ts DESC, id DESC);

UPDATE schema_version SET version = 2;
