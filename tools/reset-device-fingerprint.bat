@echo off
chcp 65001 >nul
echo ========================================
echo Windsurf 设备指纹重置工具
echo ========================================
echo.

echo [1/7] 关闭 Windsurf IDE...
taskkill /F /IM Windsurf.exe 2>nul
taskkill /F /IM language_server_windows_x64.exe 2>nul
taskkill /F /IM devin.exe 2>nul
timeout /t 2 /nobreak >nul

echo [2/7] 删除注册表设备ID...
reg delete "HKCU\SOFTWARE\Microsoft\DeveloperTools" /v deviceid /f 2>nul
if %errorlevel%==0 (
    echo   ✓ 已删除注册表 deviceid
) else (
    echo   - 注册表 deviceid 不存在或已删除
)

echo [3/7] 删除文件系统设备ID...
if exist "%USERPROFILE%\.codeium\windsurf\installation_id" (
    del /F "%USERPROFILE%\.codeium\windsurf\installation_id" 2>nul
    echo   ✓ 已删除 installation_id
) else (
    echo   - installation_id 不存在
)

echo [4/7] 删除 Windsurf storage.json...
if exist "%APPDATA%\Windsurf\storage.json" (
    del /F "%APPDATA%\Windsurf\storage.json" 2>nul
    echo   ✓ 已删除 storage.json (含 machineId, macMachineId, devDeviceId)
) else (
    echo   - storage.json 不存在
)

echo [5/7] 删除认证状态...
if exist "%APPDATA%\Windsurf\User\globalStorage\windsurf_auth.json" (
    del /F "%APPDATA%\Windsurf\User\globalStorage\windsurf_auth.json" 2>nul
    echo   ✓ 已删除 windsurf_auth.json
)
del /F "%APPDATA%\Windsurf\User\globalStorage\windsurf_auth.json.bak.*" 2>nul

echo [6/7] 删除会话缓存和限速状态...
if exist "%APPDATA%\Windsurf\User\globalStorage\state.vscdb" (
    del /F "%APPDATA%\Windsurf\User\globalStorage\state.vscdb" 2>nul
    echo   ✓ 已删除 state.vscdb (含限速缓存)
)
if exist "%APPDATA%\Windsurf\User\globalStorage\state.vscdb.backup" (
    del /F "%APPDATA%\Windsurf\User\globalStorage\state.vscdb.backup" 2>nul
)
if exist "%USERPROFILE%\.codeium\config.json" (
    del /F "%USERPROFILE%\.codeium\config.json" 2>nul
    echo   ✓ 已删除 codeium config.json
)

echo [7/7] 清除 cascade 会话...
if exist "%USERPROFILE%\.codeium\windsurf\cascade\*.pb" (
    del /F "%USERPROFILE%\.codeium\windsurf\cascade\*.pb" 2>nul
    echo   ✓ 已清除 cascade 会话文件
)

echo.
echo ========================================
echo 重置完成！请重新启动 Windsurf IDE
echo 新的设备指纹将在启动时自动生成
echo ========================================
pause
