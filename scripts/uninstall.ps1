$ErrorActionPreference = 'Stop'

$installDir = Join-Path $env:LOCALAPPDATA 'Programs\riot-switcher'
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')

if (-not [string]::IsNullOrWhiteSpace($userPath)) {
    $pathEntries = $userPath -split ';' | Where-Object {
        -not [string]::IsNullOrWhiteSpace($_) -and $_.TrimEnd('\') -ine $installDir.TrimEnd('\')
    }
    [Environment]::SetEnvironmentVariable('Path', ($pathEntries -join ';'), 'User')
}

Remove-Item -LiteralPath $installDir -Recurse -Force -ErrorAction SilentlyContinue
Write-Host "Uninstalled: $installDir"
