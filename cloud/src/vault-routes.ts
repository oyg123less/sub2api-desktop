import { Hono } from "hono";
import { requireAuth } from "./auth-middleware";
import { AppError, readJSON, requireJSONSize } from "./errors";
import type { AppEnv, VaultRow } from "./types";

const vault = new Hono<AppEnv>();
const kinds = new Set(["account", "proxy", "codex_remote", "settings"]);
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function publicItem(row: VaultRow) {
  return {
    kind: row.kind,
    client_uid: row.client_uid,
    ciphertext: row.ciphertext,
    version: row.version,
    deleted: Boolean(row.deleted),
    updated_at: row.updated_at,
  };
}

interface IncomingItem {
  kind: VaultRow["kind"];
  client_uid: string;
  ciphertext: string;
  version: number;
  deleted: boolean;
}

function validateItem(value: unknown): IncomingItem {
  if (!value || typeof value !== "object") throw new AppError(400, "invalid_vault_item", "A vault item is invalid.");
  const item = value as Record<string, unknown>;
  if (typeof item.kind !== "string" || !kinds.has(item.kind) || typeof item.client_uid !== "string" ||
      !uuidPattern.test(item.client_uid) || typeof item.ciphertext !== "string" || !item.ciphertext.startsWith("v1.") ||
      item.ciphertext.length > 512 * 1024 || !Number.isInteger(item.version) || Number(item.version) < 0 ||
      typeof item.deleted !== "boolean") {
    throw new AppError(400, "invalid_vault_item", "A vault item is invalid.");
  }
  return item as unknown as IncomingItem;
}

vault.use("/*", requireAuth);

vault.get("/", async (c) => {
  const user = c.get("auth");
  const since = c.req.query("since") || "";
  if (since && (!/^\d{4}-\d{2}-\d{2}T/.test(since) || Number.isNaN(Date.parse(since)))) {
    throw new AppError(400, "invalid_sync_cursor", "The sync cursor is invalid.");
  }
  const result = since
    ? await c.env.DB.prepare(`SELECT id, kind, client_uid, ciphertext, version, deleted, updated_at
        FROM vault_items WHERE user_id=? AND updated_at>? ORDER BY updated_at,id LIMIT 1000`).bind(user.id, since).all<VaultRow>()
    : await c.env.DB.prepare(`SELECT id, kind, client_uid, ciphertext, version, deleted, updated_at
        FROM vault_items WHERE user_id=? ORDER BY updated_at,id LIMIT 1000`).bind(user.id).all<VaultRow>();
  const items = result.results.map(publicItem);
  return c.json({ items, cursor: items.at(-1)?.updated_at || since || new Date(0).toISOString() });
});

vault.put("/batch", async (c) => {
  requireJSONSize(c, 2 * 1024 * 1024);
  const body = await readJSON<{ items?: unknown[] }>(c);
  if (!Array.isArray(body.items) || body.items.length === 0 || body.items.length > 200) {
    throw new AppError(400, "invalid_vault_batch", "Provide between 1 and 200 vault items.");
  }
  const user = c.get("auth");
  const items = body.items.map(validateItem);
  const seen = new Set<string>();
  for (const item of items) {
    const key = `${item.kind}:${item.client_uid}`;
    if (seen.has(key)) throw new AppError(400, "duplicate_vault_item", "The batch contains duplicate vault items.");
    seen.add(key);
  }

  const existing = new Map<string, VaultRow>();
  for (const item of items) {
    const row = await c.env.DB.prepare(`SELECT id, kind, client_uid, ciphertext, version, deleted, updated_at
      FROM vault_items WHERE user_id=? AND kind=? AND client_uid=?`).bind(user.id, item.kind, item.client_uid).first<VaultRow>();
    if (row) existing.set(`${item.kind}:${item.client_uid}`, row);
  }
  const conflicts = items.flatMap((item) => {
    const row = existing.get(`${item.kind}:${item.client_uid}`);
    return row && row.version !== item.version ? [publicItem(row)] : [];
  });
  if (conflicts.length) return c.json({ error: { code: "vault_conflict", message: "One or more items changed on another device." }, conflicts }, 409);

  const now = new Date().toISOString();
  const statements = items.map((item) => {
    const row = existing.get(`${item.kind}:${item.client_uid}`);
    if (row) {
      return c.env.DB.prepare(`UPDATE vault_items SET ciphertext=?, deleted=?, version=version+1, updated_at=?
        WHERE user_id=? AND kind=? AND client_uid=? AND version=?`).bind(
          item.ciphertext, item.deleted ? 1 : 0, now, user.id, item.kind, item.client_uid, item.version,
        );
    }
    if (item.version !== 0) throw new AppError(409, "vault_conflict", "A new vault item must start at version zero.");
    return c.env.DB.prepare(`INSERT INTO vault_items(user_id,kind,client_uid,ciphertext,version,deleted,updated_at)
      VALUES(?,?,?,?,1,?,?)`).bind(user.id, item.kind, item.client_uid, item.ciphertext, item.deleted ? 1 : 0, now);
  });
  await c.env.DB.batch(statements);

  const deletedAccounts = items.filter((item) => item.kind === "account" && item.deleted);
  if (deletedAccounts.length) {
    await c.env.DB.batch(deletedAccounts.map((item) => c.env.DB.prepare(`UPDATE share_grants
      SET revoked=1,updated_at=? WHERE owner_id=? AND account_uid=? AND revoked=0`).bind(now, user.id, item.client_uid)));
  }

  const updated: VaultRow[] = [];
  for (const item of items) {
    const row = await c.env.DB.prepare(`SELECT id, kind, client_uid, ciphertext, version, deleted, updated_at
      FROM vault_items WHERE user_id=? AND kind=? AND client_uid=?`).bind(user.id, item.kind, item.client_uid).first<VaultRow>();
    if (row) updated.push(row);
  }
  return c.json({ items: updated.map(publicItem), cursor: now });
});

export default vault;
