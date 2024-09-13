# For deploying on Netlify
go version
go get iznik-server-go
GOBIN=$(pwd)/functions go install ./...