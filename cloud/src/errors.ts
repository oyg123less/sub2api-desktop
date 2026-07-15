import type { Context } from "hono";

export class AppError extends Error {
  constructor(
    readonly status: 400 | 401 | 403 | 404 | 409 | 413 | 429 | 500 | 503,
    readonly code: string,
    readonly publicMessage: string,
  ) {
    super(code);
    this.name = "AppError";
  }
}

export function errorResponse(c: Context, status: number, code: string, message: string) {
  return c.json({ error: { code, message }, request_id: c.get("requestId") }, status as never);
}

export function requireJSONSize(c: Context, maxBytes = 128 * 1024): void {
  const length = Number(c.req.header("content-length") || 0);
  if (Number.isFinite(length) && length > maxBytes) {
    throw new AppError(413, "request_too_large", "The request body is too large.");
  }
}

export async function readJSON<T>(c: Context): Promise<T> {
  requireJSONSize(c);
  try {
    return await c.req.json<T>();
  } catch {
    throw new AppError(400, "invalid_json", "The request body must be valid JSON.");
  }
}
