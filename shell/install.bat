@echo off
powershell "Start-Process powershell -ArgumentList '-NoProfile -ExecutionPolicy Bypass -File ""%~dp0install-watcher.ps1""' -Verb RunAs"
