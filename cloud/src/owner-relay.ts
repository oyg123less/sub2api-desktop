import type { Bindings } from "./types";

interface InventoryAccount {
  account_uid: string;
  enabled: boolean;
  healthy: boolean;
  proxy_ready: boolean;
  active_requests: number;
  max_concurrency: number;
}

interface RelayConnection {
  deviceID: string;
  socket: WebSocket;
  primary: boolean;
  capabilities: string[];
  sessionID: string;
  protocol: 1 | 2;
  lastHeartbeat: number;
  inventory: Map<string, InventoryAccount>;
}

interface PendingRelay {
  requestID: string;
  attemptID: string;
  payload: Record<string, unknown>;
  candidates: RelayConnection[];
  candidateIndex: number;
  connection: RelayConnection;
  started: boolean;
  resolved: boolean;
  resolve: (response: Response) => void;
  controller?: ReadableStreamDefaultController<Uint8Array>;
  timer: ReturnType<typeof setTimeout>;
  hardTimer: ReturnType<typeof setTimeout>;
}

interface RelayMessage {
  protocol?: number;
  type?: string;
  request_id?: string;
  attempt_id?: string;
  status?: number;
  headers?: Record<string, string>;
  data?: string;
  error_code?: string;
  message?: string;
  upstream_started?: boolean;
  accounts?: InventoryAccount[];
}

const encoder = new TextEncoder();

function base64URLToBytes(value: string): Uint8Array {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - normalized.length % 4) % 4);
  const raw = atob(padded);
  return Uint8Array.from(raw, (char) => char.charCodeAt(0));
}

function safeRelayHeaders(input: Record<string, string> | undefined): Headers {
  const headers = new Headers();
  for (const name of ["content-type", "openai-processing-ms", "x-request-id", "x-ratelimit-limit-requests", "x-ratelimit-remaining-requests", "x-ratelimit-reset-requests"]) {
    const value = input?.[name];
    if (value) headers.set(name, value.slice(0, 2048));
  }
  headers.set("Cache-Control", "no-store");
  headers.set("X-Content-Type-Options", "nosniff");
  return headers;
}

function relayError(status: number, code: string, message: string): Response {
  return Response.json({ error: { code, message } }, { status, headers: { "Cache-Control": "no-store" } });
}

function validInventoryAccount(value: InventoryAccount): boolean {
  return Boolean(value && typeof value.account_uid === "string" && value.account_uid.length > 0 && value.account_uid.length <= 128
    && typeof value.enabled === "boolean" && typeof value.healthy === "boolean" && typeof value.proxy_ready === "boolean"
    && Number.isInteger(value.active_requests) && value.active_requests >= 0
    && Number.isInteger(value.max_concurrency) && value.max_concurrency >= 0 && value.max_concurrency <= 100);
}

export class OwnerRelay {
  private readonly state: DurableObjectState;
  private readonly env: Bindings;
  private readonly connections = new Map<string, RelayConnection>();
  private readonly pending = new Map<string, PendingRelay>();

  constructor(state: DurableObjectState, env: Bindings) {
    this.state = state;
    this.env = env;
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/connect") return this.connectWebSocket(request);
    if (url.pathname === "/status") return this.status();
    if (url.pathname === "/request" && request.method === "POST") return this.relay(request);
    if (url.pathname === "/disconnect" && request.method === "POST") return this.disconnect(request);
    return new Response("not found", { status: 404 });
  }

  private connectWebSocket(request: Request): Response {
    if ((request.headers.get("Upgrade") || "").toLowerCase() !== "websocket") return new Response("upgrade required", { status: 426 });
    const deviceID = request.headers.get("X-Amber-Device-ID") || "";
    const sessionID = request.headers.get("X-Amber-Relay-Session") || "";
    const protocol = request.headers.get("X-Amber-Relay-Protocol") === "2" ? 2 : 1;
    if (!deviceID || !sessionID) return new Response("invalid device", { status: 400 });
    const existing = this.connections.get(deviceID);
    if (existing) existing.socket.close(4001, "replaced");
    const pair = new WebSocketPair();
    const client = pair[0];
    const server = pair[1];
    server.accept();
    const connection: RelayConnection = {
      deviceID,
      socket: server,
      sessionID,
      protocol,
      primary: request.headers.get("X-Amber-Device-Primary") === "1",
      capabilities: (request.headers.get("X-Amber-Device-Capabilities") || "").split(",").filter(Boolean),
      lastHeartbeat: Date.now(),
      inventory: new Map(),
    };
    this.connections.set(deviceID, connection);
    server.addEventListener("message", (event) => this.onMessage(connection, event));
    server.addEventListener("close", () => this.onClose(connection, "socket_closed"));
    server.addEventListener("error", () => this.onClose(connection, "socket_error"));
    server.send(JSON.stringify({ protocol, type: "hello_ack", session_id: sessionID, heartbeat_seconds: 20 }));
    return new Response(null, { status: 101, webSocket: client });
  }

  private status(): Response {
    const now = Date.now();
    const devices = [...this.connections.values()].filter((connection) => now - connection.lastHeartbeat <= 60_000).map((connection) => ({
      public_id: connection.deviceID,
      primary: connection.primary,
      protocol: connection.protocol,
      capabilities: connection.capabilities,
      available_accounts: [...connection.inventory.values()].filter((account) => this.inventoryEligible(account)).length,
      last_heartbeat_at: new Date(connection.lastHeartbeat).toISOString(),
      active_requests: [...this.pending.values()].filter((pending) => pending.connection.deviceID === connection.deviceID).length,
    }));
    return Response.json({ devices });
  }

  private inventoryEligible(account: InventoryAccount | undefined): boolean {
    return Boolean(account?.enabled && account.healthy && account.proxy_ready
      && (account.max_concurrency <= 0 || account.active_requests < account.max_concurrency));
  }

  private connectionEligible(connection: RelayConnection, accountUID: string, fixedRoute: boolean): boolean {
    if (Date.now() - connection.lastHeartbeat > 60_000) return false;
    if (connection.protocol === 1) return fixedRoute;
    return this.inventoryEligible(connection.inventory.get(accountUID));
  }

  private candidateConnections(payload: Record<string, unknown>): RelayConnection[] {
    const host = typeof payload.host_device_id === "string" ? payload.host_device_id : "";
    const fallbacks = Array.isArray(payload.fallback_device_ids)
      ? payload.fallback_device_ids.filter((item): item is string => typeof item === "string").slice(0, 10)
      : [];
    const accountUID = typeof payload.account_uid === "string" ? payload.account_uid : "";
    if (host) {
      return [...new Set([host, ...fallbacks])]
        .map((deviceID) => this.connections.get(deviceID))
        .filter((connection): connection is RelayConnection => Boolean(connection))
        .filter((connection) => this.connectionEligible(connection, accountUID, true));
    }
    return [...this.connections.values()]
      .filter((connection) => this.connectionEligible(connection, accountUID, false) || connection.protocol === 1)
      .sort((left, right) => Number(right.primary) - Number(left.primary));
  }

  private noCandidateError(payload: Record<string, unknown>): Response {
    const host = typeof payload.host_device_id === "string" ? payload.host_device_id : "";
    if (!host) return relayError(503, "owner_device_offline", "The share owner's device is offline.");
    const connection = this.connections.get(host);
    if (!connection || Date.now() - connection.lastHeartbeat > 60_000) {
      return relayError(503, "share_host_device_offline", "The device hosting this share is offline.");
    }
    return relayError(503, "share_account_not_on_host", "The shared account is not available on the configured device.");
  }

  private async relay(request: Request): Promise<Response> {
    if (this.pending.size >= 100) return relayError(503, "owner_relay_busy", "The owner's relay is busy.");
    const payload = await request.json<Record<string, unknown>>();
    const requestID = typeof payload.request_id === "string" ? payload.request_id : "";
    const accountUID = typeof payload.account_uid === "string" ? payload.account_uid : "";
    if (!requestID || !accountUID || this.pending.has(requestID)) return relayError(400, "invalid_relay_request", "The relay request is invalid.");
    const candidates = this.candidateConnections(payload);
    if (!candidates.length) return this.noCandidateError(payload);
    return new Promise<Response>((resolve) => {
      const pending: PendingRelay = {
        requestID,
        attemptID: "",
        payload,
        candidates,
        candidateIndex: -1,
        connection: candidates[0]!,
        started: false,
        resolved: false,
        resolve,
        timer: setTimeout(() => undefined, 0),
        hardTimer: setTimeout(() => this.timeoutPending(requestID, true), 30 * 60_000),
      };
      clearTimeout(pending.timer);
      this.pending.set(requestID, pending);
      if (!this.dispatchNext(pending)) {
        resolve(this.noCandidateError(payload));
        this.finishPending(pending, false);
      }
    });
  }

  private dispatchNext(pending: PendingRelay): boolean {
    clearTimeout(pending.timer);
    while (++pending.candidateIndex < pending.candidates.length) {
      const connection = pending.candidates[pending.candidateIndex];
		if (!connection) continue;
      const accountUID = String(pending.payload.account_uid || "");
      if (!this.connectionEligible(connection, accountUID, Boolean(pending.payload.host_device_id))) continue;
      pending.connection = connection;
      pending.attemptID = crypto.randomUUID();
      pending.started = false;
      pending.resolved = false;
      pending.controller = undefined;
      try {
        connection.socket.send(JSON.stringify({
          protocol: connection.protocol,
          type: "relay_request",
          ...pending.payload,
          attempt_id: connection.protocol === 2 ? pending.attemptID : undefined,
        }));
        pending.timer = setTimeout(() => this.timeoutPending(pending.requestID, false), 35_000);
        return true;
      } catch {
        continue;
      }
    }
    return false;
  }

  private timeoutPending(requestID: string, hard: boolean): void {
    const pending = this.pending.get(requestID);
    if (!pending) return;
    if (!hard && !pending.started && !pending.resolved) {
      this.cancelAttempt(pending);
      if (this.dispatchNext(pending)) return;
    }
    const code = pending.started || pending.resolved ? "relay_result_unknown" : "relay_timeout";
    if (pending.resolved) pending.controller?.error(new Error(code));
    else pending.resolve(relayError(code === "relay_timeout" ? 504 : 502, code,
      code === "relay_timeout" ? "The owner-device relay timed out." : "The relay stopped after the upstream request started. Amber did not replay it."));
    this.cancelAttempt(pending);
    this.finishPending(pending, false);
  }

  private cancelAttempt(pending: PendingRelay): void {
    try {
      pending.connection.socket.send(JSON.stringify({
        protocol: pending.connection.protocol,
        type: "cancel_request",
        request_id: pending.requestID,
        attempt_id: pending.connection.protocol === 2 ? pending.attemptID : undefined,
      }));
    } catch { /* closed */ }
  }

  private async disconnect(request: Request): Promise<Response> {
    const body = await request.json<{ device_id?: string }>();
    const connection = body.device_id ? this.connections.get(body.device_id) : undefined;
    connection?.socket.close(4002, "revoked");
    return Response.json({ ok: true });
  }

  private onMessage(connection: RelayConnection, event: MessageEvent): void {
    if (typeof event.data !== "string" || event.data.length > 128 * 1024) {
      connection.socket.close(4003, "invalid_message");
      return;
    }
    let message: RelayMessage;
    try { message = JSON.parse(event.data) as RelayMessage; } catch { connection.socket.close(4003, "invalid_message"); return; }
    if (message.protocol !== connection.protocol || !message.type) return;
    if (message.type === "heartbeat") {
      connection.lastHeartbeat = Date.now();
      connection.socket.send(JSON.stringify({ protocol: connection.protocol, type: "heartbeat_ack", at: new Date().toISOString() }));
      this.state.waitUntil(this.env.DB.prepare("UPDATE share_device_sessions SET last_heartbeat_at=? WHERE id=? AND disconnected_at IS NULL")
        .bind(new Date().toISOString(), connection.sessionID).run());
      return;
    }
    if (message.type === "inventory" && connection.protocol === 2) {
      if (!Array.isArray(message.accounts) || message.accounts.length > 500 || message.accounts.some((account) => !validInventoryAccount(account))) {
        connection.socket.close(4003, "invalid_inventory");
        return;
      }
      connection.inventory = new Map(message.accounts.map((account) => [account.account_uid, account]));
      connection.lastHeartbeat = Date.now();
      return;
    }
    const requestID = message.request_id || "";
    const pending = this.pending.get(requestID);
    if (!pending || pending.connection !== connection) return;
    if (connection.protocol === 2 && message.attempt_id !== pending.attemptID) return;
    if (message.type === "relay_accepted") { this.armIdleTimeout(pending, 35_000); return; }
    if (message.type === "upstream_started") { pending.started = true; this.armIdleTimeout(pending, 30_000); return; }
    if (message.type === "response_start") {
      if (pending.resolved) return;
      pending.started = true;
      pending.resolved = true;
      this.armIdleTimeout(pending, 90_000);
      const stream = new ReadableStream<Uint8Array>({ start: (controller) => { pending.controller = controller; } });
      pending.resolve(new Response(stream, { status: Number(message.status) || 502, headers: safeRelayHeaders(message.headers) }));
      return;
    }
    if (message.type === "response_chunk") {
      if (!pending.resolved || !pending.controller || typeof message.data !== "string") return;
      try {
        pending.controller.enqueue(base64URLToBytes(message.data));
        this.armIdleTimeout(pending, 90_000);
        connection.socket.send(JSON.stringify({ protocol: connection.protocol, type: "chunk_ack", request_id: requestID, attempt_id: message.attempt_id }));
      } catch { this.finishPending(pending, false); }
      return;
    }
    if (message.type === "response_end") { this.finishPending(pending, true); return; }
    if (message.type === "relay_error") {
      if (!pending.started && !pending.resolved) {
        this.cancelAttempt(pending);
        if (this.dispatchNext(pending)) return;
      }
      const status = Number(message.status) || 502;
      const code = message.upstream_started || pending.started ? "relay_result_unknown" : (message.error_code || "owner_relay_failed");
      const publicMessage = code === "relay_result_unknown"
        ? "The relay disconnected after the upstream request started. Amber did not replay it."
        : (message.message || "The owner-device relay failed.");
      if (pending.resolved) pending.controller?.error(new Error(code));
      else pending.resolve(relayError(status, code, publicMessage));
      this.finishPending(pending, false);
    }
  }

  private onClose(connection: RelayConnection, reason: string): void {
    if (this.connections.get(connection.deviceID) === connection) this.connections.delete(connection.deviceID);
    const now = new Date().toISOString();
    this.state.waitUntil(this.env.DB.prepare(`UPDATE share_device_sessions SET disconnected_at=?,close_reason=?
      WHERE id=? AND disconnected_at IS NULL`).bind(now, reason, connection.sessionID).run());
    for (const pending of [...this.pending.values()]) {
      if (pending.connection !== connection) continue;
      if (!pending.started && !pending.resolved && this.dispatchNext(pending)) continue;
      const code = pending.started || pending.resolved ? "relay_result_unknown" : "share_host_device_offline";
      if (pending.resolved) pending.controller?.error(new Error(code));
      else pending.resolve(relayError(code === "relay_result_unknown" ? 502 : 503, code, code === "relay_result_unknown"
        ? "The relay disconnected after the upstream request started. Amber did not replay it."
        : "The device hosting this share is offline."));
      this.finishPending(pending, false);
    }
  }

  private finishPending(pending: PendingRelay, closeStream: boolean): void {
    clearTimeout(pending.timer);
    clearTimeout(pending.hardTimer);
    if (closeStream) pending.controller?.close();
    this.pending.delete(pending.requestID);
  }

  private armIdleTimeout(pending: PendingRelay, delay: number): void {
    clearTimeout(pending.timer);
    pending.timer = setTimeout(() => this.timeoutPending(pending.requestID, false), delay);
  }
}
