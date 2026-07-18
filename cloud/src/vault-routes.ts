import { Hono } from "hono";
import { requireAuth } from "./auth-middleware";
import { AppError, readJSON } from "./errors";
import { sha256 } from "./security";
import type { AppEnv, VaultRow } from "./types";

const vault = new Hono<AppEnv>();
const kinds = new Set(["account", "proxy", "codex_remote", "settings"]);
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const idempotencyKeyPattern = /^[A-Za-z0-9][A-Za-z0-9._:-]{15,127}$/;
const receiptTTL = 7 * 24 * 60 * 60 * 1000;

interface BatchReceipt {
  request_hash: string;
  response_status: number;
  response_body: string;
}

function receiptResponse(receipt: BatchReceipt, replayed: boolean): Response {
  return new Response(receipt.response_body, {
    status: receipt.response_status,
    headers: {
      "Content-Type": "application/json; charset=UTF-8",
      "Cache-Control": "no-store",
      ...(replayed ? { "Idempotency-Replayed": "true" } : {}),
    },
  });
}

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

function cursorFor(row: VaultRow): string {
  return `${row.updated_at}|${row.id}`;
}

function parseCursor(value: string): { updatedAt: string; id: number; composite: boolean } | null {
  if (!value) return null;
  const separator = value.lastIndexOf("|");
  const composite = separator >= 0;
  const updatedAt = composite ? value.slice(0, separator) : value;
  const idText = composite ? value.slice(separator + 1) : "0";
  const id = Number(idText);
  if (!/^\d{4}-\d{2}-\d{2}T/.test(updatedAt) || Number.isNaN(Date.parse(updatedAt)) ||
      (composite && (!/^\d+$/.test(idText) || !Number.isSafeInteger(id) || id < 0))) {
    throw new AppError(400, "invalid_sync_cursor", "The sync cursor is invalid.");
  }
  return { updatedAt, id, composite };
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
  const cursor = parseCursor(since);
  const result = cursor?.composite
    ? await c.env.DB.prepare(`SELECT id, kind, client_uid, ciphertext, version, deleted, updated_at
        FROM vault_items WHERE user_id=? AND (updated_at>? OR (updated_at=? AND id>?))
        ORDER BY updated_at,id LIMIT 1000`).bind(user.id, cursor.updatedAt, cursor.updatedAt, cursor.id).all<VaultRow>()
    : cursor
      ? await c.env.DB.prepare(`SELECT id, kind, client_uid, ciphertext, version, deleted, updated_at
          FROM vault_items WHERE user_id=? AND updated_at>? ORDER BY updated_at,id LIMIT 1000`).bind(user.id, cursor.updatedAt).all<VaultRow>()
      : await c.env.DB.prepare(`SELECT id, kind, client_uid, ciphertext, version, deleted, updated_at
          FROM vault_items WHERE user_id=? ORDER BY updated_at,id LIMIT 1000`).bind(user.id).all<VaultRow>();
  const items = result.results.map(publicItem);
  const last = result.results.at(-1);
  return c.json({ items, cursor: last ? cursorFor(last) : since || `${new Date(0).toISOString()}|0` });
});

vault.put("/batch", async (c) => {
  const body = await readJSON<{ items?: unknown[] }>(c, 2 * 1024 * 1024);
  if (!Array.isArray(body.items) || body.items.length === 0 || body.items.length > 200) {
    throw new AppError(400, "invalid_vault_batch", "Provide between 1 and 200 vault items.");
  }
  const user = c.get("auth");
  const items = body.items.map(validateItem);
  const idempotencyKey = (c.req.header("idempotency-key") || "").trim();
  if (idempotencyKey && !idempotencyKeyPattern.test(idempotencyKey)) {
    throw new AppError(400, "invalid_idempotency_key", "The idempotency key is invalid.");
  }
  const seen = new Set<string>();
  for (const item of items) {
    const key = `${item.kind}:${item.client_uid}`;
    if (seen.has(key)) throw new AppError(400, "duplicate_vault_item", "The batch contains duplicate vault items.");
    seen.add(key);
  }

  const requestHash = await sha256(JSON.stringify(items.map((item) => ({
    kind: item.kind,
    client_uid: item.client_uid,
    ciphertext: item.ciphertext,
    version: item.version,
    deleted: item.deleted,
  }))));
  const now = new Date().toISOString();
  c.executionCtx.waitUntil(c.env.DB.prepare("DELETE FROM vault_batch_receipts WHERE expires_at<=?").bind(now).run());
  c.executionCtx.waitUntil(c.env.DB.prepare("DELETE FROM vault_write_claims WHERE expires_at<=?").bind(now).run());

  const findReceipt = () => c.env.DB.prepare(`SELECT request_hash,response_status,response_body
    FROM vault_batch_receipts WHERE user_id=? AND idempotency_key=?`)
    .bind(user.id, idempotencyKey).first<BatchReceipt>();
  if (idempotencyKey) {
    await c.env.DB.prepare(`DELETE FROM vault_batch_receipts
      WHERE user_id=? AND idempotency_key=? AND expires_at<=?`).bind(user.id, idempotencyKey, now).run();
    const receipt = await findReceipt();
    if (receipt) {
      if (receipt.request_hash !== requestHash) {
        throw new AppError(409, "idempotency_key_reused", "The idempotency key was already used for a different batch.");
      }
      return receiptResponse(receipt, true);
    }
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
  if (conflicts.length) {
    const responseBody = JSON.stringify({
      error: { code: "vault_conflict", message: "One or more items changed on another device." },
      conflicts,
    });
    if (idempotencyKey) {
      const expiresAt = new Date(Date.now() + receiptTTL).toISOString();
      try {
        await c.env.DB.prepare(`INSERT INTO vault_batch_receipts
          (user_id,idempotency_key,request_hash,response_status,response_body,created_at,expires_at)
          VALUES(?,?,?,?,?,?,?)`).bind(user.id, idempotencyKey, requestHash, 409, responseBody, now, expiresAt).run();
      } catch {
        const receipt = await findReceipt();
        if (receipt?.request_hash === requestHash) return receiptResponse(receipt, true);
        if (receipt) throw new AppError(409, "idempotency_key_reused", "The idempotency key was already used for a different batch.");
        throw new AppError(503, "idempotency_receipt_failed", "The batch receipt could not be saved.");
      }
    }
    return new Response(responseBody, { status: 409, headers: { "Content-Type": "application/json; charset=UTF-8" } });
  }

  const updated = items.map((item) => ({
    kind: item.kind,
    client_uid: item.client_uid,
    ciphertext: item.ciphertext,
    version: item.version + 1,
    deleted: item.deleted,
    updated_at: now,
  }));
  const responseBody = JSON.stringify({ items: updated, cursor: `${now}|0` });
  const claimExpiry = new Date(Date.now() + receiptTTL).toISOString();
  const requestKey = idempotencyKey || c.get("requestId");
  const statements = items.map((item) => c.env.DB.prepare(`INSERT INTO vault_write_claims
    (user_id,kind,client_uid,base_version,request_key,expires_at) VALUES(?,?,?,?,?,?)`).bind(
      user.id, item.kind, item.client_uid, item.version, requestKey, claimExpiry,
    ));
  statements.push(...items.map((item) => {
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
  }));
  const deletedAccounts = items.filter((item) => item.kind === "account" && item.deleted);
  statements.push(...deletedAccounts.map((item) => c.env.DB.prepare(`UPDATE share_grants
    SET revoked=1,updated_at=? WHERE owner_id=? AND account_uid=? AND revoked=0`).bind(now, user.id, item.client_uid)));
  if (idempotencyKey) {
    statements.unshift(c.env.DB.prepare(`INSERT INTO vault_batch_receipts
      (user_id,idempotency_key,request_hash,response_status,response_body,created_at,expires_at)
      VALUES(?,?,?,?,?,?,?)`).bind(
        user.id, idempotencyKey, requestHash, 200, responseBody, now, new Date(Date.now() + receiptTTL).toISOString(),
      ));
  }
  try {
    await c.env.DB.batch(statements);
  } catch (error) {
    if (idempotencyKey) {
      const receipt = await findReceipt();
      if (receipt?.request_hash === requestHash) return receiptResponse(receipt, true);
      if (receipt) throw new AppError(409, "idempotency_key_reused", "The idempotency key was already used for a different batch.");
    }
    const racedConflicts: ReturnType<typeof publicItem>[] = [];
    for (const item of items) {
      const row = await c.env.DB.prepare(`SELECT id,kind,client_uid,ciphertext,version,deleted,updated_at
        FROM vault_items WHERE user_id=? AND kind=? AND client_uid=?`).bind(user.id, item.kind, item.client_uid).first<VaultRow>();
      if (row && row.version !== item.version) racedConflicts.push(publicItem(row));
    }
    if (racedConflicts.length) {
      const conflictBody = JSON.stringify({
        error: { code: "vault_conflict", message: "One or more items changed on another device." },
        conflicts: racedConflicts,
      });
      if (idempotencyKey) {
        await c.env.DB.prepare(`INSERT OR IGNORE INTO vault_batch_receipts
          (user_id,idempotency_key,request_hash,response_status,response_body,created_at,expires_at)
          VALUES(?,?,?,?,?,?,?)`).bind(
            user.id, idempotencyKey, requestHash, 409, conflictBody, now, new Date(Date.now() + receiptTTL).toISOString(),
          ).run();
      }
      return new Response(conflictBody, { status: 409, headers: { "Content-Type": "application/json; charset=UTF-8" } });
    }
    throw error;
  }
  return new Response(responseBody, { status: 200, headers: { "Content-Type": "application/json; charset=UTF-8" } });
});

export default vault;
