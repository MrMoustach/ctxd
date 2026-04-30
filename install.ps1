$ErrorActionPreference = "Stop"

$Repo = "MrMoustach/ctxd"
$BinName = "ctxd.exe"
$InstallDir = if ($env:CTXD_INSTALL_DIR) { $env:CTXD_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "ctxd\bin" }

function Fail($Message) {
    Write-Error "ctxd install: $Message"
    exit 1
}

function Get-Arch {
    switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()) {
        "x64" { "x86_64"; break }
        "arm64" { "arm64"; break }
        default { Fail "unsupported architecture: $([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture)" }
    }
}

function Get-LatestTag {
    if ($env:CTXD_VERSION) {
        return $env:CTXD_VERSION
    }
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "ctxd-installer" }
    return $release.tag_name
}

function Add-UserPath($PathToAdd) {
    $current = [Environment]::GetEnvironmentVariable("Path", "User")
    $parts = @()
    if ($current) {
        $parts = $current -split ";"
    }
    if ($parts -notcontains $PathToAdd) {
        $next = if ($current) { "$current;$PathToAdd" } else { $PathToAdd }
        [Environment]::SetEnvironmentVariable("Path", $next, "User")
        $env:Path = "$env:Path;$PathToAdd"
        Write-Host "Added $PathToAdd to the user PATH. Open a new terminal for other shells to see it."
    }
}

$Arch = Get-Arch
$Tag = Get-LatestTag
if (-not $Tag) {
    Fail "could not resolve latest release tag"
}

$Asset = "ctxd_Windows_$Arch.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/$Tag"
$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("ctxd-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    $Archive = Join-Path $TempDir $Asset
    $Checksums = Join-Path $TempDir "checksums.txt"

    Write-Host "Downloading $Asset from $Repo $Tag"
    Invoke-WebRequest -Uri "$BaseUrl/$Asset" -OutFile $Archive
    Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $Checksums

    $expectedLine = Get-Content $Checksums | Where-Object { $_ -match "\s+$([Regex]::Escape($Asset))$" } | Select-Object -First 1
    if (-not $expectedLine) {
        Fail "checksum not found for $Asset"
    }
    $expected = ($expectedLine -split "\s+")[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 $Archive).Hash.ToLowerInvariant()
    if ($expected -ne $actual) {
        Fail "checksum mismatch for $Asset"
    }

    Expand-Archive -Path $Archive -DestinationPath $TempDir -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $TempDir $BinName) -Destination (Join-Path $InstallDir $BinName) -Force
    Add-UserPath $InstallDir

    Write-Host "Installed ctxd to $(Join-Path $InstallDir $BinName)"
    Write-Host "Run: ctxd doctor"
}
finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
