import { createMiddleware } from "hono/factory";
import { AppError } from "./errors";
import { authUserFromRow, safeEqual, verifyAccessToken } from "./security";
import type { AppEnv, UserRow } from "./types";

export const requireAuth = createMiddleware<AppEnv>(async (c, next) => {
  const header = c.req.header("authorization") || "";
  const token = header.startsWith("Bearer ") ? header.slice(7).trim() : "";
  if (!token) throw new AppError(401, "authentication_required", "Authentication is required.");

  const claims = await verifyAccessToken(c.env, token);
  const row = await c.env.DB.prepare(`SELECT id, email, auth_hash, salt_kdf, salt_auth, wrapped_vault_key,
    email_verified, role, banned, session_version, created_at, updated_at, last_active_at
    FROM users WHERE id=?`).bind(Number(claims.sub)).first<UserRow>();
  if (!row || !row.email_verified || row.banned || row.session_version !== claims.sv) {
    throw new AppError(401, row?.banned ? "account_disabled" : "session_expired", row?.banned
      ? "This account has been disabled."
      : "The session has expired.");
  }
  c.set("auth", authUserFromRow(row));
  await next();
});

export const requireAdmin = createMiddleware<AppEnv>(async (c, next) => {
  const user = c.get("auth");
  if (user.role !== "admin") throw new AppError(403, "admin_required", "Administrator access is required.");
  const supplied = c.req.header("x-admin-key") || "";
  const expected = c.env.ADMIN_API_KEY || "";
  if (!expected || !safeEqual(expected, supplied)) {
    throw new AppError(403, "admin_second_factor_required", "Administrator second-factor verification is required.");
  }
  await next();
});
