CREATE TABLE email_delivery_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  webhook_id TEXT NOT NULL UNIQUE,
  message_id TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK(event_type IN (
    'email.sent','email.delivered','email.delivery_delayed','email.bounced',
    'email.complained','email.failed','email.suppressed'
  )),
  provider_created_at TEXT NOT NULL,
  recorded_at TEXT NOT NULL
);

CREATE INDEX idx_email_delivery_message ON email_delivery_events(message_id,provider_created_at DESC,id DESC);
CREATE INDEX idx_email_delivery_type ON email_delivery_events(event_type,provider_created_at DESC,id DESC);

UPDATE schema_version SET version=5;
