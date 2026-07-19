interface AttemptState {
  failures: number[];
  lockedUntil: number;
}

export class ShareConnectGuard {
  constructor(private readonly state: DurableObjectState) {}

  async fetch(request: Request): Promise<Response> {
    if (request.method !== "POST") return new Response("method not allowed", { status: 405 });
    const body = await request.json<{ key?: string; success?: boolean; record?: boolean }>();
    const key = (body.key || "").slice(0, 256);
    if (!key) return Response.json({ allowed: false, retry_after: 600 }, { status: 429 });
    const now = Date.now();
    const stored = await this.state.storage.get<AttemptState>(key) || { failures: [], lockedUntil: 0 };
    if (body.success) {
      await this.state.storage.delete(key);
      return Response.json({ allowed: true });
    }
    stored.failures = stored.failures.filter((value) => now - value < 10 * 60_000);
    if (stored.lockedUntil > now) {
      return Response.json({ allowed: false, retry_after: Math.ceil((stored.lockedUntil - now) / 1000) }, { status: 429 });
    }
    if (body.record) {
      stored.failures.push(now);
      if (stored.failures.length >= 5) stored.lockedUntil = now + 10 * 60_000;
      await this.state.storage.put(key, stored);
    }
    return Response.json({ allowed: stored.lockedUntil <= now, retry_after: stored.lockedUntil > now ? 600 : 0 },
      { status: stored.lockedUntil > now ? 429 : 200 });
  }
}
