import { Hono, type Context } from "hono";
import { requireAuth } from "./auth-middleware";
import {
  boundedInteger, cleanText, futureExpiry, mutationReceipt, newPublicID, optionalText, parsePublicID,
  publicOrigin, receiptStatement, requireFeature, validateKeyMaterial, writeShareAudit, type KeyMaterialInput,
} from "./cloud-v2";
import { AppError, readJSON, requireJSONSize } from "./errors";
import { encryptShareCredential, validateShareCredential } from "./share-crypto";
import type { AppEnv } from "./types";

const groups = new Hono<AppEnv>();
const accountUIDPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

interface GroupRow {
  id: number;
  public_id: string;
  owner_id: number;
  name: string;
  description: string;
  status: "active" | "paused" | "deleted";
  route_policy: "balanced" | "failover";
  default_rpm: number;
  default_concurrency: number;
  default_quota_requests: number;
  default_expires_at: string | null;
  created_at: string;
  updated_at: string;
}

interface AccountInput {
  account_uid: string;
  account_type: "oauth" | "api_key";
  relay_mode: "owner_device" | "worker_direct";
  priority: number;
  weight: number;
  token_cipher: string | null;
  public_id: string;
}

interface RecipientInput {
  friendship_id: string;
  recipient_id: number;
  display_name: string;
  public_id: string;
  key_public_id: string;
  generation: number;
  key: KeyMaterialInput;
  rpm_limit: number;
  concurrency_limit: number;
  quota_requests: number;
  expires_at: string | null;
}

interface RecipientRow {
  id: number;
  public_id: string;
  group_id: number;
  recipient_id: number;
  generation: number;
  status: string;
  rpm_limit: number;
  concurrency_limit: number;
  quota_requests: number;
  used_requests: number;
  reserved_requests: number;
  expires_at: string | null;
  created_at: string;
  accepted_at: string | null;
  updated_at: string;
}

groups.use("/share-groups", requireAuth);
groups.use("/share-groups/*", requireAuth);

function routePolicy(value: unknown, fallback: "balanced" | "failover" = "balanced") {
  if (value === undefined) return fallback;
  if (value !== "balanced" && value !== "failover") {
    throw new AppError(400, "invalid_route_policy", "The share route policy is invalid.");
  }
  return value;
}

function groupStatus(value: unknown): "active" | "paused" {
  if (value !== "active" && value !== "paused") throw new AppError(400, "invalid_share_group_status", "The share group status is invalid.");
  return value;
}

async function ownedGroup(c: Context<AppEnv>, publicID: string, includeDeleted = false): Promise<GroupRow> {
  const row = await c.env.DB.prepare(`SELECT id,public_id,owner_id,name,description,status,route_policy,default_rpm,
    default_concurrency,default_quota_requests,default_expires_at,created_at,updated_at FROM share_groups
    WHERE public_id=? AND owner_id=?${includeDeleted ? "" : " AND status<>'deleted'"}`)
    .bind(parsePublicID(publicID, "invalid_share_group_id"), c.get("auth").id).first<GroupRow>();
  if (!row) throw new AppError(404, "share_group_not_found", "The share group was not found.");
  return row;
}

async function parseAccount(c: Context<AppEnv>, raw: unknown, publicID = newPublicID("sga")): Promise<AccountInput> {
  if (!raw || typeof raw !== "object") throw new AppError(400, "invalid_share_account", "A shared account is invalid.");
  const value = raw as Record<string, unknown>;
  const accountUID = typeof value.account_uid === "string" ? value.account_uid.trim() : "";
  const accountType = value.account_type;
  const relayMode = value.relay_mode ?? "owner_device";
  if (!accountUIDPattern.test(accountUID) || (accountType !== "oauth" && accountType !== "api_key") ||
      (relayMode !== "owner_device" && relayMode !== "worker_direct")) {
    throw new AppError(400, "invalid_share_account", "A shared account is invalid.");
  }
  const synced = await c.env.DB.prepare(`SELECT id FROM vault_items
    WHERE user_id=? AND kind='account' AND client_uid=? AND deleted=0`).bind(c.get("auth").id, accountUID).first();
  if (!synced) throw new AppError(409, "account_not_synced", "Sync every selected account to Amber Cloud before sharing it.");
  let tokenCipher: string | null = null;
  if (relayMode === "worker_direct") {
    if (accountType !== "api_key") throw new AppError(400, "oauth_owner_relay_required", "OAuth accounts must use owner-device relay.");
    const credential = validateShareCredential(value.credential);
    if (credential.account_type !== "api_key") throw new AppError(400, "invalid_share_credential", "A Worker-direct account requires an API-key credential.");
    tokenCipher = await encryptShareCredential(c.env, c.get("auth").id, accountUID, credential);
  } else if (value.credential !== undefined) {
    throw new AppError(400, "unexpected_share_credential", "Owner-device accounts must not upload a credential.");
  }
  return {
    account_uid: accountUID,
    account_type: accountType,
    relay_mode: relayMode,
    priority: boundedInteger(value.priority, 100, 1, 1000, "invalid_account_priority", "Account priority must be between 1 and 1000."),
    weight: boundedInteger(value.weight, 100, 1, 1000, "invalid_account_weight", "Account weight must be between 1 and 1000."),
    token_cipher: tokenCipher,
    public_id: publicID,
  };
}

async function resolveRecipient(
  c: Context<AppEnv>, raw: unknown, defaults: { rpm: number; concurrency: number; quota: number; expiry: string | null },
  generation = 1,
): Promise<RecipientInput> {
  if (!raw || typeof raw !== "object") throw new AppError(400, "invalid_share_recipient", "A share recipient is invalid.");
  const value = raw as Record<string, unknown>;
  const friendshipID = parsePublicID(typeof value.friendship_id === "string" ? value.friendship_id : "", "invalid_friendship_id");
  const userID = c.get("auth").id;
  const friend = await c.env.DB.prepare(`SELECT f.user_low_id,f.user_high_id,
      CASE WHEN f.user_low_id=? THEN high.display_name ELSE low.display_name END AS display_name,
      CASE WHEN f.user_low_id=? THEN high.encryption_key_version ELSE low.encryption_key_version END AS encryption_key_version
    FROM friendships f JOIN friend_profiles low ON low.user_id=f.user_low_id JOIN friend_profiles high ON high.user_id=f.user_high_id
    WHERE f.public_id=? AND f.status='active' AND (f.user_low_id=? OR f.user_high_id=?)`)
    .bind(userID, userID, friendshipID, userID, userID).first<{
      user_low_id: number; user_high_id: number; display_name: string; encryption_key_version: number;
    }>();
  if (!friend) throw new AppError(403, "friendship_required", "Only accepted friends can receive a share.");
  const recipientID = friend.user_low_id === userID ? friend.user_high_id : friend.user_low_id;
  const key = validateKeyMaterial(value.key_material);
  if (key.recipient_key_version !== friend.encryption_key_version) {
    throw new AppError(409, "recipient_key_changed", "The friend's encryption key changed. Refresh the friend list and try again.");
  }
  return {
    friendship_id: friendshipID,
    recipient_id: recipientID,
    display_name: friend.display_name,
    public_id: newPublicID("sgr"),
    key_public_id: newPublicID("sak"),
    generation,
    key,
    rpm_limit: boundedInteger(value.rpm_limit, defaults.rpm, 1, 600, "invalid_share_rpm", "The share RPM must be between 1 and 600."),
    concurrency_limit: boundedInteger(value.concurrency_limit, defaults.concurrency, 1, 20, "invalid_share_concurrency", "The share concurrency must be between 1 and 20."),
    quota_requests: boundedInteger(value.quota_requests, defaults.quota, 0, 1_000_000, "invalid_share_quota", "The share quota must be between 0 and 1000000."),
    expires_at: futureExpiry(value.expires_at, defaults.expiry),
  };
}

function groupPayload(group: GroupRow | Record<string, unknown>, origin: string) {
  return { ...group, base_url: `${origin}/v1` };
}

groups.get("/share-groups", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const result = await c.env.DB.prepare(`SELECT g.public_id,g.name,g.description,g.status,g.route_policy,g.default_rpm,
    g.default_concurrency,g.default_quota_requests,g.default_expires_at,g.created_at,g.updated_at,
    (SELECT COUNT(*) FROM share_group_accounts a WHERE a.group_id=g.id) AS account_count,
    (SELECT COUNT(*) FROM share_group_accounts a WHERE a.group_id=g.id AND a.enabled=1) AS enabled_account_count,
    (SELECT COUNT(*) FROM share_group_recipients r WHERE r.group_id=g.id AND r.status IN ('pending','active','paused')) AS recipient_count,
    (SELECT COALESCE(SUM(r.used_requests),0) FROM share_group_recipients r WHERE r.group_id=g.id) AS used_requests
    FROM share_groups g WHERE g.owner_id=? AND g.status<>'deleted' ORDER BY g.updated_at DESC,g.id DESC LIMIT 200`)
    .bind(c.get("auth").id).all();
  return c.json({ groups: result.results.map((row) => groupPayload(row, publicOrigin(c))) });
});

groups.post("/share-groups", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  requireJSONSize(c, 256 * 1024);
  const body = await readJSON<Record<string, unknown>>(c);
  const receipt = await mutationReceipt(c, "share_group.create", body);
  if (receipt.replay) return receipt.replay;
  const name = cleanText(body.name, 2, 40, "invalid_share_group_name", "The share group name must contain 2 to 40 characters.");
  const description = optionalText(body.description, 120, "invalid_share_group_description", "The share group description is too long.");
  const policy = routePolicy(body.route_policy);
  const defaultRPM = boundedInteger(body.default_rpm, 30, 1, 600, "invalid_share_rpm", "The share RPM must be between 1 and 600.");
  const defaultConcurrency = boundedInteger(body.default_concurrency, 2, 1, 20, "invalid_share_concurrency", "The share concurrency must be between 1 and 20.");
  const defaultQuota = boundedInteger(body.default_quota_requests, 0, 0, 1_000_000, "invalid_share_quota", "The share quota must be between 0 and 1000000.");
  const defaultExpiry = futureExpiry(body.default_expires_at);
  const rawAccounts = Array.isArray(body.accounts) ? body.accounts : [];
  const rawRecipients = Array.isArray(body.recipients) ? body.recipients : [];
  if (rawAccounts.length < 1 || rawAccounts.length > 20) throw new AppError(400, "invalid_share_accounts", "Select between 1 and 20 accounts.");
  if (rawRecipients.length < 1 || rawRecipients.length > 50) throw new AppError(400, "invalid_share_recipients", "Select between 1 and 50 friends.");
  const accounts: AccountInput[] = [];
  const accountUIDs = new Set<string>();
  for (const raw of rawAccounts) {
    const account = await parseAccount(c, raw);
    if (accountUIDs.has(account.account_uid)) throw new AppError(400, "duplicate_share_account", "Each account can be selected only once.");
    accountUIDs.add(account.account_uid);
    accounts.push(account);
  }
  const recipientDefaults = { rpm: defaultRPM, concurrency: defaultConcurrency, quota: defaultQuota, expiry: defaultExpiry };
  const recipients: RecipientInput[] = [];
  const recipientIDs = new Set<number>();
  for (const raw of rawRecipients) {
    const recipient = await resolveRecipient(c, raw, recipientDefaults);
    if (recipientIDs.has(recipient.recipient_id)) throw new AppError(400, "duplicate_share_recipient", "Each friend can be selected only once.");
    recipientIDs.add(recipient.recipient_id);
    recipients.push(recipient);
  }
  const groupPublicID = newPublicID("grp");
  const now = new Date().toISOString();
  const response = {
    group: groupPayload({
      public_id: groupPublicID, name, description, status: "active", route_policy: policy,
      default_rpm: defaultRPM, default_concurrency: defaultConcurrency, default_quota_requests: defaultQuota,
      default_expires_at: defaultExpiry, account_count: accounts.length, recipient_count: recipients.length,
      created_at: now, updated_at: now,
    }, publicOrigin(c)),
    accounts: accounts.map(({ token_cipher: _secret, ...account }) => account),
    recipients: recipients.map((recipient) => ({
      public_id: recipient.public_id, display_name: recipient.display_name, status: "pending",
      key_prefix: recipient.key.key_prefix, rpm_limit: recipient.rpm_limit,
      concurrency_limit: recipient.concurrency_limit, quota_requests: recipient.quota_requests,
      expires_at: recipient.expires_at,
    })),
  };
  const responseBody = JSON.stringify(response);
  const statements: D1PreparedStatement[] = [
    c.env.DB.prepare(`INSERT INTO share_groups
      (public_id,owner_id,name,description,status,route_policy,default_rpm,default_concurrency,default_quota_requests,
       default_expires_at,created_at,updated_at) VALUES(?,?,?,?,'active',?,?,?,?,?,?,?)`)
      .bind(groupPublicID, c.get("auth").id, name, description, policy, defaultRPM, defaultConcurrency, defaultQuota, defaultExpiry, now, now),
  ];
  for (const account of accounts) {
    statements.push(c.env.DB.prepare(`INSERT INTO share_group_accounts
      (public_id,group_id,account_uid,account_type,relay_mode,priority,weight,enabled,token_cipher,created_at,updated_at)
      VALUES(?,(SELECT id FROM share_groups WHERE public_id=?),?,?,?,?,?,1,?,?,?)`)
      .bind(account.public_id, groupPublicID, account.account_uid, account.account_type, account.relay_mode,
        account.priority, account.weight, account.token_cipher, now, now));
  }
  for (const recipient of recipients) {
    statements.push(
      c.env.DB.prepare(`INSERT INTO share_group_recipients
        (public_id,group_id,recipient_id,generation,status,rpm_limit,concurrency_limit,quota_requests,expires_at,created_at,updated_at)
        VALUES(?,(SELECT id FROM share_groups WHERE public_id=?),?,?,'pending',?,?,?,?,?,?)`)
        .bind(recipient.public_id, groupPublicID, recipient.recipient_id, recipient.generation, recipient.rpm_limit,
          recipient.concurrency_limit, recipient.quota_requests, recipient.expires_at, now, now),
      c.env.DB.prepare(`INSERT INTO share_access_keys
        (public_id,recipient_grant_id,key_version,key_prefix,guest_key_hash,key_envelope,envelope_context,recipient_key_version,status,created_at)
        VALUES(?,(SELECT id FROM share_group_recipients WHERE public_id=?),1,?,?,?,?,?,'prepared',?)`)
        .bind(recipient.key_public_id, recipient.public_id, recipient.key.key_prefix, recipient.key.guest_key_hash,
          recipient.key.key_envelope, recipient.key.envelope_context, recipient.key.recipient_key_version, now),
    );
  }
  statements.push(
    c.env.DB.prepare(`INSERT INTO share_audit_log(group_id,actor_user_id,action,target_type,target_public_id,details,created_at)
      VALUES((SELECT id FROM share_groups WHERE public_id=?),?,'share_group.create','share_group',?,'{}',?)`)
      .bind(groupPublicID, c.get("auth").id, groupPublicID, now),
    receiptStatement(c, "share_group.create", receipt.key, receipt.requestHash, 201, responseBody),
  );
  try {
    await c.env.DB.batch(statements);
  } catch (error) {
    const replay = await mutationReceipt(c, "share_group.create", body);
    if (replay.replay) return replay.replay;
    throw error;
  }
  return c.json(response, 201);
});

async function groupDetails(c: Context<AppEnv>, group: GroupRow) {
  const [accounts, recipients] = await Promise.all([
    c.env.DB.prepare(`SELECT public_id,account_uid,account_type,relay_mode,priority,weight,enabled,created_at,updated_at
      FROM share_group_accounts WHERE group_id=? ORDER BY priority,id`).bind(group.id).all(),
    c.env.DB.prepare(`SELECT r.public_id,r.status,r.generation,r.rpm_limit,r.concurrency_limit,r.quota_requests,
      r.used_requests,r.reserved_requests,r.expires_at,r.created_at,r.accepted_at,r.updated_at,p.display_name,
      k.public_id AS key_id,k.key_prefix,k.key_version,k.status AS key_status,f.public_id AS friendship_id
      FROM share_group_recipients r JOIN friend_profiles p ON p.user_id=r.recipient_id
      LEFT JOIN share_access_keys k ON k.recipient_grant_id=r.id AND k.status IN ('prepared','active')
      LEFT JOIN friendships f ON f.status='active' AND f.user_low_id=MIN(?,r.recipient_id) AND f.user_high_id=MAX(?,r.recipient_id)
      WHERE r.group_id=? ORDER BY r.created_at DESC,r.id DESC`).bind(group.owner_id, group.owner_id, group.id).all(),
  ]);
  return { group: groupPayload(group, publicOrigin(c)), accounts: accounts.results, recipients: recipients.results };
}

groups.get("/share-groups/:id", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  return c.json(await groupDetails(c, await ownedGroup(c, c.req.param("id"))));
});

groups.patch("/share-groups/:id", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const body = await readJSON<Record<string, unknown>>(c);
  const updates: string[] = [];
  const values: unknown[] = [];
  if (body.name !== undefined) { updates.push("name=?"); values.push(cleanText(body.name, 2, 40, "invalid_share_group_name", "The share group name must contain 2 to 40 characters.")); }
  if (body.description !== undefined) { updates.push("description=?"); values.push(optionalText(body.description, 120, "invalid_share_group_description", "The share group description is too long.")); }
  if (body.status !== undefined) { updates.push("status=?"); values.push(groupStatus(body.status)); }
  if (body.route_policy !== undefined) { updates.push("route_policy=?"); values.push(routePolicy(body.route_policy)); }
  if (body.default_rpm !== undefined) { updates.push("default_rpm=?"); values.push(boundedInteger(body.default_rpm, 30, 1, 600, "invalid_share_rpm", "The share RPM must be between 1 and 600.")); }
  if (body.default_concurrency !== undefined) { updates.push("default_concurrency=?"); values.push(boundedInteger(body.default_concurrency, 2, 1, 20, "invalid_share_concurrency", "The share concurrency must be between 1 and 20.")); }
  if (body.default_quota_requests !== undefined) { updates.push("default_quota_requests=?"); values.push(boundedInteger(body.default_quota_requests, 0, 0, 1_000_000, "invalid_share_quota", "The share quota must be between 0 and 1000000.")); }
  if (body.default_expires_at !== undefined) { updates.push("default_expires_at=?"); values.push(futureExpiry(body.default_expires_at)); }
  if (!updates.length) throw new AppError(400, "invalid_share_group_update", "No supported share group changes were provided.");
  const now = new Date().toISOString();
  updates.push("updated_at=?");
  values.push(now, group.id);
  await c.env.DB.prepare(`UPDATE share_groups SET ${updates.join(",")} WHERE id=?`).bind(...values).run();
  await writeShareAudit(c, group.id, body.status === "paused" ? "share_group.pause" : body.status === "active" ? "share_group.resume" : "share_group.update", "share_group", group.public_id);
  return c.json(await groupDetails(c, await ownedGroup(c, group.public_id)));
});

groups.delete("/share-groups/:id", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_access_keys SET status='revoked',revoked_at=? WHERE recipient_grant_id IN (SELECT id FROM share_group_recipients WHERE group_id=?) AND status IN ('prepared','active')").bind(now, group.id),
    c.env.DB.prepare("UPDATE share_group_recipients SET status='revoked',updated_at=? WHERE group_id=? AND status IN ('pending','active','paused')").bind(now, group.id),
    c.env.DB.prepare("UPDATE share_groups SET status='deleted',deleted_at=?,updated_at=? WHERE id=?").bind(now, now, group.id),
    c.env.DB.prepare(`INSERT INTO share_audit_log(group_id,actor_user_id,action,target_type,target_public_id,details,created_at)
      VALUES(?,?,'share_group.delete','share_group',?,'{}',?)`).bind(group.id, c.get("auth").id, group.public_id, now),
  ]);
  return c.json({ ok: true });
});

groups.post("/share-groups/:id/accounts", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const body = await readJSON<Record<string, unknown>>(c);
  const account = await parseAccount(c, body);
  const now = new Date().toISOString();
  try {
    await c.env.DB.prepare(`INSERT INTO share_group_accounts
      (public_id,group_id,account_uid,account_type,relay_mode,priority,weight,enabled,token_cipher,created_at,updated_at)
      VALUES(?,?,?,?,?,?,?,1,?,?,?)`).bind(account.public_id, group.id, account.account_uid, account.account_type,
        account.relay_mode, account.priority, account.weight, account.token_cipher, now, now).run();
  } catch {
    throw new AppError(409, "share_account_exists", "This account is already in the share group.");
  }
  await writeShareAudit(c, group.id, "share_account.add", "share_account", account.public_id);
  return c.json({ account: { ...account, token_cipher: undefined, enabled: true, created_at: now, updated_at: now } }, 201);
});

groups.patch("/share-groups/:id/accounts/:accountId", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const accountID = parsePublicID(c.req.param("accountId"), "invalid_share_account_id");
  const body = await readJSON<Record<string, unknown>>(c);
  const updates: string[] = [];
  const values: unknown[] = [];
  if (typeof body.enabled === "boolean") { updates.push("enabled=?"); values.push(body.enabled ? 1 : 0); }
  if (body.priority !== undefined) { updates.push("priority=?"); values.push(boundedInteger(body.priority, 100, 1, 1000, "invalid_account_priority", "Account priority must be between 1 and 1000.")); }
  if (body.weight !== undefined) { updates.push("weight=?"); values.push(boundedInteger(body.weight, 100, 1, 1000, "invalid_account_weight", "Account weight must be between 1 and 1000.")); }
  if (!updates.length) throw new AppError(400, "invalid_share_account_update", "No supported account changes were provided.");
  updates.push("updated_at=?");
  values.push(new Date().toISOString(), group.id, accountID);
  const result = await c.env.DB.prepare(`UPDATE share_group_accounts SET ${updates.join(",")} WHERE group_id=? AND public_id=?`).bind(...values).run();
  if (!result.meta.changes) throw new AppError(404, "share_account_not_found", "The shared account was not found.");
  await writeShareAudit(c, group.id, "share_account.update", "share_account", accountID);
  return c.json({ ok: true });
});

groups.delete("/share-groups/:id/accounts/:accountId", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const accountID = parsePublicID(c.req.param("accountId"), "invalid_share_account_id");
  const result = await c.env.DB.prepare("DELETE FROM share_group_accounts WHERE group_id=? AND public_id=?").bind(group.id, accountID).run();
  if (!result.meta.changes) throw new AppError(404, "share_account_not_found", "The shared account was not found.");
  await writeShareAudit(c, group.id, "share_account.remove", "share_account", accountID);
  return c.json({ ok: true });
});

async function ownedRecipient(c: Context<AppEnv>, group: GroupRow, publicID: string): Promise<RecipientRow> {
  const row = await c.env.DB.prepare(`SELECT id,public_id,group_id,recipient_id,generation,status,rpm_limit,concurrency_limit,
    quota_requests,used_requests,reserved_requests,expires_at,created_at,accepted_at,updated_at
    FROM share_group_recipients WHERE public_id=? AND group_id=?`)
    .bind(parsePublicID(publicID, "invalid_share_recipient_id"), group.id).first<RecipientRow>();
  if (!row) throw new AppError(404, "share_recipient_not_found", "The share recipient was not found.");
  return row;
}

groups.post("/share-groups/:id/recipients", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const body = await readJSON<Record<string, unknown>>(c);
  const receipt = await mutationReceipt(c, `share_group.${group.public_id}.invite`, body);
  if (receipt.replay) return receipt.replay;
  const rawRecipients = Array.isArray(body.recipients) ? body.recipients : [];
  if (rawRecipients.length < 1 || rawRecipients.length > 50) throw new AppError(400, "invalid_share_recipients", "Select between 1 and 50 friends.");
  const defaults = { rpm: group.default_rpm, concurrency: group.default_concurrency, quota: group.default_quota_requests, expiry: group.default_expires_at };
  const recipients: RecipientInput[] = [];
  const seen = new Set<number>();
  for (const raw of rawRecipients) {
    const provisional = await resolveRecipient(c, raw, defaults);
    if (seen.has(provisional.recipient_id)) throw new AppError(400, "duplicate_share_recipient", "Each friend can be selected only once.");
    seen.add(provisional.recipient_id);
    const active = await c.env.DB.prepare(`SELECT id FROM share_group_recipients WHERE group_id=? AND recipient_id=?
      AND status IN ('pending','active','paused')`).bind(group.id, provisional.recipient_id).first();
    if (active) throw new AppError(409, "share_recipient_exists", "This friend already has a current invitation or access grant.");
    const latest = await c.env.DB.prepare("SELECT COALESCE(MAX(generation),0) AS value FROM share_group_recipients WHERE group_id=? AND recipient_id=?")
      .bind(group.id, provisional.recipient_id).first<{ value: number }>();
    provisional.generation = (latest?.value ?? 0) + 1;
    recipients.push(provisional);
  }
  const now = new Date().toISOString();
  const response = { recipients: recipients.map((recipient) => ({
    public_id: recipient.public_id, display_name: recipient.display_name, status: "pending", key_prefix: recipient.key.key_prefix,
    rpm_limit: recipient.rpm_limit, concurrency_limit: recipient.concurrency_limit, quota_requests: recipient.quota_requests,
    expires_at: recipient.expires_at,
  })) };
  const responseBody = JSON.stringify(response);
  const statements: D1PreparedStatement[] = [];
  for (const recipient of recipients) {
    statements.push(
      c.env.DB.prepare(`INSERT INTO share_group_recipients
        (public_id,group_id,recipient_id,generation,status,rpm_limit,concurrency_limit,quota_requests,expires_at,created_at,updated_at)
        VALUES(?,?,?,?,'pending',?,?,?,?,?,?)`).bind(recipient.public_id, group.id, recipient.recipient_id, recipient.generation,
          recipient.rpm_limit, recipient.concurrency_limit, recipient.quota_requests, recipient.expires_at, now, now),
      c.env.DB.prepare(`INSERT INTO share_access_keys
        (public_id,recipient_grant_id,key_version,key_prefix,guest_key_hash,key_envelope,envelope_context,recipient_key_version,status,created_at)
        VALUES(?,(SELECT id FROM share_group_recipients WHERE public_id=?),1,?,?,?,?,?,'prepared',?)`)
        .bind(recipient.key_public_id, recipient.public_id, recipient.key.key_prefix, recipient.key.guest_key_hash,
          recipient.key.key_envelope, recipient.key.envelope_context, recipient.key.recipient_key_version, now),
    );
  }
  statements.push(receiptStatement(c, `share_group.${group.public_id}.invite`, receipt.key, receipt.requestHash, 201, responseBody));
  await c.env.DB.batch(statements);
  await writeShareAudit(c, group.id, "share_recipient.invite", "share_group", group.public_id, { count: recipients.length });
  return c.json(response, 201);
});

groups.patch("/share-groups/:id/recipients/:recipientId", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const recipient = await ownedRecipient(c, group, c.req.param("recipientId"));
  const body = await readJSON<Record<string, unknown>>(c);
  const updates: string[] = [];
  const values: unknown[] = [];
  if (body.status !== undefined) {
    if (body.status !== "active" && body.status !== "paused") throw new AppError(400, "invalid_share_recipient_status", "The recipient status is invalid.");
    if (!(["active", "paused"].includes(recipient.status))) throw new AppError(409, "share_recipient_terminal", "This access grant can no longer be changed.");
    if (body.status === "active") {
      const friendship = await c.env.DB.prepare(`SELECT id FROM friendships WHERE status='active'
        AND ((user_low_id=? AND user_high_id=?) OR (user_low_id=? AND user_high_id=?))`)
        .bind(Math.min(group.owner_id, recipient.recipient_id), Math.max(group.owner_id, recipient.recipient_id),
          Math.min(group.owner_id, recipient.recipient_id), Math.max(group.owner_id, recipient.recipient_id)).first();
      if (!friendship) throw new AppError(403, "friendship_required", "Restore the friendship before restoring access.");
    }
    updates.push("status=?"); values.push(body.status);
  }
  if (body.rpm_limit !== undefined) { updates.push("rpm_limit=?"); values.push(boundedInteger(body.rpm_limit, recipient.rpm_limit, 1, 600, "invalid_share_rpm", "The share RPM must be between 1 and 600.")); }
  if (body.concurrency_limit !== undefined) { updates.push("concurrency_limit=?"); values.push(boundedInteger(body.concurrency_limit, recipient.concurrency_limit, 1, 20, "invalid_share_concurrency", "The share concurrency must be between 1 and 20.")); }
  if (body.quota_requests !== undefined) { updates.push("quota_requests=?"); values.push(boundedInteger(body.quota_requests, recipient.quota_requests, 0, 1_000_000, "invalid_share_quota", "The share quota must be between 0 and 1000000.")); }
  if (body.expires_at !== undefined) { updates.push("expires_at=?"); values.push(futureExpiry(body.expires_at)); }
  if (!updates.length) throw new AppError(400, "invalid_share_recipient_update", "No supported recipient changes were provided.");
  updates.push("updated_at=?"); values.push(new Date().toISOString(), recipient.id);
  await c.env.DB.prepare(`UPDATE share_group_recipients SET ${updates.join(",")} WHERE id=?`).bind(...values).run();
  await writeShareAudit(c, group.id, body.status === "paused" ? "share_recipient.pause" : body.status === "active" ? "share_recipient.resume" : "share_recipient.update", "share_recipient", recipient.public_id);
  return c.json({ recipient: await ownedRecipient(c, group, recipient.public_id) });
});

groups.delete("/share-groups/:id/recipients/:recipientId", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const recipient = await ownedRecipient(c, group, c.req.param("recipientId"));
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_access_keys SET status='revoked',revoked_at=? WHERE recipient_grant_id=? AND status IN ('prepared','active')").bind(now, recipient.id),
    c.env.DB.prepare("UPDATE share_group_recipients SET status='revoked',updated_at=? WHERE id=?").bind(now, recipient.id),
  ]);
  await writeShareAudit(c, group.id, "share_recipient.revoke", "share_recipient", recipient.public_id);
  return c.json({ ok: true });
});

groups.post("/share-groups/:id/recipients/:recipientId/keys/rotate", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const recipient = await ownedRecipient(c, group, c.req.param("recipientId"));
  if (!(["active", "paused"].includes(recipient.status))) throw new AppError(409, "share_recipient_terminal", "This access grant can no longer rotate keys.");
  const body = await readJSON<Record<string, unknown>>(c);
  const receipt = await mutationReceipt(c, `share_recipient.${recipient.public_id}.rotate`, body);
  if (receipt.replay) return receipt.replay;
  const key = validateKeyMaterial(body.key_material);
  const profile = await c.env.DB.prepare("SELECT encryption_key_version FROM friend_profiles WHERE user_id=?")
    .bind(recipient.recipient_id).first<{ encryption_key_version: number }>();
  if (!profile || profile.encryption_key_version !== key.recipient_key_version) {
    throw new AppError(409, "recipient_key_changed", "The friend's encryption key changed. Refresh the friend list and try again.");
  }
  const current = await c.env.DB.prepare("SELECT COALESCE(MAX(key_version),0) AS value FROM share_access_keys WHERE recipient_grant_id=?")
    .bind(recipient.id).first<{ value: number }>();
  const keyVersion = (current?.value ?? 0) + 1;
  const keyPublicID = newPublicID("sak");
  const now = new Date().toISOString();
  const response = { key: { public_id: keyPublicID, key_prefix: key.key_prefix, key_version: keyVersion, status: "active" } };
  const responseBody = JSON.stringify(response);
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_access_keys SET status='replaced',revoked_at=? WHERE recipient_grant_id=? AND status='active'").bind(now, recipient.id),
    c.env.DB.prepare(`INSERT INTO share_access_keys
      (public_id,recipient_grant_id,key_version,key_prefix,guest_key_hash,key_envelope,envelope_context,recipient_key_version,status,created_at,activated_at)
      VALUES(?,?,?,?,?,?,?,?,'active',?,?)`).bind(keyPublicID, recipient.id, keyVersion, key.key_prefix, key.guest_key_hash,
        key.key_envelope, key.envelope_context, key.recipient_key_version, now, now),
    receiptStatement(c, `share_recipient.${recipient.public_id}.rotate`, receipt.key, receipt.requestHash, 201, responseBody),
  ]);
  await writeShareAudit(c, group.id, "share_key.rotate", "share_recipient", recipient.public_id, { key_version: keyVersion });
  return c.json(response, 201);
});

groups.get("/share-groups/:id/usage", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const result = await c.env.DB.prepare(`SELECT u.request_id,u.route_mode,u.model,u.status,u.error_code,u.input_tokens,u.output_tokens,
    u.latency_ms,u.created_at,r.public_id AS recipient_id,p.display_name,a.public_id AS account_id
    FROM share_usage_log_v2 u JOIN share_group_recipients r ON r.id=u.recipient_grant_id
    JOIN friend_profiles p ON p.user_id=r.recipient_id LEFT JOIN share_group_accounts a ON a.id=u.group_account_id
    WHERE u.group_id=? ORDER BY u.created_at DESC,u.id DESC LIMIT 500`).bind(group.id).all();
  return c.json({ usage: result.results });
});

groups.get("/share-groups/:id/audit", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const group = await ownedGroup(c, c.req.param("id"));
  const result = await c.env.DB.prepare(`SELECT action,target_type,target_public_id,details,created_at
    FROM share_audit_log WHERE group_id=? ORDER BY created_at DESC,id DESC LIMIT 500`).bind(group.id).all();
  return c.json({ audit: result.results });
});

export default groups;
