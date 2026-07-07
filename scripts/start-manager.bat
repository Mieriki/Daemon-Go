@echo off
chcp 65001 >nul
setlocal EnableDelayedExpansion

set "SCRIPT_DIR=%~dp0"
set "BASE_DIR=%SCRIPT_DIR%..\"
cd /d "%BASE_DIR%"

set "TOKEN="
if exist ".guard" (
    for /f "usebackq eol=# delims=" %%i in (".guard") do (
        set "LINE=%%i"
        if /I "!LINE:~0,6!"=="token=" (
            set "TOKEN=!LINE:~6!"
            goto :found
        )
    )
)

:found
if "%TOKEN%"=="" (
    echo Token not found. Please install and start ProcessGuard services first.
    echo Current directory: %CD%
    pause
    exit /b 1
)

echo Opening ProcessGuard console...
start "" "http://127.0.0.1:18080/?token=%TOKEN%"
endlocal
