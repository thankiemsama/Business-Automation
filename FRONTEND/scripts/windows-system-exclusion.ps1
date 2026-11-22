# One-time setup: stop Windows Defender real-time scanning from locking the
# project's build cache and node_modules.
#
# Why this is the root cause of the EPERM crash:
#   Windows Defender real-time protection opens a brief lock on freshly written
#   .next/dev chunk files. When next dev then unlinks/rewrites those files a
#   moment later, the call fails with "EPERM: unlink". Adding the cache paths
#   to Defender exclusions removes the lock for good. Exclusions are by path,
#   so they keep working after .next is deleted and recreated.
#
# Run ONCE from an elevated (Administrator) terminal:
#   cd D:\GOLANG\FRONTEND
#   powershell -ExecutionPolicy Bypass -File scripts\windows-system-exclusion.ps1
#
# Idempotent: re-running only adds any missing exclusions.

$ErrorActionPreference = "Stop"

# --- 1) Require administrator -------------------------------------------------
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
if (-not (New-Object Security.Principal.WindowsPrincipal $identity).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "Administrator privileges are required." -ForegroundColor Yellow
    Write-Host "Right-click PowerShell -> 'Run as administrator', then run this script again."
    exit 1
}

# --- 2) Defender must be present ---------------------------------------------
if (-not (Get-Command Get-MpComputerStatus -ErrorAction SilentlyContinue)) {
    Write-Warning "Windows Defender (Defender) AV service not found; skipping Defender exclusions."
}
else {
    # script lives in <project>/scripts -> project root is one level up
    $projectRoot = Split-Path -Parent $PSScriptRoot
    $paths = @(
        (Join-Path $projectRoot ".next")
        (Join-Path $projectRoot "node_modules")
    )

    $existing = (Get-MpPreference).ExclusionPath
    foreach ($path in $paths) {
        if ($existing -notcontains $path) {
            Add-MpPreference -ExclusionPath $path
            Write-Host "Added Defender exclusion: $path" -ForegroundColor Green
        }
        else {
            Write-Host "Already present: $path" -ForegroundColor DarkGray
        }
    }

    Write-Host ""
    Write-Host "Current Defender path exclusions:"
    (Get-MpPreference).ExclusionPath | ForEach-Object { Write-Host "  $_" }
}

Write-Host ""
Write-Host "Done. Re-run 'npm run dev' to verify the EPERM error no longer appears." -ForegroundColor Cyan
