#!/bin/bash

# Check for swagger binary
SWAGGER_CMD=""
LOCAL_SWAGGER_PATH="./swagger/bin/swagger"
LOCAL_SWAGGER_PATH_WIN="./swagger/bin/swagger.exe"

if [[ -x "$LOCAL_SWAGGER_PATH" ]]; then
    SWAGGER_CMD="$LOCAL_SWAGGER_PATH"
    echo "Using local swagger binary: $SWAGGER_CMD"
elif [[ -x "$LOCAL_SWAGGER_PATH_WIN" ]]; then
    SWAGGER_CMD="$LOCAL_SWAGGER_PATH_WIN"
    echo "Using local swagger binary: $SWAGGER_CMD"
elif command -v swagger &> /dev/null; then
    SWAGGER_CMD="swagger"
    echo "Using globally installed swagger"
elif command -v swagger.exe &> /dev/null; then
    SWAGGER_CMD="swagger.exe"
    echo "Using globally installed swagger.exe"
else
    # No swagger binary found, try to download it
    echo "Swagger command not found. Attempting to download..."

    if [[ -f "./swagger/download-swagger.sh" ]]; then
        bash ./swagger/download-swagger.sh

        # Check if download was successful
        if [[ -x "$LOCAL_SWAGGER_PATH" ]]; then
            SWAGGER_CMD="$LOCAL_SWAGGER_PATH"
            echo "Using downloaded swagger binary: $SWAGGER_CMD"
        elif [[ -x "$LOCAL_SWAGGER_PATH_WIN" ]]; then
            SWAGGER_CMD="$LOCAL_SWAGGER_PATH_WIN"
            echo "Using downloaded swagger binary: $SWAGGER_CMD"
        else
            echo "❌ Failed to download swagger binary"
            echo "Please run ./swagger/download-swagger.sh manually or install go-swagger"
            exit 1
        fi
    else
        echo "❌ Error: swagger command not found and download script not available"
        echo "Please install go-swagger first:"
        echo "  go install github.com/go-swagger/go-swagger/cmd/swagger@v0.30.5"
        exit 1
    fi
fi

echo "Generating Swagger documentation..."

# Make sure the swagger directory exists
mkdir -p swagger 2>/dev/null || mkdir swagger 2>/dev/null

# Generate the swagger spec
echo "Generating Swagger specification..."
$SWAGGER_CMD generate spec \
  -o ./swagger/swagger.json \
  --scan-models \
  --include=".*" \
  --exclude=".*/vendor/.*" \
  -m

if [ $? -ne 0 ]; then
    echo "❌ Failed to generate Swagger documentation"
    exit 1
fi

echo "✅ Swagger documentation generated successfully"

# Check if paths is empty and error out
if grep -q "\"paths\": {}" ./swagger/swagger.json; then
    echo "❌ ERROR: Generated Swagger spec doesn't contain any API paths"
    echo "Make sure your route annotations are correct in swagger/swagger.go (see README.md for guidance)"
    exit 1
fi

# Validate the swagger spec
echo "Validating Swagger specification..."
$SWAGGER_CMD validate ./swagger/swagger.json

if [ $? -ne 0 ]; then
    echo "❌ Swagger specification validation failed"
    exit 1
fi

echo "✅ Swagger specification validation passed"

# For deploying on Netlify
echo "Building application..."
GOBIN=$(pwd)/functions go install main.go

if [ $? -eq 0 ]; then
    echo "✅ Build completed successfully"
    echo "Swagger UI is available at http://localhost:8192/swagger/ when the server is running"
else
    echo "❌ Build failed"
    exit 1
fi