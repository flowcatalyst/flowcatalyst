@echo off
setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set PROJECT_DIR=%SCRIPT_DIR%..
set ROOT_DIR=%PROJECT_DIR%\..\..
set DATA_DIR=%PROJECT_DIR%\data

echo.
echo   _____ _                ____      _        _           _
echo  ^|  ___^| ^| _____      __/ ___^|__ _^| ^|_ __ _^| ^|_   _ ___^| ^|_
echo  ^| ^|_  ^| ^|/ _ \ \ /\ / / ^|   / _` ^| __/ _` ^| ^| ^| ^| / __^| __^|
echo  ^|  _^| ^| ^| (_) \ V  V /^| ^|__^| (_^| ^| ^|^| (_^| ^| ^| ^|_^| \__ \ ^|_
echo  ^|_^|   ^|_^|\___/ \_/\_/  \____\__,_^|\__\__,_^|_^|\__, ^|___/\__^|
echo                                               ^|___/
echo.
echo Developer Build - All Services in One
echo ======================================

:: Load environment if exists
if exist "%PROJECT_DIR%\.env" (
    echo Loading environment from .env
    for /f "usebackq tokens=1,* delims==" %%a in ("%PROJECT_DIR%\.env") do (
        set "line=%%a"
        if not "!line:~0,1!"=="#" (
            if not "%%a"=="" set "%%a=%%b"
        )
    )
)

:: Create data directory
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%"

:: Check if using external MongoDB
if /i "%EXTERNAL_MONGODB%"=="true" (
    echo.
    echo Using external MongoDB: %MONGODB_URI%
    echo Note: External MongoDB must be a replica set for change streams
) else (
    :: Check if Docker is running
    docker info >nul 2>&1
    if %errorlevel% neq 0 (
        echo Warning: Docker is not running. MongoDB will not be started.
        echo Options:
        echo   1. Start Docker and re-run this script
        echo   2. Set EXTERNAL_MONGODB=true and MONGODB_URI to use your own MongoDB
        exit /b 1
    ) else (
        :: Start MongoDB
        echo.
        echo Starting MongoDB via Docker...
        docker compose -f "%PROJECT_DIR%\docker-compose.yml" up -d mongo

        :: Wait for MongoDB
        echo Waiting for MongoDB replica set to initialize...
        timeout /t 10 /nobreak >nul
    )
)

:: Check for native executable
set NATIVE_EXE=%PROJECT_DIR%\build\flowcatalyst-dev-build-1.0.0-SNAPSHOT-runner.exe
if exist "%NATIVE_EXE%" (
    echo.
    echo Starting FlowCatalyst (native executable)...
    cd /d "%PROJECT_DIR%"
    "%NATIVE_EXE%"
) else (
    echo.
    echo Starting FlowCatalyst (JVM mode via Gradle)...
    echo Tip: Build native with:
    echo   gradlew :core:flowcatalyst-dev-build:build -Dquarkus.native.enabled=true
    echo.
    cd /d "%ROOT_DIR%"
    call gradlew :core:flowcatalyst-dev-build:quarkusDev
)

endlocal
