import { Hono } from "hono";
import { AppError } from "./errors";
import { safeEqual } from "./security";
import type { AppEnv } from "./types";

const webhooks = new Hono<AppEnv>();
const encoder = new TextEncoder();
const signatureToleranceSeconds = 5 * 60;
const maxWebhookBytes = 64 * 1024;
const recordedEvents = new Set([
  "email.sent",
  "email.delivered",
  "email.delivery_delayed",
  "email.bounced",
  "email.complained",
  "email.failed",
  "email.suppressed",
]);

function decodeWebhookSecret(secret: string): Uint8Array<ArrayBuffer> {
  if (!secret.startsWith("whsec_") || secret.length > 512) {
    throw new AppError(503, "webhook_not_configured", "Email delivery tracking is not configured.");
  }
  try {
    const encoded = secret.slice("whsec_".length).replace(/-/g, "+").replace(/_/g, "/");
    const binary = atob(encoded + "=".repeat((4 - encoded.length % 4) % 4));
    const bytes = new Uint8Array(binary.length);
    for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
    if (bytes.length < 16) throw new Error("short secret");
    return bytes;
  } catch {
    throw new AppError(503, "webhook_not_configured", "Email delivery tracking is not configured.");
  }
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

async function verifyWebhookSignature(
  secret: string,
  webhookID: string,
  timestamp: string,
  signatureHeader: string,
  body: string,
): Promise<boolean> {
  if (!/^[A-Za-z0-9_-]{8,128}$/.test(webhookID) || !/^\d{10}$/.test(timestamp)) return false;
  const timestampSeconds = Number(timestamp);
  if (!Number.isSafeInteger(timestampSeconds) ||
      Math.abs(Math.floor(Date.now() / 1000) - timestampSeconds) > signatureToleranceSeconds) return false;
  const key = await crypto.subtle.importKey(
    "raw",
    decodeWebhookSecret(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const expected = bytesToBase64(new Uint8Array(await crypto.subtle.sign(
    "HMAC",
    key,
    encoder.encode(`${webhookID}.${timestamp}.${body}`),
  )));
  return signatureHeader.split(/\s+/).some((candidate) => {
    const [version, signature] = candidate.split(",", 2);
    return version === "v1" && typeof signature === "string" && safeEqual(expected, signature);
  });
}

webhooks.post("/resend", async (c) => {
  const configuredSecret = c.env.RESEND_WEBHOOK_SECRET;
  if (!configuredSecret) {
    throw new AppError(503, "webhook_not_configured", "Email delivery tracking is not configured.");
  }
  const contentLength = Number(c.req.header("content-length") || 0);
  if (!Number.isFinite(contentLength) || contentLength < 0 || contentLength > maxWebhookBytes) {
    throw new AppError(413, "request_too_large", "The webhook request is too large.");
  }
  const body = await c.req.text();
  if (encoder.encode(body).length > maxWebhookBytes) {
    throw new AppError(413, "request_too_large", "The webhook request is too large.");
  }
  const webhookID = c.req.header("svix-id") || "";
  const timestamp = c.req.header("svix-timestamp") || "";
  const signature = c.req.header("svix-signature") || "";
  if (!await verifyWebhookSignature(configuredSecret, webhookID, timestamp, signature, body)) {
    throw new AppError(401, "invalid_webhook_signature", "The webhook signature is invalid.");
  }

  let payload: unknown;
  try {
    payload = JSON.parse(body);
  } catch {
    throw new AppError(400, "invalid_webhook_payload", "The webhook payload is invalid.");
  }
  if (!payload || typeof payload !== "object") {
    throw new AppError(400, "invalid_webhook_payload", "The webhook payload is invalid.");
  }
  const event = payload as { type?: unknown; created_at?: unknown; data?: { email_id?: unknown } };
  if (typeof event.type !== "string" || !recordedEvents.has(event.type)) {
    return c.json({ ok: true, ignored: true });
  }
  const messageID = event.data?.email_id;
  if (typeof messageID !== "string" || !/^[A-Za-z0-9_-]{8,128}$/.test(messageID)) {
    throw new AppError(400, "invalid_webhook_payload", "The webhook payload is invalid.");
  }
  const now = new Date().toISOString();
  const providerCreatedAt = typeof event.created_at === "string" && Number.isFinite(Date.parse(event.created_at))
    ? new Date(event.created_at).toISOString() : now;
  await c.env.DB.prepare(`INSERT INTO email_delivery_events
    (webhook_id,message_id,event_type,provider_created_at,recorded_at)
    VALUES(?,?,?,?,?) ON CONFLICT(webhook_id) DO NOTHING`).bind(
      webhookID, messageID, event.type, providerCreatedAt, now,
    ).run();
  return c.json({ ok: true });
});

export default webhooks;
