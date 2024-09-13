# For deploying on Netlify
GOBIN=$(pwd)/functions go build -o cmd/gateway/gateway main.go