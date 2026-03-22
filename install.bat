@echo off
powershell "Start-Process powershell -ArgumentList '-NoProfile -ExecutionPolicy Bypass -File ""%~dp0shell\install-watcher.ps1""' -Verb RunAs"
