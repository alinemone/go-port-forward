param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $RepoRoot

$Commit = (git rev-parse --short HEAD).Trim()
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$Ldflags = "-s -w -X main.Version=$Version -X main.Commit=$Commit -X main.BuildDate=$BuildDate"

New-Item -ItemType Directory -Force -Path dist | Out-Null

$WindowsIcon = Join-Path $RepoRoot "assets\app.ico"
$WindowsSyso = Join-Path $RepoRoot "rsrc_windows_amd64.syso"
$GeneratedWindowsSyso = $false

if (Test-Path $WindowsIcon) {
    $Rsrc = Get-Command rsrc -ErrorAction SilentlyContinue
    if (-not $Rsrc) {
        throw "Icon file found at assets/app.ico but 'rsrc' is not installed. Run: go install github.com/akavel/rsrc@latest"
    }

    Write-Host "Generating Windows icon resource from assets/app.ico..."
    & $Rsrc.Source -ico $WindowsIcon -arch amd64 -o $WindowsSyso
    $GeneratedWindowsSyso = $true
}

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Out = "dist/pf-windows-amd64.exe" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Out = "dist/pf-linux-amd64" },
    @{ GOOS = "linux";   GOARCH = "arm64"; Out = "dist/pf-linux-arm64" },
    @{ GOOS = "darwin";  GOARCH = "amd64"; Out = "dist/pf-darwin-amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Out = "dist/pf-darwin-arm64" }
)

try {
    foreach ($t in $targets) {
        Write-Host "Building $($t.Out)..."
        $env:GOOS = $t.GOOS
        $env:GOARCH = $t.GOARCH
        $env:CGO_ENABLED = "0"
        go build -trimpath -buildvcs=false -ldflags $Ldflags -o $t.Out .
    }
}
finally {
    if ($GeneratedWindowsSyso -and (Test-Path $WindowsSyso)) {
        Remove-Item $WindowsSyso -Force
    }
}

Set-Location dist
Get-ChildItem -File | Where-Object { $_.Name -ne "SHA256SUMS.txt" } | Get-FileHash -Algorithm SHA256 | ForEach-Object {
    "$($_.Hash.ToLower())  $($_.Path | Split-Path -Leaf)"
} | Set-Content SHA256SUMS.txt
Set-Location ..

Write-Host "Release artifacts generated in ./dist"
