# Amber Workspace Instructions

## Node.js Runtime

- Always use the bundled Codex Node.js 24 executable for this repository:
  `C:\Users\Astin\.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe`
- Do not use the system Node.js installation at
  `D:\Setup\Nodejs\nodejs\node.exe`; it is Node.js 18 and has caused build
  failures in this workspace.
- Do not invoke plain `node`, `npm`, or `npx` for project validation or builds.
  Invoke local JavaScript entry points explicitly with the Node.js 24 path.
- Examples:
  - Vitest: `<node24> node_modules\vitest\vitest.mjs run`
  - Type check: `<node24> node_modules\vue-tsc\bin\vue-tsc.js --noEmit`
  - Vite: `<node24> node_modules\vite\bin\vite.js build`
  - Playwright: `<node24> node_modules\@playwright\test\cli.js test`
  - Tauri CLI: `<node24> node_modules\@tauri-apps\cli\tauri.js build`
- Tauri's default `beforeBuildCommand` runs `npm run build`, which can resolve
  to the system Node.js 18 installation. For packaging, use a temporary local
  Tauri config whose `beforeBuildCommand` invokes the Node.js 24 executable
  directly, and remove that temporary config after packaging.

## Installed Application

- Building or packaging must not install, uninstall, stop, restart, or otherwise
  modify the user's installed Amber application. Report the completed installer
  path and checksum only.

## Windows Upgrade Installer

- An intermittent NSIS error opening `sub2api-sidecar.exe` for writing is a
  process-lifecycle race, not a Node.js or compression-mode problem.
- Do not kill only `sub2api-sidecar.exe` before an in-place upgrade. While the
  desktop process remains alive, its supervisor can restart the Sidecar and
  reacquire the executable file lock.
- The installer shutdown order must be: terminate the
  `sub2api-desktop.exe` process tree first, wait for the supervisor to stop,
  terminate any remaining `sub2api-sidecar.exe`, wait until both processes and
  file handles are gone, and only then copy new files.
- Apply the same order to pre-install and pre-uninstall hooks. Keep Tauri's
  built-in running-app check as a secondary guard, not as the primary Sidecar
  shutdown mechanism.
- Every Windows release that changes installer hooks must pass a manual or CI
  in-place upgrade test from the previous installed version without uninstalling
  it first. Packaging agents must not perform that test against the user's live
  installation; report the installer and let an authorized isolated test perform
  the upgrade.
