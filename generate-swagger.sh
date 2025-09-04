#!/bin/bash

# Make sure the script is executable (on Linux/macOS)
# chmod +x generate-swagger.sh

# Check for locally downloaded swagger binary first
LOCAL_SWAGGER_PATH="./swagger/bin/swagger"
LOCAL_SWAGGER_PATH_WIN="./swagger/bin/swagger.exe"

SWAGGER_CMD=""

# Check for globally installed swagger first (preferred method)
if command -v swagger &> /dev/null; then
    SWAGGER_CMD="swagger"
    echo "Using globally installed swagger"
elif command -v swagger.exe &> /dev/null; then
    SWAGGER_CMD="swagger.exe" 
    echo "Using globally installed swagger.exe"
# Fall back to local binaries if available
elif [[ -x "$LOCAL_SWAGGER_PATH" ]]; then
    SWAGGER_CMD="$LOCAL_SWAGGER_PATH"
    echo "Using local swagger binary: $SWAGGER_CMD"
elif [[ -x "$LOCAL_SWAGGER_PATH_WIN" ]]; then
    SWAGGER_CMD="$LOCAL_SWAGGER_PATH_WIN"
    echo "Using local swagger binary: $SWAGGER_CMD"
else
    # No swagger found, install it via go install
    echo "Swagger command not found. Installing via go install..."
    
    if command -v go &> /dev/null; then
        go install github.com/go-swagger/go-swagger/cmd/swagger@latest
        
        # Check if installation was successful
        if command -v swagger &> /dev/null; then
            SWAGGER_CMD="swagger"
            echo "Successfully installed go-swagger"
        else
            echo "go install completed but swagger not found in PATH"
            echo "Make sure $(go env GOPATH)/bin is in your PATH"
            exit 1
        fi
    else
        echo "Error: Go not found. Please install Go first or manually install go-swagger"
        exit 1
    fi
fi

echo "Generating Swagger documentation..."

# Make sure the swagger directory exists (works on both Windows and Unix)
mkdir -p swagger 2>/dev/null || mkdir swagger 2>/dev/null

# Generate the swagger spec
echo "Generating spec with all files..."
# Generate the spec with appropriate parameters based on the swagger version

# First try with the base dir parameter, excluding test files which might cause issues
$SWAGGER_CMD generate spec \
  -o ./swagger/swagger.json \
  --scan-models \
  --include=".*" \
  --exclude=".*/vendor/.*" \
  --exclude=".*/test/.*" \
  -m

if [ $? -eq 0 ]; then
    echo "✅ Swagger spec generated successfully at ./swagger/swagger.json"

    # Validate the swagger spec
    echo "Validating the generated spec..."
    $SWAGGER_CMD validate ./swagger/swagger.json

    if [ $? -eq 0 ]; then
        echo "✅ Swagger spec validation passed"
    else
        echo "⚠️ Swagger spec validation has warnings"
        
        # Check if paths is empty and display an error
        if grep -q "\"paths\": {}" ./swagger/swagger.json; then
            echo "❌ ERROR: Generated spec doesn't contain any API paths"
            echo "Make sure your route annotations are correct (see README.md for guidance)"
            exit 1
        fi
    fi
else
    echo "❌ Failed to generate Swagger spec - please check your swagger command"
    exit 1
fi

echo "The Swagger UI is available at http://localhost:8192/swagger/ when the server is running."