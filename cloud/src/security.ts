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
  user_id: number;
  session_version: number;
  created_at: string;
}

export async function issueSession(env: Bindings, user: AuthUser) {
  const refreshToken = randomToken();
  const key = `refresh:${await sha256(refreshToken)}`;
  const session: RefreshSession = {
    user_id: user.id,
    session_version: user.sessionVersion,
    created_at: new Date().toISOString(),
  };
  await env.SESSIONS.put(key, JSON.stringify(session), { expirationTtl: 30 * 24 * 60 * 60 });
  return {
    access_token: await signAccessToken(env, user),
    access_expires_in: 15 * 60,
    refresh_token: refreshToken,
    refresh_expires_in: 30 * 24 * 60 * 60,
  };
}

export async function consumeRefreshSession(env: Bindings, token: string): Promise<RefreshSession> {
  if (!token || token.length > 512) throw new AppError(401, "invalid_refresh_token", "The session has expired.");
  const key = `refresh:${await sha256(token)}`;
  const session = await env.SESSIONS.get<RefreshSession>(key, "json");
  if (!session) throw new AppError(401, "invalid_refresh_token", "The session has expired.");
  await env.SESSIONS.delete(key);
  return session;
}

export async function deleteRefreshSession(env: Bindings, token: string): Promise<void> {
  if (!token || token.length > 512) return;
  await env.SESSIONS.delete(`refresh:${await sha256(token)}`);
}

export function authUserFromRow(row: UserRow): AuthUser {
  return { id: row.id, email: row.email, role: row.role, sessionVersion: row.session_version };
}

export async function fakeLoginParameters(secret: string, email: string) {
  const kdf = (await hmac(secret, `fake-kdf:${email}`)).slice(0, 16);
  const auth = (await hmac(secret, `fake-auth:${email}`)).slice(0, 16);
  const wrappedA = await hmac(secret, `fake-vault-a:${email}`);
  const wrappedB = await hmac(secret, `fake-vault-b:${email}`);
  const wrapped = new Uint8Array(60);
  wrapped.set(wrappedA);
  wrapped.set(wrappedB.slice(0, 28), 32);
  return {
    salt_kdf: bytesToBase64URL(kdf),
    salt_auth: bytesToBase64URL(auth),
    wrapped_vault_key: `v1.${bytesToBase64URL(wrapped)}`,
  };
}
