@echo off
setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set PROJECT_DIR=%SCRIPT_DIR%..
set DATA_DIR=%PROJECT_DIR%\data

:: Load environment if exists
if exist "%PROJECT_DIR%\.env" (
    for /f "usebackq tokens=1,* delims==" %%a in ("%PROJECT_DIR%\.env") do (
        set "line=%%a"
        if not "!line:~0,1!"=="#" (
            if not "%%a"=="" set "%%a=%%b"
        )
    )
)

echo FlowCatalyst Developer Build - Reset
echo ======================================
echo.

if /i "%EXTERNAL_MONGODB%"=="true" (
    echo WARNING: This will delete SQLite queue databases.
    echo Note: Using external MongoDB - you must reset that separately.
) else (
    echo WARNING: This will delete ALL data including:
    echo   - MongoDB data (all collections)
    echo   - SQLite queue databases
)

echo.
set /p CONFIRM="Are you sure you want to continue? (y/N) "

if /i "%CONFIRM%"=="y" (
    echo.

    if /i "%EXTERNAL_MONGODB%"=="true" (
        echo Using external MongoDB - skipping Docker cleanup
    ) else (
        echo Stopping containers and removing volumes...
        docker info >nul 2>&1
        if %errorlevel% neq 0 (
            echo Docker not running, skipping container cleanup
        ) else (
            docker compose -f "%PROJECT_DIR%\docker-compose.yml" --profile tools down -v
        )
    )

    echo Removing SQLite databases...
    if exist "%DATA_DIR%\*.db" del /q "%DATA_DIR%\*.db"

    echo.
    echo Reset complete.
    echo Run scripts\start.bat to start fresh.
) else (
    echo Cancelled.
)

endlocal
