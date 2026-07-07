@echo off
echo Restarting ProcessGuard-A service...
sc stop ProcessGuard-A
:wait_stop
sc query ProcessGuard-A | findstr /C:"STOPPED" >nul
if errorlevel 1 (
    timeout /t 1 /nobreak >nul
    goto wait_stop
)
sc start ProcessGuard-A
echo Restart complete.
pause
