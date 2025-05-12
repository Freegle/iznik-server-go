#!/bin/bash

# Check for swagger binary
SWAGGER_CMD=""
LOCAL_SWAGGER_PATH="./swagger/bin/swagger"
LOCAL_SWAGGER_PATH_WIN="./swagger/bin/swagger.exe"

if [[ -x "$LOCAL_SWAGGER_PATH" ]]; then
    SWAGGER_CMD="$LOCAL_SWAGGER_PATH"
elif [[ -x "$LOCAL_SWAGGER_PATH_WIN" ]]; then
    SWAGGER_CMD="$LOCAL_SWAGGER_PATH_WIN"
elif command -v swagger &> /dev/null; then
    SWAGGER_CMD="swagger"
elif command -v swagger.exe &> /dev/null; then
    SWAGGER_CMD="swagger.exe"
fi

if [[ -n "$SWAGGER_CMD" ]]; then
    echo "Generating Swagger documentation..."

    # Make sure the swagger directory exists
    mkdir -p swagger 2>/dev/null || mkdir swagger 2>/dev/null

    # Generate the swagger spec
    $SWAGGER_CMD generate spec \
      -o ./swagger/swagger.json \
      --scan-models \
      --include=".*" \
      --exclude=".*/vendor/.*" \
      -m

    if [ $? -eq 0 ]; then
        echo "✅ Swagger documentation generated successfully"

        # Check if paths is empty and display a warning
        if grep -q "\"paths\": {}" ./swagger/swagger.json; then
            echo "⚠️ WARNING: Generated spec doesn't contain any API paths"
            echo "Make sure your route annotations are correct (see README.md for guidance)"
        fi
    else
        echo "⚠️ Failed to generate Swagger documentation - continuing with build"
    fi
else
    echo "Skipping Swagger generation - no swagger command found"
fi

# For deploying on Netlify
echo "Building application..."
GOBIN=$(pwd)/functions go install main.go

if [ $? -eq 0 ]; then
    echo "✅ Build completed successfully"
else
    echo "❌ Build failed"
    exit 1
fi