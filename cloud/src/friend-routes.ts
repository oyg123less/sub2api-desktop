import { Hono } from "hono";
import { requireAuth } from "./auth-middleware";
import { cleanText, friendPair, newPublicID, parsePublicID, requireFeature, validateEncryptionPublicKey, writeShareAudit } from "./cloud-v2";
import { AppError, readJSON, requireJSONSize } from "./errors";
import type { AppEnv } from "./types";

const social = new Hono<AppEnv>();
const friendCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";

interface ProfileRow {
  user_id: number;
  display_name: string;
  friend_code: string;
  encryption_public_key: string;
  encryption_private_cipher: string;
  encryption_key_version: number;
  created_at: string;
  updated_at: string;
}

interface FriendshipRow {
  id: number;
  public_id: string;
  user_low_id: number;
  user_high_id: number;
  status: string;
}

interface FriendRequestRow {
  id: number;
  public_id: string;
  sender_id: number;
  receiver_id: number;
  status: string;
  created_at: string;
  responded_at: string | null;
  expires_at: string;
}

social.use("/profile", requireAuth);
social.use("/profile/*", requireAuth);
social.use("/friends", requireAuth);
social.use("/friends/*", requireAuth);
social.use("/friend-requests", requireAuth);
social.use("/friend-requests/*", requireAuth);

function publicProfile(row: ProfileRow) {
  return {
    display_name: row.display_name,
    friend_code: row.friend_code,
    encryption_public_key: row.encryption_public_key,
    encryption_private_cipher: row.encryption_private_cipher,
    encryption_key_version: row.encryption_key_version,
    created_at: row.created_at,
    updated_at: row.updated_at,
  };
}

function formatFriendCode(raw: string): string {
  return `AMB-${raw.slice(0, 4)}-${raw.slice(4, 8)}`;
}

async function uniqueFriendCode(c: Parameters<typeof requireFeature>[0]): Promise<string> {
  for (let attempt = 0; attempt < 8; attempt += 1) {
    const bytes = crypto.getRandomValues(new Uint8Array(8));
    const raw = Array.from(bytes, (byte) => friendCodeAlphabet[byte % friendCodeAlphabet.length]).join("");
    const code = formatFriendCode(raw);
    const exists = await c.env.DB.prepare("SELECT user_id FROM friend_profiles WHERE friend_code=?").bind(code).first();
    if (!exists) return code;
  }
  throw new AppError(503, "friend_code_unavailable", "A Friend Code could not be generated.");
}

async function currentProfile(c: Parameters<typeof requireFeature>[0]): Promise<ProfileRow | null> {
  return c.env.DB.prepare(`SELECT user_id,display_name,friend_code,encryption_public_key,encryption_private_cipher,encryption_key_version,created_at,updated_at
    FROM friend_profiles WHERE user_id=?`).bind(c.get("auth").id).first<ProfileRow>();
}

social.get("/profile", async (c) => {
  await requireFeature(c, "friends_enabled");
  const profile = await currentProfile(c);
  return c.json({ profile: profile ? publicProfile(profile) : null, needs_setup: !profile });
});

social.put("/profile", async (c) => {
  await requireFeature(c, "friends_enabled");
  requireJSONSize(c, 16 * 1024);
  const body = await readJSON<Record<string, unknown>>(c);
  const displayName = cleanText(body.display_name, 2, 40, "invalid_display_name", "The display name must contain 2 to 40 characters.");
  const publicKey = validateEncryptionPublicKey(body.encryption_public_key);
  const privateCipher = typeof body.encryption_private_cipher === "string" ? body.encryption_private_cipher.trim() : "";
  if (!privateCipher.startsWith("v1.") || privateCipher.length < 80 || privateCipher.length > 4096) {
    throw new AppError(400, "invalid_encryption_private_cipher", "The encrypted account identity key is invalid.");
  }
  const existing = await currentProfile(c);
  const now = new Date().toISOString();
  if (existing) {
    if (existing.encryption_public_key !== publicKey || existing.encryption_private_cipher !== privateCipher) {
      throw new AppError(409, "identity_key_rotation_required", "Use the identity-key rotation flow to replace this encryption key.");
    }
    await c.env.DB.prepare("UPDATE friend_profiles SET display_name=?,updated_at=? WHERE user_id=?")
      .bind(displayName, now, c.get("auth").id).run();
  } else {
    await c.env.DB.prepare(`INSERT INTO friend_profiles
      (user_id,display_name,friend_code,encryption_public_key,encryption_private_cipher,encryption_key_version,created_at,updated_at)
      VALUES(?,?,?,?,?,1,?,?)`).bind(c.get("auth").id, displayName, await uniqueFriendCode(c), publicKey, privateCipher, now, now).run();
  }
  return c.json({ profile: publicProfile((await currentProfile(c))!) });
});

social.patch("/profile", async (c) => {
  await requireFeature(c, "friends_enabled");
  const body = await readJSON<Record<string, unknown>>(c);
  const displayName = cleanText(body.display_name, 2, 40, "invalid_display_name", "The display name must contain 2 to 40 characters.");
  const result = await c.env.DB.prepare("UPDATE friend_profiles SET display_name=?,updated_at=? WHERE user_id=?")
    .bind(displayName, new Date().toISOString(), c.get("auth").id).run();
  if (!result.meta.changes) throw new AppError(409, "profile_setup_required", "Set up your cloud profile first.");
  return c.json({ profile: publicProfile((await currentProfile(c))!) });
});

social.post("/profile/friend-code/rotate", async (c) => {
  await requireFeature(c, "friends_enabled");
  const profile = await currentProfile(c);
  if (!profile) throw new AppError(409, "profile_setup_required", "Set up your cloud profile first.");
  const friendCode = await uniqueFriendCode(c);
  await c.env.DB.prepare("UPDATE friend_profiles SET friend_code=?,updated_at=? WHERE user_id=?")
    .bind(friendCode, new Date().toISOString(), c.get("auth").id).run();
  return c.json({ friend_code: friendCode });
});

social.get("/friends", async (c) => {
  await requireFeature(c, "friends_enabled");
  const userID = c.get("auth").id;
  const result = await c.env.DB.prepare(`SELECT f.public_id,
      CASE WHEN f.user_low_id=? THEN high.display_name ELSE low.display_name END AS display_name,
      CASE WHEN f.user_low_id=? THEN high.friend_code ELSE low.friend_code END AS friend_code,
      CASE WHEN f.user_low_id=? THEN high.encryption_public_key ELSE low.encryption_public_key END AS encryption_public_key,
      CASE WHEN f.user_low_id=? THEN high.encryption_key_version ELSE low.encryption_key_version END AS encryption_key_version,
      COALESCE(a.alias,'') AS alias,f.created_at,f.updated_at
    FROM friendships f
    JOIN friend_profiles low ON low.user_id=f.user_low_id
    JOIN friend_profiles high ON high.user_id=f.user_high_id
    LEFT JOIN friendship_aliases a ON a.friendship_id=f.id AND a.owner_user_id=?
    WHERE f.status='active' AND (f.user_low_id=? OR f.user_high_id=?)
    ORDER BY COALESCE(NULLIF(a.alias,''),CASE WHEN f.user_low_id=? THEN high.display_name ELSE low.display_name END) COLLATE NOCASE`)
    .bind(userID, userID, userID, userID, userID, userID, userID, userID).all();
  return c.json({ friends: result.results });
});

social.get("/friend-requests", async (c) => {
  await requireFeature(c, "friends_enabled");
  const now = new Date().toISOString();
  await c.env.DB.prepare("UPDATE friend_requests SET status='expired',responded_at=? WHERE status='pending' AND expires_at<=?")
    .bind(now, now).run();
  const userID = c.get("auth").id;
  const result = await c.env.DB.prepare(`SELECT r.public_id,r.status,r.created_at,r.responded_at,r.expires_at,
      CASE WHEN r.sender_id=? THEN 'outgoing' ELSE 'incoming' END AS direction,
      CASE WHEN r.sender_id=? THEN receiver.display_name ELSE sender.display_name END AS display_name,
      CASE WHEN r.sender_id=? THEN receiver.friend_code ELSE sender.friend_code END AS friend_code
    FROM friend_requests r
    JOIN friend_profiles sender ON sender.user_id=r.sender_id
    JOIN friend_profiles receiver ON receiver.user_id=r.receiver_id
    WHERE r.sender_id=? OR r.receiver_id=? ORDER BY r.created_at DESC,r.id DESC LIMIT 200`)
    .bind(userID, userID, userID, userID, userID).all();
  return c.json({ requests: result.results });
});

social.post("/friend-requests", async (c) => {
  await requireFeature(c, "friends_enabled");
  const body = await readJSON<Record<string, unknown>>(c);
  const code = typeof body.friend_code === "string" ? body.friend_code.trim().toUpperCase() : "";
  if (!/^AMB-[A-Z2-9]{4}-[A-Z2-9]{4}$/.test(code)) {
    throw new AppError(404, "friend_code_not_found", "No account was found for that complete Friend Code.");
  }
  const senderID = c.get("auth").id;
  if (!await currentProfile(c)) throw new AppError(409, "profile_setup_required", "Set up your cloud profile first.");
  const target = await c.env.DB.prepare("SELECT user_id FROM friend_profiles WHERE friend_code=?")
    .bind(code).first<{ user_id: number }>();
  if (!target || target.user_id === senderID) {
    throw new AppError(404, "friend_code_not_found", "No account was found for that complete Friend Code.");
  }
  const pair = friendPair(senderID, target.user_id);
  const blocked = await c.env.DB.prepare(`SELECT 1 AS found FROM friend_blocks
    WHERE (blocker_id=? AND blocked_id=?) OR (blocker_id=? AND blocked_id=?) LIMIT 1`)
    .bind(senderID, target.user_id, target.user_id, senderID).first();
  if (blocked) throw new AppError(404, "friend_code_not_found", "No account was found for that complete Friend Code.");
  const friendship = await c.env.DB.prepare(`SELECT public_id FROM friendships
    WHERE user_low_id=? AND user_high_id=? AND status='active'`).bind(pair.low, pair.high).first();
  if (friendship) throw new AppError(409, "friend_request_exists", "You are already friends with this account.");
  const now = new Date();
  const requestID = newPublicID("frq");
  try {
    await c.env.DB.prepare(`INSERT INTO friend_requests
      (public_id,sender_id,receiver_id,pair_key,status,created_at,expires_at)
      VALUES(?,?,?,?,'pending',?,?)`).bind(
        requestID, senderID, target.user_id, pair.key, now.toISOString(),
        new Date(now.getTime() + 30 * 24 * 60 * 60 * 1000).toISOString(),
      ).run();
  } catch {
    throw new AppError(409, "friend_request_exists", "A friend request is already pending.");
  }
  await writeShareAudit(c, null, "friend.request", "friend_request", requestID);
  return c.json({ request: { public_id: requestID, status: "pending", friend_code: code } }, 201);
});

async function loadRequest(c: Parameters<typeof requireFeature>[0], publicID: string): Promise<FriendRequestRow> {
  const row = await c.env.DB.prepare(`SELECT id,public_id,sender_id,receiver_id,status,created_at,responded_at,expires_at
    FROM friend_requests WHERE public_id=?`).bind(parsePublicID(publicID, "invalid_friend_request_id")).first<FriendRequestRow>();
  if (!row) throw new AppError(404, "friend_request_not_found", "The friend request was not found.");
  return row;
}

social.post("/friend-requests/:id/accept", async (c) => {
  await requireFeature(c, "friends_enabled");
  const request = await loadRequest(c, c.req.param("id"));
  const userID = c.get("auth").id;
  if (request.receiver_id !== userID) throw new AppError(404, "friend_request_not_found", "The friend request was not found.");
  const pair = friendPair(request.sender_id, request.receiver_id);
  const existing = await c.env.DB.prepare(`SELECT id,public_id,user_low_id,user_high_id,status FROM friendships
    WHERE user_low_id=? AND user_high_id=?`).bind(pair.low, pair.high).first<FriendshipRow>();
  if (request.status === "accepted" && existing?.status === "active") return c.json({ friendship: { public_id: existing.public_id, status: "active" } });
  if (request.status !== "pending" || Date.parse(request.expires_at) <= Date.now()) {
    throw new AppError(409, "friend_action_unavailable", "This friend request can no longer be accepted.");
  }
  const blocked = await c.env.DB.prepare(`SELECT 1 AS found FROM friend_blocks
    WHERE (blocker_id=? AND blocked_id=?) OR (blocker_id=? AND blocked_id=?) LIMIT 1`)
    .bind(pair.low, pair.high, pair.high, pair.low).first();
  if (blocked) throw new AppError(409, "friend_action_unavailable", "This friend request can no longer be accepted.");
  const now = new Date().toISOString();
  const friendshipID = existing?.public_id || newPublicID("fri");
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE friend_requests SET status='accepted',responded_at=? WHERE id=? AND status='pending'").bind(now, request.id),
    c.env.DB.prepare(`INSERT INTO friendships(public_id,user_low_id,user_high_id,status,created_at,updated_at)
      VALUES(?,?,?,'active',?,?) ON CONFLICT(user_low_id,user_high_id) DO UPDATE SET status='active',updated_at=excluded.updated_at`)
      .bind(friendshipID, pair.low, pair.high, now, now),
  ]);
  await writeShareAudit(c, null, "friend.accept", "friendship", friendshipID);
  return c.json({ friendship: { public_id: friendshipID, status: "active" } });
});

social.post("/friend-requests/:id/decline", async (c) => {
  await requireFeature(c, "friends_enabled");
  const request = await loadRequest(c, c.req.param("id"));
  if (request.receiver_id !== c.get("auth").id) throw new AppError(404, "friend_request_not_found", "The friend request was not found.");
  if (request.status === "declined") return c.json({ ok: true });
  const result = await c.env.DB.prepare("UPDATE friend_requests SET status='declined',responded_at=? WHERE id=? AND status='pending'")
    .bind(new Date().toISOString(), request.id).run();
  if (!result.meta.changes) throw new AppError(409, "friend_action_unavailable", "This friend request can no longer be declined.");
  await writeShareAudit(c, null, "friend.decline", "friend_request", request.public_id);
  return c.json({ ok: true });
});

social.post("/friend-requests/:id/cancel", async (c) => {
  await requireFeature(c, "friends_enabled");
  const request = await loadRequest(c, c.req.param("id"));
  if (request.sender_id !== c.get("auth").id) throw new AppError(404, "friend_request_not_found", "The friend request was not found.");
  if (request.status === "cancelled") return c.json({ ok: true });
  const result = await c.env.DB.prepare("UPDATE friend_requests SET status='cancelled',responded_at=? WHERE id=? AND status='pending'")
    .bind(new Date().toISOString(), request.id).run();
  if (!result.meta.changes) throw new AppError(409, "friend_action_unavailable", "This friend request can no longer be cancelled.");
  await writeShareAudit(c, null, "friend.cancel", "friend_request", request.public_id);
  return c.json({ ok: true });
});

async function loadFriendship(c: Parameters<typeof requireFeature>[0], publicID: string): Promise<{ row: FriendshipRow; otherID: number }> {
  const row = await c.env.DB.prepare(`SELECT id,public_id,user_low_id,user_high_id,status FROM friendships WHERE public_id=?`)
    .bind(parsePublicID(publicID, "invalid_friendship_id")).first<FriendshipRow>();
  const userID = c.get("auth").id;
  if (!row || row.status !== "active" || (row.user_low_id !== userID && row.user_high_id !== userID)) {
    throw new AppError(404, "friend_not_found", "The friend was not found.");
  }
  return { row, otherID: row.user_low_id === userID ? row.user_high_id : row.user_low_id };
}

social.patch("/friends/:id", async (c) => {
  await requireFeature(c, "friends_enabled");
  const { row } = await loadFriendship(c, c.req.param("id"));
  const body = await readJSON<Record<string, unknown>>(c);
  const alias = body.alias === "" ? "" : cleanText(body.alias, 1, 40, "invalid_friend_alias", "The friend alias must contain at most 40 characters.");
  if (!alias) {
    await c.env.DB.prepare("DELETE FROM friendship_aliases WHERE friendship_id=? AND owner_user_id=?").bind(row.id, c.get("auth").id).run();
  } else {
    await c.env.DB.prepare(`INSERT INTO friendship_aliases(friendship_id,owner_user_id,alias,updated_at) VALUES(?,?,?,?)
      ON CONFLICT(friendship_id,owner_user_id) DO UPDATE SET alias=excluded.alias,updated_at=excluded.updated_at`)
      .bind(row.id, c.get("auth").id, alias, new Date().toISOString()).run();
  }
  return c.json({ ok: true });
});

async function terminateSharedAccess(c: Parameters<typeof requireFeature>[0], otherID: number, revoke: boolean): Promise<void> {
  const userID = c.get("auth").id;
  const terminalStatus = revoke ? "revoked" : "paused";
  const affected = `SELECT r.id FROM share_group_recipients r JOIN share_groups g ON g.id=r.group_id
    WHERE ((g.owner_id=${userID} AND r.recipient_id=${otherID}) OR (g.owner_id=${otherID} AND r.recipient_id=${userID}))
      AND r.status IN ('pending','active','paused')`;
  const now = new Date().toISOString();
  const statements = [c.env.DB.prepare(`UPDATE share_group_recipients SET status=?,updated_at=? WHERE id IN (${affected})`)
    .bind(terminalStatus, now)];
  if (revoke) {
    statements.unshift(c.env.DB.prepare(`UPDATE share_access_keys SET status='revoked',revoked_at=?
      WHERE recipient_grant_id IN (${affected}) AND status IN ('prepared','active')`).bind(now));
  }
  await c.env.DB.batch(statements);
}

social.delete("/friends/:id", async (c) => {
  await requireFeature(c, "friends_enabled");
  const { row, otherID } = await loadFriendship(c, c.req.param("id"));
  const revoke = c.req.query("mode") === "revoke";
  await terminateSharedAccess(c, otherID, revoke);
  await c.env.DB.prepare("UPDATE friendships SET status='removed',updated_at=? WHERE id=?")
    .bind(new Date().toISOString(), row.id).run();
  await writeShareAudit(c, null, revoke ? "friend.remove_revoke" : "friend.remove_pause", "friendship", row.public_id);
  return c.json({ ok: true, shared_access: revoke ? "revoked" : "paused" });
});

social.post("/friends/:id/block", async (c) => {
  await requireFeature(c, "friends_enabled");
  const { row, otherID } = await loadFriendship(c, c.req.param("id"));
  await terminateSharedAccess(c, otherID, true);
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare("UPDATE friendships SET status='removed',updated_at=? WHERE id=?").bind(now, row.id),
    c.env.DB.prepare("INSERT INTO friend_blocks(blocker_id,blocked_id,created_at) VALUES(?,?,?) ON CONFLICT DO NOTHING")
      .bind(c.get("auth").id, otherID, now),
  ]);
  await writeShareAudit(c, null, "friend.block", "friendship", row.public_id);
  return c.json({ ok: true });
});

social.delete("/friends/:id/block", async (c) => {
  await requireFeature(c, "friends_enabled");
  const publicID = parsePublicID(c.req.param("id"), "invalid_friendship_id");
  const row = await c.env.DB.prepare(`SELECT user_low_id,user_high_id FROM friendships WHERE public_id=?`)
    .bind(publicID).first<{ user_low_id: number; user_high_id: number }>();
  const userID = c.get("auth").id;
  if (!row || (row.user_low_id !== userID && row.user_high_id !== userID)) throw new AppError(404, "friend_not_found", "The friend was not found.");
  const otherID = row.user_low_id === userID ? row.user_high_id : row.user_low_id;
  await c.env.DB.prepare("DELETE FROM friend_blocks WHERE blocker_id=? AND blocked_id=?").bind(userID, otherID).run();
  return c.json({ ok: true });
});

export default social;
