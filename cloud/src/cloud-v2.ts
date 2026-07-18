import type { Context } from "hono";
import { AppError } from "./errors";
import { isBase64URLBytes, randomToken, sha256 } from "./security";
import type { AppEnv } from "./types";

const publicIDPattern = /^[a-z]{2,8}_[A-Za-z0-9_-]{12,64}$/;
const idempotencyPattern = /^[A-Za-z0-9._:-]{16,128}$/;
const controlCharacters = /[\u0000-\u001f\u007f]/;

export function newPublicID(prefix: string): string {
  return `${prefix}_${randomToken(15)}`;
}

export function parsePublicID(value: string, code = "invalid_id"): string {
  if (!publicIDPattern.test(value)) throw new AppError(400, code, "The resource ID is invalid.");
  return value;
}

export function cleanText(value: unknown, min: number, max: number, code: string, message: string): string {
  const text = typeof value === "string" ? value.trim() : "";
  if (text.length < min || text.length > max || controlCharacters.test(text)) {
    throw new AppError(400, code, message);
  }
  return text;
}

export function optionalText(value: unknown, max: number, code: string, message: string): string {
  if (value === undefined || value === null || value === "") return "";
  return cleanText(value, 0, max, code, message);
}

export function boundedInteger(value: unknown, fallback: number, min: number, max: number, code: string, message: string): number {
  const parsed = value === undefined || value === null || value === "" ? fallback : Number(value);
  if (!Number.isInteger(parsed) || parsed < min || parsed > max) throw new AppError(400, code, message);
  return parsed;
}

export function futureExpiry(value: unknown, fallback: string | null = null): string | null {
  if (value === undefined) return fallback;
  if (value === null || value === "") return null;
  if (typeof value !== "string") throw new AppError(400, "invalid_share_expiry", "The share expiry is invalid.");
  const parsed = Date.parse(value);
  const max = Date.now() + 366 * 24 * 60 * 60 * 1000;
  if (!Number.isFinite(parsed) || parsed <= Date.now() || parsed > max) {
    throw new AppError(400, "invalid_share_expiry", "The share expiry must be within the next year.");
  }
  return new Date(parsed).toISOString();
}

export function friendPair(left: number, right: number): { low: number; high: number; key: string } {
  if (!Number.isInteger(left) || !Number.isInteger(right) || left <= 0 || right <= 0 || left === right) {
    throw new AppError(400, "invalid_friend_target", "The friend target is invalid.");
  }
  const low = Math.min(left, right);
  const high = Math.max(left, right);
  return { low, high, key: `${low}:${high}` };
}

export async function requireFeature(c: Context<AppEnv>, key: string): Promise<void> {
  const setting = await c.env.DB.prepare("SELECT value FROM platform_settings WHERE key=?").bind(key).first<{ value: string }>();
  if (setting?.value === "false") throw new AppError(503, "feature_unavailable", "This cloud feature is temporarily unavailable.");
}

export function publicOrigin(c: Context<AppEnv>): string {
  return new URL(c.req.url).origin;
}

export function validateEncryptionPublicKey(value: unknown): string {
  if (!isBase64URLBytes(value, 32)) {
    throw new AppError(400, "invalid_encryption_public_key", "The account encryption public key is invalid.");
  }
  return value;
}

export function validateDevicePublicKey(value: unknown): string {
  if (!isBase64URLBytes(value, 32)) {
    throw new AppError(400, "invalid_device_public_key", "The device signing public key is invalid.");
  }
  return value;
}

export interface KeyMaterialInput {
  key_prefix: string;
  guest_key_hash: string;
  key_envelope: string;
  envelope_context: string;
  recipient_key_version: number;
}

export function validateKeyMaterial(value: unknown): KeyMaterialInput {
  if (!value || typeof value !== "object") throw new AppError(400, "invalid_key_material", "The access key material is invalid.");
  const input = value as Record<string, unknown>;
  const keyPrefix = typeof input.key_prefix === "string" ? input.key_prefix.trim() : "";
  if (!/^sk-amber-[A-Za-z0-9_-]{4,24}$/.test(keyPrefix) || !isBase64URLBytes(input.guest_key_hash, 32)) {
    throw new AppError(400, "invalid_key_material", "The access key material is invalid.");
  }
  const envelope = typeof input.key_envelope === "string" ? input.key_envelope : JSON.stringify(input.key_envelope ?? null);
  const envelopeContext = typeof input.envelope_context === "string" ? input.envelope_context.trim() : "";
  if (!/^ctx_[A-Za-z0-9_-]{20,64}$/.test(envelopeContext)) {
    throw new AppError(400, "invalid_key_envelope", "The access key envelope context is invalid.");
  }
  if (envelope.length < 80 || envelope.length > 4096) {
    throw new AppError(400, "invalid_key_envelope", "The access key envelope is invalid.");
  }
  try {
    const parsed = JSON.parse(envelope) as Record<string, unknown>;
    if (parsed.version !== 1 || parsed.algorithm !== "X25519-HKDF-SHA256-AES-256-GCM" ||
        !isBase64URLBytes(parsed.ephemeral_public_key, 32) || !isBase64URLBytes(parsed.salt, 16) ||
        !isBase64URLBytes(parsed.nonce, 12) || typeof parsed.ciphertext !== "string" ||
        parsed.ciphertext.length < 22 || parsed.ciphertext.length > 2048) {
      throw new Error("invalid envelope");
    }
  } catch {
    throw new AppError(400, "invalid_key_envelope", "The access key envelope is invalid.");
  }
  const keyVersion = Number(input.recipient_key_version);
  if (!Number.isInteger(keyVersion) || keyVersion < 1 || keyVersion > 1_000_000) {
    throw new AppError(400, "invalid_key_envelope", "The access key envelope version is invalid.");
  }
  return {
    key_prefix: keyPrefix,
    guest_key_hash: input.guest_key_hash as string,
    key_envelope: envelope,
    envelope_context: envelopeContext,
    recipient_key_version: keyVersion,
  };
}

export async function writeShareAudit(
  c: Context<AppEnv>, groupID: number | null, action: string, targetType: string, targetPublicID: string,
  details: Record<string, unknown> = {},
): Promise<void> {
  await c.env.DB.prepare(`INSERT INTO share_audit_log
    (group_id,actor_user_id,action,target_type,target_public_id,details,created_at) VALUES(?,?,?,?,?,?,?)`)
    .bind(groupID, c.get("auth").id, action, targetType, targetPublicID, JSON.stringify(details), new Date().toISOString()).run();
}

interface MutationReceipt {
  request_hash: string;
  response_status: number;
  response_body: string;
  expires_at: string;
}

export async function mutationReceipt(
  c: Context<AppEnv>, operation: string, requestBody: unknown,
): Promise<{ key: string; requestHash: string; replay?: Response }> {
  const key = c.req.header("idempotency-key") || "";
  if (!idempotencyPattern.test(key)) {
    throw new AppError(400, "idempotency_key_required", "A valid Idempotency-Key header is required.");
  }
  const requestHash = await sha256(JSON.stringify(requestBody));
  const existing = await c.env.DB.prepare(`SELECT request_hash,response_status,response_body,expires_at
    FROM cloud_mutation_receipts WHERE user_id=? AND operation=? AND idempotency_key=?`)
    .bind(c.get("auth").id, operation, key).first<MutationReceipt>();
  if (existing && Date.parse(existing.expires_at) > Date.now()) {
    if (existing.request_hash !== requestHash) {
      throw new AppError(409, "idempotency_key_reused", "The idempotency key was already used for a different request.");
    }
    const headers = new Headers({ "Content-Type": "application/json", "Idempotency-Replayed": "true" });
    return { key, requestHash, replay: new Response(existing.response_body, { status: existing.response_status, headers }) };
  }
  if (existing) {
    await c.env.DB.prepare("DELETE FROM cloud_mutation_receipts WHERE user_id=? AND operation=? AND idempotency_key=?")
      .bind(c.get("auth").id, operation, key).run();
  }
  c.executionCtx.waitUntil(c.env.DB.prepare("DELETE FROM cloud_mutation_receipts WHERE expires_at<=?")
    .bind(new Date().toISOString()).run());
  return { key, requestHash };
}

export function receiptStatement(
  c: Context<AppEnv>, operation: string, key: string, requestHash: string, status: number, responseBody: string,
): D1PreparedStatement {
  const now = new Date();
  return c.env.DB.prepare(`INSERT INTO cloud_mutation_receipts
    (user_id,operation,idempotency_key,request_hash,response_status,response_body,created_at,expires_at)
    VALUES(?,?,?,?,?,?,?,?)`).bind(
      c.get("auth").id, operation, key, requestHash, status, responseBody, now.toISOString(),
      new Date(now.getTime() + 7 * 24 * 60 * 60 * 1000).toISOString(),
    );
}
