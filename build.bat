@echo off
REM Build script for Windows that includes Swagger generation

REM Check for swagger binary
set SWAGGER_CMD=
set LOCAL_SWAGGER_PATH=.\swagger\bin\swagger.exe

if exist "%LOCAL_SWAGGER_PATH%" (
    set SWAGGER_CMD=%LOCAL_SWAGGER_PATH%
    goto :GENERATE_SWAGGER
)

where swagger >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    set SWAGGER_CMD=swagger
    goto :GENERATE_SWAGGER
)

where swagger.exe >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    set SWAGGER_CMD=swagger.exe
    goto :GENERATE_SWAGGER
) else (
    echo Swagger not found, skipping documentation generation
    goto :BUILD
)

:GENERATE_SWAGGER
echo Generating Swagger documentation...

REM Make sure the swagger directory exists
if not exist "swagger\" mkdir swagger

REM Generate the swagger spec
%SWAGGER_CMD% generate spec -o ./swagger/swagger.json --scan-models --include=".*" --exclude=".*/vendor/.*" -m

if %ERRORLEVEL% EQU 0 (
    echo ✅ Swagger documentation generated successfully

    REM Check if paths are empty
    findstr "\"paths\": {}" .\swagger\swagger.json >nul
    if %ERRORLEVEL% EQU 0 (
        echo ⚠️ WARNING: Generated spec doesn't contain any API paths
        echo Make sure your route annotations are correct (see README.md for guidance)
    )
) else (
    echo ⚠️ Failed to generate Swagger documentation - continuing with build
)

:BUILD
echo Building application...
set GOBIN=%CD%\functions
go install main.go

if %ERRORLEVEL% EQU 0 (
    echo ✅ Build completed successfully
) else (
    echo ❌ Build failed
    exit /b 1
)