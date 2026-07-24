param(
    [Parameter(Mandatory = $true)]
    [string]$CurrentInstaller,
    [string]$PreviousVersion = '0.4.4'
)

$ErrorActionPreference = 'Stop'
$currentInstallerPath = (Resolve-Path -LiteralPath $CurrentInstaller).Path
$previousInstallerName = "Amber_${PreviousVersion}_x64-setup.exe"
$previousInstallerPath = Join-Path $env:RUNNER_TEMP $previousInstallerName
$previousInstallerURL = "https://github.com/oyg123less/sub2api-desktop/releases/download/v${PreviousVersion}/${previousInstallerName}"

function Find-AmberDesktop {
    $candidates = @(
        (Join-Path $env:LOCALAPPDATA 'Amber\sub2api-desktop.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\Amber\sub2api-desktop.exe')
    )
    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }
    $found = Get-ChildItem -LiteralPath $env:LOCALAPPDATA -Filter 'sub2api-desktop.exe' -File -Recurse -ErrorAction SilentlyContinue |
        Select-Object -First 1 -ExpandProperty FullName
    if (-not $found) { throw 'Unable to locate the installed Amber desktop executable.' }
    return $found
}

function Run-Installer([string]$path) {
    $process = Start-Process -FilePath $path -ArgumentList '/S' -PassThru -Wait -WindowStyle Hidden
    if ($process.ExitCode -ne 0) {
        throw "Installer '$path' exited with code $($process.ExitCode)."
    }
}

Invoke-WebRequest -Uri $previousInstallerURL -OutFile $previousInstallerPath
Run-Installer $previousInstallerPath
$desktopPath = Find-AmberDesktop

$desktop = Start-Process -FilePath $desktopPath -PassThru -WindowStyle Hidden
try {
    $sidecarReady = $false
    foreach ($attempt in 1..50) {
        Start-Sleep -Milliseconds 200
        if (Get-Process -Name 'sub2api-sidecar' -ErrorAction SilentlyContinue) {
            $sidecarReady = $true
            break
        }
    }
    if (-not $sidecarReady) { throw 'The previous Amber release did not start its sidecar.' }

    Run-Installer $currentInstallerPath
    $expectedVersion = [version]((Get-Item -LiteralPath $currentInstallerPath).VersionInfo.ProductVersion)
    $actualVersion = [version]((Get-Item -LiteralPath $desktopPath).VersionInfo.ProductVersion)
    if ($actualVersion -ne $expectedVersion) {
        throw "Installed desktop version '$actualVersion' != installer version '$expectedVersion'."
    }
    if (Get-Process -Name 'sub2api-desktop','sub2api-sidecar' -ErrorAction SilentlyContinue) {
        throw 'Amber processes remained alive after the in-place upgrade completed.'
    }
    Write-Output "IN-PLACE UPGRADE OK v$PreviousVersion -> v$expectedVersion"
}
finally {
    Stop-Process -Name 'sub2api-desktop','sub2api-sidecar' -Force -ErrorAction SilentlyContinue
    if ($desktop -and -not $desktop.HasExited) {
        Stop-Process -Id $desktop.Id -Force -ErrorAction SilentlyContinue
    }
}
