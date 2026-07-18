export type UserRole = "user" | "admin";

export interface Bindings {
  DB: D1Database;
  SESSIONS: KVNamespace;
  JWT_SECRET: string;
  TURNSTILE_SECRET: string;
  RESEND_API_KEY?: string;
  ADMIN_API_KEY?: string;
  SHARE_KMS_KEY?: string;
  ENVIRONMENT?: "development" | "test" | "production";
  MAILER_MODE?: "console" | "resend";
  RESEND_FROM?: string;
  TURNSTILE_HOSTNAME?: string;
  OWNER_RELAY: DurableObjectNamespace;
  SHARE_ACCESS: DurableObjectNamespace;
}

export interface AuthUser {
  id: number;
  email: string;
  role: UserRole;
  sessionVersion: number;
}

export interface Variables {
  auth: AuthUser;
  requestId: string;
}

export type AppEnv = {
  Bindings: Bindings;
  Variables: Variables;
};

export interface UserRow {
  id: number;
  email: string;
  auth_hash: string;
  salt_kdf: string;
  salt_auth: string;
  wrapped_vault_key: string;
  email_verified: number;
  role: UserRole;
  banned: number;
  session_version: number;
  created_at: string;
  updated_at: string;
  last_active_at: string | null;
}

export interface VaultRow {
  id: number;
  kind: "account" | "proxy" | "codex_remote" | "settings";
  client_uid: string;
  ciphertext: string;
  version: number;
  deleted: number;
  updated_at: string;
}
