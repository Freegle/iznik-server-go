# For deploying on Netlify
go version
rm go.mod
go mod init
go mod tidy
GOBIN=$(pwd)/functions go install ./...