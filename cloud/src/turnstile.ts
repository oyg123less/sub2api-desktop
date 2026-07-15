import { AppError } from "./errors";
import type { Bindings } from "./types";

interface TurnstileResponse {
  success?: boolean;
  hostname?: string;
}

export async function verifyTurnstile(env: Bindings, token: unknown, remoteIP: string): Promise<void> {
  if (typeof token !== "string" || !token.trim()) {
    throw new AppError(400, "turnstile_required", "Human verification is required.");
  }
  if (env.ENVIRONMENT === "test" && token === "test-pass") return;
  if (!env.TURNSTILE_SECRET) {
    throw new AppError(503, "turnstile_not_configured", "Human verification is not configured.");
  }
  const body = new URLSearchParams({ secret: env.TURNSTILE_SECRET, response: token });
  if (remoteIP && remoteIP !== "unknown") body.set("remoteip", remoteIP);
  const response = await fetch("https://challenges.cloudflare.com/turnstile/v0/siteverify", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });
  const result = response.ok ? await response.json<TurnstileResponse>() : {};
  if (!result.success || (env.TURNSTILE_HOSTNAME && result.hostname !== env.TURNSTILE_HOSTNAME)) {
    throw new AppError(400, "turnstile_failed", "Human verification failed.");
  }
}
