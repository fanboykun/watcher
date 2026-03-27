@echo off
setlocal

set "SRC_ROOT=%~dp0"
set "STAGING=%TEMP%\watcher-installer-%RANDOM%%RANDOM%"
set "SCRIPT=%STAGING%\shell\install-watcher.ps1"
set "WATCHER_EXE=%STAGING%\watcher.exe"

mkdir "%STAGING%\shell" >nul 2>nul

copy /Y "%SRC_ROOT%shell\install-watcher.ps1" "%SCRIPT%" >nul
copy /Y "%SRC_ROOT%watcher.exe" "%WATCHER_EXE%" >nul

if not exist "%SCRIPT%" (
  echo Installer script not found after staging: "%SCRIPT%"
  pause
  exit /b 1
)

if not exist "%WATCHER_EXE%" (
  echo watcher.exe could not be staged from "%SRC_ROOT%watcher.exe"
  echo If you launched this from inside a zip preview, try extracting the zip once and rerun.
  pause
  exit /b 1
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -Command ^
  "Start-Process powershell.exe -Verb RunAs -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-STA','-File','\"%SCRIPT%\"')"

endlocal
