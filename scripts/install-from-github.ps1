param(
    [string]$Repo = 'hoangvu12/riot-switcher',
    [string]$Version = 'latest',
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\riot-switcher')
)

$ErrorActionPreference = 'Stop'

if ($Repo -notmatch '^[^/]+/[^/]+$') {
    throw 'Repo must be in owner/name form, for example: hoangvu12/riot-switcher'
}

$assetName = 'rsw-windows-amd64.exe'
$apiUrl = if ($Version -eq 'latest') {
    "https://api.github.com/repos/$Repo/releases/latest"
} else {
    "https://api.github.com/repos/$Repo/releases/tags/$Version"
}

Write-Host "Fetching release metadata: $apiUrl"
$release = Invoke-RestMethod -Uri $apiUrl -Headers @{ 'User-Agent' = 'rsw-installer' }
$asset = $release.assets | Where-Object { $_.name -eq $assetName } | Select-Object -First 1

if ($null -eq $asset) {
    throw "Release asset not found: $assetName"
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$exePath = Join-Path $InstallDir 'rsw.exe'
$tmpPath = Join-Path $env:TEMP $assetName

Write-Host "Downloading: $($asset.browser_download_url)"
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $tmpPath -Headers @{ 'User-Agent' = 'rsw-installer' }
Move-Item -LiteralPath $tmpPath -Destination $exePath -Force

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathEntries = @()
if (-not [string]::IsNullOrWhiteSpace($userPath)) {
    $pathEntries = $userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
}

$alreadyOnPath = $pathEntries | Where-Object { $_.TrimEnd('\') -ieq $InstallDir.TrimEnd('\') }
if (-not $alreadyOnPath) {
    $nextPath = (($pathEntries + $InstallDir) -join ';')
    [Environment]::SetEnvironmentVariable('Path', $nextPath, 'User')
    Write-Host "Added to user PATH: $InstallDir"
    Write-Host 'Open a new terminal before running rsw globally.'
}

Write-Host "Installed: $exePath"
& $exePath --help
