@echo off
rem Run the Go race detector for Nexa backend (Docker-based; see race-detector.ps1)
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0race-detector.ps1" %*
