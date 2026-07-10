# Build the Go sidecar into src-tauri/binaries (required before `npm run tauri build`).
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$package = Get-Content -Raw -LiteralPath (Join-Path $repo 'package.json') | ConvertFrom-Json
$version = $package.version
if (-not $version) { throw 'Unable to read version from package.json' }

Push-Location (Join-Path $repo 'core')
try {
    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o (Join-Path $repo 'src-tauri\binaries\sub2api-sidecar-x86_64-pc-windows-msvc.exe') .\cmd\sidecar
    if ($LASTEXITCODE -ne 0) { Write-Error 'SIDECAR BUILD FAILED'; exit 1 }
}
finally {
    Pop-Location
}
Write-Output "SIDECAR OK v$version"
