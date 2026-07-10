param([string]$Expected = "")

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$package = Get-Content -Raw -LiteralPath (Join-Path $repo 'package.json') | ConvertFrom-Json
$tauri = Get-Content -Raw -LiteralPath (Join-Path $repo 'src-tauri\tauri.conf.json') | ConvertFrom-Json
$cargo = Get-Content -Raw -LiteralPath (Join-Path $repo 'src-tauri\Cargo.toml')
$cargoVersion = [regex]::Match($cargo, '(?ms)^\[package\].*?^version\s*=\s*"([^"]+)"').Groups[1].Value
$lockPath = Join-Path $repo 'package-lock.json'
$lockVersions = & node -e "const p=JSON.parse(require('fs').readFileSync(process.argv[1],'utf8')); console.log(p.version); console.log(p.packages[''].version)" $lockPath
if ($LASTEXITCODE -ne 0 -or $lockVersions.Count -lt 2) { throw 'Unable to read package-lock.json versions' }

$versions = [ordered]@{
    package_json = [string]$package.version
    package_lock = [string]$lockVersions[0]
    package_lock_root = [string]$lockVersions[1]
    tauri = [string]$tauri.version
    cargo = $cargoVersion
}

$baseline = $versions.package_json
foreach ($entry in $versions.GetEnumerator()) {
    if (-not $entry.Value -or $entry.Value -ne $baseline) {
        throw "Version mismatch: $($entry.Key)='$($entry.Value)', package.json='$baseline'"
    }
}
if ($Expected) {
    $normalized = $Expected.TrimStart('v')
    if ($normalized -ne $baseline) {
        throw "Expected version '$normalized', repository version is '$baseline'"
    }
}
Write-Output "VERSION OK $baseline"
