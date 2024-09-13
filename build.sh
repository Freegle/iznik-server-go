# For deploying on Netlify
go mod tidy
go mod download github.com/Azure/go-ansiterm
GOBIN=$(pwd)/functions go install ./...