# Amber Cloud

Cloudflare Worker for Amber v0.3.0. It provides account verification, short-lived access sessions, revocable refresh sessions, encrypted vault synchronization, and administrator governance. Vault payloads are opaque AES-256-GCM ciphertext; this service does not receive a vault key or plaintext upstream credential.

## Resources

Create one D1 database and one KV namespace, then replace the placeholder IDs in `wrangler.toml`:

```powershell
npx wrangler d1 create amber-cloud
npx wrangler kv namespace create amber-sessions
npx wrangler kv namespace create amber-sessions --preview
npx wrangler d1 migrations apply amber-cloud --remote
```

## Secrets

Never put production values in `.dev.vars`, source files, CI logs, or `wrangler.toml`. Inject them with Wrangler:

```powershell
npx wrangler secret put JWT_SECRET
npx wrangler secret put TURNSTILE_SECRET
npx wrangler secret put RESEND_API_KEY
npx wrangler secret put ADMIN_API_KEY
```

`JWT_SECRET` and `ADMIN_API_KEY` must be independent random values of at least 32 bytes. M2 will additionally require `SHARE_KMS_KEY`. Set `RESEND_FROM` to a verified Resend sender and `TURNSTILE_HOSTNAME` to the production desktop registration host when deploying.

The checked-in `RESEND_FROM` uses Resend's `onboarding@resend.dev` test sender. That sender is intentionally limited and normally delivers only to the Resend account owner's email address. Replace it with a verified sender domain before opening registration to other users.

## Local development

Copy `.dev.vars.example` to `.dev.vars`, use non-production credentials, and run:

```powershell
npm install
npm run dev
npm run typecheck
npm test
```

`MAILER_MODE=console` is allowed only outside production. Production always uses Resend. Registration test bypasses are compiled behind `ENVIRONMENT=test` and accept only the fixed integration-test token; production cannot enable that path accidentally.

## Login protocol

The desktop first requests `/v1/auth/parameters` with an email address, derives its master key and authentication hash locally, then calls `/v1/auth/login`. Unknown emails receive deterministic fake parameters of the same shape to reduce account enumeration. D1 stores a one-way SHA-256 verifier of the already memory-hard 32-byte authentication hash, so a D1-only leak cannot be replayed directly as a login credential.

All API errors use stable codes and a request ID. Request bodies, vault ciphertext, tokens, API keys, passwords, and email verification values are never included in production error logs.

## API outline

- `POST /v1/auth/register`, `POST /v1/auth/resend-verification`, and `POST /v1/auth/verify-email` implement email verification.
- `POST /v1/auth/parameters` returns only the KDF and authentication salts; the wrapped vault key is returned only by a successful `POST /v1/auth/login`.
- `POST /v1/auth/refresh` rotates refresh tokens, while `POST /v1/auth/logout` revokes one session.
- `PUT /v1/auth/master-password` rewraps the vault key and invalidates every existing session.
- `GET /v1/vault` and `PUT /v1/vault/batch` synchronize opaque encrypted items using optimistic versions and composite cursors.
- `/v1/admin/*` requires both an administrator access token and the independent `X-Admin-Key` second factor.

## Sharing gateway

M2 share owners create and manage grants through `/v1/shares`. A grant stores the upstream credential only as AES-256-GCM ciphertext encrypted under `SHARE_KMS_KEY`; the one-time `sk-share-*` guest key is represented in D1 only by its SHA-256 hash. Friends call the returned Base URL with that guest key and do not need Amber installed.

`POST /v1/responses` supports OAuth and API-key grants with streamed response passthrough. `POST /v1/chat/completions` is available for API-key grants. For the hard guarantee that an upstream cannot reflect an Authorization header, production sharing accepts only `chatgpt.com` and `api.openai.com` upstream hosts. Usage logs store only timestamp, model, HTTP status, and latency; request and response bodies are never stored.
