$ErrorActionPreference = 'Stop'
$target = Join-Path $env:LOCALAPPDATA 'Programs\Fetch'
# Remove only files owned by Fetch; do not delete unrelated files in this directory.
@('fetch.exe', 'fetch.cmd', 'fetch.py', 'README.md') | ForEach-Object {
    $item = Join-Path $target $_
    if (Test-Path $item) { Remove-Item $item -Force }
}
if (Test-Path $target) {
    $remaining = Get-ChildItem -Force $target
    if (-not $remaining) { Remove-Item $target -Force }
}
Write-Host 'Fetch program files removed. Your ~/.config/fetch configuration and logo cache were kept.'
Write-Host 'If another fetch command is installed elsewhere, it is not modified.'
