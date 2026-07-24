# Amber v0.4.5 Release Notes

## Release Summary

Amber v0.4.5 is a reliability release for Windows in-place upgrades and Codex
client compatibility across local gateway, direct cloud sharing, and owner
device relay paths.

## User-Facing Changes

- The Windows installer stops the desktop supervisor before the Sidecar, waits
  for both processes to exit, and verifies that the Sidecar executable is
  writable before replacing files.
- Official Codex `User-Agent`, `originator`, `OpenAI-Beta`, and `session_id`
  values are preserved across local forwarding, account failover, Cloudflare
  direct sharing, and Owner Relay.
- Non-Codex clients continue to use Amber's current compatibility fallback.
- Existing custom User-Agent settings are preserved. Only the legacy Amber
  default is migrated to the current Codex fallback.
- Both NSIS EXE and MSI packages are generated for Windows x64.

## Compatibility And Migration

- Local database schema: `17`.
- Cloud latest-client migration: `0011_v045_latest_client.sql`.
- `minimum_client_version` remains unchanged during rollout; the migration only
  updates `latest_client_version` to `0.4.5`.
- Existing accounts, proxies, cloud workspaces, sharing grants, and Codex
  targets are not re-owned or merged by this release.

## Release Gate

- Go formatting, vet, full race tests, and gateway/cloud/store tests pass.
- Vue typecheck, production build, Vitest, and all 72 Playwright tests pass on
  Node.js 24.
- Worker typecheck and all 57 Worker tests pass.
- Rust formatting, Clippy with warnings denied, and all 12 Rust tests pass.
- The NSIS hook compiles in the production Tauri bundle.
- GitHub Release runs an isolated v0.4.4 to v0.4.5 in-place upgrade test. The
  user's installed Amber is never used for release validation.

## Build Artifacts

- `Amber_0.4.5_x64-setup.exe`
  - Size: `8,008,724` bytes
  - SHA-256: `81C2B1DB460317DFD3A35F9E06B364F753DE3F82377D0760506CEAAEE5D14528`
- `Amber_0.4.5_x64_en-US.msi`
  - Size: `10,207,232` bytes
  - SHA-256: `C37DA7475C8EB33B5E341962F96FF21D7F6FB10785A9881F9522081D21D67107`

## Deployment Order

1. Commit and tag the verified source as `v0.4.5`.
2. Publish both installers and `SHA256SUMS.txt` in GitHub Releases.
3. Apply D1 migration `0011_v045_latest_client.sql`.
4. Deploy the v0.4.5 Worker and confirm `/health` reports `0.4.5`.
5. Deploy the website with the v0.4.5 download metadata.
