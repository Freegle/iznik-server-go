@echo off
REM Windows batch file for generating Swagger documentation

REM Check for locally downloaded swagger binary first
set LOCAL_SWAGGER_PATH=.\swagger\bin\swagger.exe

if exist "%LOCAL_SWAGGER_PATH%" (
    set SWAGGER_CMD=%LOCAL_SWAGGER_PATH%
    echo Using local swagger binary: %SWAGGER_CMD%
    goto :GENERATE
)

REM No local binary, check for global installation
where swagger >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    set SWAGGER_CMD=swagger
    echo Using globally installed swagger
    goto :GENERATE
)

where swagger.exe >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    set SWAGGER_CMD=swagger.exe
    echo Using globally installed swagger.exe
    goto :GENERATE
)

REM No swagger binary found, try to download it
echo Swagger command not found. Attempting to download...

if exist ".\swagger\download-swagger.bat" (
    call .\swagger\download-swagger.bat
    
    REM Check if download was successful
    if exist "%LOCAL_SWAGGER_PATH%" (
        set SWAGGER_CMD=%LOCAL_SWAGGER_PATH%
        echo Using downloaded swagger binary: %SWAGGER_CMD%
        goto :GENERATE
    ) else (
        echo Failed to download swagger binary
        echo Please run .\swagger\download-swagger.bat manually or install go-swagger
        exit /b 1
    )
) else (
    echo Error: swagger command not found and download script not available
    echo Please install go-swagger first:
    echo   go install github.com/go-swagger/go-swagger/cmd/swagger@v0.30.5
    exit /b 1
)

:GENERATE
echo Generating Swagger documentation...

REM Make sure the swagger directory exists
if not exist "swagger\" mkdir swagger

REM Generate the swagger spec
echo Generating spec with all files...
REM Generate the spec with appropriate parameters
%SWAGGER_CMD% generate spec -o ./swagger/swagger.json --scan-models --include=".*" --exclude=".*/vendor/.*" -m

if %ERRORLEVEL% EQU 0 (
    echo ✅ Swagger spec generated successfully at ./swagger/swagger.json

    REM Validate the swagger spec
    echo Validating the generated spec...
    %SWAGGER_CMD% validate ./swagger/swagger.json

    if %ERRORLEVEL% EQU 0 (
        echo ✅ Swagger spec validation passed
    ) else (
        echo ⚠️ Swagger spec validation has warnings

        REM Check if paths are empty
        findstr "\"paths\": {}" .\swagger\swagger.json >nul
        if %ERRORLEVEL% EQU 0 (
            echo ❌ ERROR: Generated spec doesn't contain any API paths
            echo Make sure your route annotations are correct (see README.md for guidance)
            exit /b 1
        )
    )
) else (
    echo ❌ Failed to generate Swagger spec - please check your swagger command
    exit /b 1
)

echo The Swagger UI is available at http://localhost:8192/swagger/ when the server is running.