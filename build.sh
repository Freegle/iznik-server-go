# For deploying on Netlify
go version
go mod tidy
GOBIN=$(pwd)/functions go install ./...