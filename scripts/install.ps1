$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\riot-switcher'
$exePath = Join-Path $installDir 'rsw.exe'

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go is not installed or not available on PATH.'
}

New-Item -ItemType Directory -Force -Path $installDir | Out-Null

Push-Location $repoRoot
try {
    go build -trimpath -ldflags='-s -w' -o $exePath .
}
finally {
    Pop-Location
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathEntries = @()
if (-not [string]::IsNullOrWhiteSpace($userPath)) {
    $pathEntries = $userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
}

$alreadyOnPath = $pathEntries | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }
if (-not $alreadyOnPath) {
    $nextPath = (($pathEntries + $installDir) -join ';')
    [Environment]::SetEnvironmentVariable('Path', $nextPath, 'User')
    Write-Host "Added to user PATH: $installDir"
    Write-Host 'Open a new terminal before running rsw globally.'
}

Write-Host "Installed: $exePath"
& $exePath --help
