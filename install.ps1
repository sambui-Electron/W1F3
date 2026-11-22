$ErrorActionPreference = 'Stop'

# Keep the historical install/config directories to make this an in-place upgrade.
$source = Split-Path -Parent $MyInvocation.MyCommand.Path
$target = Join-Path $env:LOCALAPPDATA 'Programs\Fetch'
$configDir = Join-Path $HOME '.config\fetch'
$logoDir = Join-Path $configDir 'logos'

$required = @('w1f3.exe', 'w1f3.cmd', 'fetch.cmd')
foreach ($name in $required) {
    if (-not (Test-Path (Join-Path $source $name))) {
        throw "$name is missing from $source. Extract the complete W1F3 release ZIP before installing."
    }
}

New-Item -ItemType Directory -Force -Path $target, $configDir, $logoDir | Out-Null
Copy-Item (Join-Path $source 'w1f3.exe') (Join-Path $target 'w1f3.exe') -Force
Copy-Item (Join-Path $source 'w1f3.cmd') (Join-Path $target 'w1f3.cmd') -Force
Copy-Item (Join-Path $source 'fetch.cmd') (Join-Path $target 'fetch.cmd') -Force
if (Test-Path (Join-Path $source 'README.md')) {
    Copy-Item (Join-Path $source 'README.md') (Join-Path $target 'README.md') -Force
}

# Remove old binary/launcher names so there is only one W1F3 executable.
@('fetch.exe', 'fetch.py') | ForEach-Object {
    $old = Join-Path $target $_
    if (Test-Path $old) { Remove-Item $old -Force }
}

# Preserve user config and logos. Install schema only when not already present.
$schemaSource = Join-Path $source 'config.schema.json'
$schemaTarget = Join-Path $configDir 'config.schema.json'
if ((Test-Path $schemaSource) -and -not (Test-Path $schemaTarget)) {
    Copy-Item $schemaSource $schemaTarget
}

# Keep the existing install directory on PATH for upgrade compatibility.
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathEntries = @()
if (-not [string]::IsNullOrWhiteSpace($userPath)) {
    $pathEntries = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}
$pathEntries = @($pathEntries | Where-Object {
    -not [string]::Equals($_.TrimEnd('\\'), $target.TrimEnd('\\'), [StringComparison]::OrdinalIgnoreCase)
})
$pathEntries = @($target) + $pathEntries
[Environment]::SetEnvironmentVariable('Path', ($pathEntries -join ';'), 'User')

Write-Host ''
Write-Host 'W1F3 v0.5.1 installed/upgraded successfully.' -ForegroundColor Green
Write-Host "Executable:    $(Join-Path $target 'w1f3.exe')"
Write-Host "Primary command: W1F3"
Write-Host "Legacy alias:   fetch"
Write-Host "Configuration:  $(Join-Path $configDir 'config.yaml')"
Write-Host "Logo cache:     $logoDir"
Write-Host "Config schema:  $schemaTarget"
Write-Host 'Existing configuration and logo cache were preserved.'
Write-Host 'Open a new terminal, then run: W1F3 -v'

$resolved = Get-Command W1F3 -All -ErrorAction SilentlyContinue
if ($resolved) {
    $first = $resolved | Select-Object -First 1
    if ($first.Source -and -not $first.Source.StartsWith($target, [StringComparison]::OrdinalIgnoreCase)) {
        Write-Warning "Another W1F3 command takes precedence: $($first.Source)"
    }
}
if (-not (Get-Command fastfetch -ErrorAction SilentlyContinue)) {
    Write-Warning 'Fastfetch was not found on PATH. Add it to PATH or set fastfetch_path in config.yaml.'
}
