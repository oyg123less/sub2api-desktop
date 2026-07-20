param([string]$Expected = "")

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$package = Get-Content -Raw -LiteralPath (Join-Path $repo 'package.json') | ConvertFrom-Json
$cloudPackage = Get-Content -Raw -LiteralPath (Join-Path $repo 'cloud\package.json') | ConvertFrom-Json
$tauri = Get-Content -Raw -LiteralPath (Join-Path $repo 'src-tauri\tauri.conf.json') | ConvertFrom-Json
$cargo = Get-Content -Raw -LiteralPath (Join-Path $repo 'src-tauri\Cargo.toml')
$cargoVersion = [regex]::Match($cargo, '(?ms)^\[package\].*?^version\s*=\s*"([^"]+)"').Groups[1].Value
$lockPath = Join-Path $repo 'package-lock.json'
$cloudLockPath = Join-Path $repo 'cloud\package-lock.json'
$bundledNode24 = Join-Path $env:USERPROFILE '.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
if ($env:AMBER_NODE24) {
    $node24 = $env:AMBER_NODE24
} elseif (Test-Path -LiteralPath $bundledNode24) {
    $node24 = $bundledNode24
} else {
    $nodeCommand = Get-Command node -ErrorAction SilentlyContinue
    if (-not $nodeCommand) { throw 'Node.js 24 runtime not found' }
    $node24 = $nodeCommand.Source
}
$nodeVersion = (& $node24 --version).Trim()
if ($LASTEXITCODE -ne 0 -or $nodeVersion -notmatch '^v24\.') {
    throw "Node.js 24 is required, found '$nodeVersion' at '$node24'"
}
$lockVersions = & $node24 -e "const p=JSON.parse(require('fs').readFileSync(process.argv[1],'utf8')); console.log(p.version); console.log(p.packages[''].version)" $lockPath
if ($LASTEXITCODE -ne 0 -or $lockVersions.Count -lt 2) { throw 'Unable to read package-lock.json versions' }
$cloudLockVersions = & $node24 -e "const p=JSON.parse(require('fs').readFileSync(process.argv[1],'utf8')); console.log(p.version); console.log(p.packages[''].version)" $cloudLockPath
if ($LASTEXITCODE -ne 0 -or $cloudLockVersions.Count -lt 2) { throw 'Unable to read cloud/package-lock.json versions' }

$versions = [ordered]@{
    package_json = [string]$package.version
    package_lock = [string]$lockVersions[0]
    package_lock_root = [string]$lockVersions[1]
    cloud_package_json = [string]$cloudPackage.version
    cloud_package_lock = [string]$cloudLockVersions[0]
    cloud_package_lock_root = [string]$cloudLockVersions[1]
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
