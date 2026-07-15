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
