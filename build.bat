@echo off
REM Build script for Windows that includes Swagger generation

REM Check for swagger binary
set SWAGGER_CMD=
set LOCAL_SWAGGER_PATH=.\swagger\bin\swagger.exe

if exist "%LOCAL_SWAGGER_PATH%" (
    set SWAGGER_CMD=%LOCAL_SWAGGER_PATH%
    echo Using local swagger binary: %SWAGGER_CMD%
    goto :GENERATE_SWAGGER
)

where swagger >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    set SWAGGER_CMD=swagger
    echo Using globally installed swagger
    goto :GENERATE_SWAGGER
)

where swagger.exe >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    set SWAGGER_CMD=swagger.exe
    echo Using globally installed swagger.exe
    goto :GENERATE_SWAGGER
) else (
    echo Swagger command not found. Attempting to download...

    if exist ".\swagger\download-swagger.bat" (
        call .\swagger\download-swagger.bat

        REM Check if download was successful
        if exist "%LOCAL_SWAGGER_PATH%" (
            set SWAGGER_CMD=%LOCAL_SWAGGER_PATH%
            echo Using downloaded swagger binary: %SWAGGER_CMD%
            goto :GENERATE_SWAGGER
        ) else (
            echo ❌ Failed to download swagger binary
            echo Please run .\swagger\download-swagger.bat manually or install go-swagger
            exit /b 1
        )
    ) else (
        echo ❌ Error: swagger command not found and download script not available
        echo Please install go-swagger first:
        echo   go install github.com/go-swagger/go-swagger/cmd/swagger@v0.30.5
        exit /b 1
    )
)

:GENERATE_SWAGGER
echo Generating Swagger documentation...

REM Make sure the swagger directory exists
if not exist "swagger\" mkdir swagger

REM Generate the swagger spec
echo Generating Swagger specification...
%SWAGGER_CMD% generate spec -o ./swagger/swagger.json --scan-models --include=".*" --exclude=".*/vendor/.*" -m

if %ERRORLEVEL% NEQ 0 (
    echo ❌ Failed to generate Swagger documentation
    exit /b 1
)

echo ✅ Swagger documentation generated successfully

REM Check if paths are empty and error out
findstr "\"paths\": {}" .\swagger\swagger.json >nul
if %ERRORLEVEL% EQU 0 (
    echo ❌ ERROR: Generated Swagger spec doesn't contain any API paths
    echo Make sure your route annotations are correct in swagger/swagger.go (see README.md for guidance)
    exit /b 1
)

REM Validate the swagger spec
echo Validating Swagger specification...
%SWAGGER_CMD% validate ./swagger/swagger.json

if %ERRORLEVEL% NEQ 0 (
    echo ❌ Swagger specification validation failed
    exit /b 1
)

echo ✅ Swagger specification validation passed

:BUILD
echo Building application...
set GOBIN=%CD%\functions
go install main.go

if %ERRORLEVEL% EQU 0 (
    echo ✅ Build completed successfully
    echo Swagger UI is available at http://localhost:8192/swagger/ when the server is running
) else (
    echo ❌ Build failed
    exit /b 1
)