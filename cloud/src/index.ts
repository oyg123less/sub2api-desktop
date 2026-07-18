import { Hono } from "hono";
import adminRoutes from "./admin-routes";
import authRoutes from "./auth-routes";
import deviceRoutes from "./device-routes";
import { AppError, errorResponse } from "./errors";
import friendRoutes from "./friend-routes";
import { OwnerRelay } from "./owner-relay";
import { randomToken } from "./security";
import { ShareAccessCoordinator } from "./share-access";
import shareGateway from "./share-gateway";
import shareGroupRoutes from "./share-group-routes";
import shareRoutes from "./share-routes";
import receivedShareRoutes from "./received-share-routes";
import type { AppEnv } from "./types";
import vaultRoutes from "./vault-routes";

const app = new Hono<AppEnv>();

app.use("*", async (c, next) => {
  const requestID = c.req.header("cf-ray") || randomToken(12);
  c.set("requestId", requestID);
  c.header("X-Request-ID", requestID);
  c.header("X-Content-Type-Options", "nosniff");
  c.header("Referrer-Policy", "no-referrer");
  c.header("Cache-Control", "no-store");
  await next();
});

app.get("/health", (c) => c.json({ ok: true, service: "amber-cloud", version: "0.4.0" }));
app.route("/v1/auth", authRoutes);
app.route("/v1/vault", vaultRoutes);
app.route("/v1/admin", adminRoutes);
app.route("/v1/shares", shareRoutes);
app.route("/v1", friendRoutes);
app.route("/v1", shareGroupRoutes);
app.route("/v1", receivedShareRoutes);
app.route("/v1", deviceRoutes);
app.route("/v1", shareGateway);

app.notFound((c) => errorResponse(c, 404, "not_found", "The requested endpoint does not exist."));
app.onError((error, c) => {
  if (error instanceof AppError) return errorResponse(c, error.status, error.code, error.publicMessage);
  console.error(JSON.stringify({
    event: "request_failed",
    request_id: c.get("requestId"),
    error_type: error instanceof Error ? error.name : "unknown",
  }));
  return errorResponse(c, 500, "internal_error", "The request could not be completed.");
});

export default app;
export { OwnerRelay, ShareAccessCoordinator };
