import { Hono, type Context } from "hono";
import { requireAuth } from "./auth-middleware";
import {
  boundedInteger, mutationReceipt, newPublicID, parsePublicID, publicOrigin, receiptStatement,
  requireFeature, validateKeyMaterial, writeShareAudit,
} from "./cloud-v2";
import { AppError, readJSON, requireJSONSize } from "./errors";
import { hmacBase64URL, randomToken, safeEqual } from "./security";
import { parseShareAccount, type AccountInput } from "./share-group-routes";
import type { AppEnv } from "./types";
import { shareRecipientEventStatement, userEventStatement } from "./user-events";

const routes = new Hono<AppEnv>();
const passwordPattern = /^[A-HJ-NP-Z2-9]{6}$/;

interface EndpointRow {
  id: number;
  public_id: string;
  owner_id: number;
  group_id: number;
  group_public_id: string;
  connection_code: string;
  status: "active" | "paused" | "deleted";
  group_status: "active" | "paused" | "deleted";
  created_at: string;
  updated_at: string;
}

interface WindowRow {
  id: number;
  public_id: string;
  endpoint_id: number;
  password_version: number;
  password_salt: string;
  password_verifier: string;
  status: "active" | "exhausted" | "expired" | "replaced" | "stopped";
  max_claims: number;
  claimed_count: number;
  expires_at: string;
  created_at: string;
  updated_at: string;
}

routes.use("/connect", requireAuth);
routes.use("/connect/*", requireAuth);

function normalizeCode(value: unknown): string {
  const code = typeof value === "string" ? value.replace(/[\s-]/g, "") : "";
  if (!/^\d{9}$/.test(code)) throw new AppError(400, "invalid_connection_code", "Enter a valid 9-digit connection code.");
  return code;
}

function normalizePassword(value: unknown): string {
  const password = typeof value === "string" ? value.trim().toUpperCase() : "";
  if (!passwordPattern.test(password)) throw new AppError(400, "invalid_connect_password", "Enter a valid 6-character temporary password.");
  return password;
}

function randomConnectionCode(): string {
  const values = new Uint32Array(1);
  crypto.getRandomValues(values);
  return String(100_000_000 + ((values[0] ?? 0) % 900_000_000));
}

async function unusedConnectionCode(c: Context<AppEnv>): Promise<string> {
  for (let attempt = 0; attempt < 12; attempt += 1) {
    const code = randomConnectionCode();
    const exists = await c.env.DB.prepare("SELECT id FROM share_connect_endpoints WHERE connection_code=?").bind(code).first();
    if (!exists) return code;
  }
  throw new AppError(503, "connection_code_unavailable", "A connection code could not be allocated. Try again.");
}

async function ownedEndpoint(c: Context<AppEnv>, required = true): Promise<EndpointRow | null> {
  const row = await c.env.DB.prepare(`SELECT e.id,e.public_id,e.owner_id,e.group_id,e.connection_code,e.status,e.created_at,e.updated_at,
    g.public_id AS group_public_id,g.status AS group_status FROM share_connect_endpoints e
    JOIN share_groups g ON g.id=e.group_id WHERE e.owner_id=? AND e.status<>'deleted'`)
    .bind(c.get("auth").id).first<EndpointRow>();
  if (!row && required) throw new AppError(409, "connect_host_not_configured", "Select the accounts to share before starting sharing.");
  return row ?? null;
}

async function activeWindow(c: Context<AppEnv>, endpointID: number): Promise<WindowRow | null> {
  const now = new Date().toISOString();
  await c.env.DB.prepare("UPDATE share_connect_windows SET status='expired',updated_at=? WHERE endpoint_id=? AND status='active' AND expires_at<=?")
    .bind(now, endpointID, now).run();
  return c.env.DB.prepare(`SELECT id,public_id,endpoint_id,password_version,password_salt,password_verifier,status,
    max_claims,claimed_count,expires_at,created_at,updated_at FROM share_connect_windows
    WHERE endpoint_id=? AND status='active' ORDER BY password_version DESC LIMIT 1`).bind(endpointID).first<WindowRow>();
}

async function hostPayload(c: Context<AppEnv>, endpoint: EndpointRow | null) {
  if (!endpoint) return { configured: false, accounts: [], recipients: [] };
  const [window, accounts, recipients] = await Promise.all([
    activeWindow(c, endpoint.id),
    c.env.DB.prepare(`SELECT public_id,account_uid,account_type,relay_mode,priority,weight,enabled,created_at,updated_at
      FROM share_group_accounts WHERE group_id=? ORDER BY priority,id`).bind(endpoint.group_id).all(),
    c.env.DB.prepare(`SELECT r.public_id,r.status,r.rpm_limit,r.concurrency_limit,r.quota_requests,r.used_requests,
      r.expires_at,r.created_at,r.accepted_at,r.updated_at,p.display_name,p.friend_code,k.key_prefix
      FROM share_group_recipients r JOIN friend_profiles p ON p.user_id=r.recipient_id
      LEFT JOIN share_access_keys k ON k.recipient_grant_id=r.id AND k.status='active'
      WHERE r.group_id=? AND r.status IN ('active','paused') ORDER BY r.updated_at DESC,r.id DESC`).bind(endpoint.group_id).all(),
  ]);
  return {
    configured: true,
    endpoint: {
      public_id: endpoint.public_id, connection_code: endpoint.connection_code, status: endpoint.status,
      group_status: endpoint.group_status, base_url: `${publicOrigin(c)}/v1`, created_at: endpoint.created_at, updated_at: endpoint.updated_at,
    },
    window: window ? {
      public_id: window.public_id, password_version: window.password_version, max_claims: window.max_claims,
      claimed_count: window.claimed_count, expires_at: window.expires_at, created_at: window.created_at,
    } : null,
    accounts: accounts.results,
    recipients: recipients.results,
  };
}

async function requireConnectFeature(c: Context<AppEnv>) {
  await requireFeature(c, "share_groups_enabled");
  await requireFeature(c, "connect_codes_enabled");
}

routes.get("/connect/host", async (c) => {
  await requireConnectFeature(c);
  return c.json(await hostPayload(c, await ownedEndpoint(c, false)));
});

routes.put("/connect/host/accounts", async (c) => {
  await requireConnectFeature(c);
  requireJSONSize(c, 256 * 1024);
  const body = await readJSON<Record<string, unknown>>(c, 256 * 1024);
  const rawAccounts = Array.isArray(body.accounts) ? body.accounts : [];
  if (rawAccounts.length < 1 || rawAccounts.length > 20) throw new AppError(400, "invalid_share_accounts", "Select between 1 and 20 accounts.");
  const accounts: AccountInput[] = [];
  const seen = new Set<string>();
  for (const raw of rawAccounts) {
    const account = await parseShareAccount(c, raw);
    if (seen.has(account.account_uid)) throw new AppError(400, "duplicate_share_account", "Each account can be selected only once.");
    seen.add(account.account_uid);
    accounts.push(account);
  }
  const existing = await ownedEndpoint(c, false);
  const now = new Date().toISOString();
  let groupID = existing?.group_id ?? 0;
  if (!existing) {
    const groupPublicID = newPublicID("grp");
    const endpointPublicID = newPublicID("conn");
    const code = await unusedConnectionCode(c);
    const result = await c.env.DB.prepare(`INSERT INTO share_groups
      (public_id,owner_id,name,description,status,route_policy,default_rpm,default_concurrency,default_quota_requests,created_at,updated_at)
      VALUES(?,?,'快速共享','Amber connection-code sharing','paused','balanced',30,2,0,?,?) RETURNING id`)
      .bind(groupPublicID, c.get("auth").id, now, now).first<{ id: number }>();
    if (!result) throw new AppError(500, "connect_host_create_failed", "The sharing entry could not be created.");
    groupID = result.id;
    await c.env.DB.prepare(`INSERT INTO share_connect_endpoints
      (public_id,owner_id,group_id,connection_code,status,created_at,updated_at) VALUES(?,?,?,?, 'paused',?,?)`)
      .bind(endpointPublicID, c.get("auth").id, groupID, code, now, now).run();
  }
  const statements: D1PreparedStatement[] = [
    c.env.DB.prepare("DELETE FROM share_group_accounts WHERE group_id=?").bind(groupID),
  ];
  for (const account of accounts) {
    statements.push(c.env.DB.prepare(`INSERT INTO share_group_accounts
      (public_id,group_id,account_uid,account_type,relay_mode,priority,weight,enabled,token_cipher,created_at,updated_at)
      VALUES(?,?,?,?,?,?,?,1,?,?,?)`).bind(account.public_id, groupID, account.account_uid, account.account_type,
        account.relay_mode, account.priority, account.weight, account.token_cipher, now, now));
  }
  await c.env.DB.batch(statements);
  const endpoint = await ownedEndpoint(c);
  await userEventStatement(c, c.get("auth").id, "connect.accounts_updated", "connect_endpoint", endpoint!.public_id).run();
  await writeShareAudit(c, groupID, "share_connect.accounts", "share_connect_endpoint", endpoint!.public_id, { count: accounts.length });
  return c.json(await hostPayload(c, endpoint));
});

async function openWindow(c: Context<AppEnv>, operation: string) {
  await requireConnectFeature(c);
  const endpoint = await ownedEndpoint(c);
  const body = await readJSON<Record<string, unknown>>(c);
  const receipt = await mutationReceipt(c, operation, body);
  if (receipt.replay) return receipt.replay;
  const password = normalizePassword(body.password);
  const maxClaims = boundedInteger(body.max_claims, 1, 1, 20, "invalid_connect_claim_limit", "The allowed connection count must be between 1 and 20.");
  const minutes = boundedInteger(body.duration_minutes, 30, 5, 1440, "invalid_connect_duration", "The temporary password duration must be between 5 minutes and 24 hours.");
  const account = await c.env.DB.prepare("SELECT id FROM share_group_accounts WHERE group_id=? AND enabled=1 LIMIT 1").bind(endpoint!.group_id).first();
  if (!account) throw new AppError(409, "connect_accounts_required", "Select at least one available account before starting sharing.");
  const current = await activeWindow(c, endpoint!.id);
  const latest = await c.env.DB.prepare("SELECT COALESCE(MAX(password_version),0) AS version FROM share_connect_windows WHERE endpoint_id=?")
    .bind(endpoint!.id).first<{ version: number }>();
  const version = (latest?.version ?? 0) + 1;
  const salt = randomToken(16);
  const pepper = c.env.SHARE_CONNECT_PEPPER || "";
  if (pepper.length < 32) throw new AppError(503, "connect_not_configured", "Connection-code sharing is not configured.");
  const verifier = await hmacBase64URL(pepper, `${endpoint!.public_id}:${version}:${salt}:${password}`);
  const now = new Date();
  const expiresAt = new Date(now.getTime() + minutes * 60_000).toISOString();
  const windowPublicID = newPublicID("win");
  const response = { ok: true, password_version: version, max_claims: maxClaims, claimed_count: 0, expires_at: expiresAt };
  const statements: D1PreparedStatement[] = [];
  if (current) statements.push(c.env.DB.prepare("UPDATE share_connect_windows SET status='replaced',updated_at=? WHERE id=? AND status='active'").bind(now.toISOString(), current.id));
  statements.push(
    c.env.DB.prepare(`INSERT INTO share_connect_windows
      (public_id,endpoint_id,password_version,password_salt,password_verifier,status,max_claims,claimed_count,expires_at,created_at,updated_at)
      VALUES(?,?,?,?,?,'active',?,0,?,?,?)`).bind(windowPublicID, endpoint!.id, version, salt, verifier, maxClaims, expiresAt, now.toISOString(), now.toISOString()),
    c.env.DB.prepare("UPDATE share_connect_endpoints SET status='active',updated_at=? WHERE id=?").bind(now.toISOString(), endpoint!.id),
    c.env.DB.prepare("UPDATE share_groups SET status='active',updated_at=? WHERE id=?").bind(now.toISOString(), endpoint!.group_id),
    receiptStatement(c, operation, receipt.key, receipt.requestHash, 200, JSON.stringify(response)),
    userEventStatement(c, c.get("auth").id, `connect.${operation.endsWith("rotate_password") ? "password_rotated" : "started"}`, "connect_endpoint", endpoint!.public_id),
  );
  await c.env.DB.batch(statements);
  await writeShareAudit(c, endpoint!.group_id, operation, "share_connect_endpoint", endpoint!.public_id, { max_claims: maxClaims, expires_at: expiresAt });
  return c.json({ ...response, host: await hostPayload(c, await ownedEndpoint(c)) });
}

routes.post("/connect/host/start", (c) => openWindow(c, "share_connect.start"));
routes.post("/connect/host/rotate-password", (c) => openWindow(c, "share_connect.rotate_password"));

async function setHostState(c: Context<AppEnv>, status: "active" | "paused") {
  await requireConnectFeature(c);
  const endpoint = await ownedEndpoint(c);
  const now = new Date().toISOString();
  if (status === "active" && !(await activeWindow(c, endpoint!.id))) {
    throw new AppError(409, "connect_password_required", "Create a new temporary password before resuming sharing.");
  }
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_connect_endpoints SET status=?,updated_at=? WHERE id=?").bind(status, now, endpoint!.id),
    c.env.DB.prepare("UPDATE share_groups SET status=?,updated_at=? WHERE id=?").bind(status, now, endpoint!.group_id),
    userEventStatement(c, c.get("auth").id, `connect.${status}`, "connect_endpoint", endpoint!.public_id),
    shareRecipientEventStatement(c, endpoint!.group_id, `connect.host_${status}`, "received_share", endpoint!.public_id),
  ]);
  await writeShareAudit(c, endpoint!.group_id, `share_connect.${status === "active" ? "resume" : "pause"}`, "share_connect_endpoint", endpoint!.public_id);
  return c.json(await hostPayload(c, await ownedEndpoint(c)));
}

routes.post("/connect/host/pause", (c) => setHostState(c, "paused"));
routes.post("/connect/host/resume", (c) => setHostState(c, "active"));

routes.post("/connect/host/reset-code", async (c) => {
  await requireConnectFeature(c);
  const endpoint = await ownedEndpoint(c);
  const code = await unusedConnectionCode(c);
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_connect_endpoints SET connection_code=?,updated_at=? WHERE id=?").bind(code, now, endpoint!.id),
    c.env.DB.prepare("UPDATE share_connect_windows SET status='replaced',updated_at=? WHERE endpoint_id=? AND status='active'").bind(now, endpoint!.id),
    userEventStatement(c, c.get("auth").id, "connect.code_reset", "connect_endpoint", endpoint!.public_id),
  ]);
  await writeShareAudit(c, endpoint!.group_id, "share_connect.reset_code", "share_connect_endpoint", endpoint!.public_id);
  return c.json(await hostPayload(c, await ownedEndpoint(c)));
});

routes.get("/connect/host/recipients", async (c) => {
  await requireConnectFeature(c);
  return c.json(await hostPayload(c, await ownedEndpoint(c)));
});

async function ownedConnectedRecipient(c: Context<AppEnv>, publicID: string) {
  const endpoint = await ownedEndpoint(c);
  const recipient = await c.env.DB.prepare(`SELECT id,public_id,recipient_id,status FROM share_group_recipients
    WHERE public_id=? AND group_id=?`).bind(parsePublicID(publicID, "invalid_share_recipient_id"), endpoint!.group_id)
    .first<{ id: number; public_id: string; recipient_id: number; status: string }>();
  if (!recipient) throw new AppError(404, "share_recipient_not_found", "The connected user was not found.");
  return { endpoint: endpoint!, recipient };
}

routes.patch("/connect/host/recipients/:id", async (c) => {
  await requireConnectFeature(c);
  const { endpoint, recipient } = await ownedConnectedRecipient(c, c.req.param("id"));
  if (!(["active", "paused"].includes(recipient.status))) throw new AppError(409, "share_recipient_terminal", "This access grant can no longer be changed.");
  const body = await readJSON<Record<string, unknown>>(c);
  const updates: string[] = [];
  const values: unknown[] = [];
  if (body.status !== undefined) {
    if (body.status !== "active" && body.status !== "paused") throw new AppError(400, "invalid_share_recipient_status", "The connected user status is invalid.");
    updates.push("status=?"); values.push(body.status);
  }
  if (body.rpm_limit !== undefined) { updates.push("rpm_limit=?"); values.push(boundedInteger(body.rpm_limit, 30, 1, 600, "invalid_share_rpm", "The share RPM must be between 1 and 600.")); }
  if (body.concurrency_limit !== undefined) { updates.push("concurrency_limit=?"); values.push(boundedInteger(body.concurrency_limit, 2, 1, 20, "invalid_share_concurrency", "The share concurrency must be between 1 and 20.")); }
  if (body.quota_requests !== undefined) { updates.push("quota_requests=?"); values.push(boundedInteger(body.quota_requests, 0, 0, 1_000_000, "invalid_share_quota", "The share quota must be between 0 and 1000000.")); }
  if (!updates.length) throw new AppError(400, "invalid_share_recipient_update", "No supported connected-user changes were provided.");
  const now = new Date().toISOString();
  updates.push("updated_at=?"); values.push(now, recipient.id);
  await c.env.DB.batch([
    c.env.DB.prepare(`UPDATE share_group_recipients SET ${updates.join(",")} WHERE id=?`).bind(...values),
    userEventStatement(c, c.get("auth").id, "connect.recipient_updated", "share_recipient", recipient.public_id),
    userEventStatement(c, recipient.recipient_id, "connect.access_updated", "received_share", recipient.public_id),
  ]);
  await writeShareAudit(c, endpoint.group_id, "share_connect.recipient_update", "share_recipient", recipient.public_id);
  return c.json(await hostPayload(c, endpoint));
});

routes.delete("/connect/host/recipients/:id", async (c) => {
  await requireConnectFeature(c);
  const { endpoint, recipient } = await ownedConnectedRecipient(c, c.req.param("id"));
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_access_keys SET status='revoked',revoked_at=? WHERE recipient_grant_id=? AND status IN ('prepared','active')").bind(now, recipient.id),
    c.env.DB.prepare("UPDATE share_group_recipients SET status='revoked',updated_at=? WHERE id=? AND status IN ('pending','active','paused')").bind(now, recipient.id),
    userEventStatement(c, c.get("auth").id, "connect.recipient_removed", "share_recipient", recipient.public_id),
    userEventStatement(c, recipient.recipient_id, "connect.access_removed", "received_share", recipient.public_id),
  ]);
  await writeShareAudit(c, endpoint.group_id, "share_connect.recipient_revoke", "share_recipient", recipient.public_id);
  return c.json({ ok: true });
});

async function guardAttempt(c: Context<AppEnv>, code: string, success = false, record = false): Promise<void> {
  const address = c.req.header("cf-connecting-ip") || "unknown";
  const id = c.env.SHARE_CONNECT_GUARD.idFromName(code);
  const response = await c.env.SHARE_CONNECT_GUARD.get(id).fetch("https://guard/check", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ key: `${c.get("auth").id}:${address}`, success, record }),
  });
  if (response.status === 429) {
    const data = await response.json<{ retry_after?: number }>();
    throw new AppError(429, "connect_attempts_locked", `Too many unsuccessful attempts. Try again in ${data.retry_after || 600} seconds.`);
  }
}

routes.post("/connect/claim", async (c) => {
  await requireConnectFeature(c);
  const body = await readJSON<Record<string, unknown>>(c);
  const receipt = await mutationReceipt(c, "share_connect.claim", body);
  if (receipt.replay) return receipt.replay;
  const code = normalizeCode(body.connection_code);
  const password = normalizePassword(body.password);
  const key = validateKeyMaterial(body.key_material);
  await guardAttempt(c, code);
  const endpoint = await c.env.DB.prepare(`SELECT e.id,e.public_id,e.owner_id,e.group_id,e.connection_code,e.status,e.created_at,e.updated_at,
    g.public_id AS group_public_id,g.status AS group_status FROM share_connect_endpoints e JOIN share_groups g ON g.id=e.group_id
    WHERE e.connection_code=? AND e.status='active' AND g.status='active'`).bind(code).first<EndpointRow>();
  if (!endpoint) {
    await guardAttempt(c, code, false, true);
    throw new AppError(404, "connect_entry_not_found", "The connection code or temporary password is incorrect.");
  }
  if (endpoint.owner_id === c.get("auth").id) throw new AppError(409, "connect_self_not_allowed", "You cannot connect to your own sharing entry.");
  const blocked = await c.env.DB.prepare(`SELECT 1 AS blocked FROM friend_blocks WHERE
    (blocker_id=? AND blocked_id=?) OR (blocker_id=? AND blocked_id=?) LIMIT 1`)
    .bind(endpoint.owner_id, c.get("auth").id, c.get("auth").id, endpoint.owner_id).first();
  if (blocked) throw new AppError(403, "connect_blocked", "This sharing entry is unavailable.");
  const window = await activeWindow(c, endpoint.id);
  if (!window || window.claimed_count >= window.max_claims) {
    throw new AppError(409, "connect_window_unavailable", "This temporary password has expired or reached its connection limit.");
  }
  const pepper = c.env.SHARE_CONNECT_PEPPER || "";
  if (pepper.length < 32) throw new AppError(503, "connect_not_configured", "Connection-code sharing is not configured.");
  const expected = await hmacBase64URL(pepper, `${endpoint.public_id}:${window.password_version}:${window.password_salt}:${password}`);
  if (!safeEqual(expected, window.password_verifier)) {
    await guardAttempt(c, code, false, true);
    throw new AppError(404, "connect_entry_not_found", "The connection code or temporary password is incorrect.");
  }
  const profile = await c.env.DB.prepare("SELECT display_name,encryption_key_version FROM friend_profiles WHERE user_id=?")
    .bind(c.get("auth").id).first<{ display_name: string; encryption_key_version: number }>();
  if (!profile || profile.encryption_key_version !== key.recipient_key_version) {
    throw new AppError(409, "recipient_key_changed", "Refresh your Cloud profile and try again.");
  }
  const current = await c.env.DB.prepare(`SELECT public_id FROM share_group_recipients WHERE group_id=? AND recipient_id=?
    AND status IN ('pending','active','paused')`).bind(endpoint.group_id, c.get("auth").id).first<{ public_id: string }>();
  if (current) throw new AppError(409, "already_connected", "You are already connected to this sharing entry.");
  const latest = await c.env.DB.prepare("SELECT COALESCE(MAX(generation),0) AS value FROM share_group_recipients WHERE group_id=? AND recipient_id=?")
    .bind(endpoint.group_id, c.get("auth").id).first<{ value: number }>();
  const recipientPublicID = newPublicID("sgr");
  const keyPublicID = newPublicID("sak");
  const claimPublicID = newPublicID("clm");
  const now = new Date().toISOString();
  const response = {
    share: {
      public_id: recipientPublicID, status: "active", group: { public_id: endpoint.group_public_id, name: "快速共享" },
      owner: { display_name: "" }, base_url: `${publicOrigin(c)}/v1`, rpm_limit: 30, concurrency_limit: 2,
      quota_requests: 0, used_requests: 0, accepted_at: now,
      key: { public_id: keyPublicID, key_prefix: key.key_prefix, key_version: 1, status: "active" },
    },
  };
  const owner = await c.env.DB.prepare("SELECT display_name FROM friend_profiles WHERE user_id=?").bind(endpoint.owner_id).first<{ display_name: string }>();
  response.share.owner.display_name = owner?.display_name || "Amber user";
  try {
    await c.env.DB.batch([
      c.env.DB.prepare(`INSERT INTO share_group_recipients
        (public_id,group_id,recipient_id,generation,status,rpm_limit,concurrency_limit,quota_requests,created_at,accepted_at,updated_at)
        VALUES(?,?,?,?,'active',30,2,0,?,?,?)`).bind(recipientPublicID, endpoint.group_id, c.get("auth").id, (latest?.value ?? 0) + 1, now, now, now),
      c.env.DB.prepare(`INSERT INTO share_access_keys
        (public_id,recipient_grant_id,key_version,key_prefix,guest_key_hash,key_envelope,envelope_context,recipient_key_version,status,created_at,activated_at)
        VALUES(?,(SELECT id FROM share_group_recipients WHERE public_id=?),1,?,?,?,?,?,'active',?,?)`)
        .bind(keyPublicID, recipientPublicID, key.key_prefix, key.guest_key_hash, key.key_envelope, key.envelope_context, key.recipient_key_version, now, now),
      c.env.DB.prepare(`INSERT INTO share_connect_claims
        (public_id,window_id,recipient_user_id,recipient_grant_id,idempotency_key,created_at)
        VALUES(?,?,?,(SELECT id FROM share_group_recipients WHERE public_id=?),?,?)`)
        .bind(claimPublicID, window.id, c.get("auth").id, recipientPublicID, receipt.key, now),
      receiptStatement(c, "share_connect.claim", receipt.key, receipt.requestHash, 201, JSON.stringify(response)),
      userEventStatement(c, endpoint.owner_id, "connect.recipient_joined", "share_recipient", recipientPublicID),
      userEventStatement(c, c.get("auth").id, "connect.access_claimed", "received_share", recipientPublicID),
    ]);
  } catch (error) {
    const replay = await mutationReceipt(c, "share_connect.claim", body);
    if (replay.replay) return replay.replay;
    const message = error instanceof Error ? error.message : "";
    if (message.includes("connect_window_unavailable")) {
      throw new AppError(409, "connect_window_unavailable", "This temporary password has expired or reached its connection limit.");
    }
    throw error;
  }
  await guardAttempt(c, code, true);
  await writeShareAudit(c, endpoint.group_id, "share_connect.claim", "share_recipient", recipientPublicID, { window_id: window.public_id });
  return c.json(response, 201);
});

export default routes;
