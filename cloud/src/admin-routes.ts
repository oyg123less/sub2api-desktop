import { Hono } from "hono";
import { requireAdmin, requireAuth } from "./auth-middleware";
import { AppError, readJSON } from "./errors";
import { revokeUserSessions } from "./security";
import type { AppEnv } from "./types";

const admin = new Hono<AppEnv>();

async function audit(
  env: AppEnv["Bindings"],
  actorID: number,
  action: string,
  targetType: string,
  targetID: string,
  details: Record<string, unknown> = {},
) {
  await env.DB.prepare(`INSERT INTO admin_audit(actor_user_id,action,target_type,target_id,details,created_at)
    VALUES(?,?,?,?,?,?)`).bind(
      actorID, action, targetType, targetID, JSON.stringify(details), new Date().toISOString(),
    ).run();
}

function positiveID(value: string): number {
  const id = Number(value);
  if (!Number.isInteger(id) || id <= 0) throw new AppError(400, "invalid_user_id", "The user ID is invalid.");
  return id;
}

admin.use("/*", requireAuth, requireAdmin);

admin.get("/users", async (c) => {
  const search = (c.req.query("search") || "").trim().toLowerCase().slice(0, 120);
  const limit = Math.min(Math.max(Number(c.req.query("limit") || 50), 1), 100);
  const offset = Math.max(Number(c.req.query("offset") || 0), 0);
  const query = `SELECT u.id,u.email,u.role,u.email_verified,u.banned,u.created_at,u.updated_at,u.last_active_at,
    (SELECT COUNT(*) FROM vault_items v WHERE v.user_id=u.id AND v.deleted=0) AS vault_count
    FROM users u ${search ? "WHERE u.email LIKE ?" : ""} ORDER BY u.created_at DESC,u.id DESC LIMIT ? OFFSET ?`;
  const statement = search
    ? c.env.DB.prepare(query).bind(`%${search}%`, limit, offset)
    : c.env.DB.prepare(query).bind(limit, offset);
  const result = await statement.all();
  return c.json({ users: result.results, limit, offset });
});

admin.patch("/users/:id", async (c) => {
  const targetID = positiveID(c.req.param("id"));
  const actor = c.get("auth");
  if (targetID === actor.id) throw new AppError(400, "cannot_modify_self", "Use a different administrator to modify this account.");
  const body = await readJSON<{ banned?: unknown }>(c);
  if (typeof body.banned !== "boolean") throw new AppError(400, "invalid_admin_action", "The banned field must be a boolean.");
  const result = await c.env.DB.prepare(`UPDATE users SET banned=?, session_version=session_version+1, updated_at=? WHERE id=?`)
    .bind(body.banned ? 1 : 0, new Date().toISOString(), targetID).run();
  if (!result.meta.changes) throw new AppError(404, "user_not_found", "The user was not found.");
  if (body.banned) await revokeUserSessions(c.env, targetID);
  await audit(c.env, actor.id, body.banned ? "user.ban" : "user.unban", "user", String(targetID));
  return c.json({ ok: true });
});

admin.post("/users/:id/logout-all", async (c) => {
  const targetID = positiveID(c.req.param("id"));
  const result = await c.env.DB.prepare("UPDATE users SET session_version=session_version+1, updated_at=? WHERE id=?")
    .bind(new Date().toISOString(), targetID).run();
  if (!result.meta.changes) throw new AppError(404, "user_not_found", "The user was not found.");
  await revokeUserSessions(c.env, targetID);
  await audit(c.env, c.get("auth").id, "user.logout_all", "user", String(targetID));
  return c.json({ ok: true });
});

admin.delete("/users/:id", async (c) => {
  const targetID = positiveID(c.req.param("id"));
  const actor = c.get("auth");
  if (targetID === actor.id) throw new AppError(400, "cannot_delete_self", "An administrator cannot delete their own account.");
  const body = await readJSON<{ confirm?: unknown }>(c);
  if (body.confirm !== "DELETE") throw new AppError(400, "delete_confirmation_required", "Type DELETE to confirm user deletion.");
  const target = await c.env.DB.prepare("SELECT id FROM users WHERE id=?").bind(targetID).first();
  if (!target) throw new AppError(404, "user_not_found", "The user was not found.");
  await revokeUserSessions(c.env, targetID);
  await audit(c.env, actor.id, "user.delete", "user", String(targetID));
  await c.env.DB.batch([
    c.env.DB.prepare("DELETE FROM share_usage_log WHERE grant_id IN (SELECT id FROM share_grants WHERE owner_id=?)").bind(targetID),
    c.env.DB.prepare("DELETE FROM share_grants WHERE owner_id=?").bind(targetID),
    c.env.DB.prepare("DELETE FROM vault_items WHERE user_id=?").bind(targetID),
    c.env.DB.prepare("DELETE FROM users WHERE id=?").bind(targetID),
  ]);
  return c.json({ ok: true });
});

admin.get("/settings", async (c) => {
  const result = await c.env.DB.prepare("SELECT key,value,updated_at FROM platform_settings ORDER BY key").all();
  return c.json({ settings: result.results });
});

admin.patch("/settings", async (c) => {
  const body = await readJSON<Record<string, unknown>>(c);
  const allowed = ["registration_enabled", "invite_mode"] as const;
  const entries = allowed.flatMap((key) => typeof body[key] === "boolean" ? [[key, body[key] as boolean] as const] : []);
  if (!entries.length) throw new AppError(400, "invalid_platform_settings", "No supported settings were provided.");
  const now = new Date().toISOString();
  await c.env.DB.batch(entries.map(([key, value]) => c.env.DB.prepare(`INSERT INTO platform_settings(key,value,updated_at)
    VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`)
    .bind(key, String(value), now)));
  await audit(c.env, c.get("auth").id, "platform.settings.update", "platform", "global", {
    keys: entries.map(([key]) => key),
  });
  return c.json({ ok: true });
});

admin.get("/stats", async (c) => {
  const [users, active, vault, shares, requests, failed] = await Promise.all([
    c.env.DB.prepare("SELECT COUNT(*) AS count FROM users").first<{ count: number }>(),
    c.env.DB.prepare("SELECT COUNT(*) AS count FROM users WHERE last_active_at>=datetime('now','-1 day')").first<{ count: number }>(),
    c.env.DB.prepare("SELECT COUNT(*) AS count FROM vault_items WHERE deleted=0").first<{ count: number }>(),
    c.env.DB.prepare("SELECT COUNT(*) AS count FROM share_grants WHERE revoked=0").first<{ count: number }>(),
    c.env.DB.prepare("SELECT COUNT(*) AS count FROM share_usage_log").first<{ count: number }>(),
    c.env.DB.prepare("SELECT COUNT(*) AS count FROM share_usage_log WHERE status>=400").first<{ count: number }>(),
  ]);
  const requestCount = requests?.count ?? 0;
  return c.json({
    users: users?.count ?? 0,
    daily_active_users: active?.count ?? 0,
    vault_items: vault?.count ?? 0,
    active_shares: shares?.count ?? 0,
    share_requests: requestCount,
    share_error_rate: requestCount ? (failed?.count ?? 0) / requestCount : 0,
  });
});

admin.get("/shares", async (c) => {
  const limit = Math.min(Math.max(Number(c.req.query("limit") || 100), 1), 100);
  const result = await c.env.DB.prepare(`SELECT s.id,s.owner_id,u.email AS owner_email,s.account_uid,s.share_code,
    s.quota_requests,s.used_requests,s.expires_at,s.revoked,s.created_at,s.updated_at
    FROM share_grants s JOIN users u ON u.id=s.owner_id ORDER BY s.created_at DESC,s.id DESC LIMIT ?`).bind(limit).all();
  return c.json({ shares: result.results });
});

admin.patch("/shares/:id", async (c) => {
  const id = positiveID(c.req.param("id"));
  const body = await readJSON<{ revoked?: unknown }>(c);
  if (typeof body.revoked !== "boolean") throw new AppError(400, "invalid_admin_action", "The revoked field must be a boolean.");
  const accountGuard = body.revoked ? "" : ` AND EXISTS (SELECT 1 FROM vault_items v
    WHERE v.user_id=share_grants.owner_id AND v.kind='account' AND v.client_uid=share_grants.account_uid AND v.deleted=0)`;
  const result = await c.env.DB.prepare(`UPDATE share_grants SET revoked=?,updated_at=? WHERE id=?${accountGuard}`)
    .bind(body.revoked ? 1 : 0, new Date().toISOString(), id).run();
  if (!result.meta.changes) {
    const existing = await c.env.DB.prepare("SELECT id FROM share_grants WHERE id=?").bind(id).first();
    if (!existing) throw new AppError(404, "share_not_found", "The share was not found.");
    if (!body.revoked) throw new AppError(409, "share_account_deleted", "The deleted account's share cannot be restored.");
    throw new AppError(404, "share_not_found", "The share was not found.");
  }
  await audit(c.env, c.get("auth").id, body.revoked ? "share.revoke" : "share.restore", "share", String(id));
  return c.json({ ok: true });
});

admin.get("/audit", async (c) => {
  const limit = Math.min(Math.max(Number(c.req.query("limit") || 50), 1), 100);
  const result = await c.env.DB.prepare(`SELECT id,actor_user_id,action,target_type,target_id,details,created_at
    FROM admin_audit ORDER BY created_at DESC,id DESC LIMIT ?`).bind(limit).all();
  return c.json({ audit: result.results });
});

export default admin;
