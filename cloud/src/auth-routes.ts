import { Hono } from "hono";
import { requireAuth } from "./auth-middleware";
import { AppError, readJSON } from "./errors";
import { mailerFor } from "./mailer";
import { checkLoginLock, clearLoginFailures, rateLimit, recordLoginFailure } from "./rate-limit";
import {
  authUserFromRow,
  authVerifier,
  beginRefreshSession,
  completeRefreshSession,
  deleteRefreshSession,
  fakeLoginParameters,
  isBase64URLBytes,
  issueSession,
  randomVerificationCode,
  safeEqual,
  sha256,
} from "./security";
import { verifyTurnstile } from "./turnstile";
import type { AppEnv, UserRow } from "./types";
import {
  newVerificationRecord,
  normalizeVerificationRecord,
  rotateVerificationRecord,
  verificationKVOptions,
  verificationMatches,
} from "./verification";

const auth = new Hono<AppEnv>();

const userColumns = `id, email, auth_hash, salt_kdf, salt_auth, wrapped_vault_key,
  email_verified, role, banned, session_version, created_at, updated_at, last_active_at`;

function normalizeEmail(value: unknown): string {
  const email = typeof value === "string" ? value.trim().toLowerCase() : "";
  if (email.length < 3 || email.length > 254 || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    throw new AppError(400, "invalid_email", "Enter a valid email address.");
  }
  return email;
}

function validateWrappedKey(value: unknown): value is string {
  return typeof value === "string" && value.startsWith("v1.") && value.length >= 48 && value.length <= 2048;
}

function validateAuthMaterial(body: Record<string, unknown>) {
  if (!isBase64URLBytes(body.auth_hash, 32) || !isBase64URLBytes(body.salt_kdf, 16) ||
      !isBase64URLBytes(body.salt_auth, 16) || !validateWrappedKey(body.wrapped_vault_key)) {
    throw new AppError(400, "invalid_auth_material", "The authentication material is invalid.");
  }
  return {
    authHash: body.auth_hash,
    saltKDF: body.salt_kdf,
    saltAuth: body.salt_auth,
    wrappedVaultKey: body.wrapped_vault_key,
  };
}

function clientIP(headers: Headers): string {
  return headers.get("cf-connecting-ip") || headers.get("x-forwarded-for")?.split(",")[0]?.trim() || "unknown";
}

async function registrationEnabled(env: AppEnv["Bindings"]): Promise<boolean> {
  const row = await env.DB.prepare("SELECT value FROM platform_settings WHERE key='registration_enabled'").first<{ value: string }>();
  return row?.value !== "false";
}

auth.post("/register", async (c) => {
  const ip = clientIP(c.req.raw.headers);
  await rateLimit(c.env, "register", ip, 5, 60 * 60);
  if (!await registrationEnabled(c.env)) {
    throw new AppError(503, "registration_disabled", "New registrations are temporarily disabled.");
  }
  const body = await readJSON<Record<string, unknown>>(c);
  const email = normalizeEmail(body.email);
  const material = validateAuthMaterial(body);
  await verifyTurnstile(c.env, body.turnstile_token, ip);

  const existing = await c.env.DB.prepare(`SELECT ${userColumns} FROM users WHERE email=?`).bind(email).first<UserRow>();
  if (existing?.email_verified) {
    throw new AppError(409, "email_already_registered", "This email address is already registered.");
  }
  const now = new Date().toISOString();
  const verifier = await authVerifier(material.authHash);
  let userID: number;
  if (existing) {
    await c.env.DB.prepare(`UPDATE users SET auth_hash=?, salt_kdf=?, salt_auth=?, wrapped_vault_key=?,
      updated_at=? WHERE id=?`).bind(verifier, material.saltKDF, material.saltAuth, material.wrappedVaultKey, now, existing.id).run();
    userID = existing.id;
  } else {
    const result = await c.env.DB.prepare(`INSERT INTO users
      (email, auth_hash, salt_kdf, salt_auth, wrapped_vault_key, created_at, updated_at)
      VALUES(?,?,?,?,?,?,?)`).bind(
        email, verifier, material.saltKDF, material.saltAuth, material.wrappedVaultKey, now, now,
      ).run();
    userID = Number(result.meta.last_row_id);
  }

  const code = randomVerificationCode();
  const verificationKey = `verify:${await sha256(email)}`;
  const delivery = await mailerFor(c.env).sendVerification(email, code);
  const record = newVerificationRecord(userID, await sha256(`amber-verify-v1:${code}`), delivery.id);
  await c.env.SESSIONS.put(verificationKey, JSON.stringify(record), verificationKVOptions(record));
  return c.json({ ok: true, verification_required: true }, 202);
});

auth.post("/resend-verification", async (c) => {
  const body = await readJSON<Record<string, unknown>>(c);
  const email = normalizeEmail(body.email);
  await rateLimit(c.env, "resend", email, 3, 60 * 60);
  const user = await c.env.DB.prepare("SELECT id, email_verified FROM users WHERE email=?")
    .bind(email).first<{ id: number; email_verified: number }>();
  if (!user || user.email_verified) return c.json({ ok: true }, 202);

  const code = randomVerificationCode();
  let delivery: { id: string };
  try {
    delivery = await mailerFor(c.env).sendVerification(email, code);
  } catch (error) {
    console.error(JSON.stringify({
      event: "verification_resend_failed",
      error_type: error instanceof Error ? error.name : "unknown",
    }));
    return c.json({ ok: true }, 202);
  }
  const verificationKey = `verify:${await sha256(email)}`;
  const previous = normalizeVerificationRecord(await c.env.SESSIONS.get(verificationKey, "json"));
  const record = rotateVerificationRecord(previous, user.id, await sha256(`amber-verify-v1:${code}`), delivery.id);
  await c.env.SESSIONS.put(verificationKey, JSON.stringify(record), verificationKVOptions(record));
  return c.json({ ok: true }, 202);
});

auth.post("/verify-email", async (c) => {
  const body = await readJSON<Record<string, unknown>>(c);
  const email = normalizeEmail(body.email);
  const code = typeof body.code === "string" ? body.code.trim() : "";
  if (!/^\d{6}$/.test(code)) throw new AppError(400, "invalid_verification_code", "The verification code is invalid.");
  const key = `verify:${await sha256(email)}`;
  const record = normalizeVerificationRecord(await c.env.SESSIONS.get(key, "json"));
  if (!record || record.expires_at <= Date.now()) {
    if (record) await c.env.SESSIONS.delete(key);
    throw new AppError(400, "verification_expired", "The verification code has expired.");
  }
  const matches = verificationMatches(record, await sha256(`amber-verify-v1:${code}`));
  if (!matches) {
    const attempts = record.attempts + 1;
    if (attempts >= 5) await c.env.SESSIONS.delete(key);
    else await c.env.SESSIONS.put(key, JSON.stringify({ ...record, attempts }), verificationKVOptions(record));
    throw new AppError(400, attempts >= 5 ? "verification_exhausted" : "invalid_verification_code",
      attempts >= 5 ? "Too many incorrect codes. Register again." : "The verification code is invalid.");
  }
  const now = new Date().toISOString();
  const result = await c.env.DB.prepare("UPDATE users SET email_verified=1, updated_at=? WHERE id=? AND email=?")
    .bind(now, record.user_id, email).run();
  await c.env.SESSIONS.delete(key);
  if (!result.meta.changes) throw new AppError(400, "verification_expired", "The verification request is no longer valid.");
  return c.json({ ok: true });
});

auth.post("/parameters", async (c) => {
  const ip = clientIP(c.req.raw.headers);
  await rateLimit(c.env, "auth-parameters", ip, 60, 60 * 60);
  const body = await readJSON<Record<string, unknown>>(c);
  const email = normalizeEmail(body.email);
  const row = await c.env.DB.prepare("SELECT salt_kdf, salt_auth FROM users WHERE email=? AND email_verified=1")
    .bind(email).first<{ salt_kdf: string; salt_auth: string }>();
  return c.json(row ?? await fakeLoginParameters(c.env.JWT_SECRET, email));
});

auth.post("/login", async (c) => {
  const ip = clientIP(c.req.raw.headers);
  const body = await readJSON<Record<string, unknown>>(c);
  const email = normalizeEmail(body.email);
  const authHash = body.auth_hash;
  if (!isBase64URLBytes(authHash, 32)) throw new AppError(401, "invalid_credentials", "Email or master password is incorrect.");
  const subject = `${ip}:${email}`;
  await checkLoginLock(c.env, subject);
  const row = await c.env.DB.prepare(`SELECT ${userColumns} FROM users WHERE email=?`).bind(email).first<UserRow>();
  const suppliedVerifier = await authVerifier(authHash);
  if (!row || !row.email_verified || !safeEqual(row.auth_hash, suppliedVerifier)) {
    await recordLoginFailure(c.env, subject);
    throw new AppError(401, "invalid_credentials", "Email or master password is incorrect.");
  }
  if (row.banned) {
    await clearLoginFailures(c.env, subject);
    throw new AppError(403, "account_disabled", "This account has been disabled.");
  }
  await clearLoginFailures(c.env, subject);
  await c.env.DB.prepare("UPDATE users SET last_active_at=?, updated_at=? WHERE id=?")
    .bind(new Date().toISOString(), new Date().toISOString(), row.id).run();
  return c.json({
    ...await issueSession(c.env, authUserFromRow(row)),
    user: { id: row.id, email: row.email, role: row.role },
    salt_kdf: row.salt_kdf,
    salt_auth: row.salt_auth,
    wrapped_vault_key: row.wrapped_vault_key,
  });
});

auth.post("/refresh", async (c) => {
  const body = await readJSON<Record<string, unknown>>(c);
  const refreshToken = typeof body.refresh_token === "string" ? body.refresh_token : "";
  const rotation = await beginRefreshSession(c.env, refreshToken);
  const session = rotation.session;
  const row = await c.env.DB.prepare(`SELECT ${userColumns} FROM users WHERE id=?`).bind(session.user_id).first<UserRow>();
  if (!row || !row.email_verified || row.banned || row.session_version !== session.session_version) {
    await deleteRefreshSession(c.env, refreshToken);
    throw new AppError(401, row?.banned ? "account_disabled" : "session_expired", row?.banned
      ? "This account has been disabled."
      : "The session has expired.");
  }
  const response = await issueSession(c.env, authUserFromRow(row), rotation.nextRefreshToken);
  await completeRefreshSession(c.env, refreshToken, session, rotation.nextRefreshToken, rotation.replay);
  return c.json(response);
});

auth.post("/logout", async (c) => {
  const body = await readJSON<Record<string, unknown>>(c);
  await deleteRefreshSession(c.env, typeof body.refresh_token === "string" ? body.refresh_token : "");
  return c.json({ ok: true });
});

auth.get("/me", requireAuth, (c) => c.json({ user: c.get("auth") }));

auth.put("/master-password", requireAuth, async (c) => {
  const body = await readJSON<Record<string, unknown>>(c);
  const material = validateAuthMaterial(body);
  const user = c.get("auth");
  const currentAuthHash = body.current_auth_hash;
  if (!isBase64URLBytes(currentAuthHash, 32)) {
    throw new AppError(401, "invalid_credentials", "The current master password is incorrect.");
  }
  const current = await c.env.DB.prepare("SELECT auth_hash FROM users WHERE id=?").bind(user.id).first<{ auth_hash: string }>();
  if (!current || !safeEqual(current.auth_hash, await authVerifier(currentAuthHash))) {
    throw new AppError(401, "invalid_credentials", "The current master password is incorrect.");
  }
  const now = new Date().toISOString();
  await c.env.DB.prepare(`UPDATE users SET auth_hash=?, salt_kdf=?, salt_auth=?, wrapped_vault_key=?,
    session_version=session_version+1, updated_at=? WHERE id=?`).bind(
      await authVerifier(material.authHash), material.saltKDF, material.saltAuth, material.wrappedVaultKey, now, user.id,
    ).run();
  return c.json({ ok: true, reauthentication_required: true });
});

export default auth;
