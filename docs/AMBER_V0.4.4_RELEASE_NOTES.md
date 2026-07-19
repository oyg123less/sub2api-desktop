# Amber v0.4.4 Release Notes

## Release Summary

Amber v0.4.4 focuses on local data isolation, deterministic cloud sharing,
network reliability, and a complete Codex injection flow. It is a data-model
upgrade from v0.4.3 and should be distributed as one final Windows installer.

## User-Facing Changes

- Each cloud user now has an isolated local workspace. Accounts, proxies,
  settings, sync queues, tombstones, conflicts, and Codex targets do not leak
  between users on the same computer.
- Ambiguous legacy databases open in a read-only recovery workspace. Amber does
  not guess ownership or automatically upload uncertain data.
- Cloud sharing no longer requires friends. The primary flow is now a
  nine-digit connection code plus a six-character temporary password.
- Owners explicitly select one primary relay device and up to two backup
  devices. Requests are never replayed on another device after upstream work
  has started.
- Accounts have explicit `direct`, `system`, and `proxy` network modes. A direct
  account cannot accidentally inherit a process or system proxy.
- Codex injection now starts the local service, validates the Amber instance,
  local API key, and model list, writes the configuration, and reads it back
  before reporting success.
- Amber Cloud uses `https://api.amberapp.asia` as the preferred endpoint and the
  Workers domain as a fallback for idempotent requests.
- Moving the data directory now migrates and validates the complete workspace
  tree instead of copying only the current database.

## Compatibility And Migration

- Local database schema: `16`.
- Cloud routing migration: `0009_v044_device_routing.sql`.
- Cloud latest-client migration: `0010_v044_latest_client.sql`.
- Existing v0.4.3 files are preserved during migration and are not merged back
  during rollback.
- Historical friend and share-group tables remain server-side for compatibility,
  but the v0.4.4 primary interface does not expose the friend workflow.
- `minimum_client_version` remains `0.4.3` during Worker deployment. It may be
  raised to `0.4.4` only after the final installer and GitHub Release are
  publicly downloadable.

## Release Gate

- Go tests and race tests pass.
- Vue typecheck, Vitest, Playwright E2E, and production build pass on Node.js 24.
- Worker typecheck and the full Worker test suite pass.
- Rust tests pass.
- One NSIS installer is generated and its SHA-256 is recorded.
- The installer is not installed or tested against the user's live Amber
  installation. In-place upgrade validation must run in an isolated environment.
- No production secret is stored in source code, documentation, build logs, or
  Git history.

## Build Artifact

- File: `Amber_0.4.4_x64-setup.exe`
- Size: `8,005,866` bytes
- SHA-256: `BA956575A2F326ECF7D29F42CE48938C42A927DB9805BCB30018347FFBFBE6FD`

## Post-Release Operations

1. Publish the final v0.4.4 installer and checksum.
2. Confirm the Release asset is downloadable.
3. Deploy Worker migrations and code.
4. Set `minimum_client_version` to `0.4.4` only after steps 1-3 succeed.
5. Update the public website download metadata from its separate website task.
