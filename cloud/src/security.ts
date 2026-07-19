import { AppError } from "./errors";
import type { AuthUser, Bindings, UserRow } from "./types";

const encoder = new TextEncoder();
const decoder = new TextDecoder();

export function bytesToBase64URL(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

export function base64URLToBytes(value: string): Uint8Array {
  if (!/^[A-Za-z0-9_-]+$/.test(value)) throw new Error("invalid base64url");
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - normalized.length % 4) % 4);
  const binary = atob(padded);
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}

export function isBase64URLBytes(value: unknown, length: number): value is string {
  if (typeof value !== "string") return false;
  try {
    return base64URLToBytes(value).length === length;
  } catch {
    return false;
  }
}

export async function sha256(value: string): Promise<string> {
  return bytesToBase64URL(new Uint8Array(await crypto.subtle.digest("SHA-256", encoder.encode(value))));
}

export async function authVerifier(authHash: string): Promise<string> {
  return sha256(`amber-auth-verifier-v1:${authHash}`);
}

export function randomToken(bytes = 32): string {
  return bytesToBase64URL(crypto.getRandomValues(new Uint8Array(bytes)));
}

export function randomVerificationCode(): string {
  const limit = Math.floor(0x1_0000_0000 / 1_000_000) * 1_000_000;
  const sample = new Uint32Array(1);
  do crypto.getRandomValues(sample); while ((sample[0] ?? limit) >= limit);
  return String((sample[0] ?? 0) % 1_000_000).padStart(6, "0");
}

export function safeEqual(left: string, right: string): boolean {
  const a = encoder.encode(left);
  const b = encoder.encode(right);
  let different = a.length ^ b.length;
  const length = Math.max(a.length, b.length);
  for (let index = 0; index < length; index += 1) {
    different |= (a[index % Math.max(a.length, 1)] ?? 0) ^ (b[index % Math.max(b.length, 1)] ?? 0);
  }
  return different === 0;
}

function encodeJSON(value: unknown): string {
  return bytesToBase64URL(encoder.encode(JSON.stringify(value)));
}

async function hmac(secret: string, value: string): Promise<Uint8Array> {
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  return new Uint8Array(await crypto.subtle.sign("HMAC", key, encoder.encode(value)));
}

export async function hmacBase64URL(secret: string, value: string): Promise<string> {
  return bytesToBase64URL(await hmac(secret, value));
}

export async function signAccessToken(env: Bindings, user: AuthUser): Promise<string> {
  if (!env.JWT_SECRET || env.JWT_SECRET.length < 32) {
    throw new AppError(503, "cloud_not_configured", "Cloud authentication is not configured.");
  }
  const now = Math.floor(Date.now() / 1000);
  const header = encodeJSON({ alg: "HS256", typ: "JWT" });
  const payload = encodeJSON({
    sub: String(user.id),
    email: user.email,
    role: user.role,
    sv: user.sessionVersion,
    iat: now,
    exp: now + 15 * 60,
  });
  const signingInput = `${header}.${payload}`;
  return `${signingInput}.${bytesToBase64URL(await hmac(env.JWT_SECRET, signingInput))}`;
}

interface AccessClaims {
  sub: string;
  email: string;
  role: "user" | "admin";
  sv: number;
  exp: number;
}

export async function verifyAccessToken(env: Bindings, token: string): Promise<AccessClaims> {
  const parts = token.split(".");
  if (parts.length !== 3 || !parts[0] || !parts[1] || !parts[2]) {
    throw new AppError(401, "invalid_access_token", "Authentication is required.");
  }
  const expected = bytesToBase64URL(await hmac(env.JWT_SECRET, `${parts[0]}.${parts[1]}`));
  if (!safeEqual(expected, parts[2])) {
    throw new AppError(401, "invalid_access_token", "Authentication is required.");
  }
  try {
    const claims = JSON.parse(decoder.decode(base64URLToBytes(parts[1]))) as AccessClaims;
    if (!claims.sub || !claims.exp || claims.exp <= Math.floor(Date.now() / 1000)) throw new Error("expired");
    return claims;
  } catch {
    throw new AppError(401, "invalid_access_token", "Authentication is required.");
  }
}

interface RefreshSession {
  kind?: "active";
  user_id: number;
  session_version: number;
  created_at: string;
}

interface RefreshRotationReceipt {
  kind: "rotated";
  user_id: number;
  session_version: number;
  created_at: string;
  next_token_hash: string;
}

const refreshRotationGraceSeconds = 60;

export async function issueSession(env: Bindings, user: AuthUser, refreshToken = randomToken()) {
  const tokenHash = await sha256(refreshToken);
  const key = `refresh:${tokenHash}`;
  const userKey = `refresh-user:${user.id}:${tokenHash}`;
  const session: RefreshSession = {
    user_id: user.id,
    session_version: user.sessionVersion,
    created_at: new Date().toISOString(),
  };
  const options = { expirationTtl: 30 * 24 * 60 * 60 };
  await Promise.all([
    env.SESSIONS.put(key, JSON.stringify(session), options),
    env.SESSIONS.put(userKey, key, options),
  ]);
  return {
    access_token: await signAccessToken(env, user),
    access_expires_in: 15 * 60,
    refresh_token: refreshToken,
    refresh_expires_in: 30 * 24 * 60 * 60,
  };
}

async function derivedRefreshToken(env: Bindings, token: string): Promise<string> {
  if (!env.JWT_SECRET || env.JWT_SECRET.length < 32) {
    throw new AppError(503, "cloud_not_configured", "Cloud authentication is not configured.");
  }
  return bytesToBase64URL(await hmac(env.JWT_SECRET, `amber-refresh-rotation-v1:${token}`));
}

export async function beginRefreshSession(env: Bindings, token: string): Promise<{
  session: RefreshSession;
  nextRefreshToken: string;
  replay: boolean;
}> {
  if (!token || token.length > 512) throw new AppError(401, "invalid_refresh_token", "The session has expired.");
  const key = `refresh:${await sha256(token)}`;
  const state = await env.SESSIONS.get<RefreshSession | RefreshRotationReceipt>(key, "json");
  if (!state) throw new AppError(401, "invalid_refresh_token", "The session has expired.");
  const nextRefreshToken = await derivedRefreshToken(env, token);
  const nextTokenHash = await sha256(nextRefreshToken);
  if (state.kind === "rotated") {
    if (state.next_token_hash !== nextTokenHash) {
      throw new AppError(401, "invalid_refresh_token", "The session has expired.");
    }
    const nextSession = await env.SESSIONS.get<RefreshSession | RefreshRotationReceipt>(`refresh:${nextTokenHash}`, "json");
    if (!nextSession || nextSession.kind === "rotated") {
      throw new AppError(401, "invalid_refresh_token", "The session has expired.");
    }
    return {
      session: { user_id: state.user_id, session_version: state.session_version, created_at: state.created_at },
      nextRefreshToken,
      replay: true,
    };
  }
  return { session: state, nextRefreshToken, replay: false };
}

export async function completeRefreshSession(
  env: Bindings,
  token: string,
  session: RefreshSession,
  nextRefreshToken: string,
  replay: boolean,
): Promise<void> {
  if (replay) return;
  const tokenHash = await sha256(token);
  const nextTokenHash = await sha256(nextRefreshToken);
  const receipt: RefreshRotationReceipt = {
    kind: "rotated",
    user_id: session.user_id,
    session_version: session.session_version,
    created_at: session.created_at,
    next_token_hash: nextTokenHash,
  };
  await Promise.all([
    env.SESSIONS.put(`refresh:${tokenHash}`, JSON.stringify(receipt), { expirationTtl: refreshRotationGraceSeconds }),
    env.SESSIONS.delete(`refresh-user:${session.user_id}:${tokenHash}`),
  ]);
}

export async function deleteRefreshSession(env: Bindings, token: string): Promise<void> {
  if (!token || token.length > 512) return;
  const tokenHash = await sha256(token);
  const key = `refresh:${tokenHash}`;
  const session = await env.SESSIONS.get<RefreshSession | RefreshRotationReceipt>(key, "json");
  const nextTokenHash = session?.kind === "rotated" ? session.next_token_hash : "";
  await Promise.all([
    env.SESSIONS.delete(key),
    ...(session ? [env.SESSIONS.delete(`refresh-user:${session.user_id}:${tokenHash}`)] : []),
    ...(session && nextTokenHash ? [
      env.SESSIONS.delete(`refresh:${nextTokenHash}`),
      env.SESSIONS.delete(`refresh-user:${session.user_id}:${nextTokenHash}`),
    ] : []),
  ]);
}

export async function revokeUserSessions(env: Bindings, userID: number): Promise<void> {
  let cursor: string | undefined;
  do {
    const page = await env.SESSIONS.list({ prefix: `refresh-user:${userID}:`, cursor });
    await Promise.all(page.keys.flatMap(({ name }) => {
      const tokenHash = name.slice(name.lastIndexOf(":") + 1);
      return [env.SESSIONS.delete(name), env.SESSIONS.delete(`refresh:${tokenHash}`)];
    }));
    cursor = page.list_complete ? undefined : page.cursor;
  } while (cursor);
}

export function authUserFromRow(row: UserRow): AuthUser {
  return { id: row.id, email: row.email, role: row.role, sessionVersion: row.session_version };
}

export async function fakeLoginParameters(secret: string, email: string) {
  const kdf = (await hmac(secret, `fake-kdf:${email}`)).slice(0, 16);
  const auth = (await hmac(secret, `fake-auth:${email}`)).slice(0, 16);
  return {
    salt_kdf: bytesToBase64URL(kdf),
    salt_auth: bytesToBase64URL(auth),
  };
}
