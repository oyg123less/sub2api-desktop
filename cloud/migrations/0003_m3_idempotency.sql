CREATE TABLE vault_batch_receipts (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response_status INTEGER NOT NULL,
  response_body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  PRIMARY KEY(user_id, idempotency_key)
);

CREATE INDEX idx_vault_batch_receipts_expiry ON vault_batch_receipts(expires_at);

CREATE TABLE vault_write_claims (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  client_uid TEXT NOT NULL,
  base_version INTEGER NOT NULL,
  request_key TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  PRIMARY KEY(user_id, kind, client_uid, base_version)
);

CREATE INDEX idx_vault_write_claims_expiry ON vault_write_claims(expires_at);

ALTER TABLE share_grants ADD COLUMN reserved_requests INTEGER NOT NULL DEFAULT 0;

CREATE TABLE share_request_reservations (
  id TEXT PRIMARY KEY,
  grant_id INTEGER NOT NULL REFERENCES share_grants(id) ON DELETE CASCADE,
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending', 'settled', 'released')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE INDEX idx_share_reservations_expiry ON share_request_reservations(state, expires_at);

CREATE TRIGGER share_reservation_reserve BEFORE INSERT ON share_request_reservations
BEGIN
  SELECT RAISE(ABORT, 'share_reservation_unavailable') WHERE NOT EXISTS (
    SELECT 1 FROM share_grants
    WHERE id=NEW.grant_id AND revoked=0
      AND (expires_at IS NULL OR expires_at>NEW.created_at)
      AND (quota_requests=0 OR used_requests+reserved_requests<quota_requests)
  );
  UPDATE share_grants SET reserved_requests=reserved_requests+1,updated_at=NEW.created_at WHERE id=NEW.grant_id;
END;

CREATE TRIGGER share_reservation_settle AFTER UPDATE OF state ON share_request_reservations
WHEN OLD.state='pending' AND NEW.state='settled'
BEGIN
  UPDATE share_grants SET reserved_requests=MAX(0,reserved_requests-1),used_requests=used_requests+1,
    updated_at=NEW.updated_at WHERE id=NEW.grant_id;
END;

CREATE TRIGGER share_reservation_release AFTER UPDATE OF state ON share_request_reservations
WHEN OLD.state='pending' AND NEW.state='released'
BEGIN
  UPDATE share_grants SET reserved_requests=MAX(0,reserved_requests-1),updated_at=NEW.updated_at WHERE id=NEW.grant_id;
END;

CREATE TRIGGER share_reservation_delete AFTER DELETE ON share_request_reservations
WHEN OLD.state='pending'
BEGIN
  UPDATE share_grants SET reserved_requests=MAX(0,reserved_requests-1),updated_at=datetime('now') WHERE id=OLD.grant_id;
END;

UPDATE schema_version SET version = 3;
