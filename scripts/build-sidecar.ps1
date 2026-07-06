# Build the Go sidecar into src-tauri/binaries (required before `npm run tauri build`).
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot

Set-Location (Join-Path $repo 'core')
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -o (Join-Path $repo 'src-tauri\binaries\sub2api-sidecar-x86_64-pc-windows-msvc.exe') .\cmd\sidecar
if ($LASTEXITCODE -ne 0) { Write-Error 'SIDECAR BUILD FAILED'; exit 1 }
Write-Output 'SIDECAR OK'
