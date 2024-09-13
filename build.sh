# For deploying on Netlify
set -euxo pipefail

mkdir -p "$(pwd)/functions"
go mod download github.com/Azure/go-ansiterm
go mod tidy
GOBIN=$(pwd)/functions go install ./...
chmod +x "$(pwd)"/functions/*
go env