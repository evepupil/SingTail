@echo off
setlocal EnableExtensions

net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [SingTail] Administrator privileges are required to install the system service.
    echo [SingTail] Please right-click this script and choose "Run as administrator".
    pause
    exit /b 1
)

set "SCRIPT_DIR=%~dp0"
set "SERVICE_EXE=%SCRIPT_DIR%singtail-service.exe"

if not exist "%SERVICE_EXE%" (
    echo [SingTail] singtail-service.exe not found in %SCRIPT_DIR%
    pause
    exit /b 1
)

echo [SingTail] Stopping existing SingTailService if running...
sc stop SingTailService >nul 2>&1
timeout /t 2 /nobreak >nul

echo [SingTail] Registering SingTailService...
"%SERVICE_EXE%" -install
if %errorlevel% neq 0 (
    echo [SingTail] Failed to register SingTailService.
    pause
    exit /b %errorlevel%
)

echo [SingTail] Starting SingTailService...
sc start SingTailService
if %errorlevel% neq 0 (
    echo [SingTail] Service registered, check Windows Services if startup is pending.
) else (
    echo [SingTail] SingTailService installed and running.
)

endlocal
