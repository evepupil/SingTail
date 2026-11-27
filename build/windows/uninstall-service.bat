@echo off
setlocal EnableExtensions

net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [SingTail] Administrator privileges are required to uninstall the system service.
    echo [SingTail] Please right-click this script and choose "Run as administrator".
    pause
    exit /b 1
)

set "SCRIPT_DIR=%~dp0"
set "SERVICE_EXE=%SCRIPT_DIR%singtail-service.exe"

echo [SingTail] Stopping SingTailService...
sc stop SingTailService >nul 2>&1
timeout /t 2 /nobreak >nul

if exist "%SERVICE_EXE%" (
    echo [SingTail] Removing SingTailService...
    "%SERVICE_EXE%" -uninstall
) else (
    echo [SingTail] Removing SingTailService via sc...
    sc delete SingTailService >nul 2>&1
)

if %errorlevel% neq 0 (
    echo [SingTail] Service removal returned code %errorlevel%.
    pause
    exit /b %errorlevel%
)

echo [SingTail] SingTailService removed.
endlocal
