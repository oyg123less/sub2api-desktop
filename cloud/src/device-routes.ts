import { Hono } from "hono";
import { requireAuth } from "./auth-middleware";
import { cleanText, newPublicID, parsePublicID, requireFeature, validateDevicePublicKey } from "./cloud-v2";
import { AppError, readJSON } from "./errors";
import { base64URLToBytes, bytesToBase64URL, isBase64URLBytes, randomToken, sha256 } from "./security";
import type { AppEnv } from "./types";

const devices = new Hono<AppEnv>();
const encoder = new TextEncoder();
const allowedCapabilities = new Set(["oauth", "api_key", "proxy", "streaming"]);

function owned(bytes: Uint8Array): Uint8Array<ArrayBuffer> {
  return new Uint8Array(bytes);
}

devices.use("/devices", requireAuth);
devices.use("/devices/*", requireAuth);
devices.use("/relay/connect", requireAuth);

function capabilities(value: unknown): string[] {
  if (!Array.isArray(value) || value.length < 1 || value.length > allowedCapabilities.size) {
    throw new AppError(400, "invalid_device_capabilities", "The device capabilities are invalid.");
  }
  const result = [...new Set(value.map((item) => typeof item === "string" ? item : ""))];
  if (result.some((item) => !allowedCapabilities.has(item))) {
    throw new AppError(400, "invalid_device_capabilities", "The device capabilities are invalid.");
  }
  return result.sort();
}

devices.get("/devices", async (c) => {
  await requireFeature(c, "owner_relay_enabled");
  const userID = c.get("auth").id;
  const result = await c.env.DB.prepare(`SELECT public_id,name,capabilities,is_primary,revoked,last_seen_at,created_at,updated_at
    FROM share_devices WHERE user_id=? ORDER BY revoked,is_primary DESC,updated_at DESC`).bind(userID).all<{
      public_id: string; name: string; capabilities: string; is_primary: number; revoked: number;
      last_seen_at: string | null; created_at: string; updated_at: string;
    }>();
  let online = new Map<string, Record<string, unknown>>();
  try {
    const stub = c.env.OWNER_RELAY.get(c.env.OWNER_RELAY.idFromName(`owner:${userID}`));
    const response = await stub.fetch("https://relay.internal/status");
    const status = await response.json<{ devices: Array<Record<string, unknown> & { public_id: string }> }>();
    online = new Map(status.devices.map((device) => [device.public_id, device]));
  } catch {
    // A missing relay instance means every registered device is offline.
  }
  return c.json({ devices: result.results.map((device) => ({
    ...device,
    capabilities: JSON.parse(device.capabilities) as string[],
    is_primary: Boolean(device.is_primary),
    revoked: Boolean(device.revoked),
    online: online.has(device.public_id),
    relay: online.get(device.public_id) || null,
  })) });
});

devices.post("/devices", async (c) => {
  await requireFeature(c, "owner_relay_enabled");
  const body = await readJSON<Record<string, unknown>>(c);
  const name = cleanText(body.name, 2, 60, "invalid_device_name", "The device name must contain 2 to 60 characters.");
  const publicKey = validateDevicePublicKey(body.device_public_key);
  const deviceCapabilities = capabilities(body.capabilities);
  const userID = c.get("auth").id;
  const existing = await c.env.DB.prepare("SELECT public_id,revoked FROM share_devices WHERE user_id=? AND device_public_key=?")
    .bind(userID, publicKey).first<{ public_id: string; revoked: number }>();
  if (existing && !existing.revoked) throw new AppError(409, "device_exists", "This device is already registered.");
  const count = await c.env.DB.prepare("SELECT COUNT(*) AS value FROM share_devices WHERE user_id=? AND revoked=0")
    .bind(userID).first<{ value: number }>();
  if ((count?.value ?? 0) >= 10) throw new AppError(409, "device_limit_reached", "Remove an old device before adding another one.");
  const publicID = existing?.public_id || newPublicID("dev");
  const primary = (count?.value ?? 0) === 0;
  const now = new Date().toISOString();
  if (existing) {
    await c.env.DB.prepare(`UPDATE share_devices SET name=?,capabilities=?,is_primary=?,revoked=0,updated_at=?
      WHERE user_id=? AND public_id=?`).bind(name, JSON.stringify(deviceCapabilities), primary ? 1 : 0, now, userID, publicID).run();
  } else {
    await c.env.DB.prepare(`INSERT INTO share_devices
      (public_id,user_id,name,device_public_key,capabilities,is_primary,created_at,updated_at)
      VALUES(?,?,?,?,?,?,?,?)`).bind(publicID, userID, name, publicKey, JSON.stringify(deviceCapabilities), primary ? 1 : 0, now, now).run();
  }
  return c.json({ device: { public_id: publicID, name, capabilities: deviceCapabilities, is_primary: primary, revoked: false, online: false } }, 201);
});

devices.patch("/devices/:id", async (c) => {
  await requireFeature(c, "owner_relay_enabled");
  const publicID = parsePublicID(c.req.param("id"), "invalid_device_id");
  const body = await readJSON<Record<string, unknown>>(c);
  const userID = c.get("auth").id;
  const current = await c.env.DB.prepare("SELECT id,revoked FROM share_devices WHERE public_id=? AND user_id=?")
    .bind(publicID, userID).first<{ id: number; revoked: number }>();
  if (!current || current.revoked) throw new AppError(404, "device_not_found", "The device was not found.");
  const statements: D1PreparedStatement[] = [];
  const updates: string[] = [];
  const values: unknown[] = [];
  if (body.name !== undefined) { updates.push("name=?"); values.push(cleanText(body.name, 2, 60, "invalid_device_name", "The device name must contain 2 to 60 characters.")); }
  if (body.is_primary === true) {
    statements.push(c.env.DB.prepare("UPDATE share_devices SET is_primary=0,updated_at=? WHERE user_id=? AND revoked=0")
      .bind(new Date().toISOString(), userID));
    updates.push("is_primary=1");
  }
  if (!updates.length) throw new AppError(400, "invalid_device_update", "No supported device changes were provided.");
  updates.push("updated_at=?"); values.push(new Date().toISOString(), current.id);
  statements.push(c.env.DB.prepare(`UPDATE share_devices SET ${updates.join(",")} WHERE id=?`).bind(...values));
  await c.env.DB.batch(statements);
  return c.json({ ok: true });
});

devices.delete("/devices/:id", async (c) => {
  await requireFeature(c, "owner_relay_enabled");
  const publicID = parsePublicID(c.req.param("id"), "invalid_device_id");
  const userID = c.get("auth").id;
  const row = await c.env.DB.prepare("SELECT id FROM share_devices WHERE public_id=? AND user_id=? AND revoked=0")
    .bind(publicID, userID).first<{ id: number }>();
  if (!row) throw new AppError(404, "device_not_found", "The device was not found.");
  const now = new Date().toISOString();
  await c.env.DB.prepare("UPDATE share_devices SET revoked=1,is_primary=0,updated_at=? WHERE id=?").bind(now, row.id).run();
  const stub = c.env.OWNER_RELAY.get(c.env.OWNER_RELAY.idFromName(`owner:${userID}`));
  c.executionCtx.waitUntil(stub.fetch("https://relay.internal/disconnect", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ device_id: publicID }),
  }));
  return c.json({ ok: true });
});

devices.post("/devices/:id/challenge", async (c) => {
  await requireFeature(c, "owner_relay_enabled");
  const publicID = parsePublicID(c.req.param("id"), "invalid_device_id");
  const row = await c.env.DB.prepare("SELECT id FROM share_devices WHERE public_id=? AND user_id=? AND revoked=0")
    .bind(publicID, c.get("auth").id).first<{ id: number }>();
  if (!row) throw new AppError(404, "device_not_found", "The device was not found.");
  const challenge = randomToken(32);
  const now = new Date();
  const expiresAt = new Date(now.getTime() + 60_000).toISOString();
  await c.env.DB.prepare(`INSERT INTO share_device_challenges(challenge_hash,device_id,expires_at,created_at)
    VALUES(?,?,?,?)`).bind(await sha256(challenge), row.id, expiresAt, now.toISOString()).run();
  c.executionCtx.waitUntil(c.env.DB.prepare("DELETE FROM share_device_challenges WHERE expires_at<=? OR consumed_at IS NOT NULL")
    .bind(new Date(now.getTime() - 5 * 60_000).toISOString()).run());
  return c.json({ challenge, expires_at: expiresAt, signature_context: "amber-relay-v1" });
});

devices.get("/relay/connect", async (c) => {
  await requireFeature(c, "owner_relay_enabled");
  if ((c.req.header("upgrade") || "").toLowerCase() !== "websocket") throw new AppError(426, "websocket_required", "A WebSocket upgrade is required.");
  if (c.req.query("protocol") !== "1") throw new AppError(426, "relay_protocol_mismatch", "Upgrade Amber to a compatible relay protocol.");
  const deviceID = parsePublicID(c.req.query("device_id") || "", "invalid_device_id");
  const challenge = c.req.header("x-amber-device-challenge") || "";
  const expiresAt = c.req.header("x-amber-device-challenge-expires") || "";
  const signature = c.req.header("x-amber-device-proof") || "";
  if (!isBase64URLBytes(challenge, 32) || !isBase64URLBytes(signature, 64) || !expiresAt || Date.parse(expiresAt) <= Date.now()) {
    throw new AppError(401, "invalid_device_proof", "The device proof is invalid or expired.");
  }
  const device = await c.env.DB.prepare(`SELECT id,device_public_key,capabilities,is_primary FROM share_devices
    WHERE public_id=? AND user_id=? AND revoked=0`).bind(deviceID, c.get("auth").id).first<{
      id: number; device_public_key: string; capabilities: string; is_primary: number;
    }>();
  if (!device) throw new AppError(401, "invalid_device_proof", "The device proof is invalid or expired.");
  const challengeHash = await sha256(challenge);
  const stored = await c.env.DB.prepare(`SELECT expires_at FROM share_device_challenges
    WHERE challenge_hash=? AND device_id=? AND consumed_at IS NULL`).bind(challengeHash, device.id).first<{ expires_at: string }>();
  if (!stored || stored.expires_at !== expiresAt || Date.parse(stored.expires_at) <= Date.now()) {
    throw new AppError(401, "invalid_device_proof", "The device proof is invalid or expired.");
  }
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(`amber-relay-v1|${deviceID}|${challenge}|${expiresAt}`)));
  let verified = false;
  try {
    const key = await crypto.subtle.importKey("raw", owned(base64URLToBytes(device.device_public_key)), { name: "Ed25519" }, false, ["verify"]);
    verified = await crypto.subtle.verify("Ed25519", key, owned(base64URLToBytes(signature)), owned(digest));
  } catch { verified = false; }
  if (!verified) throw new AppError(401, "invalid_device_proof", "The device proof is invalid or expired.");
  const consumed = await c.env.DB.prepare(`UPDATE share_device_challenges SET consumed_at=?
    WHERE challenge_hash=? AND device_id=? AND consumed_at IS NULL AND expires_at>?`)
    .bind(new Date().toISOString(), challengeHash, device.id, new Date().toISOString()).run();
  if (consumed.meta.changes !== 1) throw new AppError(401, "invalid_device_proof", "The device proof is invalid or expired.");
  const sessionID = newPublicID("rly");
  const now = new Date().toISOString();
  await c.env.DB.batch([
    c.env.DB.prepare(`INSERT INTO share_device_sessions(id,device_id,connected_at,last_heartbeat_at)
      VALUES(?,?,?,?)`).bind(sessionID, device.id, now, now),
    c.env.DB.prepare("UPDATE share_devices SET last_seen_at=?,updated_at=? WHERE id=?").bind(now, now, device.id),
  ]);
  const headers = new Headers(c.req.raw.headers);
  headers.set("X-Amber-Device-ID", deviceID);
  headers.set("X-Amber-Relay-Session", sessionID);
  headers.set("X-Amber-Device-Primary", device.is_primary ? "1" : "0");
  headers.set("X-Amber-Device-Capabilities", (JSON.parse(device.capabilities) as string[]).join(","));
  headers.delete("Authorization");
  headers.delete("X-Amber-Device-Proof");
  const stub = c.env.OWNER_RELAY.get(c.env.OWNER_RELAY.idFromName(`owner:${c.get("auth").id}`));
  return stub.fetch(new Request("https://relay.internal/connect", { headers }));
});

export default devices;
