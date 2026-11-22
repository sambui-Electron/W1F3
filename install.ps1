$ErrorActionPreference = 'Stop'

# Upgrade in place: v0.1 (Python) and v0.2 (Go) intentionally share this path.
$source = Split-Path -Parent $MyInvocation.MyCommand.Path
$target = Join-Path $env:LOCALAPPDATA 'Programs\Fetch'
$configDir = Join-Path $HOME '.config\fetch'
$logoDir = Join-Path $configDir 'logos'

$sourceExe = Join-Path $source 'fetch.exe'
$sourceCmd = Join-Path $source 'fetch.cmd'
if (-not (Test-Path $sourceExe)) {
    throw "fetch.exe is missing from $source. Extract the complete release ZIP before installing."
}
if (-not (Test-Path $sourceCmd)) {
    throw "fetch.cmd is missing from $source. Extract the complete release ZIP before installing."
}

New-Item -ItemType Directory -Force -Path $target, $configDir, $logoDir | Out-Null

# Install the new binary and launcher over the previous Python launcher.
Copy-Item $sourceExe (Join-Path $target 'fetch.exe') -Force
Copy-Item $sourceCmd (Join-Path $target 'fetch.cmd') -Force
if (Test-Path (Join-Path $source 'README.md')) {
    Copy-Item (Join-Path $source 'README.md') (Join-Path $target 'README.md') -Force
}

# Remove the stale v0.1 Python entry point only after the Go executable is in place.
$oldPythonEntry = Join-Path $target 'fetch.py'
if (Test-Path $oldPythonEntry) {
    Remove-Item $oldPythonEntry -Force
}

# Preserve the user's existing config. Fetch downloads the canonical default config
# from the W1F3 GitHub repository on first launch when this file is absent.
$configFile = Join-Path $configDir 'config.yaml'
$schemaSource = Join-Path $source 'config.schema.json'
$schemaTarget = Join-Path $configDir 'config.schema.json'
if ((Test-Path $schemaSource) -and -not (Test-Path $schemaTarget)) {
    Copy-Item $schemaSource $schemaTarget
}

# Ensure the canonical install directory is on the user PATH, without duplicate entries.
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathEntries = @()
if (-not [string]::IsNullOrWhiteSpace($userPath)) {
    $pathEntries = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}
# Remove any existing copies, then put the canonical install directory first.
$pathEntries = @($pathEntries | Where-Object {
    -not [string]::Equals($_.TrimEnd('\'), $target.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)
})
$pathEntries = @($target) + $pathEntries
[Environment]::SetEnvironmentVariable('Path', ($pathEntries -join ';'), 'User')

Write-Host ''
Write-Host 'W1F3 was installed/upgraded in place.' -ForegroundColor Green
Write-Host "Executable:   $(Join-Path $target 'fetch.exe')"
Write-Host "Launcher:     $(Join-Path $target 'fetch.cmd')"
Write-Host "Configuration: $configFile"
Write-Host "Logo cache:    $logoDir"
Write-Host "Config schema: $schemaTarget"
Write-Host 'The previous fetch.py launcher has been removed; existing config and logo cache were preserved.'
Write-Host 'If this is a first install, Fetch downloads its default config from the W1F3 GitHub repository on first launch.'

$resolved = Get-Command fetch -All -ErrorAction SilentlyContinue
if ($resolved) {
    Write-Host ''
    Write-Host 'Fetch commands currently visible to this PowerShell session:' -ForegroundColor Yellow
    $resolved | ForEach-Object { Write-Host "  $($_.Source)" }
    $first = $resolved | Select-Object -First 1
    if ($first.Source -and -not $first.Source.StartsWith($target, [StringComparison]::OrdinalIgnoreCase)) {
        Write-Warning "Another fetch command takes precedence: $($first.Source). Remove or reorder that other PATH entry if you want this installation to run first."
    }
} else {
    Write-Host 'Open a new terminal so the updated user PATH is loaded, then run: fetch'
}

if (-not (Get-Command fastfetch -ErrorAction SilentlyContinue)) {
    Write-Warning 'Fastfetch was not found on PATH. Add it to PATH or set fastfetch_path in your config.yaml.'
}
