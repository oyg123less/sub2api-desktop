import { Hono, type Context } from "hono";
import { requireAuth } from "./auth-middleware";
import { parsePublicID, publicOrigin, requireFeature, writeShareAudit } from "./cloud-v2";
import { AppError } from "./errors";
import type { AppEnv } from "./types";

const received = new Hono<AppEnv>();

interface ReceivedRow {
  id: number;
  public_id: string;
  group_id: number;
  recipient_id: number;
  status: string;
  rpm_limit: number;
  concurrency_limit: number;
  quota_requests: number;
  used_requests: number;
  expires_at: string | null;
  created_at: string;
  accepted_at: string | null;
  group_public_id: string;
  group_name: string;
  group_description: string;
  group_status: string;
  route_policy: string;
  owner_id: number;
  owner_name: string;
  account_count: number;
  owner_device_required: number;
}

received.use("/received-shares", requireAuth);
received.use("/received-shares/*", requireAuth);

async function loadReceived(c: Context<AppEnv>, publicID: string): Promise<ReceivedRow> {
  const row = await c.env.DB.prepare(`SELECT r.id,r.public_id,r.group_id,r.recipient_id,r.status,r.rpm_limit,r.concurrency_limit,
    r.quota_requests,r.used_requests,r.expires_at,r.created_at,r.accepted_at,g.public_id AS group_public_id,
    g.name AS group_name,g.description AS group_description,g.status AS group_status,g.route_policy,g.owner_id,
    owner.display_name AS owner_name,
    (SELECT COUNT(*) FROM share_group_accounts a WHERE a.group_id=g.id AND a.enabled=1) AS account_count,
    (SELECT CASE WHEN EXISTS(SELECT 1 FROM share_group_accounts a WHERE a.group_id=g.id AND a.enabled=1 AND a.relay_mode='owner_device') THEN 1 ELSE 0 END) AS owner_device_required
    FROM share_group_recipients r JOIN share_groups g ON g.id=r.group_id JOIN friend_profiles owner ON owner.user_id=g.owner_id
    WHERE r.public_id=? AND r.recipient_id=?`)
    .bind(parsePublicID(publicID, "invalid_received_share_id"), c.get("auth").id).first<ReceivedRow>();
  if (!row) throw new AppError(404, "received_share_not_found", "The received share was not found.");
  return row;
}

function publicReceived(row: ReceivedRow, origin: string, key?: Record<string, unknown> | null) {
  return {
    public_id: row.public_id,
    status: row.status,
    group: {
      public_id: row.group_public_id,
      name: row.group_name,
      description: row.group_description,
      status: row.group_status,
      route_policy: row.route_policy,
      account_count: row.account_count,
      owner_device_required: Boolean(row.owner_device_required),
    },
    owner: { display_name: row.owner_name },
    rpm_limit: row.rpm_limit,
    concurrency_limit: row.concurrency_limit,
    quota_requests: row.quota_requests,
    used_requests: row.used_requests,
    expires_at: row.expires_at,
    created_at: row.created_at,
    accepted_at: row.accepted_at,
    base_url: `${origin}/v1`,
    ...(key ? { key } : {}),
  };
}

received.get("/received-shares", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const now = new Date().toISOString();
  await c.env.DB.prepare(`UPDATE share_group_recipients SET status='expired',updated_at=?
    WHERE recipient_id=? AND status IN ('pending','active','paused') AND expires_at IS NOT NULL AND expires_at<=?`)
    .bind(now, c.get("auth").id, now).run();
  await c.env.DB.prepare(`UPDATE share_access_keys SET status='expired',revoked_at=? WHERE recipient_grant_id IN
    (SELECT id FROM share_group_recipients WHERE recipient_id=? AND status='expired') AND status IN ('prepared','active')`)
    .bind(now, c.get("auth").id).run();
  const result = await c.env.DB.prepare(`SELECT r.id,r.public_id,r.group_id,r.recipient_id,r.status,r.rpm_limit,r.concurrency_limit,
    r.quota_requests,r.used_requests,r.expires_at,r.created_at,r.accepted_at,g.public_id AS group_public_id,
    g.name AS group_name,g.description AS group_description,g.status AS group_status,g.route_policy,g.owner_id,
    owner.display_name AS owner_name,
    (SELECT COUNT(*) FROM share_group_accounts a WHERE a.group_id=g.id AND a.enabled=1) AS account_count,
    (SELECT CASE WHEN EXISTS(SELECT 1 FROM share_group_accounts a WHERE a.group_id=g.id AND a.enabled=1 AND a.relay_mode='owner_device') THEN 1 ELSE 0 END) AS owner_device_required,
    k.public_id AS key_public_id,k.key_prefix,k.key_version,k.status AS key_status
    FROM share_group_recipients r JOIN share_groups g ON g.id=r.group_id JOIN friend_profiles owner ON owner.user_id=g.owner_id
    LEFT JOIN share_access_keys k ON k.recipient_grant_id=r.id AND k.status IN ('prepared','active')
    WHERE r.recipient_id=? ORDER BY CASE r.status WHEN 'pending' THEN 0 WHEN 'active' THEN 1 WHEN 'paused' THEN 2 ELSE 3 END,
      r.updated_at DESC,r.id DESC LIMIT 200`).bind(c.get("auth").id).all<ReceivedRow & {
        key_public_id: string | null; key_prefix: string | null; key_version: number | null; key_status: string | null;
      }>();
  return c.json({ shares: result.results.map((row) => publicReceived(row, publicOrigin(c), row.key_public_id ? {
    public_id: row.key_public_id, key_prefix: row.key_prefix, key_version: row.key_version, status: row.key_status,
  } : null)) });
});

received.post("/received-shares/:id/accept", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const share = await loadReceived(c, c.req.param("id"));
  if (share.status === "active") {
    const key = await c.env.DB.prepare(`SELECT public_id,key_prefix,key_version,key_envelope,envelope_context,recipient_key_version,status FROM share_access_keys
      WHERE recipient_grant_id=? AND status='active'`).bind(share.id).first<Record<string, unknown>>();
    return c.json({ share: publicReceived(share, publicOrigin(c), key) });
  }
  if (share.status !== "pending") throw new AppError(409, "share_invitation_unavailable", "This share invitation can no longer be accepted.");
  if (share.group_status === "deleted" || (share.expires_at && Date.parse(share.expires_at) <= Date.now())) {
    throw new AppError(409, "share_invitation_unavailable", "This share invitation has expired or was withdrawn.");
  }
  const friendship = await c.env.DB.prepare(`SELECT id FROM friendships WHERE status='active'
    AND user_low_id=? AND user_high_id=?`).bind(Math.min(share.owner_id, share.recipient_id), Math.max(share.owner_id, share.recipient_id)).first();
  if (!friendship) throw new AppError(403, "friendship_required", "You must still be friends with the owner to accept this share.");
  const profile = await c.env.DB.prepare("SELECT encryption_key_version FROM friend_profiles WHERE user_id=?")
    .bind(share.recipient_id).first<{ encryption_key_version: number }>();
  const prepared = await c.env.DB.prepare(`SELECT id,public_id,key_prefix,key_version,key_envelope,envelope_context,recipient_key_version,status
    FROM share_access_keys WHERE recipient_grant_id=? AND status='prepared'`).bind(share.id).first<{
      id: number; public_id: string; key_prefix: string; key_version: number; key_envelope: string; envelope_context: string; recipient_key_version: number; status: string;
    }>();
  if (!prepared) throw new AppError(409, "key_rotation_required", "The access key is unavailable. Ask the owner to rotate it.");
  if (!profile || profile.encryption_key_version !== prepared.recipient_key_version) {
    throw new AppError(409, "key_rotation_required", "Your encryption key changed. Ask the owner to rotate the access key.");
  }
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_group_recipients SET status='active',accepted_at=?,updated_at=? WHERE id=? AND status='pending'")
      .bind(now, now, share.id),
    c.env.DB.prepare("UPDATE share_access_keys SET status='active',activated_at=? WHERE id=? AND status='prepared'")
      .bind(now, prepared.id),
  ]);
  await writeShareAudit(c, share.group_id, "share_recipient.accept", "share_recipient", share.public_id);
  const accepted = await loadReceived(c, share.public_id);
  return c.json({ share: publicReceived(accepted, publicOrigin(c), {
    public_id: prepared.public_id,
    key_prefix: prepared.key_prefix,
    key_version: prepared.key_version,
    key_envelope: prepared.key_envelope,
    envelope_context: prepared.envelope_context,
    recipient_key_version: prepared.recipient_key_version,
    status: "active",
  }) });
});

received.post("/received-shares/:id/decline", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const share = await loadReceived(c, c.req.param("id"));
  if (share.status === "declined") return c.json({ ok: true });
  if (share.status !== "pending") throw new AppError(409, "share_invitation_unavailable", "This share invitation can no longer be declined.");
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_group_recipients SET status='declined',updated_at=? WHERE id=?").bind(now, share.id),
    c.env.DB.prepare("UPDATE share_access_keys SET status='revoked',revoked_at=? WHERE recipient_grant_id=? AND status='prepared'").bind(now, share.id),
  ]);
  await writeShareAudit(c, share.group_id, "share_recipient.decline", "share_recipient", share.public_id);
  return c.json({ ok: true });
});

received.post("/received-shares/:id/leave", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const share = await loadReceived(c, c.req.param("id"));
  if (share.status === "left") return c.json({ ok: true });
  if (!(["active", "paused"].includes(share.status))) throw new AppError(409, "share_access_unavailable", "This share access can no longer be left.");
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE share_group_recipients SET status='left',updated_at=? WHERE id=?").bind(now, share.id),
    c.env.DB.prepare("UPDATE share_access_keys SET status='revoked',revoked_at=? WHERE recipient_grant_id=? AND status='active'").bind(now, share.id),
  ]);
  await writeShareAudit(c, share.group_id, "share_recipient.leave", "share_recipient", share.public_id);
  return c.json({ ok: true });
});

received.get("/received-shares/:id/key-envelope", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const share = await loadReceived(c, c.req.param("id"));
  if (!(["active", "paused"].includes(share.status))) throw new AppError(409, "share_access_unavailable", "The access key is not available.");
  const key = await c.env.DB.prepare(`SELECT public_id,key_prefix,key_version,key_envelope,envelope_context,recipient_key_version,status
    FROM share_access_keys WHERE recipient_grant_id=? AND status='active'`).bind(share.id).first();
  if (!key) throw new AppError(409, "key_rotation_required", "The access key is unavailable. Ask the owner to rotate it.");
  return c.json({ key });
});

received.get("/received-shares/:id/usage", async (c) => {
  await requireFeature(c, "share_groups_enabled");
  const share = await loadReceived(c, c.req.param("id"));
  const result = await c.env.DB.prepare(`SELECT request_id,route_mode,model,status,error_code,input_tokens,output_tokens,latency_ms,created_at
    FROM share_usage_log_v2 WHERE recipient_grant_id=? ORDER BY created_at DESC,id DESC LIMIT 500`).bind(share.id).all();
  return c.json({ usage: result.results });
});

export default received;
