import { Hono } from "hono";
import { requireAuth } from "./auth-middleware";
import { AppError, readJSON, requireJSONSize } from "./errors";
import { encryptShareCredential, validateShareCredential } from "./share-crypto";
import { randomToken, sha256 } from "./security";
import type { AppEnv } from "./types";

const shares = new Hono<AppEnv>();
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const shareAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";

interface ShareRow {
  id: number;
  owner_id: number;
  account_uid: string;
  share_code: string;
  quota_requests: number;
  used_requests: number;
  expires_at: string | null;
  revoked: number;
  created_at: string;
  updated_at: string;
}

function publicShare(row: ShareRow, origin: string) {
  return {
    id: row.id,
    account_uid: row.account_uid,
    share_code: row.share_code,
    quota_requests: row.quota_requests,
    used_requests: row.used_requests,
    expires_at: row.expires_at,
    revoked: Boolean(row.revoked),
    created_at: row.created_at,
    updated_at: row.updated_at,
    base_url: `${origin}/v1`,
  };
}

function parseQuota(value: unknown): number {
  const quota = value === undefined ? 0 : Number(value);
  if (!Number.isInteger(quota) || quota < 0 || quota > 1_000_000) {
    throw new AppError(400, "invalid_share_quota", "The request quota must be between 0 and 1000000.");
  }
  return quota;
}

function parseExpiry(value: unknown): string | null {
  if (value === undefined || value === null || value === "") return null;
  if (typeof value !== "string") throw new AppError(400, "invalid_share_expiry", "The share expiry is invalid.");
  const date = new Date(value);
  const max = Date.now() + 366 * 24 * 60 * 60 * 1000;
  if (Number.isNaN(date.getTime()) || date.getTime() <= Date.now() || date.getTime() > max) {
    throw new AppError(400, "invalid_share_expiry", "The share expiry must be within the next year.");
  }
  return date.toISOString();
}

function parseShareID(value: string): number {
  const id = Number(value);
  if (!Number.isInteger(id) || id <= 0) throw new AppError(400, "invalid_share_id", "The share ID is invalid.");
  return id;
}

function newShareCode(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  return Array.from(bytes, (byte) => shareAlphabet[byte % shareAlphabet.length]).join("");
}

async function uniqueShareCode(env: AppEnv["Bindings"]): Promise<string> {
  for (let attempt = 0; attempt < 8; attempt += 1) {
    const code = newShareCode();
    const existing = await env.DB.prepare("SELECT id FROM share_grants WHERE share_code=?").bind(code).first();
    if (!existing) return code;
  }
  throw new AppError(503, "share_code_unavailable", "A share code could not be generated.");
}

shares.use("/*", requireAuth);

shares.get("/", async (c) => {
  const result = await c.env.DB.prepare(`SELECT id,owner_id,account_uid,share_code,quota_requests,used_requests,
    expires_at,revoked,created_at,updated_at FROM share_grants WHERE owner_id=? ORDER BY created_at DESC,id DESC`)
    .bind(c.get("auth").id).all<ShareRow>();
  return c.json({ shares: result.results.map((row) => publicShare(row, new URL(c.req.url).origin)) });
});

shares.post("/", async (c) => {
  requireJSONSize(c, 64 * 1024);
  const body = await readJSON<Record<string, unknown>>(c);
  const accountUID = typeof body.account_uid === "string" ? body.account_uid.trim() : "";
  if (!uuidPattern.test(accountUID)) throw new AppError(400, "invalid_account_uid", "The account ID is invalid.");
  if (body.consent !== true) throw new AppError(400, "share_consent_required", "Confirm cloud custody for this shared account token.");
  const credential = validateShareCredential(body.credential);
  const quota = parseQuota(body.quota_requests);
  const expiresAt = parseExpiry(body.expires_at);
  const owner = c.get("auth");
  const account = await c.env.DB.prepare(`SELECT id FROM vault_items
    WHERE user_id=? AND kind='account' AND client_uid=? AND deleted=0`).bind(owner.id, accountUID).first();
  if (!account) throw new AppError(409, "account_not_synced", "Sync this account to Amber Cloud before sharing it.");

  const guestKey = `sk-share-${randomToken(32)}`;
  const shareCode = await uniqueShareCode(c.env);
  const tokenCipher = await encryptShareCredential(c.env, owner.id, accountUID, credential);
  const now = new Date().toISOString();
  const result = await c.env.DB.prepare(`INSERT INTO share_grants
    (owner_id,account_uid,token_cipher,share_code,guest_key_hash,quota_requests,expires_at,created_at,updated_at)
    VALUES(?,?,?,?,?,?,?,?,?)`).bind(
      owner.id, accountUID, tokenCipher, shareCode, await sha256(guestKey), quota, expiresAt, now, now,
    ).run();
  const row = await c.env.DB.prepare(`SELECT id,owner_id,account_uid,share_code,quota_requests,used_requests,
    expires_at,revoked,created_at,updated_at FROM share_grants WHERE id=?`).bind(result.meta.last_row_id).first<ShareRow>();
  if (!row) throw new AppError(500, "share_create_failed", "The share could not be created.");
  return c.json({ share: publicShare(row, new URL(c.req.url).origin), guest_key: guestKey }, 201);
});

shares.patch("/:id", async (c) => {
  const id = parseShareID(c.req.param("id"));
  const body = await readJSON<Record<string, unknown>>(c);
  const updates: string[] = [];
  const values: unknown[] = [];
  if (typeof body.revoked === "boolean") {
    updates.push("revoked=?");
    values.push(body.revoked ? 1 : 0);
  }
  if (body.quota_requests !== undefined) {
    updates.push("quota_requests=?");
    values.push(parseQuota(body.quota_requests));
  }
  if (body.expires_at !== undefined) {
    updates.push("expires_at=?");
    values.push(parseExpiry(body.expires_at));
  }
  if (!updates.length) throw new AppError(400, "invalid_share_update", "No supported share changes were provided.");
  updates.push("updated_at=?");
  values.push(new Date().toISOString(), id, c.get("auth").id);
  const restoring = body.revoked === false;
  const accountGuard = restoring ? ` AND EXISTS (SELECT 1 FROM vault_items v
    WHERE v.user_id=share_grants.owner_id AND v.kind='account' AND v.client_uid=share_grants.account_uid AND v.deleted=0)` : "";
  const result = await c.env.DB.prepare(`UPDATE share_grants SET ${updates.join(",")} WHERE id=? AND owner_id=?${accountGuard}`).bind(...values).run();
  if (!result.meta.changes) {
    const existing = await c.env.DB.prepare("SELECT id FROM share_grants WHERE id=? AND owner_id=?").bind(id, c.get("auth").id).first();
    if (!existing) throw new AppError(404, "share_not_found", "The share was not found.");
    if (restoring) throw new AppError(409, "share_account_deleted", "The deleted account's share cannot be restored.");
    throw new AppError(404, "share_not_found", "The share was not found.");
  }
  const row = await c.env.DB.prepare(`SELECT id,owner_id,account_uid,share_code,quota_requests,used_requests,
    expires_at,revoked,created_at,updated_at FROM share_grants WHERE id=?`).bind(id).first<ShareRow>();
  return c.json({ share: publicShare(row!, new URL(c.req.url).origin) });
});

shares.get("/:id/usage", async (c) => {
  const id = parseShareID(c.req.param("id"));
  const owned = await c.env.DB.prepare("SELECT id FROM share_grants WHERE id=? AND owner_id=?").bind(id, c.get("auth").id).first();
  if (!owned) throw new AppError(404, "share_not_found", "The share was not found.");
  const result = await c.env.DB.prepare(`SELECT id,ts,model,status,latency_ms FROM share_usage_log
    WHERE grant_id=? ORDER BY ts DESC,id DESC LIMIT 200`).bind(id).all();
  return c.json({ usage: result.results });
});

export default shares;
