#!/bin/bash

# Simple test script for Swagger generation
echo "Testing Swagger generation..."

# Get the root directory of the project
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SWAGGER_SCRIPT="${ROOT_DIR}/generate-swagger.sh"
SWAGGER_JSON_PATH="${ROOT_DIR}/swagger/swagger.json"

# Remove existing swagger.json if it exists
rm -f "${SWAGGER_JSON_PATH}"

# Run the generate-swagger.sh script
bash "${SWAGGER_SCRIPT}"

# Check if swagger.json was created
if [ ! -f "${SWAGGER_JSON_PATH}" ]; then
    echo "❌ ERROR: swagger.json was not created"
    exit 1
fi

# Check if paths object exists and is not empty
if grep -q "\"paths\": {}" "${SWAGGER_JSON_PATH}"; then
    echo "❌ ERROR: Paths object is empty in swagger.json"
    exit 1
fi

# Check for specific critical paths
CRITICAL_PATHS=("/message/{ids}" "/activity" "/user/{id}")
for path in "${CRITICAL_PATHS[@]}"; do
    if ! grep -q "\"${path}\"" "${SWAGGER_JSON_PATH}"; then
        echo "❌ ERROR: Critical path ${path} not found in swagger.json"
        exit 1
    fi
done

echo "✅ Swagger generation test passed"
exit 0