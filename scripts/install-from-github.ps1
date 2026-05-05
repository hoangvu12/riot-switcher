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
$checksumName = 'checksums.txt'
$apiUrl = if ($Version -eq 'latest') {
    "https://api.github.com/repos/$Repo/releases/latest"
} else {
    "https://api.github.com/repos/$Repo/releases/tags/$Version"
}

Write-Host "Fetching release metadata: $apiUrl"
$release = Invoke-RestMethod -Uri $apiUrl -Headers @{ 'User-Agent' = 'rsw-installer' }
$asset = $release.assets | Where-Object { $_.name -eq $assetName } | Select-Object -First 1
$checksumAsset = $release.assets | Where-Object { $_.name -eq $checksumName } | Select-Object -First 1

if ($null -eq $asset) {
    throw "Release asset not found: $assetName"
}
if ($null -eq $checksumAsset) {
    throw "Release checksum not found: $checksumName"
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$exePath = Join-Path $InstallDir 'rsw.exe'
$tmpPath = Join-Path $env:TEMP $assetName
$checksumPath = Join-Path $env:TEMP $checksumName

function Download-File {
    param(
        [string]$Url,
        [string]$OutFile
    )

    $curl = Get-Command curl.exe -ErrorAction SilentlyContinue
    if ($null -ne $curl) {
        & $curl.Source -fL --retry 3 --connect-timeout 15 --output $OutFile $Url
        if ($LASTEXITCODE -eq 0) {
            return
        }
        Write-Warning "curl.exe failed with exit code $LASTEXITCODE; falling back to Invoke-WebRequest."
    }

    Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $OutFile -Headers @{ 'User-Agent' = 'rsw-installer' }
}

Write-Host "Downloading: $($asset.browser_download_url)"
Download-File -Url $asset.browser_download_url -OutFile $tmpPath

Write-Host "Downloading: $($checksumAsset.browser_download_url)"
Download-File -Url $checksumAsset.browser_download_url -OutFile $checksumPath

$expectedHash = (Get-Content -LiteralPath $checksumPath | Where-Object { $_ -match "\s$([regex]::Escape($assetName))$" } | Select-Object -First 1) -split '\s+' | Select-Object -First 1
if ([string]::IsNullOrWhiteSpace($expectedHash)) {
    throw "Checksum entry not found for $assetName"
}

$actualHash = (Get-FileHash -LiteralPath $tmpPath -Algorithm SHA256).Hash.ToLower()
if ($actualHash -ne $expectedHash.ToLower()) {
    Remove-Item -LiteralPath $tmpPath -Force -ErrorAction SilentlyContinue
    throw "Checksum mismatch for $assetName"
}

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
