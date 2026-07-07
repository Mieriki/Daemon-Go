@echo off
echo Uninstalling ProcessGuard services...
echo.

set "SCRIPT_DIR=%~dp0"
set "BASE_DIR=%SCRIPT_DIR%..\"
cd /d "%BASE_DIR%"

echo Stopping services...
sc stop ProcessGuard-A
sc stop ProcessGuard-B

timeout /t 2 /nobreak >nul

echo.
echo Uninstalling ProcessGuard-A...
"%BASE_DIR%daemon-go.exe" --instance a uninstall

echo.
echo Uninstalling ProcessGuard-B...
"%BASE_DIR%daemon-go.exe" --instance b uninstall

echo.
echo Uninstallation complete.
pause
