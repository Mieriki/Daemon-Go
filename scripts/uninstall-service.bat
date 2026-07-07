@echo off
chcp 65001 >nul
echo 正在卸载进程守护服务...
echo.

:: 获取当前目录
set "DIR=%~dp0"
cd /d "%DIR%"

:: 停止服务
echo 停止服务...
sc stop ProcessGuard-A
sc stop ProcessGuard-B

timeout /t 2 /nobreak >nul

:: 卸载服务
echo.
echo 卸载 A 服务...
"%DIR%daemon-go.exe" --instance a uninstall

echo.
echo 卸载 B 服务...
"%DIR%daemon-go.exe" --instance b uninstall

echo.
echo 服务卸载完成。
pause
