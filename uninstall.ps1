$ErrorActionPreference = 'Stop'
$target = Join-Path $env:LOCALAPPDATA 'Programs\Fetch'
# Remove only W1F3-owned program files; preserve config and logo cache.
@('w1f3.exe', 'w1f3.cmd', 'fetch.cmd', 'fetch.exe', 'fetch.py', 'README.md') | ForEach-Object {
    $item = Join-Path $target $_
    if (Test-Path $item) { Remove-Item $item -Force }
}
if (Test-Path $target) {
    $remaining = Get-ChildItem -Force $target
    if (-not $remaining) { Remove-Item $target -Force }
}
Write-Host 'W1F3 program files removed. Your ~/.config/fetch configuration and logo cache were kept.'
Write-Host 'The legacy `fetch` command alias has also been removed from the W1F3 installation.'
