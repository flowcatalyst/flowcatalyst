@echo off
setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set PROJECT_DIR=%SCRIPT_DIR%..

:: Load environment if exists
if exist "%PROJECT_DIR%\.env" (
    for /f "usebackq tokens=1,* delims==" %%a in ("%PROJECT_DIR%\.env") do (
        set "line=%%a"
        if not "!line:~0,1!"=="#" (
            if not "%%a"=="" set "%%a=%%b"
        )
    )
)

echo Stopping FlowCatalyst Developer Build...

:: Stop Docker containers (unless using external MongoDB)
if /i "%EXTERNAL_MONGODB%"=="true" (
    echo Using external MongoDB - no Docker containers to stop
) else (
    docker info >nul 2>&1
    if %errorlevel% neq 0 (
        echo Docker not running, skipping container cleanup
    ) else (
        docker compose -f "%PROJECT_DIR%\docker-compose.yml" down
    )
)

echo Stopped.

endlocal
