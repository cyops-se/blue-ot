@echo off
rem Runs build.ps1 without changing the PowerShell execution policy, e.g. build -Test
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1" %*
