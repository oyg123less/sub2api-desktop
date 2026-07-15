import { AppError } from "./errors";
import { sha256 } from "./security";
import type { Bindings } from "./types";

interface Counter {
  count: number;
}

export async function rateLimit(
  env: Bindings,
  scope: string,
  subject: string,
  limit: number,
  windowSeconds: number,
): Promise<void> {
  const key = `rate:${scope}:${await sha256(subject)}`;
  const current = await env.SESSIONS.get<Counter>(key, "json") ?? { count: 0 };
  if (current.count >= limit) {
    throw new AppError(429, "rate_limited", "Too many attempts. Try again later.");
  }
  await env.SESSIONS.put(key, JSON.stringify({ count: current.count + 1 }), { expirationTtl: windowSeconds });
}

export async function checkLoginLock(env: Bindings, subject: string): Promise<void> {
  const key = `login-lock:${await sha256(subject)}`;
  if (await env.SESSIONS.get(key)) {
    throw new AppError(429, "login_locked", "Too many failed logins. Try again in 15 minutes.");
  }
}

export async function recordLoginFailure(env: Bindings, subject: string): Promise<void> {
  const digest = await sha256(subject);
  const key = `login-fail:${digest}`;
  const current = await env.SESSIONS.get<Counter>(key, "json") ?? { count: 0 };
  const next = current.count + 1;
  if (next >= 10) {
    await Promise.all([
      env.SESSIONS.delete(key),
      env.SESSIONS.put(`login-lock:${digest}`, "1", { expirationTtl: 15 * 60 }),
    ]);
    return;
  }
  await env.SESSIONS.put(key, JSON.stringify({ count: next }), { expirationTtl: 60 * 60 });
}

export async function clearLoginFailures(env: Bindings, subject: string): Promise<void> {
  const digest = await sha256(subject);
  await env.SESSIONS.delete(`login-fail:${digest}`);
}
