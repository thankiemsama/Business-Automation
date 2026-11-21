<#
.SYNOPSIS
    Runs the Go race detector for the Nexa backend inside a Linux Docker
    container. The race detector is a Linux/macOS-only tool, so this script
    provides a working path on a Windows host that has Docker.

.DESCRIPTION
    - Builds the module dependencies first (no race) to keep the image light.
    - Runs `go test -race ./... -count=1` with CGO_ENABLED=1.
    - Mounts the repo read-only; the Go build cache is a container volume.
    - Fails the script (exit code 1) if the race detector reports anything.

.PARAMETER NoDocker
    Assume you are already on a Linux host and just run `go test -race` directly.

.EXAMPLE
    .\scripts\race-detector.ps1
    .\scripts\race-detector.ps1 -NoDocker
#>
[CmdletBinding()]
param(
    [switch]$NoDocker
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$backend  = Join-Path $repoRoot "BACKEND"
$image    = "golang:1.24-bookworm"

if ($NoDocker) {
    Write-Host "Running race detector directly (Linux host assumed)..." -ForegroundColor Cyan
    Push-Location $backend
    try {
        $env:CGO_ENABLED = "1"
        go test -race ./... -count=1
        if ($LASTEXITCODE -ne 0) { throw "race detector reported issues (exit $LASTEXITCODE)" }
        Write-Host "No race conditions detected." -ForegroundColor Green
    } finally {
        Pop-Location
    }
    return
}

# Verify Docker is available
$docker = Get-Command docker -ErrorAction SilentlyContinue
if (-not $docker) {
    Write-Warning "Docker not found on PATH."
    Write-Warning "Either install Docker Desktop, or run this script on a Linux CI host with -NoDocker."
    exit 1
}

Write-Host "Pulling $image (first run may take a moment)..." -ForegroundColor Cyan
docker pull $image 2>&1 | Out-Null

Write-Host "Running go test -race inside $image ..." -ForegroundColor Cyan
Push-Location $backend
try {
    $containerName = "nexa-race-$(Get-Random)"
    $cmd = @(
        "docker", "run", "--rm",
        "--name", $containerName,
        "-v", "${backend}:/src:ro",
        "-v", "go-cache:/go/pkg/mod",
        "-w", "/src",
        "-e", "CGO_ENABLED=1",
        "-e", "GOFLAGS=-mod=mod",
        $image,
        "sh", "-c", "go test -race ./... -count=1"
    )
    & $cmd
    $code = $LASTEXITCODE
    if ($code -ne 0) {
        Write-Host "Race detector reported problems (exit $code). See output above." -ForegroundColor Red
    } else {
        Write-Host "All tests passed under the Go race detector." -ForegroundColor Green
    }
    exit $code
} finally {
    Pop-Location
}
