# Full release build: Go sidecar + Tauri bundle (NSIS / MSI).
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot

& (Join-Path $PSScriptRoot 'build-sidecar.ps1')
if ($LASTEXITCODE -ne 0) { exit 1 }

Set-Location $repo
npm run tauri build
if ($LASTEXITCODE -ne 0) { Write-Error 'TAURI BUILD FAILED'; exit 1 }
Write-Output 'ALL DONE'
Write-Output "Installers: $repo\src-tauri\target\release\bundle\nsis\ and ...\msi\"
