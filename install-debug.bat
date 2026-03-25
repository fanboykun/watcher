@echo off
setlocal

set "SCRIPT=%~dp0shell\install-watcher.ps1"

if not exist "%SCRIPT%" (
  echo Installer script not found: "%SCRIPT%"
  pause
  exit /b 1
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
  "Start-Process powershell.exe -Verb RunAs -ArgumentList @('-NoExit','-NoProfile','-ExecutionPolicy','Bypass','-STA','-File','\"%SCRIPT%\"','-DebugMode')"

endlocal
