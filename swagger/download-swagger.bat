@echo off
REM This script downloads the go-swagger binary directly and uses it locally
REM This allows the team to generate Swagger documentation without installing go-swagger globally

REM Set the version to use
set SWAGGER_VERSION=v0.30.5

REM Use Windows binary
set DOWNLOAD_URL=https://github.com/go-swagger/go-swagger/releases/download/%SWAGGER_VERSION%/swagger_windows_amd64.exe

set SWAGGER_DIR=.\swagger\bin
if not exist "%SWAGGER_DIR%" mkdir "%SWAGGER_DIR%"

REM Download the binary
echo Downloading swagger from %DOWNLOAD_URL%
set SWAGGER_BIN=%SWAGGER_DIR%\swagger.exe

REM Use PowerShell to download the file
powershell -Command "(New-Object System.Net.WebClient).DownloadFile('%DOWNLOAD_URL%', '%SWAGGER_BIN%')"

if %ERRORLEVEL% NEQ 0 (
    echo Failed to download swagger binary
    exit /b 1
)

echo Swagger binary downloaded to %SWAGGER_BIN%
echo You can now use this binary to generate Swagger documentation