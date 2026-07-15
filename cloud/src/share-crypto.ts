import { AppError } from "./errors";
import { base64URLToBytes, bytesToBase64URL } from "./security";
import type { Bindings } from "./types";

const encoder = new TextEncoder();
const decoder = new TextDecoder();

export interface ShareCredential {
  token: string;
  account_type: "oauth" | "api_key";
  upstream_url: string;
  chatgpt_account_id?: string;
}

function owned(bytes: Uint8Array): Uint8Array<ArrayBuffer> {
  return new Uint8Array(bytes);
}

function shareKeyBytes(secret: string): Uint8Array<ArrayBuffer> {
  try {
    const bytes = base64URLToBytes(secret);
    if (bytes.length === 32) return owned(bytes);
  } catch {
    // Report one public configuration error below.
  }
  throw new AppError(503, "share_kms_not_configured", "Cloud sharing is not configured.");
}

async function shareKey(env: Bindings): Promise<CryptoKey> {
  if (!env.SHARE_KMS_KEY) throw new AppError(503, "share_kms_not_configured", "Cloud sharing is not configured.");
  return crypto.subtle.importKey("raw", shareKeyBytes(env.SHARE_KMS_KEY), { name: "AES-GCM" }, false, ["encrypt", "decrypt"]);
}

function aad(ownerID: number, accountUID: string): Uint8Array<ArrayBuffer> {
  return owned(encoder.encode(`amber-share-v1:${ownerID}:${accountUID}`));
}

export function validateShareCredential(value: unknown): ShareCredential {
  if (!value || typeof value !== "object") throw new AppError(400, "invalid_share_credential", "The shared account credential is invalid.");
  const credential = value as Record<string, unknown>;
  const token = typeof credential.token === "string" ? credential.token.trim() : "";
  const accountType = credential.account_type;
  const accountID = typeof credential.chatgpt_account_id === "string" ? credential.chatgpt_account_id.trim() : "";
  if (!token || token.length > 16 * 1024 || (accountType !== "oauth" && accountType !== "api_key") || accountID.length > 512) {
    throw new AppError(400, "invalid_share_credential", "The shared account credential is invalid.");
  }
  let upstream: URL;
  try {
    upstream = new URL(typeof credential.upstream_url === "string" ? credential.upstream_url : "");
  } catch {
    throw new AppError(400, "unsupported_share_upstream", "Only supported OpenAI upstreams can be shared.");
  }
  const host = upstream.hostname.toLowerCase();
  if (upstream.protocol !== "https:" || upstream.username || upstream.password || upstream.hash ||
      (host !== "chatgpt.com" && host !== "api.openai.com")) {
    throw new AppError(400, "unsupported_share_upstream", "Only supported OpenAI upstreams can be shared.");
  }
  return { token, account_type: accountType, upstream_url: upstream.toString(), ...(accountID ? { chatgpt_account_id: accountID } : {}) };
}

export async function encryptShareCredential(env: Bindings, ownerID: number, accountUID: string, credential: ShareCredential): Promise<string> {
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const plaintext = encoder.encode(JSON.stringify(credential));
  const ciphertext = new Uint8Array(await crypto.subtle.encrypt(
    { name: "AES-GCM", iv: nonce, additionalData: aad(ownerID, accountUID) },
    await shareKey(env),
    plaintext,
  ));
  const packed = new Uint8Array(nonce.length + ciphertext.length);
  packed.set(nonce);
  packed.set(ciphertext, nonce.length);
  return `v1.${bytesToBase64URL(packed)}`;
}

export async function decryptShareCredential(env: Bindings, ownerID: number, accountUID: string, value: string): Promise<ShareCredential> {
  if (!value.startsWith("v1.")) throw new AppError(503, "share_credential_unavailable", "The shared account credential is unavailable.");
  try {
    const packed = base64URLToBytes(value.slice(3));
    if (packed.length < 29) throw new Error("short ciphertext");
    const plaintext = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: owned(packed.slice(0, 12)), additionalData: aad(ownerID, accountUID) },
      await shareKey(env),
      owned(packed.slice(12)),
    );
    return validateShareCredential(JSON.parse(decoder.decode(plaintext)));
  } catch (error) {
    if (error instanceof AppError) throw error;
    throw new AppError(503, "share_credential_unavailable", "The shared account credential is unavailable.");
  }
}
