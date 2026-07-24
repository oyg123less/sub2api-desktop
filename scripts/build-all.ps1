# Full release build: Go sidecar + Tauri bundle (NSIS / MSI).
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$bundledNode24 = Join-Path $env:USERPROFILE '.cache\codex-runtimes\codex-primary-runtime\dependencies\node\bin\node.exe'
$node24 = if ($env:AMBER_NODE24) { $env:AMBER_NODE24 } else { $bundledNode24 }
if (-not (Test-Path -LiteralPath $node24)) { throw "Node.js 24 runtime not found at '$node24'." }
$nodeVersion = (& $node24 --version).Trim()
if ($nodeVersion -notmatch '^v24\.') { throw "Node.js 24 is required, found '$nodeVersion'." }

& (Join-Path $PSScriptRoot 'build-sidecar.ps1')
if ($LASTEXITCODE -ne 0) { exit 1 }

$temporaryConfig = Join-Path $repo '.tauri.release.conf.json'
$beforeBuild = "$node24 node_modules\vue-tsc\bin\vue-tsc.js --noEmit && $node24 node_modules\vite\bin\vite.js build"
@{
    build = @{ beforeBuildCommand = $beforeBuild }
} | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $temporaryConfig -Encoding utf8

try {
    Set-Location $repo
    & $node24 node_modules\@tauri-apps\cli\tauri.js build --config $temporaryConfig
    if ($LASTEXITCODE -ne 0) { throw 'TAURI BUILD FAILED' }
}
finally {
    Remove-Item -LiteralPath $temporaryConfig -Force -ErrorAction SilentlyContinue
}

Write-Output 'ALL DONE'
Write-Output "Installers: $repo\src-tauri\target\release\bundle\nsis\ and ...\msi\"
