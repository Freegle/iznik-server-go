build:
    export GOOS=linux
    export GOARCH=amd64
    go mod download github.com/Azure/go-ansiterm
	go build -o functions/run/online functions/src/online.go