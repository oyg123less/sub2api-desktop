CREATE TABLE friend_profiles (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  friend_code TEXT NOT NULL UNIQUE,
  encryption_public_key TEXT NOT NULL,
  encryption_private_cipher TEXT NOT NULL,
  encryption_key_version INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX idx_friend_profiles_code ON friend_profiles(friend_code);

CREATE TABLE friend_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  sender_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  receiver_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  pair_key TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending','accepted','declined','cancelled','expired')),
  created_at TEXT NOT NULL,
  responded_at TEXT,
  expires_at TEXT NOT NULL,
  CHECK(sender_id <> receiver_id)
);

CREATE UNIQUE INDEX idx_friend_requests_pending_pair ON friend_requests(pair_key) WHERE status='pending';
CREATE INDEX idx_friend_requests_receiver ON friend_requests(receiver_id,status,created_at DESC);
CREATE INDEX idx_friend_requests_sender ON friend_requests(sender_id,status,created_at DESC);

CREATE TABLE friendships (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  user_low_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  user_high_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK(status IN ('active','removed')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK(user_low_id < user_high_id),
  UNIQUE(user_low_id,user_high_id)
);

CREATE INDEX idx_friendships_low ON friendships(user_low_id,status,updated_at DESC);
CREATE INDEX idx_friendships_high ON friendships(user_high_id,status,updated_at DESC);

CREATE TABLE friendship_aliases (
  friendship_id INTEGER NOT NULL REFERENCES friendships(id) ON DELETE CASCADE,
  owner_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  alias TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(friendship_id,owner_user_id)
);

CREATE TABLE friend_blocks (
  blocker_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  blocked_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  CHECK(blocker_id <> blocked_id),
  PRIMARY KEY(blocker_id,blocked_id)
);

CREATE INDEX idx_friend_blocks_blocked ON friend_blocks(blocked_id,blocker_id);

CREATE TABLE share_groups (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK(status IN ('active','paused','deleted')),
  route_policy TEXT NOT NULL CHECK(route_policy IN ('balanced','failover')),
  default_rpm INTEGER NOT NULL,
  default_concurrency INTEGER NOT NULL,
  default_quota_requests INTEGER NOT NULL DEFAULT 0,
  default_expires_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT
);

CREATE INDEX idx_share_groups_owner ON share_groups(owner_id,status,updated_at DESC);

CREATE TABLE share_group_accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  group_id INTEGER NOT NULL REFERENCES share_groups(id) ON DELETE CASCADE,
  account_uid TEXT NOT NULL,
  account_type TEXT NOT NULL CHECK(account_type IN ('oauth','api_key')),
  relay_mode TEXT NOT NULL CHECK(relay_mode IN ('owner_device','worker_direct')),
  priority INTEGER NOT NULL DEFAULT 100,
  weight INTEGER NOT NULL DEFAULT 100,
  enabled INTEGER NOT NULL DEFAULT 1,
  token_cipher TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK((relay_mode='owner_device' AND token_cipher IS NULL) OR (relay_mode='worker_direct' AND account_type='api_key' AND token_cipher IS NOT NULL)),
  UNIQUE(group_id,account_uid)
);

CREATE INDEX idx_share_group_accounts_route ON share_group_accounts(group_id,enabled,priority,id);

CREATE TABLE share_group_recipients (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  group_id INTEGER NOT NULL REFERENCES share_groups(id) ON DELETE CASCADE,
  recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  generation INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL CHECK(status IN ('pending','active','paused','declined','expired','revoked','left')),
  rpm_limit INTEGER NOT NULL,
  concurrency_limit INTEGER NOT NULL,
  quota_requests INTEGER NOT NULL DEFAULT 0,
  used_requests INTEGER NOT NULL DEFAULT 0,
  reserved_requests INTEGER NOT NULL DEFAULT 0,
  expires_at TEXT,
  created_at TEXT NOT NULL,
  accepted_at TEXT,
  updated_at TEXT NOT NULL,
  UNIQUE(group_id,recipient_id,generation)
);

CREATE UNIQUE INDEX idx_share_recipients_current ON share_group_recipients(group_id,recipient_id)
  WHERE status IN ('pending','active','paused');
CREATE INDEX idx_share_recipients_user ON share_group_recipients(recipient_id,status,updated_at DESC);
CREATE INDEX idx_share_recipients_group ON share_group_recipients(group_id,status,updated_at DESC);

CREATE TABLE share_access_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  recipient_grant_id INTEGER NOT NULL REFERENCES share_group_recipients(id) ON DELETE CASCADE,
  key_version INTEGER NOT NULL,
  key_prefix TEXT NOT NULL,
  guest_key_hash TEXT NOT NULL UNIQUE,
  key_envelope TEXT NOT NULL,
  envelope_context TEXT NOT NULL UNIQUE,
  recipient_key_version INTEGER NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('prepared','active','replaced','revoked','expired')),
  created_at TEXT NOT NULL,
  activated_at TEXT,
  revoked_at TEXT,
  UNIQUE(recipient_grant_id,key_version)
);

CREATE UNIQUE INDEX idx_share_access_keys_active ON share_access_keys(recipient_grant_id) WHERE status='active';
CREATE INDEX idx_share_access_keys_hash ON share_access_keys(guest_key_hash,status);

CREATE TABLE share_devices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  public_id TEXT NOT NULL UNIQUE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  device_public_key TEXT NOT NULL,
  capabilities TEXT NOT NULL,
  is_primary INTEGER NOT NULL DEFAULT 0,
  revoked INTEGER NOT NULL DEFAULT 0,
  last_seen_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_share_devices_primary ON share_devices(user_id) WHERE is_primary=1 AND revoked=0;
CREATE INDEX idx_share_devices_user ON share_devices(user_id,revoked,updated_at DESC);

CREATE TABLE share_device_sessions (
  id TEXT PRIMARY KEY,
  device_id INTEGER NOT NULL REFERENCES share_devices(id) ON DELETE CASCADE,
  connected_at TEXT NOT NULL,
  last_heartbeat_at TEXT NOT NULL,
  disconnected_at TEXT,
  close_reason TEXT
);

CREATE INDEX idx_share_device_sessions_device ON share_device_sessions(device_id,connected_at DESC);

CREATE TABLE share_device_challenges (
  challenge_hash TEXT PRIMARY KEY,
  device_id INTEGER NOT NULL REFERENCES share_devices(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX idx_share_device_challenges_expiry ON share_device_challenges(expires_at,consumed_at);

CREATE TABLE share_request_reservations_v2 (
  id TEXT PRIMARY KEY,
  recipient_grant_id INTEGER NOT NULL REFERENCES share_group_recipients(id) ON DELETE CASCADE,
  access_key_id INTEGER NOT NULL REFERENCES share_access_keys(id) ON DELETE CASCADE,
  group_account_id INTEGER REFERENCES share_group_accounts(id) ON DELETE SET NULL,
  state TEXT NOT NULL CHECK(state IN ('pending','settled','released')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE INDEX idx_share_reservations_v2_expiry ON share_request_reservations_v2(state,expires_at);

CREATE TRIGGER share_reservation_v2_reserve BEFORE INSERT ON share_request_reservations_v2
BEGIN
  SELECT RAISE(ABORT,'share_reservation_unavailable') WHERE NOT EXISTS (
    SELECT 1 FROM share_group_recipients r
    JOIN share_groups g ON g.id=r.group_id
    JOIN share_access_keys k ON k.id=NEW.access_key_id AND k.recipient_grant_id=r.id
    WHERE r.id=NEW.recipient_grant_id AND r.status='active' AND g.status='active' AND k.status='active'
      AND (r.expires_at IS NULL OR r.expires_at>NEW.created_at)
      AND (r.quota_requests=0 OR r.used_requests+r.reserved_requests<r.quota_requests)
  );
  UPDATE share_group_recipients SET reserved_requests=reserved_requests+1,updated_at=NEW.created_at
    WHERE id=NEW.recipient_grant_id;
END;

CREATE TRIGGER share_reservation_v2_settle AFTER UPDATE OF state ON share_request_reservations_v2
WHEN OLD.state='pending' AND NEW.state='settled'
BEGIN
  UPDATE share_group_recipients SET reserved_requests=MAX(0,reserved_requests-1),used_requests=used_requests+1,
    updated_at=NEW.updated_at WHERE id=NEW.recipient_grant_id;
END;

CREATE TRIGGER share_reservation_v2_release AFTER UPDATE OF state ON share_request_reservations_v2
WHEN OLD.state='pending' AND NEW.state='released'
BEGIN
  UPDATE share_group_recipients SET reserved_requests=MAX(0,reserved_requests-1),updated_at=NEW.updated_at
    WHERE id=NEW.recipient_grant_id;
END;

CREATE TRIGGER share_reservation_v2_delete AFTER DELETE ON share_request_reservations_v2
WHEN OLD.state='pending'
BEGIN
  UPDATE share_group_recipients SET reserved_requests=MAX(0,reserved_requests-1),updated_at=datetime('now')
    WHERE id=OLD.recipient_grant_id;
END;

CREATE TABLE share_usage_log_v2 (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  request_id TEXT NOT NULL UNIQUE,
  group_id INTEGER NOT NULL REFERENCES share_groups(id) ON DELETE CASCADE,
  recipient_grant_id INTEGER NOT NULL REFERENCES share_group_recipients(id) ON DELETE CASCADE,
  group_account_id INTEGER REFERENCES share_group_accounts(id) ON DELETE SET NULL,
  device_id INTEGER REFERENCES share_devices(id) ON DELETE SET NULL,
  route_mode TEXT NOT NULL CHECK(route_mode IN ('worker_direct','owner_device')),
  model TEXT,
  status INTEGER NOT NULL,
  error_code TEXT,
  input_tokens INTEGER,
  output_tokens INTEGER,
  latency_ms INTEGER NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX idx_share_usage_v2_group ON share_usage_log_v2(group_id,created_at DESC,id DESC);
CREATE INDEX idx_share_usage_v2_recipient ON share_usage_log_v2(recipient_grant_id,created_at DESC,id DESC);

CREATE TABLE share_audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER REFERENCES share_groups(id) ON DELETE SET NULL,
  actor_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_public_id TEXT NOT NULL,
  details TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX idx_share_audit_group ON share_audit_log(group_id,created_at DESC,id DESC);
CREATE INDEX idx_share_audit_actor ON share_audit_log(actor_user_id,created_at DESC,id DESC);

CREATE TABLE cloud_mutation_receipts (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  operation TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response_status INTEGER NOT NULL,
  response_body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  PRIMARY KEY(user_id,operation,idempotency_key)
);

CREATE INDEX idx_cloud_mutation_receipts_expiry ON cloud_mutation_receipts(expires_at);

INSERT INTO platform_settings(key,value,updated_at) VALUES
  ('friends_enabled','false',datetime('now')),
  ('share_groups_enabled','false',datetime('now')),
  ('owner_relay_enabled','false',datetime('now'))
ON CONFLICT(key) DO NOTHING;

UPDATE schema_version SET version=4;
