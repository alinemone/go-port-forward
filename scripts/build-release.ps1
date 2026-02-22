param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"

$Commit = (git rev-parse --short HEAD).Trim()
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$Ldflags = "-s -w -X main.Version=$Version -X main.Commit=$Commit -X main.BuildDate=$BuildDate"

New-Item -ItemType Directory -Force -Path dist | Out-Null

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Out = "dist/pf-windows-amd64.exe" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Out = "dist/pf-linux-amd64" },
    @{ GOOS = "linux";   GOARCH = "arm64"; Out = "dist/pf-linux-arm64" },
    @{ GOOS = "darwin";  GOARCH = "amd64"; Out = "dist/pf-darwin-amd64" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Out = "dist/pf-darwin-arm64" }
)

foreach ($t in $targets) {
    Write-Host "Building $($t.Out)..."
    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    $env:CGO_ENABLED = "0"
    go build -trimpath -buildvcs=false -ldflags $Ldflags -o $t.Out .
}

Set-Location dist
Get-FileHash -Algorithm SHA256 * | ForEach-Object {
    "$($_.Hash.ToLower())  $($_.Path | Split-Path -Leaf)"
} | Set-Content SHA256SUMS.txt
Set-Location ..

Write-Host "Release artifacts generated in ./dist"
