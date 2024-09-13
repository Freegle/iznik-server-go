# For deploying on Netlify
rm go.mod go.sum
go mod tidy
GOBIN=$(pwd)/functions go install ./...