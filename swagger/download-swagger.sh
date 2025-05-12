#!/bin/bash

# This script downloads the go-swagger binary directly and uses it locally
# This allows the team to generate Swagger documentation without installing go-swagger globally

# Determine OS and architecture
OS="$(uname)"
ARCH="$(uname -m)"

# Set the version to use
SWAGGER_VERSION="v0.30.5"

# Determine the download URL based on OS and architecture
if [ "$OS" = "Linux" ]; then
  if [ "$ARCH" = "x86_64" ] || [ "$ARCH" = "amd64" ]; then
    DOWNLOAD_URL="https://github.com/go-swagger/go-swagger/releases/download/$SWAGGER_VERSION/swagger_linux_amd64"
  elif [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
    DOWNLOAD_URL="https://github.com/go-swagger/go-swagger/releases/download/$SWAGGER_VERSION/swagger_linux_arm64"
  fi
elif [ "$OS" = "Darwin" ]; then
  if [ "$ARCH" = "x86_64" ] || [ "$ARCH" = "amd64" ]; then
    DOWNLOAD_URL="https://github.com/go-swagger/go-swagger/releases/download/$SWAGGER_VERSION/swagger_darwin_amd64"
  elif [ "$ARCH" = "arm64" ]; then
    DOWNLOAD_URL="https://github.com/go-swagger/go-swagger/releases/download/$SWAGGER_VERSION/swagger_darwin_arm64"
  fi
else
  # Default to Windows - detecting WSL or Git Bash
  if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "win32" ]] || [[ "$OSTYPE" == "cygwin" ]] || grep -q Microsoft /proc/version 2>/dev/null; then
    DOWNLOAD_URL="https://github.com/go-swagger/go-swagger/releases/download/$SWAGGER_VERSION/swagger_windows_amd64.exe"
  fi
fi

if [ -z "$DOWNLOAD_URL" ]; then
  echo "Error: Could not determine download URL for your system ($OS $ARCH)"
  exit 1
fi

SWAGGER_DIR="./swagger/bin"
mkdir -p "$SWAGGER_DIR"

# Download the binary
echo "Downloading swagger from $DOWNLOAD_URL"
if [[ "$DOWNLOAD_URL" == *".exe" ]]; then
  SWAGGER_BIN="$SWAGGER_DIR/swagger.exe"
else
  SWAGGER_BIN="$SWAGGER_DIR/swagger"
fi

curl -L -o "$SWAGGER_BIN" "$DOWNLOAD_URL"
chmod +x "$SWAGGER_BIN"

echo "Swagger binary downloaded to $SWAGGER_BIN"
echo "You can now use this binary to generate Swagger documentation"