# For deploying on Netlify
go version
go get iznik-server-go
go get iznik-server-go/adapter
go get iznik-server-go/handler
GOBIN=$(pwd)/functions go install ./...