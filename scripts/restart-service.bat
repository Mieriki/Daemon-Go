@echo off
chcp 65001 >nul
echo Restarting ProcessGuard services...
echo.

set "SCRIPT_DIR=%~dp0"
set "BASE_DIR=%SCRIPT_DIR%..\"
cd /d "%BASE_DIR%"

echo Stopping ProcessGuard-B...
sc stop ProcessGuard-B

echo Stopping ProcessGuard-A...
sc stop ProcessGuard-A

echo Waiting for services to stop...
:wait_loop
sc query ProcessGuard-A | findstr "STOPPED" > nul
if errorlevel 1 (
    timeout /t 1 /nobreak > nul
    goto wait_loop
)
sc query ProcessGuard-B | findstr "STOPPED" > nul
if errorlevel 1 (
    timeout /t 1 /nobreak > nul
    goto wait_loop
)

echo.
echo Starting ProcessGuard-A...
sc start ProcessGuard-A

echo Starting ProcessGuard-B...
sc start ProcessGuard-B

echo.
echo Restart complete.
pause
