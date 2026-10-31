# zsync installer for Windows: downloads the latest release binary into
# %LOCALAPPDATA%\Programs\zsync and adds that folder to the user's PATH.
#
# Usage (PowerShell):
#   irm https://raw.githubusercontent.com/renatormc/zsync/master/install.ps1 | iex
#
# Set $env:ZSYNC_VERSION (e.g. v0.1.0) to install a specific version.

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
# Windows PowerShell 5.1 may default to TLS 1.0, which GitHub rejects.
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$Repo = 'renatormc/zsync'
$InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\zsync'

function Info($Message) { Write-Host '==> ' -ForegroundColor Green -NoNewline; Write-Host $Message }

# PROCESSOR_ARCHITEW6432 is set when running 32-bit PowerShell on a 64-bit system.
$cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($cpu) {
    'AMD64' { 'x86_64' }
    'ARM64' { 'aarch64' }
    default { throw "unsupported architecture: $cpu. Download the binary manually from https://github.com/$Repo/releases" }
}
$target = "$arch-pc-windows-msvc"

$version = $env:ZSYNC_VERSION
if (-not $version) {
    $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ 'User-Agent' = 'zsync-installer' }
    $version = $release.tag_name
    if (-not $version) { throw "no release found at https://github.com/$Repo/releases" }
}

$name = "zsync-$version-$target"
$url = "https://github.com/$Repo/releases/download/$version/$name.zip"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory $tmp | Out-Null
try {
    Info "Downloading zsync $version ($target)"
    $zip = Join-Path $tmp 'zsync.zip'
    try {
        Invoke-WebRequest $url -OutFile $zip -UseBasicParsing
    } catch {
        throw "download failed: $url"
    }
    Expand-Archive $zip -DestinationPath $tmp

    New-Item -ItemType Directory $InstallDir -Force | Out-Null
    Copy-Item (Join-Path $tmp "$name\zsync.exe") $InstallDir -Force
    Info "Installed to $InstallDir\zsync.exe"
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = if ($userPath) { $userPath -split ';' } else { @() }
if ($entries -notcontains $InstallDir) {
    $newPath = (@($entries | Where-Object { $_ }) + $InstallDir) -join ';'
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
    Info "Added $InstallDir to your PATH"
    Info 'Open a new terminal to use zsync'
}
# Also make it available in the current session.
if (($env:Path -split ';') -notcontains $InstallDir) { $env:Path = "$env:Path;$InstallDir" }

if (-not (Get-Command rclone -ErrorAction SilentlyContinue)) {
    Info 'Note: rclone was not found. Install it with `winget install Rclone.Rclone` or from https://rclone.org/install/'
}

Info "Done! Run 'zsync --help' to get started."
