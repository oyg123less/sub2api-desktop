import type { Bindings } from "./types";

interface AccessState {
  events: number[];
  leases: Record<string, number>;
}

export class ShareAccessCoordinator {
  private readonly state: DurableObjectState;

  constructor(state: DurableObjectState, _env: Bindings) {
    this.state = state;
  }

  async fetch(request: Request): Promise<Response> {
    const path = new URL(request.url).pathname;
    if (path === "/acquire" && request.method === "POST") return this.acquire(request);
    if (path === "/release" && request.method === "POST") return this.release(request);
    return new Response("not found", { status: 404 });
  }

  private async acquire(request: Request): Promise<Response> {
    const body = await request.json<{ ticket?: string; rpm?: number; concurrency?: number }>();
    const ticket = typeof body.ticket === "string" ? body.ticket : "";
    const rpm = Number(body.rpm);
    const concurrency = Number(body.concurrency);
    if (!ticket || !Number.isInteger(rpm) || rpm < 1 || rpm > 600 || !Number.isInteger(concurrency) || concurrency < 1 || concurrency > 20) {
      return Response.json({ error: "invalid_access_policy" }, { status: 400 });
    }
    return this.state.storage.transaction(async (transaction) => {
      const now = Date.now();
      const current = await transaction.get<AccessState>("state") || { events: [], leases: {} };
      current.events = current.events.filter((event) => event > now - 60_000);
      current.leases = Object.fromEntries(Object.entries(current.leases).filter(([, expiry]) => expiry > now));
      if (current.events.length >= rpm) {
        const retryAfter = Math.max(1, Math.ceil(((current.events[0] ?? now) + 60_000 - now) / 1000));
        await transaction.put("state", current);
        return Response.json({ error: "share_rate_limited", retry_after: retryAfter }, { status: 429 });
      }
      if (Object.keys(current.leases).length >= concurrency) {
        await transaction.put("state", current);
        return Response.json({ error: "share_concurrency_limited", retry_after: 1 }, { status: 429 });
      }
      current.events.push(now);
      current.leases[ticket] = now + 30 * 60_000;
      await transaction.put("state", current);
      return Response.json({ ok: true });
    });
  }

  private async release(request: Request): Promise<Response> {
    const body = await request.json<{ ticket?: string }>();
    const ticket = typeof body.ticket === "string" ? body.ticket : "";
    if (ticket) {
      await this.state.storage.transaction(async (transaction) => {
        const current = await transaction.get<AccessState>("state");
        if (!current || !current.leases[ticket]) return;
        delete current.leases[ticket];
        await transaction.put("state", current);
      });
    }
    return Response.json({ ok: true });
  }
}
