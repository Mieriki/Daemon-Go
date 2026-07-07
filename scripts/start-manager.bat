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
    echo 未找到 .guard 文件或 Token，请先安装并启动 ProcessGuard 服务。
    echo 当前目录：%CD%
    pause
    exit /b 1
)

echo 正在打开 ProcessGuard 控制台...
start "" "http://127.0.0.1:18080/?token=%TOKEN%"
endlocal
