# Amber v0.4.0 Deployment Checklist

> This runbook deploys Cloud 2.0 without installing, uninstalling, starting, or stopping the desktop Amber application. Never paste secret values into source files, shell history, screenshots, or Git.

## 1. Release Artifacts

- Desktop version: `0.4.0`
- Worker health version: `0.4.0`
- Local database schema: `12`
- Cloud D1 schema: `4`
- NSIS installer: `src-tauri/target/release/bundle/nsis/Amber_0.4.0_x64-setup.exe`
- SHA-256: `57C2C86D4A9EC67D67DAFC4186EA721A7070AD0E8FC8894F4B6AEC42FB3D95E7`

## 2. Preflight

Run from `cloud/` with `CLOUDFLARE_API_TOKEN` supplied through the process environment:

```powershell
npx wrangler secret list
npx wrangler d1 migrations list amber-cloud --remote
npx wrangler deploy --dry-run
```

Confirm these secret names exist. The commands must never print or persist their values:

- `JWT_SECRET`
- `TURNSTILE_SECRET`
- `RESEND_API_KEY`
- `RESEND_WEBHOOK_SECRET`
- `ADMIN_API_KEY`
- `SHARE_KMS_KEY` (32 random bytes encoded as Base64URL)

Confirm `RESEND_FROM` is the verified sender configured in `wrangler.toml`.

In Resend, configure `https://amber-cloud-api.484486528.workers.dev/v1/webhooks/resend` and subscribe to sent, delivered, delivery delayed, bounced, complained, failed, and suppressed events. Store the generated `whsec_...` value only through `npx wrangler secret put RESEND_WEBHOOK_SECRET`.

## 3. Backup And Migration

Export a D1 backup before applying schema 4:

```powershell
npx wrangler d1 export amber-cloud --remote --output amber-cloud-pre-v0.4.0.sql
npx wrangler d1 migrations apply amber-cloud --remote
npx wrangler d1 migrations list amber-cloud --remote
```

Migration 4 creates friend, share-group, recipient-key, device, Relay, quota, audit, and idempotency tables. It deliberately creates all v0.4.0 feature flags as `false`.

## 4. Worker Deployment

```powershell
npx wrangler deploy
```

Verify the public health endpoint returns `version: "0.4.0"`. Then confirm a legacy v0.3.3 Guest Key still reaches the legacy gateway before enabling Cloud 2.0.

## 5. Staged Feature Enablement

Enable friends first:

```powershell
npx wrangler d1 execute amber-cloud --remote --command="UPDATE platform_settings SET value='true',updated_at=datetime('now') WHERE key='friends_enabled';"
```

After a two-user Friend Code smoke test, enable share groups:

```powershell
npx wrangler d1 execute amber-cloud --remote --command="UPDATE platform_settings SET value='true',updated_at=datetime('now') WHERE key='share_groups_enabled';"
```

Keep Owner Relay disabled until device challenge, disconnect, timeout, and no-replay smoke tests pass. Then enable it separately:

```powershell
npx wrangler d1 execute amber-cloud --remote --command="UPDATE platform_settings SET value='true',updated_at=datetime('now') WHERE key='owner_relay_enabled';"
```

## 6. Required Smoke Tests

1. Two real verified users become friends using an exact Friend Code.
2. The owner creates one group containing two accounts and two friends.
3. D1 contains different `guest_key_hash` values for both recipients and no plaintext Guest Key.
4. A recipient accepts the invitation, copies the Base URL, and runs the low-output connection test.
5. Pausing one recipient blocks only that recipient; rotating its key invalidates only its old key.
6. Disabling one group account leaves the other account available.
7. An OAuth request succeeds only while an authorized owner device is online.
8. Replaying a consumed device challenge returns `invalid_device_proof`.
9. Disconnecting after `upstream_started` returns an unknown-result error and is not replayed.
10. Legacy v0.3.3 sharing remains operational.

## 7. Rollback

Disable new traffic without deleting data:

```powershell
npx wrangler d1 execute amber-cloud --remote --command="UPDATE platform_settings SET value='false',updated_at=datetime('now') WHERE key IN ('share_groups_enabled','owner_relay_enabled');"
```

If necessary, deploy the previous Worker. Do not delete schema 4 tables during rollback. The previous Worker ignores them, while retained audit and recipient state allow a controlled recovery.

## 8. Local Verification Completed

- Vue production build passed.
- Frontend Vitest: 33 passed.
- Playwright: 46 passed across `1280x800` and `900x650`.
- Worker typecheck and Vitest: 22 passed.
- Go full suite and race detector passed.
- Rust: 9 tests passed; Clippy passed with warnings denied.
- Wrangler deployment dry-run passed with D1, KV, and both Durable Object bindings.

Remote secret listing, remote D1 migration inspection, migration application, and Worker deployment still require a `CLOUDFLARE_API_TOKEN` in the deployment process environment.
