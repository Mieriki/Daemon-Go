@echo off
chcp 65001 >nul
echo Installing ProcessGuard services...
echo.

set "SCRIPT_DIR=%~dp0"
set "BASE_DIR=%SCRIPT_DIR%..\"
cd /d "%BASE_DIR%"

echo Installing ProcessGuard-A...
"%BASE_DIR%daemon-go.exe" --instance a install
if errorlevel 1 (
    echo Failed to install ProcessGuard-A. Please run as administrator.
    pause
    exit /b 1
)

echo Installing ProcessGuard-B...
"%BASE_DIR%daemon-go.exe" --instance b install
if errorlevel 1 (
    echo Failed to install ProcessGuard-B. Please run as administrator.
    pause
    exit /b 1
)

echo.
echo Starting services...
sc start ProcessGuard-A
sc start ProcessGuard-B

echo.
echo Installation complete.
pause
