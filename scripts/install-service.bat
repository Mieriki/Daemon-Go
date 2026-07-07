@echo off
chcp 65001 >nul
echo 正在安装进程守护服务...
echo.

:: 获取当前目录
set "DIR=%~dp0"
cd /d "%DIR%"

:: 安装 A 服务（主守护）
echo 安装主守护服务 ProcessGuard-A...
"%DIR%daemon-go.exe" --instance a install
if errorlevel 1 (
    echo 安装 A 服务失败，请检查是否以管理员身份运行。
    pause
    exit /b 1
)

:: 安装 B 服务（备守护）
echo 安装备守护服务 ProcessGuard-B...
"%DIR%daemon-go.exe" --instance b install
if errorlevel 1 (
    echo 安装 B 服务失败，请检查是否以管理员身份运行。
    pause
    exit /b 1
)

:: 启动服务
echo.
echo 启动服务...
sc start ProcessGuard-A
sc start ProcessGuard-B

echo.
echo 服务安装完成。
pause
