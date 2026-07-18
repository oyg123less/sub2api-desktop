import { env, SELF } from "cloudflare:test";
import { describe, expect, it } from "vitest";

const webhookSecret = "whsec_dGVzdC1yZXNlbmQtd2ViaG9vay1zZWNyZXQtMzItYnl0ZXM=";

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

async function signature(webhookID: string, timestamp: string, body: string): Promise<string> {
  const binary = atob(webhookSecret.slice("whsec_".length));
  const secret = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  const key = await crypto.subtle.importKey("raw", secret, { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const signed = await crypto.subtle.sign(
    "HMAC",
    key,
    new TextEncoder().encode(`${webhookID}.${timestamp}.${body}`),
  );
  return `v1,${bytesToBase64(new Uint8Array(signed))}`;
}

async function webhookRequest(body: string, webhookID: string, timestamp: string, suppliedSignature?: string) {
  return SELF.fetch("https://amber.test/v1/webhooks/resend", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "svix-id": webhookID,
      "svix-timestamp": timestamp,
      "svix-signature": suppliedSignature ?? await signature(webhookID, timestamp, body),
    },
    body,
  });
}

describe("Resend webhooks", () => {
  it("verifies, minimizes, and deduplicates delivery events", async () => {
    const webhookID = "msg_webhook_delivery_001";
    const messageID = "b4390394-1a2b-4173-8751-bdd000000001";
    const timestamp = String(Math.floor(Date.now() / 1000));
    const body = JSON.stringify({
      type: "email.delivered",
      created_at: new Date(Number(timestamp) * 1000).toISOString(),
      data: { email_id: messageID, to: ["private-recipient@example.test"], subject: "private code" },
    });
    const first = await webhookRequest(body, webhookID, timestamp);
    expect(first.status).toBe(200);
    expect(await first.json()).toEqual({ ok: true });
    const replay = await webhookRequest(body, webhookID, timestamp);
    expect(replay.status).toBe(200);
    const rows = await env.DB.prepare("SELECT * FROM email_delivery_events WHERE webhook_id=?")
      .bind(webhookID).all<Record<string, unknown>>();
    expect(rows.results).toHaveLength(1);
    expect(rows.results[0]).toMatchObject({ message_id: messageID, event_type: "email.delivered" });
    expect(JSON.stringify(rows.results[0])).not.toContain("private-recipient");
    expect(JSON.stringify(rows.results[0])).not.toContain("private code");
  });

  it("rejects invalid and expired signatures", async () => {
    const timestamp = String(Math.floor(Date.now() / 1000));
    const body = JSON.stringify({
      type: "email.sent",
      created_at: new Date().toISOString(),
      data: { email_id: "b4390394-1a2b-4173-8751-bdd000000002" },
    });
    const invalid = await webhookRequest(body, "msg_webhook_invalid_001", timestamp, "v1,invalid");
    expect(invalid.status).toBe(401);
    const oldTimestamp = String(Math.floor(Date.now() / 1000) - 301);
    const expired = await webhookRequest(body, "msg_webhook_expired_001", oldTimestamp);
    expect(expired.status).toBe(401);
  });

  it("acknowledges untracked event types without storing them", async () => {
    const webhookID = "msg_webhook_opened_001";
    const timestamp = String(Math.floor(Date.now() / 1000));
    const body = JSON.stringify({
      type: "email.opened",
      created_at: new Date().toISOString(),
      data: { email_id: "b4390394-1a2b-4173-8751-bdd000000003" },
    });
    const response = await webhookRequest(body, webhookID, timestamp);
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ ok: true, ignored: true });
    const row = await env.DB.prepare("SELECT id FROM email_delivery_events WHERE webhook_id=?")
      .bind(webhookID).first();
    expect(row).toBeNull();
  });
});
